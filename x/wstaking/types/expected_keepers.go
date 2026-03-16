package types

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	customparamstypes "github.com/tokenize-x/tx-chain/v7/x/customparams/types"
)

// CustomParamsKeeper defines the custom params keeper interface required for the module.
type CustomParamsKeeper interface {
	GetStakingParams(ctx sdk.Context) (customparamstypes.StakingParams, error)
}

// StakingKeeper defines the staking keeper interface required for the module.
type StakingKeeper interface {
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
	TotalBondedTokens(ctx context.Context) (math.Int, error)
}
