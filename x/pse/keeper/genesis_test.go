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
	valAddr1 := sdk.ValAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	genesisState.Params.ExcludedAddresses = []string{
		addr1,
		addr2,
	}
	now := time.Now()
	genesisState.DelegationTimeEntries = []types.DelegationTimeEntryExport{
		{
			ValidatorAddress:   valAddr1,
			DelegatorAddress:   addr1,
			Shares:             sdkmath.LegacyNewDec(432),
			LastChangedUnixSec: now.Unix(),
		},
		{
			ValidatorAddress:   valAddr1,
			DelegatorAddress:   addr2,
			Shares:             sdkmath.LegacyNewDec(832),
			LastChangedUnixSec: now.Unix(),
		},
	}
	genesisState.AccountScores = []types.AccountScore{
		{
			Address: addr1,
			Score:   sdkmath.NewInt(1234),
		},
		{
			Address: addr2,
			Score:   sdkmath.NewInt(5678),
		},
	}
	genesisState.DistributionsDisabled = true

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
	requireT.Equal(genesisState.DistributionsDisabled, got.DistributionsDisabled)
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
	// since test app has a default validator, the exported genesis will contain an staking snapshot,
	// so AccountScores and DelegationTimeEntries cannot be compared directly.
	requireT.EqualExportedValues(defaultGenesis.ScheduledDistributions, exported.ScheduledDistributions)
	requireT.EqualExportedValues(defaultGenesis.Params, exported.Params)
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
				// Only include 4 accounts, missing Community and Team
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
			expectError: "missing allocation for required clearing account",
		},
		{
			name: "invalid_allocation_schedule_excluded_account",
			modifyGenesis: func(gs *types.GenesisState) {
				now := uint64(time.Now().Unix())
				// Include only non-Community accounts, missing Community
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
			},
			expectError: "missing allocation for required clearing account",
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
