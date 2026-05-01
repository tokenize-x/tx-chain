package v7

import (
	"encoding/json"
	"errors"
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
	authcodec "github.com/cosmos/cosmos-sdk/x/auth/codec"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/pkg/config"
	"github.com/tokenize-x/tx-chain/v7/x/pse"
	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

const testAddrPrefix = "testcore"

func init() {
	// Match the global bech32 prefix to the test codecs so Params.ValidateBasic accepts the addresses.
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(testAddrPrefix, testAddrPrefix+"pub")
	cfg.SetBech32PrefixForValidator(testAddrPrefix+"valoper", testAddrPrefix+"valoperpub")
	cfg.SetBech32PrefixForConsensusNode(testAddrPrefix+"valcons", testAddrPrefix+"valconspub")
}

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
		authcodec.NewBech32Codec(testAddrPrefix),
		authcodec.NewBech32Codec(testAddrPrefix+"valoper"),
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

	// Verify TotalScore invariant: TotalScore[firstMultiBlockDistributionID] equals
	// the sum of migrated AccountScoreSnapshot scores.
	gotTotal, err := pseKeeper.TotalScore.Get(ctx, firstMultiBlockDistributionID)
	requireT.NoError(err)
	requireT.True(score1.Add(score2).Equal(gotTotal))
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

// TestMigratePSEStore_TotalScoreInvariant asserts that after migration
// TotalScore[firstMultiBlockDistributionID] equals the sum of migrated
// AccountScoreSnapshot entries. Without it, the first multi-block distribution
// divides by an understated denominator and overshoots.
func TestMigratePSEStore_TotalScoreInvariant(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	storeService := pseKeeper.StoreService()

	delAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	delAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	delAddr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())

	// Seed the old-format account_score store with pre-v7 accumulated scores.
	oldScoreSB := collections.NewSchemaBuilder(storeService)
	oldScoreMap := collections.NewMap(
		oldScoreSB,
		types.AccountScoreKey,
		"account_score",
		sdk.AccAddressKey,
		sdk.IntValue,
	)
	_, err := oldScoreSB.Build()
	requireT.NoError(err)

	score1 := sdkmath.NewInt(2_199_878_160_334_053)
	score2 := sdkmath.NewInt(2_003_391_705_000_000)
	score3 := sdkmath.NewInt(595_409_231_313_825)
	expectedSum := score1.Add(score2).Add(score3)

	requireT.NoError(oldScoreMap.Set(ctx, delAddr1, score1))
	requireT.NoError(oldScoreMap.Set(ctx, delAddr2, score2))
	requireT.NoError(oldScoreMap.Set(ctx, delAddr3, score3))

	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	got1, err := pseKeeper.AccountScoreSnapshot.Get(ctx, collections.Join(firstMultiBlockDistributionID, delAddr1))
	requireT.NoError(err)
	requireT.True(score1.Equal(got1))

	got2, err := pseKeeper.AccountScoreSnapshot.Get(ctx, collections.Join(firstMultiBlockDistributionID, delAddr2))
	requireT.NoError(err)
	requireT.True(score2.Equal(got2))

	got3, err := pseKeeper.AccountScoreSnapshot.Get(ctx, collections.Join(firstMultiBlockDistributionID, delAddr3))
	requireT.NoError(err)
	requireT.True(score3.Equal(got3))

	totalScore, err := pseKeeper.TotalScore.Get(ctx, firstMultiBlockDistributionID)
	requireT.NoError(err)
	requireT.True(expectedSum.Equal(totalScore),
		"TotalScore[%d]=%s but sum(AccountScoreSnapshot)=%s",
		firstMultiBlockDistributionID, totalScore, expectedSum)
}

// TestMigratePSEStore_TotalScoreFromMultipleEntries asserts migration writes
// TotalScore equal to the sum of pre-v7 entries spanning multiple magnitudes.
func TestMigratePSEStore_TotalScoreFromMultipleEntries(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	storeService := pseKeeper.StoreService()

	type entry struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}
	entries := []entry{
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(2_199_878_160_334_053)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(2_003_391_705_000_000)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(595_409_231_313_825)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(8_819_999_720)},
		{sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()), sdkmath.NewInt(1_749_486_060)},
	}

	oldScoreSB := collections.NewSchemaBuilder(storeService)
	oldScoreMap := collections.NewMap(
		oldScoreSB,
		types.AccountScoreKey,
		"account_score",
		sdk.AccAddressKey,
		sdk.IntValue,
	)
	_, err := oldScoreSB.Build()
	requireT.NoError(err)

	for _, e := range entries {
		requireT.NoError(oldScoreMap.Set(ctx, e.addr, e.score))
	}

	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// Every entry must land in AccountScoreSnapshot under the new key.
	expectedSum := sdkmath.ZeroInt()
	for _, e := range entries {
		got, err := pseKeeper.AccountScoreSnapshot.Get(
			ctx, collections.Join(firstMultiBlockDistributionID, e.addr),
		)
		requireT.NoError(err)
		requireT.True(e.score.Equal(got))
		expectedSum = expectedSum.Add(e.score)
	}

	// TotalScore must equal the sum of all migrated entries.
	totalScore, err := pseKeeper.TotalScore.Get(ctx, firstMultiBlockDistributionID)
	requireT.NoError(err)
	requireT.True(expectedSum.Equal(totalScore),
		"TotalScore[%d]=%s, expected=%s",
		firstMultiBlockDistributionID, totalScore, expectedSum)
}

// TestMigratePSEStore_RoutesExcludedAddresses verifies that pre-v7 entries
// for addresses in the current excluded-addresses list are routed to
// ExcludedAddressScore (not AccountScoreSnapshot) and excluded from TotalScore.
func TestMigratePSEStore_RoutesExcludedAddresses(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	storeService := pseKeeper.StoreService()
	addressCodec := pseKeeper.AddressCodec()

	// Two addresses in the old store: one normal, one excluded.
	normal := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	excluded := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	excludedBech32, err := addressCodec.BytesToString(excluded)
	requireT.NoError(err)

	// Seed the old-format account_score store.
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

	normalScore := sdkmath.NewInt(1_000_000)
	excludedScore := sdkmath.NewInt(500_000)
	requireT.NoError(oldScoreMap.Set(ctx, normal, normalScore))
	requireT.NoError(oldScoreMap.Set(ctx, excluded, excludedScore))

	// Configure params with the excluded address list.
	params := types.DefaultParams()
	params.ExcludedAddresses = []string{excludedBech32}
	requireT.NoError(pseKeeper.SetParams(ctx, params))

	// Run migration.
	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// Non-excluded entry lands in AccountScoreSnapshot under firstMultiBlockDistributionID.
	gotNormal, err := pseKeeper.AccountScoreSnapshot.Get(
		ctx, collections.Join(firstMultiBlockDistributionID, normal),
	)
	requireT.NoError(err)
	requireT.True(normalScore.Equal(gotNormal))

	// Excluded entry is NOT in AccountScoreSnapshot.
	_, err = pseKeeper.AccountScoreSnapshot.Get(
		ctx, collections.Join(firstMultiBlockDistributionID, excluded),
	)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Excluded entry IS in ExcludedAddressScore with the same value.
	gotExcluded, err := pseKeeper.ExcludedAddressScore.Get(ctx, excluded)
	requireT.NoError(err)
	requireT.True(excludedScore.Equal(gotExcluded))

	// TotalScore includes only the non-excluded score.
	gotTotal, err := pseKeeper.TotalScore.Get(ctx, firstMultiBlockDistributionID)
	requireT.NoError(err)
	requireT.True(normalScore.Equal(gotTotal))
}

// TestMigratePSEStore_EmptyStoreLeavesTotalScoreUnset: empty pre-v7 store
// migrates cleanly, leaves AccountScoreSnapshot/TotalScore unset, PSE enabled.
func TestMigratePSEStore_EmptyStoreLeavesTotalScoreUnset(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	// No seed data — old account_score store is empty.

	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// AccountScoreSnapshot must be empty for the first multi-block distribution.
	iter, err := pseKeeper.AccountScoreSnapshot.Iterate(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](firstMultiBlockDistributionID),
	)
	requireT.NoError(err)
	defer iter.Close()
	requireT.False(iter.Valid(), "AccountScoreSnapshot must be empty after no-data migration")

	// TotalScore must not be written when there is nothing to sum.
	_, err = pseKeeper.TotalScore.Get(ctx, firstMultiBlockDistributionID)
	requireT.ErrorIs(err, collections.ErrNotFound,
		"TotalScore must remain unset when no eligible entries are migrated")

	// PSE must remain enabled when the score snapshot is empty.
	disabled, err := pseKeeper.DistributionDisabled.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		disabled = false
	} else {
		requireT.NoError(err)
	}
	requireT.False(disabled)
}

// TestMigratePSEStore_AllExcludedLeavesTotalScoreUnset: when every pre-v7
// entry is excluded, migration routes them to ExcludedAddressScore, leaves
// AccountScoreSnapshot/TotalScore unset, and PSE remains enabled.
func TestMigratePSEStore_AllExcludedLeavesTotalScoreUnset(t *testing.T) {
	requireT := require.New(t)
	ctx, pseKeeper := setup(t)

	storeService := pseKeeper.StoreService()
	addressCodec := pseKeeper.AddressCodec()

	excluded1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	excluded2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())
	excluded1Bech32, err := addressCodec.BytesToString(excluded1)
	requireT.NoError(err)
	excluded2Bech32, err := addressCodec.BytesToString(excluded2)
	requireT.NoError(err)

	// Seed the old-format account_score store — both entries excluded.
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

	score1 := sdkmath.NewInt(1_000_000)
	score2 := sdkmath.NewInt(500_000)
	requireT.NoError(oldScoreMap.Set(ctx, excluded1, score1))
	requireT.NoError(oldScoreMap.Set(ctx, excluded2, score2))

	params := types.DefaultParams()
	params.ExcludedAddresses = []string{excluded1Bech32, excluded2Bech32}
	requireT.NoError(pseKeeper.SetParams(ctx, params))

	requireT.NoError(migratePSEStore(ctx, pseKeeper))

	// AccountScoreSnapshot must be empty — every entry was routed away.
	iter, err := pseKeeper.AccountScoreSnapshot.Iterate(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](firstMultiBlockDistributionID),
	)
	requireT.NoError(err)
	defer iter.Close()
	requireT.False(iter.Valid(),
		"AccountScoreSnapshot must be empty when all migrated entries are excluded")

	// TotalScore must not be written.
	_, err = pseKeeper.TotalScore.Get(ctx, firstMultiBlockDistributionID)
	requireT.ErrorIs(err, collections.ErrNotFound,
		"TotalScore must remain unset when no eligible entries are migrated")

	// Both scores land in ExcludedAddressScore, intact.
	got1, err := pseKeeper.ExcludedAddressScore.Get(ctx, excluded1)
	requireT.NoError(err)
	requireT.True(score1.Equal(got1))

	got2, err := pseKeeper.ExcludedAddressScore.Get(ctx, excluded2)
	requireT.NoError(err)
	requireT.True(score2.Equal(got2))

	// PSE must remain enabled when the score snapshot is empty.
	disabled, err := pseKeeper.DistributionDisabled.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		disabled = false
	} else {
		requireT.NoError(err)
	}
	requireT.False(disabled)
}
