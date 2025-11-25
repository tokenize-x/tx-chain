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
	ParamsKey               = collections.NewPrefix(0)
	StakingTimeKey          = collections.NewPrefix(1)
	AccountScoreKey         = collections.NewPrefix(2)
	AllocationScheduleKey   = collections.NewPrefix(3) // Map: timestamp -> ScheduledDistribution
	SkippedDistributionsKey = collections.NewPrefix(4)
)
