package v6patch1

import (
	"context"

	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/tokenize-x/tx-chain/v6/app/upgrade"
	wbankkeeper "github.com/tokenize-x/tx-chain/v6/x/wbank/keeper"
)

// Name defines the upgrade name.
const Name = "v6patch1"

// New makes an upgrade handler for v6patch1 upgrade.
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
			if err := MigrateDenomMetadata(ctx, bankKeeper); err != nil {
				return nil, err
			}

			return mm.RunMigrations(ctx, configurator, vm)
		},
	}
}
