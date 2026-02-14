package types

import (
	errorsmod "cosmossdk.io/errors"
)

// DefaultGenesisState returns genesis state with default values.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params:                       DefaultParams(),
		ScheduledDistributions:       []ScheduledDistribution{},
		DelegationTimeEntries:        []DelegationTimeEntryExport{},
		AccountScores:                []AccountScore{},
		CommunityDistributionEntries: []CommunityDistributionEntry{},
		DistributionsDisabled:        false,
	}
}

// Validate validates genesis parameters.
func (m *GenesisState) Validate() error {
	// Validate params (includes clearing account mappings validation)
	if err := m.Params.ValidateBasic(); err != nil {
		return err
	}

	// Validate allocation schedule (includes all 6 clearing accounts validation)
	if err := ValidateDistributionSchedule(m.ScheduledDistributions); err != nil {
		return errorsmod.Wrapf(err, "invalid allocation schedule")
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

	// Validate community distribution entries
	for _, entry := range m.CommunityDistributionEntries {
		if entry.DelegatorAddress == "" {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution entry address cannot be empty")
		}
		if entry.Score.IsNil() {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution entry score cannot be nil")
		}
		if entry.Score.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution entry score cannot be negative")
		}
	}

	if m.CommunityDistributionJob != nil {
		job := m.CommunityDistributionJob
		if job.ScheduledAt == 0 {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution job scheduled_at cannot be zero")
		}
		if job.TotalAmount.IsNil() || job.TotalAmount.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution job total_amount must be non-negative")
		}
		if job.TotalScore.IsNil() || job.TotalScore.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution job total_score must be non-negative")
		}
		if job.Leftover.IsNil() || job.Leftover.IsNegative() {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution job leftover must be non-negative")
		}
		if job.ProcessedEntries > job.TotalEntries {
			return errorsmod.Wrapf(ErrInvalidInput, "community distribution job processed_entries exceeds total_entries")
		}
	}

	return nil
}
