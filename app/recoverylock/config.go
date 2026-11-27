// Package recoverylock enforces a height and time bounded policy for listed
// wallets. The configuration is embedded (params.json and listed.txt) and
// validated at startup. The app wires the lock components when the
// configuration sets "enabled" to true and matches the chain.
package recoverylock

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"cosmossdk.io/core/address"
	"cosmossdk.io/log"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
)

//go:embed listed.txt
var listedFile []byte

//go:embed params.json
var paramsFile []byte

// MinReleaseFeePayers is the minimum fee payer set size in release builds.
const MinReleaseFeePayers = 1

// ExpiryMarginSeconds is the margin required in release builds on top of two
// unbonding periods after the estimated activation time:
// expiryTime >= activationTimeEstimate + 2 x unbondingSeconds + 7 days.
// Two periods because the attacker can fill all 7 unbonding entries of a
// delegation before the halt; an undelegation by the recovery operator can
// then only start when the first of those entries completes, up to one
// unbonding period after activation, and completes one period later.
const ExpiryMarginSeconds = 7 * 24 * 3600

// MaxReleaseWindowSeconds bounds expiryTime in release builds:
// expiryTime <= activationTimeEstimate + 365 days, so a mistyped expiry
// cannot keep the listed wallets locked until another binary ships.
const MaxReleaseWindowSeconds = 365 * 24 * 3600

// The release estimate band is a sanity check around the block time of
// coreum-mainnet-1 (about 0.7 seconds per block in November 2026: heights
// 95053554 to 95063554 took 7014 seconds). Use measured mainnet headers for
// releases. MinReleaseUnbondingSeconds is the mainnet staking unbonding_time
// (604800s).
const (
	MinMillisPerBlock          = 300
	MaxMillisPerBlock          = 5000
	MinReleaseUnbondingSeconds = 7 * 24 * 3600
)

// ReleaseChainID is the only chain ID a release build accepts.
const ReleaseChainID = "coreum-mainnet-1"

// Config holds the validated recovery lock settings.
type Config struct {
	recoveries       map[string][]byte
	feePayers        map[string]struct{}
	activationHeight int64
	expiryTime       int64
	chainID          string
}

// The seeded local-chain keys are public and must never enter a release.
func knownTestAddresses() map[string]bool {
	seeds := []string{"recoverylock-test-recovery", "recoverylock-unlisted"}
	for i := range 3 {
		seeds = append(seeds, fmt.Sprintf("recoverylock-test-listed-%d", i))
	}
	for i := range 16 {
		seeds = append(seeds, fmt.Sprintf("recoverylock-test-fee-payer-%d", i))
	}
	out := make(map[string]bool, len(seeds))
	for _, seed := range seeds {
		h := sha256.Sum256([]byte(seed))
		out[string((&secp256k1.PrivKey{Key: h[:]}).PubKey().Address())] = true
	}
	return out
}

type params struct {
	// Enabled false is the committed state: no list, no other field set, and
	// the lock is not wired into the app.
	Enabled bool `json:"enabled"`
	// ChainID is the chain the list belongs to. The binary serves several
	// networks (mainnet, testnet, devnet, with different address prefixes);
	// on a node of another chain the lock is not wired. Release builds
	// require ReleaseChainID.
	ChainID           string            `json:"chainId"`
	RecoveryAddresses map[string]string `json:"recoveryAddresses"`
	FeePayers         []string          `json:"feePayers"`
	ActivationHeight  int64             `json:"activationHeight"`
	ExpiryTime        int64             `json:"expiryTime"`
	// ActivationTimeEstimate is the expected block time (unix seconds) of the
	// activation height; UnbondingSeconds is the chain's live staking
	// unbonding_time. Release builds require
	// expiryTime >= activationTimeEstimate + 2 x unbondingSeconds + 7 days
	// (see ExpiryMarginSeconds).
	//
	// EstimateRefHeight and EstimateRefTime are a real mainnet block height
	// and its block time (unix seconds) taken when the config is built.
	// Release builds require activationHeight > estimateRefHeight and the
	// implied block time (activationTimeEstimate - estimateRefTime) /
	// (activationHeight - estimateRefHeight) within [MinMillisPerBlock,
	// MaxMillisPerBlock], which cross-checks the activation height against
	// the time estimate.
	//
	// Estimate method (at release time): take the latest block (height h0,
	// time t0; these are estimateRefHeight and estimateRefTime) and the
	// block 10000 heights earlier (time t1) from a node; average block time
	// b = (t0 - t1) / 10000; estimate = t0 + (H - h0) * b rounded up to the
	// hour, where H is the activation height. Use the later of this and the
	// planned halt time. The 7 day margin absorbs block time drift and a slow
	// restart at the halt.
	ActivationTimeEstimate int64  `json:"activationTimeEstimate"`
	UnbondingSeconds       int64  `json:"unbondingSeconds"`
	EstimateRefHeight      int64  `json:"estimateRefHeight"`
	EstimateRefTime        int64  `json:"estimateRefTime"`
	ListSHA256             string `json:"listSha256"`
	ListCount              int    `json:"listCount"`
	TestValues             bool   `json:"testValues"`
}

// decodeParams checks case-sensitive field names, duplicate keys, value
// types and trailing data before decoding an embedded manifest.
func decodeParams[T any](b []byte, p *T) error {
	v, err := strictJSON(b)
	if err != nil {
		return err
	}
	top, ok := v.(map[string]any)
	if !ok {
		return errors.New("params must be a JSON object")
	}
	fields := reflect.TypeOf(*p)
	keys := make(map[string]bool, fields.NumField())
	for i := range fields.NumField() {
		keys[strings.Split(fields.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for k, value := range top {
		if !keys[k] {
			return fmt.Errorf("unknown key %q", k)
		}
		if value == nil {
			return fmt.Errorf("key %q must not be null", k)
		}
	}
	return json.Unmarshal(b, p)
}

func parseParams(b []byte) (params, error) {
	var p params
	err := decodeParams(b, &p)
	return p, err
}

func source() (listed, paramsJSON []byte, test bool) {
	if listed, paramsJSON, ok := testConfigOverride(); ok {
		return listed, paramsJSON, true
	}
	return listedFile, paramsFile, testBuild
}

// MustConfig validates the embedded configuration and logs its release
// identifiers. It returns nil when the configuration is disabled or belongs
// to another chain than chainID; the app then wires no lock component. It
// panics on an invalid configuration, so a node with a bad or incomplete list
// does not start. The blocked map must include every module account as a key.
// The list is decoded only on the chain it belongs to, because the address
// codec of another network has another prefix.
func MustConfig(chainID string, ac address.Codec, blocked map[string]bool, logger log.Logger) *Config {
	listed, paramsJSON, test := source()
	p, err := parseParams(paramsJSON)
	if err != nil {
		panic("recovery lock config: params: " + err.Error())
	}
	// An invalid release chain ID must reach build's refusal before the
	// cross-network skip can disable the lock.
	if p.Enabled && p.ChainID != "" && p.ChainID != chainID && (test || p.ChainID == ReleaseChainID) {
		if logger != nil {
			logger.Info("recovery lock not wired: configured for another chain",
				"lock_chain_id", p.ChainID, "chain_id", chainID,
				"params_sha256", fmt.Sprintf("%x", sha256.Sum256(paramsJSON)))
		}
		return nil
	}
	cfg, p, err := parseConfig(listed, paramsJSON, ac, blocked, test)
	if err != nil {
		panic("recovery lock config: " + err.Error())
	}
	mode := "release"
	if test {
		mode = "test"
	}
	paramsSum := fmt.Sprintf("%x", sha256.Sum256(paramsJSON))
	if cfg == nil {
		if logger != nil {
			logger.Info("recovery lock disabled", "params_sha256", paramsSum)
		}
		return nil
	}
	if logger != nil {
		expiry := cfg.expiryTime
		logger.Info("recovery lock config",
			"mode", mode,
			"chain_id", cfg.chainID,
			"activation_height", cfg.activationHeight,
			"expiry_time", expiry,
			"expiry_utc", time.Unix(expiry, 0).UTC().Format(time.RFC3339),
			"recovery_count", len(cfg.recoveries),
			"params_sha256", paramsSum,
			"fee_payers", len(cfg.feePayers),
			"list_count", p.ListCount,
			"list_sha256", strings.ToLower(p.ListSHA256),
		)
	}
	return cfg
}

func (c *Config) isListed(addr []byte) bool {
	_, ok := c.recoveries[string(addr)]
	return ok
}

func (c *Config) isRecovery(wallet, destination []byte) bool {
	recovery, ok := c.recoveries[string(wallet)]
	return ok && bytes.Equal(recovery, destination)
}

func (c *Config) isFeePayer(addr []byte) bool {
	_, ok := c.feePayers[string(addr)]
	return ok
}

// checkEstimate cross-checks the activation height against the activation
// time estimate (release builds).
func checkEstimate(p params) error {
	if p.EstimateRefHeight <= 0 || p.EstimateRefTime <= 0 {
		return errors.New("estimate_ref_height and estimate_ref_time are required")
	}
	if p.ActivationHeight <= p.EstimateRefHeight {
		return fmt.Errorf(
			"activation_height %d must be above estimate_ref_height %d", p.ActivationHeight, p.EstimateRefHeight,
		)
	}
	if p.ActivationTimeEstimate <= p.EstimateRefTime {
		return errors.New("activation_time_estimate must be later than estimate_ref_time")
	}
	dh := p.ActivationHeight - p.EstimateRefHeight
	dt := p.ActivationTimeEstimate - p.EstimateRefTime
	// bounds keep the products below 2^63
	const limit = int64(1) << 40
	if dt >= limit || dh >= limit {
		return fmt.Errorf(
			"activation_time_estimate %d is not plausible after estimate_ref_time %d",
			p.ActivationTimeEstimate, p.EstimateRefTime,
		)
	}
	if dt*1000 < MinMillisPerBlock*dh || dt*1000 > MaxMillisPerBlock*dh {
		return fmt.Errorf(
			"implied block time %d ms ((activation_time_estimate - estimate_ref_time) / "+
				"(activation_height - estimate_ref_height)) is outside %d to %d ms",
			dt*1000/dh, MinMillisPerBlock, MaxMillisPerBlock)
	}
	return nil
}

// parseConfig returns a nil Config without error for a valid disabled
// configuration.
func parseConfig(
	listed, paramsJSON []byte, ac address.Codec, blocked map[string]bool, test bool,
) (*Config, params, error) {
	p, err := parseParams(paramsJSON)
	if err != nil {
		return nil, p, fmt.Errorf("params: %w", err)
	}
	if !p.Enabled {
		return nil, p, checkDisabled(listed, p)
	}
	cfg, err := build(listed, p, ac, blocked, test)
	return cfg, p, err
}

// checkDisabled refuses a disabled configuration that carries data, so a real
// list with "enabled" left false fails at startup instead of running unlocked.
func checkDisabled(listed []byte, p params) error {
	if !reflect.ValueOf(p).IsZero() {
		return errors.New(`params.json sets fields while "enabled" is false`)
	}
	for _, line := range strings.Split(string(listed), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return errors.New(`listed.txt has entries while "enabled" is false`)
		}
	}
	return nil
}

func build(listed []byte, p params, ac address.Codec, blocked map[string]bool, test bool) (*Config, error) {
	if p.ActivationHeight <= 0 || p.ExpiryTime <= 0 {
		return nil, errors.New("activation height and expiry time must be positive")
	}
	if len(p.FeePayers) == 0 || len(p.FeePayers) > 16 {
		return nil, errors.New("need 1 to 16 fee payers")
	}
	if p.ChainID == "" {
		return nil, errors.New("chain_id is required")
	}

	if !test {
		if err := checkReleaseParams(p); err != nil {
			return nil, err
		}
	}
	if err := checkListHash("listed.txt", listed, p.ListSHA256); err != nil {
		return nil, err
	}
	decode := walletDecoder(ac, blocked, test)
	cfg := &Config{
		recoveries:       make(map[string][]byte),
		feePayers:        make(map[string]struct{}, len(p.FeePayers)),
		activationHeight: p.ActivationHeight,
		expiryTime:       p.ExpiryTime,
		chainID:          p.ChainID,
	}
	lines := strings.Split(string(listed), "\n")
	wallets := make([][]byte, 0, len(lines))
	listedKeys := make(map[string]struct{}, len(lines))
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		wallet, err := decode(fmt.Sprintf("list line %d", i+1), line)
		if err != nil {
			return nil, err
		}
		if _, duplicate := listedKeys[string(wallet)]; duplicate {
			return nil, fmt.Errorf("duplicate listed wallet on line %d", i+1)
		}
		listedKeys[string(wallet)] = struct{}{}
		wallets = append(wallets, wallet)
	}
	if len(wallets) == 0 || len(wallets) != p.ListCount {
		return nil, fmt.Errorf("listed.txt has %d wallets, expected a nonempty list of %d", len(wallets), p.ListCount)
	}
	if len(p.RecoveryAddresses) != len(wallets) {
		return nil, errors.New("recovery_addresses must have one entry per listed wallet")
	}
	for _, wallet := range wallets {
		canonical, err := ac.BytesToString(wallet)
		if err != nil {
			return nil, err
		}
		recovery, err := decode("recovery address for "+canonical, p.RecoveryAddresses[canonical])
		if err != nil {
			return nil, err
		}
		if _, listed := listedKeys[string(recovery)]; listed {
			return nil, errors.New("a recovery address cannot be a listed wallet")
		}
		cfg.recoveries[string(wallet)] = recovery
	}
	for i, value := range p.FeePayers {
		payer, err := decode(fmt.Sprintf("fee payer %d", i), value)
		if err != nil {
			return nil, err
		}
		key := string(payer)
		if cfg.isListed(payer) || cfg.isFeePayer(payer) {
			return nil, fmt.Errorf("fee payer %d overlaps a listed wallet or another fee payer", i)
		}
		cfg.feePayers[key] = struct{}{}
	}
	return cfg, nil
}

func checkReleaseParams(p params) error {
	if p.TestValues {
		return errors.New("test values outside a recoverylock_test build")
	}
	if p.ChainID != ReleaseChainID {
		return fmt.Errorf("release builds need chain_id %s, got %q", ReleaseChainID, p.ChainID)
	}
	if len(p.FeePayers) < MinReleaseFeePayers {
		return fmt.Errorf("release builds need at least %d fee payers, got %d", MinReleaseFeePayers, len(p.FeePayers))
	}
	if p.ActivationTimeEstimate <= 0 {
		return errors.New("activation_time_estimate is required")
	}
	if p.UnbondingSeconds < MinReleaseUnbondingSeconds {
		return fmt.Errorf(
			"unbonding_seconds %d is below the mainnet unbonding time %d", p.UnbondingSeconds, MinReleaseUnbondingSeconds,
		)
	}
	if p.UnbondingSeconds > 365*24*3600 {
		return fmt.Errorf("unbonding_seconds %d is not plausible", p.UnbondingSeconds)
	}
	if err := checkEstimate(p); err != nil {
		return err
	}
	if p.ExpiryTime-p.ActivationTimeEstimate < 2*p.UnbondingSeconds+ExpiryMarginSeconds {
		return errors.New("expiry_time must allow two unbonding periods and seven days after the activation estimate")
	}
	if p.ExpiryTime-p.ActivationTimeEstimate > MaxReleaseWindowSeconds {
		return errors.New("expiry_time must be at most one year after the activation estimate")
	}
	return nil
}

func checkListHash(name string, data []byte, expected string) error {
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, expected) {
		return fmt.Errorf("%s sha256 %s does not match params %s", name, got, expected)
	}
	return nil
}

func walletDecoder(ac address.Codec, blocked map[string]bool, test bool) func(string, string) ([]byte, error) {
	known := knownTestAddresses()
	return func(what, s string) ([]byte, error) {
		b, err := ac.StringToBytes(s)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", what, s, err)
		}
		if len(b) != 20 {
			return nil, fmt.Errorf("%s %q has %d bytes, want 20", what, s, len(b))
		}
		if !test && known[string(b)] {
			return nil, fmt.Errorf("%s %q is a known test address", what, s)
		}
		canon, err := ac.BytesToString(b)
		if err != nil {
			return nil, err
		}
		if _, isBlocked := blocked[canon]; isBlocked {
			return nil, fmt.Errorf("%s %q is a module account or blocked address", what, s)
		}
		return b, nil
	}
}
