package v7patch1

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	addresscodec "cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
)

// RecoverOngoingDistribution restores TotalScore[ongoingID] to the sum of its
// AccountScoreSnapshot entries, routes excluded-address entries to
// ExcludedAddressScore, and clears DistributionDisabled. No-op when no ongoing
// distribution exists.
func RecoverOngoingDistribution(
	ctx context.Context,
	pseKeeper pskeeper.Keeper,
	addressCodec addresscodec.Codec,
) error {
	ongoing, err := pseKeeper.OngoingDistribution.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	distID := ongoing.ID

	excludedMap, err := pseKeeper.LoadExcludedAddressMap(ctx)
	if err != nil {
		return err
	}

	type snapshotEntry struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}

	var (
		keep            []snapshotEntry
		excludedEntries []snapshotEntry
	)
	iter, err := pseKeeper.AccountScoreSnapshot.Iterate(
		ctx,
		collections.NewPrefixedPairRange[uint64, sdk.AccAddress](distID),
	)
	if err != nil {
		return err
	}
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			iter.Close()
			return err
		}
		addr := kv.Key.K2()
		addrStr, err := addressCodec.BytesToString(addr)
		if err != nil {
			iter.Close()
			return err
		}
		entry := snapshotEntry{addr: addr, score: kv.Value}
		if excludedMap[addrStr] {
			excludedEntries = append(excludedEntries, entry)
		} else {
			keep = append(keep, entry)
		}
	}
	iter.Close()

	for _, e := range excludedEntries {
		if err := pseKeeper.AccountScoreSnapshot.Remove(
			ctx, collections.Join(distID, e.addr),
		); err != nil {
			return err
		}
		if e.score.IsZero() {
			continue
		}
		existing, err := pseKeeper.ExcludedAddressScore.Get(ctx, e.addr)
		if errors.Is(err, collections.ErrNotFound) {
			existing = sdkmath.ZeroInt()
		} else if err != nil {
			return err
		}
		if err := pseKeeper.ExcludedAddressScore.Set(ctx, e.addr, existing.Add(e.score)); err != nil {
			return err
		}
	}

	// Restore the TotalScore invariant for the non-excluded, kept entries.
	totalScore := sdkmath.ZeroInt()
	for _, e := range keep {
		totalScore = totalScore.Add(e.score)
	}
	if err := pseKeeper.TotalScore.Set(ctx, distID, totalScore); err != nil {
		return err
	}

	// Clear the circuit breaker so the next EndBlock resumes the stuck
	// distribution through ProcessNextDistribution -> resumeOngoingDistribution.
	return pseKeeper.DistributionDisabled.Set(ctx, false)
}
