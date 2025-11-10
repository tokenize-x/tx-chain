package keeper_test

import (
	"context"
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

	return schedule
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
	multisigAddr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	multisigAddr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountCommunity, RecipientAddress: multisigAddr1},
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: multisigAddr2},
		{ClearingAccount: types.ModuleAccountTeam, RecipientAddress: multisigAddr3},
	}

	err = pseKeeper.UpdateClearingMappings(ctx, authority, mappings)
	requireT.NoError(err)

	// Step 2: Create a distribution schedule manually for testing
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix()) // 1 hour ago (already due)

	// Create allocations and calculate total mint amount
	totalMint := sdkmath.NewInt(200_000_000_000) // 200B total
	allocations := []v6.InitialAllocation{
		{ModuleAccount: types.ModuleAccountCommunity, Percentage: sdkmath.LegacyMustNewDecFromStr("0.50")},  // 100B
		{ModuleAccount: types.ModuleAccountFoundation, Percentage: sdkmath.LegacyMustNewDecFromStr("0.25")}, // 50B
		{ModuleAccount: types.ModuleAccountTeam, Percentage: sdkmath.LegacyMustNewDecFromStr("0.25")},       // 50B
	}

	// Mint tokens to module accounts for distribution
	for _, allocation := range allocations {
		amount := allocation.Percentage.MulInt(totalMint).TruncateInt()
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, amount))
		err = bankKeeper.MintCoins(ctx, types.ModuleName, coins)
		requireT.NoError(err)
		err = bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, allocation.ModuleAccount, coins)
		requireT.NoError(err)
	}

	// Create schedule
	schedule, err := v6.CreateDistributionSchedule(allocations, totalMint, startTime)
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
	err = pseKeeper.ProcessNextDistribution(ctx)
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
	err = pseKeeper.ProcessNextDistribution(ctx)
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
