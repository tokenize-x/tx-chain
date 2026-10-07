package v2

import (
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v8/x/customparams/types"
)

// Keeper specifies methods of the keeper required by the migration.
type Keeper interface {
	GetStakingParams(ctx sdk.Context) (types.StakingParams, error)
	SetStakingParams(ctx sdk.Context, params types.StakingParams) error
}

// MigrateParams sets max_voting_power to 1, which keeps the cap disabled.
func MigrateParams(ctx sdk.Context, keeper Keeper) error {
	params, err := keeper.GetStakingParams(ctx)
	if err != nil {
		return err
	}
	params.MaxVotingPower = sdkmath.LegacyOneDec()

	return keeper.SetStakingParams(ctx, params)
}
