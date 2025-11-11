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
						ClearingAccount:  ModuleAccountFoundation,
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
					{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
					{ClearingAccount: ModuleAccountTeam, RecipientAddress: addr2},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_empty_module_account",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: "", RecipientAddress: addr1},
				},
			},
			expectErr: true,
			errMsg:    "module_account cannot be empty",
		},
		{
			name: "invalid_malformed_sub_account_address",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ModuleAccountFoundation, RecipientAddress: "invalid"},
				},
			},
			expectErr: true,
			errMsg:    "invalid sub account address",
		},
		{
			name: "invalid_duplicate_module_account",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
					{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr2},
				},
			},
			expectErr: true,
			errMsg:    "duplicate module account",
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

// Helper function to create distribution schedule allocations for all eligible PSE module accounts.
// These are used in ScheduledDistribution periods to define how tokens are allocated across
// clearing accounts. Note: Community is excluded as it's not eligible for distribution.
func createDistributionScheduleAllocations(amount sdkmath.Int) []ClearingAccountAllocation {
	return []ClearingAccountAllocation{
		{ClearingAccount: ModuleAccountFoundation, Amount: amount},
		{ClearingAccount: ModuleAccountAlliance, Amount: amount},
		{ClearingAccount: ModuleAccountPartnership, Amount: amount},
		{ClearingAccount: ModuleAccountInvestors, Amount: amount},
		{ClearingAccount: ModuleAccountTeam, Amount: amount},
	}
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
					Allocations: createDistributionScheduleAllocations(sdkmath.NewInt(1000)),
				},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_periods_sorted",
			schedule: []ScheduledDistribution{
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createDistributionScheduleAllocations(sdkmath.NewInt(1000)),
				},
				{
					Timestamp:   getTestTimestamp(12),
					Allocations: createDistributionScheduleAllocations(sdkmath.NewInt(2000)),
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_with_excluded_community_account",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						// Community (excluded) should NOT be in schedule
						{ClearingAccount: ModuleAccountCommunity, Amount: sdkmath.NewInt(5000)},
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "is not an eligible PSE module account",
		},
		{
			name: "invalid_zero_timestamp",
			schedule: []ScheduledDistribution{
				{
					Timestamp: 0,
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
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
					Allocations: createDistributionScheduleAllocations(sdkmath.NewInt(1000)),
				},
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createDistributionScheduleAllocations(sdkmath.NewInt(2000)),
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
					Allocations: createDistributionScheduleAllocations(sdkmath.NewInt(2000)),
				},
				{
					Timestamp:   getTestTimestamp(0),
					Allocations: createDistributionScheduleAllocations(sdkmath.NewInt(1000)),
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
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "missing allocation for required eligible PSE module account",
		},
		{
			name: "invalid_empty_clearing_account",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: "", Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
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
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "is not an eligible PSE module account",
		},
		{
			name: "invalid_duplicate_clearing_account_in_period",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(2000)}, // Duplicate
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "duplicate clearing account",
		},
		{
			name: "invalid_missing_module_account",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						// Missing ModuleAccountTeam
					},
				},
			},
			expectErr: true,
			errMsg:    "missing allocation for required eligible PSE module account",
		},
		{
			name: "invalid_nil_amount",
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.Int{}},
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
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
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(-1000)},
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
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
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.ZeroInt()},
						{ClearingAccount: ModuleAccountAlliance, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountInvestors, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
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
				{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
				{ClearingAccount: ModuleAccountTeam, RecipientAddress: addr2},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(2000)},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_empty_schedule",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
			},
			schedule:  []ScheduledDistribution{},
			expectErr: false,
		},
		{
			name: "valid_extra_mappings_not_in_schedule",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
				{ClearingAccount: ModuleAccountTeam, RecipientAddress: addr2},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_schedule_without_mapping",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "no recipient mapping found for clearing account 'pse_team'",
		},
		{
			name: "invalid_multiple_modules_one_missing_mapping",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
						{ClearingAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(2000)},
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
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(1000)},
					},
				},
			},
			expectErr: true,
			errMsg:    "no recipient mapping found for clearing account 'pse_foundation'",
		},
		{
			name: "valid_community_excluded_no_mapping_required",
			mappings: []ClearingAccountMapping{
				{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
			},
			schedule: []ScheduledDistribution{
				{
					Timestamp: getTestTimestamp(0),
					Allocations: []ClearingAccountAllocation{
						{ClearingAccount: ModuleAccountCommunity, Amount: sdkmath.NewInt(1000)},  // Excluded - no mapping needed
						{ClearingAccount: ModuleAccountFoundation, Amount: sdkmath.NewInt(2000)}, // Needs mapping
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

func TestParamsValidation_ModuleAccountNames(t *testing.T) {
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
					{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
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
			name: "invalid_bech32_address_as_module_account_in_mapping",
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
					{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr2},
					{ClearingAccount: ModuleAccountTeam, RecipientAddress: addr3},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_modules_all_mapped",
			params: Params{
				ClearingAccountMappings: []ClearingAccountMapping{
					{ClearingAccount: ModuleAccountFoundation, RecipientAddress: addr1},
					{ClearingAccount: ModuleAccountPartnership, RecipientAddress: addr2},
					{ClearingAccount: ModuleAccountTeam, RecipientAddress: addr3},
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
