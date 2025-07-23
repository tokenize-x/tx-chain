package v6

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/CoreumFoundation/coreum/v6/pkg/config/constant"
	wbankkeeper "github.com/CoreumFoundation/coreum/v6/x/wbank/keeper"
)

func migrateDenomSymbol(ctx context.Context, bankKeeper wbankkeeper.BaseKeeperWrapper) error {
	var denom string
	var prefix string

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	switch sdkCtx.ChainID() {
	case string(constant.ChainIDMain):
		denom = constant.DenomMain
		prefix = ""
	case string(constant.ChainIDTest):
		denom = constant.DenomTest
		prefix = "test"
	case string(constant.ChainIDDev):
		denom = constant.DenomDev
		prefix = "dev"
	default:
		return fmt.Errorf("unknown chain id: %s", sdkCtx.ChainID())
	}

	meta, found := bankKeeper.GetDenomMetaData(ctx, denom)
	if !found {
		return fmt.Errorf("denom metadata not found for %s", denom)
	}

	meta.Description = prefix + "tx coin"      // "devcore coin" -> "devtx coin"
	meta.Display = prefix + "tx"               // "devcore" -> "devtx"
	meta.Symbol = fmt.Sprintf("u%stx", prefix) // "udevcore" -> "udevtx"

	bankKeeper.SetDenomMetaData(ctx, meta)

	return nil
}
