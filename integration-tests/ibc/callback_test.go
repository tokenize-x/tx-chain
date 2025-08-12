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

	integrationtests "github.com/CoreumFoundation/coreum/v6/integration-tests"
	ibcwasm "github.com/CoreumFoundation/coreum/v6/integration-tests/contracts/ibc"
	"github.com/CoreumFoundation/coreum/v6/testutil/integration"
)

// TestIBCWASMCallback tests ibc-callback integration by deploying the ibc-callbacks-counter WASM contract
// on Coreum and using it as a callback for IBC transfer sent to Gaia.
func TestIBCWASMCallback(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	coreumChain := chains.Coreum
	gaiaChain := chains.Gaia

	gaiaChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, coreumChain.ChainContext,
	)
	coreumToGaiaChannelID := coreumChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, gaiaChain.ChainContext,
	)

	coreumContractAdmin := coreumChain.GenAccount()
	coreumSender := coreumChain.GenAccount()
	coreumReceiver := coreumChain.GenAccount()

	gaiaSender := gaiaChain.GenAccount()
	gaiaReceiver := gaiaChain.GenAccount()

	coreumChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: coreumContractAdmin,
			Amount:  coreumChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
		integration.FundedAccount{
			Address: coreumSender,
			Amount:  coreumChain.NewCoin(sdkmath.NewInt(20_000_000)),
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

	coreumContractAddr, _, err := coreumChain.Wasm.DeployAndInstantiateWASMContract(
		ctx,
		coreumChain.TxFactoryAuto(),
		coreumContractAdmin,
		ibcwasm.IBCCallbacksCounter,
		integration.InstantiateConfig{
			Admin:      coreumContractAdmin,
			AccessType: wasmtypes.AccessTypeUnspecified,
			Payload:    initialPayload,
			Label:      "ibc_callbacks_counter",
		},
	)
	requireT.NoError(err)

	_, coreumContract, err := bech32.DecodeAndConvert(coreumContractAddr)
	requireT.NoError(err)

	coreumChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: coreumContract,
			Amount:  coreumChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)

	sendToCoreumCoin := gaiaChain.NewCoin(sdkmath.NewInt(1))

	ibcCallbackMemo := fmt.Sprintf(`{"dest_callback": {
					"address": "%s",
					"gas_limit": "%d"
				  }}`, coreumContractAddr, 10_000_000)

	// We send a Gaia to Coreum transfer here to trigger the dest_callback of the smart contract deployed on Coreum
	_, err = gaiaChain.ExecuteIBCTransferWithMemo(
		ctx,
		t,
		gaiaChain.TxFactory().WithGas(2_000_000),
		gaiaSender,
		sendToCoreumCoin,
		coreumChain.ChainContext,
		coreumReceiver.String(),
		ibcCallbackMemo,
	)
	requireT.NoError(err)

	awaitCounterContractState(
		ctx,
		t,
		coreumChain,
		coreumContractAddr,
		coreumContractAddr,
		1,
		sdk.Coins{},
	)

	_, err = gaiaChain.ExecuteIBCTransferWithMemo(
		ctx,
		t,
		gaiaChain.TxFactory().WithGas(2_000_000),
		gaiaSender,
		sendToCoreumCoin,
		coreumChain.ChainContext,
		coreumReceiver.String(),
		ibcCallbackMemo,
	)
	requireT.NoError(err)

	awaitCounterContractState(
		ctx,
		t,
		coreumChain,
		coreumContractAddr,
		coreumContractAddr,
		2,
		sdk.Coins{},
	)

	// We send a Coreum to Gaia transfer here to trigger the src_callback of the smart contract deployed on Coreum
	// in the transfer_funds method, it sends an IBC transfer with src_callback filled in memo
	transferFundsPayload, err := json.Marshal(map[string]transferFunds{
		"transfer_funds": {
			Channel:   coreumToGaiaChannelID,
			Amount:    coreumChain.NewCoin(sdkmath.NewInt(1)),
			Recipient: gaiaChain.MustConvertToBech32Address(gaiaReceiver),
		},
	})
	requireT.NoError(err)

	_, err = coreumChain.Wasm.ExecuteWASMContract(
		ctx,
		coreumChain.TxFactory().WithGas(2_000_000),
		coreumSender,
		coreumContractAddr,
		transferFundsPayload,
		sdk.Coin{},
	)
	requireT.NoError(err)

	awaitCounterContractState(
		ctx,
		t,
		coreumChain,
		coreumContractAddr,
		coreumContractAddr,
		3,
		sdk.Coins{},
	)
}

type transferFunds struct {
	Channel   string   `json:"channel"`
	Amount    sdk.Coin `json:"amount"`
	Recipient string   `json:"recipient"`
}
