package v6_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cometbft/cometbft/crypto/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/stretchr/testify/require"

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestPseInit_DefaultAllocations(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).
		WithChainID(string(constant.ChainIDDev)).
		WithBlockTime(time.Now())
	pseKeeper := testApp.PSEKeeper
	bankKeeper := testApp.BankKeeper

	// Get bond denom
	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	// Get total supply before initialization
	supplyBefore := bankKeeper.GetSupply(ctx, bondDenom)

	// Step 1: Perform Initialization (uses internal constants)
	// Note: InitPSEAllocationsAndSchedule will create clearing account mappings with placeholder addresses
	err = v6.InitPSEAllocationsAndSchedule(ctx, pseKeeper, bankKeeper, stakingkeeper.NewQuerier(testApp.StakingKeeper))
	requireT.NoError(err)

	// Get total supply after initialization
	supplyAfter := bankKeeper.GetSupply(ctx, bondDenom)

	// Calculate actual mint amount from supply diff
	totalActualMint := supplyAfter.Amount.Sub(supplyBefore.Amount)

	// Step 2: Verify clearing account mappings were created
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	requireT.Len(params.ClearingAccountMappings, 5, "should have mappings for 5 non-excluded clearing accounts")
	// Verify all mappings have recipient addresses (placeholder in production, valid test addresses in tests)
	for _, mapping := range params.ClearingAccountMappings {
		requireT.NotEmpty(mapping.RecipientAddresses,
			"mapping for %s should have recipient addresses", mapping.ClearingAccount)
		// Verify Community is not in mappings
		requireT.NotEqual(types.ClearingAccountCommunity, mapping.ClearingAccount,
			"Community should not have a mapping")
	}

	// Step 3: Verify module accounts have correct balances
	allocations := v6.DefaultInitialFundAllocations()
	totalMintAmount := sdkmath.NewInt(v6.InitialTotalMint)

	totalVerified := sdkmath.ZeroInt()
	for _, allocation := range allocations {
		expectedAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		totalVerified = totalVerified.Add(expectedAmount)

		moduleAddr := testApp.AccountKeeper.GetModuleAddress(allocation.ClearingAccount)
		requireT.NotNil(moduleAddr)

		balance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
		requireT.Equal(expectedAmount.String(), balance.Amount.String(),
			"module %s should have correct balance", allocation.ClearingAccount)
	}

	// Step 4: Verify total actually minted (from supply diff) equals expected amount
	requireT.Equal(totalMintAmount.String(), totalActualMint.String(),
		"actual minted amount (from supply diff) should equal total mint amount")

	// Step 4b: Verify sum of allocations equals total mint amount (no rounding errors)
	requireT.Equal(totalMintAmount.String(), totalVerified.String(),
		"sum of allocations should equal total mint amount")

	// Step 5: Verify allocation schedule was created with n periods
	allocationSchedule, err := pseKeeper.GetDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.Len(allocationSchedule, v6.TotalAllocationMonths,
		"should have n distribution periods")

	// Step 6: Verify first and last timestamps (schedule uses fixed 30-day intervals)
	// The distribution should start at 00:00:00 UTC on the same day as the upgrade
	upgradeBlockTime := ctx.BlockTime()
	expectedStartTime := uint64(time.Date(
		upgradeBlockTime.Year(),
		upgradeBlockTime.Month(),
		upgradeBlockTime.Day(),
		0, 0, 0, 0,
		time.UTC,
	).Unix())
	requireT.Equal(expectedStartTime, allocationSchedule[0].Timestamp,
		"first period should start at 00:00:00 UTC on upgrade day")
	requireT.Greater(allocationSchedule[v6.TotalAllocationMonths-1].Timestamp, expectedStartTime,
		"last period should be after start time")

	// Step 6b: Verify each distribution happens exactly 30 days apart at 00:00:00 UTC
	upgradeTime := ctx.BlockTime()
	// Start from midnight UTC on the upgrade day
	startTime := time.Date(
		upgradeTime.Year(),
		upgradeTime.Month(),
		upgradeTime.Day(),
		0, 0, 0, 0,
		time.UTC,
	)
	var prevTime time.Time
	for i, period := range allocationSchedule {
		currentTime := time.Unix(int64(period.Timestamp), 0).UTC()

		// Verify each period is exactly 30 days from the start
		// All distributions should be at 00:00:00 UTC
		expectedTime := startTime.AddDate(0, 0, i*30)
		requireT.Equal(expectedTime.Unix(), currentTime.Unix(),
			"period %d should be exactly %d days after upgrade date at 00:00:00 UTC", i, i*30)
		requireT.Equal(0, currentTime.Hour(), "period %d should be at hour 00", i)
		requireT.Equal(0, currentTime.Minute(), "period %d should be at minute 00", i)
		requireT.Equal(0, currentTime.Second(), "period %d should be at second 00", i)

		// Verify each period is exactly 30 days from the previous
		if i > 0 {
			daysDiff := currentTime.Sub(prevTime).Hours() / 24
			requireT.Equal(float64(30), daysDiff,
				"period %d should be exactly 30 days after period %d", i, i-1)
		}

		prevTime = currentTime
	}

	// Step 7: Verify each period has allocations for all PSE module accounts
	for i, period := range allocationSchedule {
		requireT.Len(period.Allocations, len(allocations),
			"period %d should have allocations for all %d modules", i, len(allocations))

		// Verify each module's per-period amount
		for _, allocation := range period.Allocations {
			var expectedTotal sdkmath.Int
			for _, initialAlloc := range allocations {
				if initialAlloc.ClearingAccount == allocation.ClearingAccount {
					expectedTotal = initialAlloc.Percentage.MulInt(totalMintAmount).TruncateInt()
					break
				}
			}
			expectedPerPeriod := expectedTotal.QuoRaw(v6.TotalAllocationMonths)
			requireT.Equal(expectedPerPeriod.String(), allocation.Amount.String(),
				"period %d: amount for %s should be 1/n of total", i, allocation.ClearingAccount)
		}
	}

	// Step 8: Verify all n periods are in the schedule
	requireT.Len(allocationSchedule, v6.TotalAllocationMonths,
		"all n periods should be in the schedule")
}

func TestCreateDistributionSchedule_Success(t *testing.T) {
	testCases := []struct {
		name        string
		allocations []v6.InitialFundAllocation
		totalMint   sdkmath.Int
		startTime   uint64
		verifyFn    func(
			*require.Assertions,
			[]types.ScheduledDistribution,
			[]v6.InitialFundAllocation,
			sdkmath.Int,
		)
	}{
		{
			name: "standard_five_accounts",
			allocations: []v6.InitialFundAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.40")},  // 8.4M
				{ClearingAccount: types.ClearingAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.20")},        // 4.2M
				{ClearingAccount: types.ClearingAccountPartnership, Percentage: sdkmath.LegacyMustNewDecFromStr("0.12")}, // 2.52M
				{ClearingAccount: types.ClearingAccountAlliance, Percentage: sdkmath.LegacyMustNewDecFromStr("0.08")},    // 1.68M
				{ClearingAccount: types.ClearingAccountInvestors, Percentage: sdkmath.LegacyMustNewDecFromStr("0.06")},   // 1.26M
			},
			totalMint: sdkmath.NewInt(21_000_000), // 21M total
			startTime: uint64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions,
				schedule []types.ScheduledDistribution, allocations []v6.InitialFundAllocation,
				totalMint sdkmath.Int,
			) {
				// Verify second period is exactly 30 days after start
				startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
				expected30DaysLater := uint64(startDate.AddDate(0, 0, 30).Unix())
				req.Equal(expected30DaysLater, schedule[1].Timestamp, "second period should be 30 days after start")
			},
		},
		{
			name: "large_balances",
			allocations: []v6.InitialFundAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.353")},  // 30B
				{ClearingAccount: types.ClearingAccountPartnership, Percentage: sdkmath.LegacyMustNewDecFromStr("0.235")}, // 20B
				{ClearingAccount: types.ClearingAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.235")},        // 20B
				{ClearingAccount: types.ClearingAccountInvestors, Percentage: sdkmath.LegacyMustNewDecFromStr("0.177")},   // 15B
			},
			totalMint: sdkmath.NewInt(85_000_000_000_000_000), // 85B total
			startTime: uint64(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions,
				schedule []types.ScheduledDistribution, allocations []v6.InitialFundAllocation,
				totalMint sdkmath.Int,
			) {
				// Verify no overflow or precision issues with large numbers
				for _, period := range schedule {
					for _, allocation := range period.Allocations {
						req.True(allocation.Amount.IsPositive(), "amount should be positive")
						req.False(allocation.Amount.IsZero(), "amount should not be zero")
					}
				}
			},
		},
		{
			name: "includes_excluded_accounts",
			allocations: []v6.InitialFundAllocation{
				{ClearingAccount: types.ClearingAccountCommunity, Percentage: sdkmath.LegacyMustNewDecFromStr("0.40")},   // 40B
				{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.30")},  // 30B
				{ClearingAccount: types.ClearingAccountAlliance, Percentage: sdkmath.LegacyMustNewDecFromStr("0.20")},    // 20B
				{ClearingAccount: types.ClearingAccountPartnership, Percentage: sdkmath.LegacyMustNewDecFromStr("0.03")}, // 3B
				{ClearingAccount: types.ClearingAccountInvestors, Percentage: sdkmath.LegacyMustNewDecFromStr("0.05")},   // 5B
				{ClearingAccount: types.ClearingAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.02")},        // 2B
			},
			totalMint: sdkmath.NewInt(100_000_000_000_000_000), // 100B total
			startTime: uint64(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions,
				schedule []types.ScheduledDistribution, allocations []v6.InitialFundAllocation,
				totalMint sdkmath.Int,
			) {
				// Verify Community account is included in schedule
				foundCommunity := false
				for _, period := range schedule {
					for _, allocation := range period.Allocations {
						if allocation.ClearingAccount == types.ClearingAccountCommunity {
							foundCommunity = true
							// Verify Community has correct allocation amount
							communityTotal := allocations[0].Percentage.MulInt(totalMint).TruncateInt()
							expectedPerPeriod := communityTotal.QuoRaw(v6.TotalAllocationMonths)
							req.Equal(expectedPerPeriod.String(), allocation.Amount.String(),
								"Community per-period allocation should be correct")
						}
					}
				}
				req.True(foundCommunity, "Community account should be in schedule even though excluded from distribution")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			// Execute: Create schedule
			schedule, err := v6.CreateDistributionSchedule(tc.allocations, tc.totalMint, tc.startTime)
			requireT.NoError(err)

			// Verify: Should have n periods
			requireT.Len(schedule, v6.TotalAllocationMonths)

			// Verify: First period timestamp
			requireT.Equal(tc.startTime, schedule[0].Timestamp)

			// Verify: Each period has allocations for all modules
			for i, period := range schedule {
				requireT.Len(period.Allocations, len(tc.allocations),
					"period %d should have allocations for all modules", i)

				// Verify each allocation amount is 1/n of total
				for _, periodAlloc := range period.Allocations {
					// Find corresponding initial allocation
					var expectedTotal sdkmath.Int
					for _, alloc := range tc.allocations {
						if alloc.ClearingAccount == periodAlloc.ClearingAccount {
							expectedTotal = alloc.Percentage.MulInt(tc.totalMint).TruncateInt()
							break
						}
					}
					expectedPerPeriod := expectedTotal.QuoRaw(v6.TotalAllocationMonths)
					requireT.Equal(expectedPerPeriod.String(), periodAlloc.Amount.String(),
						"period %d: amount for %s should be 1/n of total", i, periodAlloc.ClearingAccount)
				}
			}

			// Verify: Last period is exactly (83 * 30) days after start
			startDateTime := time.Unix(int64(tc.startTime), 0).UTC()
			expectedLast := uint64(startDateTime.AddDate(0, 0, 83*30).Unix())
			requireT.Equal(expectedLast, schedule[83].Timestamp,
				"last period should be exactly 2490 days (83 * 30) after start")

			// Run test-specific verifications
			if tc.verifyFn != nil {
				tc.verifyFn(requireT, schedule, tc.allocations, tc.totalMint)
			}
		})
	}
}

func TestCreateDistributionSchedule_DateHandling(t *testing.T) {
	testCases := []struct {
		name      string
		startTime time.Time
		verifyFn  func(*require.Assertions, []types.ScheduledDistribution, time.Time)
	}{
		{
			name:      "exact_30_day_intervals",
			startTime: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// Period 12 should be exactly 360 days (12 * 30) after start
				expected360DaysLater := uint64(start.AddDate(0, 0, 360).Unix())
				req.Equal(expected360DaysLater, schedule[12].Timestamp,
					"period 12 should be exactly 360 days after start")
			},
		},
		{
			name:      "consistent_across_month_boundaries",
			startTime: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// With 30-day intervals, we don't have month overflow issues
				// Period 1 should be exactly 30 days after Jan 31
				expected30DaysLater := start.AddDate(0, 0, 30) // March 2, 2025
				actualSecondPeriod := time.Unix(int64(schedule[1].Timestamp), 0).UTC()
				req.Equal(expected30DaysLater.Unix(), actualSecondPeriod.Unix(),
					"30 days after Jan 31 should be March 2 (consistent 30-day intervals)")
			},
		},
	}

	allocations := []v6.InitialFundAllocation{
		{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("1.0")},
	}
	totalMint := sdkmath.NewInt(8_400_000)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			// Execute: Create schedule
			schedule, err := v6.CreateDistributionSchedule(allocations, totalMint, uint64(tc.startTime.Unix()))
			requireT.NoError(err)

			// Verify: All timestamps follow exact 30-day intervals
			for i, period := range schedule {
				expectedTime := tc.startTime.AddDate(0, 0, i*30)
				requireT.Equal(uint64(expectedTime.Unix()), period.Timestamp,
					"period %d should be exactly %d days after start", i, i*30)
			}

			// Run test-specific verifications
			if tc.verifyFn != nil {
				tc.verifyFn(requireT, schedule, tc.startTime)
			}
		})
	}
}

// Temporary test to compare 30-day intervals with Gregorian calendar months
func TestCreateDistributionSchedule_SpecialDatesComparison(t *testing.T) {
	// This test demonstrates why 30-day intervals are superior to Gregorian calendar months
	// by showing edge cases that would be problematic with month-based scheduling
	testCases := []struct {
		name        string
		startDate   time.Time
		description string
		verifyFn    func(*require.Assertions, []types.ScheduledDistribution, time.Time)
	}{
		{
			name:        "start_on_31st_of_month",
			startDate:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			description: "If using Gregorian months, Jan 31 + 1 month = Feb 31 (invalid) would normalize to Mar 2/3",
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// With 30-day intervals: Jan 31 + 30 days = March 1 (predictable)
				expected30Days := start.AddDate(0, 0, 30)
				actual := time.Unix(int64(schedule[1].Timestamp), 0).UTC()
				req.Equal(expected30Days, actual, "30-day interval gives consistent result")
				// TODO: Discuss with team - verify these specific dates are acceptable
				// req.Equal(time.March, actual.Month(), "should be March")
				// req.Equal(1, actual.Day(), "should be March 1st")

				// Show what would happen with Gregorian months (for comparison)
				gregorianResult := start.AddDate(0, 1, 0) // Jan 31 + 1 month
				req.NotEqual(gregorianResult, actual, "Gregorian month addition gives different result")
				// TODO: Discuss with team - Gregorian calendar behavior
				// req.Equal(time.March, gregorianResult.Month(), "Gregorian also gives March")
				// req.Equal(2, gregorianResult.Day(), "but Gregorian gives March 2nd (normalized from Feb 31)")
			},
		},
		{
			name:        "start_on_29th_non_leap_year",
			startDate:   time.Date(2025, 1, 29, 0, 0, 0, 0, time.UTC),
			description: "Jan 29 in non-leap year: Gregorian months would skip to March 1 (Feb has only 28 days)",
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// With 30-day intervals: Jan 29 + 30 days = Feb 28 (consistent)
				expected30Days := start.AddDate(0, 0, 30)
				actual := time.Unix(int64(schedule[1].Timestamp), 0).UTC()
				req.Equal(expected30Days, actual)
				// TODO: Discuss with team - verify these specific dates are acceptable
				// req.Equal(time.February, actual.Month(), "should be February")
				// req.Equal(28, actual.Day(), "should be Feb 28")

				// Show what would happen with Gregorian months
				gregorianResult := start.AddDate(0, 1, 0) // Jan 29 + 1 month
				req.NotEqual(gregorianResult, actual, "Gregorian gives different result")
				// TODO: Discuss with team - Gregorian calendar behavior
				// req.Equal(time.March, gregorianResult.Month(), "Gregorian gives March")
				// req.Equal(1, gregorianResult.Day(), "Gregorian normalizes to March 1st (Feb 29 invalid in 2025)")
			},
		},
		{
			name:        "start_on_30th_of_month",
			startDate:   time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC),
			description: "Jan 30 + months would normalize differently in February (28/29 days)",
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// With 30-day intervals: Always exactly 30 days
				for i := 1; i <= 12; i++ {
					expected := start.AddDate(0, 0, i*30)
					actual := time.Unix(int64(schedule[i].Timestamp), 0).UTC()
					req.Equal(expected, actual, "period %d is exactly %d days", i, i*30)
				}

				// Show Gregorian inconsistency: Jan 30 + 1 month = March 1 (2024 is leap year)
				// TODO: Discuss with team - Gregorian calendar behavior
				// gregorianResult := start.AddDate(0, 1, 0)
				// req.Equal(time.March, gregorianResult.Month(), "Gregorian: Jan 30 + 1 month = March 1 (Feb 30 invalid)")
				// req.Equal(1, gregorianResult.Day(), "Gregorian normalizes to March 1st")
			},
		},
		{
			name:        "leap_year_february_29",
			startDate:   time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC),
			description: "Starting on leap day: Gregorian months would cause issues in non-leap years",
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// With 30-day intervals: predictable every time
				expected30Days := start.AddDate(0, 0, 30)
				actual := time.Unix(int64(schedule[1].Timestamp), 0).UTC()
				req.Equal(expected30Days, actual)
				// TODO: Discuss with team - verify these specific dates for leap day start
				// req.Equal(time.March, actual.Month(), "should be March")
				// req.Equal(30, actual.Day(), "should be March 30")

				// Gregorian: Feb 29, 2024 + 12 months = Feb 29, 2025 (invalid, normalizes to March 1)
				// TODO: Discuss with team - Gregorian calendar leap year behavior
				// gregorian12Months := start.AddDate(0, 12, 0)
				// req.Equal(time.March, gregorian12Months.Month(), "Gregorian: +12 months from leap day lands in March")
				// req.Equal(1, gregorian12Months.Day(), "normalizes to March 1 (2025 is not leap year)")

				// With 30-day intervals: Feb 29, 2024 + (12 * 30 days) = Feb 23, 2025 (predictable)
				expected12Periods := start.AddDate(0, 0, 12*30)
				actual12 := time.Unix(int64(schedule[12].Timestamp), 0).UTC()
				req.Equal(expected12Periods, actual12)
				// TODO: Discuss with team - verify 30-day interval results for leap day
				// req.Equal(time.February, actual12.Month(), "30-day intervals: stays in February")
				// req.Equal(23, actual12.Day(), "Feb 23, 2025")
			},
		},
		{
			name:        "varying_month_lengths",
			startDate:   time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			description: "Demonstrates consistent intervals vs. variable Gregorian month lengths",
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// With 30-day intervals: every period is exactly 30 days
				var prevTime time.Time
				for i := 0; i < 12; i++ {
					currentTime := time.Unix(int64(schedule[i].Timestamp), 0).UTC()
					if i > 0 {
						daysDiff := currentTime.Sub(prevTime).Hours() / 24
						req.Equal(float64(30), daysDiff, "period %d is exactly 30 days from previous", i)
					}
					prevTime = currentTime
				}

				// Show Gregorian variability: months have 28-31 days
				gregorianDurations := []int{}
				prevGregorian := start
				for i := 1; i <= 12; i++ {
					nextGregorian := start.AddDate(0, i, 0)
					days := int(nextGregorian.Sub(prevGregorian).Hours() / 24)
					gregorianDurations = append(gregorianDurations, days)
					prevGregorian = nextGregorian
				}
				// Verify Gregorian months have different durations
				hasVariety := false
				firstDuration := gregorianDurations[0]
				for _, duration := range gregorianDurations {
					if duration != firstDuration {
						hasVariety = true
						break
					}
				}
				req.True(hasVariety, "Gregorian months have varying lengths: %v", gregorianDurations)
			},
		},
	}

	allocations := []v6.InitialFundAllocation{
		{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("1.0")},
	}
	totalMint := sdkmath.NewInt(8_400_000)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			// Execute: Create schedule with 30-day intervals
			schedule, err := v6.CreateDistributionSchedule(allocations, totalMint, uint64(tc.startDate.Unix()))
			requireT.NoError(err)
			requireT.Len(schedule, v6.TotalAllocationMonths)

			t.Logf("\n=== %s ===", tc.name)
			t.Logf("Description: %s", tc.description)
			t.Logf("Start Date: %s", tc.startDate.Format("2006-01-02 (Monday)"))
			t.Logf("\n30-Day Intervals (Current Implementation):")
			for i := 0; i < 5; i++ {
				schedTime := time.Unix(int64(schedule[i].Timestamp), 0).UTC()
				t.Logf("  Period %2d: %s", i, schedTime.Format("2006-01-02 (Monday)"))
			}
			t.Logf("\nGregorian Months (For Comparison):")
			for i := 0; i < 5; i++ {
				gregTime := tc.startDate.AddDate(0, i, 0)
				t.Logf("  Month  %2d: %s", i, gregTime.Format("2006-01-02 (Monday)"))
			}

			// Verify: All periods are exactly 30 days apart
			for i := 0; i < len(schedule); i++ {
				expected := tc.startDate.AddDate(0, 0, i*30)
				actual := time.Unix(int64(schedule[i].Timestamp), 0).UTC()
				requireT.Equal(expected.Unix(), actual.Unix(),
					"period %d should be exactly %d days after start", i, i*30)
			}

			// Run test-specific verifications
			if tc.verifyFn != nil {
				tc.verifyFn(requireT, schedule, tc.startDate)
			}
		})
	}
}

func TestCreateDistributionSchedule_EmptyBalances(t *testing.T) {
	requireT := require.New(t)

	startTime := uint64(time.Now().Unix())
	emptyAllocations := []v6.InitialFundAllocation{}
	totalMint := sdkmath.NewInt(1000)

	// Execute: Should fail with empty allocations
	schedule, err := v6.CreateDistributionSchedule(emptyAllocations, totalMint, startTime)
	requireT.Error(err)
	requireT.Nil(schedule)
	requireT.ErrorIs(err, types.ErrNoModuleBalances)
}

func TestCreateDistributionSchedule_ZeroBalance(t *testing.T) {
	requireT := require.New(t)

	startTime := uint64(time.Now().Unix())

	// Allocation that results in zero monthly amount (< TotalAllocationMonths)
	allocations := []v6.InitialFundAllocation{
		{
			ClearingAccount: types.ClearingAccountFoundation,
			Percentage:      sdkmath.LegacyMustNewDecFromStr("0.0000000001"), // Very small percentage
		},
	}
	totalMint := sdkmath.NewInt(50) // 50 * tiny percentage = 0 (integer division)

	// Execute: Should fail with zero per-period amount
	schedule, err := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
	requireT.Error(err)
	requireT.Nil(schedule)
	requireT.Contains(err.Error(), "balance too small to divide into distribution periods")
}

func TestCreateDistributionSchedule_Deterministic(t *testing.T) {
	requireT := require.New(t)

	// Setup
	allocations := []v6.InitialFundAllocation{
		{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.667")}, // ~8.4M
		{ClearingAccount: types.ClearingAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.333")},       // ~4.2M
	}
	totalMint := sdkmath.NewInt(12_600_000)

	startTime := uint64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix())

	// Execute twice
	schedule1, err1 := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
	requireT.NoError(err1)

	schedule2, err2 := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
	requireT.NoError(err2)

	// Verify: Results should be identical
	requireT.Len(schedule2, len(schedule1))

	for i := range schedule1 {
		requireT.Equal(schedule1[i].Timestamp, schedule2[i].Timestamp,
			"period %d timestamps should match", i)
		requireT.Len(schedule2[i].Allocations, len(schedule1[i].Allocations),
			"period %d should have same number of allocations", i)

		// Note: map iteration order is not guaranteed, so we need to match by clearing account
		allocs1 := make(map[string]sdkmath.Int)
		for _, a := range schedule1[i].Allocations {
			allocs1[a.ClearingAccount] = a.Amount
		}

		allocs2 := make(map[string]sdkmath.Int)
		for _, a := range schedule2[i].Allocations {
			allocs2[a.ClearingAccount] = a.Amount
		}

		requireT.Equal(allocs1, allocs2,
			"period %d allocations should match", i)
	}
}

func TestDistribution_DistributeAllocatedTokens(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).
		WithChainID(string(constant.ChainIDDev)).
		WithBlockTime(time.Now())
	pseKeeper := testApp.PSEKeeper
	bankKeeper := testApp.BankKeeper

	// Get bond denom
	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	// Set up sub-account mappings with both single and multiple recipients
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Must include all non-Community clearing accounts
	// Mix single and multiple recipients to test both scenarios
	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ClearingAccountFoundation, RecipientAddresses: []string{addr2, addr4}},      // 2 recipients
		{ClearingAccount: types.ClearingAccountAlliance, RecipientAddresses: []string{addr1, addr2, addr3}}, // 3 recipients
		{ClearingAccount: types.ClearingAccountPartnership, RecipientAddresses: []string{addr1}},            // 1 recipient
		{ClearingAccount: types.ClearingAccountInvestors, RecipientAddresses: []string{addr1}},              // 1 recipient
		{ClearingAccount: types.ClearingAccountTeam, RecipientAddresses: []string{addr3}},                   // 1 recipient
	}

	err = pseKeeper.UpdateClearingMappings(ctx, authority, mappings)
	requireT.NoError(err)

	// Create a distribution schedule manually for testing
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix()) // 1 hour ago (already due)

	// Create allocations for ALL clearing accounts to ensure they're properly funded
	totalMint := sdkmath.NewInt(200_000_000_000) // 200B total
	allocations := []v6.InitialFundAllocation{
		{ClearingAccount: types.ClearingAccountCommunity, Percentage: sdkmath.LegacyMustNewDecFromStr("0.40")},   // 80B
		{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.20")},  // 40B
		{ClearingAccount: types.ClearingAccountAlliance, Percentage: sdkmath.LegacyMustNewDecFromStr("0.15")},    // 30B
		{ClearingAccount: types.ClearingAccountPartnership, Percentage: sdkmath.LegacyMustNewDecFromStr("0.10")}, // 20B
		{ClearingAccount: types.ClearingAccountInvestors, Percentage: sdkmath.LegacyMustNewDecFromStr("0.10")},   // 20B
		{ClearingAccount: types.ClearingAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.05")},        // 10B
	}

	// Mint and fund clearing accounts for distribution
	err = v6.MintAndFundClearingAccounts(ctx, bankKeeper, allocations, totalMint, bondDenom)
	requireT.NoError(err)

	// Create schedule
	schedule, err := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
	requireT.NoError(err)

	// Save only the first distribution (for testing)
	firstDist := schedule[0]
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{firstDist})
	requireT.NoError(err)

	// Verify schedule was saved
	allocationSchedule, err := pseKeeper.GetDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.Len(allocationSchedule, 1, "should have 1 allocation")

	// Fast-forward time to first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(startTime)+10, 0)) // 10 seconds after first distribution time
	ctx = ctx.WithBlockHeight(100)

	// Process distributions
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// Verify non-Community clearing accounts have distributed the scheduled amount
	// (not empty because schedule spreads over TotalAllocationMonths periods)
	for _, allocation := range firstDist.Allocations {
		if allocation.ClearingAccount == types.ClearingAccountCommunity {
			continue
		}
		moduleAddr := testApp.AccountKeeper.GetModuleAddress(allocation.ClearingAccount)
		moduleBalance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)

		// Balance should have decreased by the allocated amount
		var initialBalance sdkmath.Int
		for _, alloc := range allocations {
			if alloc.ClearingAccount == allocation.ClearingAccount {
				initialBalance = alloc.Percentage.MulInt(totalMint).TruncateInt()
				break
			}
		}
		expectedRemaining := initialBalance.Sub(allocation.Amount)
		requireT.Equal(expectedRemaining.String(), moduleBalance.Amount.String(),
			"clearing account %s should have correct remaining balance", allocation.ClearingAccount)
	}

	// Verify recipient balances (accounting for multiple sources)
	// Build expected balances for each recipient and track total remainders
	expectedBalances := make(map[string]sdkmath.Int)
	totalRemainder := sdkmath.ZeroInt()
	communityAllocationAmount := sdkmath.ZeroInt()

	for _, allocation := range firstDist.Allocations {
		if allocation.ClearingAccount == types.ClearingAccountCommunity {
			totalRemainder = totalRemainder.Add(allocation.Amount)
			continue
		}

		var recipientAddrs []string
		for _, mapping := range mappings {
			if mapping.ClearingAccount == allocation.ClearingAccount {
				recipientAddrs = mapping.RecipientAddresses
				break
			}
		}

		numRecipients := sdkmath.NewInt(int64(len(recipientAddrs)))
		baseAmount := allocation.Amount.Quo(numRecipients)
		remainder := allocation.Amount.Mod(numRecipients)

		// Remainder goes to community pool, not to any recipient
		totalRemainder = totalRemainder.Add(remainder)

		// All recipients get equal base amount
		for _, recipientAddr := range recipientAddrs {
			if current, exists := expectedBalances[recipientAddr]; exists {
				expectedBalances[recipientAddr] = current.Add(baseAmount)
			} else {
				expectedBalances[recipientAddr] = baseAmount
			}
		}
	}

	// Verify actual balances match expected
	for addr, expectedAmount := range expectedBalances {
		recipient := sdk.MustAccAddressFromBech32(addr)
		actualBalance := bankKeeper.GetBalance(ctx, recipient, bondDenom)
		requireT.Equal(expectedAmount.String(), actualBalance.Amount.String(),
			"recipient %s should have received correct total amount", addr)
	}

	// Verify community pool received all remainders
	communityPoolCoins, err := testApp.DistrKeeper.FeePool.Get(ctx)
	requireT.NoError(err)
	communityPoolBalance := communityPoolCoins.CommunityPool.AmountOf(bondDenom)
	expectedCommunityPoolTotal := sdkmath.LegacyNewDecFromInt(totalRemainder.Add(communityAllocationAmount))
	requireT.Equal(expectedCommunityPoolTotal.String(), communityPoolBalance.String(),
		"community pool should have received all distribution remainders and Community leftover")

	// Verify allocation schedule count decreased (first period removed)
	allocationScheduleAfter, err := pseKeeper.GetDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.Empty(allocationScheduleAfter, "should have 0 remaining allocations")
}
