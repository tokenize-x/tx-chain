package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// TestDistribution_FrozenDelegator_SkippedNotDisabled verifies the interaction
// between the emergency account freeze and PSE: a frozen delegator must be
// skipped by the distribution (no reward, no auto-delegate) rather than causing
// the auto-delegate to be rejected by the bank hook — which would abort the
// distribution and disable PSE chain-wide.
func TestDistribution_FrozenDelegator_SkippedNotDisabled(t *testing.T) {
	requireT := require.New(t)
	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	r := &runEnv{testApp: testApp, ctx: ctx, requireT: requireT, currentDistID: firstDistributionID}

	validatorOperator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOperator, sdk.NewCoins(sdk.NewInt64Coin(sdk.DefaultBondDenom, 1000))))
	validator, err := testApp.AddValidator(ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil)
	requireT.NoError(err)
	r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))

	for range 2 {
		delegator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, delegator, sdk.NewCoins(sdk.NewInt64Coin(sdk.DefaultBondDenom, 10_000_000))))
		r.delegators = append(r.delegators, delegator)
	}
	requireT.NoError(testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{Timestamp: uint64(ctx.BlockTime().Unix()), ID: firstDistributionID},
	}))

	frozen := r.delegators[0]
	clean := r.delegators[1]

	// Both delegate while the freeze is inactive, so both accrue a PSE score.
	delegateAction(r, frozen, r.validators[0], 1_100_000)
	delegateAction(r, clean, r.validators[0], 900_000)
	waitAction(r, 8*time.Second)

	// Now freeze the first delegator.
	restoreHeight := constant.FreezeActivationHeight
	constant.FreezeActivationHeight = 1
	constant.FrozenAddresses[frozen.String()] = struct{}{}
	t.Cleanup(func() {
		constant.FreezeActivationHeight = restoreHeight
		delete(constant.FrozenAddresses, frozen.String())
	})

	frozenBefore := sumDelegations(r, frozen)
	cleanBefore := sumDelegations(r, clean)

	// Run the full distribution. Without the skip-frozen guard, the frozen
	// delegator's auto-delegate is rejected by the bank hook, ProcessOngoing...
	// returns an error, and this harness call fails.
	distributeAction(r, sdkmath.NewInt(1_000_000))

	// Frozen delegator: skipped — no reward, no auto-delegate, delegation unchanged.
	requireT.Equal(frozenBefore.String(), sumDelegations(r, frozen).String(),
		"frozen delegator must be skipped (delegation unchanged)")

	// Clean delegator: unaffected — still receives and auto-delegates its reward.
	requireT.True(sumDelegations(r, clean).GT(cleanBefore),
		"clean delegator must still receive its reward")

	// PSE must remain enabled — the freeze must not trip the disable-on-error path.
	disabled, err := testApp.PSEKeeper.DistributionDisabled.Get(r.ctx)
	requireT.NoError(err)
	requireT.False(disabled, "PSE distributions must not be disabled by the freeze")
}

func sumDelegations(r *runEnv, addr sdk.AccAddress) sdkmath.Int {
	querier := stakingkeeper.NewQuerier(r.testApp.StakingKeeper)
	rsp, err := querier.DelegatorDelegations(r.ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: addr.String(),
	})
	r.requireT.NoError(err)
	total := sdkmath.NewInt(0)
	for _, d := range rsp.DelegationResponses {
		total = total.Add(d.Balance.Amount)
	}
	return total
}
