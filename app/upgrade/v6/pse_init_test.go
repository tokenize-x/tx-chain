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

	// Step 6: Verify first and last timestamps (schedule uses calendar months)
	// The distribution should start at 12:00:00 GMT on the same day as the upgrade (capped at 27)
	upgradeBlockTime := ctx.BlockTime()
	distributionDay := upgradeBlockTime.Day()
	if distributionDay > v6.MaxDistributionDay {
		distributionDay = v6.MaxDistributionDay
	}
	expectedStartTime := uint64(time.Date(
		upgradeBlockTime.Year(),
		upgradeBlockTime.Month(),
		distributionDay,
		12, 0, 0, 0,
		time.UTC,
	).Unix())
	requireT.Equal(expectedStartTime, allocationSchedule[0].Timestamp,
		"first period should start at 12:00:00 GMT on upgrade day (capped at day 28)")
	requireT.Greater(allocationSchedule[v6.TotalAllocationMonths-1].Timestamp, expectedStartTime,
		"last period should be after start time")

	// Step 6b: Verify each distribution happens on the same day every month at 12:00:00 GMT
	// Start from noon GMT on the upgrade day (capped at 28) - reuse distributionDay from Step 6
	startTime := time.Date(
		upgradeBlockTime.Year(),
		upgradeBlockTime.Month(),
		distributionDay,
		12, 0, 0, 0,
		time.UTC,
	)
	for i, period := range allocationSchedule {
		currentTime := time.Unix(int64(period.Timestamp), 0).UTC()

		// Verify each period is on the same day of the month
		// All distributions should be at 12:00:00 GMT on the same day every month
		expectedTime := startTime.AddDate(0, i, 0)
		requireT.Equal(expectedTime.Unix(), currentTime.Unix(),
			"period %d should be %d months after upgrade date at 12:00:00 GMT on day %d", i, i, distributionDay)
		requireT.Equal(distributionDay, currentTime.Day(), "period %d should be on day %d", i, distributionDay)
		requireT.Equal(12, currentTime.Hour(), "period %d should be at hour 12", i)
		requireT.Equal(0, currentTime.Minute(), "period %d should be at minute 00", i)
		requireT.Equal(0, currentTime.Second(), "period %d should be at second 00", i)
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
			startTime: uint64(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions,
				schedule []types.ScheduledDistribution, allocations []v6.InitialFundAllocation,
				totalMint sdkmath.Int,
			) {
				// Verify second period is exactly 1 calendar month after start
				startDate := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
				expected1MonthLater := uint64(startDate.AddDate(0, 1, 0).Unix())
				req.Equal(expected1MonthLater, schedule[1].Timestamp, "second period should be 1 month after start (Feb 1)")
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
			startTime: uint64(time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC).Unix()),
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
			startTime: uint64(time.Date(2025, 12, 1, 12, 0, 0, 0, time.UTC).Unix()),
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

			// Verify: Last period is exactly 83 calendar months after start
			startDateTime := time.Unix(int64(tc.startTime), 0).UTC()
			distributionDay := startDateTime.Day()
			if distributionDay > v6.MaxDistributionDay {
				distributionDay = v6.MaxDistributionDay
			}
			baseTime := time.Date(
				startDateTime.Year(),
				startDateTime.Month(),
				distributionDay,
				startDateTime.Hour(),
				startDateTime.Minute(),
				startDateTime.Second(),
				startDateTime.Nanosecond(),
				time.UTC,
			)
			expectedLast := uint64(baseTime.AddDate(0, 83, 0).Unix())
			requireT.Equal(expectedLast, schedule[83].Timestamp,
				"last period should be exactly 83 months after start")

			// Run test-specific verifications
			if tc.verifyFn != nil {
				tc.verifyFn(requireT, schedule, tc.allocations, tc.totalMint)
			}
		})
	}
}

func TestCreateDistributionSchedule_DateHandling(t *testing.T) {
	// This test verifies the schedule creation function handles calendar month arithmetic correctly.
	// Basic day capping is tested in TestPseInit_DayCapping (integration level).
	testCases := []struct {
		name      string
		startTime time.Time
		verifyFn  func(*require.Assertions, []types.ScheduledDistribution, time.Time)
	}{
		{
			name:      "same_day_across_all_months",
			startTime: time.Date(2024, 1, 28, 12, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// Verify all 84 periods maintain day 28 consistently (max allowed day)
				for i, period := range schedule {
					actualTime := time.Unix(int64(period.Timestamp), 0).UTC()
					req.Equal(28, actualTime.Day(), "period %d should be on day 28", i)
					req.Equal(12, actualTime.Hour(), "period %d should be at 12:00", i)
				}
				// Verify month arithmetic is correct (12 months = 1 year)
				expected12MonthsLater := uint64(start.AddDate(0, 12, 0).Unix())
				req.Equal(expected12MonthsLater, schedule[12].Timestamp,
					"period 12 should be exactly 12 calendar months after start")
			},
		},
		{
			name:      "leap_year_feb_29_capped_and_consistent",
			startTime: time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// Feb 29 (leap year) should be capped to 28 for all periods
				for i, period := range schedule {
					actualTime := time.Unix(int64(period.Timestamp), 0).UTC()
					req.Equal(28, actualTime.Day(), "period %d should be on day 28 (capped from leap day 29)", i)
				}
				// Verify works correctly across leap and non-leap Februaries
				expectedFeb2025 := time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC)
				actualFeb2025 := time.Unix(int64(schedule[12].Timestamp), 0).UTC()
				req.Equal(expectedFeb2025.Unix(), actualFeb2025.Unix(),
					"February 2025 (non-leap) should be on the 28th")

				expectedFeb2026 := time.Date(2026, 2, 28, 12, 0, 0, 0, time.UTC)
				actualFeb2026 := time.Unix(int64(schedule[24].Timestamp), 0).UTC()
				req.Equal(expectedFeb2026.Unix(), actualFeb2026.Unix(),
					"February 2026 (non-leap) should be on the 28th")
			},
		},
		{
			name:      "year_boundary_crossing",
			startTime: time.Date(2024, 12, 31, 12, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// Verify year boundary is crossed correctly
				expectedJan28 := time.Date(2025, 1, 28, 12, 0, 0, 0, time.UTC)
				actualJan := time.Unix(int64(schedule[1].Timestamp), 0).UTC()
				req.Equal(expectedJan28.Unix(), actualJan.Unix(),
					"January 2025 should be on the 28th (crosses year boundary)")
				req.Equal(2025, actualJan.Year(), "should cross into year 2025")
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

			// Verify: All timestamps follow calendar month intervals on the same day
			startDay := tc.startTime.Day()
			if startDay > v6.MaxDistributionDay {
				startDay = v6.MaxDistributionDay
			}
			baseTime := time.Date(
				tc.startTime.Year(),
				tc.startTime.Month(),
				startDay,
				tc.startTime.Hour(),
				tc.startTime.Minute(),
				tc.startTime.Second(),
				tc.startTime.Nanosecond(),
				time.UTC,
			)
			for i, period := range schedule {
				expectedTime := baseTime.AddDate(0, i, 0)
				requireT.Equal(uint64(expectedTime.Unix()), period.Timestamp,
					"period %d should be exactly %d months after start on day %d", i, i, startDay)
			}

			// Run test-specific verifications
			if tc.verifyFn != nil {
				tc.verifyFn(requireT, schedule, tc.startTime)
			}
		})
	}
}

func TestPseInit_DayCapping(t *testing.T) {
	// This test verifies the upgrade handler correctly applies day capping.
	// Detailed date edge cases are tested in TestCreateDistributionSchedule_DateHandling.
	testCases := []struct {
		name        string
		upgradeDay  int
		expectedDay int
	}{
		{
			name:        "day_28_not_capped",
			upgradeDay:  28,
			expectedDay: 28,
		},
		{
			name:        "day_29_capped_to_28",
			upgradeDay:  29,
			expectedDay: 28,
		},
		{
			name:        "day_30_capped_to_28",
			upgradeDay:  30,
			expectedDay: 28,
		},
		{
			name:        "day_31_capped_to_28",
			upgradeDay:  31,
			expectedDay: 28,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			testApp := simapp.New()
			// Create a context with the specific day for upgrade
			upgradeTime := time.Date(2024, 1, tc.upgradeDay, 15, 30, 45, 0, time.UTC)
			ctx := testApp.NewContext(false).
				WithChainID(string(constant.ChainIDDev)).
				WithBlockTime(upgradeTime)
			pseKeeper := testApp.PSEKeeper
			bankKeeper := testApp.BankKeeper

			// Perform Initialization
			err := v6.InitPSEAllocationsAndSchedule(ctx, pseKeeper, bankKeeper, stakingkeeper.NewQuerier(testApp.StakingKeeper))
			requireT.NoError(err)

			// Get allocation schedule
			allocationSchedule, err := pseKeeper.GetDistributionSchedule(ctx)
			requireT.NoError(err)
			requireT.Len(allocationSchedule, v6.TotalAllocationMonths)

			// Verify all periods use the expected day (capped if needed)
			for i, period := range allocationSchedule {
				actualTime := time.Unix(int64(period.Timestamp), 0).UTC()
				requireT.Equal(tc.expectedDay, actualTime.Day(),
					"period %d should be on day %d (upgrade was on day %d)", i, tc.expectedDay, tc.upgradeDay)
				requireT.Equal(12, actualTime.Hour(),
					"period %d should be at 12:00 GMT", i)
				requireT.Equal(0, actualTime.Minute(),
					"period %d should have 0 minutes", i)
				requireT.Equal(0, actualTime.Second(),
					"period %d should have 0 seconds", i)
			}

			// Verify first period specifically
			firstPeriod := time.Unix(int64(allocationSchedule[0].Timestamp), 0).UTC()
			requireT.Equal(upgradeTime.Year(), firstPeriod.Year())
			requireT.Equal(upgradeTime.Month(), firstPeriod.Month())
			requireT.Equal(tc.expectedDay, firstPeriod.Day())
			requireT.Equal(12, firstPeriod.Hour(), "first period should start at 12:00 GMT")
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

	startTime := uint64(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC).Unix())

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
