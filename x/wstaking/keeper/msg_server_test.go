package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/testutil/simapp"
	customparamstypes "github.com/tokenize-x/tx-chain/v7/x/customparams/types"
)

func setupTestApp(t *testing.T) (*simapp.App, sdk.Context, sdk.AccAddress, *secp256k1.PrivKey, string, sdk.Coin) {
	t.Helper()
	app := simapp.New()
	ctx := app.NewContext(false)

	require.NoError(t, app.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    sdkmath.LegacyOneDec(),
	}))
	require.NoError(t, app.FinalizeBlock())

	accountAddress, privateKey := app.GenAccount(ctx)
	require.NoError(t, app.FinalizeBlock())

	bondDenom, err := app.StakingKeeper.BondDenom(ctx)
	require.NoError(t, err)

	balance := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000_000_000)))
	require.NoError(t, app.FundAccount(ctx, accountAddress, balance))
	require.NoError(t, app.FinalizeBlock())

	feeAmt := sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000))

	return app, ctx, accountAddress, privateKey, bondDenom, feeAmt
}

func Test_WrappedMsgCreateValidatorHandler(t *testing.T) {
	simApp := simapp.New()

	// set min delegation param to 10k
	ctx := simApp.NewContext(false)
	minSelfDelegation := sdkmath.NewInt(10_000)
	require.NoError(t, simApp.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: minSelfDelegation,
		MaxVotingPower:    customparamstypes.DefaultMaxVotingPower,
	}))
	require.NoError(t, simApp.FinalizeBlock())

	// create new account
	accountAddress, privateKey := simApp.GenAccount(ctx)
	require.NoError(t, simApp.FinalizeBlock())

	// fund account
	bondDenom, err := simApp.StakingKeeper.BondDenom(ctx)
	require.NoError(t, err)
	balance := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000_000_000)))
	require.NoError(t, simApp.FundAccount(ctx, accountAddress, balance))
	require.NoError(t, simApp.FinalizeBlock())

	// create validator
	description := stakingtypes.Description{Moniker: "moniker"}
	selfDelegation := sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000_000))
	commission := stakingtypes.CommissionRates{
		Rate:          sdkmath.LegacyZeroDec(),
		MaxRate:       sdkmath.LegacyZeroDec(),
		MaxChangeRate: sdkmath.LegacyZeroDec(),
	}

	feeAmt := sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000))
	gas := uint64(300_000)

	// try to create with insufficient min self delegation
	createValidatorMsg, err := stakingtypes.NewMsgCreateValidator(
		sdk.ValAddress(accountAddress).String(),
		ed25519.GenPrivKey().PubKey(),
		selfDelegation,
		description,
		commission,
		sdkmath.OneInt(),
	)
	require.NoError(t, err)
	_, _, err = simApp.SendTx(ctx, feeAmt, gas, privateKey, createValidatorMsg)
	require.Error(t, err)

	// try to create with min self delegation
	createValidatorMsg, err = stakingtypes.NewMsgCreateValidator(
		sdk.ValAddress(accountAddress).String(),
		ed25519.GenPrivKey().PubKey(),
		selfDelegation,
		description,
		commission,
		minSelfDelegation,
	)
	require.NoError(t, err)
	_, _, err = simApp.SendTx(ctx, feeAmt, gas, privateKey, createValidatorMsg)
	require.NoError(t, err)

	require.NoError(t, simApp.FinalizeBlock())
}

func Test_VotingPowerCap_DelegateExceedsCap(t *testing.T) {
	app, ctx, _, _, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	// Create a second validator with small self-delegation so genesis validator doesn't have 100%
	account2, privateKey2 := app.GenAccount(ctx)
	require.NoError(t, app.FinalizeBlock())
	require.NoError(t, app.FundAccount(ctx, account2, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000_000_000)))))
	require.NoError(t, app.FinalizeBlock())

	createMsg, err := stakingtypes.NewMsgCreateValidator(
		sdk.ValAddress(account2).String(),
		ed25519.GenPrivKey().PubKey(),
		sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000)),
		stakingtypes.Description{Moniker: "val2"},
		stakingtypes.CommissionRates{
			Rate:          sdkmath.LegacyNewDecWithPrec(10, 2),
			MaxRate:       sdkmath.LegacyNewDecWithPrec(20, 2),
			MaxChangeRate: sdkmath.LegacyNewDecWithPrec(1, 2),
		},
		sdkmath.OneInt(),
	)
	require.NoError(t, err)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey2, createMsg)
	require.NoError(t, err)
	require.NoError(t, app.FinalizeBlock())

	// Now set cap to 10%
	require.NoError(t, app.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    sdkmath.LegacyNewDecWithPrec(10, 2),
	}))
	require.NoError(t, app.FinalizeBlock())

	// Get genesis validator
	validators, err := app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, validators)
	genesisValAddr := validators[0].GetOperator()

	// Create a third account to delegate from
	account3, privateKey3 := app.GenAccount(ctx)
	require.NoError(t, app.FinalizeBlock())
	require.NoError(t, app.FundAccount(ctx, account3, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000_000_000)))))
	require.NoError(t, app.FinalizeBlock())

	// Try to delegate a large amount to genesis validator (already has ~99% of tokens)
	delegateMsg := stakingtypes.NewMsgDelegate(
		account3.String(),
		genesisValAddr,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(50_000_000_000)),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey3, delegateMsg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds the max allowed")
}

func Test_VotingPowerCap_DelegateUnderCap(t *testing.T) {
	app, ctx, accountAddress, privateKey, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	// Cap is already 100% from setup (unrestricted)
	validators, err := app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, validators)
	valAddr := validators[0].GetOperator()

	delegateMsg := stakingtypes.NewMsgDelegate(
		accountAddress.String(),
		valAddr,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)),
	)

	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, delegateMsg)
	require.NoError(t, err)
}

func Test_VotingPowerCap_CreateValidatorExceedsCap(t *testing.T) {
	app, ctx, accountAddress, privateKey, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	// Set cap to 50%
	require.NoError(t, app.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    sdkmath.LegacyNewDecWithPrec(50, 2),
	}))
	require.NoError(t, app.FinalizeBlock())

	totalBonded, err := app.StakingKeeper.TotalBondedTokens(ctx)
	require.NoError(t, err)

	// Self-delegation > totalBonded means VP > 50%
	hugeAmount := totalBonded.Add(sdkmath.NewInt(1))

	createMsg, err := stakingtypes.NewMsgCreateValidator(
		sdk.ValAddress(accountAddress).String(),
		ed25519.GenPrivKey().PubKey(),
		sdk.NewCoin(bondDenom, hugeAmount),
		stakingtypes.Description{Moniker: "new-val"},
		stakingtypes.CommissionRates{
			Rate:          sdkmath.LegacyNewDecWithPrec(10, 2),
			MaxRate:       sdkmath.LegacyNewDecWithPrec(20, 2),
			MaxChangeRate: sdkmath.LegacyNewDecWithPrec(1, 2),
		},
		sdkmath.OneInt(),
	)
	require.NoError(t, err)

	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, createMsg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds the max allowed")
}

func Test_VotingPowerCap_RedelegateExceedsCap(t *testing.T) {
	app, ctx, accountAddress, privateKey, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	// Get genesis validator
	validators, err := app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, validators)
	genesisValAddr := validators[0].GetOperator()

	// Create second validator
	account2, privateKey2 := app.GenAccount(ctx)
	require.NoError(t, app.FinalizeBlock())
	require.NoError(t, app.FundAccount(ctx, account2, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000_000_000)))))
	require.NoError(t, app.FinalizeBlock())

	createMsg, err := stakingtypes.NewMsgCreateValidator(
		sdk.ValAddress(account2).String(),
		ed25519.GenPrivKey().PubKey(),
		sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000_000)),
		stakingtypes.Description{Moniker: "val2"},
		stakingtypes.CommissionRates{
			Rate:          sdkmath.LegacyNewDecWithPrec(10, 2),
			MaxRate:       sdkmath.LegacyNewDecWithPrec(20, 2),
			MaxChangeRate: sdkmath.LegacyNewDecWithPrec(1, 2),
		},
		sdkmath.OneInt(),
	)
	require.NoError(t, err)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey2, createMsg)
	require.NoError(t, err)
	require.NoError(t, app.FinalizeBlock())

	// Delegate large amount to validator2
	delegateAmt := sdkmath.NewInt(50_000_000_000)
	delegateMsg := stakingtypes.NewMsgDelegate(
		accountAddress.String(),
		sdk.ValAddress(account2).String(),
		sdk.NewCoin(bondDenom, delegateAmt),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, delegateMsg)
	require.NoError(t, err)
	require.NoError(t, app.FinalizeBlock())

	// Now set cap to 10%
	require.NoError(t, app.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    sdkmath.LegacyNewDecWithPrec(10, 2),
	}))
	require.NoError(t, app.FinalizeBlock())

	// Redelegate from validator2 to genesis validator - should fail (genesis already has > 10%)
	redelegateMsg := stakingtypes.NewMsgBeginRedelegate(
		accountAddress.String(),
		sdk.ValAddress(account2).String(),
		genesisValAddr,
		sdk.NewCoin(bondDenom, delegateAmt),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, redelegateMsg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds the max allowed")
}

func Test_VotingPowerCap_RedelegateSrcEqualsDst(t *testing.T) {
	app, ctx, accountAddress, privateKey, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	// Get genesis validator
	validators, err := app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, validators)
	genesisValAddr := validators[0].GetOperator()

	// Delegate to genesis validator
	delegateMsg := stakingtypes.NewMsgDelegate(
		accountAddress.String(),
		genesisValAddr,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000)),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, delegateMsg)
	require.NoError(t, err)
	require.NoError(t, app.FinalizeBlock())

	// Set cap to 1% (very restrictive)
	require.NoError(t, app.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    sdkmath.LegacyNewDecWithPrec(1, 2),
	}))
	require.NoError(t, app.FinalizeBlock())

	// Redelegate from genesis to genesis (src == dst) - should pass through without VP check
	redelegateMsg := stakingtypes.NewMsgBeginRedelegate(
		accountAddress.String(),
		genesisValAddr,
		genesisValAddr,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, redelegateMsg)
	// SDK may reject src==dst redelegate, but our wrapper should not be the one rejecting it
	if err != nil {
		require.NotContains(t, err.Error(), "exceeds the max allowed")
	}
}

func Test_VotingPowerCap_CreateValidatorUnderCap(t *testing.T) {
	app, ctx, accountAddress, privateKey, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	// Set cap to 50%
	require.NoError(t, app.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    sdkmath.LegacyNewDecWithPrec(50, 2),
	}))
	require.NoError(t, app.FinalizeBlock())

	// Create validator with small self-delegation (well under 50% of total bonded)
	createMsg, err := stakingtypes.NewMsgCreateValidator(
		sdk.ValAddress(accountAddress).String(),
		ed25519.GenPrivKey().PubKey(),
		sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000)),
		stakingtypes.Description{Moniker: "small-val"},
		stakingtypes.CommissionRates{
			Rate:          sdkmath.LegacyNewDecWithPrec(10, 2),
			MaxRate:       sdkmath.LegacyNewDecWithPrec(20, 2),
			MaxChangeRate: sdkmath.LegacyNewDecWithPrec(1, 2),
		},
		sdkmath.OneInt(),
	)
	require.NoError(t, err)

	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, createMsg)
	require.NoError(t, err)
}

func Test_VotingPowerCap_UnrestrictedAt100Percent(t *testing.T) {
	app, ctx, accountAddress, privateKey, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	// Cap is 1.0 (100%) from setup - any delegation should work
	validators, err := app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, validators)
	valAddr := validators[0].GetOperator()

	// Delegate a huge amount - should succeed with 100% cap
	delegateMsg := stakingtypes.NewMsgDelegate(
		accountAddress.String(),
		valAddr,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(500_000_000_000)),
	)

	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, delegateMsg)
	require.NoError(t, err)
}

func Test_VotingPowerCap_CancelUnbondingExceedsCap(t *testing.T) {
	app, ctx, accountAddress, privateKey, bondDenom, feeAmt := setupTestApp(t)
	gas := uint64(300_000)

	validators, err := app.StakingKeeper.GetAllValidators(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, validators)
	genesisValAddr := validators[0].GetOperator()

	// Create second validator
	account2, privateKey2 := app.GenAccount(ctx)
	require.NoError(t, app.FinalizeBlock())
	require.NoError(t, app.FundAccount(ctx, account2, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000_000_000)))))
	require.NoError(t, app.FinalizeBlock())

	createMsg, err := stakingtypes.NewMsgCreateValidator(
		sdk.ValAddress(account2).String(),
		ed25519.GenPrivKey().PubKey(),
		sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000_000)),
		stakingtypes.Description{Moniker: "val2"},
		stakingtypes.CommissionRates{
			Rate:          sdkmath.LegacyNewDecWithPrec(10, 2),
			MaxRate:       sdkmath.LegacyNewDecWithPrec(20, 2),
			MaxChangeRate: sdkmath.LegacyNewDecWithPrec(1, 2),
		},
		sdkmath.OneInt(),
	)
	require.NoError(t, err)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey2, createMsg)
	require.NoError(t, err)
	require.NoError(t, app.FinalizeBlock())

	// Delegate to genesis validator
	delegateAmt := sdkmath.NewInt(50_000_000_000)
	delegateMsg := stakingtypes.NewMsgDelegate(
		accountAddress.String(),
		genesisValAddr,
		sdk.NewCoin(bondDenom, delegateAmt),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, delegateMsg)
	require.NoError(t, err)
	require.NoError(t, app.FinalizeBlock())

	// Undelegate from genesis validator
	undelegateMsg := stakingtypes.NewMsgUndelegate(
		accountAddress.String(),
		genesisValAddr,
		sdk.NewCoin(bondDenom, delegateAmt),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, undelegateMsg)
	require.NoError(t, err)
	require.NoError(t, app.FinalizeBlock())

	// Get the unbonding delegation to find the creation height
	delAddr := accountAddress
	valAddr, err := sdk.ValAddressFromBech32(genesisValAddr)
	require.NoError(t, err)
	ubd, err := app.StakingKeeper.GetUnbondingDelegation(ctx, delAddr, valAddr)
	require.NoError(t, err)
	require.NotEmpty(t, ubd.Entries)
	creationHeight := ubd.Entries[0].CreationHeight

	// Set cap to 10%
	require.NoError(t, app.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: sdkmath.OneInt(),
		MaxVotingPower:    sdkmath.LegacyNewDecWithPrec(10, 2),
	}))
	require.NoError(t, app.FinalizeBlock())

	// Try to cancel unbonding - should fail because it would push genesis validator above 10%
	cancelMsg := stakingtypes.NewMsgCancelUnbondingDelegation(
		accountAddress.String(),
		genesisValAddr,
		creationHeight,
		sdk.NewCoin(bondDenom, delegateAmt),
	)
	_, _, err = app.SendTx(ctx, feeAmt, gas, privateKey, cancelMsg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds the max allowed")
}
