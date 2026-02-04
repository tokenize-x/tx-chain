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
	// txSupplyForSOLOHolders is the additional supply amount minted for SOLO holders.
	txSupplyForSOLOHolders = 914_205_182_000_000
	txSupplyForBinance     = 114_949_808_000_000
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
		string(constant.ChainIDMain): "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		string(constant.ChainIDTest): "testcore1c6y9nwvu9jxx468qx3js620zq34c6hnpg9qgqu8rz3krjrxqmk9s5vzxkj",
		string(constant.ChainIDDev):  "devcore17we2jgjyxexcz8rg29dn622axt7s9l263fl0zt",
	}[sdkCtx.ChainID()]
	soloHoldersAcc, err := sdk.AccAddressFromBech32(soloHoldersAddress)
	if err != nil {
		return err
	}

	coinsSOLO := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(txSupplyForSOLOHolders)))
	if err := bankKeeper.MintCoins(ctx, banktypes.ModuleName, coinsSOLO); err != nil {
		return err
	}
	if err := bankKeeper.SendCoinsFromModuleToAccount(ctx, banktypes.ModuleName, soloHoldersAcc, coinsSOLO); err != nil {
		return err
	}

	// BINANCE
	binanceAddress := map[string]string{
		string(constant.ChainIDMain): "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		string(constant.ChainIDTest): "testcore19gcp0mkgml3l9pmm269000f6kxacpus0x5ru9pg95tt37dxjx0ksd30rx9",
		string(constant.ChainIDDev):  "devcore17we2jgjyxexcz8rg29dn622axt7s9l263fl0zt",
	}[sdkCtx.ChainID()]
	binanceAcc, err := sdk.AccAddressFromBech32(binanceAddress)
	if err != nil {
		return err
	}

	coinsBinance := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(txSupplyForBinance)))
	if err := bankKeeper.MintCoins(ctx, banktypes.ModuleName, coinsBinance); err != nil {
		return err
	}
	return bankKeeper.SendCoinsFromModuleToAccount(ctx, banktypes.ModuleName, binanceAcc, coinsBinance)
}
