//go:build integrationtests

package modules

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	"github.com/tokenize-x/tx-chain/v6/pkg/client"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
	customparamstypes "github.com/tokenize-x/tx-chain/v6/x/customparams/types"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// TestPSEScore_DelegationFlow tests the end-to-end delegation flow and score accumulation.
func TestPSEScore_DelegationFlow(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	customParamsClient := customparamstypes.NewQueryClient(chain.ClientContext)

	// Get staking params
	customStakingParams, err := customParamsClient.StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)

	validatorStakingAmount := customStakingParams.Params.MinSelfDelegation.Mul(sdkmath.NewInt(2))

	// Create validator
	_, validatorAddress, deactivateValidator, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator()

	// Create delegator
	delegator := chain.GenAccount()
	delegateAmount := sdkmath.NewInt(1_000_000)

	chain.FundAccountWithOptions(ctx, t, delegator, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&stakingtypes.MsgDelegate{},
		},
		Amount: delegateAmount,
	})

	// Query initial score (should be zero)
	initialResp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.True(initialResp.Score.IsZero(), "initial score should be zero")

	// Delegate coins
	delegateMsg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(delegateAmount),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg)),
		delegateMsg,
	)
	requireT.NoError(err)

	t.Logf("Delegation executed, delegated %s to validator %s", delegateAmount.String(), validatorAddress.String())

	// Wait for blocks to accumulate score
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 2))

	// Query score after delegation
	finalResp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(finalResp)

	t.Logf("Score after delegation: %s", finalResp.Score.String())

	// Score should be positive after delegation and block accumulation
	requireT.True(finalResp.Score.IsPositive(), "score should be positive after delegation")
}

// TestPSEScore_MultipleDelegations tests score calculation with multiple delegation transactions.
// This test verifies that scores accumulate correctly when delegating to multiple validators.
func TestPSEScore_MultipleDelegations(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	customParamsClient := customparamstypes.NewQueryClient(chain.ClientContext)

	// Get staking params
	customStakingParams, err := customParamsClient.StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)

	validatorStakingAmount := customStakingParams.Params.MinSelfDelegation.Mul(sdkmath.NewInt(2))

	// Create two validators
	_, validator1Address, deactivateValidator1, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator1()

	_, validator2Address, deactivateValidator2, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator2()

	// Create delegator
	delegator := chain.GenAccount()
	delegateAmount1 := sdkmath.NewInt(500_000)
	delegateAmount2 := sdkmath.NewInt(300_000)
	totalAmount := delegateAmount1.Add(delegateAmount2)

	chain.FundAccountWithOptions(ctx, t, delegator, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&stakingtypes.MsgDelegate{},
			&stakingtypes.MsgDelegate{},
		},
		Amount: totalAmount,
	})

	// Delegate to first validator
	delegateMsg1 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator1Address.String(),
		Amount:           chain.NewCoin(delegateAmount1),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg1)),
		delegateMsg1,
	)
	requireT.NoError(err)

	t.Logf("First delegation executed: %s to validator %s", delegateAmount1.String(), validator1Address.String())

	// Delegate to second validator
	delegateMsg2 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator2Address.String(),
		Amount:           chain.NewCoin(delegateAmount2),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg2)),
		delegateMsg2,
	)
	requireT.NoError(err)

	t.Logf("Second delegation executed: %s to validator %s", delegateAmount2.String(), validator2Address.String())

	// Wait for some blocks to accumulate score
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 2))

	// Query score - should account for both delegations
	resp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp)

	t.Logf("Score with multiple delegations: %s", resp.Score.String())

	// Score should be positive after delegations and block accumulation
	requireT.True(resp.Score.IsPositive(), "score should be positive after delegations")
}

// TestPSEScore_UndelegationFlow tests the end-to-end undelegation flow and score behavior.
func TestPSEScore_UndelegationFlow(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	customParamsClient := customparamstypes.NewQueryClient(chain.ClientContext)

	// Get staking params
	customStakingParams, err := customParamsClient.StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)

	validatorStakingAmount := customStakingParams.Params.MinSelfDelegation.Mul(sdkmath.NewInt(2))

	// Create validator
	_, validatorAddress, deactivateValidator, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator()

	// Create delegator
	delegator := chain.GenAccount()
	delegateAmount := sdkmath.NewInt(1_000_000)
	undelegateAmount := sdkmath.NewInt(500_000)

	chain.FundAccountWithOptions(ctx, t, delegator, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&stakingtypes.MsgDelegate{},
			&stakingtypes.MsgUndelegate{},
			&stakingtypes.MsgUndelegate{},
		},
		Amount: delegateAmount,
	})

	// Delegate coins
	delegateMsg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(delegateAmount),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg)),
		delegateMsg,
	)
	requireT.NoError(err)

	t.Logf("Delegation executed: %s", delegateAmount.String())

	// Wait for blocks to accumulate some score
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 2))

	// Query score after delegation
	scoreAfterDelegate, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	t.Logf("Score after delegation: %s", scoreAfterDelegate.Score.String())

	// Verify delegation exists
	delResp, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delegator.String(),
	})
	requireT.NoError(err)
	requireT.Len(delResp.DelegationResponses, 1)
	requireT.Equal(delegateAmount, delResp.DelegationResponses[0].Balance.Amount)

	// Undelegate some coins
	undelegateMsg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(undelegateAmount),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(undelegateMsg)),
		undelegateMsg,
	)
	requireT.NoError(err)

	t.Logf("Undelegation executed: %s", undelegateAmount.String())

	// Wait for a block
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 1))

	// Query score after undelegation
	scoreAfterUndelegate, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(scoreAfterUndelegate)

	t.Logf("Score after undelegation: %s", scoreAfterUndelegate.Score.String())

	// Score should still be positive and should have increased
	requireT.True(scoreAfterUndelegate.Score.IsPositive(), "score should be positive")
	requireT.True(scoreAfterUndelegate.Score.GT(scoreAfterDelegate.Score), "score should increase after more blocks")

	// Verify remaining delegation
	delRespAfter, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delegator.String(),
	})
	requireT.NoError(err)
	requireT.Len(delRespAfter.DelegationResponses, 1)
	expectedRemaining := delegateAmount.Sub(undelegateAmount)
	requireT.Equal(expectedRemaining, delRespAfter.DelegationResponses[0].Balance.Amount)

	// Now undelegate the remaining amount
	undelegateRemainingMsg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(expectedRemaining),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(undelegateRemainingMsg)),
		undelegateRemainingMsg,
	)
	requireT.NoError(err)

	t.Logf("Full undelegation executed: %s", expectedRemaining.String())

	// Wait for a block
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 1))

	// Query score after full undelegation
	scoreAfterFullUndelegate, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)

	t.Logf("Score after full undelegation: %s", scoreAfterFullUndelegate.Score.String())

	// Wait for more blocks
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 3))

	// Query score again - it should not have increased since there's no active delegation
	scoreAfterWaiting, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)

	t.Logf("Score after waiting with no delegation: %s", scoreAfterWaiting.Score.String())

	// Score should not increase after full undelegation
	requireT.Equal(
		scoreAfterFullUndelegate.Score.String(), scoreAfterWaiting.Score.String(),
		"score should not increase after full undelegation")
}

// TestPSEQueryScore_AddressWithoutDelegation tests querying scores for addresses that have no delegation.
func TestPSEQueryScore_AddressWithoutDelegation(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	// Create multiple accounts that have never delegated
	addresses := []sdk.AccAddress{
		chain.GenAccount(),
		chain.GenAccount(),
		chain.GenAccount(),
	}

	// Query score for each address - all should be zero
	for i, addr := range addresses {
		scoreResp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
			Address: addr.String(),
		})
		requireT.NoError(err)
		requireT.NotNil(scoreResp)

		t.Logf("Address %d (%s): Score = %s", i, addr.String(), scoreResp.Score.String())

		// Score should be zero for addresses with no delegation
		requireT.True(scoreResp.Score.IsZero(), "score should be zero for address with no delegation")
	}
}

// TestPSEQueryClearingAccountBalances tests the ClearingAccountBalances query endpoint.
// Note: In znet, the PSE upgrade handler has already run and funded these accounts
// with the initial mint of 100 billion tokens according to the allocation percentages.
func TestPSEQueryClearingAccountBalances(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	// Query clearing account balances
	resp, err := pseClient.ClearingAccountBalances(ctx, &psetypes.QueryClearingAccountBalancesRequest{})
	requireT.NoError(err)
	requireT.NotNil(resp)
	requireT.NotNil(resp.Balances)

	// Should return all clearing accounts
	allAccounts := psetypes.GetAllClearingAccounts()
	requireT.Len(resp.Balances, len(allAccounts), "should return all clearing accounts")

	t.Logf("Clearing account balances count: %d", len(resp.Balances))

	// Calculate expected balances from PSE upgrade handler constants
	// This ensures the test stays in sync with any changes to the upgrade handler
	totalMintAmount := sdkmath.NewInt(v6.InitialTotalMint)
	allocations := v6.DefaultInitialFundAllocations()

	expectedBalances := make(map[string]sdkmath.Int)
	for _, allocation := range allocations {
		expectedAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		expectedBalances[allocation.ClearingAccount] = expectedAmount
	}

	// Verify balances and log them
	balanceMap := make(map[string]sdkmath.Int)
	for _, balance := range resp.Balances {
		balanceMap[balance.ClearingAccount] = balance.Balance
		t.Logf("Account %s = %s", balance.ClearingAccount, balance.Balance.String())

		// Verify account name is valid
		requireT.Contains(allAccounts, balance.ClearingAccount, "clearing account should be valid")

		// Verify balance matches expected amount from PSE upgrade handler
		expectedBalance, exists := expectedBalances[balance.ClearingAccount]
		requireT.True(exists, "should have expected balance for %s", balance.ClearingAccount)
		requireT.Equal(expectedBalance, balance.Balance,
			"balance for %s should match PSE upgrade handler allocation", balance.ClearingAccount)
	}

	// Verify all known clearing accounts are present
	for _, expectedAccount := range allAccounts {
		_, exists := balanceMap[expectedAccount]
		requireT.True(exists, "clearing account %s should be present", expectedAccount)
	}
}
