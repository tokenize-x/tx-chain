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

func TestDistribution_WithBootstrap(t *testing.T) {
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

	// Step 1: Set up sub-account mappings BEFORE bootstrap (required by referential integrity)
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

	// Step 2: Perform Bootstrap
	totalMintAmount := sdkmath.NewInt(100_000_000_000)        // 100 billion
	startTime := uint64(time.Now().Add(1 * time.Hour).Unix()) // 1 hour from now

	err = pseKeeper.PerformBootstrap(ctx, totalMintAmount, bondDenom, nil, startTime)
	requireT.NoError(err)

	// Step 3: Verify module accounts have correct balances
	allocations := keeper.DefaultBootstrapAllocations()

	for _, allocation := range allocations {
		expectedAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		moduleAddr := testApp.AccountKeeper.GetModuleAddress(allocation.ModuleAccount)
		requireT.NotNil(moduleAddr)

		balance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
		requireT.Equal(expectedAmount.String(), balance.Amount.String(),
			"module %s should have correct balance", allocation.ModuleAccount)
	}

	// Step 4: Verify distribution schedule was created
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	requireT.Len(params.DistributionSchedule, keeper.TotalDistributionMonths,
		"should have 84 monthly distributions")

	// Step 5: Verify first distribution amounts
	firstPeriod := params.DistributionSchedule[0]
	requireT.Equal(startTime, firstPeriod.DistributionTime)
	requireT.Len(firstPeriod.Distributions, len(allocations))

	for _, dist := range firstPeriod.Distributions {
		// Each monthly distribution should be 1/84th of the module's total
		var expectedTotal sdkmath.Int
		for _, alloc := range allocations {
			if alloc.ModuleAccount == dist.ModuleAccount {
				expectedTotal = alloc.Percentage.MulInt(totalMintAmount).TruncateInt()
				break
			}
		}
		expectedMonthly := expectedTotal.QuoRaw(keeper.TotalDistributionMonths)
		requireT.Equal(expectedMonthly.String(), dist.Amount.String(),
			"monthly amount for %s should be 1/84th of total", dist.ModuleAccount)
	}

	// Step 6: Verify pending timestamps were added
	pendingInfo, err := pseKeeper.GetPendingDistributionsInfo(ctx)
	requireT.NoError(err)
	requireT.Len(pendingInfo, keeper.TotalDistributionMonths,
		"all 84 months should be pending")

	// Step 7: Fast-forward time to first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(startTime)+10, 0)) // 10 seconds after first distribution time
	ctx = ctx.WithBlockHeight(100)

	// Step 8: Process distributions
	err = pseKeeper.ProcessPeriodicDistributions(ctx)
	requireT.NoError(err)

	// Step 9: Verify first month distributions were completed
	for _, dist := range firstPeriod.Distributions {
		// Check it's marked as completed
		key := types.MakeCompletedDistributionKey(dist.ModuleAccount, int64(startTime))
		has, err := pseKeeper.CompletedDistributions.Has(ctx, key)
		requireT.NoError(err)
		requireT.True(has, "distribution for %s should be completed", dist.ModuleAccount)

		// Verify sub-account received the tokens
		var subAccountAddr string
		for _, mapping := range mappings {
			if mapping.ModuleAccount == dist.ModuleAccount {
				subAccountAddr = mapping.SubAccountAddress
				break
			}
		}

		subAccAddr := sdk.MustAccAddressFromBech32(subAccountAddr)
		subAccBalance := bankKeeper.GetBalance(ctx, subAccAddr, bondDenom)
		requireT.Equal(dist.Amount.String(), subAccBalance.Amount.String(),
			"sub-account should have received distribution amount")
	}

	// Step 10: Verify pending distributions count decreased
	pendingInfoAfter, err := pseKeeper.GetPendingDistributionsInfo(ctx)
	requireT.NoError(err)
	requireT.Len(pendingInfoAfter, keeper.TotalDistributionMonths-1,
		"should have 83 pending months remaining")

	// Step 11: Verify completed distributions query
	completedDists, err := pseKeeper.GetCompletedDistributions(ctx)
	requireT.NoError(err)
	requireT.Len(completedDists, len(firstPeriod.Distributions),
		"should have completed distributions equal to first period count")
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

	mappings := []types.SubAccountMapping{
		{ModuleAccount: types.ModuleAccountTreasury, SubAccountAddress: multisigAddr1},
		{ModuleAccount: types.ModuleAccountTeam, SubAccountAddress: multisigAddr2},
	}

	for _, moduleAccount := range []string{types.ModuleAccountTreasury, types.ModuleAccountTeam} {
		fundAmount := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(5000)))
		err = testApp.BankKeeper.MintCoins(ctx, types.ModuleName, fundAmount)
		requireT.NoError(err)
		err = testApp.BankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, moduleAccount, fundAmount)
		requireT.NoError(err)
	}

	time1 := uint64(time.Now().Add(1 * time.Hour).Unix())
	time2 := uint64(time.Now().Add(2 * time.Hour).Unix())

	// Create a schedule
	schedule := []types.DistributionPeriod{
		{
			DistributionTime: time1,
			Distributions: []types.ModuleDistribution{
				{ModuleAccount: types.ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
				{ModuleAccount: types.ModuleAccountTeam, Amount: sdkmath.NewInt(500)},
			},
		},
		{
			DistributionTime: time2,
			Distributions: []types.ModuleDistribution{
				{ModuleAccount: types.ModuleAccountTreasury, Amount: sdkmath.NewInt(2000)},
			},
		},
	}

	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.SubAccountMappings = mappings
	params.DistributionSchedule = schedule
	err = pseKeeper.SetParams(ctx, params)
	requireT.NoError(err)

	// Manually add to pending queue
	err = pseKeeper.PendingTimestamps.Set(ctx, time1)
	requireT.NoError(err)
	err = pseKeeper.PendingTimestamps.Set(ctx, time2)
	requireT.NoError(err)

	// Process first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(time1)+10, 0))
	ctx = ctx.WithBlockHeight(100)
	err = pseKeeper.ProcessPeriodicDistributions(ctx)
	requireT.NoError(err)

	// Export genesis
	genesisState, err := pseKeeper.ExportGenesis(ctx)
	requireT.NoError(err)

	// Verify export contains:
	// - 2 distributions in schedule
	// - 2 completed distributions (treasury + team from time1)
	// Note: PendingDistributionTimestamps might have stale data, will be rebuilt on import
	requireT.Len(genesisState.Params.DistributionSchedule, 2)
	requireT.Len(genesisState.CompletedDistributions, 2)

	// Create new app and import genesis
	testApp2 := simapp.New()
	ctx2 := testApp2.NewContext(false)
	ctx2 = ctx2.WithBlockTime(time.Unix(int64(time1)+10, 0)) // Set to same time as when we exported
	pseKeeper2 := testApp2.PSEKeeper

	// InitGenesis should rebuild pending queue from schedule
	err = pseKeeper2.InitGenesis(ctx2, *genesisState)
	requireT.NoError(err)

	// Verify pending queue was correctly rebuilt
	// Should only have time2 since time1 is completed
	pendingInfo, err := pseKeeper2.GetPendingDistributionsInfo(ctx2)
	requireT.NoError(err)
	requireT.Len(pendingInfo, 1, "should have 1 pending period (time2)")
	requireT.Equal(time2, pendingInfo[0].DistributionTime)

	// Verify completed distributions were loaded
	completedDists, err := pseKeeper2.GetCompletedDistributions(ctx2)
	requireT.NoError(err)
	requireT.Len(completedDists, 2, "should have 2 completed distributions")
}
