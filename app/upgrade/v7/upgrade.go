package v7

import (
	"context"

	addresscodec "cosmossdk.io/core/address"
	store "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	"github.com/tokenize-x/tx-chain/v7/app/upgrade"
	customparamskeeper "github.com/tokenize-x/tx-chain/v7/x/customparams/keeper"
	customparamstypes "github.com/tokenize-x/tx-chain/v7/x/customparams/types"
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
	customParamsKeeper customparamskeeper.Keeper,
) upgrade.Upgrade {
	return upgrade.Upgrade{
		Name: Name,
		StoreUpgrades: store.StoreUpgrades{
			Added:   []string{},
			Deleted: []string{},
		},
		Upgrade: func(ctx context.Context, _ upgradetypes.Plan, vm module.VersionMap) (module.VersionMap, error) {
			if err := migratePSEStore(ctx, pseKeeper); err != nil {
				return nil, err
			}

			if err := migrateCustomParams(ctx, customParamsKeeper); err != nil {
				return nil, err
			}

			return mm.RunMigrations(ctx, configurator, vm)
		},
	}
}

// migrateCustomParams adds the MaxVotingPower field to existing StakingParams.
func migrateCustomParams(ctx context.Context, cpk customparamskeeper.Keeper) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	params, err := cpk.GetStakingParams(sdkCtx)
	if err != nil {
		return err
	}

	// Set default MaxVotingPower (1.0 = unrestricted) for existing chains
	params.MaxVotingPower = customparamstypes.DefaultMaxVotingPower

	return cpk.SetStakingParams(sdkCtx, params)
}
