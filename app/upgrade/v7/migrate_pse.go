package v7

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"cosmossdk.io/collections"
	sdkstore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

//go:embed scheduled-distributions-mainnet.json
var mainnetScheduleJSON []byte

// firstMultiBlockDistributionID is the ID of the first distribution that will be
// processed using multi-block logic.
// The first entry (ID=1) will be processed by single-block PSE logic.
const firstMultiBlockDistributionID uint64 = 2

// lastProcessedID is the ID of the distribution already processed before the upgrade.
const lastProcessedID uint64 = 1

// scheduledDistributionsJSON is the JSON structure for the embedded schedule file.
type scheduledDistributionsJSON struct {
	ScheduledDistributions []scheduledDistributionJSON `json:"scheduled_distributions"` //nolint:tagliatelle
}

type scheduledDistributionJSON struct {
	Timestamp   string           `json:"timestamp"`
	Allocations []allocationJSON `json:"allocations"`
}

type allocationJSON struct {
	ClearingAccount string `json:"clearing_account"` //nolint:tagliatelle
	Amount          string `json:"amount"`
}

// migratePSEStore migrates the PSE module state for multi-block distribution support.
// - Replaces AllocationSchedule with full mainnet schedule.
// - DelegationTimeEntries key: Pair[AccAddress, ValAddress] -> Triple[uint64, AccAddress, ValAddress].
// - AccountScoreSnapshot key: AccAddress -> Pair[uint64, AccAddress].
// - Sets LastProcessedDistributionID to 1.
func migratePSEStore(ctx context.Context, pseKeeper pskeeper.Keeper) error {
	storeService := pseKeeper.StoreService()
	cdc := pseKeeper.Codec()

	if err := migrateDelegationTimeEntries(ctx, storeService, cdc, firstMultiBlockDistributionID); err != nil {
		return err
	}

	if err := migrateAccountScoreSnapshots(ctx, storeService, firstMultiBlockDistributionID); err != nil {
		return err
	}

	if err := migrateAllocationSchedule(ctx, storeService, cdc); err != nil {
		return err
	}

	return initLastProcessedDistributionID(ctx, pseKeeper)
}

// migrateAllocationSchedule clears the existing timestamp-keyed AllocationSchedule
// and re-initializes it from the embedded mainnet schedule JSON with sequential
// ID-based keys starting from 1.
func migrateAllocationSchedule(
	ctx context.Context,
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
) error {
	sb := collections.NewSchemaBuilder(storeService)
	schedule := collections.NewMap(
		sb,
		types.AllocationScheduleKey,
		"allocation_schedule",
		collections.Uint64Key,
		codec.CollValue[types.ScheduledDistribution](cdc),
	)
	if _, err := sb.Build(); err != nil {
		return err
	}

	// Clear old entries.
	if err := schedule.Clear(ctx, nil); err != nil {
		return err
	}

	// Parse the embedded mainnet schedule JSON.
	var scheduleData scheduledDistributionsJSON
	if err := json.Unmarshal(mainnetScheduleJSON, &scheduleData); err != nil {
		return fmt.Errorf("failed to parse mainnet schedule JSON: %w", err)
	}

	// Write all entries with sequential IDs starting from 1.
	for i, entry := range scheduleData.ScheduledDistributions {
		id := uint64(i + 1)

		timestamp, ok := sdkmath.NewIntFromString(entry.Timestamp)
		if !ok {
			return fmt.Errorf("invalid timestamp %q at index %d", entry.Timestamp, i)
		}

		var allocations []types.ClearingAccountAllocation
		for _, alloc := range entry.Allocations {
			amount, ok := sdkmath.NewIntFromString(alloc.Amount)
			if !ok {
				return fmt.Errorf("invalid amount %q for %s at index %d", alloc.Amount, alloc.ClearingAccount, i)
			}
			allocations = append(allocations, types.ClearingAccountAllocation{
				ClearingAccount: alloc.ClearingAccount,
				Amount:          amount,
			})
		}

		dist := types.ScheduledDistribution{
			ID:          id,
			Timestamp:   timestamp.Uint64(),
			Allocations: allocations,
		}
		if err := schedule.Set(ctx, id, dist); err != nil {
			return err
		}
	}

	return nil
}

// initLastProcessedDistributionID sets LastProcessedDistributionID to 1,
// indicating the first distribution is already processed before this upgrade.
func initLastProcessedDistributionID(ctx context.Context, pseKeeper pskeeper.Keeper) error {
	return pseKeeper.LastProcessedDistributionID.Set(ctx, lastProcessedID)
}

func migrateDelegationTimeEntries(
	ctx context.Context,
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
	distributionID uint64,
) error {
	oldSB := collections.NewSchemaBuilder(storeService)
	oldMap := collections.NewMap(
		oldSB,
		types.StakingTimeKey,
		"delegation_time_entries",
		collections.PairKeyCodec(sdk.AccAddressKey, sdk.ValAddressKey),
		codec.CollValue[types.DelegationTimeEntry](cdc),
	)
	if _, err := oldSB.Build(); err != nil {
		return err
	}

	type entry struct {
		delAddr sdk.AccAddress
		valAddr sdk.ValAddress
		value   types.DelegationTimeEntry
	}

	var entries []entry
	err := oldMap.Walk(ctx, nil, func(
		key collections.Pair[sdk.AccAddress, sdk.ValAddress],
		value types.DelegationTimeEntry,
	) (bool, error) {
		entries = append(entries, entry{
			delAddr: key.K1(),
			valAddr: key.K2(),
			value:   value,
		})
		return false, nil
	})
	if err != nil {
		return err
	}

	if err := oldMap.Clear(ctx, nil); err != nil {
		return err
	}

	newSB := collections.NewSchemaBuilder(storeService)
	newMap := collections.NewMap(
		newSB,
		types.StakingTimeKey,
		"delegation_time_entries",
		collections.TripleKeyCodec(collections.Uint64Key, sdk.AccAddressKey, sdk.ValAddressKey),
		codec.CollValue[types.DelegationTimeEntry](cdc),
	)
	if _, err := newSB.Build(); err != nil {
		return err
	}

	for _, e := range entries {
		key := collections.Join3(distributionID, e.delAddr, e.valAddr)
		if err := newMap.Set(ctx, key, e.value); err != nil {
			return err
		}
	}

	return nil
}

func migrateAccountScoreSnapshots(
	ctx context.Context,
	storeService sdkstore.KVStoreService,
	distributionID uint64,
) error {
	oldSB := collections.NewSchemaBuilder(storeService)
	oldMap := collections.NewMap(
		oldSB,
		types.AccountScoreKey,
		"account_score",
		sdk.AccAddressKey,
		sdk.IntValue,
	)
	if _, err := oldSB.Build(); err != nil {
		return err
	}

	type entry struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}

	var entries []entry
	err := oldMap.Walk(ctx, nil, func(key sdk.AccAddress, value sdkmath.Int) (bool, error) {
		entries = append(entries, entry{addr: key, score: value})
		return false, nil
	})
	if err != nil {
		return err
	}

	if err := oldMap.Clear(ctx, nil); err != nil {
		return err
	}

	newSB := collections.NewSchemaBuilder(storeService)
	newMap := collections.NewMap(
		newSB,
		types.AccountScoreKey,
		"account_score",
		collections.PairKeyCodec(collections.Uint64Key, sdk.AccAddressKey),
		sdk.IntValue,
	)
	if _, err := newSB.Build(); err != nil {
		return err
	}

	for _, e := range entries {
		key := collections.Join(distributionID, e.addr)
		if err := newMap.Set(ctx, key, e.score); err != nil {
			return err
		}
	}

	return nil
}
