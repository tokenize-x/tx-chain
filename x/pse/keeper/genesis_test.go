package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestGenesis(t *testing.T) {
	requireT := require.New(t)

	// Use DefaultGenesisState to ensure all slices are properly initialized
	genesisState := *types.DefaultGenesisState()

	// Setup test app first to get correct bech32 prefix
	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	// Generate addresses after setting up test app (which sets the correct bech32 config)
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	genesisState.Params.ExcludedAddresses = []string{
		addr1,
		addr2,
	}

	err := pseKeeper.InitGenesis(ctx, genesisState)
	requireT.NoError(err)
	got, err := pseKeeper.ExportGenesis(ctx)
	requireT.NoError(err)
	requireT.NotNil(got)

	if got.Params.ClearingAccountMappings == nil {
		got.Params.ClearingAccountMappings = []types.ClearingAccountMapping{}
	}
	if got.ScheduledDistributions == nil {
		got.ScheduledDistributions = []types.ScheduledDistribution{}
	}

	requireT.EqualExportedValues(&genesisState.Params, &got.Params)
	requireT.EqualExportedValues(&genesisState.ScheduledDistributions, &got.ScheduledDistributions)
}

// TestGenesis_HardForkWithAllocations tests the hard fork scenario.
func TestGenesis_HardForkWithAllocations(t *testing.T) {
	requireT := require.New(t)

	// Setup initial chain state
	testApp1 := simapp.New()
	ctx1 := testApp1.NewContext(false)
	ctx1 = ctx1.WithBlockTime(time.Now())
	pseKeeper1 := testApp1.PSEKeeper

	// Get bond denom
	stakingParams, err := testApp1.StakingKeeper.GetParams(ctx1)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	// Setup mappings for all eligible module accounts
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr5 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ClearingAccountFoundation, RecipientAddress: addr1},
		{ClearingAccount: types.ClearingAccountAlliance, RecipientAddress: addr2},
		{ClearingAccount: types.ClearingAccountPartnership, RecipientAddress: addr3},
		{ClearingAccount: types.ClearingAccountInvestors, RecipientAddress: addr4},
		{ClearingAccount: types.ClearingAccountTeam, RecipientAddress: addr5},
	}

	// Setup params with mappings
	params, err := pseKeeper1.GetParams(ctx1)
	requireT.NoError(err)
	params.ClearingAccountMappings = mappings
	err = pseKeeper1.SetParams(ctx1, params)
	requireT.NoError(err)

	// Create allocation schedule with 3 periods
	now := time.Now()
	time1 := uint64(now.Add(1 * time.Hour).Unix())
	time2 := uint64(now.Add(2 * time.Hour).Unix())
	time3 := uint64(now.Add(3 * time.Hour).Unix())

	schedule := []types.ScheduledDistribution{
		{
			Timestamp: time1,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(200)},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(300)},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(400)},
				{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(500)},
			},
		},
		{
			Timestamp: time2,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(2000)},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(400)},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(600)},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(800)},
				{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
			},
		},
		{
			Timestamp: time3,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(3000)},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(600)},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(900)},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(1200)},
				{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(1500)},
			},
		},
	}

	// Store allocation schedule
	for _, scheduledDist := range schedule {
		err = pseKeeper1.AllocationSchedule.Set(ctx1, scheduledDist.Timestamp, scheduledDist)
		requireT.NoError(err)
	}

	// Fund all non-Community clearing accounts
	for _, clearingAccount := range []string{
		types.ClearingAccountFoundation,
		types.ClearingAccountAlliance,
		types.ClearingAccountPartnership,
		types.ClearingAccountInvestors,
		types.ClearingAccountTeam,
	} {
		fundAmount := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100000)))
		err = testApp1.BankKeeper.MintCoins(ctx1, types.ModuleName, fundAmount)
		requireT.NoError(err)
		err = testApp1.BankKeeper.SendCoinsFromModuleToModule(ctx1, types.ModuleName, clearingAccount, fundAmount)
		requireT.NoError(err)
	}

	// Process first distribution (time1)
	ctx1 = ctx1.WithBlockTime(time.Unix(int64(time1)+10, 0))
	ctx1 = ctx1.WithBlockHeight(100)
	err = pseKeeper1.ProcessNextDistribution(ctx1)
	requireT.NoError(err)

	// Export genesis state (simulating hard fork preparation)
	exportedGenesis, err := pseKeeper1.ExportGenesis(ctx1)
	requireT.NoError(err)
	requireT.NotNil(exportedGenesis)

	// Verify exported state has 2 remaining allocations (time2 and time3)
	requireT.Len(exportedGenesis.ScheduledDistributions, 2, "should have 2 unprocessed allocations")
	requireT.Equal(time2, exportedGenesis.ScheduledDistributions[0].Timestamp)
	requireT.Equal(time3, exportedGenesis.ScheduledDistributions[1].Timestamp)

	// Verify exported state has all 5 mappings
	requireT.Len(exportedGenesis.Params.ClearingAccountMappings, 5, "should have all 5 mappings")

	// Verify each period has all 5 eligible accounts
	for i, period := range exportedGenesis.ScheduledDistributions {
		requireT.Len(period.Allocations, 5, "period %d should have all 5 eligible accounts", i)
	}

	// Verify exported state is valid
	err = exportedGenesis.Validate()
	requireT.NoError(err, "exported genesis should be valid")

	// Create new chain and import genesis (hard fork scenario)
	testApp2 := simapp.New()
	ctx2 := testApp2.NewContext(false)
	ctx2 = ctx2.WithBlockTime(time.Unix(int64(time1)+10, 0))
	ctx2 = ctx2.WithBlockHeight(100)
	pseKeeper2 := testApp2.PSEKeeper

	// InitGenesis should successfully import the exported state
	err = pseKeeper2.InitGenesis(ctx2, *exportedGenesis)
	requireT.NoError(err, "InitGenesis should succeed with valid exported state")

	// Verify allocation schedule was properly imported
	importedSchedule, err := pseKeeper2.GetAllocationSchedule(ctx2)
	requireT.NoError(err)
	requireT.Len(importedSchedule, 2, "imported schedule should have 2 allocations")
	requireT.Equal(time2, importedSchedule[0].Timestamp)
	requireT.Equal(time3, importedSchedule[1].Timestamp)

	// Verify params were properly imported
	importedParams, err := pseKeeper2.GetParams(ctx2)
	requireT.NoError(err)
	requireT.Len(importedParams.ClearingAccountMappings, 5, "imported params should have all 5 mappings")

	// Verify we can export the same state again (round-trip test)
	reexportedGenesis, err := pseKeeper2.ExportGenesis(ctx2)
	requireT.NoError(err)
	requireT.EqualExportedValues(exportedGenesis, reexportedGenesis, "re-exported genesis should match")

	// Process next distribution on new chain (time2)
	ctx2 = ctx2.WithBlockTime(time.Unix(int64(time2)+10, 0))
	ctx2 = ctx2.WithBlockHeight(200)

	// Fund non-Community clearing accounts on new chain before processing
	for _, clearingAccount := range []string{
		types.ClearingAccountFoundation,
		types.ClearingAccountAlliance,
		types.ClearingAccountPartnership,
		types.ClearingAccountInvestors,
		types.ClearingAccountTeam,
	} {
		fundAmount := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100000)))
		err = testApp2.BankKeeper.MintCoins(ctx2, types.ModuleName, fundAmount)
		requireT.NoError(err)
		err = testApp2.BankKeeper.SendCoinsFromModuleToModule(ctx2, types.ModuleName, clearingAccount, fundAmount)
		requireT.NoError(err)
	}

	err = pseKeeper2.ProcessNextDistribution(ctx2)
	requireT.NoError(err, "should process time2 distribution on new chain")

	// Verify only time3 remains
	finalSchedule, err := pseKeeper2.GetAllocationSchedule(ctx2)
	requireT.NoError(err)
	requireT.Len(finalSchedule, 1, "should have 1 remaining allocation")
	requireT.Equal(time3, finalSchedule[0].Timestamp)
}

// TestGenesis_EmptyState tests that default genesis state is valid and can be imported/exported.
func TestGenesis_EmptyState(t *testing.T) {
	requireT := require.New(t)

	// Get default genesis state
	defaultGenesis := types.DefaultGenesisState()
	requireT.NotNil(defaultGenesis)

	// Validate default genesis
	err := defaultGenesis.Validate()
	requireT.NoError(err, "default genesis should be valid")

	// Verify default state has nil allocations (not empty slices)
	requireT.Empty(defaultGenesis.ScheduledDistributions, "default genesis should have nil allocations")
	requireT.Empty(defaultGenesis.Params.ClearingAccountMappings, "default genesis should have nil mappings")
	requireT.Empty(defaultGenesis.Params.ExcludedAddresses, "default genesis should have nil excluded addresses")

	// Import into keeper
	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	err = pseKeeper.InitGenesis(ctx, *defaultGenesis)
	requireT.NoError(err, "should import default genesis")

	// Export and verify it matches
	exported, err := pseKeeper.ExportGenesis(ctx)
	requireT.NoError(err)

	if exported.Params.ExcludedAddresses == nil {
		exported.Params.ExcludedAddresses = []string{}
	}
	if exported.Params.ClearingAccountMappings == nil {
		exported.Params.ClearingAccountMappings = []types.ClearingAccountMapping{}
	}
	if exported.ScheduledDistributions == nil {
		exported.ScheduledDistributions = []types.ScheduledDistribution{}
	}
	requireT.EqualExportedValues(defaultGenesis, exported, "exported should match default")
}

// TestGenesis_InvalidState tests that invalid genesis state is rejected.
func TestGenesis_InvalidState(t *testing.T) {
	testCases := []struct {
		name          string
		modifyGenesis func(*types.GenesisState)
		expectError   string
	}{
		{
			name: "invalid_allocation_schedule_missing_account",
			modifyGenesis: func(gs *types.GenesisState) {
				now := uint64(time.Now().Unix())
				// Only include 4 accounts, missing ClearingAccountTeam
				gs.ScheduledDistributions = []types.ScheduledDistribution{
					{
						Timestamp: now,
						Allocations: []types.ClearingAccountAllocation{
							{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
							{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(200)},
							{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(300)},
							{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(400)},
						},
					},
				}
			},
			expectError: "missing allocation for required non-Community PSE clearing account",
		},
		{
			name: "invalid_allocation_schedule_excluded_account",
			modifyGenesis: func(gs *types.GenesisState) {
				now := uint64(time.Now().Unix())
				// Include Community account (which is excluded)
				gs.ScheduledDistributions = []types.ScheduledDistribution{
					{
						Timestamp: now,
						Allocations: []types.ClearingAccountAllocation{
							{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
							{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(200)},
							{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(300)},
							{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(400)},
							{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(500)},
							{ClearingAccount: types.ClearingAccountCommunity, Amount: sdkmath.NewInt(600)},
						},
					},
				}
			},
			expectError: "is not a non-Community PSE clearing account",
		},
		{
			name: "missing_mappings_for_allocations",
			modifyGenesis: func(gs *types.GenesisState) {
				now := uint64(time.Now().Unix())
				// Add allocations without corresponding mappings
				gs.ScheduledDistributions = []types.ScheduledDistribution{
					{
						Timestamp: now,
						Allocations: []types.ClearingAccountAllocation{
							{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
							{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(200)},
							{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(300)},
							{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(400)},
							{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(500)},
						},
					},
				}
				// No mappings (nil)
				gs.Params.ClearingAccountMappings = nil
			},
			expectError: "no recipient mapping found for clearing account",
		},
		{
			name: "invalid_excluded_address",
			modifyGenesis: func(gs *types.GenesisState) {
				gs.Params.ExcludedAddresses = []string{"invalid-address"}
			},
			expectError: "invalid address",
		},
		{
			name: "duplicate_excluded_address",
			modifyGenesis: func(gs *types.GenesisState) {
				addr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
				gs.Params.ExcludedAddresses = []string{addr, addr}
			},
			expectError: "duplicate address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			// Start with default genesis
			genesisState := types.DefaultGenesisState()

			// Apply modification
			tc.modifyGenesis(genesisState)

			// Create app and try to import
			testApp := simapp.New()
			ctx := testApp.NewContext(false)
			pseKeeper := testApp.PSEKeeper

			// Should fail validation
			err := pseKeeper.InitGenesis(ctx, *genesisState)
			requireT.Error(err, "InitGenesis should reject invalid state")
			requireT.Contains(err.Error(), tc.expectError, "error should contain expected message")
		})
	}
}
