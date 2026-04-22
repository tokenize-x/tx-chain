// Package v7patch1 is the testnet upgrade that recovers the PSE state stuck by
// the v7 migration bug: it restores TotalScore for the ongoing distribution
// and clears the disabled flag so the next EndBlock resumes distribution.
package v7patch1

import (
	"context"

	addresscodec "cosmossdk.io/core/address"
	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"github.com/tokenize-x/tx-chain/v7/app/upgrade"
	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
)

// Name must match the governance proposal's plan name.
const Name = "v7patch1"

// New returns the v7patch1 upgrade handler.
func New(
	mm *module.Manager,
	configurator module.Configurator,
	pseKeeper pskeeper.Keeper,
	addressCodec addresscodec.Codec,
) upgrade.Upgrade {
	return upgrade.Upgrade{
		Name: Name,
		StoreUpgrades: store.StoreUpgrades{
			Added:   []string{},
			Deleted: []string{},
		},
		Upgrade: func(ctx context.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
			if err := recoverOngoingDistribution(ctx, pseKeeper, addressCodec); err != nil {
				return nil, err
			}
			return mm.RunMigrations(ctx, configurator, vm)
		},
	}
}
