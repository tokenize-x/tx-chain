package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// ProcessPeriodicDistributions processes all pending periodic distributions.
// This should be called from EndBlock to automatically distribute tokens based on scheduled timestamps.
// Distributions are guaranteed to succeed due to upfront validation of mappings and balances.
func (k Keeper) ProcessPeriodicDistributions(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	currentTimeUnix := uint64(sdkCtx.BlockTime().Unix())

	// Get first pending timestamp (queue is sorted ascending)
	timestamp, found, err := k.nextPendingTimestamp(ctx)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	// If earliest timestamp is in the future, nothing to do this block
	if timestamp > currentTimeUnix {
		return nil
	}

	// Prepare distribution context with all necessary data
	distCtx, err := k.prepareDistributionContext(ctx)
	if err != nil {
		return err
	}

	// Process distributions for this timestamp
	if err := k.processDistributionPeriod(ctx, timestamp, distCtx); err != nil {
		return err
	}

	// Clean up completed timestamp
	return k.cleanupCompletedTimestamp(ctx, timestamp)
}

// GetCompletedDistributions returns all completed distributions.
func (k Keeper) GetCompletedDistributions(ctx context.Context) ([]types.CompletedDistribution, error) {
	var distributions []types.CompletedDistribution

	iter, err := k.CompletedDistributions.Iterate(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return nil, err
		}
		distributions = append(distributions, kv.Value)
	}

	return distributions, nil
}

// GetCompletedDistributionsByModule returns completed distributions
// for a specific module account.
func (k Keeper) GetCompletedDistributionsByModule(
	ctx context.Context, moduleAccount string,
) ([]types.CompletedDistribution, error) {
	var distributions []types.CompletedDistribution

	iter, err := k.CompletedDistributions.Iterate(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return nil, err
		}
		if kv.Value.ModuleAccount == moduleAccount {
			distributions = append(distributions, kv.Value)
		}
	}

	return distributions, nil
}

// GetPendingDistributionsInfo returns detailed information about all pending scheduled distributions.
// It includes timing details, remaining time, and total amounts for each distribution period.
// Uses the PendingTimestamps queue as the source of truth for what's actually pending.
func (k Keeper) GetPendingDistributionsInfo(ctx context.Context) ([]types.PendingDistributionInfo, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	currentTime := uint64(sdkCtx.BlockTime().Unix())

	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	// Build a map of timestamp -> period for quick lookup
	periodByTimestamp := make(map[uint64]types.DistributionPeriod)
	for _, period := range params.DistributionSchedule {
		periodByTimestamp[period.DistributionTime] = period
	}

	var pendingDistributions []types.PendingDistributionInfo

	// Iterate through the ACTUAL pending queue (source of truth)
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

		// Get period from schedule - should always exist due to queue sync
		period, exists := periodByTimestamp[timestamp]
		if !exists {
			// Orphaned timestamp - this shouldn't happen but skip if it does
			sdkCtx.Logger().Error("orphaned timestamp in pending queue", "timestamp", timestamp)
			continue
		}

		// Calculate remaining time
		remainingSeconds := int64(timestamp) - int64(currentTime)

		// Calculate total amount across all module accounts
		totalAmount := sdkmath.ZeroInt()
		for _, dist := range period.Distributions {
			totalAmount = totalAmount.Add(dist.Amount)
		}

		pendingInfo := types.PendingDistributionInfo{
			DistributionTime: timestamp,
			RemainingSeconds: remainingSeconds,
			Distributions:    period.Distributions,
			TotalAmount:      totalAmount,
		}

		pendingDistributions = append(pendingDistributions, pendingInfo)
	}

	return pendingDistributions, nil
}

// distributionContext holds all the context needed for processing distributions.
type distributionContext struct {
	bondDenom          string
	subAccountMappings map[string]string // module_account -> sub_account
	periodByTimestamp  map[uint64]types.DistributionPeriod
	currentTimeUnix    uint64
	currentBlockHeight int64
}

// prepareDistributionContext prepares all necessary context for distribution processing.
func (k Keeper) prepareDistributionContext(ctx context.Context) (*distributionContext, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Get bond denom from staking params
	//nolint:contextcheck // this is correct context passing
	bondDenom, err := k.GetBondDenom(sdkCtx)
	if err != nil {
		return nil, err
	}

	// Get params which contains the distribution schedule
	params, err := k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	// Build sub account mappings map
	subAccountMappings := make(map[string]string)
	for _, mapping := range params.SubAccountMappings {
		subAccountMappings[mapping.ModuleAccount] = mapping.SubAccountAddress
	}

	// Build period lookup map
	periodByTimestamp := make(map[uint64]types.DistributionPeriod)
	for _, period := range params.DistributionSchedule {
		periodByTimestamp[period.DistributionTime] = period
	}

	return &distributionContext{
		bondDenom:          bondDenom,
		subAccountMappings: subAccountMappings,
		periodByTimestamp:  periodByTimestamp,
		currentTimeUnix:    uint64(sdkCtx.BlockTime().Unix()),
		currentBlockHeight: sdkCtx.BlockHeight(),
	}, nil
}

// processDistributionPeriod processes all distributions for a specific timestamp.
// Any failure indicates an invariant violation and returns an error to halt processing.
func (k Keeper) processDistributionPeriod(
	ctx context.Context,
	timestamp uint64,
	distCtx *distributionContext,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Get period from schedule - should always exist due to queue sync
	period, exists := distCtx.periodByTimestamp[timestamp]
	if !exists {
		// Orphaned timestamp in pending queue but not in schedule
		// This indicates queue desynchronization - critical invariant violation
		return errorsmod.Wrapf(
			types.ErrDistributionScheduleNotFound,
			"pending timestamp %d has no matching distribution period",
			timestamp,
		)
	}

	for _, dist := range period.Distributions {
		key := types.MakeCompletedDistributionKey(dist.ModuleAccount, int64(timestamp))

		// Check for duplicate completion entries - invariant violation if found
		hasCompleted, err := k.CompletedDistributions.Has(ctx, key)
		if err != nil {
			return errorsmod.Wrapf(
				types.ErrCompletedDistributionRead,
				"module '%s' timestamp %d: %v",
				dist.ModuleAccount,
				timestamp,
				err,
			)
		}
		if hasCompleted {
			return errorsmod.Wrapf(
				types.ErrDistributionAlreadyCompleted,
				"module '%s' timestamp %d",
				dist.ModuleAccount,
				timestamp,
			)
		}

		// Execute single distribution (returns error on invariant failure)
		if err := k.executeSingleDistribution(ctx, dist, timestamp, distCtx); err != nil {
			return err
		}

		sdkCtx.Logger().Info("distributed tokens",
			"module_account", dist.ModuleAccount,
			"sub_account", distCtx.subAccountMappings[dist.ModuleAccount],
			"amount", dist.Amount.String())
	}

	return nil
}

// executeSingleDistribution executes a single module distribution.
func (k Keeper) executeSingleDistribution(
	ctx context.Context,
	dist types.ModuleDistribution,
	timestamp uint64,
	distCtx *distributionContext,
) error {
	// Get the sub account - guaranteed to exist by validation
	subAccount := distCtx.subAccountMappings[dist.ModuleAccount]

	// Parse sub account address - guaranteed valid by validation
	subAccountAddr := sdk.MustAccAddressFromBech32(subAccount)

	// Create coins to distribute
	coinsToDistribute := sdk.NewCoins(sdk.NewCoin(distCtx.bondDenom, dist.Amount))

	// Transfer tokens - error indicates invariant violation
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(
		ctx,
		dist.ModuleAccount,
		subAccountAddr,
		coinsToDistribute,
	); err != nil {
		return errorsmod.Wrapf(
			types.ErrTransferFailed,
			"balance invariant violated for module '%s': %v",
			dist.ModuleAccount,
			err,
		)
	}

	// Record completion
	key := types.MakeCompletedDistributionKey(dist.ModuleAccount, int64(timestamp))
	completedDist := types.CompletedDistribution{
		ModuleAccount:          dist.ModuleAccount,
		SubAccount:             subAccount,
		ScheduledTime:          timestamp,
		ActualDistributionTime: distCtx.currentTimeUnix,
		Amount:                 dist.Amount,
		BlockHeight:            distCtx.currentBlockHeight,
	}

	if err := k.CompletedDistributions.Set(ctx, key, completedDist); err != nil {
		return errorsmod.Wrapf(
			types.ErrCompletedDistributionRecord,
			"module '%s' timestamp %d: %v",
			dist.ModuleAccount,
			timestamp,
			err,
		)
	}

	// Emit event
	k.emitDistributionCompletedEvent(ctx, dist, timestamp, distCtx, subAccount)

	return nil
}

func (k Keeper) emitDistributionCompletedEvent(
	ctx context.Context,
	dist types.ModuleDistribution,
	timestamp uint64,
	distCtx *distributionContext,
	subAccount string,
) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventDistributionCompleted{
		ModuleAccount: dist.ModuleAccount,
		SubAccount:    subAccount,
		ScheduledTime: timestamp,
		ActualTime:    distCtx.currentTimeUnix,
		Amount:        dist.Amount,
		Denom:         distCtx.bondDenom,
		BlockHeight:   distCtx.currentBlockHeight,
	}); err != nil {
		sdkCtx.Logger().Error("failed to emit event", "error", err)
	}
}

func (k Keeper) nextPendingTimestamp(ctx context.Context) (uint64, bool, error) {
	iter, err := k.PendingTimestamps.Iterate(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer iter.Close()

	if !iter.Valid() {
		return 0, false, nil
	}

	timestamp, err := iter.Key()
	if err != nil {
		return 0, false, err
	}

	return timestamp, true, nil
}

// cleanupCompletedTimestamp removes the processed timestamp from the pending queue.
func (k Keeper) cleanupCompletedTimestamp(ctx context.Context, timestamp uint64) error {
	if err := k.PendingTimestamps.Remove(ctx, timestamp); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	sdkCtx.Logger().Info("removed completed timestamp from pending queue",
		"timestamp", timestamp)

	return nil
}

// rebuildPendingQueue rebuilds the pending timestamps queue from the distribution schedule
// and completed distributions. This ensures the queue is in sync with actual state.
// Used by InitGenesis and when schedule updates are made via governance.
func (k Keeper) rebuildPendingQueue(ctx context.Context) error {
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Clear existing pending timestamps
	iter, err := k.PendingTimestamps.Iterate(ctx, nil)
	if err != nil {
		return err
	}

	var timestampsToRemove []uint64
	for ; iter.Valid(); iter.Next() {
		timestamp, err := iter.Key()
		if err != nil {
			iter.Close()
			return err
		}
		timestampsToRemove = append(timestampsToRemove, timestamp)
	}
	iter.Close()

	for _, timestamp := range timestampsToRemove {
		if err := k.PendingTimestamps.Remove(ctx, timestamp); err != nil {
			return err
		}
	}

	// Rebuild from schedule: add timestamps where not all distributions are completed
	for _, period := range params.DistributionSchedule {
		allCompleted := true
		for _, dist := range period.Distributions {
			key := types.MakeCompletedDistributionKey(dist.ModuleAccount, int64(period.DistributionTime))
			has, err := k.CompletedDistributions.Has(ctx, key)
			if err != nil {
				return err
			}
			if !has {
				allCompleted = false
				break
			}
		}

		// If not all completed, add to pending queue
		if !allCompleted {
			if err := k.PendingTimestamps.Set(ctx, period.DistributionTime); err != nil {
				return err
			}
		}
	}

	return nil
}
