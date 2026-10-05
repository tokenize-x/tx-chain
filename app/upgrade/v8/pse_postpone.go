package v8

import (
	"context"
	"time"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/pkg/errors"

	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	psekeeper "github.com/tokenize-x/tx-chain/v8/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

// PSEPostponeCutoff is the last PSE distribution kept on its original date (mainnet proposal 46).
// Every unprocessed distribution scheduled after it moves forward by PSEPostponeYears.
var PSEPostponeCutoff = time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)

// PSETestnetPostponeCutoff is the testnet cutoff, so the pause can be checked on testnet before mainnet.
// Testnet pays on the 5th, so its October 2026 distribution is the last one kept.
var PSETestnetPostponeCutoff = time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)

// PSEPostponeCutoffFor returns the cutoff for a chain: testnet has its own, every other chain uses PSEPostponeCutoff.
func PSEPostponeCutoffFor(chainID string) time.Time {
	if constant.ChainID(chainID) == constant.ChainIDTest {
		return PSETestnetPostponeCutoff
	}

	return PSEPostponeCutoff
}

// PSEPostponeYears is how far the postponed distributions move (mainnet proposal 46).
const PSEPostponeYears = 1

// PostponePSEDistributions moves every unprocessed distribution scheduled after cutoff forward by PSEPostponeYears.
// IDs and allocations are kept, because scores and delegation entries are keyed by distribution ID.
// A distribution that is already being processed is left untouched and finishes normally.
// The new schedule is validated before anything is written, so the store is never left half-shifted.
func PostponePSEDistributions(ctx context.Context, pseKeeper psekeeper.Keeper, cutoff time.Time) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	logger := sdkCtx.Logger().With("upgrade", Name, "step", "pse-postpone")

	schedule, err := pseKeeper.GetUnprocessedDistributionSchedule(ctx)
	if err != nil {
		return err
	}

	ongoingID, err := ongoingDistributionID(ctx, pseKeeper)
	if err != nil {
		return err
	}

	shifted := make([]psetypes.ScheduledDistribution, 0, len(schedule))
	for i, distribution := range schedule {
		if distribution.ID == ongoingID || distribution.Timestamp <= uint64(cutoff.Unix()) {
			continue
		}
		newTime := time.Unix(int64(distribution.Timestamp), 0).UTC().AddDate(PSEPostponeYears, 0, 0)
		schedule[i].Timestamp = uint64(newTime.Unix())
		shifted = append(shifted, schedule[i])
	}

	if len(shifted) == 0 {
		logger.Info("no PSE distributions to postpone")
		return nil
	}

	if err := psetypes.ValidateDistributionSchedule(schedule); err != nil {
		return errors.Wrap(err, "postponed PSE schedule is invalid")
	}
	params, err := pseKeeper.GetParams(ctx)
	if err != nil {
		return err
	}
	if err := psetypes.ValidateDistributionGap(schedule, params.MinDistributionGapSeconds); err != nil {
		return errors.Wrap(err, "postponed PSE schedule violates the minimum distribution gap")
	}

	for _, distribution := range shifted {
		if err := pseKeeper.AllocationSchedule.Set(ctx, distribution.ID, distribution); err != nil {
			return err
		}
	}

	logger.Info("PSE distributions postponed",
		"count", len(shifted),
		"first_id", shifted[0].ID,
		"first_timestamp", shifted[0].Timestamp,
		"last_id", shifted[len(shifted)-1].ID,
		"last_timestamp", shifted[len(shifted)-1].Timestamp,
	)

	return nil
}

// ongoingDistributionID returns the ID of the distribution being processed, or 0 when there is none.
// Distribution IDs start at 1, so 0 never matches a scheduled distribution.
func ongoingDistributionID(ctx context.Context, pseKeeper psekeeper.Keeper) (uint64, error) {
	ongoing, err := pseKeeper.OngoingDistribution.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return ongoing.ID, nil
}
