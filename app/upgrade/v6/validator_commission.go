package v6

import (
	"context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// MigrateValidatorCommission updates validators with commission rates below the minimum
// to have their commission rate set to the minimum commission rate.
func MigrateValidatorCommission(ctx context.Context, stakingKeeper *stakingkeeper.Keeper) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	params, err := stakingKeeper.GetParams(ctx)
	if err != nil {
		return err
	}

	minCommissionRate := params.MinCommissionRate

	var validatorsToUpdate []stakingtypes.Validator

	// Iterate through all validators to find those with commission below minimum
	err = stakingKeeper.IterateValidators(ctx, func(_ int64, val stakingtypes.ValidatorI) (stop bool) {
		validator, ok := val.(stakingtypes.Validator)
		if !ok {
			return false
		}

		if validator.Commission.Rate.LT(minCommissionRate) {
			validatorsToUpdate = append(validatorsToUpdate, validator)
		}
		return false
	})
	if err != nil {
		return err
	}

	// Update validators with commission below minimum
	for _, validator := range validatorsToUpdate {
		validator.Commission.Rate = minCommissionRate

		if validator.Commission.MaxRate.LT(minCommissionRate) {
			validator.Commission.MaxRate = minCommissionRate
		}

		// Update the commission update time
		validator.Commission.UpdateTime = sdkCtx.BlockTime()

		if err := stakingKeeper.SetValidator(ctx, validator); err != nil {
			return err
		}
	}

	return nil
}

// SetMinCommissionRate is a helper function to set the minimum commission rate
// in staking params during upgrade if needed.
func SetMinCommissionRate(ctx context.Context, stakingKeeper *stakingkeeper.Keeper, minRate sdkmath.LegacyDec) error {
	params, err := stakingKeeper.GetParams(ctx)
	if err != nil {
		return err
	}

	params.MinCommissionRate = minRate

	return stakingKeeper.SetParams(ctx, params)
}
