//go:build integrationtests

package upgrade

import (
	"encoding/json"
	"os"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
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
	preUpgradeBlockTimeSec int64
	validatorTokens        sdkmath.Int
}

func (p *pseMigrationTest) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	tmClient := cmtservice.NewServiceClient(chain.ClientContext)

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
	p.validatorTokens = validatorsRes.Validators[0].Tokens

	// Capture pre-upgrade score and block time for deterministic growth assertion.
	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)
	p.preUpgradeScore = scoreRes.Score
	requireT.True(p.preUpgradeScore.GT(sdkmath.ZeroInt()), "genesis validator should have non-zero PSE score")

	latestBlock, err := tmClient.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
	requireT.NoError(err)
	p.preUpgradeBlockTimeSec = latestBlock.SdkBlock.Header.Time.Unix()

	t.Logf("PSE Before: validator=%s tokens=%s score=%s blockTime=%d",
		p.validatorDelegatorAddr, p.validatorTokens, p.preUpgradeScore, p.preUpgradeBlockTimeSec)
}

func (p *pseMigrationTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	tmClient := cmtservice.NewServiceClient(chain.ClientContext)

	// Params should be preserved across the upgrade.
	paramsRes, err := pseClient.Params(ctx, &psetypes.QueryParamsRequest{})
	requireT.NoError(err)
	requireT.Equal(p.preUpgradeParams, paramsRes.Params)

	// LastProcessedDistributionID should be set to 1 by migration
	// (first distribution already processed by single-block logic).
	lastIDRes, err := pseClient.LastProcessedDistributionID(
		ctx, &psetypes.QueryLastProcessedDistributionIDRequest{},
	)
	requireT.NoError(err)
	requireT.Equal(uint64(1), lastIDRes.LastProcessedDistributionId)

	// AllocationSchedule should be replaced with the full mainnet schedule from embedded JSON.
	schedRes, err := pseClient.ScheduledDistributions(
		ctx, &psetypes.QueryScheduledDistributionsRequest{},
	)
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

	// Validate score growth (with a percentage-based tolerance).
	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)

	latestBlock, err := tmClient.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
	requireT.NoError(err)
	afterBlockTimeSec := latestBlock.SdkBlock.Header.Time.Unix()

	elapsedSec := afterBlockTimeSec - p.preUpgradeBlockTimeSec
	requireT.Positive(elapsedSec, "time must have elapsed between Before and After")

	expectedGrowth := p.validatorTokens.MulRaw(elapsedSec)

	actualGrowth := scoreRes.Score.Sub(p.preUpgradeScore)
	requireT.True(actualGrowth.IsPositive(), "score must have grown")

	// Allow 10% deviation to account for block time jitter between queries.
	diff := actualGrowth.Sub(expectedGrowth).Abs()
	maxDeviation := expectedGrowth.QuoRaw(10) // 10%
	requireT.True(diff.LTE(maxDeviation),
		"score growth %s deviates from expected %s by %s (>10%%)",
		actualGrowth, expectedGrowth, diff)

	t.Logf("PSE After: schedule=%d entries, lastProcessedID=1, score %s -> %s (growth=%s, expected~%s, elapsed=%ds)",
		len(schedRes.ScheduledDistributions), p.preUpgradeScore, scoreRes.Score,
		actualGrowth, expectedGrowth, elapsedSec)
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
