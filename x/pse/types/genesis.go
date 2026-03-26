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
		ExcludedAddressScores:  []ExcludedAddressScoreEntry{},
		DistributionsDisabled:  false,
	}
}

// Validate validates genesis parameters.
func (m *GenesisState) Validate() error {
	// Validate params (includes clearing account mappings validation)
	if err := m.Params.ValidateBasic(); err != nil {
		return err
	}

	// Validate only unprocessed entries for ordering and gap constraints.
	var unprocessed []ScheduledDistribution
	for _, sd := range m.ScheduledDistributions {
		if sd.ID > m.LastProcessedDistributionID {
			unprocessed = append(unprocessed, sd)
		}
	}

	if err := ValidateDistributionSchedule(unprocessed); err != nil {
		return errorsmod.Wrapf(err, "invalid allocation schedule")
	}

	if err := ValidateDistributionGap(unprocessed, m.Params.MinDistributionGapSeconds); err != nil {
		return errorsmod.Wrapf(err, "invalid distribution gap")
	}

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

	// Validate excluded address scores
	for _, entry := range m.ExcludedAddressScores {
		if entry.Address == "" {
			return errorsmod.Wrapf(ErrInvalidInput, "excluded address cannot be empty")
		}
		if entry.Score.IsNil() {
			return errorsmod.Wrapf(ErrInvalidInput, "excluded address score cannot be nil")
		}
		if entry.Score.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidInput, "excluded address score cannot be negative")
		}
	}

	return nil
}
