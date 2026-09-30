package v8_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	v8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// TestApplyPSEPauseMintParams checks that only the completion of the November 2026 distribution sets the params.
func TestApplyPSEPauseMintParams(t *testing.T) {
	for name, tc := range map[string]struct {
		before, after uint64
		applied       bool
	}{
		"October completes":       {before: 6, after: 7},
		"November completes":      {before: 7, after: 8, applied: true},
		"nothing completes":       {before: 8, after: 8},
		"December 2027 completes": {before: 8, after: 9},
	} {
		t.Run(name, func(t *testing.T) {
			requireT := require.New(t)

			testApp, ctx, _ := setupPSESchedule(t, tc.after)
			requireT.NoError(v8.PostponePSEDistributions(ctx, testApp.PSEKeeper, v8.PSEPostponeCutoff))
			v8.ApplyPSEPauseMintParams(ctx, pauseKeepers(testApp), tc.before)

			params, err := testApp.MintKeeper.Params.Get(ctx)
			requireT.NoError(err)
			requireT.Equal(tc.applied, params.BlocksPerYear == v8.PSEPauseBlocksPerYear)
		})
	}
}

// TestPSEPauseMintParams_AppliedWhenNovemberCompletes drives the app EndBlocker through the November 2026 distribution.
// The mint params stay unchanged while October and November are paid, change in the block November completes,
// and are not applied again afterwards, so a later governance change sticks.
func TestPSEPauseMintParams_AppliedWhenNovemberCompletes(t *testing.T) {
	requireT := require.New(t)

	// Mainnet state after the October 2026 distribution, upgraded to v8.
	start := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	const lastProcessedID = 7

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
	november := schedule[lastProcessedID]
	fundPSEDistribution(t, testApp, ctx, november)

	operator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, operator, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1000))))
	validator, err := testApp.AddValidator(ctx, operator, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	delegator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delegator, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1_000_000))))
	_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator.GetOperator(),
		Amount:           sdk.NewInt64Coin(bondDenom, 1_000_000),
	})
	requireT.NoError(err)

	original, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.NotEqual(v8.PSEPauseBlocksPerYear, original.BlocksPerYear)

	endBlock := func(blockTime time.Time) uint64 {
		ctx, _, err = testApp.BeginNextBlockAtTime(blockTime)
		requireT.NoError(err)
		_, err = testApp.EndBlocker(ctx)
		requireT.NoError(err)
		lastProcessed, err := v8.LastProcessedPSEDistributionID(ctx, pseKeeper)
		requireT.NoError(err)
		return lastProcessed
	}
	requireUnchanged := func(when string) {
		params, err := testApp.MintKeeper.Params.Get(ctx)
		requireT.NoError(err)
		requireT.Equalf(original, params, "mint params must not change %s", when)
	}

	endBlock(time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC))
	requireUnchanged("before the November distribution")
	inflationBefore, err := v8.PSEPauseInflation(ctx, testApp.StakingKeeper, testApp.BankKeeper, bondDenom)
	requireT.NoError(err)

	// November is paid over several blocks; the params change only in the block it completes.
	blockTime := time.Unix(int64(november.Timestamp), 0).UTC()
	completed := false
	for range 100 {
		if endBlock(blockTime) == november.ID {
			completed = true
			break
		}
		requireUnchanged("while the November distribution is running")
		blockTime = blockTime.Add(time.Second)
	}
	requireT.True(completed, "the November distribution must complete")

	// The inflation is computed from the stake after the November distribution, including its auto-delegation.
	expected, err := v8.PSEPauseInflation(ctx, testApp.StakingKeeper, testApp.BankKeeper, bondDenom)
	requireT.NoError(err)
	requireT.True(expected.GT(inflationBefore))

	params, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(expected, params.InflationMin)
	requireT.Equal(expected, params.InflationMax)
	requireT.Equal(v8.PSEPauseBlocksPerYear, params.BlocksPerYear)
	minter, err := testApp.MintKeeper.Minter.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(expected, minter.Inflation)

	// A later governance change must stick: the params are applied only once.
	params.InflationMax = sdkmath.LegacyMustNewDecFromStr("0.02")
	params.InflationMin = sdkmath.LegacyZeroDec()
	requireT.NoError(testApp.MintKeeper.Params.Set(ctx, params))
	endBlock(time.Date(2026, time.December, 6, 12, 0, 0, 0, time.UTC))
	after, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(params, after)
}
