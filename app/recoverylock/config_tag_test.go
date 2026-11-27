//go:build recoverylock_test

package recoverylock

import (
	"bytes"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/stretchr/testify/require"
)

func TestUseTestConfig(t *testing.T) {
	ac := address.NewBech32Codec("devcore")
	listed, paramsJSON := readTestData(t)
	restore := UseTestConfig(listed, paramsJSON)
	cfg := MustConfig(testChainID, ac, nil, nil)
	require.NotNil(t, cfg)
	require.Len(t, cfg.recoveries, 3)
	restore()
	require.Nil(t, MustConfig(testChainID, ac, nil, nil))
	bad := bytes.Replace(paramsJSON, []byte(`"listCount": 3`), []byte(`"listCount": 4`), 1)
	require.NotEqual(t, paramsJSON, bad)
	t.Cleanup(UseTestConfig(listed, bad))
	require.Panics(t, func() { MustConfig(testChainID, ac, nil, nil) })
}

// A list for another chain is not decoded and not wired: the binary serves
// several networks with different address prefixes.
func TestOtherChainNotWired(t *testing.T) {
	listed, paramsJSON := readTestData(t)
	p, err := parseParams(paramsJSON)
	require.NoError(t, err)
	require.Equal(t, testChainID, p.ChainID)
	t.Cleanup(UseTestConfig(listed, paramsJSON))
	require.Nil(t, MustConfig("coreum-mainnet-1", address.NewBech32Codec("core"), nil, nil))
	require.NotNil(t, MustConfig(testChainID, address.NewBech32Codec("devcore"), nil, nil))
	// a wrong prefix on the right chain is a config error, not a silent skip
	require.Panics(t, func() { MustConfig(testChainID, address.NewBech32Codec("core"), nil, nil) })
}
