package v6_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cometbft/cometbft/crypto/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestPseInit_DefaultAllocations(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	ctx = ctx.WithBlockTime(time.Now())
	pseKeeper := testApp.PSEKeeper
	bankKeeper := testApp.BankKeeper

	// Get bond denom
	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr5 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Override default clearing account mappings with valid test addresses
	v6.DefaultClearingAccountMappings = func() []psetypes.ClearingAccountMapping {
		return []psetypes.ClearingAccountMapping{
			{ClearingAccount: psetypes.ModuleAccountFoundation, RecipientAddress: addr1},
			{ClearingAccount: psetypes.ModuleAccountAlliance, RecipientAddress: addr2},
			{ClearingAccount: psetypes.ModuleAccountPartnership, RecipientAddress: addr3},
			{ClearingAccount: psetypes.ModuleAccountInvestors, RecipientAddress: addr4},
			{ClearingAccount: psetypes.ModuleAccountTeam, RecipientAddress: addr5},
		}
	}

	// Get total supply before initialization
	supplyBefore := bankKeeper.GetSupply(ctx, bondDenom)

	// Step 1: Perform Initialization (uses internal constants)
	// Note: InitPSEAllocationsAndSchedule will create clearing account mappings with placeholder addresses
	err = v6.InitPSEAllocationsAndSchedule(ctx, pseKeeper, bankKeeper, testApp.StakingKeeper)
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
		requireT.NotEmpty(mapping.RecipientAddress,
			"mapping for %s should have a recipient address", mapping.ClearingAccount)
		// Verify Community is not in mappings
		requireT.NotEqual(psetypes.ModuleAccountCommunity, mapping.ClearingAccount,
			"Community should not have a mapping")
	}

	// Step 3: Verify module accounts have correct balances
	fundAllocations := v6.DefaultInitialFundAllocations()
	scheduleAllocations := v6.FilterFundAllocationsForDistribution(fundAllocations)
	totalMintAmount := sdkmath.NewInt(v6.InitialTotalMint)

	totalVerified := sdkmath.ZeroInt()
	for _, allocation := range fundAllocations {
		expectedAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		totalVerified = totalVerified.Add(expectedAmount)

		moduleAddr := testApp.AccountKeeper.GetModuleAddress(allocation.ModuleAccount)
		requireT.NotNil(moduleAddr)

		balance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
		requireT.Equal(expectedAmount.String(), balance.Amount.String(),
			"module %s should have correct balance", allocation.ModuleAccount)
	}

	// Step 4: Verify total actually minted (from supply diff) equals expected amount
	requireT.Equal(totalMintAmount.String(), totalActualMint.String(),
		"actual minted amount (from supply diff) should equal total mint amount")

	// Step 4b: Verify sum of allocations equals total mint amount (no rounding errors)
	requireT.Equal(totalMintAmount.String(), totalVerified.String(),
		"sum of allocations should equal total mint amount")

	// Step 5: Verify allocation schedule was created with n months
	allocationSchedule, err := pseKeeper.GetAllocationSchedule(ctx)
	requireT.NoError(err)
	requireT.Len(allocationSchedule, v6.TotalAllocationMonths,
		"should have n monthly allocations")

	// Step 6: Verify first and last timestamps (schedule uses actual months, not fixed 30-day intervals)
	requireT.Equal(uint64(v6.DefaultDistributionStartTime), allocationSchedule[0].Timestamp,
		"first period should start at default distribution start time")
	requireT.Greater(allocationSchedule[v6.TotalAllocationMonths-1].Timestamp, uint64(v6.DefaultDistributionStartTime),
		"last period should be after start time")

	// Step 6b: Verify each distribution is on the first day of month and months increment correctly
	var prevTime time.Time
	for i, period := range allocationSchedule {
		currentTime := time.Unix(int64(period.Timestamp), 0).UTC()

		// Verify it's the first day of the month
		requireT.Equal(1, currentTime.Day(),
			"period %d should be on the first day of the month, got day %d", i, currentTime.Day())

		// Verify month increases by exactly 1 from previous (accounting for year rollover)
		if i > 0 {
			expectedTime := prevTime.AddDate(0, 1, 0)
			requireT.Equal(expectedTime.Year(), currentTime.Year(),
				"period %d: year should be %d, got %d", i, expectedTime.Year(), currentTime.Year())
			requireT.Equal(expectedTime.Month(), currentTime.Month(),
				"period %d: month should be %s, got %s", i, expectedTime.Month(), currentTime.Month())
		}

		prevTime = currentTime
	}

	// Step 7: Verify each period has allocations for all PSE module accounts
	for i, period := range allocationSchedule {
		requireT.Len(period.Allocations, len(scheduleAllocations),
			"period %d should have allocations for all %d modules", i, len(scheduleAllocations))

		// Verify each module's monthly amount
		for _, allocation := range period.Allocations {
			var expectedTotal sdkmath.Int
			for _, initialAlloc := range scheduleAllocations {
				if initialAlloc.ModuleAccount == allocation.ClearingAccount {
					expectedTotal = initialAlloc.Percentage.MulInt(totalMintAmount).TruncateInt()
					break
				}
			}
			expectedMonthly := expectedTotal.QuoRaw(v6.TotalAllocationMonths)
			requireT.Equal(expectedMonthly.String(), allocation.Amount.String(),
				"period %d: monthly amount for %s should be 1/n of total", i, allocation.ClearingAccount)
		}
	}

	// Step 8: Verify all n months are in the schedule
	requireT.Len(allocationSchedule, v6.TotalAllocationMonths,
		"all n months should be in the schedule")
}

func TestCreateDistributionSchedule_Success(t *testing.T) {
	testCases := []struct {
		name        string
		allocations []v6.InitialFundAllocation
		totalMint   sdkmath.Int
		startTime   uint64
		verifyFn    func(*require.Assertions, []psetypes.ScheduledDistribution, []v6.InitialFundAllocation, sdkmath.Int)
	}{
		{
			name: "standard_five_accounts",
			allocations: []v6.InitialFundAllocation{
				{ModuleAccount: psetypes.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.40")},  // 8.4M
				{ModuleAccount: psetypes.ModuleAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.20")},        // 4.2M
				{ModuleAccount: psetypes.ModuleAccountPartnership, Percentage: sdkmath.LegacyMustNewDecFromStr("0.12")}, // 2.52M
				{ModuleAccount: psetypes.ModuleAccountAlliance, Percentage: sdkmath.LegacyMustNewDecFromStr("0.08")},    // 1.68M
				{ModuleAccount: psetypes.ModuleAccountInvestors, Percentage: sdkmath.LegacyMustNewDecFromStr("0.06")},   // 1.26M
			},
			totalMint: sdkmath.NewInt(21_000_000), // 21M total
			startTime: uint64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions, schedule []psetypes.ScheduledDistribution, allocations []v6.InitialFundAllocation, totalMint sdkmath.Int) {
				// Verify Feb 2025 is properly calculated
				expectedFeb2025 := uint64(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Unix())
				req.Equal(expectedFeb2025, schedule[1].Timestamp, "second period should be Feb 1, 2025")
			},
		},
		{
			name: "large_balances",
			allocations: []v6.InitialFundAllocation{
				{ModuleAccount: psetypes.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.353")},  // 30B
				{ModuleAccount: psetypes.ModuleAccountPartnership, Percentage: sdkmath.LegacyMustNewDecFromStr("0.235")}, // 20B
				{ModuleAccount: psetypes.ModuleAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.235")},        // 20B
				{ModuleAccount: psetypes.ModuleAccountInvestors, Percentage: sdkmath.LegacyMustNewDecFromStr("0.177")},   // 15B
			},
			totalMint: sdkmath.NewInt(85_000_000_000_000_000), // 85B total
			startTime: uint64(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions, schedule []psetypes.ScheduledDistribution, allocations []v6.InitialFundAllocation, totalMint sdkmath.Int) {
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
				{ModuleAccount: psetypes.ModuleAccountCommunity, Percentage: sdkmath.LegacyMustNewDecFromStr("0.40")},   // 40B
				{ModuleAccount: psetypes.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.30")},  // 30B
				{ModuleAccount: psetypes.ModuleAccountAlliance, Percentage: sdkmath.LegacyMustNewDecFromStr("0.20")},    // 20B
				{ModuleAccount: psetypes.ModuleAccountPartnership, Percentage: sdkmath.LegacyMustNewDecFromStr("0.03")}, // 3B
				{ModuleAccount: psetypes.ModuleAccountInvestors, Percentage: sdkmath.LegacyMustNewDecFromStr("0.05")},   // 5B
				{ModuleAccount: psetypes.ModuleAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.02")},        // 2B
			},
			totalMint: sdkmath.NewInt(100_000_000_000_000_000), // 100B total
			startTime: uint64(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions, schedule []psetypes.ScheduledDistribution, allocations []v6.InitialFundAllocation, totalMint sdkmath.Int) {
				// Verify Community account is included in schedule
				foundCommunity := false
				for _, period := range schedule {
					for _, allocation := range period.Allocations {
						if allocation.ClearingAccount == psetypes.ModuleAccountCommunity {
							foundCommunity = true
							// Verify Community has correct allocation amount
							communityTotal := allocations[0].Percentage.MulInt(totalMint).TruncateInt()
							expectedMonthly := communityTotal.QuoRaw(v6.TotalAllocationMonths)
							req.Equal(expectedMonthly.String(), allocation.Amount.String(),
								"Community monthly allocation should be correct")
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
						if alloc.ModuleAccount == periodAlloc.ClearingAccount {
							expectedTotal = alloc.Percentage.MulInt(tc.totalMint).TruncateInt()
							break
						}
					}
					expectedMonthly := expectedTotal.QuoRaw(v6.TotalAllocationMonths)
					requireT.Equal(expectedMonthly.String(), periodAlloc.Amount.String(),
						"period %d: monthly amount for %s should be 1/n of total", i, periodAlloc.ClearingAccount)
				}
			}

			// Verify: Last period is 83 months after start
			startDateTime := time.Unix(int64(tc.startTime), 0).UTC()
			expectedLast := uint64(startDateTime.AddDate(0, 83, 0).Unix())
			requireT.Equal(expectedLast, schedule[83].Timestamp,
				"last period should be 83 months after start using Gregorian calendar")

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
		verifyFn  func(*require.Assertions, []psetypes.ScheduledDistribution, time.Time)
	}{
		{
			name:      "leap_year_transition",
			startTime: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []psetypes.ScheduledDistribution, start time.Time) {
				// Feb 2025 (month 12) should be Feb 1, 2025
				expectedFeb2025 := uint64(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Unix())
				req.Equal(expectedFeb2025, schedule[12].Timestamp,
					"12 months after Feb 1, 2024 should be Feb 1, 2025 (leap year handling)")
			},
		},
		{
			name:      "month_end_boundaries",
			startTime: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []psetypes.ScheduledDistribution, start time.Time) {
				// Jan 31 + 1 month = Feb 31 (invalid) -> normalizes to Mar 3
				// This is Go's AddDate behavior for overflow dates
				expectedMar3 := time.Date(2025, 3, 3, 0, 0, 0, 0, time.UTC)
				actualSecondMonth := time.Unix(int64(schedule[1].Timestamp), 0).UTC()
				req.Equal(expectedMar3.Unix(), actualSecondMonth.Unix(),
					"Jan 31 + 1 month normalizes to Mar 3 (AddDate overflow normalization)")
			},
		},
	}

	allocations := []v6.InitialFundAllocation{
		{ModuleAccount: psetypes.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("1.0")},
	}
	totalMint := sdkmath.NewInt(8_400_000)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			// Execute: Create schedule
			schedule, err := v6.CreateDistributionSchedule(allocations, totalMint, uint64(tc.startTime.Unix()))
			requireT.NoError(err)

			// Verify: All timestamps follow Gregorian calendar rules
			for i, period := range schedule {
				expectedTime := tc.startTime.AddDate(0, i, 0)
				requireT.Equal(uint64(expectedTime.Unix()), period.Timestamp,
					"period %d should be %s", i, expectedTime.Format(time.RFC3339))
			}

			// Run test-specific verifications
			if tc.verifyFn != nil {
				tc.verifyFn(requireT, schedule, tc.startTime)
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
	requireT.ErrorIs(err, psetypes.ErrNoModuleBalances)
}

func TestCreateDistributionSchedule_ZeroBalance(t *testing.T) {
	requireT := require.New(t)

	startTime := uint64(time.Now().Unix())

	// Allocation that results in zero monthly amount (< TotalAllocationMonths)
	allocations := []v6.InitialFundAllocation{
		{ModuleAccount: psetypes.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.0000000001")}, // Very small percentage
	}
	totalMint := sdkmath.NewInt(50) // 50 * tiny percentage = 0 (integer division)

	// Execute: Should fail with zero monthly amount
	schedule, err := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
	requireT.Error(err)
	requireT.Nil(schedule)
	requireT.Contains(err.Error(), "balance too small to divide into monthly distributions")
}

func TestCreateDistributionSchedule_Deterministic(t *testing.T) {
	requireT := require.New(t)

	// Setup
	allocations := []v6.InitialFundAllocation{
		{ModuleAccount: psetypes.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.667")}, // ~8.4M
		{ModuleAccount: psetypes.ModuleAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.333")},       // ~4.2M
	}
	totalMint := sdkmath.NewInt(12_600_000)

	startTime := uint64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix())

	// Execute twice
	schedule1, err1 := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
	requireT.NoError(err1)

	schedule2, err2 := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
	requireT.NoError(err2)

	// Verify: Results should be identical
	requireT.Equal(len(schedule1), len(schedule2))

	for i := range schedule1 {
		requireT.Equal(schedule1[i].Timestamp, schedule2[i].Timestamp,
			"period %d timestamps should match", i)
		requireT.Equal(len(schedule1[i].Allocations), len(schedule2[i].Allocations),
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
	ctx := testApp.NewContext(false)
	ctx = ctx.WithBlockTime(time.Now()) // Set proper block time
	pseKeeper := testApp.PSEKeeper
	bankKeeper := testApp.BankKeeper

	// Get bond denom
	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	// Step 1: Set up sub-account mappings
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Must include all eligible clearing accounts (Community is excluded)
	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: addr2},
		{ClearingAccount: types.ModuleAccountAlliance, RecipientAddress: addr1},
		{ClearingAccount: types.ModuleAccountPartnership, RecipientAddress: addr1},
		{ClearingAccount: types.ModuleAccountInvestors, RecipientAddress: addr1},
		{ClearingAccount: types.ModuleAccountTeam, RecipientAddress: addr3},
	}

	err = pseKeeper.UpdateClearingMappings(ctx, authority, mappings)
	requireT.NoError(err)

	// Step 2: Create a distribution schedule manually for testing
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix()) // 1 hour ago (already due)

	// Create allocations and calculate total mint amount
	totalMint := sdkmath.NewInt(200_000_000_000) // 200B total
	allocations := []v6.InitialFundAllocation{
		{ModuleAccount: types.ModuleAccountCommunity, Percentage: sdkmath.LegacyMustNewDecFromStr("0.50")},  // 100B
		{ModuleAccount: types.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.25")}, // 50B
		{ModuleAccount: types.ModuleAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.25")},       // 50B
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
	allocationSchedule, err := pseKeeper.GetAllocationSchedule(ctx)
	requireT.NoError(err)
	requireT.Len(allocationSchedule, 1, "should have 1 allocation")

	// Step 3: Fast-forward time to first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(startTime)+10, 0)) // 10 seconds after first distribution time
	ctx = ctx.WithBlockHeight(100)

	// Step 4: Process distributions
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// Step 5: Verify allocations transferred funds to recipient accounts (excluding Community)
	for _, allocation := range firstDist.Allocations {
		// Check if this is an excluded account
		if types.IsExcludedForAllocation(allocation.ClearingAccount) {
			// Excluded accounts should NOT transfer to recipients
			moduleAddr := testApp.AccountKeeper.GetModuleAddress(allocation.ClearingAccount)
			moduleBalance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
			requireT.False(moduleBalance.Amount.IsZero(),
				"excluded account %s should still have tokens", allocation.ClearingAccount)
			continue
		}

		// Find the recipient address for this allocation
		var recipientAddr string
		for _, mapping := range mappings {
			if mapping.ClearingAccount == allocation.ClearingAccount {
				recipientAddr = mapping.RecipientAddress
				break
			}
		}
		requireT.NotEmpty(recipientAddr, "should have recipient address for %s", allocation.ClearingAccount)

		recipient := sdk.MustAccAddressFromBech32(recipientAddr)
		recipientBalance := bankKeeper.GetBalance(ctx, recipient, bondDenom)

		// Non-excluded accounts should transfer to recipients
		requireT.Equal(allocation.Amount.String(), recipientBalance.Amount.String(),
			"recipient should have received allocation amount from %s", allocation.ClearingAccount)
	}

	// Step 6: Verify allocation schedule count decreased (first period removed)
	allocationScheduleAfter, err := pseKeeper.GetAllocationSchedule(ctx)
	requireT.NoError(err)
	requireT.Len(allocationScheduleAfter, 0, "should have 0 remaining allocations")
}
