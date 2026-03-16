package types

import (
	sdkmath "cosmossdk.io/math"
	paramtypes "github.com/cosmos/cosmos-sdk/x/params/types"
	"github.com/pkg/errors"
)

// ParamStoreKeyMinSelfDelegation defines the param key for the min_self_delegation param.
var ParamStoreKeyMinSelfDelegation = []byte("minselfdelegation")

// DefaultMaxVotingPower is the default max voting power (1.0 = 100% = unrestricted).
var DefaultMaxVotingPower = sdkmath.LegacyOneDec()

// MinMaxVotingPower is the minimum allowed value for max_voting_power (1% floor).
var MinMaxVotingPower = sdkmath.LegacyNewDecWithPrec(1, 2)

// StakingParamKeyTable returns the parameter key table.
func StakingParamKeyTable() paramtypes.KeyTable {
	return paramtypes.NewKeyTable().RegisterParamSet(&StakingParams{})
}

// DefaultStakingParams returns default staking parameters.
func DefaultStakingParams() StakingParams {
	return StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    DefaultMaxVotingPower,
	}
}

// ParamSetPairs returns the parameter set pairs.
func (p *StakingParams) ParamSetPairs() paramtypes.ParamSetPairs {
	return paramtypes.ParamSetPairs{
		paramtypes.NewParamSetPair(ParamStoreKeyMinSelfDelegation, &p.MinSelfDelegation, validateMinSelfDelegation),
	}
}

// ValidateBasic performs basic validation on staking parameters.
func (p StakingParams) ValidateBasic() error {
	if err := validateMinSelfDelegation(p.MinSelfDelegation); err != nil {
		return err
	}
	return validateMaxVotingPower(p.MaxVotingPower)
}

func validateMinSelfDelegation(i interface{}) error {
	v, ok := i.(sdkmath.Int)
	if !ok {
		return errors.Errorf("invalid parameter type: %T", i)
	}

	if v.IsNil() {
		return errors.New("param min_self_delegation must be not nil")
	}
	if !v.IsPositive() {
		return errors.Errorf("param min_self_delegation must be positive: %s", v)
	}

	return nil
}

func validateMaxVotingPower(v sdkmath.LegacyDec) error {
	if v.IsNil() {
		return errors.New("param max_voting_power must be not nil")
	}
	if v.LT(MinMaxVotingPower) {
		return errors.Errorf("param max_voting_power must be at least %s (1%%): %s", MinMaxVotingPower, v)
	}
	if v.GT(sdkmath.LegacyOneDec()) {
		return errors.Errorf("param max_voting_power must not exceed 1.0 (100%%): %s", v)
	}
	return nil
}
