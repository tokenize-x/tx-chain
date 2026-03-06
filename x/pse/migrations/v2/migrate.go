package v2

import (
	"context"

	"cosmossdk.io/collections"
	sdkstore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// MigrateStore migrates the PSE module state from v1 to v2.
// - DelegationTimeEntries key: Pair[AccAddress, ValAddress] -> Triple[uint64, AccAddress, ValAddress] with ID=1.
// - AccountScoreSnapshot key: AccAddress -> Pair[uint64, AccAddress] with ID=1.
// - Clears old timestamp-based AllocationSchedule.
// - Initializes LastProcessedDistributionID = 0 so getNextDistributionID returns 0+1=1.
func MigrateStore(
	ctx context.Context,
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
) error {
	// All existing entries are re-keyed under distribution ID=1.
	// getNextDistributionID returns 0+1=1, matching where entries are stored.
	const distributionID uint64 = 1

	if err := migrateDelegationTimeEntries(ctx, storeService, cdc, distributionID); err != nil {
		return err
	}

	if err := migrateAccountScoreSnapshot(ctx, storeService, distributionID); err != nil {
		return err
	}

	if err := clearAllocationSchedule(ctx, storeService, cdc); err != nil {
		return err
	}

	return initLastProcessedDistributionID(ctx, storeService)
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

func migrateAccountScoreSnapshot(
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

// clearAllocationSchedule removes all entries from the old timestamp-based allocation schedule.
// The old format is incompatible with the new ID-based format; governance must submit a fresh schedule post-upgrade.
func clearAllocationSchedule(
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

	return schedule.Clear(ctx, nil)
}

// initLastProcessedDistributionID sets LastProcessedDistributionID to 0,
// so that getNextDistributionID returns 1 after migration.
func initLastProcessedDistributionID(
	ctx context.Context,
	storeService sdkstore.KVStoreService,
) error {
	sb := collections.NewSchemaBuilder(storeService)
	lastProcessedID := collections.NewItem(
		sb,
		types.LastProcessedDistributionIDKey,
		"last_processed_distribution_id",
		collections.Uint64Value,
	)
	if _, err := sb.Build(); err != nil {
		return err
	}

	return lastProcessedID.Set(ctx, 0)
}
