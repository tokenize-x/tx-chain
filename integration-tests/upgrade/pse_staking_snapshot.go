//go:build integrationtests

package upgrade

import (
	"testing"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

type pseStakingSnapshot struct {
	delegatorAddresses []string
}

func (pss *pseStakingSnapshot) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	validators, err := stakingClient.Validators(ctx, &stakingtypes.QueryValidatorsRequest{
		Status: stakingtypes.Bonded.String(),
	})
	requireT.NoError(err)
	requireT.NotEmpty(validators.Validators)

	delegationsResponse, err := stakingClient.ValidatorDelegations(ctx, &stakingtypes.QueryValidatorDelegationsRequest{
		ValidatorAddr: validators.Validators[0].OperatorAddress,
	})
	requireT.NoError(err)
	requireT.NotEmpty(delegationsResponse.DelegationResponses)
	delegators := make([]string, 0)
	for _, delegator := range delegationsResponse.DelegationResponses {
		requireT.Positive(delegator.Balance.Amount.Int64())
		delegators = append(delegators, delegator.Delegation.DelegatorAddress)
	}
	pss.delegatorAddresses = delegators

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	_, err = pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: delegators[0]})
	requireT.Error(err)
	requireT.Contains(err.Error(), "Unimplemented")
}

func (pss *pseStakingSnapshot) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	for _, delegatorAddr := range pss.delegatorAddresses {
		score, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: delegatorAddr})
		requireT.NoError(err)
		requireT.Positive(score.Score.Int64(), "account: %s", delegatorAddr)
	}
}
