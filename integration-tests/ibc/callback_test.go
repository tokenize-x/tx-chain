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
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	ibcwasm "github.com/tokenize-x/tx-chain/v6/integration-tests/contracts/ibc"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
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
