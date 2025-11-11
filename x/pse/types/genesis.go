package types

import (
	"cosmossdk.io/errors"
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
	if err := m.Params.ValidateBasic(); err != nil {
		return err
	}

	// Validate allocation schedule
	if err := ValidateAllocationSchedule(m.ScheduledDistributions); err != nil {
		return errors.Wrapf(err, "invalid allocation schedule")
	}

	// Validate referential integrity: all clearing accounts in schedule must have mappings
	if err := ValidateScheduleMappingConsistency(m.ScheduledDistributions, m.Params.ClearingAccountMappings); err != nil {
		return errors.Wrapf(err, "invalid allocation schedule mapping consistency")
	}

	return nil
}
