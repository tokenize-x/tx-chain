package v6

import (
	"context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// MinCommissionRate is the minimum commission rate for validators (5%).
var MinCommissionRate = sdkmath.LegacyNewDecWithPrec(5, 2) // 5 * 10^(-2) = 0.05 = 5%

// MigrateValidatorCommission sets the minimum commission rate to 5% and updates
// validators with commission rates below the minimum to have their commission
// rate set to the minimum commission rate.
func MigrateValidatorCommission(ctx context.Context, stakingKeeper *stakingkeeper.Keeper) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Set minimum commission rate in staking params
	params, err := stakingKeeper.GetParams(ctx)
	if err != nil {
		return err
	}
	params.MinCommissionRate = MinCommissionRate
	if err := stakingKeeper.SetParams(ctx, params); err != nil {
		return err
	}

	// Update validators with commission rates below the minimum
	var updateErr error
	err = stakingKeeper.IterateValidators(ctx, func(_ int64, val stakingtypes.ValidatorI) (stop bool) {
		validator, ok := val.(stakingtypes.Validator)
		if !ok {
			return false
		}

		if validator.Commission.Rate.LT(MinCommissionRate) {
			validator.Commission.Rate = MinCommissionRate

			if validator.Commission.MaxRate.LT(MinCommissionRate) {
				validator.Commission.MaxRate = MinCommissionRate
			}

			validator.Commission.UpdateTime = sdkCtx.BlockTime()

			if updateErr = stakingKeeper.SetValidator(ctx, validator); updateErr != nil {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}

	return updateErr
}
