package v8_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	v8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	psetypes "github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// TestPostponePSEDistributions_PauseAndResume runs PSE through the one-year pause on the postponed mainnet schedule.
// No distribution may run during the pause, and the December 2027 distribution must then run normally.
// Delegator scores keep accumulating through the pause, so its community share covers the whole 13 months.
func TestPostponePSEDistributions_PauseAndResume(t *testing.T) {
	requireT := require.New(t)

	// Mainnet state right after the November 2026 distribution.
	start := time.Date(2026, time.November, 7, 12, 0, 0, 0, time.UTC)
	resume := time.Date(2027, time.December, 6, 12, 0, 0, 0, time.UTC)
	lateJoin := time.Date(2027, time.November, 6, 12, 0, 0, 0, time.UTC)
	const lastProcessedID = 8

	testApp := simapp.New(simapp.WithStartTime(start))
	ctx, _, err := testApp.BeginNextBlockAtTime(start)
	requireT.NoError(err)
	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)
	pseKeeper := testApp.PSEKeeper

	schedule := mainnetPSESchedule()
	requireT.NoError(pseKeeper.AllocationSchedule.Clear(ctx, nil))
	for _, distribution := range schedule {
		requireT.NoError(pseKeeper.AllocationSchedule.Set(ctx, distribution.ID, distribution))
	}
	requireT.NoError(pseKeeper.LastProcessedDistributionID.Set(ctx, lastProcessedID))
	requireT.NoError(v8.PostponePSEDistributions(ctx, pseKeeper, v8.PSEPostponeCutoff))

	resumed := schedule[lastProcessedID]
	recipients := fundPSEDistribution(t, testApp, ctx, resumed)

	operator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, operator, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1000))))
	validator, err := testApp.AddValidator(ctx, operator, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(validator.GetOperator())

	const stake = 1_000_000
	delegate := func(ctx sdk.Context, delegator sdk.AccAddress) {
		requireT.NoError(testApp.FundAccount(ctx, delegator, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, stake))))
		_, err := stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
			DelegatorAddress: delegator.String(),
			ValidatorAddress: valAddr.String(),
			Amount:           sdk.NewInt64Coin(bondDenom, stake),
		})
		requireT.NoError(err)
	}
	delegated := func(ctx sdk.Context, delegator sdk.AccAddress) sdkmath.Int {
		delegation, err := testApp.StakingKeeper.GetDelegation(ctx, delegator, valAddr)
		requireT.NoError(err)
		return validator.TokensFromShares(delegation.Shares).TruncateInt()
	}

	// The early delegator stakes for the whole pause, the late one only for its last month.
	early, _ := testApp.GenAccount(ctx)
	late, _ := testApp.GenAccount(ctx)
	delegate(ctx, early)

	// Nothing may be distributed during the pause, up to one second before the resumed date.
	communityBalance := testApp.BankKeeper.GetBalance(
		ctx, testApp.AccountKeeper.GetModuleAddress(psetypes.ClearingAccountCommunity), bondDenom,
	)
	for _, blockTime := range []time.Time{
		time.Date(2026, time.December, 6, 12, 0, 0, 0, time.UTC),
		time.Date(2027, time.June, 6, 12, 0, 0, 0, time.UTC),
		lateJoin,
		resume.Add(-time.Second),
	} {
		ctx, _, err = testApp.BeginNextBlockAtTime(blockTime)
		requireT.NoError(err)
		if blockTime.Equal(lateJoin) {
			delegate(ctx, late)
		}

		requireT.NoError(pseKeeper.ProcessNextDistribution(ctx))
		lastProcessed, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
		requireT.NoError(err)
		requireT.EqualValuesf(lastProcessedID, lastProcessed, "no distribution may run at %s", blockTime)
		_, err = pseKeeper.OngoingDistribution.Get(ctx)
		requireT.Errorf(err, "no distribution may start at %s", blockTime)
		requireT.Equal(communityBalance, testApp.BankKeeper.GetBalance(
			ctx, testApp.AccountKeeper.GetModuleAddress(psetypes.ClearingAccountCommunity), bondDenom,
		))
	}

	// On the resumed date the distribution runs to completion over several blocks.
	blockTime := resume
	for range 100 {
		ctx, _, err = testApp.BeginNextBlockAtTime(blockTime)
		requireT.NoError(err)
		requireT.NoError(pseKeeper.ProcessNextDistribution(ctx))
		lastProcessed, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
		requireT.NoError(err)
		if lastProcessed == resumed.ID {
			break
		}
		blockTime = blockTime.Add(time.Second)
	}
	lastProcessed, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(resumed.ID, lastProcessed, "the December 2027 distribution must complete")

	for _, allocation := range resumed.Allocations {
		if allocation.ClearingAccount == psetypes.ClearingAccountCommunity {
			continue
		}
		requireT.Equal(allocation.Amount.String(),
			testApp.BankKeeper.GetBalance(ctx, recipients[allocation.ClearingAccount], bondDenom).Amount.String())
	}

	// Rewards follow stake x time and both staked the same amount, so the ratio is the ratio of staking durations.
	earlyReward := delegated(ctx, early).SubRaw(stake)
	lateReward := delegated(ctx, late).SubRaw(stake)
	requireT.True(lateReward.IsPositive(), "late delegator must be rewarded")
	expectedRatio := resume.Sub(start).Seconds() / resume.Sub(lateJoin).Seconds()
	actualRatio, err := earlyReward.ToLegacyDec().Quo(lateReward.ToLegacyDec()).Float64()
	requireT.NoError(err)
	requireT.InEpsilon(expectedRatio, actualRatio, 0.01, "early/late reward ratio")

	disabled, err := pseKeeper.DistributionDisabled.Get(ctx)
	if err == nil {
		requireT.False(disabled)
	}

	// The next distribution is the postponed January one, and it is not due yet.
	next, due, err := pseKeeper.PeekNextAllocationSchedule(ctx)
	requireT.NoError(err)
	requireT.False(due)
	requireT.Equal(resumed.ID+1, next.ID)
	requireT.Equal(uint64(time.Date(2028, time.January, 6, 12, 0, 0, 0, time.UTC).Unix()), next.Timestamp)
}

// fundPSEDistribution funds the clearing accounts for one distribution.
// It maps each non-community account to a new recipient.
// It returns the recipient of each non-community clearing account.
func fundPSEDistribution(
	t *testing.T,
	testApp *simapp.App,
	ctx sdk.Context,
	distribution psetypes.ScheduledDistribution,
) map[string]sdk.AccAddress {
	t.Helper()
	requireT := require.New(t)

	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	recipients := map[string]sdk.AccAddress{}
	mappings := make([]psetypes.ClearingAccountMapping, 0)
	for _, allocation := range distribution.Allocations {
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, allocation.Amount))
		requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, coins))
		requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
			ctx, minttypes.ModuleName, allocation.ClearingAccount, coins,
		))
		if allocation.ClearingAccount == psetypes.ClearingAccountCommunity {
			continue
		}
		recipient, _ := testApp.GenAccount(ctx)
		recipients[allocation.ClearingAccount] = recipient
		mappings = append(mappings, psetypes.ClearingAccountMapping{
			ClearingAccount:    allocation.ClearingAccount,
			RecipientAddresses: []string{recipient.String()},
		})
	}
	requireT.NoError(testApp.PSEKeeper.UpdateClearingAccountMappings(
		ctx, authtypes.NewModuleAddress(govtypes.ModuleName).String(), mappings,
	))

	return recipients
}
