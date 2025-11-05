package types

import (
	"cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultGenesisState returns genesis state with default values.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params:                        DefaultParams(),
		CompletedDistributions:        []CompletedDistribution{},
		PendingDistributionTimestamps: []uint64{},
	}
}

// Validate validates genesis parameters.
func (m *GenesisState) Validate() error {
	if err := m.Params.ValidateBasic(); err != nil {
		return err
	}

	// Validate distribution schedule in params
	if err := validateDistributionSchedule(m.Params.DistributionSchedule); err != nil {
		return errors.Wrapf(err, "invalid distribution schedule")
	}

	// Validate completed distributions
	if err := validateCompletedDistributions(m.CompletedDistributions); err != nil {
		return errors.Wrapf(err, "invalid completed distributions")
	}

	return nil
}

func validateCompletedDistributions(distributions []CompletedDistribution) error {
	if len(distributions) == 0 {
		// Empty list is valid
		return nil
	}

	for i, dist := range distributions {
		// Validate module_account (module name) is not empty
		if dist.ModuleAccount == "" {
			return errors.Wrapf(ErrInvalidInput, "completed distribution %d: module_account cannot be empty", i)
		}

		// Validate sub account
		if _, err := sdk.AccAddressFromBech32(dist.SubAccount); err != nil {
			return errors.Wrapf(err, "completed distribution %d: invalid sub account", i)
		}

		// Validate timestamps
		if dist.ScheduledTime == 0 {
			return errors.Wrapf(ErrInvalidInput, "completed distribution %d: scheduled_time cannot be zero", i)
		}
		if dist.ActualDistributionTime == 0 {
			return errors.Wrapf(ErrInvalidInput, "completed distribution %d: actual_distribution_time cannot be zero", i)
		}

		// Validate amount is not nil
		if dist.Amount.IsNil() {
			return errors.Wrapf(ErrInvalidInput, "completed distribution %d: amount cannot be nil", i)
		}

		// Validate amount is not negative
		if dist.Amount.IsNegative() {
			return errors.Wrapf(ErrInvalidInput, "completed distribution %d: amount cannot be negative", i)
		}

		// Validate amount is not zero (a completed distribution should have transferred something)
		if dist.Amount.IsZero() {
			return errors.Wrapf(ErrInvalidInput, "completed distribution %d: amount cannot be zero", i)
		}

		// Validate block height
		if dist.BlockHeight <= 0 {
			return errors.Wrapf(ErrInvalidInput, "completed distribution %d: block_height must be positive", i)
		}
	}

	return nil
}
