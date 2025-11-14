package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestQueryParams(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	queryService := keeper.NewQueryService(testApp.PSEKeeper)

	// Query params
	resp, err := queryService.Params(ctx, &types.QueryParamsRequest{})
	requireT.NoError(err)
	requireT.NotNil(resp)
	requireT.NotNil(resp.Params)

	// Default params should have empty excluded addresses
	requireT.Empty(resp.Params.ExcludedAddresses)
	requireT.Empty(resp.Params.ClearingAccountMappings)
}

func TestQueryScore_NoScore(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	queryService := keeper.NewQueryService(testApp.PSEKeeper)

	// Generate a random address
	addr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())

	// Query score for address with no delegations
	resp, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: addr.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp)
	requireT.True(resp.Score.IsZero(), "score should be zero for address with no delegations")
}

func TestQueryScore_WithAccumulatedScore(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now())
	queryService := keeper.NewQueryService(testApp.PSEKeeper)

	// Generate delegator address
	delAddr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address())

	// Set an accumulated score in the snapshot
	expectedScore := sdkmath.NewInt(1000000)
	err := testApp.PSEKeeper.AccountScoreSnapshot.Set(ctx, delAddr, expectedScore)
	requireT.NoError(err)

	// Query score
	resp, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp)
	requireT.Equal(expectedScore, resp.Score, "score should equal accumulated score")
}

func TestQueryScore_WithActiveDelegation(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now())
	queryService := keeper.NewQueryService(testApp.PSEKeeper)

	// Create validator
	validatorOperator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	validator, err := createValidator(ctx, testApp, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(validator.GetOperator())

	// Create delegator
	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, delAddr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(600000))),
	))

	// Delegate tokens
	msg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 500000),
	}
	_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, msg)
	requireT.NoError(err)

	// Get initial score (should be zero immediately after delegation)
	resp1, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp1)
	requireT.True(resp1.Score.IsZero(), "score should be zero immediately after delegation")

	// Advance time by 1 hour (3600 seconds)
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(1 * time.Hour))
	requireT.NoError(err)

	// Query score again - should now have accumulated score
	resp2, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp2)

	// Expected score = delegated_tokens (500000) × time_seconds (3600)
	expectedScore := sdkmath.NewInt(500000).MulRaw(3600)
	requireT.Equal(expectedScore, resp2.Score, "score should be tokens × time")
}

func TestQueryScore_AccumulatedPlusCurrentPeriod(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now())
	queryService := keeper.NewQueryService(testApp.PSEKeeper)

	// Create validator
	validatorOperator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	validator, err := createValidator(ctx, testApp, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(validator.GetOperator())

	// Create delegator
	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, delAddr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(600000))),
	))

	// Delegate tokens
	msg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 500000),
	}
	_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, msg)
	requireT.NoError(err)

	// Set accumulated score from previous periods
	accumulatedScore := sdkmath.NewInt(10000000)
	err = testApp.PSEKeeper.AccountScoreSnapshot.Set(ctx, delAddr, accumulatedScore)
	requireT.NoError(err)

	// Advance time by 1 hour
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(1 * time.Hour))
	requireT.NoError(err)

	// Query score
	resp, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp)

	// Expected score = accumulated + (tokens × time)
	currentPeriodScore := sdkmath.NewInt(500000).MulRaw(3600)
	expectedTotalScore := accumulatedScore.Add(currentPeriodScore)
	requireT.Equal(expectedTotalScore, resp.Score, "total score should be accumulated + current period")
}

func TestQueryScore_MultipleDelegations(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now())
	queryService := keeper.NewQueryService(testApp.PSEKeeper)

	// Create two validators
	validatorOperator1, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOperator1, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	validator1, err := createValidator(ctx, testApp, validatorOperator1, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
	requireT.NoError(err)
	valAddr1 := sdk.MustValAddressFromBech32(validator1.GetOperator())

	validatorOperator2, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOperator2, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	validator2, err := createValidator(ctx, testApp, validatorOperator2, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
	requireT.NoError(err)
	valAddr2 := sdk.MustValAddressFromBech32(validator2.GetOperator())

	// Create delegator
	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, delAddr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(600000))),
	))

	// Delegate to first validator
	msg1 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr1.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 300000),
	}
	_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, msg1)
	requireT.NoError(err)

	// Delegate to second validator
	msg2 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr2.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 200000),
	}
	_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, msg2)
	requireT.NoError(err)

	// Advance time by 2 hours
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(2 * time.Hour))
	requireT.NoError(err)

	// Query score
	resp, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp)

	// Expected score = (300000 × 7200) + (200000 × 7200)
	expectedScore := sdkmath.NewInt(300000).MulRaw(7200).Add(sdkmath.NewInt(200000).MulRaw(7200))
	requireT.Equal(expectedScore, resp.Score, "score should be sum of all delegations")
}

func TestQueryScore_InvalidAddress(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	queryService := keeper.NewQueryService(testApp.PSEKeeper)

	// Query with invalid address
	_, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: "invalid_address",
	})
	requireT.Error(err, "should return error for invalid address")
}

// Helper function to create a validator - matching the pattern from hooks_test.go.
func createValidator(
	ctx sdk.Context,
	testApp *simapp.App,
	operator sdk.AccAddress,
	value sdk.Coin,
) (val stakingtypes.Validator, err error) {
	stakingKeeper := testApp.StakingKeeper
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()
	valAddr := sdk.ValAddress(operator)

	pkAny, err := codectypes.NewAnyWithValue(pubKey)
	if err != nil {
		return stakingtypes.Validator{}, err
	}
	msg := &stakingtypes.MsgCreateValidator{
		Description: stakingtypes.Description{
			Moniker: "Validator power",
		},
		Commission: stakingtypes.CommissionRates{
			Rate:          sdkmath.LegacyMustNewDecFromStr("0.1"),
			MaxRate:       sdkmath.LegacyMustNewDecFromStr("0.2"),
			MaxChangeRate: sdkmath.LegacyMustNewDecFromStr("0.01"),
		},
		MinSelfDelegation: sdkmath.OneInt(),
		DelegatorAddress:  operator.String(),
		ValidatorAddress:  valAddr.String(),
		Pubkey:            pkAny,
		Value:             value,
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).CreateValidator(ctx, msg)
	if err != nil {
		return stakingtypes.Validator{}, err
	}

	return stakingKeeper.GetValidator(ctx, valAddr)
}
