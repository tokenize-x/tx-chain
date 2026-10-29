//go:build integrationtests

package upgrade

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v8/integration-tests"
	customparamstypes "github.com/tokenize-x/tx-chain/v8/x/customparams/types"
)

// maxVotingPowerTest verifies that the upgrade sets max_voting_power to 1, which keeps the cap disabled.
type maxVotingPowerTest struct {
	minSelfDelegation sdkmath.Int
}

func (m *maxVotingPowerTest) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	params, err := customparamstypes.NewQueryClient(chain.ClientContext).
		StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)
	requireT.True(params.Params.MaxVotingPower.IsNil())
	m.minSelfDelegation = params.Params.MinSelfDelegation
}

func (m *maxVotingPowerTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	params, err := customparamstypes.NewQueryClient(chain.ClientContext).
		StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)
	requireT.Equal(sdkmath.LegacyOneDec().String(), params.Params.MaxVotingPower.String())
	requireT.Equal(m.minSelfDelegation.String(), params.Params.MinSelfDelegation.String())
}
