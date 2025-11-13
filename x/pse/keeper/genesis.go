package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
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

	// Populate delegation time entries from genesis state
	for _, delegationTimeEntryExported := range genState.DelegationTimeEntries {
		valAddr, err := k.valAddressCodec.StringToBytes(delegationTimeEntryExported.ValidatorAddress)
		if err != nil {
			return err
		}
		delAddr, err := k.addressCodec.StringToBytes(delegationTimeEntryExported.DelegatorAddress)
		if err != nil {
			return err
		}
		if err = k.SetDelegationTimeEntry(ctx, valAddr, delAddr, types.DelegationTimeEntry{
			Shares:             delegationTimeEntryExported.Shares,
			LastChangedUnixSec: delegationTimeEntryExported.LastChangedUnixSec,
		}); err != nil {
			return err
		}
	}

	// Populate account scores from genesis state
	for _, accountScore := range genState.AccountScores {
		addr, err := k.addressCodec.StringToBytes(accountScore.Address)
		if err != nil {
			return err
		}
		if err := k.AccountScoreSnapshot.Set(ctx, addr, accountScore.Score); err != nil {
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
	genesis.ScheduledDistributions, err = k.GetAllocationSchedule(ctx)
	if err != nil {
		return nil, err
	}

	// Export delegation time entries from genesis state
	delegationTimeEntriesExported := make([]types.DelegationTimeEntryExport, 0)
	err = k.DelegationTimeEntries.Walk(ctx, nil,
		func(key collections.Pair[sdk.ValAddress, sdk.AccAddress], value types.DelegationTimeEntry) (stop bool, err error) {
			valAddr, err := k.valAddressCodec.BytesToString(key.K1())
			if err != nil {
				return false, err
			}
			delAddr, err := k.addressCodec.BytesToString(key.K2())
			if err != nil {
				return false, err
			}
			delegationTimeEntriesExported = append(delegationTimeEntriesExported, types.DelegationTimeEntryExport{
				ValidatorAddress:   valAddr,
				DelegatorAddress:   delAddr,
				Shares:             value.Shares,
				LastChangedUnixSec: value.LastChangedUnixSec,
			})
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	genesis.DelegationTimeEntries = delegationTimeEntriesExported

	// Export account scores from genesis state
	err = k.AccountScoreSnapshot.Walk(ctx, nil,
		func(key sdk.AccAddress, value sdkmath.Int) (stop bool, err error) {
			addr, err := k.addressCodec.BytesToString(key)
			if err != nil {
				return false, err
			}
			genesis.AccountScores = append(genesis.AccountScores, types.AccountScore{
				Address: addr,
				Score:   value,
			})
			return false, nil
		})

	return genesis, nil
}
