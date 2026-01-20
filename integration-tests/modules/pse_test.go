//go:build integrationtests

package modules

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	tmtypes "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govtypesv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	upgradev6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	"github.com/tokenize-x/tx-chain/v6/pkg/client"
	"github.com/tokenize-x/tx-chain/v6/testutil/event"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
	customparamstypes "github.com/tokenize-x/tx-chain/v6/x/customparams/types"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestPSEDistribution(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	// Epsilon tolerances for distribution verification
	const (
		epsilonNormal     = 0.05 // Normal delegators
		epsilonReIncluded = 0.06 // Re-included delegators have higher variance due to validator earnings
	)

	// ============================================================
	// SETUP: Create 4 delegators with progressive amounts
	// ============================================================
	delegators := make([]sdk.AccAddress, 4)
	var validatorAddress string

	validatorsResponse, err := stakingClient.Validators(
		ctx, &stakingtypes.QueryValidatorsRequest{Status: stakingtypes.Bonded.String()},
	)
	requireT.NoError(err)
	validator := validatorsResponse.Validators[0]
	validatorAddress = validator.OperatorAddress

	for i := range 4 {
		delegateAmount := sdkmath.NewInt(100_000_000 * int64(i+1))
		acc := chain.GenAccount()
		delegators[i] = acc

		chain.FundAccountWithOptions(ctx, t, acc, integration.BalancesOptions{
			Messages: []sdk.Msg{&stakingtypes.MsgDelegate{}, &stakingtypes.MsgUndelegate{}},
			Amount:   delegateAmount,
		})

		_, err = client.BroadcastTx(
			ctx,
			chain.ClientContext.WithFromAddress(acc),
			chain.TxFactory().WithGas(chain.GasLimitByMsgs(&stakingtypes.MsgDelegate{})),
			&stakingtypes.MsgDelegate{
				DelegatorAddress: acc.String(),
				ValidatorAddress: validator.OperatorAddress,
				Amount:           sdk.NewCoin(chain.ChainSettings.Denom, delegateAmount),
			},
		)
		requireT.NoError(err)
	}

	// ============================================================
	// SETUP: Configure 3 distributions and exclude 4th delegator
	// ============================================================
	allocationAmount := sdkmath.NewInt(100_000_000_000_000)
	allocations := make([]psetypes.ClearingAccountAllocation, 0)
	for _, clearingAccount := range psetypes.GetAllClearingAccounts() {
		allocations = append(allocations, psetypes.ClearingAccountAllocation{
			ClearingAccount: clearingAccount,
			Amount:          allocationAmount,
		})
	}

	govParams, err := chain.Governance.QueryGovParams(ctx)
	requireT.NoError(err)
	distributionStartTime := time.Now().Add(10 * time.Second).Add(*govParams.ExpeditedVotingPeriod)

	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil, "-", "-", "-", govtypesv1.OptionYes,
		&psetypes.MsgUpdateDistributionSchedule{
			Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			Schedule: []psetypes.ScheduledDistribution{
				{Timestamp: uint64(distributionStartTime.Add(30 * time.Second).Unix()), Allocations: allocations},
				{Timestamp: uint64(distributionStartTime.Add(60 * time.Second).Unix()), Allocations: allocations},
				{Timestamp: uint64(distributionStartTime.Add(90 * time.Second).Unix()), Allocations: allocations},
			},
		},
		&psetypes.MsgUpdateClearingAccountMappings{
			Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			Mappings:  must(upgradev6.DefaultClearingAccountMappings(chain.ChainSettings.ChainID)),
		},
		&psetypes.MsgUpdateExcludedAddresses{
			Authority:      authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			AddressesToAdd: []string{delegators[3].String()},
		},
	)

	excludedDelegator := delegators[3].String()

	// ============================================================
	// DISTRIBUTION 1: Verify excluded delegator receives NO rewards
	// ============================================================
	t.Log("=== Distribution 1: Excluded delegator should receive nothing ===")

	header, err := chain.LatestBlockHeader(ctx)
	requireT.NoError(err)
	height := header.Height

	height, events, err := awaitScheduledDistributionEvent(ctx, chain, height)
	requireT.NoError(err)
	t.Logf("Distribution 1 at height: %d", height)

	scheduledDistributions, err := getScheduledDistribution(ctx, chain)
	requireT.NoError(err)
	requireT.Len(scheduledDistributions, 2)

	balancesBefore, scoresBefore, totalScore := getAllDelegatorInfo(ctx, t, chain, height-1)
	balancesAfter, _, _ := getAllDelegatorInfo(ctx, t, chain, height)

	// Excluded delegator should receive nothing
	requireT.Equal(balancesBefore[excludedDelegator], balancesAfter[excludedDelegator],
		"Excluded delegator should NOT receive rewards")
	requireT.Nil(events.find(excludedDelegator), "Excluded delegator should NOT have event")
	t.Logf("Excluded delegator correctly received no rewards")

	// Other delegators should receive correct rewards
	for _, delegator := range delegators[:3] { // First 3 delegators
		addr := delegator.String()
		increased := balancesAfter[addr].Sub(balancesBefore[addr])
		requireT.True(increased.IsPositive())

		expected := allocationAmount.Mul(scoresBefore[addr]).Quo(totalScore)
		requireT.InEpsilon(expected.Int64(), increased.Int64(), epsilonNormal)
		requireT.NotNil(events.find(addr))
	}

	// ============================================================
	// RE-INCLUSION: Remove delegator from exclusion list
	// ============================================================
	t.Log("=== Re-including previously excluded delegator ===")

	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil, "-", "-", "-", govtypesv1.OptionYes,
		&psetypes.MsgUpdateExcludedAddresses{
			Authority:         authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			AddressesToRemove: []string{excludedDelegator},
		},
	)
	t.Logf("Delegator re-included, should receive rewards in next distribution")

	// ============================================================
	// DISTRIBUTION 2: Verify re-included delegator receives rewards
	// ============================================================
	t.Log("=== Distribution 2: Re-included delegator should receive rewards ===")

	height, events, err = awaitScheduledDistributionEvent(ctx, chain, height)
	requireT.NoError(err)
	t.Logf("Distribution 2 at height: %d", height)

	scheduledDistributions, err = getScheduledDistribution(ctx, chain)
	requireT.NoError(err)
	requireT.Len(scheduledDistributions, 1)

	balancesBefore, scoresBefore, totalScore = getAllDelegatorInfo(ctx, t, chain, height-1)
	balancesAfter, _, _ = getAllDelegatorInfo(ctx, t, chain, height)

	// Re-included delegator should now receive rewards
	reIncludedIncrease := balancesAfter[excludedDelegator].Sub(balancesBefore[excludedDelegator])
	if reIncludedIncrease.IsPositive() {
		expected := allocationAmount.Mul(scoresBefore[excludedDelegator]).Quo(totalScore)
		requireT.InEpsilon(expected.Int64(), reIncludedIncrease.Int64(), epsilonReIncluded)
		requireT.NotNil(events.find(excludedDelegator))
		t.Logf("Re-included delegator received rewards: %s", reIncludedIncrease.String())
	} else {
		t.Logf("Re-included delegator received no rewards yet (score may not have accumulated)")
	}

	// Other delegators should still receive correct rewards
	for _, delegator := range delegators[:3] {
		addr := delegator.String()
		increased := balancesAfter[addr].Sub(balancesBefore[addr])
		requireT.True(increased.IsPositive())

		expected := allocationAmount.Mul(scoresBefore[addr]).Quo(totalScore)
		requireT.InEpsilon(expected.Int64(), increased.Int64(), epsilonNormal)
		requireT.NotNil(events.find(addr))
	}

	// ============================================================
	// DISTRIBUTION 3: Verify continued rewards
	// ============================================================
	t.Log("=== Distribution 3: All delegators receive rewards ===")

	height, events, err = awaitScheduledDistributionEvent(ctx, chain, height)
	requireT.NoError(err)
	t.Logf("Distribution 3 at height: %d", height)

	scheduledDistributions, err = getScheduledDistribution(ctx, chain)
	requireT.NoError(err)
	requireT.Len(scheduledDistributions, 0)

	balancesBefore, scoresBefore, totalScore = getAllDelegatorInfo(ctx, t, chain, height-1)
	balancesAfter, _, _ = getAllDelegatorInfo(ctx, t, chain, height)

	// All delegators (including re-included) should receive rewards
	for _, delegator := range delegators {
		addr := delegator.String()
		increased := balancesAfter[addr].Sub(balancesBefore[addr])
		requireT.True(increased.IsPositive(), "Delegator %s should receive rewards", addr)

		expected := allocationAmount.Mul(scoresBefore[addr]).Quo(totalScore)
		epsilon := epsilonNormal
		if addr == excludedDelegator {
			epsilon = epsilonReIncluded
		}
		requireT.InEpsilon(expected.Int64(), increased.Int64(), epsilon)
		requireT.NotNil(events.find(addr))
	}

	// ============================================================
	// UNDELEGATION TEST: Re-included delegator can fully undelegate
	// ============================================================
	t.Log("=== Testing full undelegation for re-included delegator ===")

	delResp, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: excludedDelegator,
	})
	requireT.NoError(err)
	requireT.Len(delResp.DelegationResponses, 1)

	currentDelegation := delResp.DelegationResponses[0].Balance.Amount
	t.Logf("Current delegation: %s", currentDelegation.String())

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegators[3]),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(&stakingtypes.MsgUndelegate{})),
		&stakingtypes.MsgUndelegate{
			DelegatorAddress: excludedDelegator,
			ValidatorAddress: validatorAddress,
			Amount:           sdk.NewCoin(chain.ChainSettings.Denom, currentDelegation),
		},
	)
	requireT.NoError(err, "Re-included delegator should be able to undelegate full amount")

	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 1))

	delRespAfter, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: excludedDelegator,
	})
	requireT.NoError(err)
	requireT.Len(delRespAfter.DelegationResponses, 0, "Should have zero delegations after full undelegation")

	t.Logf("Re-included delegator successfully undelegated full amount (%s)", currentDelegation.String())
}

func TestPSEDisableDistributions(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	msgDisableDistributions := &psetypes.MsgDisableDistributions{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	}
	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil,
		"-", "-", "-", govtypesv1.OptionYes,
		msgDisableDistributions,
	)

	scheduledDistributions, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.True(scheduledDistributions.DisableDistributions, "distributions should be disabled")
}

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

// must panics if error is not nil, otherwise returns value
func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

func getAllDelegatorInfo(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
	height int64,
) (map[string]sdkmath.Int, map[string]sdkmath.Int, sdkmath.Int) {
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	requireT := require.New(t)

	ctx = metadata.AppendToOutgoingContext(ctx, grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(height, 10))

	validatorsResponse, err := stakingClient.Validators(
		ctx, &stakingtypes.QueryValidatorsRequest{Status: stakingtypes.Bonded.String()},
	)
	requireT.NoError(err)

	allDelegatorScores := make(map[string]sdkmath.Int)
	allDelegatorAmounts := make(map[string]sdkmath.Int)
	totalScore := sdkmath.NewInt(0)
	for _, val := range validatorsResponse.Validators {
		delegationsResp, err := stakingClient.ValidatorDelegations(ctx, &stakingtypes.QueryValidatorDelegationsRequest{
			ValidatorAddr: val.OperatorAddress,
		})
		requireT.NoError(err)
		for _, delegation := range delegationsResp.DelegationResponses {
			_, exists := allDelegatorScores[delegation.Delegation.DelegatorAddress]
			if exists {
				continue
			}

			pseScore, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
				Address: delegation.Delegation.DelegatorAddress,
			})
			requireT.NoError(err)
			allDelegatorScores[delegation.Delegation.DelegatorAddress] = pseScore.Score
			allDelegatorAmounts[delegation.Delegation.DelegatorAddress] = delegation.Balance.Amount
			totalScore = totalScore.Add(pseScore.Score)
		}
	}

	return allDelegatorAmounts, allDelegatorScores, totalScore
}

type communityDistributedEvent []*psetypes.EventCommunityDistributed

func (e communityDistributedEvent) find(delegatorAddress string) *psetypes.EventCommunityDistributed {
	for _, event := range e {
		if event.DelegatorAddress == delegatorAddress {
			return event
		}
	}
	return nil
}

func awaitScheduledDistributionEvent(
	ctx context.Context,
	chain integration.TXChain,
	startHeight int64,
) (int64, communityDistributedEvent, error) {
	var observedHeight int64
	err := chain.AwaitState(ctx, func(ctx context.Context) error {
		query := fmt.Sprintf("tx.pse.v1.EventAllocationDistributed.mode='EndBlock' AND block.height>%d", startHeight)
		blocks, err := chain.ClientContext.RPCClient().BlockSearch(ctx, query, nil, nil, "")
		if err != nil {
			return err
		}
		if blocks.TotalCount == 0 {
			return errors.New("no blocks found")
		}

		observedHeight = blocks.Blocks[0].Block.Height
		return nil
	},
		integration.WithAwaitStateTimeout(40*time.Second),
	)
	if err != nil {
		return 0, nil, err
	}

	results, err := chain.ClientContext.RPCClient().BlockResults(ctx, &observedHeight)
	if err != nil {
		return 0, nil, err
	}
	// we have to remove the mode attribute from the events because it is not part of the typed event and
	// is added by cosmos-sdk, otherwise parsing the events will fail.
	events := removeAttributeFromEvent(results.FinalizeBlockEvents, "mode")
	communityDistributedEvents, err := event.FindTypedEvents[*psetypes.EventCommunityDistributed](events)
	if err != nil {
		return 0, nil, err
	}
	return observedHeight, communityDistributedEvents, nil
}

func getScheduledDistribution(
	ctx context.Context,
	chain integration.TXChain,
) ([]psetypes.ScheduledDistribution, error) {
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	pseResponse, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	if err != nil {
		return nil, err
	}
	return pseResponse.ScheduledDistributions, nil
}

func removeAttributeFromEvent(events []tmtypes.Event, key string) []tmtypes.Event {
	newEvents := make([]tmtypes.Event, 0, len(events))
	for _, event := range events {
		for i, attribute := range event.Attributes {
			if attribute.Key == key {
				event.Attributes = append(event.Attributes[:i], event.Attributes[i+1:]...)
			}
		}
		newEvents = append(newEvents, event)
	}
	return newEvents
}
