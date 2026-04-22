package v7patch1_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/app/upgrade/v7patch1"
	"github.com/tokenize-x/tx-chain/v7/testutil/simapp"
	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// TestV7Patch1Replay_FullLifecycle drives the testnet-v7 incident + recovery:
//
//	inject migration residue -> delegate -> start distribution ->
//	run Consume+Phase2 in cacheCtx (fails, rolled back) ->
//	flip circuit breaker -> run v7patch1 recovery ->
//	loop ProcessNextDistribution until finalization -> assert clean end state.
func TestV7Patch1Replay_FullLifecycle(t *testing.T) {
	requireT := require.New(t)

	// Phase 0: simapp setup.
	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx := testApp.NewContext(false).WithBlockTime(startTime)

	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper
	bankKeeper := testApp.BankKeeper
	accountKeeper := testApp.AccountKeeper

	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	validators := make([]sdk.ValAddress, 0, 3)
	for range 3 {
		valOp, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, valOp, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000_000))),
		))
		val, err := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(bondDenom, 1_000_000), nil)
		requireT.NoError(err)
		validators = append(validators, sdk.MustValAddressFromBech32(val.GetOperator()))
	}

	// Pre-generate delegator addresses so we can inject stranded entries for
	// them BEFORE they delegate — matching the real testnet timeline.
	const distID = uint64(1)
	delegators := make([]sdk.AccAddress, 0, 3)
	for range 3 {
		delAddr, _ := testApp.GenAccount(ctx)
		delegators = append(delegators, delAddr)
	}

	// Phase 1: inject v7 migration residue — AccountScoreSnapshot written
	// directly (bypass addToMainScore) so TotalScore is NOT updated.
	// Sum ≈ 10% of the consumed score accumulated below — forces Phase 2 overshoot.
	strandedScores := []sdkmath.Int{
		sdkmath.NewInt(300_000),
		sdkmath.NewInt(300_000),
		sdkmath.NewInt(300_000),
	}
	strandedSum := sdkmath.ZeroInt()
	for i, score := range strandedScores {
		requireT.NoError(pseKeeper.AccountScoreSnapshot.Set(
			ctx, collections.Join(distID, delegators[i]), score,
		))
		strandedSum = strandedSum.Add(score)
	}

	// Phase 2: delegations — staking hooks populate DelegationTimeEntries[distID].
	stakingMsgServer := stakingkeeper.NewMsgServerImpl(stakingKeeper)
	const delegationAmount = int64(100_000)
	for i, delAddr := range delegators {
		requireT.NoError(testApp.FundAccount(
			ctx, delAddr, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(5_000_000))),
		))
		_, err := stakingMsgServer.Delegate(ctx, &stakingtypes.MsgDelegate{
			DelegatorAddress: delAddr.String(),
			ValidatorAddress: validators[i].String(),
			Amount:           sdk.NewInt64Coin(bondDenom, delegationAmount),
		})
		requireT.NoError(err)
	}

	// Advance block time so score accumulates: tokens × duration = 100_000 × 30s
	// = 3_000_000 per delegator, ~9_000_000 total.
	const accumulationSeconds = 30
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(accumulationSeconds * time.Second))

	// Phase 3: schedule + trigger BeginCommunityDistribution.
	communityAmount := sdkmath.NewInt(1_000_000_000)
	schedule := []types.ScheduledDistribution{{
		ID:        distID,
		Timestamp: uint64(ctx.BlockTime().Unix()),
		Allocations: []types.ClearingAccountAllocation{
			{ClearingAccount: types.ClearingAccountCommunity, Amount: communityAmount},
		},
	}}
	requireT.NoError(pseKeeper.SaveDistributionSchedule(ctx, schedule))

	communityCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, communityAmount))
	requireT.NoError(bankKeeper.MintCoins(ctx, minttypes.ModuleName, communityCoins))
	requireT.NoError(bankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, types.ClearingAccountCommunity, communityCoins,
	))

	requireT.NoError(pseKeeper.ProcessNextDistribution(ctx))

	intermediaryAddr := accountKeeper.GetModuleAddress(types.ClearingAccountCommunityIntermediary)

	// Phase 4: pre-failure preconditions — must match testnet pre-85896300 state.
	ongoing, err := pseKeeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(distID, ongoing.ID)

	disabled, err := pseKeeper.DistributionDisabled.Get(ctx)
	if err != nil {
		requireT.ErrorIs(err, collections.ErrNotFound)
	} else {
		requireT.False(disabled)
	}

	intermediaryBalance := bankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount
	requireT.True(communityAmount.Equal(intermediaryBalance))

	snapshotSum := sumAccountScoreSnapshot(t, pseKeeper, ctx, distID)
	requireT.True(strandedSum.Equal(snapshotSum))

	// TotalScore absent = the invariant violation.
	_, err = pseKeeper.TotalScore.Get(ctx, distID)
	requireT.ErrorIs(err, collections.ErrNotFound)

	requireT.True(hasDelegationTimeEntries(t, pseKeeper, ctx, distID))

	// Phase 5: reproduce the failure in cacheCtx. Discarding it matches the
	// EndBlocker's rollback on error.
	cacheCtx, _ := ctx.CacheContext()
	failErr := pseKeeper.ProcessNextDistribution(cacheCtx)
	requireT.Error(failErr, "Phase 2 should overshoot and fail")
	requireT.True(
		strings.Contains(failErr.Error(), "insufficient") ||
			strings.Contains(failErr.Error(), "not available"),
		"failure should be insufficient-funds shaped, got %q", failErr.Error(),
	)

	// Phase 6: simulate the circuit breaker committing on the outer ctx.
	requireT.NoError(pseKeeper.DistributionDisabled.Set(ctx, true))

	// Phase 7: post-failure preconditions — matches testnet state right now.
	ongoing, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(distID, ongoing.ID)

	disabled, err = pseKeeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.True(disabled)

	intermediaryBalance = bankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount
	requireT.True(communityAmount.Equal(intermediaryBalance))

	snapshotSum = sumAccountScoreSnapshot(t, pseKeeper, ctx, distID)
	requireT.True(strandedSum.Equal(snapshotSum))

	_, err = pseKeeper.TotalScore.Get(ctx, distID)
	requireT.ErrorIs(err, collections.ErrNotFound)

	requireT.True(hasDelegationTimeEntries(t, pseKeeper, ctx, distID))

	// Phase 8: run the v7patch1 recovery handler.
	addressCodec := accountKeeper.AddressCodec()
	requireT.NoError(v7patch1.RecoverOngoingDistribution(ctx, pseKeeper, addressCodec))

	// Phase 9: recovered state.
	gotTotal, err := pseKeeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.True(strandedSum.Equal(gotTotal))

	disabled, err = pseKeeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.False(disabled)

	_, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)

	intermediaryBalance = bankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount
	requireT.True(communityAmount.Equal(intermediaryBalance))

	// Phase 10: resume via ProcessNextDistribution until OngoingDistribution clears.
	const maxIterations = 50
	completed := false
	for range maxIterations {
		requireT.NoError(pseKeeper.ProcessNextDistribution(ctx))
		if _, err := pseKeeper.OngoingDistribution.Get(ctx); errors.Is(err, collections.ErrNotFound) {
			completed = true
			break
		}
	}
	requireT.True(completed)

	// Phase 11: end state — distribution complete, dist-scoped state cleaned up.
	lastProcessed, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(distID, lastProcessed)

	_, err = pseKeeper.TotalScore.Get(ctx, distID)
	requireT.ErrorIs(err, collections.ErrNotFound)

	snapshotSum = sumAccountScoreSnapshot(t, pseKeeper, ctx, distID)
	requireT.True(snapshotSum.IsZero())

	finalIntermediary := bankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount
	requireT.True(finalIntermediary.IsZero(),
		"intermediary should be fully drained, got %s", finalIntermediary)

	disabled, err = pseKeeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.False(disabled)

	t.Logf("replay complete: distID=%d processed, intermediary drained", lastProcessed)
}

// sumAccountScoreSnapshot returns the sum of AccountScoreSnapshot entries under distID.
func sumAccountScoreSnapshot(
	t *testing.T, pseKeeper pskeeper.Keeper, ctx sdk.Context, distID uint64,
) sdkmath.Int {
	t.Helper()
	total := sdkmath.ZeroInt()
	iter, err := pseKeeper.AccountScoreSnapshot.Iterate(
		ctx, collections.NewPrefixedPairRange[uint64, sdk.AccAddress](distID),
	)
	require.NoError(t, err)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		require.NoError(t, err)
		total = total.Add(kv.Value)
	}
	return total
}

// hasDelegationTimeEntries reports whether any DelegationTimeEntries exist under distID.
func hasDelegationTimeEntries(
	t *testing.T, pseKeeper pskeeper.Keeper, ctx sdk.Context, distID uint64,
) bool {
	t.Helper()
	iter, err := pseKeeper.DelegationTimeEntries.Iterate(
		ctx, collections.NewPrefixedTripleRange[uint64, sdk.AccAddress, sdk.ValAddress](distID),
	)
	require.NoError(t, err)
	defer iter.Close()
	return iter.Valid()
}
