package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestKeeper_Hooks(t *testing.T) {
	cases := []struct {
		name    string
		actions []func(*runEnv)
	}{
		{
			name: "new delegation added, single validator",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 11) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 9) },
				func(r *runEnv) { assertScoreAction(r, r.delegators[0], sdkmath.NewInt(11*8)) },
			},
		},
		{
			name: "new delegation added multiple times",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 12) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 9) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 11) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 13) },
				func(r *runEnv) { waitAction(r, time.Second*5) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(12*8+9*8+(11+12)*5+(9+13)*5))
				},
			},
		},
		{
			name: "delegation reduced",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 12) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 9) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[0], 7) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[1], 5) },
				func(r *runEnv) { waitAction(r, time.Second*5) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(12*8+9*8+(12-7)*5+(9-5)*5))
				},
			},
		},
		{
			name: "delegation removed",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 12) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 9) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[0], 12) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[1], 9) },
				func(r *runEnv) { waitAction(r, time.Second*5) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(12*8+9*8))
				},
			},
		},
		{
			name: "new delegation after delegation removed",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 12) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 9) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[0], 12) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[1], 9) },
				func(r *runEnv) { waitAction(r, time.Second*5) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 9) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 14) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(12*8+9*8)) // = 168
				},
				func(r *runEnv) { waitAction(r, time.Second*7) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(12*8+9*8+9*7+14*7))
				},
			},
		},
		{
			name: "full redelegation",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 11) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 17) },
				func(r *runEnv) { waitAction(r, time.Second*7) },
				func(r *runEnv) { redelegateAction(r, r.delegators[0], r.validators[0], r.validators[2], 11) },
				func(r *runEnv) { redelegateAction(r, r.delegators[0], r.validators[1], r.validators[3], 17) },
				func(r *runEnv) { waitAction(r, time.Second*5) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[2], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[3], 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(11*7+17*7+11*5+17*5))
				},
			},
		},
		{
			name: "partial redelegation",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 11) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 17) },
				func(r *runEnv) { waitAction(r, time.Second*7) },
				func(r *runEnv) { redelegateAction(r, r.delegators[0], r.validators[0], r.validators[2], 5) },
				func(r *runEnv) { redelegateAction(r, r.delegators[0], r.validators[1], r.validators[3], 9) },
				func(r *runEnv) { waitAction(r, time.Second*5) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[2], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[3], 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(11*7+17*7+11*5+17*5))
				},
			},
		},
		{
			name: "cancel unbonding delegation",
			actions: []func(*runEnv){
				func(r *runEnv) { waitAction(r, time.Second) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 12) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 9) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[0], 10) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[1], 7) },
				func(r *runEnv) { waitAction(r, time.Second*5) },
				func(r *runEnv) { cancelUnbondingDelegationAction(r, r.delegators[0], r.validators[0], 5, 1) },
				func(r *runEnv) { cancelUnbondingDelegationAction(r, r.delegators[0], r.validators[1], 6, 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt((12-10)*5+(9-7)*5)) // = 20
				},
				func(r *runEnv) { waitAction(r, time.Second*6) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1) },
				func(r *runEnv) {
					assertScoreAction(r, r.delegators[0],
						sdkmath.NewInt(20+7*6+8*6))
				},
			},
		},
		{
			name: "new delegation with time rounding up",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 11) },
				func(r *runEnv) { waitAction(r, time.Second*8+time.Millisecond) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 9) },
				func(r *runEnv) { assertScoreAction(r, r.delegators[0], sdkmath.NewInt(11*8)) },
			},
		},
		{
			name: "new delegation with time rounding down",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 11) },
				func(r *runEnv) { waitAction(r, time.Second*8-time.Millisecond) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 9) },
				func(r *runEnv) { assertScoreAction(r, r.delegators[0], sdkmath.NewInt(11*7)) },
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			testApp := simapp.New()
			ctx := testApp.NewContext(false)
			runContext := &runEnv{
				testApp:  testApp,
				ctx:      ctx,
				requireT: requireT,
			}

			// add validators.
			for range 4 {
				validatorOperator, _ := testApp.GenAccount(ctx)
				requireT.NoError(testApp.FundAccount(
					ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000)))),
				)
				validator, err := addValidator(
					ctx, testApp, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10),
				)
				requireT.NoError(err)
				runContext.validators = append(
					runContext.validators,
					sdk.MustValAddressFromBech32(validator.GetOperator()),
				)
			}

			// add delegators.
			for range 3 {
				delegator, _ := testApp.GenAccount(ctx)
				requireT.NoError(testApp.FundAccount(
					ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
				))
				runContext.delegators = append(runContext.delegators, delegator)
			}

			// run actions.
			for _, action := range tc.actions {
				action(runContext)
			}
		})
	}
}

type runEnv struct {
	testApp    *simapp.App
	ctx        sdk.Context
	delegators []sdk.AccAddress
	validators []sdk.ValAddress
	requireT   *require.Assertions
}

func assertScoreAction(r *runEnv, delAddr sdk.AccAddress, expectedScore sdkmath.Int) {
	score, err := r.testApp.PSEKeeper.AccountScoreSnapshot.Get(
		r.ctx, delAddr,
	)
	r.requireT.NoError(err)
	r.requireT.Equal(expectedScore, score)
}

func assertDistributionAction(r *runEnv, balances map[*sdk.AccAddress]sdkmath.Int) {
	stakingQuerier := stakingkeeper.NewQuerier(r.testApp.StakingKeeper)
	for addr, expectedBalance := range balances {
		delegationsRsp, err := stakingQuerier.DelegatorDelegations(r.ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
			DelegatorAddr: addr.String(),
		})
		r.requireT.NoError(err)
		totalDelegationAmount := sdkmath.NewInt(0)
		for _, delegation := range delegationsRsp.DelegationResponses {
			totalDelegationAmount = totalDelegationAmount.Add(delegation.Balance.Amount)
		}
		r.requireT.Equal(expectedBalance, totalDelegationAmount)
	}
}

func delegateAction(r *runEnv, delAddr sdk.AccAddress, valAddr sdk.ValAddress, amount int64) {
	mintAndSendCoin(r, delAddr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(amount))))
	msg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, amount),
	}
	_, err := stakingkeeper.NewMsgServerImpl(r.testApp.StakingKeeper).Delegate(r.ctx, msg)
	r.requireT.NoError(err)
}

func undelegateAction(r *runEnv, delAddr sdk.AccAddress, valAddr sdk.ValAddress, amount int64) {
	msg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, amount),
	}
	_, err := stakingkeeper.NewMsgServerImpl(r.testApp.StakingKeeper).Undelegate(r.ctx, msg)
	r.requireT.NoError(err)
}

func cancelUnbondingDelegationAction(
	r *runEnv,
	delAddr sdk.AccAddress,
	valAddr sdk.ValAddress,
	amount int64,
	creationHeight int64,
) {
	msg := &stakingtypes.MsgCancelUnbondingDelegation{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, amount),
		CreationHeight:   creationHeight,
	}
	_, err := stakingkeeper.NewMsgServerImpl(r.testApp.StakingKeeper).CancelUnbondingDelegation(r.ctx, msg)
	r.requireT.NoError(err)
}

func redelegateAction(
	r *runEnv,
	delAddr sdk.AccAddress,
	oldValAddr sdk.ValAddress,
	newValAddr sdk.ValAddress,
	amount int64,
) {
	msg := &stakingtypes.MsgBeginRedelegate{
		DelegatorAddress:    delAddr.String(),
		ValidatorSrcAddress: oldValAddr.String(),
		ValidatorDstAddress: newValAddr.String(),
		Amount:              sdk.NewInt64Coin(sdk.DefaultBondDenom, amount),
	}
	_, err := stakingkeeper.NewMsgServerImpl(r.testApp.StakingKeeper).BeginRedelegate(r.ctx, msg)
	r.requireT.NoError(err)
}

func waitAction(r *runEnv, duration time.Duration) {
	ctx, _, err := r.testApp.BeginNextBlockAtTime(r.ctx.BlockTime().Add(duration))
	r.requireT.NoError(err)
	r.ctx = ctx
}

func distributeAction(r *runEnv, amount sdkmath.Int) {
	mintAndSendToPSECommunityClearingAccount(r, amount)
	err := r.testApp.PSEKeeper.DistributeCommunityPSE(r.ctx, amount)
	r.requireT.NoError(err)
}

func mintAndSendCoin(r *runEnv, recipient sdk.AccAddress, coins sdk.Coins) {
	r.requireT.NoError(
		r.testApp.BankKeeper.MintCoins(r.ctx, minttypes.ModuleName, coins),
	)
	r.requireT.NoError(
		r.testApp.BankKeeper.SendCoinsFromModuleToAccount(r.ctx, minttypes.ModuleName, recipient, coins),
	)
}

func mintAndSendToPSECommunityClearingAccount(r *runEnv, amount sdkmath.Int) {
	bondDenom, err := r.testApp.StakingKeeper.BondDenom(r.ctx)
	r.requireT.NoError(err)
	distributeCoin := sdk.NewCoin(bondDenom, amount)

	macc := r.testApp.AccountKeeper.GetModuleAccount(r.ctx, types.ClearingAccountCommunity)

	r.requireT.NoError(r.testApp.BankKeeper.MintCoins(
		r.ctx, minttypes.ModuleName, sdk.NewCoins(distributeCoin),
	))
	r.requireT.NoError(r.testApp.BankKeeper.SendCoinsFromModuleToModule(
		r.ctx, minttypes.ModuleName, macc.GetName(), sdk.NewCoins(distributeCoin),
	))
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
