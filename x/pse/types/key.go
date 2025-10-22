package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name.
	ModuleName = "pse"

	// StoreKey defines the primary module store key.
	StoreKey = ModuleName
)

// KVStore keys.
var (
	StakingTimeKey  = collections.NewPrefix(0)
	AccountScoreKey = collections.NewPrefix(1)
)
