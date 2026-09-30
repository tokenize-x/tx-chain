package v8_test

import (
	"context"
	"testing"

	sdkmath "cosmossdk.io/math"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	"github.com/stretchr/testify/require"

	v8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// fixedStake serves a fixed bonded amount and supply to PSEPauseInflation.
type fixedStake struct {
	bonded, supply sdkmath.Int
}

func (f fixedStake) TotalBondedTokens(context.Context) (sdkmath.Int, error) { return f.bonded, nil }

func (f fixedStake) GetSupply(_ context.Context, denom string) sdk.Coin {
	return sdk.NewCoin(denom, f.supply)
}

// pauseKeepers returns the app keepers used to set the PSE pause mint params.
func pauseKeepers(testApp *simapp.App) v8.PSEPauseKeepers {
	return v8.PSEPauseKeepers{
		PSE:     testApp.PSEKeeper,
		Mint:    testApp.MintKeeper,
		Staking: testApp.StakingKeeper,
		Bank:    testApp.BankKeeper,
	}
}

// TestPSEPauseInflation checks the formula on mainnet numbers: inflation = 24% x bonded / total supply.
func TestPSEPauseInflation(t *testing.T) {
	for name, tc := range map[string]struct {
		bonded, supply int64
		expected       float64
	}{
		// Live mainnet on 2026-09-30.
		"mainnet today": {bonded: 4_170_832_004_575_492, supply: 102_087_507_882_239_934, expected: 0.009805},
		// Projected for 2026-11-06: +2 x ~440M TX auto-delegated, +~54M TX inflation.
		"mainnet after November": {bonded: 5_050_000_000_000_000, supply: 102_140_000_000_000_000, expected: 0.011866},
	} {
		t.Run(name, func(t *testing.T) {
			requireT := require.New(t)
			stake := fixedStake{bonded: sdkmath.NewInt(tc.bonded), supply: sdkmath.NewInt(tc.supply)}

			inflation, err := v8.PSEPauseInflation(context.Background(), stake, stake, "ucore")
			requireT.NoError(err)
			requireT.InDelta(tc.expected, inflation.MustFloat64(), 0.000001)

			// APY as defined in the TX inflation model: inflation / (bonded / total supply).
			bondRatio := sdkmath.LegacyNewDecFromInt(stake.bonded).QuoInt(stake.supply)
			requireT.InDelta(0.24, inflation.Quo(bondRatio).MustFloat64(), 0.000001)
		})
	}

	noSupply := fixedStake{bonded: sdkmath.OneInt(), supply: sdkmath.ZeroInt()}
	_, err := v8.PSEPauseInflation(context.Background(), noSupply, noSupply, "ucore")
	require.Error(t, err)
}

// TestSetPSEPauseMintParams checks that inflation is fixed at the computed value and the other mint params are kept.
func TestSetPSEPauseMintParams(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	before, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	expected, err := v8.PSEPauseInflation(ctx, testApp.StakingKeeper, testApp.BankKeeper, before.MintDenom)
	requireT.NoError(err)
	requireT.True(expected.IsPositive())

	requireT.NoError(v8.SetPSEPauseMintParams(ctx, pauseKeepers(testApp)))

	after, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(expected, after.InflationMin)
	requireT.Equal(expected, after.InflationMax)
	requireT.Equal(v8.PSEPauseBlocksPerYear, after.BlocksPerYear)
	requireT.Equal(before.MintDenom, after.MintDenom)
	requireT.Equal(before.InflationRateChange, after.InflationRateChange)
	requireT.Equal(before.GoalBonded, after.GoalBonded)

	minter, err := testApp.MintKeeper.Minter.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(expected, minter.Inflation)
}

// TestSetPSEPauseMintParams_Mints24PercentAPY runs the real mint code on a chain with mainnet's projected stake.
// Tokens minted per year divided by the bonded amount must be 24%, the APY of the TX inflation model.
// One simulated year is BlocksPerYear blocks, so the result does not depend on block time.
func TestSetPSEPauseMintParams_Mints24PercentAPY(t *testing.T) {
	requireT := require.New(t)

	const (
		// Mainnet projected for 2026-11-06: 5.05B of 102.14B TX bonded.
		bondedMicro = int64(5_050_000_000_000_000)
		supplyMicro = int64(102_140_000_000_000_000)
		blocks      = 1000
	)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{Height: 1})
	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Bond the stake: the validator joins the bonded set at the staking EndBlock.
	operator, _ := testApp.GenAccount(ctx)
	bonded, err := testApp.StakingKeeper.TotalBondedTokens(ctx)
	requireT.NoError(err)
	stake := sdk.NewCoin(bondDenom, sdkmath.NewInt(bondedMicro).Sub(bonded))
	requireT.NoError(testApp.FundAccount(ctx, operator, sdk.NewCoins(stake)))
	_, err = testApp.AddValidator(ctx, operator, stake, nil)
	requireT.NoError(err)
	_, err = testApp.StakingKeeper.EndBlocker(ctx)
	requireT.NoError(err)

	// Top the supply up with unbonded tokens, the way PSE clearing accounts hold most of mainnet's supply.
	holder, _ := testApp.GenAccount(ctx)
	supply := testApp.BankKeeper.GetSupply(ctx, bondDenom).Amount
	requireT.NoError(testApp.FundAccount(ctx, holder, sdk.NewCoins(
		sdk.NewCoin(bondDenom, sdkmath.NewInt(supplyMicro).Sub(supply)),
	)))

	bonded, err = testApp.StakingKeeper.TotalBondedTokens(ctx)
	requireT.NoError(err)
	requireT.Equal(bondedMicro, bonded.Int64())

	requireT.NoError(v8.SetPSEPauseMintParams(ctx, pauseKeepers(testApp)))
	params, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.InDelta(0.011866, params.InflationMax.MustFloat64(), 0.000001)

	supplyBefore := testApp.BankKeeper.GetSupply(ctx, bondDenom).Amount
	for height := int64(2); height < 2+blocks; height++ {
		requireT.NoError(mint.BeginBlocker(ctx.WithBlockHeight(height), testApp.MintKeeper))
	}
	minted := testApp.BankKeeper.GetSupply(ctx, bondDenom).Amount.Sub(supplyBefore)

	mintedPerYear := sdkmath.LegacyNewDecFromInt(minted).QuoInt64(blocks).MulInt64(int64(v8.PSEPauseBlocksPerYear))
	t.Logf("minted per year %s, inflation %s", mintedPerYear.TruncateInt(), params.InflationMax)
	requireT.InDelta(params.InflationMax.MustFloat64(), mintedPerYear.QuoInt(supplyBefore).MustFloat64(), 0.00001)
	requireT.InDelta(0.24, mintedPerYear.QuoInt(bonded).MustFloat64(), 0.0001)
}
