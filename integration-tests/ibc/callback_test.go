//go:build integrationtests

package ibc

import (
	"encoding/json"
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v8/integration-tests"
	ibcwasm "github.com/tokenize-x/tx-chain/v8/integration-tests/contracts/ibc"
	"github.com/tokenize-x/tx-chain/v8/pkg/client"
	"github.com/tokenize-x/tx-chain/v8/testutil/integration"
	assetfttypes "github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
)

// TestIBCWASMCallback tests ibc-callback integration by deploying the ibc-callbacks-counter WASM contract
// on TX-Chain and using it as a callback for IBC transfer sent to Gaia.
func TestIBCWASMCallback(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)
	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)

	txContractAdmin := txChain.GenAccount()
	txSender := txChain.GenAccount()
	txReceiver := txChain.GenAccount()

	gaiaSender := gaiaChain.GenAccount()
	gaiaReceiver := gaiaChain.GenAccount()

	txChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: txContractAdmin,
			Amount:  txChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
		integration.FundedAccount{
			Address: txSender,
			Amount:  txChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)

	gaiaChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: gaiaSender,
			Amount:  gaiaChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)

	// ********** Deploy contract **********

	// instantiate the contract and set the initial adapter state.
	initialPayload, err := json.Marshal(ibcwasm.HooksCounterState{
		Count: 2024, // This is the initial counter value for contract instantiator. We don't use this value.
	})
	requireT.NoError(err)

	txContractAddr, _, err := txChain.Wasm.DeployAndInstantiateWASMContract(
		ctx,
		txChain.TxFactoryAuto(),
		txContractAdmin,
		ibcwasm.IBCCallbacksCounter,
		integration.InstantiateConfig{
			Admin:      txContractAdmin,
			AccessType: wasmtypes.AccessTypeUnspecified,
			Payload:    initialPayload,
			Label:      "ibc_callbacks_counter",
		},
	)
	requireT.NoError(err)

	_, txContract, err := bech32.DecodeAndConvert(txContractAddr)
	requireT.NoError(err)

	txChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: txContract,
			Amount:  txChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)

	sendToTXCoin := gaiaChain.NewCoin(sdkmath.NewInt(1))

	ibcCallbackMemo := fmt.Sprintf(`{"dest_callback": {
					"address": "%s",
					"gas_limit": "%d"
				  }}`, txContractAddr, 10_000_000)

	// We send a Gaia to TX transfer here to trigger the dest_callback of the smart contract deployed on TX
	_, err = gaiaChain.ExecuteIBCTransferWithMemo(
		ctx,
		t,
		gaiaChain.TxFactory().WithGas(2_000_000),
		gaiaSender,
		sendToTXCoin,
		txChain.ChainContext,
		txReceiver.String(),
		ibcCallbackMemo,
	)
	requireT.NoError(err)

	awaitCounterContractState(
		ctx,
		t,
		txChain,
		txContractAddr,
		txContractAddr,
		1,
		sdk.Coins{},
	)

	_, err = gaiaChain.ExecuteIBCTransferWithMemo(
		ctx,
		t,
		gaiaChain.TxFactory().WithGas(2_000_000),
		gaiaSender,
		sendToTXCoin,
		txChain.ChainContext,
		txReceiver.String(),
		ibcCallbackMemo,
	)
	requireT.NoError(err)

	awaitCounterContractState(
		ctx,
		t,
		txChain,
		txContractAddr,
		txContractAddr,
		2,
		sdk.Coins{},
	)

	// We send a TX to Gaia transfer here to trigger the src_callback of the smart contract deployed on TX
	// in the transfer_funds method, it sends an IBC transfer with src_callback filled in memo
	transferFundsPayload, err := json.Marshal(map[string]transferFunds{
		"transfer_funds": {
			Channel:   txToGaiaChannelID,
			Amount:    txChain.NewCoin(sdkmath.NewInt(1)),
			Recipient: gaiaChain.MustConvertToBech32Address(gaiaReceiver),
		},
	})
	requireT.NoError(err)

	_, err = txChain.Wasm.ExecuteWASMContract(
		ctx,
		txChain.TxFactory().WithGas(2_000_000),
		txSender,
		txContractAddr,
		transferFundsPayload,
		sdk.Coin{},
	)
	requireT.NoError(err)

	awaitCounterContractState(
		ctx,
		t,
		txChain,
		txContractAddr,
		txContractAddr,
		3,
		sdk.Coins{},
	)
}

type transferFunds struct {
	Channel   string   `json:"channel"`
	Amount    sdk.Coin `json:"amount"`
	Recipient string   `json:"recipient"`
}

// TestIBCWASMCallbackCannotBypassAssetFTFreeze verifies that a contract executed from an IBC source callback
// cannot move asset-ft tokens frozen by the issuer.
// The callback runs while the transfer's acknowledgement is processed, so it sees the acknowledgement purpose,
// which exempts the transfer module's own refunds from the asset-ft checks.
func TestIBCWASMCallbackCannotBypassAssetFTFreeze(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)
	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)

	issuer := txChain.GenAccount()
	txContractAdmin := txChain.GenAccount()
	frozenRecipient := txChain.GenAccount()
	controlRecipient := txChain.GenAccount()

	txChain.FundAccountWithOptions(ctx, t, issuer, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&assetfttypes.MsgIssue{},
			&banktypes.MsgSend{},
			&banktypes.MsgSend{},
			&assetfttypes.MsgFreeze{},
		},
		Amount: txChain.QueryAssetFTParams(ctx, t).IssueFee.Amount,
	})
	txChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: txContractAdmin,
		Amount:  txChain.NewCoin(sdkmath.NewInt(20_000_000)),
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        issuer.String(),
		Symbol:        "FRZ",
		Subunit:       "ufrz",
		Precision:     6,
		InitialAmount: sdkmath.NewInt(1_000_000),
		Features:      []assetfttypes.Feature{assetfttypes.Feature_freezing},
	}
	_, err := client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(issuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(issueMsg)),
		issueMsg,
	)
	requireT.NoError(err)
	denom := assetfttypes.BuildDenom(issueMsg.Subunit, issuer)
	callbackCoin := sdk.NewInt64Coin(denom, 100)

	// ********** Deploy contracts **********

	// Both contracts send callbackCoin to their recipient from the source callback.
	// The balance of the first one gets frozen, the second one is the control.
	codeID, err := txChain.Wasm.DeployWASMContract(
		ctx, txChain.TxFactoryAuto(), txContractAdmin, ibcwasm.IBCCallbacksSender,
	)
	requireT.NoError(err)

	ibcAmount := txChain.NewCoin(sdkmath.NewInt(1_000))
	instantiate := func(recipient sdk.AccAddress, label string) string {
		payload, err := json.Marshal(map[string]any{
			"recipient": recipient.String(),
			"amount":    callbackCoin,
		})
		requireT.NoError(err)
		contractAddr, err := txChain.Wasm.InstantiateWASMContract(
			ctx,
			txChain.TxFactoryAuto(),
			txContractAdmin,
			integration.InstantiateConfig{
				CodeID:     codeID,
				AccessType: wasmtypes.AccessTypeUnspecified,
				Payload:    payload,
				Amount:     ibcAmount,
				Label:      label,
			},
		)
		requireT.NoError(err)
		return contractAddr
	}
	frozenContract := instantiate(frozenRecipient, "ibc_callbacks_sender_frozen")
	controlContract := instantiate(controlRecipient, "ibc_callbacks_sender_control")

	for _, contractAddr := range []string{frozenContract, controlContract} {
		sendMsg := &banktypes.MsgSend{
			FromAddress: issuer.String(),
			ToAddress:   contractAddr,
			Amount:      sdk.NewCoins(callbackCoin),
		}
		_, err = client.BroadcastTx(
			ctx,
			txChain.ClientContext.WithFromAddress(issuer),
			txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(sendMsg)),
			sendMsg,
		)
		requireT.NoError(err)
	}

	freezeMsg := &assetfttypes.MsgFreeze{
		Sender:  issuer.String(),
		Account: frozenContract,
		Coin:    callbackCoin,
	}
	_, err = client.BroadcastTx(
		ctx,
		txChain.ClientContext.WithFromAddress(issuer),
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(freezeMsg)),
		freezeMsg,
	)
	requireT.NoError(err)

	// ********** Trigger the source callbacks **********

	// Gaia rejects the invalid receiver, so the transfer module refunds ibcAmount on the error acknowledgement.
	// The refund tells us that the acknowledgement, and so the callback, has been processed.
	transferFundsPayload, err := json.Marshal(map[string]transferFunds{
		"transfer_funds": {
			Channel:   txToGaiaChannelID,
			Amount:    ibcAmount,
			Recipient: "invalid-receiver",
		},
	})
	requireT.NoError(err)

	for _, contractAddr := range []string{frozenContract, controlContract} {
		_, err = txChain.Wasm.ExecuteWASMContract(
			ctx,
			txChain.TxFactoryAuto(),
			txContractAdmin,
			contractAddr,
			transferFundsPayload,
			sdk.Coin{},
		)
		requireT.NoError(err)
	}

	// The control callback moves its coins, so the callback path works.
	requireT.NoError(txChain.AwaitForBalance(ctx, t, controlRecipient, callbackCoin))
	requireT.NoError(txChain.AwaitForBalance(ctx, t, sdk.MustAccAddressFromBech32(controlContract), ibcAmount))

	// The frozen contract got its refund, so its callback has run, but the frozen coins did not move.
	requireT.NoError(txChain.AwaitForBalance(ctx, t, sdk.MustAccAddressFromBech32(frozenContract), ibcAmount))

	bankClient := banktypes.NewQueryClient(txChain.ClientContext)
	recipientBalance, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: frozenRecipient.String(),
		Denom:   denom,
	})
	requireT.NoError(err)
	requireT.True(recipientBalance.Balance.IsZero(), "frozen coins must not be sent by the callback")

	contractBalance, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: frozenContract,
		Denom:   denom,
	})
	requireT.NoError(err)
	requireT.Equal(callbackCoin.String(), contractBalance.Balance.String())
}
