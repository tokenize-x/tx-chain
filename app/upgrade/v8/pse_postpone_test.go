package v8_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	v8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	psetypes "github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// mainnetPSEDistributionCount is the number of monthly distributions in the mainnet PSE schedule.
const mainnetPSEDistributionCount = 84

// mainnetPSESchedule rebuilds the mainnet PSE schedule.
// It has 84 monthly distributions on the 6th at 12:00 UTC, starting in April 2026.
func mainnetPSESchedule() []psetypes.ScheduledDistribution {
	return monthlyPSESchedule(time.Date(2026, time.April, 6, 12, 0, 0, 0, time.UTC), mainnetPSEDistributionCount)
}

// testnetPSESchedule rebuilds the testnet PSE schedule.
// It has 86 monthly distributions on the 5th at 12:00 UTC, starting in January 2026.
func testnetPSESchedule() []psetypes.ScheduledDistribution {
	return monthlyPSESchedule(time.Date(2026, time.January, 5, 12, 0, 0, 0, time.UTC), 86)
}

// monthlyPSESchedule builds count monthly distributions with IDs from 1, starting at first, with mainnet amounts.
func monthlyPSESchedule(first time.Time, count int) []psetypes.ScheduledDistribution {
	amounts := map[string]int64{
		psetypes.ClearingAccountCommunity:   476190476190476,
		psetypes.ClearingAccountFoundation:  357142857142857,
		psetypes.ClearingAccountAlliance:    238095238095238,
		psetypes.ClearingAccountPartnership: 35714285714285,
		psetypes.ClearingAccountInvestors:   59523809523809,
		psetypes.ClearingAccountTeam:        23809523809523,
	}

	schedule := make([]psetypes.ScheduledDistribution, 0, count)
	for i := range count {
		allocations := make([]psetypes.ClearingAccountAllocation, 0, len(amounts))
		for _, account := range psetypes.GetAllClearingAccounts() {
			allocations = append(allocations, psetypes.ClearingAccountAllocation{
				ClearingAccount: account,
				Amount:          sdkmath.NewInt(amounts[account]),
			})
		}
		schedule = append(schedule, psetypes.ScheduledDistribution{
			ID:          uint64(i + 1),
			Timestamp:   uint64(first.AddDate(0, i, 0).Unix()),
			Allocations: allocations,
		})
	}

	return schedule
}

// setupPSESchedule stores the mainnet schedule with the given number of already processed distributions.
func setupPSESchedule(
	t *testing.T,
	lastProcessedID uint64,
) (*simapp.App, sdk.Context, []psetypes.ScheduledDistribution) {
	t.Helper()

	return setupSchedule(t, mainnetPSESchedule(), lastProcessedID)
}

// setupSchedule stores the given schedule with the given number of already processed distributions.
func setupSchedule(
	t *testing.T,
	schedule []psetypes.ScheduledDistribution,
	lastProcessedID uint64,
) (*simapp.App, sdk.Context, []psetypes.ScheduledDistribution) {
	t.Helper()
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	requireT.NoError(testApp.PSEKeeper.AllocationSchedule.Clear(ctx, nil))
	for _, distribution := range schedule {
		requireT.NoError(testApp.PSEKeeper.AllocationSchedule.Set(ctx, distribution.ID, distribution))
	}
	requireT.NoError(testApp.PSEKeeper.LastProcessedDistributionID.Set(ctx, lastProcessedID))

	return testApp, ctx, schedule
}

// requirePostponed checks the stored schedule against the original.
// Distributions up to and including the cutoff keep their dates, later ones move by exactly one year.
func requirePostponed(
	t *testing.T,
	testApp *simapp.App,
	ctx sdk.Context,
	original []psetypes.ScheduledDistribution,
) {
	t.Helper()
	requirePostponedAt(t, testApp, ctx, original, v8.PSEPostponeCutoff)
}

// requirePostponedAt is requirePostponed for a given cutoff.
func requirePostponedAt(
	t *testing.T,
	testApp *simapp.App,
	ctx sdk.Context,
	original []psetypes.ScheduledDistribution,
	cutoffTime time.Time,
) {
	t.Helper()
	requireT := require.New(t)

	stored, err := testApp.PSEKeeper.GetDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.Len(stored, len(original))

	cutoff := uint64(cutoffTime.Unix())
	for i, distribution := range stored {
		requireT.Equal(original[i].ID, distribution.ID)
		requireT.Equal(original[i].Allocations, distribution.Allocations)

		expected := original[i].Timestamp
		if expected > cutoff {
			expected = uint64(time.Unix(int64(expected), 0).UTC().AddDate(1, 0, 0).Unix())
		}
		requireT.Equalf(expected, distribution.Timestamp, "distribution %d", distribution.ID)
	}
}

// TestPostponePSEDistributions_MatchesProposal46 checks the dates proposal 46 promises on the mainnet schedule.
// October and November 2026 stay, December 2026 becomes December 2027, and the last distribution moves to March 2034.
func TestPostponePSEDistributions_MatchesProposal46(t *testing.T) {
	requireT := require.New(t)

	// Mainnet state before the October 2026 distribution.
	testApp, ctx, original := setupPSESchedule(t, 6)
	requireT.NoError(v8.PostponePSEDistributions(ctx, testApp.PSEKeeper, v8.PSEPostponeCutoff))
	requirePostponed(t, testApp, ctx, original)

	expectedDates := map[uint64]time.Time{
		7:  time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC),
		8:  time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC),
		9:  time.Date(2027, time.December, 6, 12, 0, 0, 0, time.UTC),
		10: time.Date(2028, time.January, 6, 12, 0, 0, 0, time.UTC),
		84: time.Date(2034, time.March, 6, 12, 0, 0, 0, time.UTC),
	}
	for id, expected := range expectedDates {
		distribution, err := testApp.PSEKeeper.AllocationSchedule.Get(ctx, id)
		requireT.NoError(err)
		requireT.Equalf(uint64(expected.Unix()), distribution.Timestamp, "distribution %d", id)
	}

	// Postponing must not disable PSE, because nothing but an upgrade can re-enable it.
	disabled, err := testApp.PSEKeeper.DistributionDisabled.Get(ctx)
	if err == nil {
		requireT.False(disabled)
	}

	unprocessed, err := testApp.PSEKeeper.GetUnprocessedDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.NoError(psetypes.ValidateDistributionSchedule(unprocessed))
}

// TestPostponePSEDistributions_Testnet checks the testnet schedule with the testnet cutoff (October 5, 2026).
// Testnet pays on the 5th and is three IDs ahead of mainnet; its pause starts a month before mainnet's.
func TestPostponePSEDistributions_Testnet(t *testing.T) {
	requireT := require.New(t)

	// Testnet state on 2026-10-02: distributions up to September 2026 (ID 9) are processed.
	testApp, ctx, original := setupSchedule(t, testnetPSESchedule(), 9)
	cutoff := v8.PSEPostponeCutoffFor(string(constant.ChainIDTest))
	requireT.NoError(v8.PostponePSEDistributions(ctx, testApp.PSEKeeper, cutoff))
	requirePostponedAt(t, testApp, ctx, original, cutoff)

	expectedDates := map[uint64]time.Time{
		10: time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC),
		11: time.Date(2027, time.November, 5, 12, 0, 0, 0, time.UTC),
		12: time.Date(2027, time.December, 5, 12, 0, 0, 0, time.UTC),
		86: time.Date(2034, time.February, 5, 12, 0, 0, 0, time.UTC),
	}
	for id, expected := range expectedDates {
		distribution, err := testApp.PSEKeeper.AllocationSchedule.Get(ctx, id)
		requireT.NoError(err)
		requireT.Equalf(uint64(expected.Unix()), distribution.Timestamp, "distribution %d", id)
	}
}

// TestPostponePSEDistributions_UpgradeTiming checks that the result does not depend on when the upgrade runs.
func TestPostponePSEDistributions_UpgradeTiming(t *testing.T) {
	for name, lastProcessedID := range map[string]uint64{
		"before October distribution":  6,
		"between October and November": 7,
		"after November distribution":  8,
	} {
		t.Run(name, func(t *testing.T) {
			testApp, ctx, original := setupPSESchedule(t, lastProcessedID)
			require.NoError(t, v8.PostponePSEDistributions(ctx, testApp.PSEKeeper, v8.PSEPostponeCutoff))
			requirePostponed(t, testApp, ctx, original)
		})
	}
}

// TestPostponePSEDistributions_OngoingDistribution checks that a distribution being processed is left untouched.
// Its timestamp splits delegator scores, so it must finish with the date it started with.
func TestPostponePSEDistributions_OngoingDistribution(t *testing.T) {
	requireT := require.New(t)

	testApp, ctx, original := setupPSESchedule(t, 6)
	ongoing := original[6]
	ongoing.StartedAt = int64(ongoing.Timestamp)
	requireT.NoError(testApp.PSEKeeper.AllocationSchedule.Set(ctx, ongoing.ID, ongoing))
	requireT.NoError(testApp.PSEKeeper.OngoingDistribution.Set(ctx, ongoing))

	// Make the ongoing distribution fall after the cutoff, so skipping it is what keeps its date.
	cutoff := time.Unix(int64(ongoing.Timestamp), 0).Add(-time.Hour)
	requireT.NoError(v8.PostponePSEDistributions(ctx, testApp.PSEKeeper, cutoff))

	stored, err := testApp.PSEKeeper.AllocationSchedule.Get(ctx, ongoing.ID)
	requireT.NoError(err)
	requireT.Equal(ongoing, stored)

	next, err := testApp.PSEKeeper.AllocationSchedule.Get(ctx, ongoing.ID+1)
	requireT.NoError(err)
	requireT.Equal(
		uint64(time.Unix(int64(original[7].Timestamp), 0).UTC().AddDate(1, 0, 0).Unix()),
		next.Timestamp,
	)
}

// TestPostponePSEDistributions_NothingToPostpone checks that the step is a no-op once the schedule is past the cutoff.
func TestPostponePSEDistributions_NothingToPostpone(t *testing.T) {
	requireT := require.New(t)

	testApp, ctx, original := setupPSESchedule(t, mainnetPSEDistributionCount)
	requireT.NoError(v8.PostponePSEDistributions(ctx, testApp.PSEKeeper, v8.PSEPostponeCutoff))

	stored, err := testApp.PSEKeeper.GetDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.Equal(original, stored)
}

// TestPostponePSEDistributions_InvalidResultWritesNothing checks that a rejected schedule leaves the store unchanged.
func TestPostponePSEDistributions_InvalidResultWritesNothing(t *testing.T) {
	requireT := require.New(t)

	testApp, ctx, original := setupPSESchedule(t, 6)
	params, err := testApp.PSEKeeper.GetParams(ctx)
	requireT.NoError(err)
	// Monthly distributions are about 30 days apart, so a 40-day minimum gap rejects the schedule.
	params.MinDistributionGapSeconds = uint64((40 * 24 * time.Hour).Seconds())
	requireT.NoError(testApp.PSEKeeper.SetParams(ctx, params))

	requireT.Error(v8.PostponePSEDistributions(ctx, testApp.PSEKeeper, v8.PSEPostponeCutoff))

	stored, err := testApp.PSEKeeper.GetDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.Equal(original, stored)
}
