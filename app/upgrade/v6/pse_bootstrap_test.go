package v6_test

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

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// getAllocationSchedule returns the allocation schedule as a sorted list
func getAllocationSchedule(ctx context.Context, pseKeeper pskeeper.Keeper, requireT *require.Assertions) []psetypes.ScheduledDistribution {
	var schedule []psetypes.ScheduledDistribution
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

func TestBootstrap_DefaultAllocations(t *testing.T) {
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

	// Step 1: Set up sub-account mappings BEFORE bootstrap
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	multisigAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr5 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []psetypes.ClearingAccountMapping{
		{ClearingAccount: psetypes.ModuleAccountTreasury, RecipientAddress: multisigAddr1},
		{ClearingAccount: psetypes.ModuleAccountPartnership, RecipientAddress: multisigAddr2},
		{ClearingAccount: psetypes.ModuleAccountFoundingPartner, RecipientAddress: multisigAddr3},
		{ClearingAccount: psetypes.ModuleAccountTeam, RecipientAddress: multisigAddr4},
		{ClearingAccount: psetypes.ModuleAccountInvestors, RecipientAddress: multisigAddr5},
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, mappings)
	requireT.NoError(err)

	// Step 2: Perform Bootstrap (uses internal constants)
	err = v6.PerformBootstrap(ctx, pseKeeper, bankKeeper, testApp.StakingKeeper)
	requireT.NoError(err)

	// Step 3: Verify module accounts have correct balances
	allocations := v6.DefaultBootstrapAllocations()
	totalMintAmount := sdkmath.NewInt(v6.BootstrapTotalMint)

	totalVerified := sdkmath.ZeroInt()
	for _, allocation := range allocations {
		expectedAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		totalVerified = totalVerified.Add(expectedAmount)

		moduleAddr := testApp.AccountKeeper.GetModuleAddress(allocation.ModuleAccount)
		requireT.NotNil(moduleAddr)

		balance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
		requireT.Equal(expectedAmount.String(), balance.Amount.String(),
			"module %s should have correct balance", allocation.ModuleAccount)
	}

	// Step 4: Verify total minted equals sum of allocations (no rounding errors)
	requireT.Equal(totalMintAmount.String(), totalVerified.String(),
		"sum of allocations should equal total mint amount")

	// Step 5: Verify allocation schedule was created with n months
	allocationSchedule := getAllocationSchedule(ctx, pseKeeper, requireT)
	requireT.Len(allocationSchedule, psetypes.TotalAllocationMonths,
		"should have n monthly allocations")

	// Step 6: Verify first and last timestamps (schedule uses actual months, not fixed 30-day intervals)
	requireT.Equal(uint64(v6.DefaultDistributionStartTime), allocationSchedule[0].Timestamp,
		"first period should start at default distribution start time")
	requireT.Greater(allocationSchedule[83].Timestamp, uint64(v6.DefaultDistributionStartTime),
		"last period should be after start time")

	// Step 7: Verify each period has allocations for all 5 modules
	for i, period := range allocationSchedule {
		requireT.Len(period.Allocations, len(allocations),
			"period %d should have allocations for all %d modules", i, len(allocations))

		// Verify each module's monthly amount
		for _, allocation := range period.Allocations {
			var expectedTotal sdkmath.Int
			for _, bootstrapAlloc := range allocations {
				if bootstrapAlloc.ModuleAccount == allocation.ClearingAccount {
					expectedTotal = bootstrapAlloc.Percentage.MulInt(totalMintAmount).TruncateInt()
					break
				}
			}
			expectedMonthly := expectedTotal.QuoRaw(psetypes.TotalAllocationMonths)
			requireT.Equal(expectedMonthly.String(), allocation.Amount.String(),
				"period %d: monthly amount for %s should be 1/n of total", i, allocation.ClearingAccount)
		}
	}

	// Step 8: Verify all n months are in the schedule
	requireT.Len(allocationSchedule, psetypes.TotalAllocationMonths,
		"all n months should be in the schedule")
}

func TestCreateDistributionSchedule_MatchesBootstrapAllocations(t *testing.T) {
	requireT := require.New(t)

	// This test verifies that CreateDistributionSchedule produces
	// a schedule that matches the percentages from DefaultBootstrapAllocations

	totalMint := sdkmath.NewInt(v6.BootstrapTotalMint)
	allocations := v6.DefaultBootstrapAllocations()

	// Build the same balances map that the upgrade migration would use
	balances := make(map[string]sdkmath.Int)
	for _, allocation := range allocations {
		amount := allocation.Percentage.MulInt(totalMint).TruncateInt()
		balances[allocation.ModuleAccount] = amount
	}

	// Create the schedule
	schedule, err := pskeeper.CreateDistributionSchedule(
		balances,
		v6.DefaultDistributionStartTime,
	)
	requireT.NoError(err)

	// Verify: Each period has the correct monthly amounts
	for i, period := range schedule {
		requireT.Len(period.Allocations, len(allocations),
			"period %d should have allocations for all modules", i)

		for _, periodAlloc := range period.Allocations {
			// Find the corresponding bootstrap allocation
			totalBalance := balances[periodAlloc.ClearingAccount]
			expectedMonthly := totalBalance.QuoRaw(psetypes.TotalAllocationMonths)

			requireT.Equal(expectedMonthly.String(), periodAlloc.Amount.String(),
				"period %d: %s monthly amount should be 1/n of total",
				i, periodAlloc.ClearingAccount)
		}
	}
}
