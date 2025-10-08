//go:build integrationtests

package ibc

import (
	"context"
	"strings"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	icatypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcchanneltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tokenize-x/tx-tools/pkg/retry"

	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	"github.com/tokenize-x/tx-chain/v6/pkg/client"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
)

func TestIBCTransferFromTXToGaiaAndBack(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	txSender := txChain.GenAccount()
	gaiaRecipient := gaiaChain.GenAccount()

	sendToGaiaCoin := txChain.NewCoin(sdkmath.NewInt(1000))
	txChain.FundAccountWithOptions(ctx, t, txSender, integration.BalancesOptions{
		Messages: []sdk.Msg{&ibctransfertypes.MsgTransfer{}},
		Amount:   sendToGaiaCoin.Amount,
	})

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(1000000)), // coin for the fees
	})

	txRes, err := txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendToGaiaCoin,
		gaiaChain.ChainContext,
		gaiaRecipient,
	)
	requireT.NoError(err)
	requireT.EqualValues(txRes.GasUsed, txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{}))

	expectedGaiaRecipientBalance := sdk.NewCoin(
		ConvertToIBCDenom(gaiaToTXChannelID, sendToGaiaCoin.Denom),
		sendToGaiaCoin.Amount,
	)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaRecipient, expectedGaiaRecipientBalance))
	_, err = gaiaChain.ExecuteIBCTransfer(
		ctx,
		t,
		gaiaChain.TxFactoryAuto(),
		gaiaRecipient,
		expectedGaiaRecipientBalance,
		txChain.ChainContext,
		txSender,
	)
	requireT.NoError(err)

	expectedTxSenderBalance := sdk.NewCoin(sendToGaiaCoin.Denom, expectedGaiaRecipientBalance.Amount)
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txSender, expectedTxSenderBalance))
}

// TestIBCTransferFromGaiaToTxAndBack checks IBC transfer in the following order:
// gaiaAccount1 [IBC]-> txToTxSender [bank.Send]-> txToGaiaSender [IBC]-> gaiaAccount2.
func TestIBCTransferFromGaiaToTxAndBack(t *testing.T) {
	t.Parallel()
	requireT := require.New(t)

	ctx, chains := integrationtests.NewChainsTestingContext(t)

	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	txBankClient := banktypes.NewQueryClient(txChain.ClientContext)

	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)
	sendToTXCoin := gaiaChain.NewCoin(sdkmath.NewInt(1000))

	// Generate accounts
	gaiaAccount1 := gaiaChain.GenAccount()
	gaiaAccount2 := gaiaChain.GenAccount()
	txToTxSender := txChain.GenAccount()
	txToGaiaSender := txChain.GenAccount()

	// Fund accounts
	txChain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: txToTxSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{&banktypes.MsgSend{}},
			},
		}, {
			Acc: txToGaiaSender,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{&ibctransfertypes.MsgTransfer{}},
			},
		},
	})

	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaAccount1,
		Amount:  sendToTXCoin.Add(gaiaChain.NewCoin(sdkmath.NewInt(1000000))), // coin to send + coin for the fee
	})

	// Send from gaiaAccount to txToTxSender
	_, err := gaiaChain.ExecuteIBCTransfer(
		ctx,
		t,
		gaiaChain.TxFactoryAuto(),
		gaiaAccount1,
		sendToTXCoin,
		txChain.ChainContext,
		txToTxSender,
	)
	requireT.NoError(err)

	expectedBalanceAtTx := sdk.NewCoin(
		ConvertToIBCDenom(txToGaiaChannelID, sendToTXCoin.Denom),
		sendToTXCoin.Amount,
	)
	requireT.NoError(txChain.AwaitForBalance(ctx, t, txToTxSender, expectedBalanceAtTx))

	// check that denom metadata is registered
	denomMetadataRes, err := txBankClient.DenomMetadata(ctx, &banktypes.QueryDenomMetadataRequest{
		Denom: expectedBalanceAtTx.Denom,
	})
	requireT.NoError(err)
	assert.Equal(t, expectedBalanceAtTx.Denom, denomMetadataRes.Metadata.Base)

	// Send from txToTxSender to txToGaiaSender
	sendMsg := &banktypes.MsgSend{
		FromAddress: txToTxSender.String(),
		ToAddress:   txToGaiaSender.String(),
		Amount:      []sdk.Coin{expectedBalanceAtTx},
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(txToTxSender),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(sendMsg)),
		sendMsg,
	)
	requireT.NoError(err)

	queryBalanceResponse, err := txBankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: txToGaiaSender.String(),
		Denom:   expectedBalanceAtTx.Denom,
	})
	requireT.NoError(err)
	assert.Equal(t, expectedBalanceAtTx.Amount.String(), queryBalanceResponse.Balance.Amount.String())

	// Send from txToGaiaSender back to gaiaAccount
	_, err = txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txToGaiaSender,
		expectedBalanceAtTx,
		gaiaChain.ChainContext,
		gaiaAccount2,
	)
	requireT.NoError(err)
	expectedGaiaSenderBalance := sdk.NewCoin(sendToTXCoin.Denom, expectedBalanceAtTx.Amount)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaAccount2, expectedGaiaSenderBalance))
}

func TestTimedOutTransfer(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	osmosisChain := chains.Osmosis

	osmosisToTXChannelID := osmosisChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)

	retryCtx, retryCancel := context.WithTimeout(ctx, 5*integration.AwaitStateTimeout)
	defer retryCancel()

	// This is the retry loop where we try to trigger a timeout condition for IBC transfer.
	// We can't reproduce it with 100% probability, so we may need to try it many times.
	// On every trial we send funds from one chain to the other. Then we observe accounts on both chains
	// to find if IBC transfer completed successfully or timed out. If tokens were delivered to the recipient
	// we must retry. Otherwise, if tokens were returned back to the sender, we might continue the test.
	err := retry.Do(retryCtx, time.Millisecond, func() error {
		txSender := txChain.GenAccount()
		osmosisRecipient := osmosisChain.GenAccount()

		sendToOsmosisCoin := txChain.NewCoin(sdkmath.NewInt(1000))
		txChain.FundAccountWithOptions(ctx, t, txSender, integration.BalancesOptions{
			Messages: []sdk.Msg{&ibctransfertypes.MsgTransfer{}},
			Amount:   sendToOsmosisCoin.Amount,
		})

		_, err := txChain.ExecuteTimingOutIBCTransfer(
			ctx,
			t,
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
			txSender,
			sendToOsmosisCoin,
			osmosisChain.ChainContext,
			osmosisRecipient,
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
			if err := txChain.AwaitForBalance(parallelCtx, t, txSender, sendToOsmosisCoin); err != nil {
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

			if err := osmosisChain.AwaitForBalance(
				parallelCtx,
				t,
				osmosisRecipient,
				sdk.NewCoin(ConvertToIBCDenom(osmosisToTXChannelID, sendToOsmosisCoin.Denom), sendToOsmosisCoin.Amount),
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
		bankClient := banktypes.NewQueryClient(osmosisChain.ClientContext)
		resp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
			Address: osmosisChain.MustConvertToBech32Address(osmosisRecipient),
			Denom:   ConvertToIBCDenom(osmosisToTXChannelID, sendToOsmosisCoin.Denom),
		})
		requireT.NoError(err)
		requireT.Equal("0", resp.Balance.Amount.String())

		return nil
	})
	requireT.NoError(err)
}

func TestRejectedTransfer(t *testing.T) {
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

	sendToGaiaCoin := txChain.NewCoin(sdkmath.NewInt(1000))
	txChain.FundAccountWithOptions(ctx, t, txSender, integration.BalancesOptions{
		Messages: []sdk.Msg{&ibctransfertypes.MsgTransfer{}},
		Amount:   sendToGaiaCoin.Amount,
	})
	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRecipient,
		Amount:  gaiaChain.NewCoin(sdkmath.NewIntFromUint64(1000000)),
	})

	_, err := txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendToGaiaCoin,
		gaiaChain.ChainContext,
		moduleAddress,
	)
	requireT.NoError(err)

	// funds should be returned to tx-chain
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

	// test that the reverse transfer from gaia to tx-chain is blocked too

	txChain.FundAccountWithOptions(ctx, t, txSender, integration.BalancesOptions{
		Messages: []sdk.Msg{&ibctransfertypes.MsgTransfer{}},
	})

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

	// funds should not be received on tx-chain
	bankClient = banktypes.NewQueryClient(txChain.ClientContext)
	resp, err = bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: txChain.MustConvertToBech32Address(moduleAddress),
		Denom:   sendToGaiaCoin.Denom,
	})
	requireT.NoError(err)
	requireT.Equal("0", resp.Balance.Amount.String())
}
