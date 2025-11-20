package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
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
	validator, err := testApp.AddValidator(ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
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
	validator, err := testApp.AddValidator(ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
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
	validator1, err := testApp.AddValidator(ctx, validatorOperator1, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
	requireT.NoError(err)
	valAddr1 := sdk.MustValAddressFromBech32(validator1.GetOperator())

	validatorOperator2, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOperator2, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	validator2, err := testApp.AddValidator(ctx, validatorOperator2, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
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

func TestQueryAllocationSchedule(t *testing.T) {
	t.Run("empty schedule", func(t *testing.T) {
		requireT := require.New(t)
		testApp := simapp.New()
		ctx := testApp.NewContext(false).WithBlockTime(time.Now())
		queryService := keeper.NewQueryService(testApp.PSEKeeper)

		resp, err := queryService.ScheduledDistributions(ctx, &types.QueryScheduledDistributionsRequest{})
		requireT.NoError(err)
		requireT.NotNil(resp)
		requireT.Empty(resp.ScheduledDistributions)
	})

	t.Run("multiple schedule allocation with single allocation", func(t *testing.T) {
		requireT := require.New(t)
		testApp := simapp.New()
		currentTime := time.Now()
		ctx := testApp.NewContext(false).WithBlockTime(currentTime)
		queryService := keeper.NewQueryService(testApp.PSEKeeper)

		// Create schedule allocations at different future times
		schedule1 := types.ScheduledDistribution{
			Timestamp: uint64(currentTime.Add(1 * time.Hour).Unix()),
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(2000)},
			},
		}
		schedule2 := types.ScheduledDistribution{
			Timestamp: uint64(currentTime.Add(2 * time.Hour).Unix()),
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(3000)},
			},
		}

		err := testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{schedule1, schedule2})
		requireT.NoError(err)

		resp, err := queryService.ScheduledDistributions(ctx, &types.QueryScheduledDistributionsRequest{})
		requireT.NoError(err)
		requireT.Len(resp.ScheduledDistributions, 2)
		requireT.Equal(schedule1.Timestamp, resp.ScheduledDistributions[0].Timestamp)
		requireT.Equal(schedule2.Timestamp, resp.ScheduledDistributions[1].Timestamp)
	})

	t.Run("schedule with multiple allocations", func(t *testing.T) {
		requireT := require.New(t)
		testApp := simapp.New()
		currentTime := time.Now()
		ctx := testApp.NewContext(false).WithBlockTime(currentTime)
		queryService := keeper.NewQueryService(testApp.PSEKeeper)

		schedule := types.ScheduledDistribution{
			Timestamp: uint64(currentTime.Add(1 * time.Hour).Unix()),
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountCommunity, Amount: sdkmath.NewInt(1000)},
				{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(2000)},
				{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(3000)},
			},
		}

		err := testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{schedule})
		requireT.NoError(err)

		resp, err := queryService.ScheduledDistributions(ctx, &types.QueryScheduledDistributionsRequest{})
		requireT.NoError(err)
		requireT.Len(resp.ScheduledDistributions, 1)
		requireT.Len(resp.ScheduledDistributions[0].Allocations, 3)
	})

	t.Run("schedule sorted by timestamp", func(t *testing.T) {
		requireT := require.New(t)
		testApp := simapp.New()
		currentTime := time.Now()
		ctx := testApp.NewContext(false).WithBlockTime(currentTime)
		queryService := keeper.NewQueryService(testApp.PSEKeeper)

		// Save schedule item in non-chronological order
		schedule3 := types.ScheduledDistribution{
			Timestamp: uint64(currentTime.Add(3 * time.Hour).Unix()),
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(3000)},
			},
		}
		schedule1 := types.ScheduledDistribution{
			Timestamp: uint64(currentTime.Add(1 * time.Hour).Unix()),
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
			},
		}
		schedule2 := types.ScheduledDistribution{
			Timestamp: uint64(currentTime.Add(2 * time.Hour).Unix()),
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(2000)},
			},
		}

		err := testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{schedule3, schedule1, schedule2})
		requireT.NoError(err)

		resp, err := queryService.ScheduledDistributions(ctx, &types.QueryScheduledDistributionsRequest{})
		requireT.NoError(err)
		requireT.Len(resp.ScheduledDistributions, 3)
		// Verify schedule are sorted by timestamp in ascending order
		requireT.Equal(schedule1.Timestamp, resp.ScheduledDistributions[0].Timestamp)
		requireT.Equal(schedule2.Timestamp, resp.ScheduledDistributions[1].Timestamp)
		requireT.Equal(schedule3.Timestamp, resp.ScheduledDistributions[2].Timestamp)
	})
}

func TestQueryClearingAccountBalances(t *testing.T) {
	t.Run("all accounts with zero balance", func(t *testing.T) {
		requireT := require.New(t)
		testApp := simapp.New()
		ctx := testApp.NewContext(false)
		queryService := keeper.NewQueryService(testApp.PSEKeeper)

		resp, err := queryService.ClearingAccountBalances(ctx, &types.QueryClearingAccountBalancesRequest{})
		requireT.NoError(err)
		requireT.NotNil(resp)

		// Should return all 6 clearing accounts
		requireT.Len(resp.Balances, 6)

		// Verify all accounts are present and have zero balance
		accounts := types.GetAllClearingAccounts()
		for i, balance := range resp.Balances {
			requireT.Equal(accounts[i], balance.ClearingAccount)
			requireT.True(balance.Balance.IsZero())
		}
	})

	t.Run("accounts with non-zero balances", func(t *testing.T) {
		requireT := require.New(t)
		testApp := simapp.New()
		ctx := testApp.NewContext(false)
		queryService := keeper.NewQueryService(testApp.PSEKeeper)

		// Fund some clearing accounts by sending from PSE module
		foundationAmount := sdk.NewInt64Coin(sdk.DefaultBondDenom, 1000000)
		teamAmount := sdk.NewInt64Coin(sdk.DefaultBondDenom, 2000000)
		communityAmount := sdk.NewInt64Coin(sdk.DefaultBondDenom, 3000000)

		// Calculate total to mint
		totalAmountInt := foundationAmount.Amount.Add(teamAmount.Amount).Add(communityAmount.Amount)
		totalAmount := sdk.NewCoin(sdk.DefaultBondDenom, totalAmountInt)

		// Mint total coins to PSE module
		requireT.NoError(testApp.BankKeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(totalAmount)))

		// Send to clearing accounts
		requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
			ctx, types.ModuleName, types.ClearingAccountFoundation, sdk.NewCoins(foundationAmount)))
		requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
			ctx, types.ModuleName, types.ClearingAccountTeam, sdk.NewCoins(teamAmount)))
		requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
			ctx, types.ModuleName, types.ClearingAccountCommunity, sdk.NewCoins(communityAmount)))

		resp, err := queryService.ClearingAccountBalances(ctx, &types.QueryClearingAccountBalancesRequest{})
		requireT.NoError(err)
		requireT.NotNil(resp)
		requireT.Len(resp.Balances, 6)

		// Verify balances
		balanceMap := make(map[string]sdkmath.Int)
		for _, balance := range resp.Balances {
			balanceMap[balance.ClearingAccount] = balance.Balance
		}

		requireT.Equal(communityAmount.Amount, balanceMap[types.ClearingAccountCommunity])
		requireT.Equal(foundationAmount.Amount, balanceMap[types.ClearingAccountFoundation])
		requireT.Equal(teamAmount.Amount, balanceMap[types.ClearingAccountTeam])
		requireT.True(balanceMap[types.ClearingAccountAlliance].IsZero())
		requireT.True(balanceMap[types.ClearingAccountPartnership].IsZero())
		requireT.True(balanceMap[types.ClearingAccountInvestors].IsZero())
	})
}
