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
	ParamsKey                 = collections.NewPrefix(0)
	StakingTimeKey            = collections.NewPrefix(1)
	AccountScoreKey           = collections.NewPrefix(2)
	CompletedDistributionsKey = collections.NewPrefix(3)
	PendingTimestampsKey      = collections.NewPrefix(4)
)

// MakeCompletedDistributionKey creates a key for storing completed distributions.
// The key is a pair of (module_account, distribution_time_unix).
func MakeCompletedDistributionKey(moduleAccount string, distributionTime int64) collections.Pair[string, int64] {
	return collections.Join(moduleAccount, distributionTime)
}
