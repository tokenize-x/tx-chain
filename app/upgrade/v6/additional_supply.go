package v6

import (
	"context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	wbankkeeper "github.com/tokenize-x/tx-chain/v6/x/wbank/keeper"
)

const (
	// TxSupplyForSOLOHolders is the additional supply amount minted for SOLO holders.
	TxSupplyForSOLOHolders = 914_205_182_000_000
	// TxSupplyForBinance is the additional supply amount minted for BINANCE holders.
	TxSupplyForBinance = 114_949_808_000_000
)

// TODO: replace with actual addresses.
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
		string(constant.ChainIDDev):  "devcore1dk2ger49pmqcw062hl09typhjrhxq392qd4rah",
	}[sdkCtx.ChainID()]
	soloHoldersAcc, err := sdk.AccAddressFromBech32(soloHoldersAddress)
	if err != nil {
		return err
	}

	coinsSOLO := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(TxSupplyForSOLOHolders)))
	if err := bankKeeper.MintCoins(ctx, minttypes.ModuleName, coinsSOLO); err != nil {
		return err
	}
	if err := bankKeeper.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, soloHoldersAcc, coinsSOLO); err != nil {
		return err
	}

	// BINANCE
	binanceAddress := map[string]string{
		string(constant.ChainIDMain): "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		string(constant.ChainIDTest): "testcore19gcp0mkgml3l9pmm269000f6kxacpus0x5ru9pg95tt37dxjx0ksd30rx9",
		string(constant.ChainIDDev):  "devcore1za98kfjq6pma30l5u2x9pu6w9castcs934xyw6",
	}[sdkCtx.ChainID()]
	binanceAcc, err := sdk.AccAddressFromBech32(binanceAddress)
	if err != nil {
		return err
	}

	coinsBinance := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(TxSupplyForBinance)))
	if err := bankKeeper.MintCoins(ctx, minttypes.ModuleName, coinsBinance); err != nil {
		return err
	}
	return bankKeeper.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, binanceAcc, coinsBinance)
}
