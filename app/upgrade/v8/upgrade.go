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
	pauseKeepers PSEPauseKeepers,
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
			if err := PostponePSEDistributions(ctx, pauseKeepers.PSE, PSEPostponeCutoff); err != nil {
				return nil, err
			}
			// The pause mint params are normally set by the EndBlocker when the November distribution completes.
			// If that already happened, set them here.
			lastProcessed, err := LastProcessedPSEDistributionID(ctx, pauseKeepers.PSE)
			if err != nil {
				return nil, err
			}
			last, err := isLastPrePauseDistribution(ctx, pauseKeepers.PSE, lastProcessed)
			if err != nil {
				return nil, err
			}
			if last {
				if err := SetPSEPauseMintParams(ctx, pauseKeepers); err != nil {
					return nil, err
				}
			}

			return mm.RunMigrations(ctx, configurator, vm)
		},
	}
}
