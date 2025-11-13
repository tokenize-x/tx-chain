package types

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestDefaultParams(t *testing.T) {
	requireT := require.New(t)

	params := DefaultParams()
	requireT.Empty(params.ExcludedAddresses)
	requireT.Empty(params.ClearingAccountMappings)
	requireT.NoError(params.ValidateBasic())
}

func TestParamsValidation_ExcludedAddresses(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_empty_excluded_addresses",
			params: Params{
				ExcludedAddresses: []string{},
			},
			expectErr: false,
		},
		{
			name: "valid_one_excluded_address",
			params: Params{
				ExcludedAddresses: []string{addr1},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_excluded_addresses",
			params: Params{
				ExcludedAddresses: []string{addr1, addr2, addr3},
			},
			expectErr: false,
		},
		{
			name: "invalid_malformed_address",
			params: Params{
				ExcludedAddresses: []string{"invalid-address"},
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_duplicate_address",
			params: Params{
				ExcludedAddresses: []string{addr1, addr2, addr1},
			},
			expectErr: true,
			errMsg:    "duplicate address",
		},
		{
			name: "invalid_empty_string_in_list",
			params: Params{
				ExcludedAddresses: []string{addr1, ""},
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_wrong_prefix",
			params: Params{
				ExcludedAddresses: []string{addr1, "cosmos1invalidprefix"},
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_duplicate_at_end",
			params: Params{
				ExcludedAddresses: []string{addr1, addr2, addr3, addr1},
			},
			expectErr: true,
			errMsg:    "duplicate address",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			err := tc.params.ValidateBasic()
			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
			}
		})
	}
}

func TestValidateClearingAccountMappings(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_single_mapping",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{
						ClearingAccount:  ClearingAccountFoundation,
						RecipientAddress: addr1,
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_mappings",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
					{ClearingAccount: ClearingAccountTeam, RecipientAddress: addr2},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_empty_clearing_account",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: "", RecipientAddress: addr1},
				},
			},
			expectErr: true,
			errMsg:    "clearing_account cannot be empty",
		},
		{
			name: "invalid_malformed_sub_account_address",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountFoundation, RecipientAddress: "invalid"},
				},
			},
			expectErr: true,
			errMsg:    "invalid sub account address",
		},
		{
			name: "invalid_duplicate_clearing_account",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
					{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr2},
				},
			},
			expectErr: true,
			errMsg:    "duplicate clearing account",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			err := tc.params.ValidateBasic()
			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
			}
		})
	}
}

// Helper function to create valid allocations for all PSE clearing accounts.
// All clearing accounts (including Community) are included in the schedule.
// Community uses score-based distribution, others use direct recipient transfers.
func createAllModuleAllocations(amount sdkmath.Int) []ClearingAccountAllocation {
	var allocations []ClearingAccountAllocation
	for _, clearingAccount := range GetAllClearingAccounts() {
		allocations = append(allocations, ClearingAccountAllocation{
			ClearingAccount: clearingAccount,
			Amount:          amount,
		})
	}
	return allocations
}

// Helper function to generate test timestamps
// Returns timestamp for the first day of the current month plus offsetMonths.
func getTestTimestamp(offsetMonths int) uint64 {
	now := time.Now().UTC()
	baseTime := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return uint64(baseTime.AddDate(0, offsetMonths, 0).Unix())
}

func TestValidateAllocationSchedule(t *testing.T) {
	testCases := []struct {
		name      string
		schedule  []ScheduledDistribution
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid_empty_schedule",
			schedule:  []ScheduledDistribution{},
			expectErr: false,
		},
		{
			name: "valid_single_period",
			schedule: []ScheduledDistribution{
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createAllModuleAllocations(sdkmath.NewInt(1000)),
				},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_periods_sorted",
			schedule: []ScheduledDistribution{
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createAllModuleAllocations(sdkmath.NewInt(1000)),
				},
				{
					Timestamp:   getTestTimestamp(12),
					Allocations: createAllModuleAllocations(sdkmath.NewInt(2000)),
				},
			},
			expectErr: false,
		},
		{
			name: "valid_with_community_account",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						// All clearing accounts (including Community) should be in schedule
						{ClearingAccount: ClearingAccountCommunity, Amount: sdkmath.NewInt(5000)},
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_zero_timestamp",
			schedule: []ScheduledDistribution{
				{
					Timestamp: 0,
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "timestamp cannot be zero",
		},
		{
			name: "invalid_duplicate_timestamp",
			schedule: []ScheduledDistribution{
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createAllModuleAllocations(sdkmath.NewInt(1000)),
				},
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createAllModuleAllocations(sdkmath.NewInt(2000)),
				},
			},
			expectErr: true,
			errMsg:    "duplicate timestamp",
		},
		{
			name: "invalid_unsorted_schedule",
			schedule: []ScheduledDistribution{
				{
					Timestamp:   getTestTimestamp(12),
					Allocations: createAllModuleAllocations(sdkmath.NewInt(2000)),
				},
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createAllModuleAllocations(sdkmath.NewInt(1000)),
				},
			},
			expectErr: true,
			errMsg:    "must be sorted by timestamp in ascending order",
		},
		{
			name: "invalid_empty_allocations_array",
			schedule: []ScheduledDistribution{
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{},
				},
			},
			expectErr: true,
			errMsg:    "must have at least one allocation",
		},
		{
			name: "invalid_too_few_allocations",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "missing allocation for required PSE clearing account",
		},
		{
			name: "invalid_empty_clearing_account",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: "", Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "clearing_account cannot be empty",
		},
		{
			name: "invalid_unknown_clearing_account",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: "unknown_module", Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountCommunity, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "clearing account not found",
		},
		{
			name: "invalid_duplicate_clearing_account_in_period",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(2000)}, // Duplicate
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "duplicate clearing account",
		},
		{
			name: "invalid_missing_clearing_account",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						// Missing ClearingAccountCommunity and ClearingAccountTeam
					},
				},
			},
			expectErr: true,
			errMsg:    "missing allocation for required PSE clearing account",
		},
		{
			name: "invalid_nil_amount",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.Int{}},
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "amount cannot be nil",
		},
		{
			name: "invalid_negative_amount",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(-1000)},
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "amount cannot be negative",
		},
		{
			name: "invalid_zero_amount",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.ZeroInt()},
						{ClearingAccount: ClearingAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "amount cannot be zero",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			err := ValidateAllocationSchedule(tc.schedule)
			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
			}
		})
	}
}

func TestValidateScheduleMappingConsistency(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		schedule  []ScheduledDistribution
		mappings  []ClearingAccountMapping
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_schedule_with_all_mappings",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
				{ClearingAccount: ClearingAccountTeam, RecipientAddress: addr2},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(2000)},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_empty_schedule",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
			},
			schedule:  []ScheduledDistribution{},
			expectErr: false,
		},
		{
			name: "valid_extra_mappings_not_in_schedule",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
				{ClearingAccount: ClearingAccountTeam, RecipientAddress: addr2},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_schedule_without_mapping",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "no recipient mapping found for clearing account 'pse_team'",
		},
		{
			name: "invalid_multiple_modules_one_missing_mapping",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ClearingAccountTeam, Amount: sdkmath.NewInt(2000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "no recipient mapping found for clearing account 'pse_team'",
		},
		{
			name:     "invalid_no_mappings_but_has_schedule",
			mappings: []ClearingAccountMapping{},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "no recipient mapping found for clearing account 'pse_foundation'",
		},
		{
			name: "valid_community_excluded_no_mapping_required",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ClearingAccountCommunity, Amount: sdkmath.NewInt(1000)},  // Excluded - no mapping needed
						{ClearingAccount: ClearingAccountFoundation, Amount: sdkmath.NewInt(2000)}, // Needs mapping
					},
				},
			},
			expectErr: false, // Community doesn't need a mapping since it's excluded
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			err := ValidateScheduleMappingConsistency(tc.schedule, tc.mappings)
			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
			}
		})
	}
}

func TestParamsValidation_ClearingAccountNames(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_module_name_in_mapping",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_custom_module_name",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: "my_custom_module", RecipientAddress: addr1},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_bech32_address_as_clearing_account_in_mapping",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: addr1, RecipientAddress: addr1}, // Using bech32 address as module name
				},
			},
			expectErr: false, // No validation against bech32 - just non-empty string
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			err := tc.params.ValidateBasic()
			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
			}
		})
	}
}

func TestParamsValidation_CompleteScenarios(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_complete_configuration",
			params: Params{
				ExcludedAddresses: []string{addr1},
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr2},
					{ClearingAccount: ClearingAccountTeam, RecipientAddress: addr3},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_modules_all_mapped",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountFoundation, RecipientAddress: addr1},
					{ClearingAccount: ClearingAccountPartnership, RecipientAddress: addr2},
					{ClearingAccount: ClearingAccountTeam, RecipientAddress: addr3},
				},
			},
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			err := tc.params.ValidateBasic()
			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)
			}
		})
	}
}
