package v8

import (
	"context"

	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/tokenize-x/tx-chain/v8/app/upgrade"
	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	wbankkeeper "github.com/tokenize-x/tx-chain/v8/x/wbank/keeper"
)

// Name defines the upgrade name.
const Name = "v8"

// New makes an upgrade handler for v8 upgrade.
func New(
	mm *module.Manager,
	configurator module.Configurator,
	bankKeeper wbankkeeper.BaseKeeperWrapper,
) upgrade.Upgrade {
	return upgrade.Upgrade{
		Name: Name,
		StoreUpgrades: store.StoreUpgrades{
			Added:   []string{},
			Deleted: []string{},
		},
		Upgrade: func(ctx context.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
			chainID := constant.ChainID(sdk.UnwrapSDKContext(ctx).ChainID())
			ClawbackFrozenFunds(ctx, bankKeeper, ClawbackTransfers[chainID])

			return mm.RunMigrations(ctx, configurator, vm)
		},
	}
}
