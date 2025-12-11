package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
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
	if err := k.distributeAllocatedTokens(
		ctx, timestamp, bondDenom, params.ClearingAccountMappings, scheduledDistribution,
	); err != nil {
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
	shouldProcess := timestamp <= uint64(sdkCtx.BlockTime().Unix())

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
		if allocation.Amount.IsZero() {
			continue
		}

		// Community clearing account has different distribution logic
		if allocation.ClearingAccount == types.ClearingAccountCommunity {
			if err := k.DistributeCommunityPSE(ctx, bondDenom, allocation.Amount, scheduledDistribution.Timestamp); err != nil {
				return errorsmod.Wrapf(
					types.ErrTransferFailed,
					"failed to distribute Community clearing account allocation: %v",
					err,
				)
			}
			continue
		}

		// Find the recipient addresses mapped to this clearing account
		// Note: Community clearing account is handled above and doesn't need a mapping.
		// Mappings are validated on update and genesis, so they are guaranteed to exist.
		var recipientAddrs []string
		for _, mapping := range clearingAccountMappings {
			if mapping.ClearingAccount == allocation.ClearingAccount {
				recipientAddrs = mapping.RecipientAddresses
				break
			}
		}

		// Distribution Precision Handling:
		// The allocation amount is split equally among all recipients using integer division.
		// Any remainder from division is sent to the community pool to ensure:
		// - Each recipient receives exactly: allocation.Amount / numRecipients (base amount)
		// - Remainder (if any) goes to community pool for ecosystem benefit
		// This guarantees fair distribution and no tokens are lost
		numRecipients := sdkmath.NewInt(int64(len(recipientAddrs)))
		if numRecipients.IsZero() {
			return errorsmod.Wrapf(
				types.ErrTransferFailed,
				"no recipients found for clearing account '%s'",
				allocation.ClearingAccount,
			)
		}
		amountPerRecipient := allocation.Amount.Quo(numRecipients)
		remainder := allocation.Amount.Mod(numRecipients)

		// Transfer tokens to each recipient
		for _, recipientAddr := range recipientAddrs {
			// Convert recipient address string to SDK account address
			// Safe to use Must* because addresses are validated at genesis/update time
			recipient := sdk.MustAccAddressFromBech32(recipientAddr)

			// Each recipient gets equal base amount
			coinsToSend := sdk.NewCoins(sdk.NewCoin(bondDenom, amountPerRecipient))

			// Transfer tokens from clearing account to recipient
			if err := k.bankKeeper.SendCoinsFromModuleToAccount(
				ctx,
				allocation.ClearingAccount,
				recipient,
				coinsToSend,
			); err != nil {
				return errorsmod.Wrapf(
					types.ErrTransferFailed,
					"failed to transfer from clearing account '%s' to recipient '%s': %v",
					allocation.ClearingAccount,
					recipientAddr,
					err,
				)
			}
		}

		// Send any remainder to community pool
		if !remainder.IsZero() {
			clearingAccountAddr := k.accountKeeper.GetModuleAddress(allocation.ClearingAccount)
			remainderCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, remainder))
			if err := k.distributionKeeper.FundCommunityPool(ctx, remainderCoins, clearingAccountAddr); err != nil {
				return errorsmod.Wrapf(
					types.ErrTransferFailed,
					"failed to send remainder to community pool from clearing account '%s': %v",
					allocation.ClearingAccount,
					err,
				)
			}

			sdkCtx.Logger().Info("sent distribution remainder to community pool",
				"clearing_account", allocation.ClearingAccount,
				"remainder", remainder.String())
		}

		// Emit single allocation completed event with recipient list, per-recipient amount, and community pool amount
		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventAllocationDistributed{
			ClearingAccount:     allocation.ClearingAccount,
			RecipientAddresses:  recipientAddrs,
			AmountPerRecipient:  amountPerRecipient,
			CommunityPoolAmount: remainder,
			ScheduledAt:         timestamp,
			TotalAmount:         allocation.Amount,
		}); err != nil {
			sdkCtx.Logger().Error("failed to emit allocation completed event", "error", err)
		}

		sdkCtx.Logger().Info("allocated tokens",
			"clearing_account", allocation.ClearingAccount,
			"recipients", recipientAddrs,
			"total_amount", allocation.Amount.String(),
			"amount_per_recipient", amountPerRecipient.String(),
			"community_pool_amount", remainder.String())
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

// GetDistributionSchedule returns the complete allocation schedule as a sorted list.
// The schedule is sorted by timestamp in ascending order.
// Returns an empty slice if no allocations are scheduled.
// Note: Past schedule allocations removed after processing, so this only contains future schedule allocations.
func (k Keeper) GetDistributionSchedule(ctx context.Context) ([]types.ScheduledDistribution, error) {
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

// UpdateDistributionSchedule updates the entire distribution schedule via governance.
// This clears all existing distributions and replaces them with the new schedule.
// The new schedule is validated for consistency with existing clearing account mappings.
func (k Keeper) UpdateDistributionSchedule(
	ctx context.Context,
	authority string,
	newSchedule []types.ScheduledDistribution,
) error {
	// Check authority
	if k.authority != authority {
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	// Clear all existing schedule entries
	if err := k.AllocationSchedule.Clear(ctx, nil); err != nil {
		return errorsmod.Wrap(err, "failed to clear existing allocation schedule")
	}

	// Save the new schedule
	return k.SaveDistributionSchedule(ctx, newSchedule)
}

// DisableDistributions is a governance operation that disables distributions.
func (k Keeper) DisableDistributions(ctx context.Context, authority string) error {
	// Check authority
	if k.authority != authority {
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	return k.DistributionDisabled.Set(ctx, true)
}
