//go:build integrationtests

package upgrade

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govtypesv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
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
	preUpgradeTimestamps   []uint64
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

	// Submit a distribution schedule via governance so migration has entries to re-key.
	allocationAmount := sdkmath.NewInt(1_000_000)
	allocations := make([]psetypes.ClearingAccountAllocation, 0)
	for _, clearingAccount := range psetypes.GetAllClearingAccounts() {
		allocations = append(allocations, psetypes.ClearingAccountAllocation{
			ClearingAccount: clearingAccount,
			Amount:          allocationAmount,
		})
	}

	// Use timestamps far in the future so they won't trigger before upgrade.
	ts1 := uint64(time.Now().Add(24 * time.Hour).Unix())
	ts2 := uint64(time.Now().Add(48 * time.Hour).Unix())
	p.preUpgradeTimestamps = []uint64{ts1, ts2}

	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil, "-", "-", "-", govtypesv1.OptionYes,
		&psetypes.MsgUpdateDistributionSchedule{
			Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			Schedule: []psetypes.ScheduledDistribution{
				{Timestamp: ts1, Allocations: allocations},
				{Timestamp: ts2, Allocations: allocations},
			},
		},
	)

	// Verify schedule was set on v6 chain.
	schedRes, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.Len(schedRes.ScheduledDistributions, 2, "v6 schedule should have 2 entries")

	t.Logf("PSE Before: params captured, validator delegator=%s score=%s, schedule set with %d entries",
		p.validatorDelegatorAddr, p.preUpgradeScore, len(schedRes.ScheduledDistributions))
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

	// AllocationSchedule should be re-keyed with sequential IDs by migration.
	schedRes, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.Len(schedRes.ScheduledDistributions, len(p.preUpgradeTimestamps),
		"migrated schedule should have same number of entries as pre-upgrade")
	for i, sd := range schedRes.ScheduledDistributions {
		requireT.Equal(uint64(i+1), sd.ID, "schedule entry %d should have sequential ID", i)
		requireT.Equal(p.preUpgradeTimestamps[i], sd.Timestamp,
			"schedule entry %d should preserve original timestamp", i)
	}

	// Validator's score should be preserved (and grown since time has passed).
	scoreRes, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{Address: p.validatorDelegatorAddr})
	requireT.NoError(err)
	requireT.True(scoreRes.Score.GTE(p.preUpgradeScore),
		"post-upgrade score (%s) should be >= pre-upgrade score (%s)", scoreRes.Score, p.preUpgradeScore)

	t.Logf("PSE After: params preserved, LastProcessedDistributionID=0, schedule re-keyed (%d entries with IDs), score preserved (%s -> %s)",
		len(schedRes.ScheduledDistributions), p.preUpgradeScore, scoreRes.Score)
}
