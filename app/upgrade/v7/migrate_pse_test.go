package v7

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/pkg/config"
	"github.com/tokenize-x/tx-chain/v7/x/pse"
	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

func setup(t *testing.T) (sdk.Context, pskeeper.Keeper) {
	t.Helper()

	key := storetypes.NewKVStoreKey(types.StoreKey)
	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	cms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, db)
	require.NoError(t, cms.LoadLatestVersion())

	ctx := sdk.NewContext(cms, tmproto.Header{}, false, log.NewNopLogger())
	encodingConfig := config.NewEncodingConfig(pse.AppModuleBasic{})
	storeService := runtime.NewKVStoreService(key)

	keeper := pskeeper.NewKeeper(
		storeService,
		encodingConfig.Codec,
		"",                 // authority
		nil, nil, nil, nil, // account, bank, distribution, staking keepers — not needed
		nil, nil, // address codecs — not needed
	)

	return ctx, keeper
}

func TestMigratePSEStore(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	storeService := pseKeeper.StoreService()
	cdc := pseKeeper.Codec()

	// Generate test addresses.
	delAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	delAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	valAddr1 := sdk.ValAddress(ed25519.GenPrivKey().PubKey().Address())
	valAddr2 := sdk.ValAddress(ed25519.GenPrivKey().PubKey().Address())

	// Old DelegationTimeEntries: Pair[AccAddress, ValAddress] (timestamp keyed).
	oldDelegSB := collections.NewSchemaBuilder(storeService)
	oldDelegMap := collections.NewMap(
		oldDelegSB,
		types.StakingTimeKey,
		"delegation_time_entries",
		collections.PairKeyCodec(sdk.AccAddressKey, sdk.ValAddressKey),
		codec.CollValue[types.DelegationTimeEntry](cdc),
	)
	_, err := oldDelegSB.Build()
	requireT.NoError(err)

	entry1 := types.DelegationTimeEntry{
		Shares:             sdkmath.LegacyNewDec(100),
		LastChangedUnixSec: 1000,
	}
	entry2 := types.DelegationTimeEntry{
		Shares:             sdkmath.LegacyNewDec(200),
		LastChangedUnixSec: 2000,
	}
	requireT.NoError(oldDelegMap.Set(ctx, collections.Join(delAddr1, valAddr1), entry1))
	requireT.NoError(oldDelegMap.Set(ctx, collections.Join(delAddr2, valAddr2), entry2))

	// Old AccountScoreSnapshot store.
	oldScoreSB := collections.NewSchemaBuilder(storeService)
	oldScoreMap := collections.NewMap(
		oldScoreSB,
		types.AccountScoreKey,
		"account_score",
		sdk.AccAddressKey,
		sdk.IntValue,
	)
	_, err = oldScoreSB.Build()
	requireT.NoError(err)

	score1 := sdkmath.NewInt(500)
	score2 := sdkmath.NewInt(1000)
	requireT.NoError(oldScoreMap.Set(ctx, delAddr1, score1))
	requireT.NoError(oldScoreMap.Set(ctx, delAddr2, score2))

	// Old AllocationSchedule: keyed by timestamp, no ID field in v6.
	// Migration must clear all entries so schedule can be re-submitted via governance.
	oldScheduleSB := collections.NewSchemaBuilder(storeService)
	oldScheduleMap := collections.NewMap(
		oldScheduleSB,
		types.AllocationScheduleKey,
		"allocation_schedule",
		collections.Uint64Key,
		codec.CollValue[types.ScheduledDistribution](cdc),
	)
	_, err = oldScheduleSB.Build()
	requireT.NoError(err)

	oldTimestamp := uint64(1700000000)
	requireT.NoError(oldScheduleMap.Set(ctx, oldTimestamp, types.ScheduledDistribution{
		Timestamp: oldTimestamp,
		Allocations: []types.ClearingAccountAllocation{
			{ClearingAccount: "pse_clearing_1", Amount: sdkmath.NewInt(500)},
		},
	}))

	// Run migration.
	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// Verify new format DelegationTimeEntries (re-keyed under distributionID=1).
	got1, err := pseKeeper.DelegationTimeEntries.Get(ctx, collections.Join3(uint64(1), delAddr1, valAddr1))
	requireT.NoError(err)
	requireT.True(entry1.Shares.Equal(got1.Shares))
	requireT.Equal(entry1.LastChangedUnixSec, got1.LastChangedUnixSec)

	got2, err := pseKeeper.DelegationTimeEntries.Get(ctx, collections.Join3(uint64(1), delAddr2, valAddr2))
	requireT.NoError(err)
	requireT.True(entry2.Shares.Equal(got2.Shares))
	requireT.Equal(entry2.LastChangedUnixSec, got2.LastChangedUnixSec)

	// Old-format entries should no longer exist.
	_, err = oldDelegMap.Get(ctx, collections.Join(delAddr1, valAddr1))
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify new-format AccountScoreSnapshot (re-keyed under distributionID=1).
	gotScore1, err := pseKeeper.AccountScoreSnapshot.Get(ctx, collections.Join(uint64(1), delAddr1))
	requireT.NoError(err)
	requireT.True(score1.Equal(gotScore1))

	gotScore2, err := pseKeeper.AccountScoreSnapshot.Get(ctx, collections.Join(uint64(1), delAddr2))
	requireT.NoError(err)
	requireT.True(score2.Equal(gotScore2))

	// Old-format score entries should no longer exist.
	_, err = oldScoreMap.Get(ctx, delAddr1)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify AllocationSchedule was cleared.
	_, err = pseKeeper.AllocationSchedule.Get(ctx, oldTimestamp)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify LastProcessedDistributionID = 0.
	lastID, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(uint64(0), lastID)

	// Verify new schedule can be set starting from ID=1 (simulates post-upgrade governance).
	newSchedule := []types.ScheduledDistribution{
		{ID: 1, Timestamp: 100000, Allocations: []types.ClearingAccountAllocation{{ClearingAccount: "pse_clearing_1", Amount: sdkmath.NewInt(1000)}}},
		{ID: 2, Timestamp: 200000, Allocations: []types.ClearingAccountAllocation{{ClearingAccount: "pse_clearing_1", Amount: sdkmath.NewInt(2000)}}},
	}
	for _, sd := range newSchedule {
		requireT.NoError(pseKeeper.AllocationSchedule.Set(ctx, sd.ID, sd))
	}

	got, err := pseKeeper.AllocationSchedule.Get(ctx, 1)
	requireT.NoError(err)
	requireT.Equal(uint64(1), got.ID)
	requireT.Equal(uint64(100000), got.Timestamp)

	got, err = pseKeeper.AllocationSchedule.Get(ctx, 2)
	requireT.NoError(err)
	requireT.Equal(uint64(2), got.ID)
	requireT.Equal(uint64(200000), got.Timestamp)
}

func TestMigratePSEStore_EmptyState(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	// Migrate with no existing data — should succeed without errors.
	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// Verify LastProcessedDistributionID is set to 0.
	lastID, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(uint64(0), lastID)
}
