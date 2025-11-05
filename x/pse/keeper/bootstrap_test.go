package keeper_test

import (
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

	mappings := []types.SubAccountMapping{
		{ModuleAccount: types.ModuleAccountTreasury, SubAccountAddress: multisigAddr1},
		{ModuleAccount: types.ModuleAccountPartnership, SubAccountAddress: multisigAddr2},
		{ModuleAccount: types.ModuleAccountFoundingPartner, SubAccountAddress: multisigAddr3},
		{ModuleAccount: types.ModuleAccountTeam, SubAccountAddress: multisigAddr4},
		{ModuleAccount: types.ModuleAccountInvestors, SubAccountAddress: multisigAddr5},
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, mappings)
	requireT.NoError(err)

	// Step 2: Perform Bootstrap with default allocations
	totalMintAmount := sdkmath.NewInt(100_000_000_000)        // 100 billion
	startTime := uint64(time.Now().Add(1 * time.Hour).Unix()) // 1 hour from now

	err = pseKeeper.PerformBootstrap(ctx, totalMintAmount, bondDenom, nil, startTime)
	requireT.NoError(err)

	// Step 3: Verify module accounts have correct balances
	allocations := keeper.DefaultBootstrapAllocations()

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

	// Step 5: Verify distribution schedule was created with 84 months
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	requireT.Len(params.DistributionSchedule, keeper.TotalDistributionMonths,
		"should have 84 monthly distributions")

	// Step 6: Verify first and last timestamps (schedule uses actual months, not fixed 30-day intervals)
	requireT.Equal(startTime, params.DistributionSchedule[0].DistributionTime,
		"first period should start at startTime")
	requireT.Greater(params.DistributionSchedule[83].DistributionTime, startTime,
		"last period should be after start time")

	// Step 7: Verify each period has distributions for all 5 modules
	for i, period := range params.DistributionSchedule {
		requireT.Len(period.Distributions, len(allocations),
			"period %d should have distributions for all %d modules", i, len(allocations))

		// Verify each module's monthly amount
		for _, dist := range period.Distributions {
			var expectedTotal sdkmath.Int
			for _, alloc := range allocations {
				if alloc.ModuleAccount == dist.ModuleAccount {
					expectedTotal = alloc.Percentage.MulInt(totalMintAmount).TruncateInt()
					break
				}
			}
			expectedMonthly := expectedTotal.QuoRaw(keeper.TotalDistributionMonths)
			requireT.Equal(expectedMonthly.String(), dist.Amount.String(),
				"period %d: monthly amount for %s should be 1/84th of total", i, dist.ModuleAccount)
		}
	}

	// Step 8: Verify pending timestamps were added
	pendingInfo, err := pseKeeper.GetPendingDistributionsInfo(ctx)
	requireT.NoError(err)
	requireT.Len(pendingInfo, keeper.TotalDistributionMonths,
		"all 84 months should be pending")
}

func TestBootstrap_CustomAllocations(t *testing.T) {
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

	// Set up mappings for only 2 modules
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	multisigAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []types.SubAccountMapping{
		{ModuleAccount: types.ModuleAccountTreasury, SubAccountAddress: multisigAddr1},
		{ModuleAccount: types.ModuleAccountTeam, SubAccountAddress: multisigAddr2},
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, mappings)
	requireT.NoError(err)

	// Custom allocations: 70% treasury, 30% team
	customAllocations := []keeper.BootstrapAllocation{
		{
			ModuleAccount: types.ModuleAccountTreasury,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.7"),
		},
		{
			ModuleAccount: types.ModuleAccountTeam,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.3"),
		},
	}

	totalMintAmount := sdkmath.NewInt(1_000_000)              // 1 million
	startTime := uint64(time.Now().Add(1 * time.Hour).Unix()) // 1 hour from now

	err = pseKeeper.PerformBootstrap(ctx, totalMintAmount, bondDenom, customAllocations, startTime)
	requireT.NoError(err)

	// Verify custom allocations
	treasuryAddr := testApp.AccountKeeper.GetModuleAddress(types.ModuleAccountTreasury)
	treasuryBalance := bankKeeper.GetBalance(ctx, treasuryAddr, bondDenom)
	requireT.Equal("700000", treasuryBalance.Amount.String(), "treasury should have 70%")

	teamAddr := testApp.AccountKeeper.GetModuleAddress(types.ModuleAccountTeam)
	teamBalance := bankKeeper.GetBalance(ctx, teamAddr, bondDenom)
	requireT.Equal("300000", teamBalance.Amount.String(), "team should have 30%")

	// Verify schedule has only 2 modules per period
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	for i, period := range params.DistributionSchedule {
		requireT.Len(period.Distributions, 2,
			"period %d should have distributions for 2 modules", i)
	}
}

func TestBootstrap_ValidationErrors(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	ctx = ctx.WithBlockTime(time.Now()) // Set proper block time
	pseKeeper := testApp.PSEKeeper

	// Get bond denom
	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	// Set up mappings (needed for referential integrity)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	multisigAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []types.SubAccountMapping{
		{ModuleAccount: types.ModuleAccountTreasury, SubAccountAddress: multisigAddr1},
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, mappings)
	requireT.NoError(err)

	startTime := uint64(time.Now().Add(1 * time.Hour).Unix())

	testCases := []struct {
		name        string
		totalAmount sdkmath.Int
		allocations []keeper.BootstrapAllocation
		expectErr   bool
		errContains string
	}{
		{
			name:        "empty_allocations",
			totalAmount: sdkmath.NewInt(1000000),
			allocations: []keeper.BootstrapAllocation{},
			expectErr:   true,
			errContains: "no allocations provided",
		},
		{
			name:        "allocations_dont_sum_to_one",
			totalAmount: sdkmath.NewInt(1000000),
			allocations: []keeper.BootstrapAllocation{
				{
					ModuleAccount: types.ModuleAccountTreasury,
					Percentage:    sdkmath.LegacyMustNewDecFromStr("0.5"), // Only 50%, not 100%
				},
			},
			expectErr:   true,
			errContains: "total percentage must equal 1.0",
		},
		{
			name:        "two_allocations_sum_exceeds_one",
			totalAmount: sdkmath.NewInt(1000000),
			allocations: []keeper.BootstrapAllocation{
				{
					ModuleAccount: types.ModuleAccountTreasury,
					Percentage:    sdkmath.LegacyMustNewDecFromStr("0.6"),
				},
				{
					ModuleAccount: types.ModuleAccountTeam,
					Percentage:    sdkmath.LegacyMustNewDecFromStr("0.6"),
				},
			},
			expectErr:   true,
			errContains: "total percentage must equal 1.0",
		},
		{
			name:        "negative_percentage",
			totalAmount: sdkmath.NewInt(1000000),
			allocations: []keeper.BootstrapAllocation{
				{
					ModuleAccount: types.ModuleAccountTreasury,
					Percentage:    sdkmath.LegacyMustNewDecFromStr("-0.5"),
				},
			},
			expectErr:   true,
			errContains: "negative percentage",
		},
		{
			name:        "percentage_exceeds_one",
			totalAmount: sdkmath.NewInt(1000000),
			allocations: []keeper.BootstrapAllocation{
				{
					ModuleAccount: types.ModuleAccountTreasury,
					Percentage:    sdkmath.LegacyMustNewDecFromStr("2.0"),
				},
			},
			expectErr:   true,
			errContains: "percentage exceeds 1.0",
		},
		{
			name:        "empty_module_account_name",
			totalAmount: sdkmath.NewInt(1000000),
			allocations: []keeper.BootstrapAllocation{
				{
					ModuleAccount: "",
					Percentage:    sdkmath.LegacyMustNewDecFromStr("1.0"),
				},
			},
			expectErr:   true,
			errContains: "empty module account name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := pseKeeper.PerformBootstrap(ctx, tc.totalAmount, bondDenom, tc.allocations, startTime)
			if tc.expectErr {
				requireT.Error(err)
				requireT.Contains(err.Error(), tc.errContains)
			} else {
				requireT.NoError(err)
			}
		})
	}
}
