//go:build integrationtests

package upgrade

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v7/integration-tests"
	psetypes "github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// pseMigrationTest verifies that the v6 -> v7 PSE store migration ran correctly.
type pseMigrationTest struct {
	preUpgradeParams psetypes.Params
	// Delegator address of a genesis validator (has self-delegation and a PSE score).
	validatorDelegatorAddr string
	preUpgradeScore        sdkmath.Int
}

func (p *pseMigrationTest) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	// Capture pre-upgrade params.
	paramsRes, err := pseClient.Params(ctx, &psetypes.QueryParamsRequest{})
	requireT.NoError(err)
	p.preUpgradeParams = paramsRes.Params

	// Get a bonded validator's delegator address to verify score is preserved.
	validatorsRes, err := stakingClient.Validators(
		ctx, &stakingtypes.QueryValidatorsRequest{Status: stakingtypes.Bonded.String()},
	)
	requireT.NoError(err)
	requireT.NotEmpty(validatorsRes.Validators)

	valAddr, err := sdk.ValAddressFromBech32(validatorsRes.Validators[0].OperatorAddress)
	requireT.NoError(err)
	delegatorAddr := sdk.AccAddress(valAddr)
	p.validatorDelegatorAddr = delegatorAddr.String()

	// Capture pre-upgrade score of the validator's self-delegation.
	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)
	p.preUpgradeScore = scoreRes.Score
	requireT.True(p.preUpgradeScore.GT(sdkmath.ZeroInt()), "genesis validator should have non-zero PSE score")

	t.Logf("PSE Before: params captured, validator delegator=%s score=%s", p.validatorDelegatorAddr, p.preUpgradeScore)
}

func (p *pseMigrationTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	// Params should be preserved across the upgrade.
	paramsRes, err := pseClient.Params(ctx, &psetypes.QueryParamsRequest{})
	requireT.NoError(err)
	requireT.Equal(p.preUpgradeParams, paramsRes.Params)

	// LastProcessedDistributionID should be initialized to 0 by migration.
	lastIDRes, err := pseClient.LastProcessedDistributionID(ctx, &psetypes.QueryLastProcessedDistributionIDRequest{})
	requireT.NoError(err)
	requireT.Equal(uint64(0), lastIDRes.LastProcessedDistributionId)

	// AllocationSchedule should be cleared by migration.
	schedRes, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.Empty(schedRes.ScheduledDistributions)

	// Validator's score should be preserved (and grown since time has passed).
	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)
	requireT.True(scoreRes.Score.GTE(p.preUpgradeScore),
		"post-upgrade score (%s) should be >= pre-upgrade score (%s)", scoreRes.Score, p.preUpgradeScore)

	t.Logf("PSE After: params preserved, LastProcessedDistributionID=0, schedule cleared, score preserved (%s -> %s)",
		p.preUpgradeScore, scoreRes.Score)
}
