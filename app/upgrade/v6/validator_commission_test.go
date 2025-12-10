package v6_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
)

func TestMigrateValidatorCommission(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now())

	// Set minCommissionRate to 0 so we can create validators with any commission rates
	requireT.NoError(v6.SetMinCommissionRate(ctx, testApp.StakingKeeper, sdkmath.LegacyZeroDec()))

	// Get bond denom for staking
	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	stakeAmount := sdk.NewCoin(stakingParams.BondDenom, sdkmath.NewInt(1_000_000))

	// Create operators and fund them
	operator1 := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	operator2 := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	operator3 := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	requireT.NoError(testApp.FundAccount(ctx, operator1, sdk.NewCoins(stakeAmount)))
	requireT.NoError(testApp.FundAccount(ctx, operator2, sdk.NewCoins(stakeAmount)))
	requireT.NoError(testApp.FundAccount(ctx, operator3, sdk.NewCoins(stakeAmount)))

	// Val1: 1% commission, 20% max rate (commission below future minimum)
	val1, err := testApp.AddValidator(ctx, operator1, stakeAmount, &stakingtypes.CommissionRates{
		Rate:    sdkmath.LegacyNewDecWithPrec(1, 2),
		MaxRate: sdkmath.LegacyNewDecWithPrec(20, 2),
	})
	requireT.NoError(err)

	// Val2: 10% commission, 20% max rate (commission above future minimum, no change expected)
	val2, err := testApp.AddValidator(ctx, operator2, stakeAmount, &stakingtypes.CommissionRates{
		Rate:    sdkmath.LegacyNewDecWithPrec(10, 2),
		MaxRate: sdkmath.LegacyNewDecWithPrec(20, 2),
	})
	requireT.NoError(err)

	// Val3: 3% commission, 4% max rate (both below future minimum)
	val3, err := testApp.AddValidator(ctx, operator3, stakeAmount, &stakingtypes.CommissionRates{
		Rate:    sdkmath.LegacyNewDecWithPrec(3, 2),
		MaxRate: sdkmath.LegacyNewDecWithPrec(4, 2),
	})
	requireT.NoError(err)

	// Set minimum commission rate to 5%
	minCommissionRate := sdkmath.LegacyNewDecWithPrec(5, 2)
	requireT.NoError(v6.SetMinCommissionRate(ctx, testApp.StakingKeeper, minCommissionRate))

	// Run migration
	err = v6.MigrateValidatorCommission(ctx, testApp.StakingKeeper)
	requireT.NoError(err)

	// Val1: commission should be updated to 5%, max rate unchanged at 20%
	val1Addr, err := testApp.StakingKeeper.ValidatorAddressCodec().StringToBytes(val1.OperatorAddress)
	requireT.NoError(err)
	validator1, err := testApp.StakingKeeper.GetValidator(ctx, val1Addr)
	requireT.NoError(err)
	requireT.Equal(minCommissionRate.String(), validator1.Commission.Rate.String())
	requireT.Equal(sdkmath.LegacyNewDecWithPrec(20, 2).String(), validator1.Commission.MaxRate.String())

	// Val2: both commission and max rate should be unchanged
	val2Addr, err := testApp.StakingKeeper.ValidatorAddressCodec().StringToBytes(val2.OperatorAddress)
	requireT.NoError(err)
	validator2, err := testApp.StakingKeeper.GetValidator(ctx, val2Addr)
	requireT.NoError(err)
	requireT.Equal(sdkmath.LegacyNewDecWithPrec(10, 2).String(), validator2.Commission.Rate.String())
	requireT.Equal(sdkmath.LegacyNewDecWithPrec(20, 2).String(), validator2.Commission.MaxRate.String())

	// Val3: both commission and max rate should be updated to 5%
	val3Addr, err := testApp.StakingKeeper.ValidatorAddressCodec().StringToBytes(val3.OperatorAddress)
	requireT.NoError(err)
	validator3, err := testApp.StakingKeeper.GetValidator(ctx, val3Addr)
	requireT.NoError(err)
	requireT.Equal(minCommissionRate.String(), validator3.Commission.Rate.String())
	requireT.Equal(minCommissionRate.String(), validator3.Commission.MaxRate.String())
}
