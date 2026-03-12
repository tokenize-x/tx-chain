package v6patch1

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	wbankkeeper "github.com/tokenize-x/tx-chain/v6/x/wbank/keeper"
)

// MigrateDenomMetadata fixes the DenomUnits that were missed in the v6 migration.
func MigrateDenomMetadata(ctx context.Context, bankKeeper wbankkeeper.BaseKeeperWrapper) error {
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

	displayDenom := prefix + "tx"
	for i, unit := range meta.DenomUnits {
		if unit.Exponent == 6 {
			meta.DenomUnits[i].Denom = displayDenom
		}
	}

	bankKeeper.SetDenomMetaData(ctx, meta)

	return nil
}
