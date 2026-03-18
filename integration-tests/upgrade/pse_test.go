//go:build integrationtests

package upgrade

import (
	"encoding/json"
	"os"
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
	preUpgradeParams       psetypes.Params
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

	t.Logf("PSE Before: params captured, validator delegator=%s score=%s",
		p.validatorDelegatorAddr, p.preUpgradeScore)
}

func (p *pseMigrationTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	// Params should be preserved across the upgrade.
	paramsRes, err := pseClient.Params(ctx, &psetypes.QueryParamsRequest{})
	requireT.NoError(err)
	requireT.Equal(p.preUpgradeParams, paramsRes.Params)

	// LastProcessedDistributionID should be set to 1 by migration
	// (first distribution already processed by single-block logic).
	lastIDRes, err := pseClient.LastProcessedDistributionID(ctx, &psetypes.QueryLastProcessedDistributionIDRequest{})
	requireT.NoError(err)
	requireT.Equal(uint64(1), lastIDRes.LastProcessedDistributionId)

	// AllocationSchedule should be replaced with the full mainnet schedule from embedded JSON.
	schedRes, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)

	// Load expected schedule from the embedded JSON to verify count and content.
	expectedCount := loadMainnetScheduleCount(t)
	requireT.Len(schedRes.ScheduledDistributions, expectedCount,
		"migrated schedule should match embedded mainnet schedule count")

	// Verify sequential IDs.
	for i, sd := range schedRes.ScheduledDistributions {
		requireT.Equal(uint64(i+1), sd.ID, "schedule entry %d should have sequential ID", i)
		requireT.Len(sd.Allocations, 6, "schedule entry %d should have 6 clearing accounts", i)
	}

	// First entry (ID=1) should be the already-processed distribution.
	requireT.Equal(uint64(1775476800), schedRes.ScheduledDistributions[0].Timestamp)
	// Second entry (ID=2) is the first multi-block distribution.
	requireT.Equal(uint64(1778068800), schedRes.ScheduledDistributions[1].Timestamp)
	// Last entry (ID=84) boundary check.
	last := schedRes.ScheduledDistributions[len(schedRes.ScheduledDistributions)-1]
	requireT.Equal(uint64(expectedCount), last.ID)
	requireT.Equal(uint64(1993723200), last.Timestamp)
	requireT.Len(last.Allocations, 6)

	// Validator's score should be preserved (and grown since time has passed).
	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)
	requireT.True(scoreRes.Score.GTE(p.preUpgradeScore),
		"post-upgrade score (%s) should be >= pre-upgrade score (%s)", scoreRes.Score, p.preUpgradeScore)

	t.Logf("PSE After: params OK, schedule loaded (%d entries), lastProcessedID=1, score (%s -> %s)",
		len(schedRes.ScheduledDistributions), p.preUpgradeScore, scoreRes.Score)
}

// loadMainnetScheduleCount reads the embedded mainnet schedule JSON and returns the number of entries.
func loadMainnetScheduleCount(t *testing.T) int {
	t.Helper()

	data, err := os.ReadFile("../../app/upgrade/v7/scheduled-distributions-mainnet.json")
	require.NoError(t, err)

	var schedule struct {
		ScheduledDistributions []json.RawMessage `json:"scheduled_distributions"` //nolint:tagliatelle
	}
	require.NoError(t, json.Unmarshal(data, &schedule))

	return len(schedule.ScheduledDistributions)
}
