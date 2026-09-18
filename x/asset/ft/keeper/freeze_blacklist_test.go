package keeper_test

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// withEmergencyFreeze sets the activation height and registers addr in the frozen set for a test.
// It restores both afterwards.
func withEmergencyFreeze(t *testing.T, height int64, addr sdk.AccAddress) {
	t.Helper()
	restoreHeight := constant.FreezeActivationHeight
	constant.FreezeActivationHeight = height
	constant.FrozenAddresses[addr.String()] = struct{}{}
	t.Cleanup(func() {
		constant.FreezeActivationHeight = restoreHeight
		delete(constant.FrozenAddresses, addr.String())
	})
}

func TestKeeper_EmergencyFreeze_BankSend(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})
	bankKeeper := testApp.BankKeeper

	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	const activationHeight = int64(100)
	frozen := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	clean := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	recipient := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	withEmergencyFreeze(t, activationHeight, frozen)

	requireT.NoError(testApp.FundAccount(ctx, frozen, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1_000))))
	requireT.NoError(testApp.FundAccount(ctx, clean, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1_000))))

	send := sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 100))

	// Below H: the frozen account can still send (byte-identical to the release).
	belowCtx := ctx.WithBlockHeight(activationHeight - 1)
	requireT.NoError(bankKeeper.SendCoins(belowCtx, frozen, recipient, send))

	// At H: the frozen account is blocked.
	atCtx := ctx.WithBlockHeight(activationHeight)
	err = bankKeeper.SendCoins(atCtx, frozen, recipient, send)
	requireT.ErrorIs(err, cosmoserrors.ErrUnauthorized)

	// Above H: still blocked.
	aboveCtx := ctx.WithBlockHeight(activationHeight + 5)
	err = bankKeeper.SendCoins(aboveCtx, frozen, recipient, send)
	requireT.ErrorIs(err, cosmoserrors.ErrUnauthorized)

	// A clean account is unaffected at and above H.
	requireT.NoError(bankKeeper.SendCoins(aboveCtx, clean, recipient, send))
}

func TestKeeper_EmergencyFreeze_Delegate(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	const activationHeight = int64(100)
	frozen := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	withEmergencyFreeze(t, activationHeight, frozen)

	requireT.NoError(testApp.FundAccount(ctx, frozen, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1_000_000))))

	// A real validator to delegate to.
	validators, err := testApp.StakingKeeper.GetAllValidators(ctx)
	requireT.NoError(err)
	requireT.NotEmpty(validators)
	valAddr, err := sdk.ValAddressFromBech32(validators[0].OperatorAddress)
	requireT.NoError(err)

	delegateAmount := sdk.NewInt64Coin(bondDenom, 1_000)
	msgServer := stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper)

	// At H: delegation by the frozen account is blocked at the debit hook.
	atCtx := ctx.WithBlockHeight(activationHeight)
	_, err = msgServer.Delegate(atCtx, &stakingtypes.MsgDelegate{
		DelegatorAddress: frozen.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           delegateAmount,
	})
	requireT.ErrorIs(err, cosmoserrors.ErrUnauthorized)

	// Below H: delegation succeeds.
	belowCtx := ctx.WithBlockHeight(activationHeight - 1)
	_, err = msgServer.Delegate(belowCtx, &stakingtypes.MsgDelegate{
		DelegatorAddress: frozen.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           delegateAmount,
	})
	requireT.NoError(err)
}
