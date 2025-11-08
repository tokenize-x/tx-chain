package keeper

import (
	"context"
	"sort"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	// Clear any existing allocation schedule
	iter, err := k.AllocationSchedule.Iterate(ctx, nil)
	if err != nil {
		panic(err)
	}
	var timestampsToRemove []uint64
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			iter.Close()
			panic(err)
		}
		timestampsToRemove = append(timestampsToRemove, kv.Key)
	}
	iter.Close()
	for _, ts := range timestampsToRemove {
		if err := k.AllocationSchedule.Remove(ctx, ts); err != nil {
			panic(err)
		}
	}

	// Populate allocation schedule from genesis state
	for _, scheduledDist := range genState.ScheduledDistributions {
		if err := k.AllocationSchedule.Set(ctx, scheduledDist.Timestamp, scheduledDist); err != nil {
			panic(err)
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

	// Normalize nil slices to empty slices for consistent comparison
	if genesis.Params.ExcludedAddresses == nil {
		genesis.Params.ExcludedAddresses = []string{}
	}
	if genesis.Params.ClearingAccountMappings == nil {
		genesis.Params.ClearingAccountMappings = []types.ClearingAccountMapping{}
	}

	// Export allocation schedule from map to sorted list
	var allocationSchedule []types.ScheduledDistribution
	iter, err := k.AllocationSchedule.Iterate(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return nil, err
		}
		allocationSchedule = append(allocationSchedule, kv.Value)
	}

	// Sort by timestamp in ascending order
	sort.Slice(allocationSchedule, func(i, j int) bool {
		return allocationSchedule[i].Timestamp < allocationSchedule[j].Timestamp
	})

	genesis.ScheduledDistributions = allocationSchedule

	// Normalize nil slice to empty slice
	if genesis.ScheduledDistributions == nil {
		genesis.ScheduledDistributions = []types.ScheduledDistribution{}
	}

	return genesis, nil
}
