//go:build integrationtests

package upgrade

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govtypesv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/stretchr/testify/require"

	appupgradev8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	integrationtests "github.com/tokenize-x/tx-chain/v8/integration-tests"
	psetypes "github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// psePostponeTest verifies that the v8 upgrade postpones PSE distributions after the cutoff by one year.
// Devnet starts without a PSE schedule, so the test seeds one around the cutoff before the upgrade.
type psePostponeTest struct {
	before []psetypes.ScheduledDistribution
}

func (p *psePostponeTest) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	res, err := pseClient.UnprocessedScheduledDistributions(
		ctx, &psetypes.QueryUnprocessedScheduledDistributionsRequest{},
	)
	requireT.NoError(err)
	requireT.False(res.DisableDistributions)

	allocations := make([]psetypes.ClearingAccountAllocation, 0)
	for _, clearingAccount := range psetypes.GetAllClearingAccounts() {
		allocations = append(allocations, psetypes.ClearingAccountAllocation{
			ClearingAccount: clearingAccount,
			Amount:          sdkmath.NewInt(1_000_000),
		})
	}

	// A distribution on the cutoff must keep its date, but it can only be scheduled while the cutoff is ahead.
	cutoff := appupgradev8.PSEPostponeCutoff
	timestamps := []time.Time{cutoff.AddDate(0, 1, 0), cutoff.AddDate(0, 2, 0)}
	if time.Until(cutoff) > 24*time.Hour {
		timestamps = append([]time.Time{cutoff}, timestamps...)
	}

	schedule := make([]psetypes.ScheduledDistribution, 0, len(timestamps))
	for i, timestamp := range timestamps {
		schedule = append(schedule, psetypes.ScheduledDistribution{
			ID:          res.LastProcessedDistributionId + uint64(i) + 1,
			Timestamp:   uint64(timestamp.Unix()),
			Allocations: allocations,
		})
	}

	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil, "-", "-", "-", govtypesv1.OptionYes,
		&psetypes.MsgUpdateDistributionSchedule{
			Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			Schedule:  schedule,
		},
	)

	res, err = pseClient.UnprocessedScheduledDistributions(ctx, &psetypes.QueryUnprocessedScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.Len(res.ScheduledDistributions, len(schedule))
	p.before = res.ScheduledDistributions
}

func (p *psePostponeTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	res, err := pseClient.UnprocessedScheduledDistributions(
		ctx, &psetypes.QueryUnprocessedScheduledDistributionsRequest{},
	)
	requireT.NoError(err)
	requireT.False(res.DisableDistributions, "postponing must not disable PSE")
	requireT.Len(res.ScheduledDistributions, len(p.before))

	cutoff := uint64(appupgradev8.PSEPostponeCutoff.Unix())
	for i, before := range p.before {
		after := res.ScheduledDistributions[i]
		requireT.Equal(before.ID, after.ID)
		requireT.Equal(before.Allocations, after.Allocations)

		expected := before.Timestamp
		if expected > cutoff {
			expected = uint64(time.Unix(int64(expected), 0).UTC().AddDate(1, 0, 0).Unix())
		}
		requireT.Equalf(expected, after.Timestamp, "distribution %d", after.ID)
	}
}

// mintParamsTest verifies that the v8 upgrade pins inflation for the PSE postponement.
type mintParamsTest struct {
	before minttypes.Params
}

func (m *mintParamsTest) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)

	res, err := minttypes.NewQueryClient(chain.ClientContext).Params(ctx, &minttypes.QueryParamsRequest{})
	require.NoError(t, err)
	m.before = res.Params
}

func (m *mintParamsTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	mintClient := minttypes.NewQueryClient(chain.ClientContext)
	paramsRes, err := mintClient.Params(ctx, &minttypes.QueryParamsRequest{})
	requireT.NoError(err)
	params := paramsRes.Params
	requireT.Equal(appupgradev8.PSEPauseInflation.String(), params.InflationMin.String())
	requireT.Equal(appupgradev8.PSEPauseInflation.String(), params.InflationMax.String())
	requireT.Equal(appupgradev8.PSEPauseBlocksPerYear, params.BlocksPerYear)
	requireT.Equal(m.before.MintDenom, params.MintDenom)
	requireT.Equal(m.before.InflationRateChange.String(), params.InflationRateChange.String())
	requireT.Equal(m.before.GoalBonded.String(), params.GoalBonded.String())

	inflationRes, err := mintClient.Inflation(ctx, &minttypes.QueryInflationRequest{})
	requireT.NoError(err)
	requireT.Equal(appupgradev8.PSEPauseInflation.String(), inflationRes.Inflation.String())
}
