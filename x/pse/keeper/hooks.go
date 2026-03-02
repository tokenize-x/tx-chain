package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
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

// getCurrentDistributionID returns the distribution ID that new entries should be written to.
// If an ongoing distribution exists (ID=N is being processed), returns N+1.
// Otherwise returns the next scheduled distribution's ID (zero-value ID when no schedule exists).
// TODO: handle empty distribution schedule — currently returns 0 when no schedule exists.
func (k Keeper) getCurrentDistributionID(ctx context.Context) (uint64, error) {
	ongoing, found, err := k.getOngoingDistribution(ctx)
	if err != nil {
		return 0, err
	}
	if found {
		return ongoing.ID + 1, nil
	}

	distribution, _, err := k.PeekNextAllocationSchedule(ctx)
	if err != nil {
		return 0, err
	}
	return distribution.ID, nil
}

// AfterDelegationModified implements the staking hooks interface.
// Handles 3 scenarios based on where the delegator's entry exists:
//   - Scenario 1: Entry in prevID (ongoing distribution in progress).
//   - Scenario 2: Entry in currentID — normal score calculation.
//   - Scenario 3: No entry — create new entry, no score.
func (h Hooks) AfterDelegationModified(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	delegation, err := h.k.stakingKeeper.GetDelegation(ctx, delAddr, valAddr)
	if err != nil {
		return err
	}

	currentID, err := h.k.getCurrentDistributionID(ctx)
	if err != nil {
		return err
	}

	isExcluded, err := h.k.IsExcludedAddress(ctx, delAddr)
	if err != nil {
		return err
	}
	if isExcluded {
		return nil
	}

	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	// Scenario 1: Entry exists in previous distribution (ongoing distribution in progress).
	// Split score at distribution timestamp, move entry to currentID.
	ongoing, ongoingFound, err := h.k.getOngoingDistribution(ctx)
	if err != nil {
		return err
	}
	if ongoingFound {
		handled, err := h.migrateOngoingEntry(ctx, ongoing, currentID, delAddr, valAddr, blockTime)
		if err != nil {
			return err
		}
		if handled {
			return h.k.SetDelegationTimeEntry(ctx, currentID, valAddr, delAddr, types.DelegationTimeEntry{
				LastChangedUnixSec: blockTime,
				Shares:             delegation.Shares,
			})
		}
	}

	// Scenario 2: Entry exists in current distribution.
	currentEntry, err := h.k.GetDelegationTimeEntry(ctx, currentID, valAddr, delAddr)
	if err == nil {
		score, err := calculateAddedScore(ctx, h.k, valAddr, currentEntry)
		if err != nil {
			return err
		}
		if err := h.k.addToScore(ctx, currentID, delAddr, score); err != nil {
			return err
		}
		return h.k.SetDelegationTimeEntry(ctx, currentID, valAddr, delAddr, types.DelegationTimeEntry{
			LastChangedUnixSec: blockTime,
			Shares:             delegation.Shares,
		})
	}
	if !errors.Is(err, collections.ErrNotFound) {
		return err
	}

	// Scenario 3: No entry - create new in currentID (no score, duration = 0).
	return h.k.SetDelegationTimeEntry(ctx, currentID, valAddr, delAddr, types.DelegationTimeEntry{
		LastChangedUnixSec: blockTime,
		Shares:             delegation.Shares,
	})
}

// BeforeDelegationRemoved implements the staking hooks interface.
func (h Hooks) BeforeDelegationRemoved(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	currentID, err := h.k.getCurrentDistributionID(ctx)
	if err != nil {
		return err
	}

	isExcluded, err := h.k.IsExcludedAddress(ctx, delAddr)
	if err != nil {
		return err
	}
	if isExcluded {
		return nil
	}

	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	// Scenario 1: Entry exists in previous distribution (ongoing).
	ongoing, ongoingFound, err := h.k.getOngoingDistribution(ctx)
	if err != nil {
		return err
	}
	if ongoingFound {
		handled, err := h.migrateOngoingEntry(ctx, ongoing, currentID, delAddr, valAddr, blockTime)
		if err != nil {
			return err
		}
		if handled {
			return h.k.RemoveDelegationTimeEntry(ctx, ongoing.ID, valAddr, delAddr)
		}
	}

	// Scenario 2: Entry exists in current distribution.
	currentEntry, err := h.k.GetDelegationTimeEntry(ctx, currentID, valAddr, delAddr)
	if err == nil {
		score, err := calculateAddedScore(ctx, h.k, valAddr, currentEntry)
		if err != nil {
			return err
		}
		if err := h.k.addToScore(ctx, currentID, delAddr, score); err != nil {
			return err
		}
		return h.k.RemoveDelegationTimeEntry(ctx, currentID, valAddr, delAddr)
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

// migrateOngoingEntry handles a delegation entry that still lives in the previous (ongoing) distribution.
// It calculates score for both the prev and current periods, removes the entry from prevID,
// and returns true if the entry was found and processed.
func (h Hooks) migrateOngoingEntry(
	ctx context.Context,
	ongoing types.ScheduledDistribution,
	currentID uint64,
	delAddr sdk.AccAddress,
	valAddr sdk.ValAddress,
	blockTime int64,
) (bool, error) {
	prevID := ongoing.ID
	prevEntry, err := h.k.GetDelegationTimeEntry(ctx, prevID, valAddr, delAddr)
	if errors.Is(err, collections.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	distTimestamp := int64(ongoing.Timestamp)

	// Score for previous period: lastChanged -> distribution timestamp.
	prevScore, err := calculateScoreAtTimestamp(ctx, h.k, valAddr, prevEntry, distTimestamp)
	if err != nil {
		return false, err
	}
	if err := h.k.addToScore(ctx, prevID, delAddr, prevScore); err != nil {
		return false, err
	}

	// Score for current period: distribution timestamp -> now.
	currentPeriodEntry := types.DelegationTimeEntry{
		LastChangedUnixSec: distTimestamp,
		Shares:             prevEntry.Shares,
	}
	currentScore, err := calculateScoreAtTimestamp(ctx, h.k, valAddr, currentPeriodEntry, blockTime)
	if err != nil {
		return false, err
	}
	if err := h.k.addToScore(ctx, currentID, delAddr, currentScore); err != nil {
		return false, err
	}

	return true, nil
}
