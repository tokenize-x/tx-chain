package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
)

func TestKeeper_Hooks_AfterDelegationModified(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)

	pseKeeper := testApp.PSEKeeper

	// create accounts and fund them.
	delAddr := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	validatorOperator, _ := testApp.GenAccount(ctx)
	requireT.NoError(
		testApp.FundAccount(ctx, delAddr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(100)))),
	)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(100)))),
	)

	// create validator.
	validator, err := addValidator(ctx, testApp, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10))
	requireT.NoError(err)
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(time.Second * 10))
	requireT.NoError(err)

	// delegate twice within 8 seconds. the second delegation should increase the score.
	requireT.NoError(sendDelegateMsg(
		ctx,
		testApp,
		delAddr,
		sdk.MustValAddressFromBech32(validator.GetOperator()),
		sdk.NewInt64Coin(sdk.DefaultBondDenom, 11),
	))
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(time.Second * 8))
	requireT.NoError(err)
	requireT.NoError(sendDelegateMsg(
		ctx,
		testApp,
		delAddr,
		sdk.MustValAddressFromBech32(validator.GetOperator()),
		sdk.NewInt64Coin(sdk.DefaultBondDenom, 9),
	))

	score, err := pseKeeper.AccountScore.Get(ctx, delAddr)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(11*8), score)
}

func sendDelegateMsg(
	ctx sdk.Context,
	testApp *simapp.App,
	delAddr sdk.AccAddress,
	valAddr sdk.ValAddress,
	amount sdk.Coin,
) error {
	msg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           amount,
	}
	_, err := stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, msg)
	return err
}

func addValidator(
	ctx sdk.Context,
	testApp *simapp.App,
	operator sdk.AccAddress,
	value sdk.Coin,
) (val stakingtypes.Validator, err error) {
	stakingKeeper := testApp.StakingKeeper
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()
	valAddr := sdk.ValAddress(operator)

	pkAny, err := codectypes.NewAnyWithValue(pubKey)
	if err != nil {
		return stakingtypes.Validator{}, err
	}
	msg := &stakingtypes.MsgCreateValidator{
		Description: stakingtypes.Description{
			Moniker: "Validator power",
		},
		Commission: stakingtypes.CommissionRates{
			Rate:          sdkmath.LegacyMustNewDecFromStr("0.1"),
			MaxRate:       sdkmath.LegacyMustNewDecFromStr("0.2"),
			MaxChangeRate: sdkmath.LegacyMustNewDecFromStr("0.01"),
		},
		MinSelfDelegation: sdkmath.OneInt(),
		DelegatorAddress:  operator.String(),
		ValidatorAddress:  valAddr.String(),
		Pubkey:            pkAny,
		Value:             value,
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).CreateValidator(ctx, msg)
	if err != nil {
		return stakingtypes.Validator{}, err
	}

	return stakingKeeper.GetValidator(ctx, valAddr)
}
