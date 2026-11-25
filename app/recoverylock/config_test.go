package recoverylock

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/stretchr/testify/require"
)

const testChainID = "coreum-devnet-1"

func readTestData(t *testing.T) (listed, paramsJSON []byte) {
	t.Helper()
	listed, err := os.ReadFile("testdata/listed.txt")
	require.NoError(t, err)
	paramsJSON, err = os.ReadFile("testdata/params.json")
	require.NoError(t, err)
	return listed, paramsJSON
}

func TestEmbeddedConfigDisabled(t *testing.T) {
	ac := address.NewBech32Codec("devcore")
	for _, test := range []bool{false, true} {
		cfg, p, err := parseConfig(listedFile, paramsFile, ac, nil, test)
		require.NoError(t, err)
		require.Nil(t, cfg)
		require.False(t, p.Enabled)
	}
	require.Nil(t, MustConfig(testChainID, ac, nil, nil))
	require.Nil(t, New(nil, nil, ac, nil, nil))
}

func TestDisabledConfigCarriesNoData(t *testing.T) {
	ac := address.NewBech32Codec("devcore")
	listed, paramsJSON := readTestData(t)
	disabled := bytes.Replace(paramsJSON, []byte(`"enabled": true`), []byte(`"enabled": false`), 1)
	require.NotEqual(t, paramsJSON, disabled)
	_, _, err := parseConfig(listedFile, disabled, ac, nil, true)
	require.ErrorContains(t, err, "sets fields")
	_, _, err = parseConfig(listed, paramsFile, ac, nil, true)
	require.ErrorContains(t, err, "has entries")
	_, _, err = parseConfig(listedFile, []byte(`{"enabled": false, "list_count": 0, "fee_payers": []}`), ac, nil, true)
	require.ErrorContains(t, err, "sets fields")
}

func TestTestDataConfig(t *testing.T) {
	ac := address.NewBech32Codec("devcore")
	listed, paramsJSON := readTestData(t)
	cfg, p, err := parseConfig(listed, paramsJSON, ac, nil, true)
	require.NoError(t, err)
	require.True(t, p.Enabled)
	require.True(t, p.TestValues)
	require.Len(t, cfg.recoveries, 3)
	require.Len(t, cfg.feePayers, 8)
	for i := 0; i < 3; i++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("recoverylock-test-listed-%d", i)))
		key := &secp256k1.PrivKey{Key: digest[:]}
		require.True(t, cfg.isListed(key.PubKey().Address()))
	}
	for i := 0; i < 8; i++ {
		digest := sha256.Sum256([]byte(fmt.Sprintf("recoverylock-test-fee-payer-%d", i)))
		key := &secp256k1.PrivKey{Key: digest[:]}
		require.True(t, cfg.isFeePayer(key.PubKey().Address()))
	}
	for wallet, destination := range cfg.recoveries {
		require.True(t, knownTestAddresses()[wallet])
		require.True(t, knownTestAddresses()[string(destination)])
	}
	// release rules refuse the test values
	_, _, err = parseConfig(listed, paramsJSON, ac, nil, false)
	require.ErrorContains(t, err, "test values")
}

func TestConfigValidation(t *testing.T) {
	ac := address.NewBech32Codec("core")
	encode := func(i byte) string {
		s, err := ac.BytesToString(bytes.Repeat([]byte{i}, 20))
		require.NoError(t, err)
		return s
	}
	listed := []byte(encode(1) + "\n" + encode(2) + "\n")
	valid := func() params {
		return params{
			RecoveryAddresses:      map[string]string{encode(1): encode(3), encode(2): encode(4)},
			FeePayers:              []string{encode(5)},
			ActivationHeight:       20_000_000,
			EstimateRefHeight:      19_990_000,
			EstimateRefTime:        1_700_000_000,
			ActivationTimeEstimate: 1_700_007_000,
			UnbondingSeconds:       MinReleaseUnbondingSeconds,
			ExpiryTime:             1_700_007_000 + 2*MinReleaseUnbondingSeconds + ExpiryMarginSeconds,
			ChainID:                ReleaseChainID,
			ListCount:              2, ListSHA256: fmt.Sprintf("%x", sha256.Sum256(listed)),
		}
	}
	cfg, err := build(listed, valid(), ac, nil, false)
	require.NoError(t, err)
	require.True(t, cfg.isRecovery(bytes.Repeat([]byte{1}, 20), bytes.Repeat([]byte{3}, 20)))
	require.False(t, cfg.isRecovery(bytes.Repeat([]byte{1}, 20), bytes.Repeat([]byte{4}, 20)))
	payerIsRecovery := valid()
	payerIsRecovery.FeePayers[0] = encode(3)
	cfg, err = build(listed, payerIsRecovery, ac, nil, false)
	require.NoError(t, err)
	require.True(t, cfg.isFeePayer(bytes.Repeat([]byte{3}, 20)))

	maxPayers := valid()
	for i := byte(6); i <= 20; i++ {
		maxPayers.FeePayers = append(maxPayers.FeePayers, encode(i))
	}
	_, err = build(listed, maxPayers, ac, nil, false)
	require.NoError(t, err)
	maxPayers.FeePayers = append(maxPayers.FeePayers, encode(21))
	_, err = build(listed, maxPayers, ac, nil, false)
	require.ErrorContains(t, err, "need 1 to 16 fee payers")

	latest := valid()
	latest.ExpiryTime = latest.ActivationTimeEstimate + MaxReleaseWindowSeconds
	_, err = build(listed, latest, ac, nil, false)
	require.NoError(t, err)
	_, err = build(listed, valid(), ac, map[string]bool{encode(3): false}, false)
	require.ErrorContains(t, err, "module account")
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{"duplicate listed wallet", encode(1) + "\n" + encode(1), "duplicate listed wallet"},
		{"malformed listed wallet", encode(1) + "\ninvalid", "list line 2"},
		{"empty list", "# no wallets", "expected a nonempty list"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(tt.input)
			p := valid()
			p.ListSHA256 = fmt.Sprintf("%x", sha256.Sum256(input))
			_, err := build(input, p, ac, nil, false)
			require.ErrorContains(t, err, tt.want)
		})
	}
	for _, tt := range []struct {
		name string
		edit func(*params)
	}{
		{"wrong hash", func(p *params) { p.ListSHA256 = "bad" }},
		{"wrong count", func(p *params) { p.ListCount++ }},
		{"missing recovery", func(p *params) { delete(p.RecoveryAddresses, encode(1)) }},
		{"recovery is listed", func(p *params) { p.RecoveryAddresses[encode(1)] = encode(2) }},
		{"payer is listed", func(p *params) { p.FeePayers[0] = encode(1) }},
		{"duplicate payer", func(p *params) { p.FeePayers = append(p.FeePayers, p.FeePayers[0]) }},
		{"no fee payer", func(p *params) { p.FeePayers = nil }},
		{"test values", func(p *params) { p.TestValues = true }},
		{"bad estimate", func(p *params) { p.ActivationHeight += 10_000_000 }},
		{"slow estimate", func(p *params) { p.ActivationTimeEstimate = p.EstimateRefTime + 60_000 }},
		{"no chain id", func(p *params) { p.ChainID = "" }},
		{"testnet chain id", func(p *params) { p.ChainID = "coreum-testnet-1" }},
		{"short unbonding time", func(p *params) { p.UnbondingSeconds-- }},
		{"early expiry", func(p *params) { p.ExpiryTime-- }},
		{"late expiry", func(p *params) { p.ExpiryTime = p.ActivationTimeEstimate + MaxReleaseWindowSeconds + 1 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := valid()
			tt.edit(&p)
			_, err := build(listed, p, ac, nil, false)
			require.Error(t, err)
		})
	}
}

func TestReleaseRefusesReencodedTestAddress(t *testing.T) {
	_, paramsJSON := readTestData(t)
	fixtures, err := parseParams(paramsJSON)
	require.NoError(t, err)
	fixture, err := address.NewBech32Codec("devcore").StringToBytes(fixtures.FeePayers[0])
	require.NoError(t, err)
	ac := address.NewBech32Codec("core")
	mainnetAddress, err := ac.BytesToString(fixture)
	require.NoError(t, err)
	_, err = walletDecoder(ac, nil, false)("fee payer", mainnetAddress)
	require.ErrorContains(t, err, "is a known test address")
}

func TestParamsRejectExplicitNull(t *testing.T) {
	for _, input := range []string{
		`{"enabled":null}`,
		`{"enabled":false,"test_values":null}`,
		`{"enabled":false,"recovery_addresses":null}`,
	} {
		t.Run(input, func(t *testing.T) {
			_, err := parseParams([]byte(input))
			require.ErrorContains(t, err, "must not be null")
		})
	}
}

func TestStrictParams(t *testing.T) {
	for _, json := range []string{
		`{"List_count":3}`, `{"unknown":3}`, `{"list_count":3,"list_count":3}`,
		`{"recovery_addresses":{"wallet":"destination","wallet":"other"}}`,
		`{"fee_payers":[7]}`, `{"list_count":3} {}`,
	} {
		_, err := parseParams([]byte(json))
		require.Error(t, err, "%s", json)
	}
}
