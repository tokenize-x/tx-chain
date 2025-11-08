package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestGetSetParams(t *testing.T) {
	requireT := require.New(t)
	assertT := assert.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	// Test getting default params
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Empty(params.ExcludedAddresses)

	// Test setting params with excluded addresses
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	newParams := types.Params{
		ExcludedAddresses: []string{addr1, addr2},
	}

	err = pseKeeper.SetParams(ctx, newParams)
	requireT.NoError(err)

	// Verify params were set correctly
	params, err = pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Len(params.ExcludedAddresses, 2)
	assertT.Contains(params.ExcludedAddresses, addr1)
	assertT.Contains(params.ExcludedAddresses, addr2)

	// Test setting params with empty excluded addresses
	emptyParams := types.Params{
		ExcludedAddresses: []string{},
	}

	err = pseKeeper.SetParams(ctx, emptyParams)
	requireT.NoError(err)

	params, err = pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Empty(params.ExcludedAddresses)
}

func TestUpdateExcludedAddresses(t *testing.T) {
	requireT := require.New(t)
	assertT := assert.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	// Use correct authority unless test specifies otherwise
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr5 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	invalidAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name             string
		authority        string
		initialAddresses []string
		addressesToAdd   []string
		addressesToRem   []string
		expectedFinal    []string
		expectErr        bool
		errMsg           string
	}{
		{
			name:             "add_to_empty_list",
			initialAddresses: []string{},
			addressesToAdd:   []string{addr1},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1},
			expectErr:        false,
		},
		{
			name:             "add_multiple_addresses",
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2, addr3},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2, addr3},
			expectErr:        false,
		},
		{
			name:             "remove_existing_address",
			initialAddresses: []string{addr1, addr2, addr3},
			addressesToAdd:   []string{},
			addressesToRem:   []string{addr2},
			expectedFinal:    []string{addr1, addr3},
			expectErr:        false,
		},
		{
			name:             "add_and_remove_different_addresses",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr3, addr4},
			addressesToRem:   []string{addr1},
			expectedFinal:    []string{addr2, addr3, addr4},
			expectErr:        false,
		},
		{
			name:             "remove_nonexistent_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{},
			addressesToRem:   []string{addr5},
			expectedFinal:    []string{addr1, addr2},
			expectErr:        false,
		},
		{
			name:             "add_duplicate_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr1},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2}, // Should not add duplicate
			expectErr:        false,
		},
		{
			name:             "remove_then_add_different_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr3},
			addressesToRem:   []string{addr1},
			expectedFinal:    []string{addr2, addr3},
			expectErr:        false,
		},
		{
			name:             "add_multiple_including_duplicate",
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2, addr1, addr3},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2, addr3}, // addr1 should not be duplicated
			expectErr:        false,
		},
		{
			name:             "invalid_authority",
			authority:        invalidAuthority,
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2},
			addressesToRem:   []string{},
			expectErr:        true,
			errMsg:           "expected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.authority != "" {
				authority = tc.authority
			}

			// Set initial params
			initialParams := types.Params{
				ExcludedAddresses: tc.initialAddresses,
			}
			err := pseKeeper.SetParams(ctx, initialParams)
			requireT.NoError(err)

			// Update excluded addresses
			err = pseKeeper.UpdateExcludedAddresses(ctx, authority, tc.addressesToAdd, tc.addressesToRem)

			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)

				// Verify final state
				params, err := pseKeeper.GetParams(ctx)
				requireT.NoError(err)
				assertT.Len(params.ExcludedAddresses, len(tc.expectedFinal))

				for _, expectedAddr := range tc.expectedFinal {
					assertT.Contains(params.ExcludedAddresses, expectedAddr)
				}
			}
		})
	}
}

func TestUpdateSubAccountMappings_ReferentialIntegrity(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Test 1: Can add mappings when no schedule exists
	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: addr1},
		{ClearingAccount: types.ModuleAccountTeam, RecipientAddress: addr2},
	}

	err := pseKeeper.UpdateSubAccountMappings(ctx, authority, mappings)
	requireT.NoError(err, "should allow adding mappings when no schedule")

	// Test 2: Can remove mappings when not referenced in schedule
	reducedMappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: addr1},
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, reducedMappings)
	requireT.NoError(err, "should allow removing unreferenced mappings")

	// Test 3: Add a schedule that references treasury
	scheduledDist := types.ScheduledDistribution{
		Timestamp: 2000000000,
		Allocations: []types.ClearingAccountAllocation{
			{ClearingAccount: types.ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
		},
	}

	err = pseKeeper.AllocationSchedule.Set(ctx, scheduledDist.Timestamp, scheduledDist)
	requireT.NoError(err)

	// Test 4: Cannot remove treasury mapping while schedule references it
	emptyMappings := []types.ClearingAccountMapping{}
	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, emptyMappings)
	requireT.Error(err, "should reject removal of mapping referenced in schedule")
	requireT.Contains(err.Error(), "still referenced in the allocation schedule")

	// Test 5: Can update mapping address while keeping the module
	updatedMappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: addr3}, // Changed address
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, updatedMappings)
	requireT.NoError(err, "should allow updating mapping address")

	// Verify the address was updated
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	requireT.Len(params.ClearingAccountMappings, 1)
	requireT.Equal(addr3, params.ClearingAccountMappings[0].RecipientAddress)

	// Test 6: Can add more mappings even with existing schedule
	expandedMappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: addr3},
		{ClearingAccount: types.ModuleAccountTeam, RecipientAddress: addr2},
	}

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, expandedMappings)
	requireT.NoError(err, "should allow adding new mappings")

	// Test 7: Clear schedule, then can remove mappings
	err = pseKeeper.AllocationSchedule.Remove(ctx, scheduledDist.Timestamp)
	requireT.NoError(err)

	err = pseKeeper.UpdateSubAccountMappings(ctx, authority, emptyMappings)
	requireT.NoError(err, "should allow removing all mappings after schedule is cleared")
}

func TestUpdateSubAccountMappings_Authority(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	correctAuthority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	wrongAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ModuleAccountFoundation, RecipientAddress: addr1},
	}

	// Test with wrong authority
	err := pseKeeper.UpdateSubAccountMappings(ctx, wrongAuthority, mappings)
	requireT.Error(err, "should reject wrong authority")
	requireT.Contains(err.Error(), "invalid authority")

	// Test with correct authority
	err = pseKeeper.UpdateSubAccountMappings(ctx, correctAuthority, mappings)
	requireT.NoError(err, "should accept correct authority")
}
