package v7

import (
	"context"

	"cosmossdk.io/collections"
	sdkstore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	pskeeper "github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// migratePSEStore migrates the PSE module state.
// - DelegationTimeEntries key: Pair[AccAddress, ValAddress] -> Triple[uint64, AccAddress, ValAddress].
// - AccountScoreSnapshot key: AccAddress -> Pair[uint64, AccAddress].
func migratePSEStore(ctx context.Context, pseKeeper pskeeper.Keeper) error {
	storeService := pseKeeper.StoreService()
	cdc := pseKeeper.Codec()

	distributionID, err := getFirstDistributionID(ctx, storeService, cdc)
	if err != nil {
		return err
	}

	if err := migrateDelegationTimeEntries(ctx, storeService, cdc, distributionID); err != nil {
		return err
	}

	return migrateAccountScoreSnapshot(ctx, storeService, distributionID)
}

// TODO: Currently assigns the first distribution ID to all entries. Implement proper mapping
// of entries to correct distribution IDs based on timestamps when multiple distributions exist.
func getFirstDistributionID(
	ctx context.Context,
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
) (uint64, error) {
	sb := collections.NewSchemaBuilder(storeService)
	schedule := collections.NewMap(
		sb,
		types.AllocationScheduleKey,
		"allocation_schedule",
		collections.Uint64Key,
		codec.CollValue[types.ScheduledDistribution](cdc),
	)
	if _, err := sb.Build(); err != nil {
		return 0, err
	}

	iter, err := schedule.Iterate(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	if !iter.Valid() {
		return 0, nil
	}

	kv, err := iter.KeyValue()
	if err != nil {
		return 0, err
	}

	return kv.Value.ID, nil
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
