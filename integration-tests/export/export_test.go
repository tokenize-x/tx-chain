//go:build integrationtests

package export_test

import (
	"bytes"
	"flag"
	"fmt"
	"strings"
	"testing"

	sdkstore "cosmossdk.io/core/store"
	nftkeeper "cosmossdk.io/x/nft/keeper"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	ibchost "github.com/cosmos/ibc-go/v10/modules/core/24-host"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/app"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
)

var nodeAppDir, exportedGenesisPath string

func init() {
	flag.StringVar(&nodeAppDir, "node-app-dir", "", "Node application directory to use for export tests.")
	flag.StringVar(&exportedGenesisPath, "exported-genesis-path", "", "Path to the exported genesis file.")
}

// ignoredPrefixes defines prefixes of keys that should be ignored during KVStore comparison.
// These keys are typically used for internal state management and do not affect the exported genesis.
var ignoredPrefixes = map[string][][]byte{
	stakingtypes.StoreKey: {
		stakingtypes.UnbondingIDKey,
		stakingtypes.UnbondingIndexKey,
		stakingtypes.UnbondingTypeKey,
		stakingtypes.UnbondingQueueKey,
		stakingtypes.HistoricalInfoKey,
	},
	distributiontypes.StoreKey: {
		distributiontypes.FeePoolKey,
		distributiontypes.ProposerKey,
	},
	nftkeeper.StoreKey: {
		nftkeeper.ClassTotalSupply,
	},
	ibcexported.StoreKey: {
		ibchost.KeyClientStorePrefix,
	},
	authzkeeper.StoreKey: {
		authzkeeper.GrantQueuePrefix,
	},
}

// TestExportGenesisModuleHashes tests the export of genesis and compares the module hashes
// steps:
// 1. Parse genesis and application state from a val/full node.
// 2. Initialize a new app with the exported genesis.
// 3. Move both apps to the same height by finalizing a block.
// 4. Compare the module hashes of both apps to ensure they match.
func TestExportGenesisModuleHashes(t *testing.T) {
	requireT := require.New(t)

	// the chain is stopped and the genesis is exported from a val/full node
	exportedApp, exportedGenesisBuf, err := simapp.ParseExportedGenesisAndApp(nodeAppDir, exportedGenesisPath)
	requireT.NoError(err, "failed to parse exported genesis")

	// the exported genesis is used to initialize a new app to simulate new chain initialization
	//nolint:dogsled
	initiatedApp, _, _, initChainReq, _ := simapp.NewWithGenesis(exportedGenesisBuf.Bytes())

	// sync heights of both apps stores
	syncAppsHeights(t, requireT, exportedApp, &initiatedApp.App, initChainReq)

	// check that the module hashes of both apps match
	checkModuleStoreMismatches(t, requireT, exportedApp, &initiatedApp.App, initChainReq.InitialHeight)
}

func syncAppsHeights(
	t *testing.T, requireT *require.Assertions,
	exportedApp *app.App, initiatedApp *app.App,
	initChainReq *abci.RequestInitChain,
) {
	// load the latest version from the exported app
	// the initial height is the height that need to gets finalized in the initiated app
	nodeAppStateHeight := initChainReq.InitialHeight - 1
	err := exportedApp.LoadVersion(nodeAppStateHeight)
	requireT.NoError(err, "failed to load version %d from exported app", nodeAppStateHeight)

	// finalize new block for the exported app
	_, err = exportedApp.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: initChainReq.InitialHeight,
	})
	require.NoError(t, err)
	_, err = exportedApp.Commit()
	require.NoError(t, err)

	// finalize new block for the initiated app
	_, err = initiatedApp.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: initChainReq.InitialHeight,
	})
	require.NoError(t, err)

	_, err = initiatedApp.Commit()
	require.NoError(t, err)
}

func checkModuleStoreMismatches(
	t *testing.T, requireT *require.Assertions,
	exportedApp *app.App, initiatedApp *app.App,
	height int64,
) {
	var mismatches []string

	// ensure the app contexts are created for the specified height
	exportedAppCtx := exportedApp.NewUncachedContext(false, cmtproto.Header{Height: height})
	initiatedAppCtx := initiatedApp.NewUncachedContext(false, cmtproto.Header{Height: height})

	for _, moduleName := range exportedApp.ModuleManager.ModuleNames() {
		// skip the module if it is in the ignored list
		if _, ok := simapp.IgnoredModulesForExport[moduleName]; ok {
			continue
		}

		// auth module store name is different from its module name
		// so we use a special case for it
		storeName := moduleName
		if moduleName == authtypes.StoreKey {
			storeName = "acc"
		}

		// list the prefixes to ignore for the module
		var modulePrefixesToIgnore [][]byte
		if prefixes, ok := ignoredPrefixes[moduleName]; ok {
			modulePrefixesToIgnore = prefixes
		}

		var nodeAppKvStore, initiatedAppKvStore sdkstore.KVStore

		exportedStoreKey := exportedApp.GetKey(storeName)
		if exportedStoreKey != nil {
			nodeAppKvStoreService := runtime.NewKVStoreService(exportedStoreKey)
			nodeAppKvStore = nodeAppKvStoreService.OpenKVStore(exportedAppCtx)
		}

		initiatedAppExportedStoreKey := initiatedApp.GetKey(storeName)
		if initiatedAppExportedStoreKey != nil {
			initiatedAppKvStoreService := runtime.NewKVStoreService(initiatedAppExportedStoreKey)
			initiatedAppKvStore = initiatedAppKvStoreService.OpenKVStore(initiatedAppCtx)
		}

		if nodeAppKvStore == nil {
			if initiatedAppKvStore != nil {
				// means that the module has a KVStore in the initiated app, but not in the exported app
				mismatches = append(mismatches, fmt.Sprintf("KVStore %s not found in exported app", storeName))
			}
			// means that the module does not have a KVStore in both apps, so we skip the comparison
			continue
		}

		t.Logf("Comparing KVStore %s at height %d", storeName, height)
		// compare the KVStores of the exported app and the initiated app
		// and append any mismatches to the list
		err := compareKVStores(nodeAppKvStore, initiatedAppKvStore, modulePrefixesToIgnore)
		if err != nil {
			mismatches = append(mismatches, fmt.Sprintf("failed to compare %s KV stores: %v", storeName, err))
		}
	}

	requireT.Empty(mismatches, "KVStore mismatches:\n%s", strings.Join(mismatches, "\n"))
}

func compareKVStores(exportedAppStore, initiatedAppStore sdkstore.KVStore, ignorePrefixes [][]byte) error {
	// build maps of key-value pairs for exported app store
	exportedMap := make(map[string][]byte)
	iter1, err := exportedAppStore.Iterator(nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create iterator for exported app store: %w", err)
	}
	defer iter1.Close()
	for ; iter1.Valid(); iter1.Next() {
		exportedMap[string(iter1.Key())] = append([]byte(nil), iter1.Value()...)
	}

	// build maps of key-value pairs for initiated app store
	initiatedMap := make(map[string][]byte)
	iter2, err := initiatedAppStore.Iterator(nil, nil)
	if err != nil {
		return fmt.Errorf("failed to create iterator for initiated app store: %w", err)
	}
	defer iter2.Close()
	for ; iter2.Valid(); iter2.Next() {
		initiatedMap[string(iter2.Key())] = append([]byte(nil), iter2.Value()...)
	}

	var mismatches []string

	// Check for keys in exportedMap not in initiatedMap or with different values
	for k, v := range exportedMap {
		// Skip keys with any of the ignorePrefixes
		if lo.ContainsBy(ignorePrefixes, func(prefix []byte) bool {
			return bytes.HasPrefix([]byte(k), prefix)
		}) {
			continue
		}

		// check if the key  exists in the initiated app store
		// if the key is not found, append the mismatch to the list
		nv, ok := initiatedMap[k]
		if !ok {
			mismatches = append(mismatches, fmt.Sprintf("key %q missing in initiated app store", k))
			continue
		}
		// check if the value matches
		// if the value is not equal, append the mismatch to the list
		if !bytes.Equal(v, nv) {
			mismatches = append(mismatches, fmt.Sprintf("value mismatch for key %q: %q vs %q", k, v, nv))
		}
	}

	// Check for extra keys in initiated app store
	for k := range initiatedMap {
		// Skip keys with any of the ignorePrefixes
		ignored := false
		for _, prefix := range ignorePrefixes {
			if bytes.HasPrefix([]byte(k), prefix) {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}

		// check if the key exists in the exported app store
		// if the key is not found, append the mismatch to the list
		if _, ok := exportedMap[k]; !ok {
			mismatches = append(mismatches, fmt.Sprintf("extra key %X in new store", []byte(k)))
		}
	}

	// If there are any mismatches, return an error with the details
	if len(mismatches) > 0 {
		return fmt.Errorf("KVStore mismatches:\n%s", strings.Join(mismatches, "\n"))
	}
	return nil
}
