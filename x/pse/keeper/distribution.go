package keeper

import (
	"context"
	"time"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// ProcessClearingAccountDistributions processes the next due distribution from the schedule.
// Checks the earliest scheduled distribution and processes it if the current block time has passed its timestamp.
// Only one distribution is processed per call. Should be called from EndBlock.
func (k Keeper) ProcessClearingAccountDistributions(ctx context.Context) error {
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
	if err := k.processScheduledAllocations(ctx, timestamp, bondDenom, params.ClearingAccountMappings); err != nil {
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

// processScheduledAllocations transfers tokens from clearing accounts to their mapped recipients.
// Processes all allocations within a single scheduled distribution.
// Any transfer failure indicates a state invariant violation (insufficient balance or invalid recipient).
func (k Keeper) processScheduledAllocations(
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
		// Find the recipient address mapped to this clearing account
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
		coinsToAllocate := sdk.NewCoins(sdk.NewCoin(bondDenom, allocation.Amount))

		// Transfer tokens from clearing account to recipient
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(
			ctx,
			allocation.ClearingAccount,
			recipient,
			coinsToAllocate,
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
			DistributedAt:    uint64(sdkCtx.BlockTime().Unix()),
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

// CreateDistributionSchedule generates a periodic distribution schedule over n months.
// Each distribution period allocates an equal portion (1/n) of each module account's total balance.
// Timestamps are calculated using Go's AddDate for proper Gregorian calendar handling.
// Returns the schedule without persisting it to state, making this a pure, testable function.
func CreateDistributionSchedule(
	moduleAccountBalances map[string]sdkmath.Int,
	startTime uint64,
) ([]types.ScheduledDistribution, error) {
	if len(moduleAccountBalances) == 0 {
		return nil, types.ErrNoModuleBalances
	}

	// Convert Unix timestamp to time.Time for date arithmetic
	startDateTime := time.Unix(int64(startTime), 0).UTC()

	// Pre-allocate slice with exact capacity for n distribution periods
	schedule := make([]types.ScheduledDistribution, 0, types.TotalAllocationMonths)

	for month := range types.TotalAllocationMonths {
		// Calculate distribution timestamp by adding months to start time
		// AddDate handles month length variations and leap years correctly
		distributionDateTime := startDateTime.AddDate(0, month, 0)
		distributionTime := uint64(distributionDateTime.Unix())

		// Build allocations list for this distribution period
		allocations := make([]types.ClearingAccountAllocation, 0, len(moduleAccountBalances))

		for clearingAccount, totalBalance := range moduleAccountBalances {
			// Divide total balance equally across all distribution periods using integer division
			monthlyAmount := totalBalance.QuoRaw(types.TotalAllocationMonths)

			// Fail if balance is too small to distribute over n periods
			if monthlyAmount.IsZero() {
				return nil, errorsmod.Wrapf(types.ErrInvalidInput, "clearing account %s: balance too small to divide into monthly distributions", clearingAccount)
			}

			allocations = append(allocations, types.ClearingAccountAllocation{
				ClearingAccount: clearingAccount,
				Amount:          monthlyAmount,
			})
		}

		// Add this distribution period to the schedule
		if len(allocations) > 0 {
			schedule = append(schedule, types.ScheduledDistribution{
				Timestamp:   distributionTime,
				Allocations: allocations,
			})
		}
	}

	return schedule, nil
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
