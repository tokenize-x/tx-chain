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

func TestStakingParams_ValidateMaxVotingPower(t *testing.T) {
	// Default (1.0) is valid
	p := DefaultStakingParams()
	require.NoError(t, p.ValidateBasic())

	// 10% cap is valid
	p.MaxVotingPower = sdkmath.LegacyMustNewDecFromStr("0.10")
	require.NoError(t, p.ValidateBasic())

	// Exactly 1% (minimum floor) is valid
	p.MaxVotingPower = sdkmath.LegacyNewDecWithPrec(1, 2)
	require.NoError(t, p.ValidateBasic())

	// Below 1% floor is invalid
	p.MaxVotingPower = sdkmath.LegacyMustNewDecFromStr("0.009")
	require.Error(t, p.ValidateBasic())

	// 0 is invalid (below floor)
	p.MaxVotingPower = sdkmath.LegacyZeroDec()
	require.Error(t, p.ValidateBasic())

	// Negative is invalid
	p.MaxVotingPower = sdkmath.LegacyMustNewDecFromStr("-0.10")
	require.Error(t, p.ValidateBasic())

	// > 1.0 is invalid
	p.MaxVotingPower = sdkmath.LegacyMustNewDecFromStr("1.01")
	require.Error(t, p.ValidateBasic())
}
