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
	params.InflationRateChange = math.LegacyMustNewDecFromStr("0.04")
	params.BlocksPerYear = 33_000_000 // 60*60*24*365/0.95 = 33 195 789. Considering upgrade halts, we can use 33M.
	err = keeper.Params.Set(ctx, params)
	if err != nil {
		return err
	}

	minter, err := keeper.Minter.Get(ctx)
	if err != nil {
		return err
	}

	minter.Inflation = math.LegacyMustNewDecFromStr("0.001")
	err = keeper.Minter.Set(ctx, minter)
	if err != nil {
		return err
	}

	return nil
}
