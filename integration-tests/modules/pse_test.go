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

	// create 3 new delegations
	for i := range 3 {
		validatorsResponse, err := stakingClient.Validators(
			ctx, &stakingtypes.QueryValidatorsRequest{Status: stakingtypes.Bonded.String()},
		)
		require.NoError(t, err)
		validator1 := validatorsResponse.Validators[0]

		delegateAmount := sdkmath.NewInt(100_000_000 * int64(i+1))
		acc := chain.GenAccount()
		chain.FundAccountWithOptions(ctx, t, acc, integration.BalancesOptions{
			Messages: []sdk.Msg{&stakingtypes.MsgDelegate{}},
			Amount:   delegateAmount,
		})
		delegateMsg := &stakingtypes.MsgDelegate{
			DelegatorAddress: acc.String(),
			ValidatorAddress: validator1.OperatorAddress,
			Amount:           sdk.NewCoin(chain.ChainSettings.Denom, delegateAmount),
		}
		_, err = client.BroadcastTx(
			ctx,
			chain.ClientContext.WithFromAddress(acc),
			chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg)),
			delegateMsg,
		)
		requireT.NoError(err)
	}

	allocations := make([]psetypes.ClearingAccountAllocation, 0)
	allocationAmount := sdkmath.NewInt(100_000_000_000_000) // 100 million tokens
	for _, clearingAccount := range psetypes.GetAllClearingAccounts() {
		allocations = append(allocations, psetypes.ClearingAccountAllocation{
			ClearingAccount: clearingAccount,
			Amount:          allocationAmount,
		})
	}

	// create 3 schedule distribution, each 30 seconds apart
	govParams, err := chain.Governance.QueryGovParams(ctx)
	requireT.NoError(err)
	distributionStartTime := time.Now().
		Add(10 * time.Second). // add 10 seconds for the proposal to be submitted and voted
		Add(*govParams.ExpeditedVotingPeriod)
	msgUpdateDistributionSchedule := &psetypes.MsgUpdateDistributionSchedule{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Schedule: []psetypes.ScheduledDistribution{
			{
				Timestamp:   uint64(distributionStartTime.Add(30 * time.Second).Unix()),
				Allocations: allocations,
			},
			{
				Timestamp:   uint64(distributionStartTime.Add(60 * time.Second).Unix()),
				Allocations: allocations,
			},
			{
				Timestamp:   uint64(distributionStartTime.Add(90 * time.Second).Unix()),
				Allocations: allocations,
			},
		},
	}
	mappings, err := upgradev6.DefaultClearingAccountMappings(chain.ChainSettings.ChainID)
	requireT.NoError(err)
	msgUpdateClearingAccountMappings := &psetypes.MsgUpdateClearingAccountMappings{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		Mappings:  mappings,
	}

	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil,
		"-", "-", "-", govtypesv1.OptionYes,
		msgUpdateDistributionSchedule,
		msgUpdateClearingAccountMappings,
	)

	// ensure distributions are done correctly with correct ratios
	header, err := chain.LatestBlockHeader(ctx)
	requireT.NoError(err)
	height := header.Height
	for i := range 3 {
		var events communityDistributedEvent
		height, events, err = awaitScheduledDistributionEvent(ctx, chain, height)
		requireT.NoError(err)
		t.Logf("pse event occurred in height: %d", height)
		scheduledDistributions, err := getScheduledDistribution(ctx, chain)
		requireT.NoError(err)
		requireT.Len(scheduledDistributions, 2-i)
		delegationAmountsBefore, delegatorScoresBefore, totalScoreBefore := getAllDelegatorInfo(ctx, t, chain, height-1)
		delegationAmountsAfter, _, _ := getAllDelegatorInfo(ctx, t, chain, height)
		for delegator, delegationAfter := range delegationAmountsAfter {
			delegationAmountBefore, exists := delegationAmountsBefore[delegator]
			requireT.True(exists)
			increasedAmount := delegationAfter.Sub(delegationAmountBefore)
			if !increasedAmount.IsPositive() {
				t.Fatalf("delegator: %s, delegation amount: %d, old delegation amount: %d",
					delegator,
					delegationAfter.Int64(),
					delegationAmountBefore.Int64(),
				)
			}
			delegatorScore := delegatorScoresBefore[delegator]
			expectedIncrease := allocationAmount.Mul(delegatorScore).Quo(totalScoreBefore)
			requireT.InEpsilon(expectedIncrease.Int64(), increasedAmount.Int64(), 0.05)
			event := events.find(delegator)
			requireT.NotNil(event)
			requireT.Equal(event.Amount.String(), increasedAmount.String())
		}
	}
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
