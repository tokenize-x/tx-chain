package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// ModuleAccountTreasury is the treasury module account name.
	ModuleAccountTreasury = "treasury"
	// ModuleAccountPartnership is the partnership module account name.
	ModuleAccountPartnership = "partnership"
	// ModuleAccountFoundingPartner is the founding partner module account name.
	ModuleAccountFoundingPartner = "founding_partner"
	// ModuleAccountTeam is the team module account name.
	ModuleAccountTeam = "team"
	// ModuleAccountInvestors is the investors module account name.
	ModuleAccountInvestors = "investors"
)

// GetModuleAccountPerms returns the module account permissions for PSE module accounts.
func GetModuleAccountPerms() map[string][]string {
	return map[string][]string{
		ModuleAccountTreasury:        nil,
		ModuleAccountPartnership:     nil,
		ModuleAccountFoundingPartner: nil,
		ModuleAccountTeam:            nil,
		ModuleAccountInvestors:       nil,
	}
}

// GetAllowedModuleAccounts returns a list of all allowed module account names.
func GetAllowedModuleAccounts() []string {
	accounts := make([]string, 0, len(GetModuleAccountPerms()))
	for name := range GetModuleAccountPerms() {
		accounts = append(accounts, name)
	}
	return accounts
}

// IsValidModuleAccountName checks if the given name is one of the allowed module accounts.
func IsValidModuleAccountName(name string) bool {
	_, exists := GetModuleAccountPerms()[name]
	return exists
}

// DefaultParams returns default pse module parameters.
func DefaultParams() Params {
	return Params{
		ExcludedAddresses:    []string{},
		SubAccountMappings:   []SubAccountMapping{},
		DistributionSchedule: []DistributionPeriod{},
	}
}

// ValidateBasic performs basic validation on pse module parameters.
func (p Params) ValidateBasic() error {
	// Validate excluded addresses
	if err := validateExcludedAddresses(p.ExcludedAddresses); err != nil {
		return err
	}

	// Validate sub account mappings
	if err := validateSubAccountMappings(p.SubAccountMappings); err != nil {
		return err
	}

	// Validate distribution schedule
	if err := validateDistributionSchedule(p.DistributionSchedule); err != nil {
		return err
	}

	// Validate referential integrity: all module names in schedule must have mappings
	return validateScheduleMappingConsistency(p.DistributionSchedule, p.SubAccountMappings)
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

func validateSubAccountMappings(mappings []SubAccountMapping) error {
	seenModuleAccounts := make(map[string]bool)

	for i, mapping := range mappings {
		// Validate module_account (module name) is not empty
		if mapping.ModuleAccount == "" {
			return fmt.Errorf("mapping %d: module_account cannot be empty", i)
		}

		// Validate sub account address
		if _, err := sdk.AccAddressFromBech32(mapping.SubAccountAddress); err != nil {
			return fmt.Errorf("mapping %d: invalid sub account address: %w", i, err)
		}

		// Check for duplicate module accounts
		if seenModuleAccounts[mapping.ModuleAccount] {
			return fmt.Errorf("mapping %d: duplicate module account %s", i, mapping.ModuleAccount)
		}
		seenModuleAccounts[mapping.ModuleAccount] = true
	}

	return nil
}

func validateDistributionSchedule(schedule []DistributionPeriod) error {
	if len(schedule) == 0 {
		// Empty schedule is valid (e.g., at genesis before bootstrap)
		return nil
	}

	seenTimestamps := make(map[uint64]bool)
	var lastTime uint64

	for i, period := range schedule {
		// Validate distribution time is not zero
		if period.DistributionTime == 0 {
			return fmt.Errorf("period %d: distribution_time cannot be zero", i)
		}

		// Check for duplicate timestamps
		if seenTimestamps[period.DistributionTime] {
			return fmt.Errorf("period %d: duplicate distribution_time %d", i, period.DistributionTime)
		}
		seenTimestamps[period.DistributionTime] = true

		// Validate schedule is sorted in ascending order
		if i > 0 && period.DistributionTime <= lastTime {
			return fmt.Errorf(
				"period %d: periods must be sorted by distribution_time in ascending order (got %d after %d)",
				i, period.DistributionTime, lastTime,
			)
		}
		lastTime = period.DistributionTime

		// Validate distributions array is not empty
		if len(period.Distributions) == 0 {
			return fmt.Errorf("period %d: must have at least one distribution", i)
		}

		// Validate individual distributions within the period
		seenModuleAccounts := make(map[string]bool)
		for j, dist := range period.Distributions {
			// Validate module_account (module name) is not empty
			if dist.ModuleAccount == "" {
				return fmt.Errorf("period %d, distribution %d: module_account cannot be empty", i, j)
			}

			// Check for duplicate module accounts in the same period
			if seenModuleAccounts[dist.ModuleAccount] {
				return fmt.Errorf("period %d: duplicate module account %s in same period", i, dist.ModuleAccount)
			}
			seenModuleAccounts[dist.ModuleAccount] = true

			// Validate amount is not nil (should be enforced by proto, but double-check)
			if dist.Amount.IsNil() {
				return fmt.Errorf("period %d, distribution %d: amount cannot be nil", i, j)
			}

			// Validate amount is not negative
			if dist.Amount.IsNegative() {
				return fmt.Errorf("period %d, distribution %d: amount cannot be negative", i, j)
			}

			// Validate amount is not zero (zero distributions don't make sense)
			if dist.Amount.IsZero() {
				return fmt.Errorf("period %d, distribution %d: amount cannot be zero", i, j)
			}
		}
	}

	return nil
}

// validateScheduleMappingConsistency ensures all module accounts in the schedule have corresponding mappings.
func validateScheduleMappingConsistency(schedule []DistributionPeriod, mappings []SubAccountMapping) error {
	// Build a set of available module accounts from mappings
	availableModules := make(map[string]bool)
	for _, mapping := range mappings {
		availableModules[mapping.ModuleAccount] = true
	}

	// Check that every module in the schedule has a mapping
	for i, period := range schedule {
		for j, dist := range period.Distributions {
			if !availableModules[dist.ModuleAccount] {
				return fmt.Errorf(
					"period %d, distribution %d: no sub-account mapping found for module '%s'",
					i, j, dist.ModuleAccount,
				)
			}
		}
	}

	return nil
}
