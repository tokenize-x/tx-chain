package v7

import (
	"context"

	addresscodec "cosmossdk.io/core/address"
	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	"github.com/tokenize-x/tx-chain/v7/app/upgrade"
	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	wbankkeeper "github.com/tokenize-x/tx-chain/v7/x/wbank/keeper"
)

// Name defines the upgrade name.
const Name = "v7"

// New makes an upgrade handler for v7 upgrade.
func New(
	mm *module.Manager,
	configurator module.Configurator,
	bankKeeper wbankkeeper.BaseKeeperWrapper,
	mintKeeper mintkeeper.Keeper,
	stakingKeeper *stakingkeeper.Keeper,
	pseKeeper pskeeper.Keeper,
	addressCodec addresscodec.Codec,
	valAddressCodec addresscodec.Codec,
) upgrade.Upgrade {
	return upgrade.Upgrade{
		Name: Name,
		StoreUpgrades: store.StoreUpgrades{
			Added:   []string{},
			Deleted: []string{},
		},
		Upgrade: func(ctx context.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
			return mm.RunMigrations(ctx, configurator, vm)
		},
	}
}
