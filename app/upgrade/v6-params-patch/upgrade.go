package v6paramspatch

import (
	"context"

	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"

	"github.com/tokenize-x/tx-chain/v6/app/upgrade"
	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
)

// Name defines the upgrade name.
const Name = "v6-params-patch"

// New makes an upgrade handler for v6 params patch upgrade.
func New(
	mm *module.Manager,
	configurator module.Configurator,
	mintKeeper mintkeeper.Keeper,
	pseKeeper pskeeper.Keeper,
) upgrade.Upgrade {
	return upgrade.Upgrade{
		Name: Name,
		StoreUpgrades: store.StoreUpgrades{
			Added:   []string{},
			Deleted: []string{},
		},
		Upgrade: func(ctx context.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
			vmap, err := mm.RunMigrations(ctx, configurator, vm)
			if err != nil {
				return nil, err
			}

			// Set mint module params to defaults
			if err := migrateMintParams(ctx, mintKeeper); err != nil {
				return nil, err
			}

			// Set PSE module params to testnet defaults
			if err := TestnetV6Patch(ctx, pseKeeper); err != nil {
				return nil, err
			}

			return vmap, nil
		},
	}
}
