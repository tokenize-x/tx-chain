package v7patch1

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	addresscodec "cosmossdk.io/core/address"
	errorsmod "cosmossdk.io/errors"
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
		return errorsmod.Wrap(err, "v7patch1 recover: read OngoingDistribution")
	}
	distID := ongoing.ID

	excludedMap, err := pseKeeper.LoadExcludedAddressMap(ctx)
	if err != nil {
		return errorsmod.Wrapf(err,
			"v7patch1 recover: load excluded-address map: distribution_id=%d", distID)
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
		return errorsmod.Wrapf(err,
			"v7patch1 recover: iterate snapshot: distribution_id=%d", distID)
	}
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			iter.Close()
			return errorsmod.Wrapf(err,
				"v7patch1 recover: read snapshot kv: distribution_id=%d", distID)
		}
		addr := kv.Key.K2()
		addrStr, err := addressCodec.BytesToString(addr)
		if err != nil {
			iter.Close()
			return errorsmod.Wrapf(err,
				"v7patch1 recover: encode snapshot address: distribution_id=%d", distID)
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
		addrStr, _ := addressCodec.BytesToString(e.addr)
		if err := pseKeeper.AccountScoreSnapshot.Remove(
			ctx, collections.Join(distID, e.addr),
		); err != nil {
			return errorsmod.Wrapf(err,
				"v7patch1 recover: remove excluded snapshot entry: distribution_id=%d address=%s",
				distID, addrStr)
		}
		if e.score.IsZero() {
			continue
		}
		existing, err := pseKeeper.ExcludedAddressScore.Get(ctx, e.addr)
		if errors.Is(err, collections.ErrNotFound) {
			existing = sdkmath.ZeroInt()
		} else if err != nil {
			return errorsmod.Wrapf(err,
				"v7patch1 recover: read ExcludedAddressScore: address=%s", addrStr)
		}
		if err := pseKeeper.ExcludedAddressScore.Set(ctx, e.addr, existing.Add(e.score)); err != nil {
			return errorsmod.Wrapf(err,
				"v7patch1 recover: write ExcludedAddressScore: address=%s score=%s",
				addrStr, existing.Add(e.score))
		}
	}

	// Restore the TotalScore invariant for the non-excluded, kept entries.
	totalScore := sdkmath.ZeroInt()
	for _, e := range keep {
		totalScore = totalScore.Add(e.score)
	}
	if err := pseKeeper.TotalScore.Set(ctx, distID, totalScore); err != nil {
		return errorsmod.Wrapf(err,
			"v7patch1 recover: write TotalScore: distribution_id=%d total=%s kept_entries=%d",
			distID, totalScore, len(keep))
	}

	// Clear the circuit breaker so the next EndBlock resumes the stuck
	// distribution through ProcessNextDistribution -> resumeOngoingDistribution.
	if err := pseKeeper.DistributionDisabled.Set(ctx, false); err != nil {
		return errorsmod.Wrapf(err,
			"v7patch1 recover: clear circuit breaker: distribution_id=%d", distID)
	}
	return nil
}
