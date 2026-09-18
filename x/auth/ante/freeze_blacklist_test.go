package ante_test

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v8/x/auth/ante"
)

func TestBlacklistedSignersDecorator(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	const activationHeight = int64(100)

	frozen, frozenPriv := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, frozen, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1_000_000))))

	restoreHeight := constant.FreezeActivationHeight
	constant.FreezeActivationHeight = activationHeight
	constant.FrozenAddresses[frozen.String()] = struct{}{}
	t.Cleanup(func() {
		constant.FreezeActivationHeight = restoreHeight
		delete(constant.FrozenAddresses, frozen.String())
	})

	recipient, _ := testApp.GenAccount(ctx)
	msg := banktypes.NewMsgSend(frozen, recipient, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1)))
	tx, err := testApp.GenTx(ctx, sdk.NewInt64Coin(bondDenom, 1_000), 200_000, frozenPriv, msg)
	requireT.NoError(err)

	decorator := ante.NewBlacklistedSignersDecorator()
	nextCalled := false
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		nextCalled = true
		return ctx, nil
	}

	// At H: the frozen signer's tx is rejected early and next is not reached.
	_, err = decorator.AnteHandle(ctx.WithBlockHeight(activationHeight), tx, false, next)
	requireT.ErrorIs(err, cosmoserrors.ErrUnauthorized)
	requireT.False(nextCalled)

	// Below H: the decorator is a no-op and passes to next.
	nextCalled = false
	_, err = decorator.AnteHandle(ctx.WithBlockHeight(activationHeight-1), tx, false, next)
	requireT.NoError(err)
	requireT.True(nextCalled)
}

func TestBlacklistedSignersDecorator_CleanSignerUnaffected(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	restoreHeight := constant.FreezeActivationHeight
	constant.FreezeActivationHeight = int64(100)
	t.Cleanup(func() { constant.FreezeActivationHeight = restoreHeight })

	clean, cleanPriv := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, clean, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1_000_000))))

	recipient, _ := testApp.GenAccount(ctx)
	msg := banktypes.NewMsgSend(clean, recipient, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1)))
	tx, err := testApp.GenTx(ctx, sdk.NewInt64Coin(bondDenom, 1_000), 200_000, cleanPriv, msg)
	requireT.NoError(err)

	decorator := ante.NewBlacklistedSignersDecorator()
	nextCalled := false
	next := func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		nextCalled = true
		return ctx, nil
	}

	// Even above H, a clean signer passes through untouched.
	_, err = decorator.AnteHandle(ctx.WithBlockHeight(200), tx, false, next)
	requireT.NoError(err)
	requireT.True(nextCalled)
}
