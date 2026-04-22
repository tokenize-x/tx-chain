//go:build integrationtests

package upgrade

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client/grpc/cmtservice"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
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

	// Params should be preserved across the upgrade, except for fields newly initialized by v7.
	paramsRes, err := pseClient.Params(ctx, &psetypes.QueryParamsRequest{})
	requireT.NoError(err)
	expectedParams := p.preUpgradeParams
	expectedParams.DistributionBatchSize = psetypes.DefaultParams().DistributionBatchSize
	requireT.Equal(expectedParams, paramsRes.Params)
	requireT.Equal(psetypes.DefaultParams().DistributionBatchSize, paramsRes.Params.DistributionBatchSize,
		"distribution_batch_size must be initialized to default by v7 upgrade")

	// LastProcessedDistributionID on the testnet branch is 2 (two single-block
	// distributions were processed before v7 upgrade).
	lastIDRes, err := pseClient.LastProcessedDistributionID(
		ctx, &psetypes.QueryLastProcessedDistributionIDRequest{},
	)
	requireT.NoError(err)
	requireT.Equal(uint64(2), lastIDRes.LastProcessedDistributionId)

	// AllocationSchedule should equal the full embedded schedule.
	schedRes, err := pseClient.ScheduledDistributions(
		ctx, &psetypes.QueryScheduledDistributionsRequest{},
	)
	requireT.NoError(err)

	expectedSchedule := loadEmbeddedSchedule(t)
	requireT.Len(schedRes.ScheduledDistributions, len(expectedSchedule),
		"migrated schedule should match embedded schedule count")

	// Verify sequential IDs and six clearing-account allocations per entry.
	for i, sd := range schedRes.ScheduledDistributions {
		requireT.Equal(uint64(i+1), sd.ID, "schedule entry %d should have sequential ID", i)
		requireT.Len(sd.Allocations, 6, "schedule entry %d should have 6 clearing accounts", i)
	}

	// Timestamps must match the embedded JSON (branch-specific values).
	requireT.Equal(mustParseUint64(t, expectedSchedule[0].Timestamp),
		schedRes.ScheduledDistributions[0].Timestamp)
	requireT.Equal(mustParseUint64(t, expectedSchedule[1].Timestamp),
		schedRes.ScheduledDistributions[1].Timestamp)
	last := schedRes.ScheduledDistributions[len(schedRes.ScheduledDistributions)-1]
	requireT.Equal(uint64(len(expectedSchedule)), last.ID)
	requireT.Equal(mustParseUint64(t, expectedSchedule[len(expectedSchedule)-1].Timestamp),
		last.Timestamp)
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

	// pse_community_intermediary must exist in state after the v7 migration.
	authClient := authtypes.NewQueryClient(chain.ClientContext)
	intermediaryAddr := authtypes.NewModuleAddress(psetypes.ClearingAccountCommunityIntermediary).String()
	accRes, err := authClient.Account(ctx, &authtypes.QueryAccountRequest{Address: intermediaryAddr})
	requireT.NoError(err, "pse_community_intermediary account must exist in state after upgrade")
	requireT.NotNil(accRes.Account, "pse_community_intermediary account response must not be nil")

	t.Logf("PSE After: schedule=%d entries, lastProcessedID=1, score %s -> %s (growth=%s, expected~%s, elapsed=%ds)",
		len(schedRes.ScheduledDistributions), p.preUpgradeScore, scoreRes.Score,
		actualGrowth, expectedGrowth, elapsedSec)
}

// embeddedScheduleEntry mirrors the JSON shape of the embedded schedule,
// extracting only the fields used by the test.
type embeddedScheduleEntry struct {
	Timestamp string `json:"timestamp"`
}

// loadEmbeddedSchedule reads the embedded schedule JSON and returns its
// entries. Timestamps are strings because the embedded JSON stores them as
// strings.
func loadEmbeddedSchedule(t *testing.T) []embeddedScheduleEntry {
	t.Helper()

	data, err := os.ReadFile("../../app/upgrade/v7/scheduled-distributions-mainnet.json")
	require.NoError(t, err)

	var schedule struct {
		ScheduledDistributions []embeddedScheduleEntry `json:"scheduled_distributions"` //nolint:tagliatelle
	}
	require.NoError(t, json.Unmarshal(data, &schedule))

	return schedule.ScheduledDistributions
}

// mustParseUint64 parses an unsigned 64-bit integer from a string or fails the test.
func mustParseUint64(t *testing.T, s string) uint64 {
	t.Helper()
	v, err := strconv.ParseUint(s, 10, 64)
	require.NoError(t, err)
	return v
}
