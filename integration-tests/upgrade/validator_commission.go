//go:build integrationtests

package upgrade

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v7/integration-tests"
)

type validatorCommission struct{}

func (v *validatorCommission) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	paramsRes, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	requireT.NoError(err)

	// Before upgrade, min commission rate param should be less than 5%
	expectedMinCommission := sdkmath.LegacyNewDecWithPrec(5, 2)
	requireT.True(paramsRes.Params.MinCommissionRate.LT(expectedMinCommission))

	// Verify znet validators already have commission > 5% (they are created with 10%)
	validatorsRes, err := stakingClient.Validators(ctx, &stakingtypes.QueryValidatorsRequest{})
	requireT.NoError(err)
	requireT.NotEmpty(validatorsRes.Validators)

	for _, validator := range validatorsRes.Validators {
		requireT.True(validator.Commission.Rate.GT(expectedMinCommission),
			"validator %s should have commission > 5%%", validator.OperatorAddress)
	}

	t.Logf("Before upgrade: min commission rate = %s, validators count = %d",
		paramsRes.Params.MinCommissionRate.String(), len(validatorsRes.Validators))
}

func (v *validatorCommission) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	// Get staking params after upgrade
	paramsRes, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	requireT.NoError(err)

	// After upgrade, min commission rate should be 5%
	expectedMinCommission := sdkmath.LegacyNewDecWithPrec(5, 2)
	requireT.Equal(expectedMinCommission.String(), paramsRes.Params.MinCommissionRate.String(),
		"min commission rate after upgrade should be 5%")

	// Get all validators after upgrade
	validatorsRes, err := stakingClient.Validators(ctx, &stakingtypes.QueryValidatorsRequest{})
	requireT.NoError(err)

	// Verify all validators have at least 5% commission
	for _, validator := range validatorsRes.Validators {
		commissionRate := validator.Commission.Rate
		requireT.True(commissionRate.GTE(expectedMinCommission),
			"validator %s commission rate %s should be >= 5%%",
			validator.OperatorAddress, commissionRate.String())

		// MaxRate should also be at least 5%
		maxRate := validator.Commission.MaxRate
		requireT.True(maxRate.GTE(expectedMinCommission),
			"validator %s max rate %s should be >= 5%%",
			validator.OperatorAddress, maxRate.String())
	}

	t.Logf("After upgrade: min commission rate = %s, all %d validators have commission >= 5%%",
		paramsRes.Params.MinCommissionRate.String(), len(validatorsRes.Validators))
}
