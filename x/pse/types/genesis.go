package types

import (
	errorsmod "cosmossdk.io/errors"
)

// DefaultGenesisState returns genesis state with default values.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params:                 DefaultParams(),
		ScheduledDistributions: []ScheduledDistribution{},
		DelegationTimeEntries:  []DelegationTimeEntryExport{},
		AccountScores:          []AccountScore{},
	}
}

// Validate validates genesis parameters.
func (m *GenesisState) Validate() error {
	// Validate params (includes clearing account mappings validation)
	if err := m.Params.ValidateBasic(); err != nil {
		return err
	}

	// Validate allocation schedule (includes all 6 clearing accounts validation)
	if err := ValidateAllocationSchedule(m.ScheduledDistributions); err != nil {
		return errorsmod.Wrapf(err, "invalid allocation schedule")
	}

	// Note: No need for ValidateScheduleMappingConsistency since:
	// - Params validation ensures all 5 non-Community accounts have mappings
	// - Schedule validation ensures all 6 accounts are in schedule
	// - Therefore, all non-Community accounts in schedule automatically have mappings

	// Validate delegation time entries
	for _, delegationTimeEntry := range m.DelegationTimeEntries {
		if delegationTimeEntry.ValidatorAddress == "" {
			return errorsmod.Wrapf(ErrInvalidInput, "validator address cannot be empty")
		}
		if delegationTimeEntry.DelegatorAddress == "" {
			return errorsmod.Wrapf(ErrInvalidInput, "delegator address cannot be empty")
		}
		if delegationTimeEntry.Shares.IsNil() {
			return errorsmod.Wrapf(ErrInvalidInput, "shares cannot be nil")
		}
		if delegationTimeEntry.Shares.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidInput, "shares cannot be negative")
		}
		if delegationTimeEntry.LastChangedUnixSec <= 0 {
			return errorsmod.Wrapf(ErrInvalidInput, "last changed unix sec cannot be less than or equal to zero")
		}
	}

	// Validate account scores
	for _, accountScore := range m.AccountScores {
		if accountScore.Address == "" {
			return errorsmod.Wrapf(ErrInvalidInput, "address cannot be empty")
		}
		if accountScore.Score.IsNil() {
			return errorsmod.Wrapf(ErrInvalidInput, "score cannot be nil")
		}
		if accountScore.Score.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidInput, "score cannot be negative")
		}
	}

	return nil
}
