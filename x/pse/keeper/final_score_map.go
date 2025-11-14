package keeper

import (
	"context"

	"cosmossdk.io/collections"
	addresscodec "cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

type scoreMap struct {
	items []struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}
	indexMap     map[string]int
	addressCodec addresscodec.Codec
	totalScore   sdkmath.Int
}

func newScoreMap(addressCodec addresscodec.Codec) *scoreMap {
	return &scoreMap{
		items: make([]struct {
			addr  sdk.AccAddress
			score sdkmath.Int
		}, 0),
		indexMap:     make(map[string]int),
		addressCodec: addressCodec,
		totalScore:   sdkmath.NewInt(0),
	}
}

func (m *scoreMap) AddScore(addr sdk.AccAddress, value sdkmath.Int) {
	if value.IsZero() {
		return
	}
	key, err := m.addressCodec.BytesToString(addr)
	if err != nil {
		return
	}
	idx, found := m.indexMap[key]
	if !found {
		m.items = append(m.items, struct {
			addr  sdk.AccAddress
			score sdkmath.Int
		}{
			addr:  addr,
			score: value,
		})
		m.indexMap[key] = len(m.items) - 1
	} else {
		m.items[idx].score = m.items[idx].score.Add(value)
	}

	m.totalScore = m.totalScore.Add(value)
}

func (m *scoreMap) Walk(fn func(addr sdk.AccAddress, score sdkmath.Int) error) error {
	for _, pair := range m.items {
		if err := fn(pair.addr, pair.score); err != nil {
			return err
		}
	}
	return nil
}

func (m *scoreMap) iterateAccountScoreSnapshot(ctx context.Context, k Keeper) error {
	iter, err := k.AccountScoreSnapshot.Iterate(ctx, nil)
	if err != nil {
		return err
	}
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		kv, err := iter.KeyValue()
		if err != nil {
			return err
		}
		score := kv.Value
		delAddr := kv.Key
		m.AddScore(delAddr, score)
	}

	return nil
}

func (m *scoreMap) iterateDelegationTimeEntries(ctx context.Context, k Keeper) (
	[]collections.KeyValue[collections.Pair[sdk.ValAddress, sdk.AccAddress], types.DelegationTimeEntry], error) {
	var allDelegationTimeEntry []collections.KeyValue[
		collections.Pair[sdk.ValAddress, sdk.AccAddress],
		types.DelegationTimeEntry,
	]

	delegationTimeEntriesIterator, err := k.DelegationTimeEntries.Iterate(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer delegationTimeEntriesIterator.Close()

	for ; delegationTimeEntriesIterator.Valid(); delegationTimeEntriesIterator.Next() {
		kv, err := delegationTimeEntriesIterator.KeyValue()
		if err != nil {
			return nil, err
		}
		allDelegationTimeEntry = append(allDelegationTimeEntry, kv)
		valAddr := kv.Key.K1()
		delAddr := kv.Key.K2()
		delegationTimeEntry := kv.Value
		delegationScore, err := calculateAddedScore(ctx, k, valAddr, delegationTimeEntry)
		if err != nil {
			return nil, err
		}
		m.AddScore(delAddr, delegationScore)
	}

	return allDelegationTimeEntry, nil
}
