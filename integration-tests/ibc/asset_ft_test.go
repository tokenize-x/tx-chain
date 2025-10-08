//go:build integrationtests

package ibc

import (
	"context"
	"strings"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	icatypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcchanneltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tokenize-x/tx-tools/pkg/retry"

	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	"github.com/tokenize-x/tx-chain/v6/pkg/client"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
	assetfttypes "github.com/tokenize-x/tx-chain/v6/x/asset/ft/types"
)

func TestIBCFailsIfNotEnabled(t *testing.T) {
	t.Parallel()

	requireT := require.New(t)

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	txChain := chains.TXChain
	txIssuer := txChain.GenAccount()

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount
	txChain.FundAccountWithOptions(ctx, t, txIssuer, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&assetfttypes.MsgIssue{},
			&ibctransfertypes.MsgTransfer{},
		},
		Amount: issueFee,
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)

	gaiaChain := chains.Gaia
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txIssuer,
		sdk.NewCoin(assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer), sdkmath.NewInt(1000)),
		gaiaChain.ChainContext,
		gaiaChain.GenAccount(),
	)
	requireT.ErrorContains(err, "unauthorized")
}

func TestIBCAssetFTSendCommissionAndBurnRate(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)

	txChain := chains.TXChain
	gaiaChain := chains.Gaia
	osmosisChain := chains.Osmosis

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)
	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)
	osmosisToTXChannelID := osmosisChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)
	txToOsmosisChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, osmosisChain.ChainContext,
	)

	txToGaiaEscrowAddress := ibctransfertypes.GetEscrowAddress(ibctransfertypes.PortID, txToGaiaChannelID)
	txToOsmosisEscrowAddress := ibctransfertypes.GetEscrowAddress(ibctransfertypes.PortID, txToOsmosisChannelID)

	txSender := txChain.GenAccount()
	gaiaRecipient1 := gaiaChain.GenAccount()
	gaiaRecipient2 := gaiaChain.GenAccount()
	osmosisRecipient1 := osmosisChain.GenAccount()
	osmosisRecipient2 := osmosisChain.GenAccount()

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient1,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	}, integration.FundedAccount{
		Address: gaiaRecipient2,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	})

	osmosisChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: osmosisRecipient1,
		Amount:  osmosisChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	}, integration.FundedAccount{
		Address: osmosisRecipient2,
		Amount:  osmosisChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	})

	txIssuer := txChain.GenAccount()
	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount

	txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: txIssuer,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&banktypes.MsgSend{},
					&assetfttypes.MsgIssue{},
					&ibctransfertypes.MsgTransfer{},
					&ibctransfertypes.MsgTransfer{},
				},
				Amount: issueFee,
			},
		}, {
			Acc: txSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&ibctransfertypes.MsgTransfer{},
					&ibctransfertypes.MsgTransfer{},
				},
			},
		},
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:             txIssuer.String(),
		Symbol:             "mysymbol",
		Subunit:            "mysubunit",
		Precision:          8,
		InitialAmount:      sdkmath.NewInt(1_000_000),
		BurnRate:           sdkmath.LegacyMustNewDecFromStr("0.1"),
		SendCommissionRate: sdkmath.LegacyMustNewDecFromStr("0.2"),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)

	sendCoin := sdk.NewCoin(denom, sdkmath.NewInt(1000))
	burntAmount := issueMsg.BurnRate.Mul(sdkmath.LegacyNewDecFromInt(sendCoin.Amount)).TruncateInt()
	sendCommissionAmount := issueMsg.SendCommissionRate.Mul(sdkmath.LegacyNewDecFromInt(sendCoin.Amount)).TruncateInt()
	extraAmount := sdkmath.NewInt(77) // some amount to be left at the end
	msgSend := &banktypes.MsgSend{
		FromAddress: txIssuer.String(),
		ToAddress:   txSender.String(),
		// amount to send + burn rate + send commission rate + some amount to test with none-zero reminder
		Amount: sdk.NewCoins(sdk.NewCoin(denom,
			sendCoin.Amount.MulRaw(2).
				Add(burntAmount.MulRaw(2)).
				Add(sendCommissionAmount.MulRaw(2)).
				Add(extraAmount)),
		),
	}

	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(msgSend)),
		msgSend,
	)
	requireT.NoError(err)

	receiveCoinGaia := sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, sendCoin.Denom), sendCoin.Amount)
	receiveCoinOsmosis := sdk.NewCoin(ConvertToIBCDenom(osmosisToTXChannelID, sendCoin.Denom), sendCoin.Amount)

	// ********** TX-Chain to Gaia **********
	// IBC transfer from FT issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		txChain.ChainContext,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txIssuer,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient1,
		receiveCoinGaia,
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txIssuer):              sendCoin.Amount.Neg(),
			txChain.MustConvertToBech32Address(txToGaiaEscrowAddress): sendCoin.Amount,
		},
		map[string]sdkmath.Int{
			gaiaChain.MustConvertToBech32Address(gaiaRecipient1): sendCoin.Amount,
		},
	)

	// IBC transfer from non-FT issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		txChain.ChainContext,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient2,
		receiveCoinGaia,
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txSender): sendCoin.Amount.
				Add(sendCommissionAmount).Add(burntAmount).Neg(),
			txChain.MustConvertToBech32Address(txIssuer):              sendCommissionAmount,
			txChain.MustConvertToBech32Address(txToGaiaEscrowAddress): sendCoin.Amount,
		},
		map[string]sdkmath.Int{
			gaiaChain.MustConvertToBech32Address(gaiaRecipient2): sendCoin.Amount,
		},
	)

	// ********** TX-Chain to Osmosis **********
	// IBC transfer from FT issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		txChain.ChainContext,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txIssuer,
		sendCoin,
		osmosisChain.ChainContext,
		osmosisRecipient1,
		receiveCoinOsmosis,
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txIssuer):                 sendCoin.Amount.Neg(),
			txChain.MustConvertToBech32Address(txToOsmosisEscrowAddress): sendCoin.Amount,
		},
		map[string]sdkmath.Int{
			osmosisChain.MustConvertToBech32Address(osmosisRecipient1): sendCoin.Amount,
		},
	)

	// IBC transfer from non-FT issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		txChain.ChainContext,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendCoin,
		osmosisChain.ChainContext,
		osmosisRecipient2,
		receiveCoinOsmosis,
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txSender): sendCoin.Amount.
				Add(sendCommissionAmount).Add(burntAmount).Neg(),
			txChain.MustConvertToBech32Address(txIssuer):                 sendCommissionAmount,
			txChain.MustConvertToBech32Address(txToOsmosisEscrowAddress): sendCoin.Amount,
		},
		map[string]sdkmath.Int{
			osmosisChain.MustConvertToBech32Address(osmosisRecipient2): sendCoin.Amount,
		},
	)

	// ********** Gaia to TX-Chain (send back) **********
	// IBC transfer back to issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		gaiaChain.ChainContext,
		gaiaChain.TxFactoryAuto(),
		gaiaRecipient1,
		receiveCoinGaia,
		txChain.ChainContext,
		txIssuer,
		sendCoin,
		map[string]sdkmath.Int{
			gaiaChain.MustConvertToBech32Address(gaiaRecipient1): sendCoin.Amount.Neg(),
		},
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txToGaiaEscrowAddress): sendCoin.Amount.Neg(),
			txChain.MustConvertToBech32Address(txIssuer):              sendCoin.Amount,
		},
	)

	// IBC transfer back to non-issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		gaiaChain.ChainContext,
		gaiaChain.TxFactoryAuto(),
		gaiaRecipient2,
		receiveCoinGaia,
		txChain.ChainContext,
		txSender,
		sendCoin,
		map[string]sdkmath.Int{
			gaiaChain.MustConvertToBech32Address(gaiaRecipient2): sendCoin.Amount.Neg(),
		},
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txToGaiaEscrowAddress): sendCoin.Amount.Neg(),
			txChain.MustConvertToBech32Address(txSender):              sendCoin.Amount,
			txChain.MustConvertToBech32Address(txIssuer):              sdkmath.ZeroInt(),
		},
	)

	// ********** Osmosis to TX-Chain (send back) **********
	// IBC transfer back to issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		osmosisChain.ChainContext,
		osmosisChain.TxFactoryAuto(),
		osmosisRecipient1,
		receiveCoinOsmosis,
		txChain.ChainContext,
		txIssuer,
		sendCoin,
		map[string]sdkmath.Int{
			osmosisChain.MustConvertToBech32Address(osmosisRecipient1): sendCoin.Amount.Neg(),
		},
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txToOsmosisEscrowAddress): sendCoin.Amount.Neg(),
			txChain.MustConvertToBech32Address(txIssuer):                 sendCoin.Amount,
		},
	)

	// IBC transfer back to non-issuer address.
	ibcTransferAndAssertBalanceChanges(
		ctx,
		t,
		osmosisChain.ChainContext,
		osmosisChain.TxFactoryAuto(),
		osmosisRecipient2,
		receiveCoinOsmosis,
		txChain.ChainContext,
		txSender,
		sendCoin,
		map[string]sdkmath.Int{
			osmosisChain.MustConvertToBech32Address(osmosisRecipient2): sendCoin.Amount.Neg(),
		},
		map[string]sdkmath.Int{
			txChain.MustConvertToBech32Address(txToOsmosisEscrowAddress): sendCoin.Amount.Neg(),
			txChain.MustConvertToBech32Address(txSender):                 sendCoin.Amount,
			txChain.MustConvertToBech32Address(txIssuer):                 sdkmath.ZeroInt(),
		},
	)
}

func TestIBCAssetFTWhitelisting(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia
	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	txIssuer := txChain.GenAccount()
	txRecipientBlocked := txChain.GenAccount()
	txRecipientWhitelisted := txChain.GenAccount()
	gaiaRecipient := gaiaChain.GenAccount()

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	})

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount
	txChain.FundAccountWithOptions(ctx, t, txIssuer, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&assetfttypes.MsgIssue{},
			&assetfttypes.MsgSetWhitelistedLimit{},
			&ibctransfertypes.MsgTransfer{},
			&ibctransfertypes.MsgTransfer{},
		},
		Amount: issueFee,
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
			assetfttypes.Feature_whitelisting,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)
	sendBackCoin := sdk.NewCoin(denom, sdkmath.NewInt(1000))
	sendCoin := sdk.NewCoin(denom, sendBackCoin.Amount.MulRaw(2))

	whitelistMsg := &assetfttypes.MsgSetWhitelistedLimit{
		Sender:  txIssuer.String(),
		Account: txRecipientWhitelisted.String(),
		Coin:    sendBackCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(whitelistMsg)),
		whitelistMsg,
	)
	require.NoError(t, err)

	// send minted coins to gaia
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txIssuer,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.NoError(err)

	ibcDenom := ConvertToIBCDenom(gaiaToTXChannelID, denom)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, sdk.NewCoin(ibcDenom, sendCoin.Amount)))

	// send coins back to two accounts, one blocked, one whitelisted
	ibcSendCoin := sdk.NewCoin(ibcDenom, sendBackCoin.Amount)
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx, t, gaiaChain.TxFactoryAuto(), gaiaRecipient, ibcSendCoin, txChain.ChainContext, txRecipientBlocked,
	)
	requireT.NoError(err)
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx, t, gaiaChain.TxFactoryAuto(), gaiaRecipient, ibcSendCoin, txChain.ChainContext, txRecipientWhitelisted,
	)
	requireT.NoError(err)

	// transfer to whitelisted account is expected to succeed
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txRecipientWhitelisted, sendBackCoin))

	// transfer to blocked account is expected to fail and funds should be returned back
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, sdk.NewCoin(ibcDenom, sendBackCoin.Amount)))

	bankClient := banktypes.NewQueryClient(txChain.ClientContext)
	balanceRes, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: txRecipientBlocked.String(),
		Denom:   denom,
	})
	requireT.NoError(err)
	requireT.Equal(sdk.NewCoin(denom, sdkmath.ZeroInt()).String(), balanceRes.Balance.String())
}

func TestIBCAssetFTFreezing(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	assertT := assert.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	txIssuer := txChain.GenAccount()
	txSender := txChain.GenAccount()
	gaiaRecipient := gaiaChain.GenAccount()

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	})

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount

	txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: txIssuer,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&assetfttypes.MsgIssue{},
					&banktypes.MsgSend{},
					&assetfttypes.MsgFreeze{},
				},
				Amount: issueFee,
			},
		}, {
			Acc: txSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&ibctransfertypes.MsgTransfer{},
					&ibctransfertypes.MsgTransfer{},
				},
			},
		},
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
			assetfttypes.Feature_freezing,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)

	sendCoin := sdk.NewCoin(denom, sdkmath.NewInt(1000))
	halfCoin := sdk.NewCoin(denom, sdkmath.NewInt(500))
	msgSend := &banktypes.MsgSend{
		FromAddress: txIssuer.String(),
		ToAddress:   txSender.String(),
		Amount:      sdk.NewCoins(sendCoin),
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(msgSend)),
		msgSend,
	)
	requireT.NoError(err)

	freezeMsg := &assetfttypes.MsgFreeze{
		Sender:  txIssuer.String(),
		Account: txSender.String(),
		Coin:    halfCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(freezeMsg)),
		freezeMsg,
	)
	require.NoError(t, err)

	// send more than allowed, should fail
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.Error(err)
	assertT.Contains(err.Error(), cosmoserrors.ErrInsufficientFunds.Error())

	// send up to the limit, should succeed
	ibcCoin := sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, denom), halfCoin.Amount)
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		halfCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.NoError(err)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, ibcCoin))

	// send it back, frozen limit should not affect it
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx,
		t,
		gaiaChain.TxFactoryAuto(),
		gaiaRecipient,
		ibcCoin,
		txChain.ChainContext,
		txSender,
	)
	requireT.NoError(err)
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txSender, sendCoin))
}

func TestEscrowAddressIsResistantToFreezingAndWhitelisting(t *testing.T) {
	t.Parallel()

	requireT := require.New(t)

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	txIssuer := txChain.GenAccount()
	gaiaRecipient := gaiaChain.GenAccount()

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	})

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount
	txChain.FundAccountWithOptions(ctx, t, txIssuer, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&assetfttypes.MsgIssue{},
			&assetfttypes.MsgFreeze{},
			&assetfttypes.MsgSetWhitelistedLimit{},
			&ibctransfertypes.MsgTransfer{},
			&ibctransfertypes.MsgTransfer{},
		},
		Amount: issueFee,
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
			assetfttypes.Feature_freezing,
			assetfttypes.Feature_whitelisting,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)
	sendCoin := sdk.NewCoin(denom, issueMsg.InitialAmount)

	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)

	// send minted coins to gaia
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txIssuer,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.NoError(err)

	ibcDenom := ConvertToIBCDenom(gaiaToTXChannelID, denom)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, sdk.NewCoin(ibcDenom, sendCoin.Amount)))

	// freeze escrow account
	txToGaiaEscrowAddress := ibctransfertypes.GetEscrowAddress(ibctransfertypes.PortID, txToGaiaChannelID)
	freezeMsg := &assetfttypes.MsgFreeze{
		Sender:  txIssuer.String(),
		Account: txToGaiaEscrowAddress.String(),
		Coin:    sendCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(freezeMsg)),
		freezeMsg,
	)
	require.NoError(t, err)

	// send coins back to tx, it should succeed despite frozen escrow address
	txRecipient := chains.TXChain.GenAccount()
	whitelistMsg := &assetfttypes.MsgSetWhitelistedLimit{
		Sender:  txIssuer.String(),
		Account: txRecipient.String(),
		Coin:    sendCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(whitelistMsg)),
		whitelistMsg,
	)
	require.NoError(t, err)
	ibcSendCoin := sdk.NewCoin(ibcDenom, sendCoin.Amount)
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx,
		t,
		gaiaChain.TxFactoryAuto(),
		gaiaRecipient,
		ibcSendCoin,
		txChain.ChainContext,
		txRecipient,
	)
	requireT.NoError(err)
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txRecipient, sendCoin))
}

func TestIBCGlobalFreeze(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	txIssuer := txChain.GenAccount()
	txSender := txChain.GenAccount()
	txRecipient := txChain.GenAccount()
	gaiaRecipient := gaiaChain.GenAccount()

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(2000000)), // coin for the fees
	})

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount

	txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: txIssuer,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&assetfttypes.MsgIssue{},
					&assetfttypes.MsgGloballyFreeze{},
					&banktypes.MsgSend{},
					&ibctransfertypes.MsgTransfer{},
					&assetfttypes.MsgGloballyUnfreeze{},
				},
				Amount: issueFee,
			},
		}, {
			Acc: txSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&ibctransfertypes.MsgTransfer{},
				},
			},
		},
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
			assetfttypes.Feature_freezing,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)
	sendCoin := sdk.NewCoin(denom, issueMsg.InitialAmount.QuoRaw(2))
	ibcSendCoin := sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, denom), sendCoin.Amount)
	sendCoinBack := sdk.NewCoin(denom, issueMsg.InitialAmount.QuoRaw(10))
	ibcSendCoinBack := sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, denom), sendCoinBack.Amount)

	// set global freeze
	freezeMsg := &assetfttypes.MsgGloballyFreeze{
		Sender: txIssuer.String(),
		Denom:  denom,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(freezeMsg)),
		freezeMsg,
	)
	require.NoError(t, err)

	// send some coins to the other account, should work despite global freeze
	sendMsg := &banktypes.MsgSend{
		FromAddress: txIssuer.String(),
		ToAddress:   txSender.String(),
		Amount:      sdk.NewCoins(sendCoin),
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(sendMsg)),
		sendMsg,
	)
	require.NoError(t, err)

	// send minted coins to gaia from the other account, should fail due to global freeze
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.ErrorContains(err, assetfttypes.ErrGloballyFrozen.Error())

	// send minted coins to gaia from issuer, should succeed despite global freeze
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txIssuer,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.NoError(err)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, ibcSendCoin))

	// send coins back to issuer on tx, it should fail
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx,
		t,
		gaiaChain.TxFactoryAuto(),
		gaiaRecipient,
		ibcSendCoinBack,
		txChain.ChainContext,
		txIssuer,
	)
	requireT.NoError(err)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, ibcSendCoin))

	bankClient := banktypes.NewQueryClient(txChain.ClientContext)
	balanceRes, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: txIssuer.String(),
		Denom:   denom,
	})
	requireT.NoError(err)
	requireT.Equal(sdk.NewCoin(denom, sdkmath.ZeroInt()).String(), balanceRes.Balance.String())

	// send coins back to recipient on tx, it should fail
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx, t, gaiaChain.TxFactoryAuto(), gaiaRecipient, ibcSendCoinBack, txChain.ChainContext, txRecipient,
	)
	requireT.NoError(err)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, ibcSendCoin))

	balanceRes, err = bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: txRecipient.String(),
		Denom:   denom,
	})
	requireT.NoError(err)
	requireT.Equal(sdk.NewCoin(denom, sdkmath.ZeroInt()).String(), balanceRes.Balance.String())

	// remove global freeze
	unfreezeMsg := &assetfttypes.MsgGloballyUnfreeze{
		Sender: txIssuer.String(),
		Denom:  denom,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(unfreezeMsg)),
		unfreezeMsg,
	)
	require.NoError(t, err)

	// send coins back to issuer on tx again, it should succeed
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx, t, gaiaChain.TxFactoryAuto(), gaiaRecipient, ibcSendCoinBack, txChain.ChainContext, txIssuer,
	)
	requireT.NoError(err)
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txIssuer, sendCoinBack))

	// send coins back to recipient on tx again, it should succeed
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx, t, gaiaChain.TxFactoryAuto(), gaiaRecipient, ibcSendCoinBack, txChain.ChainContext, txRecipient,
	)
	requireT.NoError(err)
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txRecipient, sendCoinBack))
}

func TestIBCAssetFTClawback(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	assertT := assert.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	txIssuer := txChain.GenAccount()
	txSender := txChain.GenAccount()
	gaiaRecipient := gaiaChain.GenAccount()

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	})

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount

	txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: txIssuer,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&assetfttypes.MsgIssue{},
					&banktypes.MsgSend{},
					&assetfttypes.MsgClawback{},
				},
				Amount: issueFee,
			},
		}, {
			Acc: txSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&ibctransfertypes.MsgTransfer{},
					&ibctransfertypes.MsgTransfer{},
				},
			},
		},
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
			assetfttypes.Feature_clawback,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)

	sendCoin := sdk.NewCoin(denom, sdkmath.NewInt(1000))
	halfCoin := sdk.NewCoin(denom, sdkmath.NewInt(500))
	msgSend := &banktypes.MsgSend{
		FromAddress: txIssuer.String(),
		ToAddress:   txSender.String(),
		Amount:      sdk.NewCoins(sendCoin),
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(msgSend)),
		msgSend,
	)
	requireT.NoError(err)

	clawbackMsg := &assetfttypes.MsgClawback{
		Sender:  txIssuer.String(),
		Account: txSender.String(),
		Coin:    halfCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(clawbackMsg)),
		clawbackMsg,
	)
	require.NoError(t, err)

	// send more than allowed, should fail
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.Error(err)
	assertT.Contains(err.Error(), cosmoserrors.ErrInsufficientFunds.Error())

	// send up to the limit, should succeed
	ibcCoin := sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, denom), halfCoin.Amount)
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		halfCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.NoError(err)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, ibcCoin))
}

func TestIBCAssetFTTimedOutTransfer(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Osmosis

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	retryCtx, retryCancel := context.WithTimeout(ctx, 5*integration.AwaitStateTimeout)
	defer retryCancel()

	// This is the retry loop where we try to trigger a timeout condition for IBC transfer.
	// We can't reproduce it with 100% probability, so we may need to try it many times.
	// On every trial we send funds from one chain to the other. Then we observe accounts on both chains
	// to find if IBC transfer completed successfully or timed out. If tokens were delivered to the recipient
	// we must retry. Otherwise, if tokens were returned back to the sender, we might continue the test.
	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount
	err := retry.Do(retryCtx, time.Millisecond, func() error {
		txSender := txChain.GenAccount()
		gaiaRecipient := gaiaChain.GenAccount()

		txChain.FundAccountWithOptions(ctx, t, txSender, integration.BalancesOptions{
			Messages: []sdk.Msg{
				&assetfttypes.MsgIssue{},
				&ibctransfertypes.MsgTransfer{},
			},
			Amount: issueFee,
		})

		issueMsg := &assetfttypes.MsgIssue{
			Issuer:        txSender.String(),
			Symbol:        "mysymbol",
			Subunit:       "mysubunit",
			Precision:     8,
			InitialAmount: sdkmath.NewInt(1_000_000),
			Features: []assetfttypes.Feature{
				assetfttypes.Feature_block_smart_contracts,
				assetfttypes.Feature_ibc,
			},
		}
		_, err := client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txSender),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
			issueMsg,
		)
		require.NoError(t, err)
		denom := assetfttypes.BuildDenom(issueMsg.Subunit, txSender)
		sendToGaiaCoin := sdk.NewCoin(denom, issueMsg.InitialAmount)

		_, err = txChain.ExecuteTimingOutIBCTransfer(
			ctx,
			t,
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
			txSender,
			sendToGaiaCoin,
			gaiaChain.ChainContext,
			gaiaRecipient,
		)
		switch {
		case err == nil:
		case strings.Contains(err.Error(), ibcchanneltypes.ErrTimeoutElapsed.Error()):
			return retry.Retryable(err)
		default:
			requireT.NoError(err)
		}

		parallelCtx, parallelCancel := context.WithCancel(ctx)
		defer parallelCancel()
		errCh := make(chan error, 1)
		go func() {
			// In this goroutine we check if funds were returned back to the sender.
			// If this happens it means timeout occurred.

			defer parallelCancel()
			if err := txChain.AwaitForBalance(parallelCtx, t, txSender, sendToGaiaCoin); err != nil {
				select {
				case errCh <- retry.Retryable(err):
				default:
				}
			} else {
				errCh <- nil
			}
		}()
		go func() {
			// In this goroutine we check if funds were delivered to the other chain.
			// If this happens it means timeout didn't occur and we must try again.

			if err := gaiaChain.AwaitForBalance(
				parallelCtx,
				t,
				gaiaRecipient,
				sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, sendToGaiaCoin.Denom), sendToGaiaCoin.Amount),
			); err == nil {
				select {
				case errCh <- retry.Retryable(errors.New("timeout didn't happen")):
					parallelCancel()
				default:
				}
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}

		// At this point we are sure that timeout happened.

		// funds should not be received on gaia
		bankClient := banktypes.NewQueryClient(gaiaChain.ClientContext)
		resp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
			Address: gaiaChain.MustConvertToBech32Address(gaiaRecipient),
			Denom:   ConvertToIBCDenom(gaiaToTXChannelID, sendToGaiaCoin.Denom),
		})
		requireT.NoError(err)
		requireT.Equal("0", resp.Balance.Amount.String())

		return nil
	})
	requireT.NoError(err)
}

func TestIBCAssetFTRejectedTransfer(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	// Bank module rejects transfers targeting some module accounts. We use this feature to test that
	// this type of IBC transfer is rejected by the receiving chain.
	moduleAddress := authtypes.NewModuleAddress(icatypes.ModuleName)
	txSender := txChain.GenAccount()
	gaiaRecipient := gaiaChain.GenAccount()

	txChain.FundAccountWithOptions(ctx, t, txSender, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&assetfttypes.MsgIssue{},
			&ibctransfertypes.MsgTransfer{},
			&ibctransfertypes.MsgTransfer{},
			&ibctransfertypes.MsgTransfer{},
		},
		Amount: txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount,
	})
	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewIntFromUint64(1000000)),
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txSender.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
			assetfttypes.Feature_freezing,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txSender),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txSender)
	sendToGaiaCoin := sdk.NewCoin(denom, issueMsg.InitialAmount)

	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendToGaiaCoin,
		gaiaChain.ChainContext,
		moduleAddress,
	)
	requireT.NoError(err)

	// funds should be returned to tx
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txSender, sendToGaiaCoin))

	// funds should not be received on gaia
	ibcGaiaDenom := ConvertToIBCDenom(gaiaToTXChannelID, sendToGaiaCoin.Denom)
	bankClient := banktypes.NewQueryClient(gaiaChain.ClientContext)
	resp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: gaiaChain.MustConvertToBech32Address(moduleAddress),
		Denom:   ibcGaiaDenom,
	})
	requireT.NoError(err)
	requireT.Equal("0", resp.Balance.Amount.String())

	// test that the reverse transfer from gaia to tx is blocked too

	sendToTXCoin := sdk.NewCoin(ibcGaiaDenom, sendToGaiaCoin.Amount)
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendToGaiaCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.NoError(err)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, sendToTXCoin))

	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx,
		t,
		gaiaChain.TxFactoryAuto(),
		gaiaRecipient,
		sendToTXCoin,
		txChain.ChainContext,
		moduleAddress,
	)
	requireT.NoError(err)

	// funds should be returned to gaia
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, sendToTXCoin))

	// funds should not be received on tx
	bankClient = banktypes.NewQueryClient(txChain.ClientContext)
	resp, err = bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: txChain.MustConvertToBech32Address(moduleAddress),
		Denom:   sendToGaiaCoin.Denom,
	})
	requireT.NoError(err)
	requireT.Equal("0", resp.Balance.Amount.String())
}

func TestIBCRejectedTransferWithWhitelistingAndFreezing(t *testing.T) {
	t.Parallel()

	requireT := require.New(t)

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	txIssuer := txChain.GenAccount()
	txSender := txChain.GenAccount()
	// Bank module rejects transfers targeting some module accounts. We use this feature to test that
	// this type of IBC transfer is rejected by the receiving chain.
	moduleAddress := authtypes.NewModuleAddress(icatypes.ModuleName)

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount

	txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: txIssuer,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&assetfttypes.MsgIssue{},
					&assetfttypes.MsgFreeze{},
					&assetfttypes.MsgSetWhitelistedLimit{},
					&banktypes.MsgSend{},
					&assetfttypes.MsgSetWhitelistedLimit{},
				},
				Amount: issueFee,
			},
		}, {
			Acc: txSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&ibctransfertypes.MsgTransfer{},
				},
			},
		},
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
			assetfttypes.Feature_freezing,
			assetfttypes.Feature_whitelisting,
		},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)
	sendCoin := sdk.NewCoin(denom, issueMsg.InitialAmount)

	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)

	// freeze escrow account
	txToGaiaEscrowAddress := ibctransfertypes.GetEscrowAddress(ibctransfertypes.PortID, txToGaiaChannelID)
	freezeMsg := &assetfttypes.MsgFreeze{
		Sender:  txIssuer.String(),
		Account: txToGaiaEscrowAddress.String(),
		Coin:    sendCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(freezeMsg)),
		freezeMsg,
	)
	require.NoError(t, err)

	// whitelist sender
	whitelistMsg := &assetfttypes.MsgSetWhitelistedLimit{
		Sender:  txIssuer.String(),
		Account: txSender.String(),
		Coin:    sendCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(whitelistMsg)),
		whitelistMsg,
	)
	require.NoError(t, err)

	// send coins from issuer to sender
	sendMsg := &banktypes.MsgSend{
		FromAddress: txIssuer.String(),
		ToAddress:   txSender.String(),
		Amount:      sdk.NewCoins(sendCoin),
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(sendMsg)),
		sendMsg,
	)
	require.NoError(t, err)

	// blacklist sender
	blacklistMsg := &assetfttypes.MsgSetWhitelistedLimit{
		Sender:  txIssuer.String(),
		Account: txSender.String(),
		Coin:    sdk.NewInt64Coin(sendCoin.Denom, 0),
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(blacklistMsg)),
		blacklistMsg,
	)
	require.NoError(t, err)

	// send coins from sender to blocked address on gaia
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendCoin,
		gaiaChain.ChainContext,
		moduleAddress,
	)
	requireT.NoError(err)

	// gaia should reject the IBC transfers and funds should be returned back to tx, despite:
	// - escrow address being frozen
	// - sender account not being whitelisted
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txSender, sendCoin))
}

func TestIBCTimedOutTransferWithWhitelistingAndFreezing(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	retryCtx, retryCancel := context.WithTimeout(ctx, 5*integration.AwaitStateTimeout)
	defer retryCancel()

	// This is the retry loop where we try to trigger a timeout condition for IBC transfer.
	// We can't reproduce it with 100% probability, so we may need to try it many times.
	// On every trial we send funds from one chain to the other. Then we observe accounts on both chains
	// to find if IBC transfer completed successfully or timed out. If tokens were delivered to the recipient
	// we must retry. Otherwise, if tokens were returned back to the sender, we might continue the test.
	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount
	err := retry.Do(retryCtx, time.Millisecond, func() error {
		txIssuer := txChain.GenAccount()
		txSender := txChain.GenAccount()
		gaiaRecipient := gaiaChain.GenAccount()

		txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
			{
				Acc: txIssuer,
				Options: integration.BalancesOptions{
					Messages: []sdk.Msg{
						&assetfttypes.MsgIssue{},
						&assetfttypes.MsgFreeze{},
						&assetfttypes.MsgSetWhitelistedLimit{},
						&banktypes.MsgSend{},
						&assetfttypes.MsgSetWhitelistedLimit{},
					},
					Amount: issueFee,
				},
			}, {
				Acc: txSender,
				Options: integration.BalancesOptions{
					Messages: []sdk.Msg{
						&ibctransfertypes.MsgTransfer{},
					},
				},
			},
		})

		issueMsg := &assetfttypes.MsgIssue{
			Issuer:        txIssuer.String(),
			Symbol:        "mysymbol",
			Subunit:       "mysubunit",
			Precision:     8,
			InitialAmount: sdkmath.NewInt(1_000_000),
			Features: []assetfttypes.Feature{
				assetfttypes.Feature_block_smart_contracts,
				assetfttypes.Feature_ibc,
				assetfttypes.Feature_whitelisting,
				assetfttypes.Feature_freezing,
			},
		}
		_, err := client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txIssuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
			issueMsg,
		)
		require.NoError(t, err)
		denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)
		sendToGaiaCoin := sdk.NewCoin(denom, issueMsg.InitialAmount)

		txToGaiaChannelID := txChain.AwaitForIBCChannelID(
			ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
		)

		// freeze escrow account
		txToGaiaEscrowAddress := ibctransfertypes.GetEscrowAddress(ibctransfertypes.PortID, txToGaiaChannelID)
		freezeMsg := &assetfttypes.MsgFreeze{
			Sender:  txIssuer.String(),
			Account: txToGaiaEscrowAddress.String(),
			Coin:    sendToGaiaCoin,
		}
		_, err = client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txIssuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(freezeMsg)),
			freezeMsg,
		)
		require.NoError(t, err)

		// whitelist sender
		whitelistMsg := &assetfttypes.MsgSetWhitelistedLimit{
			Sender:  txIssuer.String(),
			Account: txSender.String(),
			Coin:    sendToGaiaCoin,
		}
		_, err = client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txIssuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(whitelistMsg)),
			whitelistMsg,
		)
		require.NoError(t, err)

		// send coins from issuer to sender
		sendMsg := &banktypes.MsgSend{
			FromAddress: txIssuer.String(),
			ToAddress:   txSender.String(),
			Amount:      sdk.NewCoins(sendToGaiaCoin),
		}
		_, err = client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txIssuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(sendMsg)),
			sendMsg,
		)
		require.NoError(t, err)

		// blacklist sender
		blacklistMsg := &assetfttypes.MsgSetWhitelistedLimit{
			Sender:  txIssuer.String(),
			Account: txSender.String(),
			Coin:    sdk.NewInt64Coin(sendToGaiaCoin.Denom, 0),
		}
		_, err = client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txIssuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(blacklistMsg)),
			blacklistMsg,
		)
		require.NoError(t, err)

		_, err = txChain.ExecuteTimingOutIBCTransfer(
			ctx,
			t,
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
			txSender,
			sendToGaiaCoin,
			gaiaChain.ChainContext,
			gaiaRecipient,
		)
		switch {
		case err == nil:
		case strings.Contains(err.Error(), ibcchanneltypes.ErrTimeoutElapsed.Error()):
			return retry.Retryable(err)
		default:
			requireT.NoError(err)
		}

		parallelCtx, parallelCancel := context.WithCancel(ctx)
		defer parallelCancel()
		errCh := make(chan error, 1)
		go func() {
			// In this goroutine we check if funds were returned back to the sender.
			// If this happens it means timeout occurred.

			defer parallelCancel()
			if err := txChain.AwaitForBalance(parallelCtx, t, txSender, sendToGaiaCoin); err != nil {
				select {
				case errCh <- retry.Retryable(err):
				default:
				}
			} else {
				errCh <- nil
			}
		}()
		go func() {
			// In this goroutine we check if funds were delivered to the other chain.
			// If this happens it means timeout didn't occur and we must try again.

			if err := gaiaChain.AwaitForBalance(
				parallelCtx,
				t,
				gaiaRecipient,
				sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, sendToGaiaCoin.Denom), sendToGaiaCoin.Amount),
			); err == nil {
				select {
				case errCh <- retry.Retryable(errors.New("timeout didn't happen")):
					parallelCancel()
				default:
				}
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}

		// At this point we are sure that timeout happened and coins has been sent back to the sender.
		return nil
	})
	requireT.NoError(err)
}

func TestIBCRejectedTransferWithBurnRateAndSendCommission(t *testing.T) {
	t.Parallel()

	requireT := require.New(t)

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	txIssuer := txChain.GenAccount()
	txSender := txChain.GenAccount()
	// Bank module rejects transfers targeting some module accounts. We use this feature to test that
	// this type of IBC transfer is rejected by the receiving chain.
	moduleAddress := authtypes.NewModuleAddress(icatypes.ModuleName)

	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount

	txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: txIssuer,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&assetfttypes.MsgIssue{},
					&banktypes.MsgSend{},
				},
				Amount: issueFee,
			},
		}, {
			Acc: txSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&ibctransfertypes.MsgTransfer{},
				},
			},
		},
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        txIssuer.String(),
		Symbol:        "mysymbol",
		Subunit:       "mysubunit",
		Precision:     8,
		InitialAmount: sdkmath.NewInt(910_000),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_block_smart_contracts,
			assetfttypes.Feature_ibc,
		},
		BurnRate:           sdkmath.LegacyMustNewDecFromStr("0.1"),
		SendCommissionRate: sdkmath.LegacyMustNewDecFromStr("0.2"),
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	require.NoError(t, err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)

	// send coins from issuer to sender
	sendMsg := &banktypes.MsgSend{
		FromAddress: txIssuer.String(),
		ToAddress:   txSender.String(),
		Amount:      sdk.NewCoins(sdk.NewCoin(denom, issueMsg.InitialAmount)),
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txIssuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(sendMsg)),
		sendMsg,
	)
	require.NoError(t, err)

	// Send coins from sender to blocked address on Gaia.
	// We send everything except amount required to cover burn rate and send commission.
	sendCoin := sdk.NewCoin(
		denom,
		sdkmath.LegacyNewDecFromInt(issueMsg.InitialAmount).
			Quo(sdkmath.LegacyOneDec().Add(issueMsg.BurnRate).Add(issueMsg.SendCommissionRate)).
			TruncateInt(),
	)
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendCoin,
		gaiaChain.ChainContext,
		moduleAddress,
	)
	requireT.NoError(err)

	// Gaia should reject the IBC transfers and funds should be returned back to TX.
	// Burn rate and send commission should be charged only once when IBC transfer is
	// requested (we will probably change this in the future),
	// but when IBC transfer is rolled back, rates should not be charged again.
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txSender, sendCoin))

	// Balance on escrow address should be 0.
	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)
	txToGaiaEscrowAddress := ibctransfertypes.GetEscrowAddress(ibctransfertypes.PortID, txToGaiaChannelID)
	bankClient := banktypes.NewQueryClient(txChain.ClientContext)
	balanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: txToGaiaEscrowAddress.String(),
		Denom:   denom,
	})
	requireT.NoError(err)
	requireT.Equal("0", balanceResp.Balance.Amount.String())
}

func TestIBCTimedOutTransferWithBurnRateAndSendCommission(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	retryCtx, retryCancel := context.WithTimeout(ctx, 5*integration.AwaitStateTimeout)
	defer retryCancel()

	// This is the retry loop where we try to trigger a timeout condition for IBC transfer.
	// We can't reproduce it with 100% probability, so we may need to try it many times.
	// On every trial we send funds from one chain to the other. Then we observe accounts on both chains
	// to find if IBC transfer completed successfully or timed out. If tokens were delivered to the recipient
	// we must retry. Otherwise, if tokens were returned back to the sender, we might continue the test.
	issueFee := txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount
	err := retry.Do(retryCtx, time.Millisecond, func() error {
		txIssuer := txChain.GenAccount()
		txSender := txChain.GenAccount()
		gaiaRecipient := gaiaChain.GenAccount()

		txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
			{
				Acc: txIssuer,
				Options: integration.BalancesOptions{
					Messages: []sdk.Msg{
						&assetfttypes.MsgIssue{},
						&banktypes.MsgSend{},
					},
					Amount: issueFee,
				},
			}, {
				Acc: txSender,
				Options: integration.BalancesOptions{
					Messages: []sdk.Msg{
						&ibctransfertypes.MsgTransfer{},
					},
				},
			},
		})

		issueMsg := &assetfttypes.MsgIssue{
			Issuer:        txIssuer.String(),
			Symbol:        "mysymbol",
			Subunit:       "mysubunit",
			Precision:     8,
			InitialAmount: sdkmath.NewInt(910_000),
			Features: []assetfttypes.Feature{
				assetfttypes.Feature_block_smart_contracts,
				assetfttypes.Feature_ibc,
			},
			BurnRate:           sdkmath.LegacyMustNewDecFromStr("0.1"),
			SendCommissionRate: sdkmath.LegacyMustNewDecFromStr("0.2"),
		}
		_, err := client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txIssuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
			issueMsg,
		)
		require.NoError(t, err)
		denom := assetfttypes.BuildDenom(issueMsg.Subunit, txIssuer)

		// send coins from issuer to sender
		sendMsg := &banktypes.MsgSend{
			FromAddress: txIssuer.String(),
			ToAddress:   txSender.String(),
			Amount:      sdk.NewCoins(sdk.NewCoin(denom, issueMsg.InitialAmount)),
		}
		_, err = client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(txIssuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(sendMsg)),
			sendMsg,
		)
		require.NoError(t, err)

		// Send coins from sender to Gaia.
		// We send everything except amount required to cover burn rate and send commission.
		sendCoin := sdk.NewCoin(denom,
			sdkmath.
				LegacyNewDecFromInt(issueMsg.InitialAmount).
				Quo(sdkmath.LegacyOneDec().Add(issueMsg.BurnRate).Add(issueMsg.SendCommissionRate)).
				TruncateInt(),
		)

		_, err = txChain.ExecuteTimingOutIBCTransfer(
			ctx,
			t,
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
			txSender,
			sendCoin,
			gaiaChain.ChainContext,
			gaiaRecipient,
		)
		switch {
		case err == nil:
		case strings.Contains(err.Error(), ibcchanneltypes.ErrTimeoutElapsed.Error()):
			return retry.Retryable(err)
		default:
			requireT.NoError(err)
		}

		parallelCtx, parallelCancel := context.WithCancel(ctx)
		defer parallelCancel()
		errCh := make(chan error, 1)
		go func() {
			// In this goroutine we check if funds were returned back to the sender.
			// If this happens it means timeout occurred.

			defer parallelCancel()
			if err := txChain.AwaitForBalance(parallelCtx, t, txSender, sendCoin); err != nil {
				select {
				case errCh <- retry.Retryable(err):
				default:
				}
			} else {
				errCh <- nil
			}
		}()
		go func() {
			// In this goroutine we check if funds were delivered to the other chain.
			// If this happens it means timeout didn't occur and we must try again.

			if err := gaiaChain.AwaitForBalance(
				parallelCtx,
				t,
				gaiaRecipient,
				sdk.NewCoin(ConvertToIBCDenom(gaiaToTXChannelID, sendCoin.Denom), sendCoin.Amount),
			); err == nil {
				select {
				case errCh <- retry.Retryable(errors.New("timeout didn't happen")):
					parallelCancel()
				default:
				}
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			if err != nil {
				return err
			}
		}

		// At this point we are sure that timeout happened and coins has been sent back to the sender.

		// Balance on escrow address should be 0.
		txToGaiaChannelID := txChain.AwaitForIBCChannelID(
			ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
		)
		txToGaiaEscrowAddress := ibctransfertypes.GetEscrowAddress(ibctransfertypes.PortID, txToGaiaChannelID)
		bankClient := banktypes.NewQueryClient(txChain.ClientContext)
		balanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
			Address: txToGaiaEscrowAddress.String(),
			Denom:   denom,
		})
		requireT.NoError(err)
		requireT.Equal("0", balanceResp.Balance.Amount.String())

		return nil
	})
	requireT.NoError(err)
}

func ibcTransferAndAssertBalanceChanges(
	ctx context.Context,
	t *testing.T,
	srcChainCtx integration.ChainContext,
	srcChainTxf client.Factory,
	srcSender sdk.AccAddress,
	sendCoin sdk.Coin,
	dstChainCtx integration.ChainContext,
	dstChainRecipient sdk.AccAddress,
	receiveCoin sdk.Coin,
	srcExpectedBalanceChanges map[string]sdkmath.Int,
	dstExpectedBalanceChanges map[string]sdkmath.Int,
) {
	t.Helper()

	requireT := require.New(t)

	srcBalancesBeforeOperation := fetchBalanceForMultipleAddresses(
		ctx, t, srcChainCtx, sendCoin.Denom, lo.Keys(srcExpectedBalanceChanges),
	)
	dstBalancesBeforeOperation := fetchBalanceForMultipleAddresses(
		ctx, t, dstChainCtx, receiveCoin.Denom, lo.Keys(dstExpectedBalanceChanges),
	)

	dstBankClient := banktypes.NewQueryClient(dstChainCtx.ClientContext)
	dstChainRecipientBalanceBefore, err := dstBankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: dstChainCtx.MustConvertToBech32Address(dstChainRecipient),
		Denom:   receiveCoin.Denom,
	})
	requireT.NoError(err)
	dstChainRecipientBalanceExpected := dstChainRecipientBalanceBefore.Balance.Add(receiveCoin)

	_, err = srcChainCtx.ExecuteIBCTransfer(ctx, t, srcChainTxf, srcSender, sendCoin, dstChainCtx, dstChainRecipient)
	requireT.NoError(err)
	requireT.NoError(dstChainCtx.AwaitForBalance(ctx, t, dstChainRecipient, dstChainRecipientBalanceExpected))

	srcBalancesAfterOperation := fetchBalanceForMultipleAddresses(
		ctx, t, srcChainCtx, sendCoin.Denom, lo.Keys(srcExpectedBalanceChanges),
	)
	dstBalancesAfterOperation := fetchBalanceForMultipleAddresses(
		ctx, t, dstChainCtx, receiveCoin.Denom, lo.Keys(dstExpectedBalanceChanges),
	)

	assertBalanceChanges(t, srcExpectedBalanceChanges, srcBalancesBeforeOperation, srcBalancesAfterOperation)
	assertBalanceChanges(t, dstExpectedBalanceChanges, dstBalancesBeforeOperation, dstBalancesAfterOperation)
}

func fetchBalanceForMultipleAddresses(
	ctx context.Context,
	t *testing.T,
	chainCtx integration.ChainContext,
	denom string,
	addresses []string,
) map[string]sdkmath.Int {
	requireT := require.New(t)
	bankClient := banktypes.NewQueryClient(chainCtx.ClientContext)
	balances := make(map[string]sdkmath.Int, len(addresses))

	for _, addr := range addresses {
		balance, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
			Address: addr,
			Denom:   denom,
		})
		requireT.NoError(err)
		requireT.NotNil(balance.Balance)
		balances[addr] = balance.Balance.Amount
	}

	return balances
}

func assertBalanceChanges(t *testing.T, expectedBalanceChanges, balancesBefore, balancesAfter map[string]sdkmath.Int) {
	requireT := require.New(t)

	for addr, expectedBalanceChange := range expectedBalanceChanges {
		actualBalanceChange := balancesAfter[addr].Sub(balancesBefore[addr])
		requireT.Equal(expectedBalanceChange.String(), actualBalanceChange.String())
	}
}
