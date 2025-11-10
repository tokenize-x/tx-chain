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

	// Get bond denom from staking params
	//nolint:contextcheck // this is correct context passing
	bondDenom, err := k.GetBondDenom(sdkCtx)
	if err != nil {
		return err
	}

	// Get params containing clearing account to recipient address mappings
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Get iterator for the allocation schedule (sorted by timestamp ascending)
	iter, err := k.AllocationSchedule.Iterate(ctx, nil)
	if err != nil {
		return err
	}

	// Return early if schedule is empty
	if !iter.Valid() {
		iter.Close()
		return nil
	}

	// Extract the earliest scheduled distribution
	kv, err := iter.KeyValue()
	iter.Close()
	if err != nil {
		return err
	}

	timestamp := kv.Key

	// Skip if distribution time has not yet arrived
	// Since the map is sorted by timestamp, if the first item is in the future, all items are
	if timestamp > uint64(sdkCtx.BlockTime().Unix()) {
		return nil
	}

	// Process all allocations scheduled for this timestamp
	if err := k.distributeAllocatedTokens(ctx, timestamp, bondDenom, params.ClearingAccountMappings); err != nil {
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

// distributeAllocatedTokens transfers tokens from clearing accounts to their mapped recipients.
// Processes all allocations within a single scheduled distribution.
// Any transfer failure indicates a state invariant violation (insufficient balance or invalid recipient).
func (k Keeper) distributeAllocatedTokens(
	ctx context.Context,
	timestamp uint64,
	bondDenom string,
	clearingAccountMappings []types.ClearingAccountMapping,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Retrieve the scheduled distribution for this timestamp
	scheduledDistribution, err := k.AllocationSchedule.Get(ctx, timestamp)
	if err != nil {
		return errorsmod.Wrapf(err, "allocation schedule not found for timestamp %d", timestamp)
	}

	// Transfer tokens for each allocation in this distribution period
	for _, allocation := range scheduledDistribution.Allocations {
		// Skip excluded clearing accounts - tokens remain in module account for alternative distribution
		if types.IsExcludedClearingAccount(allocation.ClearingAccount) {
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
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventAllocationCompleted{
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
