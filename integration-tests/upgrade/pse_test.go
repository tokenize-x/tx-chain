//go:build integrationtests

package upgrade

import (
	"encoding/json"
	"os"
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
	preScoreBlockBeforeSec int64
	preScoreBlockAfterSec  int64
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

	// Bracket the pre-upgrade score query with block times.
	beforeScoreBlock, err := tmClient.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
	requireT.NoError(err)
	p.preScoreBlockBeforeSec = beforeScoreBlock.SdkBlock.Header.Time.Unix()

	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)
	p.preUpgradeScore = scoreRes.Score
	requireT.True(p.preUpgradeScore.GT(sdkmath.ZeroInt()), "genesis validator should have non-zero PSE score")

	afterScoreBlock, err := tmClient.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
	requireT.NoError(err)
	p.preScoreBlockAfterSec = afterScoreBlock.SdkBlock.Header.Time.Unix()
	requireT.GreaterOrEqual(
		p.preScoreBlockAfterSec,
		p.preScoreBlockBeforeSec,
		"pre-score after block time must be >= pre-score before block time",
	)

	t.Logf("PSE Before: validator=%s tokens=%s score=%s preScoreWindow=[%d..%d]",
		p.validatorDelegatorAddr, p.validatorTokens, p.preUpgradeScore,
		p.preScoreBlockBeforeSec, p.preScoreBlockAfterSec)
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

	beforeScoreBlock, err := tmClient.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
	requireT.NoError(err)
	beforeScoreBlockTimeSec := beforeScoreBlock.SdkBlock.Header.Time.Unix()

	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)

	afterScoreBlock, err := tmClient.GetLatestBlock(ctx, &cmtservice.GetLatestBlockRequest{})
	requireT.NoError(err)
	afterScoreBlockTimeSec := afterScoreBlock.SdkBlock.Header.Time.Unix()

	elapsedMinSec := beforeScoreBlockTimeSec - p.preScoreBlockAfterSec
	elapsedMaxSec := afterScoreBlockTimeSec - p.preScoreBlockBeforeSec
	requireT.Positive(elapsedMinSec, "time must have elapsed between Before and After")
	requireT.GreaterOrEqual(elapsedMaxSec, elapsedMinSec, "elapsed max must be >= elapsed min")

	expectedGrowthMin := p.validatorTokens.MulRaw(elapsedMinSec)
	expectedGrowthMax := p.validatorTokens.MulRaw(elapsedMaxSec)

	actualGrowth := scoreRes.Score.Sub(p.preUpgradeScore)
	requireT.True(actualGrowth.IsPositive(), "score must have grown")

	// Allow 10% deviation around the expected range to account for block-time jitter.
	lowerBound := expectedGrowthMin.Sub(expectedGrowthMin.QuoRaw(10))
	upperBound := expectedGrowthMax.Add(expectedGrowthMax.QuoRaw(10))
	requireT.True(
		actualGrowth.GTE(lowerBound) && actualGrowth.LTE(upperBound),
		"score growth %s outside expected range [%s, %s] (elapsed window %ds..%ds, 10%% tolerance)",
		actualGrowth, lowerBound, upperBound, elapsedMinSec, elapsedMaxSec,
	)

	// pse_community_intermediary must exist in state after the v7 migration.
	authClient := authtypes.NewQueryClient(chain.ClientContext)
	intermediaryAddr := authtypes.NewModuleAddress(
		psetypes.ClearingAccountCommunityIntermediary,
	).String()
	accRes, err := authClient.Account(ctx, &authtypes.QueryAccountRequest{Address: intermediaryAddr})
	requireT.NoError(err, "pse_community_intermediary account must exist in state after upgrade")
	requireT.NotNil(accRes.Account, "pse_community_intermediary account response must not be nil")

	t.Logf(
		"PSE After: schedule=%d entries, lastProcessedID=1, "+
			"score %s -> %s (growth=%s, expectedRange=[%s..%s], "+
			"elapsedWindow=[%ds..%ds])",
		len(schedRes.ScheduledDistributions), p.preUpgradeScore, scoreRes.Score,
		actualGrowth, expectedGrowthMin, expectedGrowthMax, elapsedMinSec, elapsedMaxSec,
	)
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
