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

	// Step 5: Verify allocation schedule was created with n months
	allocationSchedule, err := pseKeeper.GetDistributionSchedule(ctx)
	requireT.NoError(err)
	requireT.Len(allocationSchedule, v6.TotalAllocationMonths,
		"should have n monthly allocations")

	// Step 6: Verify first and last timestamps (schedule uses actual months, not fixed 30-day intervals)
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

	// Step 6b: Verify each distribution happens on the same day of the month at 00:00:00 UTC
	// (or properly normalized for month-end dates)
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

		// Verify the schedule follows AddDate behavior (adds months while preserving day, or normalizing)
		// All distributions should be at 00:00:00 UTC
		expectedTime := startTime.AddDate(0, i, 0)
		requireT.Equal(expectedTime.Unix(), currentTime.Unix(),
			"period %d should be %d months after upgrade date at 00:00:00 UTC", i, i)
		requireT.Equal(0, currentTime.Hour(), "period %d should be at hour 00", i)
		requireT.Equal(0, currentTime.Minute(), "period %d should be at minute 00", i)
		requireT.Equal(0, currentTime.Second(), "period %d should be at second 00", i)

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
		requireT.Len(period.Allocations, len(allocations),
			"period %d should have allocations for all %d modules", i, len(allocations))

		// Verify each module's monthly amount
		for _, allocation := range period.Allocations {
			var expectedTotal sdkmath.Int
			for _, initialAlloc := range allocations {
				if initialAlloc.ClearingAccount == allocation.ClearingAccount {
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
				// Verify Feb 2025 is properly calculated
				expectedFeb2025 := uint64(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Unix())
				req.Equal(expectedFeb2025, schedule[1].Timestamp, "second period should be Feb 1, 2025")
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
						if alloc.ClearingAccount == periodAlloc.ClearingAccount {
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
		verifyFn  func(*require.Assertions, []types.ScheduledDistribution, time.Time)
	}{
		{
			name:      "leap_year_transition",
			startTime: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
				// Feb 2025 (month 12) should be Feb 1, 2025
				expectedFeb2025 := uint64(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Unix())
				req.Equal(expectedFeb2025, schedule[12].Timestamp,
					"12 months after Feb 1, 2024 should be Feb 1, 2025 (leap year handling)")
			},
		},
		{
			name:      "month_end_boundaries",
			startTime: time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, start time.Time) {
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
		{ClearingAccount: types.ClearingAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("1.0")},
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
