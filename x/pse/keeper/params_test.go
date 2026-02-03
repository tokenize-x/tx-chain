package keeper_test

import (
	"fmt"
	"math"
	"runtime"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestGetSetParams(t *testing.T) {
	requireT := require.New(t)
	assertT := assert.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	// Test getting default params
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Empty(params.ExcludedAddresses)

	// Test setting params with excluded addresses
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	newParams := types.Params{
		ExcludedAddresses: []string{addr1, addr2},
	}

	err = pseKeeper.SetParams(ctx, newParams)
	requireT.NoError(err)

	// Verify params were set correctly
	params, err = pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Len(params.ExcludedAddresses, 2)
	assertT.Contains(params.ExcludedAddresses, addr1)
	assertT.Contains(params.ExcludedAddresses, addr2)

	// Test setting params with empty excluded addresses
	emptyParams := types.Params{
		ExcludedAddresses: []string{},
	}

	err = pseKeeper.SetParams(ctx, emptyParams)
	requireT.NoError(err)

	params, err = pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Empty(params.ExcludedAddresses)
}

func TestUpdateExcludedAddresses(t *testing.T) {
	requireT := require.New(t)
	assertT := assert.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	// Use correct authority unless test specifies otherwise
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr5 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	invalidAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name             string
		authority        string
		initialAddresses []string
		addressesToAdd   []string
		addressesToRem   []string
		expectedFinal    []string
		expectErr        bool
		errMsg           string
	}{
		{
			name:             "add_to_empty_list",
			initialAddresses: []string{},
			addressesToAdd:   []string{addr1},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1},
			expectErr:        false,
		},
		{
			name:             "add_multiple_addresses",
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2, addr3},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2, addr3},
			expectErr:        false,
		},
		{
			name:             "remove_existing_address",
			initialAddresses: []string{addr1, addr2, addr3},
			addressesToAdd:   []string{},
			addressesToRem:   []string{addr2},
			expectedFinal:    []string{addr1, addr3},
			expectErr:        false,
		},
		{
			name:             "add_and_remove_different_addresses",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr3, addr4},
			addressesToRem:   []string{addr1},
			expectedFinal:    []string{addr2, addr3, addr4},
			expectErr:        false,
		},
		{
			name:             "remove_nonexistent_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{},
			addressesToRem:   []string{addr5},
			expectedFinal:    []string{addr1, addr2},
			expectErr:        false,
		},
		{
			name:             "add_duplicate_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr1},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2}, // Should not add duplicate
			expectErr:        false,
		},
		{
			name:             "remove_then_add_different_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr3},
			addressesToRem:   []string{addr1},
			expectedFinal:    []string{addr2, addr3},
			expectErr:        false,
		},
		{
			name:             "add_multiple_including_duplicate",
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2, addr1, addr3},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2, addr3}, // addr1 should not be duplicated
			expectErr:        false,
		},
		{
			name:             "invalid_authority",
			authority:        invalidAuthority,
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2},
			addressesToRem:   []string{},
			expectErr:        true,
			errMsg:           "expected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.authority != "" {
				authority = tc.authority
			}

			// Set initial params
			initialParams := types.Params{
				ExcludedAddresses: tc.initialAddresses,
			}
			err := pseKeeper.SetParams(ctx, initialParams)
			requireT.NoError(err)

			// Update excluded addresses
			err = pseKeeper.UpdateExcludedAddresses(ctx, authority, tc.addressesToAdd, tc.addressesToRem)

			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)

				// Verify final state
				params, err := pseKeeper.GetParams(ctx)
				requireT.NoError(err)
				assertT.Len(params.ExcludedAddresses, len(tc.expectedFinal))

				for _, expectedAddr := range tc.expectedFinal {
					assertT.Contains(params.ExcludedAddresses, expectedAddr)
				}
			}
		})
	}
}

func TestUpdateClearingMappings_Authority(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	correctAuthority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	wrongAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Must include all eligible clearing accounts
	var mappings []types.ClearingAccountMapping
	for _, clearingAccount := range types.GetNonCommunityClearingAccounts() {
		mappings = append(mappings, types.ClearingAccountMapping{
			ClearingAccount:    clearingAccount,
			RecipientAddresses: []string{addr1},
		})
	}

	// Test with wrong authority
	err := pseKeeper.UpdateClearingAccountMappings(ctx, wrongAuthority, mappings)
	requireT.Error(err, "should reject wrong authority")
	requireT.Contains(err.Error(), "invalid authority")

	// Test with correct authority
	err = pseKeeper.UpdateClearingAccountMappings(ctx, correctAuthority, mappings)
	requireT.NoError(err, "should accept correct authority")
}

func TestCollectionsMapIteratorMemoryUsage(t *testing.T) {
	requireT := require.New(t)

	testCases := []struct {
		name           string
		closeImmediate bool // true = close after each iteration, false = defer all closes
	}{
		{
			name:           "close_immediate",
			closeImmediate: true,
		},
		{
			name:           "close_deferred",
			closeImmediate: false,
		},
	}

	humanize := func(i int64) string {
		prefix := ""
		if i < 0 {
			prefix = "-"
			i *= -1
		}
		s := uint64(i)
		if s == 0 {
			return "0 bytes"
		}

		units := []string{"", "Kilo", "Mega", "Giga", "Tera"}

		// Find the appropriate unit
		index := 0
		value := float64(s)
		for value >= 1024 && index < len(units)-1 {
			value /= 1024
			index++
		}

		// Round to 2 decimal places
		rounded := math.Round(value*100) / 100

		return fmt.Sprintf("%s%.2f %sbytes", prefix, rounded, units[index])
	}
	diff := func(after, before uint64) int64 {
		if after >= before {
			return int64(after) - int64(before)
		}
		return -(int64(before) - int64(after))
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testApp := simapp.New()
			ctx := testApp.NewContext(false)
			pseKeeper := testApp.PSEKeeper

			// Force GC before starting to get baseline memory
			runtime.GC()
			var memStatsBefore runtime.MemStats
			runtime.ReadMemStats(&memStatsBefore)

			// Add a lot of data to AccountScoreSnapshot map
			const numEntries = 10000
			values := make([]sdkmath.Int, 0, numEntries)

			// Populate the map and keep track of values
			for i := 0; i < numEntries; i++ {
				addr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
				value := sdkmath.NewInt(int64(i + 1))
				values = append(values, value)

				err := pseKeeper.AccountScoreSnapshot.Set(ctx, addr, value)
				requireT.NoError(err)
			}

			// Collect all keys first
			iter, err := pseKeeper.AccountScoreSnapshot.Iterate(ctx, nil)
			requireT.NoError(err)
			defer iter.Close()

			keys := make([]sdk.AccAddress, 0, numEntries)
			for ; iter.Valid(); iter.Next() {
				kv, err := iter.KeyValue()
				requireT.NoError(err)
				keys = append(keys, kv.Key)
			}
			iter.Close()

			// For each key, create a new iterator to find matching value
			for _, key := range keys {
				// Get the expected value for this key
				expectedValue, err := pseKeeper.AccountScoreSnapshot.Get(ctx, key)
				requireT.NoError(err)

				// Create a new iterator to search for this value
				searchIter, err := pseKeeper.AccountScoreSnapshot.Iterate(ctx, nil)
				requireT.NoError(err)

				if !tc.closeImmediate {
					defer searchIter.Close()
				}

				// Search for the matching value
				found := false
				for ; searchIter.Valid(); searchIter.Next() {
					kv, err := searchIter.KeyValue()
					requireT.NoError(err)

					if kv.Value.Equal(expectedValue) && kv.Key.Equals(key) {
						found = true
						break
					}
				}

				requireT.True(found, "should find matching value for key")

				if tc.closeImmediate {
					// Close immediately after each iteration
					requireT.NoError(searchIter.Close())
				}
			}

			// Force GC and measure memory
			runtime.GC()
			var memStatsAfter runtime.MemStats
			runtime.ReadMemStats(&memStatsAfter)

			// Report memory usage
			allocDiff := diff(memStatsAfter.Alloc, memStatsBefore.Alloc)
			sysDiff := diff(memStatsAfter.Sys, memStatsBefore.Sys)
			heapSysDiff := diff(memStatsAfter.HeapSys, memStatsBefore.HeapSys)

			t.Logf("Memory usage report for %s:", tc.name)
			t.Logf("  Alloc diff: %d bytes (%s)", allocDiff, humanize(allocDiff))
			t.Logf("  Sys diff: %d bytes (%s)", sysDiff, humanize(sysDiff))
			t.Logf("  HeapSys diff: %d bytes (%s)", heapSysDiff, humanize(heapSysDiff))
		})
	}
}
