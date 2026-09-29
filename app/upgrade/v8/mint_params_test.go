package v8_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	v8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// TestSetPSEPauseMintParams checks that inflation is pinned and the other mint params are kept.
func TestSetPSEPauseMintParams(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	before, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)

	requireT.NoError(v8.SetPSEPauseMintParams(ctx, testApp.MintKeeper))

	after, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(v8.PSEPauseInflation, after.InflationMin)
	requireT.Equal(v8.PSEPauseInflation, after.InflationMax)
	requireT.Equal(v8.PSEPauseBlocksPerYear, after.BlocksPerYear)
	requireT.Equal(before.MintDenom, after.MintDenom)
	requireT.Equal(before.InflationRateChange, after.InflationRateChange)
	requireT.Equal(before.GoalBonded, after.GoalBonded)

	minter, err := testApp.MintKeeper.Minter.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(v8.PSEPauseInflation, minter.Inflation)
}

// TestPSEPauseInflation_Targets24PercentAPR runs the real mint and distribution code on a mainnet-shaped chain.
// It checks that the pinned inflation pays a 24% APR to a delegator of an average-commission validator.
// One simulated year is BlocksPerYear blocks, so the result does not depend on block time.
func TestPSEPauseInflation_Targets24PercentAPR(t *testing.T) {
	requireT := require.New(t)

	const (
		// Mainnet after the November 2026 distribution: 5.12B of 102.09B TX staked.
		stakedMicro = int64(5_122_525_923_000_000)
		supplyMicro = int64(102_085_966_518_987_131)
		blocks      = 1000
	)
	// Stake-weighted average commission of mainnet validators.
	commission := sdkmath.LegacyMustNewDecFromStr("0.0577")

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{Height: 1})
	requireT.NoError(v8.SetPSEPauseMintParams(ctx, testApp.MintKeeper))

	// Mainnet's community tax; the simapp default is lower and would inflate the result.
	distrParams, err := testApp.DistrKeeper.Params.Get(ctx)
	requireT.NoError(err)
	distrParams.CommunityTax = sdkmath.LegacyMustNewDecFromStr("0.05")
	requireT.NoError(testApp.DistrKeeper.Params.Set(ctx, distrParams))

	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	operator, _ := testApp.GenAccount(ctx)
	stake := sdk.NewInt64Coin(bondDenom, stakedMicro)
	requireT.NoError(testApp.FundAccount(ctx, operator, sdk.NewCoins(stake)))
	validator, err := testApp.AddValidator(ctx, operator, stake, &stakingtypes.CommissionRates{Rate: commission})
	requireT.NoError(err)

	// Top the supply up with unstaked tokens, the way PSE clearing accounts hold most of mainnet's supply.
	holder, _ := testApp.GenAccount(ctx)
	supply := testApp.BankKeeper.GetSupply(ctx, bondDenom).Amount
	requireT.NoError(testApp.FundAccount(ctx, holder, sdk.NewCoins(
		sdk.NewCoin(bondDenom, sdkmath.NewInt(supplyMicro).Sub(supply)),
	)))

	consAddr, err := validator.GetConsAddr()
	requireT.NoError(err)
	// The validator is not bonded until a staking EndBlock, so its power is derived from its tokens.
	power := sdk.TokensToConsensusPower(validator.Tokens, testApp.StakingKeeper.PowerReduction(ctx))
	votes := []abci.VoteInfo{{
		Validator:   abci.Validator{Address: consAddr, Power: power},
		BlockIdFlag: tmproto.BlockIDFlagCommit,
	}}

	for height := int64(2); height < 2+blocks; height++ {
		ctx = ctx.WithBlockHeight(height)
		requireT.NoError(mint.BeginBlocker(ctx, testApp.MintKeeper))
		requireT.NoError(testApp.DistrKeeper.AllocateTokens(ctx, power, votes))
	}

	valAddr := sdk.ValAddress(operator)
	rewards, err := distrkeeper.NewQuerier(testApp.DistrKeeper).DelegationRewards(ctx,
		&distrtypes.QueryDelegationRewardsRequest{
			DelegatorAddress: operator.String(),
			ValidatorAddress: valAddr.String(),
		})
	requireT.NoError(err)

	perBlock := rewards.Rewards.AmountOf(bondDenom).QuoInt64(blocks)
	apr := perBlock.MulInt64(int64(v8.PSEPauseBlocksPerYear)).QuoInt64(stakedMicro)
	t.Logf("delegator APR: %s", apr)
	requireT.InDelta(0.24, apr.MustFloat64(), 0.001)
}
