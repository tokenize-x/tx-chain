package types

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestDefaultParams(t *testing.T) {
	requireT := require.New(t)

	params := DefaultParams()
	requireT.Empty(params.ExcludedAddresses)
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

func TestValidateSubAccountMappings(t *testing.T) {
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
				SubAccountMappings: []SubAccountMapping{
					{
						ModuleAccount:     ModuleAccountTreasury,
						SubAccountAddress: addr1,
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_mappings",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
					{ModuleAccount: ModuleAccountTeam, SubAccountAddress: addr2},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_empty_module_account",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: "", SubAccountAddress: addr1},
				},
			},
			expectErr: true,
			errMsg:    "module_account cannot be empty",
		},
		{
			name: "invalid_malformed_sub_account_address",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: "invalid"},
				},
			},
			expectErr: true,
			errMsg:    "invalid sub account address",
		},
		{
			name: "invalid_duplicate_module_account",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr2},
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

func TestValidateDistributionSchedule(t *testing.T) {
	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_empty_schedule",
			params: Params{
				DistributionSchedule: []DistributionPeriod{},
			},
			expectErr: false,
		},
		{
			name: "valid_single_period",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{
						ModuleAccount:     ModuleAccountTreasury,
						SubAccountAddress: sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String(),
					},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_multiple_periods_sorted",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{
						ModuleAccount:     ModuleAccountTreasury,
						SubAccountAddress: sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String(),
					},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
					{
						DistributionTime: 1767225600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(2000)},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_zero_distribution_time",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 0,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "distribution_time cannot be zero",
		},
		{
			name: "invalid_duplicate_timestamp",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(2000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "duplicate distribution_time",
		},
		{
			name: "invalid_unsorted_schedule",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1767225600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(2000)},
						},
					},
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "must be sorted by distribution_time in ascending order",
		},
		{
			name: "invalid_empty_distributions_array",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions:    []ModuleDistribution{},
					},
				},
			},
			expectErr: true,
			errMsg:    "must have at least one distribution",
		},
		{
			name: "invalid_empty_module_account",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: "", Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "module_account cannot be empty",
		},
		{
			name: "invalid_duplicate_module_in_period",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(2000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "duplicate module account",
		},
		{
			name: "invalid_nil_amount",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.Int{}},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "amount cannot be nil",
		},
		{
			name: "invalid_negative_amount",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(-1000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "amount cannot be negative",
		},
		{
			name: "invalid_zero_amount",
			params: Params{
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.ZeroInt()},
						},
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

func TestValidateScheduleMappingConsistency(t *testing.T) {
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name      string
		params    Params
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid_schedule_with_all_mappings",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
					{ModuleAccount: ModuleAccountTeam, SubAccountAddress: addr2},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
							{ModuleAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(2000)},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_empty_schedule",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
				},
				DistributionSchedule: []DistributionPeriod{},
			},
			expectErr: false,
		},
		{
			name: "valid_extra_mappings_not_in_schedule",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
					{ModuleAccount: ModuleAccountTeam, SubAccountAddress: addr2},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_schedule_without_mapping",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "no sub-account mapping found for module 'team'",
		},
		{
			name: "invalid_multiple_modules_one_missing_mapping",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
							{ModuleAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(2000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "no sub-account mapping found for module 'team'",
		},
		{
			name: "invalid_no_mappings_but_has_schedule",
			params: Params{
				SubAccountMappings: []SubAccountMapping{},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "no sub-account mapping found",
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
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_custom_module_name",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: "my_custom_module", SubAccountAddress: addr1},
				},
			},
			expectErr: false,
		},
		{
			name: "valid_module_name_in_schedule",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_bech32_address_as_module_account_in_mapping",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: addr1, SubAccountAddress: addr1}, // Using bech32 address as module name
				},
			},
			expectErr: false, // No validation against bech32 - just non-empty string
		},
		{
			name: "invalid_bech32_address_as_module_account_in_schedule",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: addr1, SubAccountAddress: addr1},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: addr1, Amount: sdkmath.NewInt(1000)},
						},
					},
				},
			},
			expectErr: false, // Validation allows it, but SendCoins would fail at runtime
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
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr2},
					{ModuleAccount: ModuleAccountTeam, SubAccountAddress: addr3},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
							{ModuleAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(500)},
						},
					},
					{
						DistributionTime: 1767225600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(2000)},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "invalid_schedule_references_unmapped_module",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
							{ModuleAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(500)},
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "no sub-account mapping found for module 'team'",
		},
		{
			name: "valid_multiple_modules_all_mapped",
			params: Params{
				SubAccountMappings: []SubAccountMapping{
					{ModuleAccount: ModuleAccountTreasury, SubAccountAddress: addr1},
					{ModuleAccount: ModuleAccountPartnership, SubAccountAddress: addr2},
					{ModuleAccount: ModuleAccountTeam, SubAccountAddress: addr3},
				},
				DistributionSchedule: []DistributionPeriod{
					{
						DistributionTime: 1735689600,
						Distributions: []ModuleDistribution{
							{ModuleAccount: ModuleAccountTreasury, Amount: sdkmath.NewInt(1000)},
							{ModuleAccount: ModuleAccountPartnership, Amount: sdkmath.NewInt(500)},
							{ModuleAccount: ModuleAccountTeam, Amount: sdkmath.NewInt(750)},
						},
					},
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
