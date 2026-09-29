package v8

import (
	"context"

	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"

	"github.com/tokenize-x/tx-chain/v8/app/upgrade"
	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	psekeeper "github.com/tokenize-x/tx-chain/v8/x/pse/keeper"
	wbankkeeper "github.com/tokenize-x/tx-chain/v8/x/wbank/keeper"
)

// Name defines the upgrade name.
const Name = "v8"

// New makes an upgrade handler for v8 upgrade.
func New(
	mm *module.Manager,
	configurator module.Configurator,
	bankKeeper wbankkeeper.BaseKeeperWrapper,
	pseKeeper psekeeper.Keeper,
	mintKeeper mintkeeper.Keeper,
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

			// Mainnet proposal 46: postpone PSE by one year after the November 2026 distribution, and pin inflation meanwhile.
			if err := PostponePSEDistributions(ctx, pseKeeper, PSEPostponeCutoff); err != nil {
				return nil, err
			}
			if err := SetPSEPauseMintParams(ctx, mintKeeper); err != nil {
				return nil, err
			}

			return mm.RunMigrations(ctx, configurator, vm)
		},
	}
}
