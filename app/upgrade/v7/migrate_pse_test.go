package v7

import (
	"encoding/json"
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

	// Run migration.
	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// Verify DelegationTimeEntries are re-keyed under firstMultiBlockDistributionID (2).
	distID := firstMultiBlockDistributionID
	got1, err := pseKeeper.DelegationTimeEntries.Get(ctx, collections.Join3(distID, delAddr1, valAddr1))
	requireT.NoError(err)
	requireT.True(entry1.Shares.Equal(got1.Shares))
	requireT.Equal(entry1.LastChangedUnixSec, got1.LastChangedUnixSec)

	got2, err := pseKeeper.DelegationTimeEntries.Get(ctx, collections.Join3(distID, delAddr2, valAddr2))
	requireT.NoError(err)
	requireT.True(entry2.Shares.Equal(got2.Shares))
	requireT.Equal(entry2.LastChangedUnixSec, got2.LastChangedUnixSec)

	// Old-format entries should no longer exist.
	_, err = oldDelegMap.Get(ctx, collections.Join(delAddr1, valAddr1))
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify AccountScoreSnapshot re-keyed under firstMultiBlockDistributionID (2).
	gotScore1, err := pseKeeper.AccountScoreSnapshot.Get(ctx, collections.Join(firstMultiBlockDistributionID, delAddr1))
	requireT.NoError(err)
	requireT.True(score1.Equal(gotScore1))

	gotScore2, err := pseKeeper.AccountScoreSnapshot.Get(ctx, collections.Join(firstMultiBlockDistributionID, delAddr2))
	requireT.NoError(err)
	requireT.True(score2.Equal(gotScore2))

	// Old-format score entries should no longer exist.
	_, err = oldScoreMap.Get(ctx, delAddr1)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify AllocationSchedule was replaced with full mainnet schedule from embedded JSON.
	var scheduleData scheduledDistributionsJSON
	requireT.NoError(json.Unmarshal(mainnetScheduleJSON, &scheduleData))
	totalEntries := len(scheduleData.ScheduledDistributions)
	requireT.Positive(totalEntries)

	// First entry should have ID=1 with the first mainnet timestamp.
	sched1, err := pseKeeper.AllocationSchedule.Get(ctx, 1)
	requireT.NoError(err)
	requireT.Equal(uint64(1), sched1.ID)
	requireT.Equal(uint64(1775476800), sched1.Timestamp)
	requireT.Len(sched1.Allocations, 6)

	// Second entry should have ID=2 (first multi-block distribution).
	sched2, err := pseKeeper.AllocationSchedule.Get(ctx, 2)
	requireT.NoError(err)
	requireT.Equal(uint64(2), sched2.ID)
	requireT.Equal(uint64(1778068800), sched2.Timestamp)

	// Last entry should have ID = totalEntries.
	schedLast, err := pseKeeper.AllocationSchedule.Get(ctx, uint64(totalEntries))
	requireT.NoError(err)
	requireT.Equal(uint64(totalEntries), schedLast.ID)

	// Verify LastProcessedDistributionID = 1 (first distribution already processed).
	lastID, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(uint64(1), lastID)
}

func TestMigratePSEStore_NoDelegationsOrScores(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	// Migrate with no pre-existing delegation/score data — should succeed.
	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// Verify LastProcessedDistributionID is set to 1.
	lastID, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(uint64(1), lastID)

	// Verify mainnet schedule was fully loaded from embedded JSON.
	var scheduleData scheduledDistributionsJSON
	requireT.NoError(json.Unmarshal(mainnetScheduleJSON, &scheduleData))
	expectedCount := len(scheduleData.ScheduledDistributions)

	// Count entries in store.
	var actualCount int
	iter, err := pseKeeper.AllocationSchedule.Iterate(ctx, nil)
	requireT.NoError(err)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		actualCount++
	}
	requireT.Equal(expectedCount, actualCount, "all mainnet schedule entries should be loaded")

	// Verify first and second entries have correct timestamps.
	sched1, err := pseKeeper.AllocationSchedule.Get(ctx, 1)
	requireT.NoError(err)
	requireT.Equal(uint64(1775476800), sched1.Timestamp)
	requireT.Len(sched1.Allocations, 6)

	sched2, err := pseKeeper.AllocationSchedule.Get(ctx, 2)
	requireT.NoError(err)
	requireT.Equal(uint64(1778068800), sched2.Timestamp)
}
