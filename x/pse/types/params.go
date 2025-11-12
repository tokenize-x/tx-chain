package types

import (
	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/samber/lo"
)

const (
	// ClearingAccountCommunity is the community clearing account name.
	ClearingAccountCommunity = "pse_community"
	// ClearingAccountFoundation is the foundation clearing account name.
	ClearingAccountFoundation = "pse_foundation"
	// ClearingAccountAlliance is the alliance clearing account name.
	ClearingAccountAlliance = "pse_alliance"
	// ClearingAccountPartnership is the partnership clearing account name.
	ClearingAccountPartnership = "pse_partnership"
	// ClearingAccountInvestors is the investors clearing account name.
	ClearingAccountInvestors = "pse_investors"
	// ClearingAccountTeam is the team clearing account name.
	ClearingAccountTeam = "pse_team"
)

// GetClearingAccountPerms returns the clearing account permissions for PSE clearing accounts.
func GetClearingAccountPerms() map[string][]string {
	return map[string][]string{
		ClearingAccountCommunity:   nil,
		ClearingAccountFoundation:  nil,
		ClearingAccountAlliance:    nil,
		ClearingAccountPartnership: nil,
		ClearingAccountInvestors:   nil,
		ClearingAccountTeam:        nil,
	}
}

// GetNonCommunityClearingAccounts returns the clearing accounts except for Community.
func GetNonCommunityClearingAccounts() []string {
	accounts := lo.Keys(GetClearingAccountPerms())
	return lo.Filter(accounts, func(acct string, _ int) bool {
		return acct != ClearingAccountCommunity
	})
}

// DefaultParams returns default pse clearing account parameters.
func DefaultParams() Params {
	return Params{
		ExcludedAddresses:       []string{},
		ClearingAccountMappings: []ClearingAccountMapping{},
	}
}

// ValidateBasic performs basic validation on pse clearing account parameters.
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
			return errors.Wrapf(err, "excluded address %d: invalid address %s", i, addr)
		}

		// Check for duplicates
		if seen[addr] {
			return errors.Wrapf(ErrInvalidParam, "excluded address %d: duplicate address %s", i, addr)
		}
		seen[addr] = true
	}

	return nil
}

func validateClearingAccountMappings(mappings []ClearingAccountMapping) error {
	seenClearingAccounts := make(map[string]bool)

	for i, mapping := range mappings {
		// Validate clearing_account (clearing account name) is not empty
		if mapping.ClearingAccount == "" {
			return errors.Wrapf(ErrInvalidParam, "mapping %d: clearing_account cannot be empty", i)
		}

		// Validate sub account address
		if _, err := sdk.AccAddressFromBech32(mapping.RecipientAddress); err != nil {
			return errors.Wrapf(err, "mapping %d: invalid sub account address", i)
		}

		// Check for duplicate clearing accounts
		if seenClearingAccounts[mapping.ClearingAccount] {
			return errors.Wrapf(ErrInvalidParam, "mapping %d: duplicate clearing account %s", i, mapping.ClearingAccount)
		}
		seenClearingAccounts[mapping.ClearingAccount] = true
	}

	return nil
}

// ValidateAllocationSchedule validates the allocation schedule.
func ValidateAllocationSchedule(schedule []ScheduledDistribution) error {
	if len(schedule) == 0 {
		// Empty schedule is valid (e.g., at genesis before initialization)
		return nil
	}

	// Only non-Community clearing accounts should be in the schedule
	nonCommunityClearingAccounts := GetNonCommunityClearingAccounts()

	seenTimestamps := make(map[uint64]bool)
	var lastTime uint64

	for i, period := range schedule {
		// Validate timestamp is not zero
		if period.Timestamp == 0 {
			return errors.Wrapf(ErrInvalidParam, "timestamp cannot be zero")
		}

		// Check for duplicate timestamps
		if seenTimestamps[period.Timestamp] {
			return errors.Wrapf(ErrInvalidParam, "duplicate timestamp")
		}
		seenTimestamps[period.Timestamp] = true

		// Validate schedule is sorted in ascending order
		if i > 0 && period.Timestamp <= lastTime {
			return errors.Wrapf(ErrInvalidParam, "periods must be sorted by timestamp in ascending order")
		}
		lastTime = period.Timestamp

		// Validate allocations array is not empty
		if len(period.Allocations) == 0 {
			return errors.Wrapf(ErrInvalidParam, "must have at least one allocation")
		}

		// Validate individual allocations within the period
		seenClearingAccounts := make(map[string]bool)
		for _, alloc := range period.Allocations {
			// Validate clearing_account is not empty
			if alloc.ClearingAccount == "" {
				return errors.Wrapf(ErrInvalidParam, "clearing_account cannot be empty")
			}

			// Validate clearing account is one of the non-Community PSE clearing accounts
			// Excluded accounts (like Community) should NOT be in the schedule
			if !lo.Contains(nonCommunityClearingAccounts, alloc.ClearingAccount) {
				return errors.Wrapf(ErrInvalidParam, "non-Community clearing account not found")
			}

			// Check for duplicate clearing accounts in the same period
			if seenClearingAccounts[alloc.ClearingAccount] {
				return errors.Wrapf(ErrInvalidParam, "duplicate clearing account in the same period")
			}
			seenClearingAccounts[alloc.ClearingAccount] = true

			// Validate amount is not nil (should be enforced by proto, but double-check)
			if alloc.Amount.IsNil() {
				return errors.Wrapf(ErrInvalidParam, "amount cannot be nil")
			}

			// Validate amount is not negative
			if alloc.Amount.IsNegative() {
				return errors.Wrapf(ErrInvalidParam, "amount cannot be negative")
			}

			// Validate amount is not zero (zero allocations don't make sense)
			if alloc.Amount.IsZero() {
				return errors.Wrapf(ErrInvalidParam, "amount cannot be zero")
			}
		}

		// Validate that all non-Community clearing accounts are present in this period
		for _, expectedAccount := range nonCommunityClearingAccounts {
			if !seenClearingAccounts[expectedAccount] {
				return errors.Wrapf(ErrInvalidParam, "missing allocation for required non-Community PSE clearing account")
			}
		}
	}

	return nil
}

// ValidateScheduleMappingConsistency ensures all clearing accounts in the schedule have corresponding mappings.
// Excluded clearing accounts (like Community) don't need mappings since they don't distribute to recipients.
func ValidateScheduleMappingConsistency(schedule []ScheduledDistribution, mappings []ClearingAccountMapping) error {
	// Build a set of available clearing accounts from mappings
	availableAccounts := make(map[string]bool)
	for _, mapping := range mappings {
		availableAccounts[mapping.ClearingAccount] = true
	}

	// Check that every clearing account in the schedule has a mapping
	// Excluded clearing accounts don't need mappings
	for i, period := range schedule {
		for j, alloc := range period.Allocations {
			// Skip Community clearing account - it doesn't need recipient mappings
			if alloc.ClearingAccount == ClearingAccountCommunity {
				continue
			}
			if !availableAccounts[alloc.ClearingAccount] {
				return errors.Wrapf(
					ErrInvalidParam,
					"period %d, allocation %d: no recipient mapping found for clearing account '%s'",
					i, j, alloc.ClearingAccount,
				)
			}
		}
	}

	return nil
}
