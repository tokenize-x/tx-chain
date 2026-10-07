package types

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

func TestStakingParams_ValidateBasic(t *testing.T) {
	p := DefaultStakingParams()
	require.NoError(t, p.ValidateBasic())

	p.MinSelfDelegation = sdkmath.NewInt(-1)
	require.Error(t, p.ValidateBasic())
}

func TestStakingParams_ValidateBasic_MaxVotingPower(t *testing.T) {
	for _, tc := range []struct {
		value sdkmath.LegacyDec
		valid bool
	}{
		{value: sdkmath.LegacyDec{}, valid: false},
		{value: sdkmath.LegacyZeroDec(), valid: false},
		{value: sdkmath.LegacyMustNewDecFromStr("0.0099"), valid: false},
		{value: sdkmath.LegacyMustNewDecFromStr("0.01"), valid: true},
		{value: sdkmath.LegacyMustNewDecFromStr("0.1"), valid: true},
		{value: sdkmath.LegacyOneDec(), valid: true},
		{value: sdkmath.LegacyMustNewDecFromStr("1.01"), valid: false},
	} {
		p := DefaultStakingParams()
		p.MaxVotingPower = tc.value
		if tc.valid {
			require.NoError(t, p.ValidateBasic(), tc.value.String())
		} else {
			require.Error(t, p.ValidateBasic(), tc.value.String())
		}
	}
}
