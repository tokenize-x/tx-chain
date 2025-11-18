package types

import (
	errorsmod "cosmossdk.io/errors"
)

// DefaultGenesisState returns genesis state with default values.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params:                 DefaultParams(),
		ScheduledDistributions: []ScheduledDistribution{},
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

	return nil
}
