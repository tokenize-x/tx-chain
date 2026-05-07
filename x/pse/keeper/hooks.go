package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// Hooks implements the staking hooks interface.
type Hooks struct {
	k Keeper
}

var _ stakingtypes.StakingHooks = Hooks{}

// Hooks Create new staking hooks.
func (k Keeper) Hooks() Hooks {
	return Hooks{k}
}

// getOngoingDistribution returns the ongoing distribution if one exists.
func (k Keeper) getOngoingDistribution(ctx context.Context) (types.ScheduledDistribution, bool, error) {
	ongoing, err := k.OngoingDistribution.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return types.ScheduledDistribution{}, false, nil
	}
	if err != nil {
		return types.ScheduledDistribution{}, false, err
	}
	return ongoing, true, nil
}

// getActiveDistributionID returns the distribution ID where scores are currently stored.
// If an ongoing distribution exists, returns its ID; otherwise delegates to getNextDistributionID.
func (k Keeper) getActiveDistributionID(ctx context.Context) (uint64, error) {
	ongoing, found, err := k.getOngoingDistribution(ctx)
	if err != nil {
		return 0, err
	}
	if found {
		return ongoing.ID, nil
	}
	return k.getNextDistributionID(ctx)
}

// getNextDistributionID returns the distribution ID that new entries should be written to.
// If an ongoing distribution exists (ongoingID=N is being processed), returns N+1.
// Otherwise returns LastProcessedDistributionID + 1.
func (k Keeper) getNextDistributionID(ctx context.Context) (uint64, error) {
	ongoing, found, err := k.getOngoingDistribution(ctx)
	if err != nil {
		return 0, err
	}
	if found {
		return ongoing.ID + 1, nil
	}

	lastProcessed, err := k.LastProcessedDistributionID.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return lastProcessed + 1, nil
}

// AfterDelegationModified implements the staking hooks interface.
// Handles 3 scenarios based on where the delegator's entry exists:
//   - Scenario 1: Entry in ongoingID (ongoing distribution in progress).
//   - Scenario 2: Entry in nextID — normal score calculation.
//   - Scenario 3: No entry — create new entry, no score.
func (h Hooks) AfterDelegationModified(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	delegation, err := h.k.stakingKeeper.GetDelegation(ctx, delAddr, valAddr)
	if err != nil {
		return err
	}

	nextID, err := h.k.getNextDistributionID(ctx)
	if err != nil {
		return err
	}

	isExcluded, err := h.k.IsExcludedAddress(ctx, delAddr)
	if err != nil {
		return err
	}

	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	// Scenario 1: Entry exists in previous distribution (ongoing distribution in progress).
	// Split score at distribution timestamp, move entry to nextID.
	ongoing, ongoingFound, err := h.k.getOngoingDistribution(ctx)
	if err != nil {
		return err
	}
	if ongoingFound {
		handled, err := h.migrateOngoingEntry(ctx, ongoing, nextID, delAddr, valAddr, blockTime, isExcluded)
		if err != nil {
			return err
		}
		if handled {
			return h.k.SetDelegationTimeEntry(ctx, nextID, valAddr, delAddr, types.DelegationTimeEntry{
				LastChangedUnixSec: blockTime,
				Shares:             delegation.Shares,
			})
		}
	}

	// Scenario 2: Entry exists in next distribution.
	currentEntry, err := h.k.GetDelegationTimeEntry(ctx, nextID, valAddr, delAddr)
	if err == nil {
		score, err := calculateAddedScore(ctx, h.k, valAddr, currentEntry)
		if err != nil {
			return err
		}
		if err := h.k.addScoreForAddress(ctx, nextID, delAddr, score, isExcluded); err != nil {
			return err
		}
		return h.k.SetDelegationTimeEntry(ctx, nextID, valAddr, delAddr, types.DelegationTimeEntry{
			LastChangedUnixSec: blockTime,
			Shares:             delegation.Shares,
		})
	}
	if !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	// Scenario 3: No entry — create new under nextID (no score, duration = 0).
	return h.k.SetDelegationTimeEntry(ctx, nextID, valAddr, delAddr, types.DelegationTimeEntry{
		LastChangedUnixSec: blockTime,
		Shares:             delegation.Shares,
	})
}

// BeforeDelegationRemoved implements the staking hooks interface.
func (h Hooks) BeforeDelegationRemoved(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	nextID, err := h.k.getNextDistributionID(ctx)
	if err != nil {
		return err
	}

	isExcluded, err := h.k.IsExcludedAddress(ctx, delAddr)
	if err != nil {
		return err
	}

	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	// Scenario 1: Entry exists in previous distribution (ongoing).
	ongoing, ongoingFound, err := h.k.getOngoingDistribution(ctx)
	if err != nil {
		return err
	}
	if ongoingFound {
		if _, err := h.migrateOngoingEntry(ctx, ongoing, nextID, delAddr, valAddr, blockTime, isExcluded); err != nil {
			return err
		}
	}

	// Scenario 2: Entry exists in next distribution.
	currentEntry, err := h.k.GetDelegationTimeEntry(ctx, nextID, valAddr, delAddr)
	if err == nil {
		score, err := calculateAddedScore(ctx, h.k, valAddr, currentEntry)
		if err != nil {
			return err
		}
		if err := h.k.addScoreForAddress(ctx, nextID, delAddr, score, isExcluded); err != nil {
			return err
		}
		return h.k.RemoveDelegationTimeEntry(ctx, nextID, valAddr, delAddr)
	}
	if !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	// Scenario 3: No entry.
	return nil
}

// calculateScoreAtTimestamp calculates the score for a delegation entry up to a specific timestamp.
// score = tokens × (atTimestamp - lastChanged).
func calculateScoreAtTimestamp(
	ctx context.Context,
	keeper Keeper,
	valAddr sdk.ValAddress,
	entry types.DelegationTimeEntry,
	atTimestamp int64,
) (sdkmath.Int, error) {
	val, err := keeper.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return sdkmath.NewInt(0), err
	}
	duration := atTimestamp - entry.LastChangedUnixSec
	if duration <= 0 {
		return sdkmath.NewInt(0), nil
	}
	tokens := val.TokensFromShares(entry.Shares).TruncateInt()
	return tokens.MulRaw(duration), nil
}

// calculateAddedScore calculates the score for a delegation entry up to the current block time.
func calculateAddedScore(
	ctx context.Context,
	keeper Keeper,
	valAddr sdk.ValAddress,
	delegationTimeEntry types.DelegationTimeEntry,
) (sdkmath.Int, error) {
	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	return calculateScoreAtTimestamp(ctx, keeper, valAddr, delegationTimeEntry, blockTime)
}

// BeforeValidatorSlashed implements the staking hooks interface.
func (h Hooks) BeforeValidatorSlashed(ctx context.Context, valAddr sdk.ValAddress, fraction sdkmath.LegacyDec) error {
	return nil
}

// The following hooks don't need to be implemented.

// AfterValidatorCreated implements the staking hooks interface.
func (h Hooks) AfterValidatorCreated(ctx context.Context, valAddr sdk.ValAddress) error {
	return nil
}

// AfterValidatorRemoved implements the staking hooks interface.
func (h Hooks) AfterValidatorRemoved(ctx context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// BeforeDelegationCreated implements the staking hooks interface.
func (h Hooks) BeforeDelegationCreated(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	return nil
}

// BeforeDelegationSharesModified implements the staking hooks interface.
func (h Hooks) BeforeDelegationSharesModified(
	ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress,
) error {
	return nil
}

// BeforeValidatorModified implements the staking hooks interface.
func (h Hooks) BeforeValidatorModified(_ context.Context, _ sdk.ValAddress) error {
	return nil
}

// AfterValidatorBonded implements the staking hooks interface.
func (h Hooks) AfterValidatorBonded(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterValidatorBeginUnbonding implements the staking hooks interface.
func (h Hooks) AfterValidatorBeginUnbonding(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterUnbondingInitiated implements the staking hooks interface.
func (h Hooks) AfterUnbondingInitiated(_ context.Context, _ uint64) error {
	return nil
}

// migrateOngoingEntry handles a delegation entry that still lives under the ongoing distribution.
// It calculates score for both the ongoing and next periods, removes the entry from ongoingID,
// and returns true if the entry was found and processed.
// isExcluded controls whether computed scores are routed to ExcludedAddressScore instead of AccountScoreSnapshot.
func (h Hooks) migrateOngoingEntry(
	ctx context.Context,
	ongoing types.ScheduledDistribution,
	nextID uint64,
	delAddr sdk.AccAddress,
	valAddr sdk.ValAddress,
	blockTime int64,
	isExcluded bool,
) (bool, error) {
	ongoingID := ongoing.ID
	ongoingEntry, err := h.k.GetDelegationTimeEntry(ctx, ongoingID, valAddr, delAddr)
	if errors.Is(err, collections.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	distTimestamp := int64(ongoing.Timestamp)

	// Score for ongoing period: lastChanged -> distribution timestamp.
	ongoingScore, err := calculateScoreAtTimestamp(ctx, h.k, valAddr, ongoingEntry, distTimestamp)
	if err != nil {
		return false, err
	}
	if err := h.k.addScoreForAddress(ctx, ongoingID, delAddr, ongoingScore, isExcluded); err != nil {
		return false, err
	}

	// Score for next period: distribution timestamp -> now.
	nextPeriodEntry := types.DelegationTimeEntry{
		LastChangedUnixSec: distTimestamp,
		Shares:             ongoingEntry.Shares,
	}
	nextScore, err := calculateScoreAtTimestamp(ctx, h.k, valAddr, nextPeriodEntry, blockTime)
	if err != nil {
		return false, err
	}
	if err := h.k.addScoreForAddress(ctx, nextID, delAddr, nextScore, isExcluded); err != nil {
		return false, err
	}

	// Remove the old entry from ongoingID to prevent double scoring in Phase 1 batch processing.
	if err := h.k.RemoveDelegationTimeEntry(ctx, ongoingID, valAddr, delAddr); err != nil {
		return false, err
	}

	return true, nil
}
