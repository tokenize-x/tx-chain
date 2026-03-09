package v2_test

import (
	"testing"

	"cosmossdk.io/collections"
	sdkstore "cosmossdk.io/core/store"
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
	v2 "github.com/tokenize-x/tx-chain/v7/x/pse/migrations/v2"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
	"github.com/tokenize-x/tx-tools/pkg/must"
)

func setup() (sdk.Context, sdkstore.KVStoreService, codec.BinaryCodec) {
	key := storetypes.NewKVStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	cms := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	cms.MountStoreWithDB(key, storetypes.StoreTypeIAVL, db)
	must.OK(cms.LoadLatestVersion())

	ctx := sdk.NewContext(cms, tmproto.Header{}, false, log.NewNopLogger())
	encodingConfig := config.NewEncodingConfig(pse.AppModuleBasic{})
	storeService := runtime.NewKVStoreService(key)

	return ctx, storeService, encodingConfig.Codec
}

func TestMigrateStore(t *testing.T) {
	requireT := require.New(t)
	ctx, storeService, cdc := setup()

	// Generate test addresses
	delAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	delAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	valAddr1 := sdk.ValAddress(ed25519.GenPrivKey().PubKey().Address())
	valAddr2 := sdk.ValAddress(ed25519.GenPrivKey().PubKey().Address())

	// --- Seed old-format data (v1 key format) ---

	// Old DelegationTimeEntries: Pair[AccAddress, ValAddress]
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

	// Old AccountScoreSnapshot: AccAddress
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

	// Old AllocationSchedule (timestamp-based, to be cleared)
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

	requireT.NoError(oldScheduleMap.Set(ctx, 999, types.ScheduledDistribution{
		ID:        999,
		Timestamp: 12345,
	}))

	// --- Run migration ---

	requireT.NoError(v2.MigrateStore(ctx, storeService, cdc))

	// --- Verify new-format data ---

	// Verify DelegationTimeEntries re-keyed under distributionID=1
	newDelegSB := collections.NewSchemaBuilder(storeService)
	newDelegMap := collections.NewMap(
		newDelegSB,
		types.StakingTimeKey,
		"delegation_time_entries",
		collections.TripleKeyCodec(collections.Uint64Key, sdk.AccAddressKey, sdk.ValAddressKey),
		codec.CollValue[types.DelegationTimeEntry](cdc),
	)
	_, err = newDelegSB.Build()
	requireT.NoError(err)

	got1, err := newDelegMap.Get(ctx, collections.Join3(uint64(1), delAddr1, valAddr1))
	requireT.NoError(err)
	requireT.True(entry1.Shares.Equal(got1.Shares))
	requireT.Equal(entry1.LastChangedUnixSec, got1.LastChangedUnixSec)

	got2, err := newDelegMap.Get(ctx, collections.Join3(uint64(1), delAddr2, valAddr2))
	requireT.NoError(err)
	requireT.True(entry2.Shares.Equal(got2.Shares))
	requireT.Equal(entry2.LastChangedUnixSec, got2.LastChangedUnixSec)

	// Verify old-format entries no longer exist
	_, err = oldDelegMap.Get(ctx, collections.Join(delAddr1, valAddr1))
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify AccountScoreSnapshot re-keyed under distributionID=1
	newScoreSB := collections.NewSchemaBuilder(storeService)
	newScoreMap := collections.NewMap(
		newScoreSB,
		types.AccountScoreKey,
		"account_score",
		collections.PairKeyCodec(collections.Uint64Key, sdk.AccAddressKey),
		sdk.IntValue,
	)
	_, err = newScoreSB.Build()
	requireT.NoError(err)

	gotScore1, err := newScoreMap.Get(ctx, collections.Join(uint64(1), delAddr1))
	requireT.NoError(err)
	requireT.True(score1.Equal(gotScore1))

	gotScore2, err := newScoreMap.Get(ctx, collections.Join(uint64(1), delAddr2))
	requireT.NoError(err)
	requireT.True(score2.Equal(gotScore2))

	// Verify old-format score entries no longer exist
	_, err = oldScoreMap.Get(ctx, delAddr1)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify AllocationSchedule was cleared
	newScheduleSB := collections.NewSchemaBuilder(storeService)
	newScheduleMap := collections.NewMap(
		newScheduleSB,
		types.AllocationScheduleKey,
		"allocation_schedule",
		collections.Uint64Key,
		codec.CollValue[types.ScheduledDistribution](cdc),
	)
	_, err = newScheduleSB.Build()
	requireT.NoError(err)

	_, err = newScheduleMap.Get(ctx, 999)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify LastProcessedDistributionID = 0
	lastIDSB := collections.NewSchemaBuilder(storeService)
	lastIDItem := collections.NewItem(
		lastIDSB,
		types.LastProcessedDistributionIDKey,
		"last_processed_distribution_id",
		collections.Uint64Value,
	)
	_, err = lastIDSB.Build()
	requireT.NoError(err)

	lastID, err := lastIDItem.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(uint64(0), lastID)

	// Verify new schedule can be set starting from ID=1 (LastProcessedDistributionID + 1)
	// This simulates what governance would do post-migration
	alloc := func(amount int64) []types.ClearingAccountAllocation {
		return []types.ClearingAccountAllocation{
			{ClearingAccount: "pse_clearing_1", Amount: sdkmath.NewInt(amount)},
		}
	}
	newSchedule := []types.ScheduledDistribution{
		{ID: 1, Timestamp: 100000, Allocations: alloc(1000)},
		{ID: 2, Timestamp: 200000, Allocations: alloc(2000)},
	}
	for _, sd := range newSchedule {
		requireT.NoError(newScheduleMap.Set(ctx, sd.ID, sd))
	}

	// Verify both entries are retrievable
	got, err := newScheduleMap.Get(ctx, 1)
	requireT.NoError(err)
	requireT.Equal(uint64(1), got.ID)
	requireT.Equal(uint64(100000), got.Timestamp)

	got, err = newScheduleMap.Get(ctx, 2)
	requireT.NoError(err)
	requireT.Equal(uint64(2), got.ID)
	requireT.Equal(uint64(200000), got.Timestamp)
}

func TestMigrateStore_EmptyState(t *testing.T) {
	requireT := require.New(t)
	ctx, storeService, cdc := setup()

	// Migrate with no existing data — should succeed without errors
	requireT.NoError(v2.MigrateStore(ctx, storeService, cdc))

	// Verify LastProcessedDistributionID is still set to 0
	sb := collections.NewSchemaBuilder(storeService)
	lastIDItem := collections.NewItem(
		sb,
		types.LastProcessedDistributionIDKey,
		"last_processed_distribution_id",
		collections.Uint64Value,
	)
	_, err := sb.Build()
	requireT.NoError(err)

	lastID, err := lastIDItem.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(uint64(0), lastID)
}
