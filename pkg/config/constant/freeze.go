package constant

import "math"

// Emergency account freeze, height-gated for a coordinated binary rollout.
// Frozen accounts cannot debit any balance once the chain reaches FreezeActivationHeight.
// Enforced by the bank BeforeSend hook (x/asset/ft) and the ante decorator (x/auth/ante), which both read this set.
// Below the activation height every consumer gates on IsFrozen, so the binary matches the tagged release.

// FreezeDisabledHeight keeps the freeze off.
// It is the default, so an unconfigured binary behaves like the tagged release.
const FreezeDisabledHeight = int64(math.MaxInt64)

// FreezeActivationHeight is the height H at which the freeze activates.
// Mainnet activation: block 83_530_000 (~2026-08-18 14:39 UTC).
// It is a var only so it can be set per environment and by tests, never mutated at runtime.
var FreezeActivationHeight = int64(83_530_000)

// FrozenAddresses is the hardcoded set of frozen accounts.
// A testnet rehearsal build substitutes the seeded testnet address.
// It is exported only so tests can drive the freeze, populated at init and never mutated at runtime.
var FrozenAddresses = map[string]struct{}{
	"core1e7y6qwktg7l6ajr8e2eal5j4dnc2jyceftnjce": {}, // primary exploit wallet
	"core12acz3gw3aluu4dhvtz404dqac0mjmv08pjpunl": {}, // PoC wallet
}

// IsFrozenAddress reports whether addr is in the freeze set, regardless of height.
func IsFrozenAddress(addr string) bool {
	_, ok := FrozenAddresses[addr]
	return ok
}

// IsFrozen reports whether addr must be frozen at the given block height.
// It is false below FreezeActivationHeight, matching the tagged release until height H.
func IsFrozen(blockHeight int64, addr string) bool {
	return blockHeight >= FreezeActivationHeight && IsFrozenAddress(addr)
}
