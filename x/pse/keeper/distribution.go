package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// ProcessNextDistribution processes the next due distribution from the schedule.
// Checks the earliest scheduled distribution and processes it if the current block time has passed its timestamp.
// Only one distribution is processed per call. Should be called from EndBlock.
func (k Keeper) ProcessNextDistribution(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Peek at the next scheduled distribution
	scheduledDistribution, shouldProcess, err := k.PeekNextAllocationSchedule(ctx)
	if err != nil {
		return err
	}

	// Return early if schedule is empty or not ready to process
	if !shouldProcess {
		return nil
	}

	timestamp := scheduledDistribution.Timestamp

	// Get bond denom from staking params
	//nolint:contextcheck // this is correct context passing
	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	// Get params containing clearing account to recipient address mappings
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Process all allocations scheduled for this timestamp
	if err := k.distributeAllocatedTokens(ctx, timestamp, bondDenom, params.ClearingAccountMappings, scheduledDistribution); err != nil {
		return err
	}

	// Remove the completed distribution from the schedule
	if err := k.AllocationSchedule.Remove(ctx, timestamp); err != nil {
		return err
	}

	sdkCtx.Logger().Info("processed and removed allocation from schedule",
		"timestamp", timestamp)

	return nil
}

// PeekNextAllocationSchedule returns the earliest scheduled distribution and whether it should be processed.
func (k Keeper) PeekNextAllocationSchedule(ctx context.Context) (types.ScheduledDistribution, bool, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Get iterator for the allocation schedule (sorted by timestamp ascending)
	iter, err := k.AllocationSchedule.Iterate(ctx, nil)
	if err != nil {
		return types.ScheduledDistribution{}, false, err
	}
	defer iter.Close()

	// Return early if schedule is empty
	if !iter.Valid() {
		return types.ScheduledDistribution{}, false, nil
	}

	// Extract the earliest scheduled distribution
	kv, err := iter.KeyValue()
	if err != nil {
		return types.ScheduledDistribution{}, false, err
	}

	timestamp := kv.Key
	scheduledDist := kv.Value

	// Check if distribution time has arrived
	// Since the map is sorted by timestamp, if the first item is in the future, all items are
	currentTime := uint64(sdkCtx.BlockTime().Unix())
	shouldProcess := timestamp <= currentTime

	return scheduledDist, shouldProcess, nil
}

// distributeAllocatedTokens transfers tokens from clearing accounts to their mapped recipients.
// Processes all allocations within a single scheduled distribution.
// Any transfer failure indicates a state invariant violation (insufficient balance or invalid recipient).
func (k Keeper) distributeAllocatedTokens(
	ctx context.Context,
	timestamp uint64,
	bondDenom string,
	clearingAccountMappings []types.ClearingAccountMapping,
	scheduledDistribution types.ScheduledDistribution,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Transfer tokens for each allocation in this distribution period
	for _, allocation := range scheduledDistribution.Allocations {
		// Skip excluded clearing accounts - tokens remain in module account for alternative distribution
		if types.IsExcludedForAllocation(allocation.ClearingAccount) {
			sdkCtx.Logger().Info("skipping excluded clearing account distribution",
				"clearing_account", allocation.ClearingAccount,
				"amount", allocation.Amount.String(),
			)
			continue
		}

		// Find the recipient address mapped to this clearing account
		// Note: Excluded clearing accounts (like Community) are skipped above and don't need mappings.
		// Mappings are validated on update and genesis, so we can assume they exist for non-excluded accounts.
		var recipientAddr string
		for _, mapping := range clearingAccountMappings {
			if mapping.ClearingAccount == allocation.ClearingAccount {
				recipientAddr = mapping.RecipientAddress
				break
			}
		}

		// Convert recipient address string to SDK account address
		recipient := sdk.MustAccAddressFromBech32(recipientAddr)

		// Prepare coins to transfer
		coinsToSend := sdk.NewCoins(sdk.NewCoin(bondDenom, allocation.Amount))

		// Transfer tokens from clearing account to recipient
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(
			ctx,
			allocation.ClearingAccount,
			recipient,
			coinsToSend,
		); err != nil {
			return errorsmod.Wrapf(
				types.ErrTransferFailed,
				"failed to transfer from clearing account '%s': %v",
				allocation.ClearingAccount,
				err,
			)
		}

		// Emit allocation completed event
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventAllocationDistributed{
			ClearingAccount:  allocation.ClearingAccount,
			RecipientAddress: recipientAddr,
			ScheduledAt:      timestamp,
			Amount:           allocation.Amount,
		}); err != nil {
			sdkCtx.Logger().Error("failed to emit allocation completed event", "error", err)
		}

		sdkCtx.Logger().Info("allocated tokens",
			"clearing_account", allocation.ClearingAccount,
			"recipient", recipientAddr,
			"amount", allocation.Amount.String())
	}

	return nil
}

// SaveDistributionSchedule persists the distribution schedule to blockchain state.
// Each scheduled distribution is stored in the AllocationSchedule map, indexed by its timestamp.
func (k Keeper) SaveDistributionSchedule(ctx context.Context, schedule []types.ScheduledDistribution) error {
	for _, scheduledDist := range schedule {
		if err := k.AllocationSchedule.Set(ctx, scheduledDist.Timestamp, scheduledDist); err != nil {
			return errorsmod.Wrapf(err, "failed to save distribution at timestamp %d", scheduledDist.Timestamp)
		}
	}
	return nil
}

// GetAllocationSchedule returns the complete allocation schedule as a sorted list.
// The schedule is sorted by timestamp in ascending order.
// Returns an empty slice if no allocations are scheduled.
func (k Keeper) GetAllocationSchedule(ctx context.Context) ([]types.ScheduledDistribution, error) {
	var schedule []types.ScheduledDistribution

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
		schedule = append(schedule, kv.Value)
	}

	// Note: Collections map iterates in ascending order of keys (timestamps),
	// so the schedule is already sorted. No need to sort again.
	return schedule, nil
}
