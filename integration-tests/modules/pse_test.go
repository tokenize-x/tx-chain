package modules

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
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
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestPSEDistribution(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	// create 3 new delegations
	for i := 0; i < 3; i++ {
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
	for i := 2; i >= 0; i-- {
		height, err = awaitScheduledDistributionEvent(ctx, chain, height)
		requireT.NoError(err)
		t.Logf("pse event occurred in height: %d", height)
		scheduledDistributions, err := getScheduledDistribution(ctx, chain)
		requireT.NoError(err)
		requireT.Equal(i, len(scheduledDistributions))
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
		}
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

func awaitScheduledDistributionEvent(ctx context.Context, chain integration.TXChain, startHeight int64) (int64, error) {
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
		return 0, err
	}

	return observedHeight, nil
}

func getScheduledDistribution(ctx context.Context, chain integration.TXChain) ([]psetypes.ScheduledDistribution, error) {
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	pseResponse, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	if err != nil {
		return nil, err
	}
	return pseResponse.ScheduledDistributions, nil
}
