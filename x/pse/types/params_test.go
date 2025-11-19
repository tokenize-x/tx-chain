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

	// DefaultParams returns empty mappings - valid for genesis
	// Tests and actual usage should call UpdateClearingAccountMappings to set proper values
	err := params.ValidateBasic()
	requireT.NoError(err)
}

func TestParamsValidation_ExcludedAddresses(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_empty_excluded_addresses",
			params: Params{
				ExcludedAddresses:       []string{},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
			},
			expectErr: false,
		},
		{
			name: "valid_one_excluded_address",
			params: Params{
				ExcludedAddresses:       []string{addr1},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_excluded_addresses",
			params: Params{
				ExcludedAddresses:       []string{addr1, addr2, addr3},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
			},
			expectErr: false,
		},
		{
			name: "invalid_malformed_address",
			params: Params{
				ExcludedAddresses:       []string{"invalid-address"},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_duplicate_address",
			params: Params{
				ExcludedAddresses:       []string{addr1, addr2, addr1},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
			},
			expectErr: true,
			errMsg:    "duplicate address",
		},
		{
			name: "invalid_empty_string_in_list",
			params: Params{
				ExcludedAddresses:       []string{addr1, ""},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_wrong_prefix",
			params: Params{
				ExcludedAddresses:       []string{addr1, "cosmos1invalidprefix"},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_duplicate_at_end",
			params: Params{
				ExcludedAddresses:       []string{addr1, addr2, addr3, addr1},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr4}),
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
			name: "valid_all_required_mappings",
			params: Params{
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr1, addr2}),
			},
			expectErr: false,
		},
		{
			name: "valid_partial_mappings_allowed_in_genesis",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountFoundation, RecipientAddresses: []string{addr1}},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_empty_clearing_account",
			params: Params{
				ClearingAccountMappings: append(
					createAllClearingAccountMappings([]string{addr1}),
					ClearingAccountMapping{ClearingAccount: "", RecipientAddresses: []string{addr2}},
				),
			},
			expectErr: true,
			errMsg:    "clearing_account cannot be empty",
		},
		{
			name: "invalid_malformed_sub_account_address",
			params: Params{
				ClearingAccountMappings: func() []ClearingAccountMapping {
					mappings := createAllClearingAccountMappings([]string{addr1})
					mappings[0].RecipientAddresses = []string{"invalid"}
					return mappings
				}(),
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_duplicate_clearing_account",
			params: Params{
				ClearingAccountMappings: append(
					createAllClearingAccountMappings([]string{addr1}),
					ClearingAccountMapping{ClearingAccount: ClearingAccountFoundation, RecipientAddresses: []string{addr2}},
				),
			},
			expectErr: true,
			errMsg:    "duplicate clearing account",
		},
		{
			name: "invalid_empty_recipient_list",
			params: Params{
				ClearingAccountMappings: func() []ClearingAccountMapping {
					mappings := createAllClearingAccountMappings([]string{addr1})
					mappings[0].RecipientAddresses = []string{}
					return mappings
				}(),
			},
			expectErr: true,
			errMsg:    "must have at least one recipient address",
		},
		{
			name: "invalid_duplicate_recipients_in_same_mapping",
			params: Params{
				ClearingAccountMappings: func() []ClearingAccountMapping {
					mappings := createAllClearingAccountMappings([]string{addr1})
					mappings[0].RecipientAddresses = []string{addr1, addr1}
					return mappings
				}(),
			},
			expectErr: true,
			errMsg:    "duplicate recipient address",
		},
		{
			name: "valid_multiple_recipients",
			params: Params{
				ClearingAccountMappings: func() []ClearingAccountMapping {
					mappings := createAllClearingAccountMappings([]string{addr1})
					mappings[0].RecipientAddresses = []string{addr1, addr2}
					return mappings
				}(),
			},
			expectErr: false,
		},
		{
			name: "invalid_one_valid_one_invalid_recipient",
			params: Params{
				ClearingAccountMappings: func() []ClearingAccountMapping {
					mappings := createAllClearingAccountMappings([]string{addr1})
					mappings[0].RecipientAddresses = []string{addr1, "invalid"}
					return mappings
				}(),
			},
			expectErr: true,
			errMsg:    "invalid address",
		},
		{
			name: "invalid_community_account_with_mapping",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ClearingAccountCommunity, RecipientAddresses: []string{addr1}},
				},
			},
			expectErr: true,
			errMsg:    "Community clearing account cannot have recipient mappings",
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

// Helper function to create valid mappings for all non-Community PSE clearing accounts.
func createAllClearingAccountMappings(addrs []string) []ClearingAccountMapping {
	nonCommunityAccounts := GetNonCommunityClearingAccounts()
	mappings := make([]ClearingAccountMapping, 0, len(nonCommunityAccounts))

	for i, account := range nonCommunityAccounts {
		// Use modulo to cycle through addresses if not enough provided
		addr := addrs[i%len(addrs)]
		mappings = append(mappings, ClearingAccountMapping{
			ClearingAccount:    account,
			RecipientAddresses: []string{addr},
		})
	}
	return mappings
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
			errMsg:    "missing allocation for required clearing account",
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
			errMsg:    "invalid clearing account",
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
			errMsg:    "missing allocation for required clearing account",
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

			err := ValidateDistributionSchedule(tc.schedule)
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
			name: "valid_custom_module_name_allowed_in_genesis",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: "my_custom_module", RecipientAddresses: []string{addr1}},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_all_pse_modules",
			params: Params{
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr1}),
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
				ExcludedAddresses:       []string{addr1},
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr2, addr3}),
			},
			expectErr: false,
		},
		{
			name: "valid_all_non_community_modules_mapped",
			params: Params{
				ClearingAccountMappings: createAllClearingAccountMappings([]string{addr1, addr2, addr3}),
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
