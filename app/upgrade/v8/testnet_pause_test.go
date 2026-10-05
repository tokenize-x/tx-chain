package v8_test

import (
	"testing"
	"time"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	v8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// TestPSEPostponeCutoffFor checks that only testnet gets the earlier cutoff.
func TestPSEPostponeCutoffFor(t *testing.T) {
	mainnet := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	testnet := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)

	require.Equal(t, mainnet, v8.PSEPostponeCutoffFor(string(constant.ChainIDMain)))
	require.Equal(t, testnet, v8.PSEPostponeCutoffFor(string(constant.ChainIDTest)))
	require.Equal(t, mainnet, v8.PSEPostponeCutoffFor(string(constant.ChainIDDev)))
	require.Equal(t, mainnet, v8.PSEPostponeCutoffFor(""))
}

// runV8Upgrade runs the real v8 upgrade handler on the given chain.
func runV8Upgrade(t *testing.T, testApp *simapp.App, ctx sdk.Context) {
	t.Helper()

	upgrade := v8.New(testApp.ModuleManager, testApp.Configurator(), testApp.BankKeeper, pauseKeepers(testApp))
	_, err := upgrade.Upgrade(ctx, upgradetypes.Plan{Name: v8.Name}, testApp.ModuleManager.GetVersionMap())
	require.NoError(t, err)
}

// pauseParamsApplied reports whether the PSE pause mint params are set.
func pauseParamsApplied(t *testing.T, testApp *simapp.App, ctx sdk.Context) bool {
	t.Helper()

	params, err := testApp.MintKeeper.Params.Get(ctx)
	require.NoError(t, err)
	return params.BlocksPerYear == v8.PSEPauseBlocksPerYear && params.InflationMin.Equal(params.InflationMax)
}

// requireDate checks the stored date of a distribution.
func requireDate(t *testing.T, testApp *simapp.App, ctx sdk.Context, id uint64, expected time.Time) {
	t.Helper()

	distribution, err := testApp.PSEKeeper.AllocationSchedule.Get(ctx, id)
	require.NoError(t, err)
	require.Equalf(t, uint64(expected.Unix()), distribution.Timestamp, "distribution %d", id)
}

// TestV8Upgrade_TestnetAfterOctober runs the upgrade on testnet after its October 2026 distribution.
// The pause starts at the upgrade: November moves to 2027 and the mint params are set by the handler itself.
func TestV8Upgrade_TestnetAfterOctober(t *testing.T) {
	testApp, ctx, _ := setupSchedule(t, testnetPSESchedule(), 10)
	ctx = ctx.WithChainID(string(constant.ChainIDTest))

	runV8Upgrade(t, testApp, ctx)

	requireDate(t, testApp, ctx, 10, time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC))
	requireDate(t, testApp, ctx, 11, time.Date(2027, time.November, 5, 12, 0, 0, 0, time.UTC))
	require.True(t, pauseParamsApplied(t, testApp, ctx), "the handler must set the pause mint params")

	params, err := testApp.MintKeeper.Params.Get(ctx)
	require.NoError(t, err)
	expected, err := v8.PSEPauseInflation(ctx, testApp.StakingKeeper, testApp.BankKeeper, params.MintDenom)
	require.NoError(t, err)
	require.Equal(t, expected, params.InflationMax)
}

// TestV8Upgrade_MainnetUnaffectedByTestnetCutoff runs the upgrade on mainnet after its September 2026 distribution.
// With the testnet cutoff this state would already count as paused; on mainnet October and November must stay.
func TestV8Upgrade_MainnetUnaffectedByTestnetCutoff(t *testing.T) {
	testApp, ctx, _ := setupSchedule(t, mainnetPSESchedule(), 6)
	ctx = ctx.WithChainID(string(constant.ChainIDMain))

	runV8Upgrade(t, testApp, ctx)

	requireDate(t, testApp, ctx, 7, time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC))
	requireDate(t, testApp, ctx, 8, time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC))
	requireDate(t, testApp, ctx, 9, time.Date(2027, time.December, 6, 12, 0, 0, 0, time.UTC))
	require.False(t, pauseParamsApplied(t, testApp, ctx), "mainnet mint params must wait for the November distribution")
}

// TestV8Upgrade_TestnetBeforeOctober runs the upgrade on testnet before its October 2026 distribution.
// The handler leaves the mint params alone; the EndBlocker sets them when the October distribution completes.
func TestV8Upgrade_TestnetBeforeOctober(t *testing.T) {
	requireT := require.New(t)

	start := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	testApp := simapp.New(simapp.WithStartTime(start))
	ctx, _, err := testApp.BeginNextBlockAtTime(start)
	requireT.NoError(err)
	ctx = ctx.WithChainID(string(constant.ChainIDTest))
	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	schedule := testnetPSESchedule()
	requireT.NoError(testApp.PSEKeeper.AllocationSchedule.Clear(ctx, nil))
	for _, distribution := range schedule {
		requireT.NoError(testApp.PSEKeeper.AllocationSchedule.Set(ctx, distribution.ID, distribution))
	}
	requireT.NoError(testApp.PSEKeeper.LastProcessedDistributionID.Set(ctx, 9))

	runV8Upgrade(t, testApp, ctx)
	requireDate(t, testApp, ctx, 10, time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC))
	requireDate(t, testApp, ctx, 11, time.Date(2027, time.November, 5, 12, 0, 0, 0, time.UTC))
	requireT.False(pauseParamsApplied(t, testApp, ctx), "the handler must wait for the October distribution")

	october := schedule[9]
	fundPSEDistribution(t, testApp, ctx, october)
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

	// Run the October distribution through the app EndBlocker, as testnet would.
	blockTime := time.Unix(int64(october.Timestamp), 0).UTC()
	completed := false
	for range 100 {
		ctx, _, err = testApp.BeginNextBlockAtTime(blockTime)
		requireT.NoError(err)
		ctx = ctx.WithChainID(string(constant.ChainIDTest))
		_, err = testApp.EndBlocker(ctx)
		requireT.NoError(err)
		lastProcessed, err := v8.LastProcessedPSEDistributionID(ctx, testApp.PSEKeeper)
		requireT.NoError(err)
		if lastProcessed == october.ID {
			completed = true
			break
		}
		requireT.False(pauseParamsApplied(t, testApp, ctx), "mint params must not change while October is paid")
		blockTime = blockTime.Add(time.Second)
	}
	requireT.True(completed, "the October distribution must complete")
	requireT.True(pauseParamsApplied(t, testApp, ctx), "the EndBlocker must set the pause mint params")

	disabled, err := testApp.PSEKeeper.DistributionDisabled.Get(ctx)
	if err == nil {
		requireT.False(disabled)
	}
	next, due, err := testApp.PSEKeeper.PeekNextAllocationSchedule(ctx)
	requireT.NoError(err)
	requireT.False(due)
	requireT.Equal(uint64(11), next.ID)
}
