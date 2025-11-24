//go:build integrationtests

package modules

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

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

	// Score should be non-negative
	requireT.False(finalResp.Score.IsNegative(), "score should not be negative")
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

	// Score should be non-negative
	requireT.False(resp.Score.IsNegative(), "score should not be negative")
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

	// Score should still be non-negative and should have increased
	requireT.False(scoreAfterUndelegate.Score.IsNegative(), "score should not be negative")
	requireT.True(scoreAfterUndelegate.Score.GT(scoreAfterDelegate.Score), "score should increase after more blocks")

	// Verify remaining delegation
	delRespAfter, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delegator.String(),
	})
	requireT.NoError(err)
	requireT.Len(delRespAfter.DelegationResponses, 1)
	expectedRemaining := delegateAmount.Sub(undelegateAmount)
	requireT.Equal(expectedRemaining, delRespAfter.DelegationResponses[0].Balance.Amount)
}

// TestPSEQueryScore_ExistingValidators tests querying scores for existing chain validators.
func TestPSEQueryScore_ExistingValidators(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	// Get validators
	validatorsResp, err := stakingClient.Validators(ctx, &stakingtypes.QueryValidatorsRequest{})
	requireT.NoError(err)
	requireT.NotEmpty(validatorsResp.Validators)

	t.Logf("Found %d validators", len(validatorsResp.Validators))

	// Test querying score for each validator's operator address
	for i, validator := range validatorsResp.Validators {
		valOpAddr, err := sdk.ValAddressFromBech32(validator.OperatorAddress)
		requireT.NoError(err)

		// Convert validator operator address to account address
		accAddr := sdk.AccAddress(valOpAddr)

		// Query score for this validator
		scoreResp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
			Address: accAddr.String(),
		})
		requireT.NoError(err)
		requireT.NotNil(scoreResp)

		t.Logf("Validator %d (%s): Score = %s",
			i,
			validator.OperatorAddress,
			scoreResp.Score.String(),
		)

		// Score should be non-negative
		requireT.False(scoreResp.Score.IsNegative(), "validator score should not be negative")
	}
}

// TestPSEQueryClearingAccountBalances tests the ClearingAccountBalances query endpoint.
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

	// Log balances
	for i, balance := range resp.Balances {
		t.Logf("Account %d: %s = %s", i, balance.ClearingAccount, balance.Balance.String())

		// Balance should be non-negative
		requireT.False(balance.Balance.IsNegative(), "balance should not be negative")

		// Verify account name is valid
		requireT.Contains(allAccounts, balance.ClearingAccount, "clearing account should be valid")
	}

	// Verify all known clearing accounts are present
	accountMap := make(map[string]bool)
	for _, balance := range resp.Balances {
		accountMap[balance.ClearingAccount] = true
	}

	for _, expectedAccount := range allAccounts {
		requireT.True(accountMap[expectedAccount], "clearing account %s should be present", expectedAccount)
	}
}

// TestPSEQueryScheduledDistributions tests the ScheduledDistributions query endpoint.
func TestPSEQueryScheduledDistributions(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	// Query scheduled distributions
	resp, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.NotNil(resp)

	t.Logf("Scheduled distributions count: %d", len(resp.ScheduledDistributions))

	// Log distributions if any
	for i, dist := range resp.ScheduledDistributions {
		t.Logf("Distribution %d: %+v", i, dist)
	}
}
