package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// ModuleAccountCommunity is the community module account name.
	ModuleAccountCommunity = "pse_community"
	// ModuleAccountFoundation is the foundation module account name.
	ModuleAccountFoundation = "pse_foundation"
	// ModuleAccountAlliance is the alliance module account name.
	ModuleAccountAlliance = "pse_alliance"
	// ModuleAccountPartnership is the partnership module account name.
	ModuleAccountPartnership = "pse_partnership"
	// ModuleAccountInvestors is the investors module account name.
	ModuleAccountInvestors = "pse_investors"
	// ModuleAccountTeam is the team module account name.
	ModuleAccountTeam = "pse_team"
)

// GetModuleAccountPerms returns the module account permissions for PSE module accounts.
func GetModuleAccountPerms() map[string][]string {
	return map[string][]string{
		ModuleAccountCommunity:   nil,
		ModuleAccountFoundation:  nil,
		ModuleAccountAlliance:    nil,
		ModuleAccountPartnership: nil,
		ModuleAccountInvestors:   nil,
		ModuleAccountTeam:        nil,
	}
}

// IsValidModuleAccountName checks if the given name is one of the allowed module accounts.
func IsValidModuleAccountName(name string) bool {
	_, exists := GetModuleAccountPerms()[name]
	return exists
}

// IsExcludedClearingAccount checks if a clearing account is excluded from recipient distribution.
func IsExcludedClearingAccount(account string) bool {
	return account == ModuleAccountCommunity
}

// DefaultParams returns default pse module parameters.
func DefaultParams() Params {
	return Params{
		ExcludedAddresses:       []string{},
		ClearingAccountMappings: []ClearingAccountMapping{},
	}
}

// ValidateBasic performs basic validation on pse module parameters.
func (p Params) ValidateBasic() error {
	// Validate excluded addresses
	if err := validateExcludedAddresses(p.ExcludedAddresses); err != nil {
		return err
	}

	// Validate sub account mappings
	return validateClearingAccountMappings(p.ClearingAccountMappings)
}

func validateExcludedAddresses(addresses []string) error {
	seen := make(map[string]bool)

	for i, addr := range addresses {
		// Validate address format
		if _, err := sdk.AccAddressFromBech32(addr); err != nil {
			return fmt.Errorf("excluded address %d: invalid address %s: %w", i, addr, err)
		}

		// Check for duplicates
		if seen[addr] {
			return fmt.Errorf("excluded address %d: duplicate address %s", i, addr)
		}
		seen[addr] = true
	}

	return nil
}

func validateClearingAccountMappings(mappings []ClearingAccountMapping) error {
	seenModuleAccounts := make(map[string]bool)

	for i, mapping := range mappings {
		// Validate module_account (module name) is not empty
		if mapping.ClearingAccount == "" {
			return fmt.Errorf("mapping %d: module_account cannot be empty", i)
		}

		// Validate sub account address
		if _, err := sdk.AccAddressFromBech32(mapping.RecipientAddress); err != nil {
			return fmt.Errorf("mapping %d: invalid sub account address: %w", i, err)
		}

		// Check for duplicate module accounts
		if seenModuleAccounts[mapping.ClearingAccount] {
			return fmt.Errorf("mapping %d: duplicate module account %s", i, mapping.ClearingAccount)
		}
		seenModuleAccounts[mapping.ClearingAccount] = true
	}

	return nil
}

// ValidateAllocationSchedule validates the allocation schedule.
func ValidateAllocationSchedule(schedule []ScheduledDistribution) error {
	if len(schedule) == 0 {
		// Empty schedule is valid (e.g., at genesis before initialization)
		return nil
	}

	seenTimestamps := make(map[uint64]bool)
	var lastTime uint64

	for i, period := range schedule {
		// Validate timestamp is not zero
		if period.Timestamp == 0 {
			return fmt.Errorf("period %d: timestamp cannot be zero", i)
		}

		// Check for duplicate timestamps
		if seenTimestamps[period.Timestamp] {
			return fmt.Errorf("period %d: duplicate timestamp %d", i, period.Timestamp)
		}
		seenTimestamps[period.Timestamp] = true

		// Validate schedule is sorted in ascending order
		if i > 0 && period.Timestamp <= lastTime {
			return fmt.Errorf(
				"period %d: periods must be sorted by timestamp in ascending order (got %d after %d)",
				i, period.Timestamp, lastTime,
			)
		}
		lastTime = period.Timestamp

		// Validate allocations array is not empty
		if len(period.Allocations) == 0 {
			return fmt.Errorf("period %d: must have at least one allocation", i)
		}

		// Validate individual allocations within the period
		seenClearingAccounts := make(map[string]bool)
		for j, alloc := range period.Allocations {
			// Validate clearing_account is not empty
			if alloc.ClearingAccount == "" {
				return fmt.Errorf("period %d, allocation %d: clearing_account cannot be empty", i, j)
			}

			// Check for duplicate clearing accounts in the same period
			if seenClearingAccounts[alloc.ClearingAccount] {
				return fmt.Errorf("period %d: duplicate clearing account %s in same period", i, alloc.ClearingAccount)
			}
			seenClearingAccounts[alloc.ClearingAccount] = true

			// Validate amount is not nil (should be enforced by proto, but double-check)
			if alloc.Amount.IsNil() {
				return fmt.Errorf("period %d, allocation %d: amount cannot be nil", i, j)
			}

			// Validate amount is not negative
			if alloc.Amount.IsNegative() {
				return fmt.Errorf("period %d, allocation %d: amount cannot be negative", i, j)
			}

			// Validate amount is not zero (zero allocations don't make sense)
			if alloc.Amount.IsZero() {
				return fmt.Errorf("period %d, allocation %d: amount cannot be zero", i, j)
			}
		}
	}

	return nil
}

// ValidateScheduleMappingConsistency ensures all clearing accounts in the schedule have corresponding mappings.
func ValidateScheduleMappingConsistency(schedule []ScheduledDistribution, mappings []ClearingAccountMapping) error {
	// Build a set of available clearing accounts from mappings
	availableAccounts := make(map[string]bool)
	for _, mapping := range mappings {
		availableAccounts[mapping.ClearingAccount] = true
	}

	// Check that every clearing account in the schedule has a mapping
	for i, period := range schedule {
		for j, alloc := range period.Allocations {
			if !availableAccounts[alloc.ClearingAccount] {
				return fmt.Errorf(
					"period %d, allocation %d: no recipient mapping found for clearing account '%s'",
					i, j, alloc.ClearingAccount,
				)
			}
		}
	}

	return nil
}
