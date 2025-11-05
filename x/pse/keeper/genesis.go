package keeper

import (
	"context"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	// Load completed distributions
	for _, completedDist := range genState.CompletedDistributions {
		key := types.MakeCompletedDistributionKey(completedDist.ModuleAccount, int64(completedDist.ScheduledTime))
		if err := k.CompletedDistributions.Set(ctx, key, completedDist); err != nil {
			panic(err)
		}
	}

	// Rebuild pending queue from schedule + completed distributions for consistency
	// Don't trust provided PendingDistributionTimestamps
	if err := k.rebuildPendingQueue(ctx); err != nil {
		panic(err)
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

	// Normalize nil slices to empty slices for consistent comparison
	if genesis.Params.ExcludedAddresses == nil {
		genesis.Params.ExcludedAddresses = []string{}
	}
	if genesis.Params.SubAccountMappings == nil {
		genesis.Params.SubAccountMappings = []types.SubAccountMapping{}
	}
	if genesis.Params.DistributionSchedule == nil {
		genesis.Params.DistributionSchedule = []types.DistributionPeriod{}
	}

	genesis.CompletedDistributions, err = k.GetCompletedDistributions(ctx)
	if err != nil {
		return nil, err
	}
	// Normalize nil slice to empty slice
	if genesis.CompletedDistributions == nil {
		genesis.CompletedDistributions = []types.CompletedDistribution{}
	}

	// Export pending timestamps
	iter, err := k.PendingTimestamps.Iterate(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		timestamp, err := iter.Key()
		if err != nil {
			return nil, err
		}
		genesis.PendingDistributionTimestamps = append(genesis.PendingDistributionTimestamps, timestamp)
	}
	// Normalize nil slice to empty slice
	if genesis.PendingDistributionTimestamps == nil {
		genesis.PendingDistributionTimestamps = []uint64{}
	}

	return genesis, nil
}
