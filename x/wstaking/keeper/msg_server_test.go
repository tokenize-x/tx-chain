package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	customparamstypes "github.com/tokenize-x/tx-chain/v8/x/customparams/types"
	"github.com/tokenize-x/tx-chain/v8/x/wstaking/keeper"
)

func Test_WrappedMsgCreateValidatorHandler(t *testing.T) {
	simApp := simapp.New()

	// set min delegation param to 10k
	ctx := simApp.NewContext(false)
	minSelfDelegation := sdkmath.NewInt(10_000)
	require.NoError(t, simApp.CustomParamsKeeper.SetStakingParams(ctx, customparamstypes.StakingParams{
		MinSelfDelegation: minSelfDelegation,
		MaxVotingPower:    sdkmath.LegacyOneDec(),
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

func Test_MaxVotingPower(t *testing.T) {
	requireT := require.New(t)
	simApp := simapp.New()
	ctx := simApp.NewContext(false).WithBlockHeight(1)
	stakingKeeper := simApp.StakingKeeper

	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)
	coin := func(amount sdkmath.Int) sdk.Coin { return sdk.NewCoin(bondDenom, amount) }

	// d is the stake of the genesis validator G
	validators, err := stakingKeeper.GetBondedValidatorsByPower(ctx)
	requireT.NoError(err)
	requireT.Len(validators, 1)
	valG := sdk.MustValAddressFromBech32(validators[0].GetOperator())
	d := validators[0].Tokens
	requireT.True(d.ModRaw(2).IsZero())

	delegator, _ := simApp.GenAccount(ctx)
	requireT.NoError(simApp.FundAccount(ctx, delegator, sdk.NewCoins(coin(d.MulRaw(100)))))

	// bonded set: G = 2d, A = d, total = 3d
	operatorA, _ := simApp.GenAccount(ctx)
	requireT.NoError(simApp.FundAccount(ctx, operatorA, sdk.NewCoins(coin(d))))
	_, err = simApp.AddValidator(ctx, operatorA, coin(d), nil)
	requireT.NoError(err)
	valA := sdk.ValAddress(operatorA)
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: valG.String(),
		Amount:           coin(d),
	})
	requireT.NoError(err)
	_, err = stakingKeeper.ApplyAndReturnValidatorSetUpdates(ctx)
	requireT.NoError(err)

	operatorC, _ := simApp.GenAccount(ctx)
	requireT.NoError(simApp.FundAccount(ctx, operatorC, sdk.NewCoins(coin(d.MulRaw(10)))))
	pkAny, err := codectypes.NewAnyWithValue(ed25519.GenPrivKey().PubKey())
	requireT.NoError(err)

	msgServer := keeper.NewMsgServerImpl(
		stakingkeeper.NewMsgServerImpl(stakingKeeper), simApp.CustomParamsKeeper, stakingKeeper,
	)
	delegate := func(ctx sdk.Context, val sdk.ValAddress, amount sdkmath.Int) error {
		_, err := msgServer.Delegate(ctx, &stakingtypes.MsgDelegate{
			DelegatorAddress: delegator.String(),
			ValidatorAddress: val.String(),
			Amount:           coin(amount),
		})
		return err
	}
	redelegate := func(ctx sdk.Context, amount sdkmath.Int) error {
		_, err := msgServer.BeginRedelegate(ctx, &stakingtypes.MsgBeginRedelegate{
			DelegatorAddress:    delegator.String(),
			ValidatorSrcAddress: valG.String(),
			ValidatorDstAddress: valA.String(),
			Amount:              coin(amount),
		})
		return err
	}
	cancelUnbonding := func(ctx sdk.Context, amount sdkmath.Int) error {
		_, err := stakingkeeper.NewMsgServerImpl(stakingKeeper).Undelegate(ctx, &stakingtypes.MsgUndelegate{
			DelegatorAddress: delegator.String(),
			ValidatorAddress: valG.String(),
			Amount:           coin(d),
		})
		if err != nil {
			return err
		}
		_, err = msgServer.CancelUnbondingDelegation(ctx, &stakingtypes.MsgCancelUnbondingDelegation{
			DelegatorAddress: delegator.String(),
			ValidatorAddress: valG.String(),
			Amount:           coin(amount),
			CreationHeight:   ctx.BlockHeight(),
		})
		return err
	}
	createValidator := func(ctx sdk.Context, amount sdkmath.Int) error {
		_, err := msgServer.CreateValidator(ctx, &stakingtypes.MsgCreateValidator{
			Description: stakingtypes.Description{Moniker: "C"},
			Commission: stakingtypes.CommissionRates{
				Rate:          sdkmath.LegacyZeroDec(),
				MaxRate:       sdkmath.LegacyZeroDec(),
				MaxChangeRate: sdkmath.LegacyZeroDec(),
			},
			MinSelfDelegation: sdkmath.OneInt(),
			DelegatorAddress:  operatorC.String(),
			ValidatorAddress:  sdk.ValAddress(operatorC).String(),
			Pubkey:            pkAny,
			Value:             coin(amount),
		})
		return err
	}

	jail := func(ctx sdk.Context, vals ...sdk.ValAddress) error {
		for _, val := range vals {
			validator, err := stakingKeeper.GetValidator(ctx, val)
			if err != nil {
				return err
			}
			consAddr, err := validator.GetConsAddr()
			if err != nil {
				return err
			}
			if err := stakingKeeper.Jail(ctx, consAddr); err != nil {
				return err
			}
		}
		_, err := stakingKeeper.ApplyAndReturnValidatorSetUpdates(ctx)
		return err
	}

	for _, tc := range []struct {
		name           string
		maxVotingPower string
		run            func(ctx sdk.Context) error
		allowed        bool
	}{
		// (d + a) / (3d + a) <= 0.5 while a <= d
		{"delegate up to cap", "0.5", func(ctx sdk.Context) error { return delegate(ctx, valA, d) }, true},
		{"delegate over cap", "0.5", func(ctx sdk.Context) error { return delegate(ctx, valA, d.AddRaw(1)) }, false},
		// (2d + 1) / (3d + 1) > 0.5
		{"delegate to validator already over cap", "0.5", func(ctx sdk.Context) error {
			return delegate(ctx, valG, sdkmath.OneInt())
		}, false},
		{"delegate with cap disabled", "1", func(ctx sdk.Context) error { return delegate(ctx, valG, d.MulRaw(10)) }, true},
		// total stays 3d, (d + r) / 3d <= 0.5 while r <= d / 2
		{"redelegate up to cap", "0.5", func(ctx sdk.Context) error { return redelegate(ctx, d.QuoRaw(2)) }, true},
		{"redelegate over cap", "0.5", func(ctx sdk.Context) error { return redelegate(ctx, d.QuoRaw(2).AddRaw(1)) }, false},
		// after undelegating d from G: G = d, total = 2d, (d + c) / (2d + c) <= 0.6 while c <= d / 2
		{"cancel unbonding up to cap", "0.6", func(ctx sdk.Context) error { return cancelUnbonding(ctx, d.QuoRaw(2)) }, true},
		{"cancel unbonding over cap", "0.6", func(ctx sdk.Context) error {
			return cancelUnbonding(ctx, d.QuoRaw(2).AddRaw(1))
		}, false},
		// C is not bonded yet, it is measured as if bonded: c / (3d + c) <= 0.5 while c <= 3d
		{"create validator up to cap", "0.5", func(ctx sdk.Context) error { return createValidator(ctx, d.MulRaw(3)) }, true},
		{"create validator over cap", "0.5", func(ctx sdk.Context) error {
			return createValidator(ctx, d.MulRaw(3).AddRaw(1))
		}, false},
		// A is jailed and measured as if bonded: (d + a) / (2d + d + a) <= 0.5 while a <= d
		{"delegate to jailed validator up to cap", "0.5", func(ctx sdk.Context) error {
			if err := jail(ctx, valA); err != nil {
				return err
			}
			return delegate(ctx, valA, d)
		}, true},
		{"delegate to jailed validator over cap", "0.5", func(ctx sdk.Context) error {
			if err := jail(ctx, valA); err != nil {
				return err
			}
			return delegate(ctx, valA, d.AddRaw(1))
		}, false},
		// gentxs at genesis run before any validator is bonded
		{"create validator with nothing bonded", "0.5", func(ctx sdk.Context) error {
			if err := jail(ctx, valG, valA); err != nil {
				return err
			}
			return createValidator(ctx, d)
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := ctx.CacheContext()
			params := customparamstypes.DefaultStakingParams()
			params.MaxVotingPower = sdkmath.LegacyMustNewDecFromStr(tc.maxVotingPower)
			require.NoError(t, simApp.CustomParamsKeeper.SetStakingParams(ctx, params))

			err := tc.run(ctx)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "exceeds max voting power")
			}
		})
	}
}

func Test_MaxVotingPower_TxReverted(t *testing.T) {
	requireT := require.New(t)
	simApp := simapp.New()
	ctx := simApp.NewContext(false)

	params := customparamstypes.DefaultStakingParams()
	params.MaxVotingPower = sdkmath.LegacyMustNewDecFromStr("0.5")
	requireT.NoError(simApp.CustomParamsKeeper.SetStakingParams(ctx, params))

	validators, err := simApp.StakingKeeper.GetBondedValidatorsByPower(ctx)
	requireT.NoError(err)
	requireT.Len(validators, 1)
	valAddr := validators[0].GetOperator()
	tokensBefore := validators[0].Tokens

	bondDenom, err := simApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)
	delegator, privateKey := simApp.GenAccount(ctx)
	requireT.NoError(simApp.FundAccount(
		ctx, delegator, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(100_000_000_000))),
	))
	requireT.NoError(simApp.FinalizeBlock())

	// the only bonded validator holds 100%, so any delegation to it exceeds the cap
	_, _, err = simApp.SendTx(
		ctx,
		sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000)),
		300_000,
		privateKey,
		&stakingtypes.MsgDelegate{
			DelegatorAddress: delegator.String(),
			ValidatorAddress: valAddr,
			Amount:           sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)),
		},
	)
	requireT.ErrorContains(err, "exceeds max voting power")
	requireT.NoError(simApp.FinalizeBlock())

	ctx = simApp.NewContext(false)
	validator, err := simApp.StakingKeeper.GetValidator(ctx, sdk.MustValAddressFromBech32(valAddr))
	requireT.NoError(err)
	requireT.Equal(tokensBefore.String(), validator.Tokens.String())
}
