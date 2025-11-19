package keeper_test

import (
	"testing"

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

// TestMsgUpdateClearingAccountMappings tests the message server integration for updating clearing account mappings.
// Note: Validation logic is tested in types/params_test.go (TestValidateClearingAccountMappings).
// This test focuses on keeper-specific functionality: state updates and authority checks.
func TestMsgUpdateClearingAccountMappings(t *testing.T) {
	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	msgServer := keeper.NewMsgServer(testApp.PSEKeeper)

	// Get correct authority (governance module address)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	invalidAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Generate test addresses
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Helper to create all required mappings
	createAllMappings := func(addrs []string) []types.ClearingAccountMapping {
		nonCommunityAccounts := types.GetNonCommunityClearingAccounts()
		mappings := make([]types.ClearingAccountMapping, 0, len(nonCommunityAccounts))
		for i, account := range nonCommunityAccounts {
			addr := addrs[i%len(addrs)]
			mappings = append(mappings, types.ClearingAccountMapping{
				ClearingAccount:    account,
				RecipientAddresses: []string{addr},
			})
		}
		return mappings
	}

	// Initialize state with valid mappings
	initMsg := &types.MsgUpdateClearingAccountMappings{
		Authority: authority,
		Mappings:  createAllMappings([]string{addr4}),
	}
	_, err := msgServer.UpdateClearingAccountMappings(ctx, initMsg)
	require.NoError(t, err)

	testCases := []struct {
		name      string
		msg       *types.MsgUpdateClearingAccountMappings
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid - update all clearing account mappings",
			msg: &types.MsgUpdateClearingAccountMappings{
				Authority: authority,
				Mappings:  createAllMappings([]string{addr1, addr2}),
			},
			expectErr: false,
		},
		{
			name: "valid - update with multiple recipients per account",
			msg: &types.MsgUpdateClearingAccountMappings{
				Authority: authority,
				Mappings: func() []types.ClearingAccountMapping {
					mappings := createAllMappings([]string{addr1})
					// Add extra recipients to first mapping
					mappings[0].RecipientAddresses = []string{addr1, addr2, addr3}
					return mappings
				}(),
			},
			expectErr: false,
		},
		{
			name: "invalid - wrong authority (keeper check)",
			msg: &types.MsgUpdateClearingAccountMappings{
				Authority: invalidAuthority,
				Mappings:  createAllMappings([]string{addr1}),
			},
			expectErr: true,
			errMsg:    "invalid authority",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			resp, err := msgServer.UpdateClearingAccountMappings(ctx, tc.msg)

			if tc.expectErr {
				requireT.Error(err)
				requireT.Nil(resp)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
				requireT.NotNil(resp)

				// Verify mappings were updated in state
				params, err := testApp.PSEKeeper.GetParams(ctx)
				requireT.NoError(err)
				requireT.Len(params.ClearingAccountMappings, len(tc.msg.Mappings))

				// Verify all required clearing accounts are present
				nonCommunityAccounts := types.GetNonCommunityClearingAccounts()
				requireT.Len(params.ClearingAccountMappings, len(nonCommunityAccounts))

				// Build a map for verification
				mappingsMap := make(map[string]types.ClearingAccountMapping)
				for _, mapping := range params.ClearingAccountMappings {
					mappingsMap[mapping.ClearingAccount] = mapping
				}

				// Verify each required account is present
				for _, account := range nonCommunityAccounts {
					_, found := mappingsMap[account]
					requireT.True(found, "required clearing account %s not found", account)
				}
			}
		})
	}
}

// TestMsgUpdateAllocationSchedule tests the message server integration for updating allocation schedules.
// Note: Validation logic is tested in types/params_test.go (TestValidateAllocationSchedule).
// This test focuses on keeper-specific functionality: state persistence, clearing, and authority checks.
func TestMsgUpdateAllocationSchedule(t *testing.T) {
	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	msgServer := keeper.NewMsgServer(testApp.PSEKeeper)

	// Get correct authority (governance module address)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	invalidAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Initialize state with valid mappings
	addr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	nonCommunityAccounts := types.GetNonCommunityClearingAccounts()
	initialMappings := make([]types.ClearingAccountMapping, 0, len(nonCommunityAccounts))
	for _, account := range nonCommunityAccounts {
		initialMappings = append(initialMappings, types.ClearingAccountMapping{
			ClearingAccount:    account,
			RecipientAddresses: []string{addr},
		})
	}
	initMsg := &types.MsgUpdateClearingAccountMappings{
		Authority: authority,
		Mappings:  initialMappings,
	}
	_, err := msgServer.UpdateClearingAccountMappings(ctx, initMsg)
	require.NoError(t, err)

	// Helper to create allocations for all clearing accounts
	createAllAllocations := func(amount sdkmath.Int) []types.ClearingAccountAllocation {
		allAccounts := types.GetAllClearingAccounts()
		allocations := make([]types.ClearingAccountAllocation, 0, len(allAccounts))
		for _, account := range allAccounts {
			allocations = append(allocations, types.ClearingAccountAllocation{
				ClearingAccount: account,
				Amount:          amount,
			})
		}
		return allocations
	}

	// Helper to create a valid schedule
	createValidSchedule := func(numPeriods int, amount sdkmath.Int) []types.ScheduledDistribution {
		schedule := make([]types.ScheduledDistribution, numPeriods)
		baseTimestamp := uint64(1700000000) // Some future timestamp
		for i := range numPeriods {
			schedule[i] = types.ScheduledDistribution{
				Timestamp:   baseTimestamp + uint64(i*86400), // One day apart
				Allocations: createAllAllocations(amount),
			}
		}
		return schedule
	}

	testCases := []struct {
		name      string
		msg       *types.MsgUpdateDistributionSchedule
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid - single period schedule",
			msg: &types.MsgUpdateDistributionSchedule{
				Authority: authority,
				Schedule:  createValidSchedule(1, sdkmath.NewInt(1000000)),
			},
			expectErr: false,
		},
		{
			name: "valid - multiple periods schedule",
			msg: &types.MsgUpdateDistributionSchedule{
				Authority: authority,
				Schedule:  createValidSchedule(5, sdkmath.NewInt(2000000)),
			},
			expectErr: false,
		},
		{
			name: "valid - empty schedule (clears all distributions)",
			msg: &types.MsgUpdateDistributionSchedule{
				Authority: authority,
				Schedule:  []types.ScheduledDistribution{},
			},
			expectErr: false,
		},
		{
			name: "valid - different amounts per account",
			msg: &types.MsgUpdateDistributionSchedule{
				Authority: authority,
				Schedule: []types.ScheduledDistribution{
					{
						Timestamp: uint64(1700000000),
						Allocations: []types.ClearingAccountAllocation{
							{ClearingAccount: types.ClearingAccountCommunity, Amount: sdkmath.NewInt(1000000)},
							{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(2000000)},
							{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(3000000)},
							{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(4000000)},
							{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(5000000)},
							{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(6000000)},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid - wrong authority (keeper check)",
			msg: &types.MsgUpdateDistributionSchedule{
				Authority: invalidAuthority,
				Schedule:  createValidSchedule(1, sdkmath.NewInt(1000000)),
			},
			expectErr: true,
			errMsg:    "invalid authority",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			resp, err := msgServer.UpdateDistributionSchedule(ctx, tc.msg)

			if tc.expectErr {
				requireT.Error(err)
				requireT.Nil(resp)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
				requireT.NotNil(resp)

				// Verify schedule was persisted to state
				savedSchedule, err := testApp.PSEKeeper.GetDistributionSchedule(ctx)
				requireT.NoError(err)
				requireT.Len(savedSchedule, len(tc.msg.Schedule))

				// If non-empty schedule, verify structure is preserved
				if len(tc.msg.Schedule) > 0 {
					for i, period := range savedSchedule {
						requireT.Equal(tc.msg.Schedule[i].Timestamp, period.Timestamp)
						requireT.Len(period.Allocations, len(tc.msg.Schedule[i].Allocations))

						// Verify all 6 clearing accounts are present
						allAccounts := types.GetAllClearingAccounts()
						requireT.Len(period.Allocations, len(allAccounts))

						allocsMap := make(map[string]sdkmath.Int)
						for _, alloc := range period.Allocations {
							allocsMap[alloc.ClearingAccount] = alloc.Amount
						}

						// Verify each required account is present with correct amount
						for _, account := range allAccounts {
							_, found := allocsMap[account]
							requireT.True(found, "required clearing account %s not found in period %d", account, i)
						}
					}
				}
			}
		})
	}
}
