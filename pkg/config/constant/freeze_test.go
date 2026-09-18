package constant_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
)

const (
	frozenAddr1 = "core1e7y6qwktg7l6ajr8e2eal5j4dnc2jyceftnjce"
	frozenAddr2 = "core12acz3gw3aluu4dhvtz404dqac0mjmv08pjpunl"
	cleanAddr   = "core1adst6w4e79tddzhcgaru2l2gms8jjep6a4caa7"
)

func TestIsFrozenAddress(t *testing.T) {
	requireT := require.New(t)

	requireT.True(constant.IsFrozenAddress(frozenAddr1))
	requireT.True(constant.IsFrozenAddress(frozenAddr2))
	requireT.False(constant.IsFrozenAddress(cleanAddr))
	requireT.False(constant.IsFrozenAddress(""))
}

func TestIsFrozen_HeightGate(t *testing.T) {
	requireT := require.New(t)

	const h = int64(1000)
	restore := constant.FreezeActivationHeight
	constant.FreezeActivationHeight = h
	defer func() { constant.FreezeActivationHeight = restore }()

	// Below H: no freeze, even for a blacklisted address (byte-identical behavior).
	requireT.False(constant.IsFrozen(h-1, frozenAddr1))
	requireT.False(constant.IsFrozen(0, frozenAddr1))

	// At and above H: blacklisted addresses are frozen.
	requireT.True(constant.IsFrozen(h, frozenAddr1))
	requireT.True(constant.IsFrozen(h+1, frozenAddr2))

	// Clean addresses are never frozen, at any height.
	requireT.False(constant.IsFrozen(h, cleanAddr))
	requireT.False(constant.IsFrozen(h+1_000_000, cleanAddr))
}

// TestFrozenAddresses_ValidMainnetBech32 guards against a typo in a hardcoded address.
// A malformed entry would silently never match the intended account on mainnet.
// It decodes independently of the SDK's globally-configured prefix.
func TestFrozenAddresses_ValidMainnetBech32(t *testing.T) {
	requireT := require.New(t)

	requireT.NotEmpty(constant.FrozenAddresses)
	for addr := range constant.FrozenAddresses {
		hrp, bz, err := bech32.DecodeAndConvert(addr)
		requireT.NoErrorf(err, "frozen address %q is not valid bech32", addr)
		requireT.Equalf(constant.AddressPrefixMain, hrp, "frozen address %q must use the mainnet prefix", addr)
		requireT.Lenf(bz, 20, "frozen address %q must decode to a 20-byte account address", addr)
	}
}

func TestIsFrozen_MainnetActivationHeight(t *testing.T) {
	requireT := require.New(t)

	// The built binary is armed for the coordinated mainnet activation height.
	const mainnetH = int64(83_530_000)
	requireT.Equal(mainnetH, constant.FreezeActivationHeight)

	// Below H the freeze is inert (byte-identical to the tagged release).
	requireT.False(constant.IsFrozen(mainnetH-1, frozenAddr1))
	requireT.False(constant.IsFrozen(1, frozenAddr1))
	// At and above H the frozen accounts are frozen; clean accounts never are.
	requireT.True(constant.IsFrozen(mainnetH, frozenAddr1))
	requireT.True(constant.IsFrozen(mainnetH+1, frozenAddr2))
	requireT.False(constant.IsFrozen(mainnetH, cleanAddr))
}
