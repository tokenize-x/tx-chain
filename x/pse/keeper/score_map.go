package keeper

import (
	"context"

	addresscodec "cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type scoreMap struct {
	items []struct {
		addr  sdk.AccAddress
		score sdkmath.Int
	}
	indexMap          map[string]int
	addressCodec      addresscodec.Codec
	totalScore        sdkmath.Int
	excludedAddresses []sdk.AccAddress
}

func newScoreMap(addressCodec addresscodec.Codec, excludedAddressesStr []string) (*scoreMap, error) {
	excludedAddresses := make([]sdk.AccAddress, len(excludedAddressesStr))
	for i, addr := range excludedAddressesStr {
		var err error
		excludedAddresses[i], err = addressCodec.StringToBytes(addr)
		if err != nil {
			return nil, err
		}
	}
	return &scoreMap{
		items: make([]struct {
			addr  sdk.AccAddress
			score sdkmath.Int
		}, 0),
		indexMap:          make(map[string]int),
		addressCodec:      addressCodec,
		totalScore:        sdkmath.NewInt(0),
		excludedAddresses: excludedAddresses,
	}, nil
}

func (m *scoreMap) addScore(addr sdk.AccAddress, value sdkmath.Int) error {
	if value.IsZero() {
		return nil
	}
	key, err := m.addressCodec.BytesToString(addr)
	if err != nil {
		return err
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
	return nil
}

func (m *scoreMap) walk(fn func(addr sdk.AccAddress, score sdkmath.Int) error) error {
	for _, pair := range m.items {
		if m.isExcludedAddress(pair.addr) {
			continue
		}
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
		if m.isExcludedAddress(delAddr) {
			continue
		}
		err = m.addScore(delAddr, score)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *scoreMap) iterateDelegationTimeEntries(ctx context.Context, k Keeper) error {
	delegationTimeEntriesIterator, err := k.DelegationTimeEntries.Iterate(ctx, nil)
	if err != nil {
		return err
	}
	defer delegationTimeEntriesIterator.Close()

	for ; delegationTimeEntriesIterator.Valid(); delegationTimeEntriesIterator.Next() {
		kv, err := delegationTimeEntriesIterator.KeyValue()
		if err != nil {
			return err
		}
		delAddr := kv.Key.K1()
		valAddr := kv.Key.K2()
		if m.isExcludedAddress(delAddr) {
			continue
		}

		delegationTimeEntry := kv.Value
		delegationScore, err := calculateAddedScore(ctx, k, valAddr, delegationTimeEntry)
		if err != nil {
			return err
		}
		err = m.addScore(delAddr, delegationScore)
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *scoreMap) isExcludedAddress(addr sdk.AccAddress) bool {
	for _, excludedAddress := range m.excludedAddresses {
		if excludedAddress.Equals(addr) {
			return true
		}
	}
	return false
}
