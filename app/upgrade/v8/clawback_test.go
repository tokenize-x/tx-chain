package v8_test

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"github.com/stretchr/testify/require"

	v8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// TestClawbackTransfers_ValidAddresses guards against a typo in a configured address.
// A malformed entry would be skipped at the upgrade height and the funds would stay put.
func TestClawbackTransfers_ValidAddresses(t *testing.T) {
	requireT := require.New(t)

	prefixes := map[constant.ChainID]string{
		constant.ChainIDMain: constant.AddressPrefixMain,
		constant.ChainIDTest: constant.AddressPrefixTest,
		constant.ChainIDDev:  constant.AddressPrefixDev,
	}

	requireT.NotEmpty(v8.ClawbackTransfers)
	for chainID, transfers := range v8.ClawbackTransfers {
		requireT.NotEmptyf(transfers, "chain %s has no transfers", chainID)

		prefix, ok := prefixes[chainID]
		requireT.Truef(ok, "unknown chain %s", chainID)

		for _, transfer := range transfers {
			for _, addr := range []string{transfer.From, transfer.To} {
				hrp, _, err := bech32.DecodeAndConvert(addr)
				requireT.NoErrorf(err, "%q is not valid bech32", addr)
				requireT.Equalf(prefix, hrp, "%q must use the %s prefix", addr, chainID)
			}
			requireT.NotEqual(transfer.From, transfer.To)
		}
	}
}

// TestClawbackFrozenFunds_MovesFundsDespiteFreeze is the point of the whole handler.
// The source account is frozen, so a normal bank send is rejected, and the clawback must still move the funds.
func TestClawbackFrozenFunds_MovesFundsDespiteFreeze(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	from := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	to := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())

	restoreHeight := constant.FreezeActivationHeight
	constant.FreezeActivationHeight = 1
	constant.FrozenAddresses[from.String()] = struct{}{}
	t.Cleanup(func() {
		constant.FreezeActivationHeight = restoreHeight
		delete(constant.FrozenAddresses, from.String())
	})

	funded := sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1_000_000), sdk.NewInt64Coin("uother", 7))
	requireT.NoError(testApp.FundAccount(ctx, from, funded))

	ctx = ctx.WithBlockHeight(10)

	// The freeze must reject a normal send, otherwise this test proves nothing.
	err = testApp.BankKeeper.SendCoins(ctx, from, to, sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 1)))
	requireT.Error(err)

	v8.ClawbackFrozenFunds(ctx, testApp.BankKeeper, []v8.ClawbackTransfer{{From: from.String(), To: to.String()}})

	requireT.True(testApp.BankKeeper.GetAllBalances(ctx, from).IsZero())
	requireT.Equal(funded.String(), testApp.BankKeeper.GetAllBalances(ctx, to).String())

	var found bool
	for _, event := range ctx.EventManager().Events() {
		if event.Type != v8.EventTypeClawback {
			continue
		}
		found = true
		for _, attr := range event.Attributes {
			requireT.NotEqual("error", attr.Key)
			if attr.Key == "amount" {
				requireT.Equal(funded.String(), attr.Value)
			}
		}
	}
	requireT.True(found, "clawback event must be emitted")
}

// TestClawbackFrozenFunds_EmptyAccount covers an account that holds nothing at the upgrade height.
func TestClawbackFrozenFunds_EmptyAccount(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	from := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	to := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())

	v8.ClawbackFrozenFunds(ctx, testApp.BankKeeper, []v8.ClawbackTransfer{{From: from.String(), To: to.String()}})

	requireT.True(testApp.BankKeeper.GetAllBalances(ctx, to).IsZero())
}

// TestClawbackFrozenFunds_BadAddressDoesNotStopTheRest covers the log-and-continue behaviour.
// A broken entry must not prevent the remaining transfers from running.
func TestClawbackFrozenFunds_BadAddressDoesNotStopTheRest(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	stakingParams, err := testApp.StakingKeeper.GetParams(ctx)
	requireT.NoError(err)
	bondDenom := stakingParams.BondDenom

	from := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	to := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	funded := sdk.NewCoins(sdk.NewInt64Coin(bondDenom, 500))
	requireT.NoError(testApp.FundAccount(ctx, from, funded))

	v8.ClawbackFrozenFunds(ctx, testApp.BankKeeper, []v8.ClawbackTransfer{
		{From: "core1notavalidaddress", To: to.String()},
		{From: from.String(), To: to.String()},
	})

	requireT.Equal(funded.String(), testApp.BankKeeper.GetAllBalances(ctx, to).String())
}
