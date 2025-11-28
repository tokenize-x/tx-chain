package modules

import (
	"context"
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
	for range 3 {
		validatorsResponse, err := stakingClient.Validators(
			ctx, &stakingtypes.QueryValidatorsRequest{Status: stakingtypes.Bonded.String()},
		)
		require.NoError(t, err)
		validator1 := validatorsResponse.Validators[0]

		delegateAmount := sdkmath.NewInt(1_000_000)
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
	for i := 2; i >= 0; i-- {
		client.AwaitNextBlocks(ctx, chain.ClientContext, 3)
		allDelegationAmounts, allDelegatorScores, totalScore := getAllDelegatorInfo(ctx, t, chain)
		awaitScheduledDistributionCount(ctx, chain, i)
		updatedDelegationAmounts, _, _ := getAllDelegatorInfo(ctx, t, chain)
		for delegator, delegationAmount := range updatedDelegationAmounts {
			oldDelegationAmount, exists := allDelegationAmounts[delegator]
			requireT.True(exists)
			increasedAmount := delegationAmount.Sub(oldDelegationAmount)
			if !increasedAmount.IsPositive() {
				t.Fatalf("delegator: %s, delegation amount: %d, old delegation amount: %d",
					delegator,
					delegationAmount.Int64(),
					oldDelegationAmount.Int64(),
				)
			}
			delegatorScore := allDelegatorScores[delegator]
			expectedIncrease := allocationAmount.Mul(delegatorScore).Quo(totalScore)
			requireT.InEpsilon(expectedIncrease.Int64(), increasedAmount.Int64(), 0.001)
		}
	}
}

func getAllDelegatorInfo(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
) (map[string]sdkmath.Int, map[string]sdkmath.Int, sdkmath.Int) {

	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	requireT := require.New(t)

	// fix all queries to a certain height to avoid race conditions
	header, err := chain.LatestBlockHeader(ctx)
	requireT.NoError(err)
	ctx = context.WithValue(ctx, grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(header.Height, 10))

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

func awaitScheduledDistributionCount(ctx context.Context, chain integration.TXChain, count int) error {
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	chain.AwaitState(ctx, func(ctx context.Context) error {
		pseResponse, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
		if err != nil {
			return err
		}
		if len(pseResponse.ScheduledDistributions) != count {
			return errors.New("scheduled distribution count does not match")
		}
		return nil
	},
		integration.WithAwaitStateTimeout(40*time.Second),
	)

	return nil
}
