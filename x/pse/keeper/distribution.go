package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// ProcessNextDistribution is the EndBlock entry point for distribution processing.
// It either resumes an ongoing multi-block distribution or starts a new one if due.
//  1. If OngoingDistribution exists -> resume (Phase 1 or Phase 2)
//  2. If no ongoing -> peek schedule -> if due:
//     a. Process non-community allocations immediately (single-block)
//     b. If community allocation exists -> set OngoingDistribution (Phase 1 starts next block)
//     c. Else, no community allocation, non-community distribution is already done, remove from AllocationSchedule
func (k Keeper) ProcessNextDistribution(ctx context.Context) error {
	// Resume ongoing multi-block distribution if one is in progress.
	ongoing, found, err := k.getOngoingDistribution(ctx)
	if err != nil {
		return err
	}
	if found {
		return k.resumeOngoingDistribution(ctx, ongoing)
	}

	// No ongoing distribution — check if next scheduled distribution is due.
	scheduledDistribution, shouldProcess, err := k.PeekNextAllocationSchedule(ctx)
	if err != nil {
		return err
	}
	if !shouldProcess {
		return nil
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return err
	}

	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	// Process non-community allocations immediately (single-block).
	if err := k.distributeNonCommunityAllocations(
		ctx, scheduledDistribution, bondDenom, params.ClearingAccountMappings,
	); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Invariant: every distribution must have a positive community allocation
	// (enforced at schedule creation, also checked here as safety net).
	communityAmount := getCommunityAllocationAmount(scheduledDistribution)
	if !communityAmount.IsPositive() {
		return errorsmod.Wrapf(
			types.ErrInvariantViolation,
			"non-positive community allocation %s for distribution %d",
			communityAmount, scheduledDistribution.ID,
		)
	}

	scheduledDistribution.StartedAt = sdkCtx.BlockTime().Unix()
	if err := k.BeginCommunityDistribution(ctx, scheduledDistribution, bondDenom); err != nil {
		return err
	}
	sdkCtx.Logger().Info("started multi-block community distribution",
		"distribution_id", scheduledDistribution.ID,
		"timestamp", scheduledDistribution.Timestamp)
	return nil
}

// BeginCommunityDistribution stores the ongoing distribution and moves its community allocation
// from pse_community into the short-lived intermediary account.
func (k Keeper) BeginCommunityDistribution(
	ctx context.Context, dist types.ScheduledDistribution, bondDenom string,
) error {
	if err := k.OngoingDistribution.Set(ctx, dist); err != nil {
		return err
	}
	communityAmount := getCommunityAllocationAmount(dist)
	communityCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, communityAmount))
	return k.bankKeeper.SendCoinsFromModuleToModule(
		ctx, types.ClearingAccountCommunity, types.ClearingAccountCommunityIntermediary, communityCoins,
	)
}

// resumeOngoingDistribution continues a multi-block community distribution.
// Consumes DelegationTimeEntries in batches, then distributes tokens once all entries are consumed.
func (k Keeper) resumeOngoingDistribution(ctx context.Context, ongoing types.ScheduledDistribution) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	ongoingID := ongoing.ID

	// Consume remaining DelegationTimeEntries for score conversion.
	isConsumed, err := k.ConsumeOngoingDelegationTimeEntries(ctx, ongoing)
	if err != nil {
		return errorsmod.Wrapf(err, "resume phase 1: distribution_id=%d", ongoingID)
	}
	if !isConsumed {
		return nil
	}

	// All entries consumed — distribute tokens.
	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return errorsmod.Wrap(err, "get bond denom")
	}
	done, err := k.ProcessOngoingTokenDistribution(ctx, ongoing, bondDenom)
	if err != nil {
		return errorsmod.Wrapf(err, "resume phase 2: distribution_id=%d", ongoingID)
	}
	if done {
		sdkCtx.Logger().Info("multi-block community distribution complete",
			"distribution_id", ongoingID)
	}
	return nil
}

// PeekNextAllocationSchedule returns the next unprocessed scheduled distribution
// and whether it should be processed now.
func (k Keeper) PeekNextAllocationSchedule(ctx context.Context) (types.ScheduledDistribution, bool, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	lastProcessed, err := k.LastProcessedDistributionID.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		lastProcessed = 0
	} else if err != nil {
		return types.ScheduledDistribution{}, false, err
	}

	// Start iterating from lastProcessed+1 to skip already-processed entries.
	nextID := lastProcessed + 1
	rng := new(collections.Range[uint64]).StartInclusive(nextID)
	iter, err := k.AllocationSchedule.Iterate(ctx, rng)
	if err != nil {
		return types.ScheduledDistribution{}, false, err
	}
	defer iter.Close()

	// Return if no unprocessed entries remain.
	if !iter.Valid() {
		return types.ScheduledDistribution{}, false, nil
	}

	kv, err := iter.KeyValue()
	if err != nil {
		return types.ScheduledDistribution{}, false, err
	}

	scheduledDist := kv.Value

	// Check if distribution time has arrived.
	shouldProcess := scheduledDist.Timestamp <= uint64(sdkCtx.BlockTime().Unix())

	return scheduledDist, shouldProcess, nil
}

// distributeNonCommunityAllocations processes all non-community allocations in a single block.
func (k Keeper) distributeNonCommunityAllocations(
	ctx context.Context,
	scheduledDistribution types.ScheduledDistribution,
	bondDenom string,
	clearingAccountMappings []types.ClearingAccountMapping,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	for _, allocation := range scheduledDistribution.Allocations {
		if allocation.Amount.IsZero() {
			continue
		}

		// Community allocation handled separately via multi-block distribution.
		if allocation.ClearingAccount == types.ClearingAccountCommunity {
			continue
		}

		// Look up recipient addresses for this clearing account from governance-configured mappings.
		var recipientAddrs []string
		for _, mapping := range clearingAccountMappings {
			if mapping.ClearingAccount == allocation.ClearingAccount {
				recipientAddrs = mapping.RecipientAddresses
				break
			}
		}

		// Split allocation evenly among recipients; remainder goes to community pool.
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

		// Send equal share to each recipient from the clearing account.
		for _, recipientAddr := range recipientAddrs {
			recipient := sdk.MustAccAddressFromBech32(recipientAddr)
			coinsToSend := sdk.NewCoins(sdk.NewCoin(bondDenom, amountPerRecipient))

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

		// Send remainder to the community pool.
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
		}

		if err := sdkCtx.EventManager().EmitTypedEvent(&types.EventAllocationDistributed{
			ClearingAccount:     allocation.ClearingAccount,
			RecipientAddresses:  recipientAddrs,
			AmountPerRecipient:  amountPerRecipient,
			CommunityPoolAmount: remainder,
			ScheduledAt:         scheduledDistribution.Timestamp,
			TotalAmount:         allocation.Amount,
			DistributionId:      scheduledDistribution.ID,
		}); err != nil {
			sdkCtx.Logger().Error("failed to emit allocation completed event", "error", err)
		}
	}

	return nil
}

// SaveDistributionSchedule persists the distribution schedule to blockchain state.
// Each scheduled distribution is stored in the AllocationSchedule map, indexed by its ID.
func (k Keeper) SaveDistributionSchedule(ctx context.Context, schedule []types.ScheduledDistribution) error {
	for _, scheduledDist := range schedule {
		if err := k.AllocationSchedule.Set(ctx, scheduledDist.ID, scheduledDist); err != nil {
			return errorsmod.Wrapf(err, "failed to save distribution with id %d", scheduledDist.ID)
		}
	}
	return nil
}

// GetDistributionSchedule returns the complete allocation schedule as a sorted list,
// including both processed and unprocessed entries (processed entries are kept for visibility).
// The schedule is sorted by id in ascending order.
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

	return schedule, nil
}

// GetProcessedDistributionSchedule returns only processed scheduled distributions
// (ID <= LastProcessedDistributionID), sorted by id in ascending order.
func (k Keeper) GetProcessedDistributionSchedule(ctx context.Context) ([]types.ScheduledDistribution, error) {
	lastProcessed, err := k.LastProcessedDistributionID.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		lastProcessed = 0
	} else if err != nil {
		return nil, err
	}

	var schedule []types.ScheduledDistribution
	rng := new(collections.Range[uint64]).EndInclusive(lastProcessed)

	iter, err := k.AllocationSchedule.Iterate(ctx, rng)
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

	return schedule, nil
}

// GetUnprocessedDistributionSchedule returns only unprocessed scheduled distributions
// (ID > LastProcessedDistributionID), sorted by id in ascending order.
func (k Keeper) GetUnprocessedDistributionSchedule(ctx context.Context) ([]types.ScheduledDistribution, error) {
	lastProcessed, err := k.LastProcessedDistributionID.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		lastProcessed = 0
	} else if err != nil {
		return nil, err
	}

	var schedule []types.ScheduledDistribution
	rng := new(collections.Range[uint64]).StartExclusive(lastProcessed)

	iter, err := k.AllocationSchedule.Iterate(ctx, rng)
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

	return schedule, nil
}

// UpdateDistributionSchedule updates the distribution schedule via governance.
// This clears only unprocessed entries (ID > LastProcessedDistributionID) and replaces
// them with the new schedule. Processed entries are preserved for visibility.
func (k Keeper) UpdateDistributionSchedule(
	ctx context.Context,
	authority string,
	newSchedule []types.ScheduledDistribution,
) error {
	// Check authority
	if k.authority != authority {
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	// Reject if a multi-block distribution is in progress.
	ongoing, ongoingFound, err := k.getOngoingDistribution(ctx)
	if err != nil {
		return err
	}
	if ongoingFound {
		return errorsmod.Wrapf(
			types.ErrOngoingDistribution,
			"cannot update schedule while distribution %d is in progress", ongoing.ID,
		)
	}

	lastProcessed, err := k.LastProcessedDistributionID.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		lastProcessed = 0
	} else if err != nil {
		return err
	}

	// Validate that the first schedule ID is exactly LastProcessedDistributionID + 1.
	// Gaps are not allowed because hooks write delegation time entries under lastProcessed + 1.
	if len(newSchedule) > 0 {
		nextID := lastProcessed + 1
		if newSchedule[0].ID != nextID {
			return errorsmod.Wrapf(
				types.ErrInvalidInput,
				"first schedule ID %d must be %d (LastProcessedDistributionID + 1)",
				newSchedule[0].ID, nextID,
			)
		}
	}

	// Validate minimum gap between distributions
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	if err := types.ValidateDistributionGap(newSchedule, params.MinDistributionGapSeconds); err != nil {
		return err
	}

	// Clear only unprocessed entries (ID > LastProcessedDistributionID).
	// Processed entries are kept in state for visibility/auditability.
	rng := new(collections.Range[uint64]).StartExclusive(lastProcessed)
	if err := k.AllocationSchedule.Clear(ctx, rng); err != nil {
		return errorsmod.Wrap(err, "failed to clear unprocessed allocation schedule entries")
	}

	// Save the new schedule
	return k.SaveDistributionSchedule(ctx, newSchedule)
}

// UpdateMinDistributionGap updates the minimum time gap between distributions via governance.
// The new gap is validated against the existing on-chain schedule to ensure consistency.
func (k Keeper) UpdateMinDistributionGap(
	ctx context.Context,
	authority string,
	minGapSeconds uint64,
) error {
	if k.authority != authority {
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	// Validate new gap against unprocessed schedule entries only.
	schedule, err := k.GetUnprocessedDistributionSchedule(ctx)
	if err != nil {
		return err
	}
	if err := types.ValidateDistributionGap(schedule, minGapSeconds); err != nil {
		return errorsmod.Wrapf(err, "existing schedule violates proposed min gap of %d seconds", minGapSeconds)
	}

	// Update params
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	params.MinDistributionGapSeconds = minGapSeconds
	return k.SetParams(ctx, params)
}

// UpdateDistributionBatchSize updates the number of entries processed per EndBlock
// during multi-block community distribution via governance.
func (k Keeper) UpdateDistributionBatchSize(
	ctx context.Context,
	authority string,
	batchSize uint64,
) error {
	if k.authority != authority {
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}
	if batchSize == 0 {
		return errorsmod.Wrap(types.ErrInvalidInput, "distribution_batch_size must be greater than 0")
	}
	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}
	params.DistributionBatchSize = batchSize
	return k.SetParams(ctx, params)
}

// DisableDistributions is a governance operation that disables distributions.
func (k Keeper) DisableDistributions(ctx context.Context, authority string) error {
	// Check authority
	if k.authority != authority {
		return errorsmod.Wrapf(types.ErrInvalidAuthority, "expected %s, got %s", k.authority, authority)
	}

	return k.DistributionDisabled.Set(ctx, true)
}
