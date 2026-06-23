//go:build integrationtests

package modules

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v8/integration-tests"
	"github.com/tokenize-x/tx-chain/v8/pkg/client"
	"github.com/tokenize-x/tx-chain/v8/testutil/integration"
	testcontracts "github.com/tokenize-x/tx-chain/v8/x/asset/ft/keeper/test-contracts"
	assetfttypes "github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
)

// TestAssetFTExtensionCommissionBurnRespectsFrozen checks that, on the extension
// transfer path, a holder cannot spend into their frozen balance via the
// commission+burn surcharge: the spendable check must cover amount+commission+burn.
func TestAssetFTExtensionCommissionBurnRespectsFrozen(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)
	ftClient := assetfttypes.NewQueryClient(chain.ClientContext)
	bankClient := banktypes.NewQueryClient(chain.ClientContext)

	issuer := chain.GenAccount()
	user := chain.GenAccount()
	recipient := chain.GenAccount()

	chain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: issuer,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{&assetfttypes.MsgFreeze{}},
				Amount: chain.QueryAssetFTParams(ctx, t).IssueFee.Amount.
					Add(sdkmath.NewInt(1_000_000)). // smart contract upload
					Add(sdkmath.NewInt(1_000_000)), // issue + extension transfer gas
			},
		},
		{
			Acc:     user,
			Options: integration.BalancesOptions{Amount: sdkmath.NewInt(2_000_000)}, // gas for extension sends
		},
	})

	codeID, err := chain.Wasm.DeployWASMContract(
		ctx, chain.TxFactoryAuto(), issuer, testcontracts.AssetExtensionWasm,
	)
	requireT.NoError(err)

	// Token with freezing + extension and 0.5 burn + 0.5 commission, so a transfer
	// debits 2x the sent amount.
	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        issuer.String(),
		Symbol:        "FROZ",
		Subunit:       "ufroz",
		Precision:     6,
		Description:   "extension token with commission and burn",
		InitialAmount: sdkmath.NewInt(10_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_freezing,
			assetfttypes.Feature_extension,
		},
		BurnRate:           sdkmath.LegacyMustNewDecFromStr("0.5"),
		SendCommissionRate: sdkmath.LegacyMustNewDecFromStr("0.5"),
		ExtensionSettings: &assetfttypes.ExtensionIssueSettings{
			CodeId: codeID,
			Funds:  sdk.NewCoins(chain.NewCoin(sdkmath.NewInt(10))),
			Label:  "froz-extension",
		},
	}
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, issuer)

	// Issue and seed the user with 1000 (issuer is admin, so this setup send is not surcharged).
	fundUserSend := &banktypes.MsgSend{
		FromAddress: issuer.String(),
		ToAddress:   user.String(),
		Amount:      sdk.NewCoins(sdk.NewCoin(denom, sdkmath.NewInt(1000))),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(issuer),
		chain.TxFactoryAuto(),
		issueMsg, fundUserSend)
	requireT.NoError(err)

	// Freeze 500 of the user's 1000 → spendable (unfrozen) = 500.
	freezeMsg := &assetfttypes.MsgFreeze{
		Sender:  issuer.String(),
		Account: user.String(),
		Coin:    sdk.NewCoin(denom, sdkmath.NewInt(500)),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(issuer),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(freezeMsg)),
		freezeMsg)
	requireT.NoError(err)

	// Over-debit: sending 500 costs 500 + 250 burn + 250 commission = 1000 > spendable 500 → rejected.
	overDebitSend := &banktypes.MsgSend{
		FromAddress: user.String(),
		ToAddress:   recipient.String(),
		Amount:      sdk.NewCoins(sdk.NewCoin(denom, sdkmath.NewInt(500))),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(user),
		chain.TxFactory().WithGas(500_000),
		overDebitSend)
	requireT.ErrorIs(err, cosmoserrors.ErrInsufficientFunds)

	// Frozen reserve is intact and the user keeps the full balance.
	frozen, err := ftClient.FrozenBalance(ctx, &assetfttypes.QueryFrozenBalanceRequest{
		Account: user.String(), Denom: denom,
	})
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(500), frozen.Balance.Amount)
	userBal, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{Address: user.String(), Denom: denom})
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1000), userBal.Balance.Amount)

	// Within budget: sending 250 costs 250 + 125 + 125 = 500 == spendable → succeeds.
	okSend := &banktypes.MsgSend{
		FromAddress: user.String(),
		ToAddress:   recipient.String(),
		Amount:      sdk.NewCoins(sdk.NewCoin(denom, sdkmath.NewInt(250))),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(user),
		chain.TxFactory().WithGas(500_000),
		okSend)
	requireT.NoError(err)

	// Frozen reserve still intact; balance dropped by the full debit; recipient got the bare amount.
	frozen, err = ftClient.FrozenBalance(ctx, &assetfttypes.QueryFrozenBalanceRequest{
		Account: user.String(), Denom: denom,
	})
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(500), frozen.Balance.Amount)
	userBal, err = bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{Address: user.String(), Denom: denom})
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(500), userBal.Balance.Amount)
	recipientBal, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{Address: recipient.String(), Denom: denom})
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(250), recipientBal.Balance.Amount)
}
