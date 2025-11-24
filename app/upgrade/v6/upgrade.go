package v6

import (
	"context"

	addresscodec "cosmossdk.io/core/address"
	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	"github.com/tokenize-x/tx-chain/v6/app/upgrade"
	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
	wbankkeeper "github.com/tokenize-x/tx-chain/v6/x/wbank/keeper"
)

// Name defines the upgrade name.
const Name = "v6"

// New makes an upgrade handler for v6 upgrade.
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
			Added: []string{
				psetypes.StoreKey,
			},
			Deleted: []string{
				"feeibc",
				"crisis",
			},
		},
		Upgrade: func(ctx context.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
			vmap, err := mm.RunMigrations(ctx, configurator, vm)
			if err != nil {
				return nil, err
			}

			if err := migrateDenomSymbol(ctx, bankKeeper); err != nil {
				return nil, err
			}

			if err := migrateMintParams(ctx, mintKeeper); err != nil {
				return nil, err
			}

			// Perform PSE initialization: create schedule, mint, and distribute tokens
			if err := InitPSEAllocationsAndSchedule(
				ctx,
				pseKeeper,
				bankKeeper,
				stakingkeeper.NewQuerier(stakingKeeper),
			); err != nil {
				return nil, err
			}

			if err := SnapshotPSEStaking(
				ctx,
				stakingkeeper.NewQuerier(stakingKeeper),
				pseKeeper,
				addressCodec,
				valAddressCodec,
			); err != nil {
				return nil, err
			}
			return vmap, nil
		},
	}
}
