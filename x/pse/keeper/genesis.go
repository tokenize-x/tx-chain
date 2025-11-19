package keeper

import (
	"context"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	// Validate genesis state (includes mapping consistency check)
	if err := genState.Validate(); err != nil {
		return err
	}

	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	// Populate allocation schedule from genesis state
	for _, scheduledDist := range genState.ScheduledDistributions {
		if err := k.AllocationSchedule.Set(ctx, scheduledDist.Timestamp, scheduledDist); err != nil {
			return err
		}
	}

	return nil
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	var err error

	genesis := types.DefaultGenesisState()
	genesis.Params, err = k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Export allocation schedule using keeper method (already sorted by timestamp)
	genesis.ScheduledDistributions, err = k.GetDistributionSchedule(ctx)
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
