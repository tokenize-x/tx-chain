package recoverylock

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/stretchr/testify/require"
)

func TestMainnetWalletManifest(t *testing.T) {
	var manifest struct {
		ChainID         string   `json:"chainId"`
		RecoveryAddress string   `json:"recoveryAddress"`
		FeePayerAddress string   `json:"feePayerAddress"`
		Wallets         []string `json:"wallets"`
	}
	input, err := os.ReadFile("mainnet-wallets.json")
	require.NoError(t, err)
	require.NoError(t, decodeParams(input, &manifest))
	require.Equal(t, ReleaseChainID, manifest.ChainID)
	require.Equal(t, "core1uzc4cw2jtzqx8pmdqmewpt0ww3x33k9c4k59ek", manifest.RecoveryAddress)
	require.Equal(t, "core1mhwea2pnk7td6pt44fwl2r8vx4nhm2dcjllruq", manifest.FeePayerAddress)
	require.NotEqual(t, manifest.RecoveryAddress, manifest.FeePayerAddress)

	ac := address.NewBech32Codec("core")
	listed := []byte(strings.Join(manifest.Wallets, "\n") + "\n")
	listHash := fmt.Sprintf("%x", sha256.Sum256(listed))
	// Pin the approved manifest independently of the generated release settings.
	require.Len(t, manifest.Wallets, 191)
	require.Equal(t, "e7f2c312264a29f27ed45c826b973dc9264b952fef616f8cb7a4056c7d95ab91", listHash)
	p := params{
		Enabled:                true,
		ChainID:                manifest.ChainID,
		FeePayers:              []string{manifest.FeePayerAddress},
		RecoveryAddresses:      make(map[string]string, len(manifest.Wallets)),
		ActivationHeight:       20_000_000,
		EstimateRefHeight:      19_990_000,
		EstimateRefTime:        1_700_000_000,
		ActivationTimeEstimate: 1_700_007_000,
		UnbondingSeconds:       MinReleaseUnbondingSeconds,
		ExpiryTime:             1_700_007_000 + 2*MinReleaseUnbondingSeconds + ExpiryMarginSeconds,
		ListCount:              len(manifest.Wallets),
		ListSHA256:             listHash,
	}
	for _, wallet := range manifest.Wallets {
		p.RecoveryAddresses[wallet] = manifest.RecoveryAddress
	}
	settings, err := json.Marshal(p)
	require.NoError(t, err)
	cfg, _, err := parseConfig(listed, settings, ac, nil, false)
	require.NoError(t, err)
	require.Len(t, cfg.recoveries, len(manifest.Wallets))
	payer, err := ac.StringToBytes(manifest.FeePayerAddress)
	require.NoError(t, err)
	require.False(t, cfg.isListed(payer))
	require.Len(t, cfg.feePayers, 1)
	require.True(t, cfg.isFeePayer(payer))

	t.Run("release startup with one fee payer", func(t *testing.T) {
		if testBuild {
			t.Skip("requires an untagged release build")
		}
		originalListed, originalParams := listedFile, paramsFile
		t.Cleanup(func() { listedFile, paramsFile = originalListed, originalParams })
		listedFile, paramsFile = listed, settings
		require.Equal(t, cfg, MustConfig(ReleaseChainID, ac, nil, nil))
	})
}

func TestReleaseStartupRejectsInvalidChain(t *testing.T) {
	if testBuild {
		t.Skip("tagged builds permit other chain IDs")
	}
	original := paramsFile
	t.Cleanup(func() { paramsFile = original })

	paramsFile = []byte(`{"enabled":true,"chainId":"coreum-testnet-1"` +
		`,"activationHeight":1,"expiryTime":1,"feePayers":["unused"]}`)
	ac := address.NewBech32Codec("core")
	require.PanicsWithValue(t,
		`recovery lock config: release builds need chain_id coreum-mainnet-1, got "coreum-testnet-1"`,
		func() { MustConfig(ReleaseChainID, ac, nil, nil) },
	)

	paramsFile = []byte(`{"enabled":true,"chainId":"coreum-mainnet-1"}`)
	require.Nil(t, MustConfig("coreum-testnet-1", address.NewBech32Codec("testcore"), nil, nil))
}
