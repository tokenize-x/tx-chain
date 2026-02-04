package v6

import (
	"context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	wbankkeeper "github.com/tokenize-x/tx-chain/v6/x/wbank/keeper"
)

const (
	TXSupplyForSOLOHolders = 914_205_182_000_000
	TXSupplyForBinance     = 114_949_808_000_000
)

func mintAdditionalSupply(
	ctx context.Context,
	bankKeeper wbankkeeper.BaseKeeperWrapper,
	stakingKeeper *stakingkeeper.Keeper,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	// SOLO HOLDERS
	soloHoldersAddress := map[string]string{
		string(constant.ChainIDMain): "TODO",
		string(constant.ChainIDTest): "testcore1c6y9nwvu9jxx468qx3js620zq34c6hnpg9qgqu8rz3krjrxqmk9s5vzxkj",
		string(constant.ChainIDDev):  "TODO",
	}[sdkCtx.ChainID()]
	soloHoldersAcc, err := sdk.AccAddressFromBech32(soloHoldersAddress)
	if err != nil {
		return err
	}
	txSupplyForSOLOHolders := sdkmath.NewInt(TXSupplyForSOLOHolders)
	if err := bankKeeper.MintCoins(ctx, banktypes.ModuleName, sdk.NewCoins(sdk.NewCoin(bondDenom, txSupplyForSOLOHolders))); err != nil {
		return err
	}
	if err := bankKeeper.SendCoinsFromModuleToAccount(ctx, banktypes.ModuleName, soloHoldersAcc, sdk.NewCoins(sdk.NewCoin(bondDenom, txSupplyForSOLOHolders))); err != nil {
		return err
	}

	// BINANCE
	binanceAddress := map[string]string{
		string(constant.ChainIDMain): "TODO",
		string(constant.ChainIDTest): "testcore19gcp0mkgml3l9pmm269000f6kxacpus0x5ru9pg95tt37dxjx0ksd30rx9",
		string(constant.ChainIDDev):  "TODO",
	}[sdkCtx.ChainID()]

	binanceAcc, err := sdk.AccAddressFromBech32(binanceAddress)
	if err != nil {
		return err
	}
	txSupplyForBinance := sdkmath.NewInt(TXSupplyForBinance)
	if err := bankKeeper.MintCoins(ctx, banktypes.ModuleName, sdk.NewCoins(sdk.NewCoin(bondDenom, txSupplyForBinance))); err != nil {
		return err
	}
	if err := bankKeeper.SendCoinsFromModuleToAccount(ctx, banktypes.ModuleName, binanceAcc, sdk.NewCoins(sdk.NewCoin(bondDenom, txSupplyForBinance))); err != nil {
		return err
	}

	return nil
}
