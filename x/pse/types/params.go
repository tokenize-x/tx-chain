package types

import (
	errorsmod "cosmossdk.io/errors"
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

// GetAllClearingAccounts returns all PSE clearing accounts.
func GetAllClearingAccounts() []string {
	return []string{
		ClearingAccountCommunity,
		ClearingAccountFoundation,
		ClearingAccountAlliance,
		ClearingAccountPartnership,
		ClearingAccountInvestors,
		ClearingAccountTeam,
	}
}

// GetNonCommunityClearingAccounts returns all PSE clearing accounts except for Community.
func GetNonCommunityClearingAccounts() []string {
	return lo.Filter(GetAllClearingAccounts(), func(acct string, _ int) bool {
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
			return errorsmod.Wrapf(err, "excluded address %d: invalid address %s", i, addr)
		}

		// Check for duplicates
		if seen[addr] {
			return errorsmod.Wrapf(ErrInvalidParam, "excluded address %d: duplicate address %s", i, addr)
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
			return errorsmod.Wrapf(ErrInvalidParam, "mapping %d: clearing_account cannot be empty", i)
		}

		// Community clearing account should NOT have recipient mappings
		// It uses score-based distribution logic instead
		if mapping.ClearingAccount == ClearingAccountCommunity {
			return errorsmod.Wrapf(ErrInvalidParam, "mapping %d: Community clearing account cannot have recipient mappings", i)
		}

		// Validate that there is at least one recipient address
		if len(mapping.RecipientAddresses) == 0 {
			return errorsmod.Wrapf(ErrInvalidParam, "mapping %d: must have at least one recipient address", i)
		}

		// Validate each recipient address and check for duplicates within the same clearing account
		seenRecipients := make(map[string]bool)
		for j, addr := range mapping.RecipientAddresses {
			// Validate address format
			if _, err := sdk.AccAddressFromBech32(addr); err != nil {
				return errorsmod.Wrapf(err, "mapping %d, recipient %d: invalid address %s", i, j, addr)
			}

			// Check for duplicate recipients within the same clearing account
			if seenRecipients[addr] {
				return errorsmod.Wrapf(ErrInvalidParam, "mapping %d, recipient %d: duplicate recipient address %s", i, j, addr)
			}
			seenRecipients[addr] = true
		}

		// Check for duplicate clearing accounts
		if seenClearingAccounts[mapping.ClearingAccount] {
			return errorsmod.Wrapf(ErrInvalidParam, "mapping %d: duplicate clearing account %s", i, mapping.ClearingAccount)
		}
		seenClearingAccounts[mapping.ClearingAccount] = true
	}

	return nil
}

// ValidateDistributionSchedule validates the allocation schedule.
func ValidateDistributionSchedule(schedule []ScheduledDistribution) error {
	// All clearing accounts (including Community) should be in the schedule
	allClearingAccounts := GetAllClearingAccounts()

	seenTimestamps := make(map[uint64]bool)
	var lastTime uint64

	for i, period := range schedule {
		// Validate timestamp is not zero
		if period.Timestamp == 0 {
			return errorsmod.Wrapf(ErrInvalidParam, "timestamp cannot be zero")
		}

		// Check for duplicate timestamps
		if seenTimestamps[period.Timestamp] {
			return errorsmod.Wrapf(ErrInvalidParam, "duplicate timestamp")
		}
		seenTimestamps[period.Timestamp] = true

		// Validate schedule is sorted in ascending order
		if i > 0 && period.Timestamp <= lastTime {
			return errorsmod.Wrapf(ErrInvalidParam, "periods must be sorted by timestamp in ascending order")
		}
		lastTime = period.Timestamp

		// Validate allocations array is not empty
		if len(period.Allocations) == 0 {
			return errorsmod.Wrapf(ErrInvalidParam, "must have at least one allocation")
		}

		// Validate individual allocations within the period
		// Build allowed clearing accounts set for explicit validation
		allowedClearingAccounts := make(map[string]bool)
		for _, account := range allClearingAccounts {
			allowedClearingAccounts[account] = true
		}

		seenClearingAccounts := make(map[string]bool)
		for j, alloc := range period.Allocations {
			// Validate clearing_account is not empty
			if alloc.ClearingAccount == "" {
				return errorsmod.Wrapf(ErrInvalidParam, "period %d, allocation %d: clearing_account cannot be empty", i, j)
			}

			// Explicitly validate clearing account is one of the PSE clearing accounts
			// Only these accounts are allowed, no other module accounts
			if !allowedClearingAccounts[alloc.ClearingAccount] {
				return errorsmod.Wrapf(ErrInvalidParam,
					"period %d, allocation %d: invalid clearing account '%s'. Only PSE clearing accounts are allowed: %v",
					i, j, alloc.ClearingAccount, allClearingAccounts)
			}

			// Check for duplicate clearing accounts in the same period
			if seenClearingAccounts[alloc.ClearingAccount] {
				return errorsmod.Wrapf(ErrInvalidParam,
					"period %d, allocation %d: duplicate clearing account '%s' in the same period",
					i, j, alloc.ClearingAccount)
			}
			seenClearingAccounts[alloc.ClearingAccount] = true

			// Validate amount is not nil (should be enforced by proto, but double-check)
			if alloc.Amount.IsNil() {
				return errorsmod.Wrapf(ErrInvalidParam,
					"period %d, allocation %d (%s): amount cannot be nil",
					i, j, alloc.ClearingAccount)
			}

			// Validate amount is not negative
			if alloc.Amount.IsNegative() {
				return errorsmod.Wrapf(ErrInvalidParam,
					"period %d, allocation %d (%s): amount cannot be negative",
					i, j, alloc.ClearingAccount)
			}

			// Validate amount is not zero (zero allocations don't make sense)
			if alloc.Amount.IsZero() {
				return errorsmod.Wrapf(ErrInvalidParam,
					"period %d, allocation %d (%s): amount cannot be zero",
					i, j, alloc.ClearingAccount)
			}
		}

		// Explicitly validate that ALL PSE clearing accounts are present in this period
		// Each period must have exactly one allocation for each clearing account
		for _, requiredAccount := range allClearingAccounts {
			if !seenClearingAccounts[requiredAccount] {
				return errorsmod.Wrapf(ErrInvalidParam,
					"period %d (timestamp %d): missing allocation for required clearing account '%s'. "+
						"All clearing accounts must be present",
					i, period.Timestamp, requiredAccount)
			}
		}

		// Verify exact count (should match after all validations)
		if len(period.Allocations) != len(allClearingAccounts) {
			return errorsmod.Wrapf(ErrInvalidParam,
				"period %d (timestamp %d): expected %d allocations (one per clearing account), got %d",
				i, period.Timestamp, len(allClearingAccounts), len(period.Allocations))
		}
	}

	return nil
}
