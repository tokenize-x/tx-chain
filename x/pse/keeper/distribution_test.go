package keeper_test

import (
	"context"
	"sort"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// getAllocationSchedule returns the allocation schedule as a sorted list
func getAllocationSchedule(ctx context.Context, pseKeeper keeper.Keeper, requireT *require.Assertions) []types.ScheduledDistribution {
	var schedule []types.ScheduledDistribution
	iter, err := pseKeeper.AllocationSchedule.Iterate(ctx, nil)
	requireT.NoError(err)
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		requireT.NoError(err)
		schedule = append(schedule, kv.Value)
	}

	// Sort by timestamp
	sort.Slice(schedule, func(i, j int) bool {
		return schedule[i].Timestamp < schedule[j].Timestamp
	})

	return schedule
}

func TestDistribution_ProcessScheduledAllocations(t *testing.T) {
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
	multisigAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountCommunity, RecipientAddress: multisigAddr1},
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: multisigAddr2},
		{ClearingAccount: types.ModuleAccountTeam, RecipientAddress: multisigAddr3},
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, mappings)
	requireT.NoError(err)

	// Step 2: Create a distribution schedule manually for testing
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix()) // 1 hour ago (already due)

	// Create module balances including excluded Community account
	moduleBalances := map[string]sdkmath.Int{
		types.ModuleAccountCommunity:  sdkmath.NewInt(100_000_000_000), // 100B - excluded from distribution
		types.ModuleAccountFoundation: sdkmath.NewInt(50_000_000_000),  // 50B
		types.ModuleAccountTeam:       sdkmath.NewInt(50_000_000_000),  // 50B
	}

	// Mint tokens to module accounts for distribution
	for moduleAccount, amount := range moduleBalances {
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, amount))
		err = bankKeeper.MintCoins(ctx, types.ModuleName, coins)
		requireT.NoError(err)
		err = bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, moduleAccount, coins)
		requireT.NoError(err)
	}

	// Create schedule
	schedule, err := keeper.CreateDistributionSchedule(moduleBalances, startTime)
	requireT.NoError(err)

	// Save only the first distribution (for testing)
	firstDist := schedule[0]
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{firstDist})
	requireT.NoError(err)

	// Verify schedule was saved
	allocationSchedule := getAllocationSchedule(ctx, pseKeeper, requireT)
	requireT.Len(allocationSchedule, 1, "should have 1 allocation")

	// Step 3: Fast-forward time to first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(startTime)+10, 0)) // 10 seconds after first distribution time
	ctx = ctx.WithBlockHeight(100)

	// Step 4: Process distributions
	err = pseKeeper.ProcessClearingAccountDistributions(ctx)
	requireT.NoError(err)

	// Step 5: Verify allocations transferred funds to recipient accounts (excluding Community)
	for _, allocation := range firstDist.Allocations {
		// Find the recipient address for this allocation
		var recipientAddr string
		for _, mapping := range mappings {
			if mapping.ClearingAccount == allocation.ClearingAccount {
				recipientAddr = mapping.RecipientAddress
				break
			}
		}

		recipient := sdk.MustAccAddressFromBech32(recipientAddr)
		recipientBalance := bankKeeper.GetBalance(ctx, recipient, bondDenom)

		// Check if this is an excluded account
		if types.IsExcludedClearingAccount(allocation.ClearingAccount) {
			// Excluded accounts should NOT transfer to recipients
			requireT.True(recipientBalance.Amount.IsZero(),
				"excluded account %s recipient should have zero balance", allocation.ClearingAccount)

			// Verify tokens remain in the excluded module account
			moduleAddr := testApp.AccountKeeper.GetModuleAddress(allocation.ClearingAccount)
			moduleBalance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
			requireT.False(moduleBalance.Amount.IsZero(),
				"excluded account %s should still have tokens", allocation.ClearingAccount)
		} else {
			// Non-excluded accounts should transfer to recipients
			requireT.Equal(allocation.Amount.String(), recipientBalance.Amount.String(),
				"recipient should have received allocation amount from %s", allocation.ClearingAccount)
		}
	}

	// Step 6: Verify allocation schedule count decreased (first period removed)
	allocationScheduleAfter := getAllocationSchedule(ctx, pseKeeper, requireT)
	requireT.Len(allocationScheduleAfter, 0, "should have 0 remaining allocations")
}

func TestDistribution_GenesisRebuild(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	ctx = ctx.WithBlockTime(time.Now()) // Set proper block time
	pseKeeper := testApp.PSEKeeper

	// Get bond denom
	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	// Set up mappings and fund modules
	multisigAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: multisigAddr1},
		{ClearingAccount: types.ModuleAccountTeam, RecipientAddress: multisigAddr2},
	}

	for _, moduleAccount := range []string{types.ModuleAccountFoundation, types.ModuleAccountTeam} {
		fundAmount := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(5000)))
		err = testApp.BankKeeper.MintCoins(ctx, types.ModuleName, fundAmount)
		requireT.NoError(err)
		err = testApp.BankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, moduleAccount, fundAmount)
		requireT.NoError(err)
	}

	time1 := uint64(time.Now().Add(1 * time.Hour).Unix())
	time2 := uint64(time.Now().Add(2 * time.Hour).Unix())

	// Set up params with mappings
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.ClearingAccountMappings = mappings
	err = pseKeeper.SetParams(ctx, params)
	requireT.NoError(err)

	// Create and store allocation schedule
	schedule := []types.ScheduledDistribution{
		{
			Timestamp: time1,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
				{ClearingAccount: types.ModuleAccountTeam, Amount: sdkmath.NewInt(500)},
			},
		},
		{
			Timestamp: time2,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ModuleAccountFoundation, Amount: sdkmath.NewInt(2000)},
			},
		},
	}

	// Store in allocation schedule map
	for _, scheduledDist := range schedule {
		err = pseKeeper.AllocationSchedule.Set(ctx, scheduledDist.Timestamp, scheduledDist)
		requireT.NoError(err)
	}

	// Process first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(time1)+10, 0))
	ctx = ctx.WithBlockHeight(100)
	err = pseKeeper.ProcessClearingAccountDistributions(ctx)
	requireT.NoError(err)

	// Export genesis
	genesisState, err := pseKeeper.ExportGenesis(ctx)
	requireT.NoError(err)

	// Verify export contains:
	// - 2 allocations in schedule (time2 only, since time1 was processed and removed)
	requireT.Len(genesisState.ScheduledDistributions, 1, "should have 1 remaining allocation (time2)")
	requireT.Equal(time2, genesisState.ScheduledDistributions[0].Timestamp)

	// Create new app and import genesis
	testApp2 := simapp.New()
	ctx2 := testApp2.NewContext(false)
	ctx2 = ctx2.WithBlockTime(time.Unix(int64(time1)+10, 0)) // Set to same time as when we exported
	pseKeeper2 := testApp2.PSEKeeper

	// InitGenesis should restore allocation schedule from genesis state
	err = pseKeeper2.InitGenesis(ctx2, *genesisState)
	requireT.NoError(err)

	// Verify allocation schedule only contains time2 since time1 was already processed
	allocationSchedule2 := getAllocationSchedule(ctx2, pseKeeper2, requireT)
	requireT.Len(allocationSchedule2, 1, "should have 1 remaining allocation (time2)")
	requireT.Equal(time2, allocationSchedule2[0].Timestamp)
}

func TestCreateDistributionSchedule_Success(t *testing.T) {
	testCases := []struct {
		name                  string
		moduleAccountBalances map[string]sdkmath.Int
		startTime             uint64
		verifyFn              func(*require.Assertions, []types.ScheduledDistribution, map[string]sdkmath.Int)
	}{
		{
			name: "standard_five_accounts",
			moduleAccountBalances: map[string]sdkmath.Int{
				types.ModuleAccountFoundation:  sdkmath.NewInt(8_400_000), // 100K per month
				types.ModuleAccountTeam:        sdkmath.NewInt(4_200_000), // 50K per month
				types.ModuleAccountPartnership: sdkmath.NewInt(2_520_000), // 30K per month
				types.ModuleAccountAlliance:    sdkmath.NewInt(1_680_000), // 20K per month
				types.ModuleAccountInvestors:   sdkmath.NewInt(1_260_000), // 15K per month
			},
			startTime: uint64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, balances map[string]sdkmath.Int) {
				// Verify Feb 2025 is properly calculated
				expectedFeb2025 := uint64(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Unix())
				req.Equal(expectedFeb2025, schedule[1].Timestamp, "second period should be Feb 1, 2025")
			},
		},
		{
			name: "large_balances",
			moduleAccountBalances: map[string]sdkmath.Int{
				types.ModuleAccountFoundation:  sdkmath.NewInt(30_000_000_000_000_000), // 30B
				types.ModuleAccountPartnership: sdkmath.NewInt(20_000_000_000_000_000), // 20B
				types.ModuleAccountTeam:        sdkmath.NewInt(20_000_000_000_000_000), // 20B
				types.ModuleAccountInvestors:   sdkmath.NewInt(15_000_000_000_000_000), // 15B
			},
			startTime: uint64(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, balances map[string]sdkmath.Int) {
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
			moduleAccountBalances: map[string]sdkmath.Int{
				types.ModuleAccountCommunity:   sdkmath.NewInt(40_000_000_000_000_000), // 40B - excluded
				types.ModuleAccountFoundation:  sdkmath.NewInt(30_000_000_000_000_000), // 30B
				types.ModuleAccountAlliance:    sdkmath.NewInt(20_000_000_000_000_000), // 20B
				types.ModuleAccountPartnership: sdkmath.NewInt(3_000_000_000_000_000),  // 3B
				types.ModuleAccountInvestors:   sdkmath.NewInt(5_000_000_000_000_000),  // 5B
				types.ModuleAccountTeam:        sdkmath.NewInt(2_000_000_000_000_000),  // 2B
			},
			startTime: uint64(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).Unix()),
			verifyFn: func(req *require.Assertions, schedule []types.ScheduledDistribution, balances map[string]sdkmath.Int) {
				// Verify Community account is included in schedule
				foundCommunity := false
				for _, period := range schedule {
					for _, allocation := range period.Allocations {
						if allocation.ClearingAccount == types.ModuleAccountCommunity {
							foundCommunity = true
							// Verify Community has correct allocation amount
							expectedMonthly := balances[types.ModuleAccountCommunity].QuoRaw(types.TotalAllocationMonths)
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
			schedule, err := keeper.CreateDistributionSchedule(tc.moduleAccountBalances, tc.startTime)
			requireT.NoError(err)

			// Verify: Should have n periods
			requireT.Len(schedule, types.TotalAllocationMonths)

			// Verify: First period timestamp
			requireT.Equal(tc.startTime, schedule[0].Timestamp)

			// Verify: Each period has allocations for all modules
			for i, period := range schedule {
				requireT.Len(period.Allocations, len(tc.moduleAccountBalances),
					"period %d should have allocations for all modules", i)

				// Verify each allocation amount is 1/n of total
				for _, allocation := range period.Allocations {
					expectedTotal := tc.moduleAccountBalances[allocation.ClearingAccount]
					expectedMonthly := expectedTotal.QuoRaw(types.TotalAllocationMonths)
					requireT.Equal(expectedMonthly.String(), allocation.Amount.String(),
						"period %d: monthly amount for %s should be 1/n of total", i, allocation.ClearingAccount)
				}
			}

			// Verify: Last period is 83 months after start
			startDateTime := time.Unix(int64(tc.startTime), 0).UTC()
			expectedLast := uint64(startDateTime.AddDate(0, 83, 0).Unix())
			requireT.Equal(expectedLast, schedule[83].Timestamp,
				"last period should be 83 months after start using Gregorian calendar")

			// Run test-specific verifications
			if tc.verifyFn != nil {
				tc.verifyFn(requireT, schedule, tc.moduleAccountBalances)
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

	moduleAccountBalances := map[string]sdkmath.Int{
		types.ModuleAccountFoundation: sdkmath.NewInt(8_400_000),
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			// Execute: Create schedule
			schedule, err := keeper.CreateDistributionSchedule(moduleAccountBalances, uint64(tc.startTime.Unix()))
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
	emptyBalances := map[string]sdkmath.Int{}

	// Execute: Should fail with empty balances
	schedule, err := keeper.CreateDistributionSchedule(emptyBalances, startTime)
	requireT.Error(err)
	requireT.Nil(schedule)
	requireT.ErrorIs(err, types.ErrNoModuleBalances)
}

func TestCreateDistributionSchedule_ZeroBalance(t *testing.T) {
	requireT := require.New(t)

	startTime := uint64(time.Now().Unix())

	// Balance that results in zero monthly amount (< TotalAllocationMonths)
	moduleAccountBalances := map[string]sdkmath.Int{
		types.ModuleAccountFoundation: sdkmath.NewInt(50), // 50 / n = 0 (integer division)
	}

	// Execute: Should fail with zero monthly amount
	schedule, err := keeper.CreateDistributionSchedule(moduleAccountBalances, startTime)
	requireT.Error(err)
	requireT.Nil(schedule)
	requireT.Contains(err.Error(), "balance too small to divide into monthly distributions")
}

func TestCreateDistributionSchedule_Deterministic(t *testing.T) {
	requireT := require.New(t)

	// Setup
	moduleAccountBalances := map[string]sdkmath.Int{
		types.ModuleAccountFoundation: sdkmath.NewInt(8_400_000),
		types.ModuleAccountTeam:       sdkmath.NewInt(4_200_000),
	}

	startTime := uint64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Unix())

	// Execute twice
	schedule1, err1 := keeper.CreateDistributionSchedule(moduleAccountBalances, startTime)
	requireT.NoError(err1)

	schedule2, err2 := keeper.CreateDistributionSchedule(moduleAccountBalances, startTime)
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
