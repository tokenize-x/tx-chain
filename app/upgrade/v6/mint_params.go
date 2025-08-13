package v6

import (
	"context"

	"cosmossdk.io/math"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
)

func migrateMintParams(ctx context.Context, keeper mintkeeper.Keeper) error {
	params, err := keeper.Params.Get(ctx)
	if err != nil {
		return err
	}
	params.InflationMax = math.LegacyMustNewDecFromStr("0.30")
	params.BlocksPerYear = 30_000_000
	err = keeper.Params.Set(ctx, params)
	if err != nil {
		return err
	}

	minter, err := keeper.Minter.Get(ctx)
	if err != nil {
		return err
	}

	minter.Inflation = math.LegacyMustNewDecFromStr("0.29")
	err = keeper.Minter.Set(ctx, minter)
	if err != nil {
		return err
	}

	return nil
}
