package v6

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/CoreumFoundation/coreum/v6/pkg/config/constant"
	wbankkeeper "github.com/CoreumFoundation/coreum/v6/x/wbank/keeper"
)

func migrateDenomSymbol(ctx context.Context, bankKeeper wbankkeeper.BaseKeeperWrapper) error {
	var denom string

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	prefix := ""
	if sdkCtx.ChainID() == string(constant.ChainIDTest) {
		prefix = "test"
	} else if sdkCtx.ChainID() == string(constant.ChainIDDev) {
		prefix = "dev"
	}

	meta, found := bankKeeper.GetDenomMetaData(ctx, denom)
	if !found {
		return fmt.Errorf("denom metadata not found for %s", denom)
	}

	meta.Description = strings.ReplaceAll(meta.Description, "core", "tx")
	meta.Display = strings.ReplaceAll(meta.Display, "core", "tx")
	meta.Symbol = strings.ReplaceAll(meta.Symbol, "core", "tx")

	meta.Description = prefix + "tx coin"      // "devcore coin" -> "devtx coin"
	meta.Display = prefix + "tx"               // "devcore" -> "devtx"
	meta.Symbol = fmt.Sprintf("u%stx", prefix) // "udevcore" -> "udevtx"

	bankKeeper.SetDenomMetaData(ctx, meta)

	return nil
}
