package keeper_test

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// TestDepositValidatorRewardsPool_ThroughRouter sends deposits through the message router.
// That is the path group proposals, smart contracts and ICA use, and it skips the ante handler.
func TestDepositValidatorRewardsPool_ThroughRouter(t *testing.T) {
	requireT := require.New(t)

	simApp := simapp.New()
	ctx := simApp.NewContextLegacy(false, tmproto.Header{})

	bondDenom, err := simApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	validators, err := simApp.StakingKeeper.GetAllValidators(ctx)
	requireT.NoError(err)
	requireT.NotEmpty(validators)
	valAddr, err := sdk.ValAddressFromBech32(validators[0].GetOperator())
	requireT.NoError(err)

	const otherDenom = "uother"
	depositor := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	requireT.NoError(simApp.FundAccount(ctx, depositor, sdk.NewCoins(
		sdk.NewInt64Coin(bondDenom, 1_000_000),
		sdk.NewInt64Coin(otherDenom, 1_000_000),
	)))

	deposit := func(coins sdk.Coins) error {
		msg := &distrtypes.MsgDepositValidatorRewardsPool{
			Depositor:        depositor.String(),
			ValidatorAddress: valAddr.String(),
			Amount:           coins,
		}
		handler := simApp.MsgServiceRouter().Handler(msg)
		requireT.NotNil(handler)
		_, err := handler(ctx, msg)
		return err
	}

	outstanding := func() sdk.DecCoins {
		rewards, err := simApp.DistrKeeper.GetValidatorOutstandingRewards(ctx, valAddr)
		requireT.NoError(err)
		return rewards.Rewards
	}

	before := outstanding()

	// A non-bond denom is rejected.
	err = deposit(sdk.NewCoins(sdk.NewInt64Coin(otherDenom, 100)))
	requireT.ErrorIs(err, cosmoserrors.ErrInvalidRequest)

	// A mix of bond and non-bond denoms is rejected too.
	err = deposit(sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 100), sdk.NewInt64Coin(otherDenom, 100)))
	requireT.ErrorIs(err, cosmoserrors.ErrInvalidRequest)

	// Nothing moved.
	requireT.Equal(int64(1_000_000), simApp.BankKeeper.GetBalance(ctx, depositor, otherDenom).Amount.Int64())
	requireT.Equal(int64(1_000_000), simApp.BankKeeper.GetBalance(ctx, depositor, bondDenom).Amount.Int64())
	requireT.Equal(before.String(), outstanding().String())

	// The bond denom is still accepted and reaches the pool.
	requireT.NoError(deposit(sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 100))))
	after := outstanding()
	requireT.True(after.AmountOf(bondDenom).GT(before.AmountOf(bondDenom)))
	requireT.True(after.AmountOf(otherDenom).IsZero())
}
