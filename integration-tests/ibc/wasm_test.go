//go:build integrationtests

package ibc

import (
	"context"
	_ "embed"
	"encoding/json"
	"reflect"
	"testing"
	"time"
	"unsafe"

	sdkmath "cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcchanneltypes "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	ibcwasm "github.com/tokenize-x/tx-chain/v6/integration-tests/contracts/ibc"
	"github.com/tokenize-x/tx-chain/v6/testutil/event"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
	"github.com/tokenize-x/tx-tools/pkg/retry"
)

type ibcTimeoutBlock struct {
	Revision uint64 `json:"revision"`
	Height   uint64 `json:"height"`
}

type ibcTimeout struct {
	Block ibcTimeoutBlock `json:"block"`
}

//nolint:tagliatelle // wasm requirements
type ibcTransferRequest struct {
	ChannelID string     `json:"channel_id"`
	ToAddress string     `json:"to_address"`
	Amount    sdk.Coin   `json:"amount"`
	Timeout   ibcTimeout `json:"timeout"`
}

type ibcTransferMethod string

const (
	ibcTransferMethodTransfer ibcTransferMethod = "transfer"
)

type ibcCallChannelRequest struct {
	Channel string `json:"channel"`
}

type ibcCallCountResponse struct {
	Count uint32 `json:"count"`
}

type ibcCallMethod string

const (
	ibcCallMethodIncrement ibcCallMethod = "increment"
	ibcCallMethodGetCount  ibcCallMethod = "get_count"
)

// TestIBCTransferFromSmartContract tests the IBCTransfer from the contract.
func TestIBCTransferFromSmartContract(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	osmosisChain := chains.Osmosis

	osmosisToTXChannelID := osmosisChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, txChain.ChainContext,
	)
	txToOsmosisChannelID := txChain.AwaitForIBCChannelID(
		ctx, t, ibctransfertypes.PortID, osmosisChain.ChainContext,
	)

	txAdmin := txChain.GenAccount()
	osmosisRecipient := osmosisChain.GenAccount()

	txChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: txAdmin,
		Amount:  txChain.NewCoin(sdkmath.NewInt(2000000)),
	})
	sendToOsmosisCoin := txChain.NewCoin(sdkmath.NewInt(1000))

	txBankClient := banktypes.NewQueryClient(txChain.ClientContext)

	// deploy the contract and fund it
	contractAddr, _, err := txChain.Wasm.DeployAndInstantiateWASMContract(
		ctx,
		txChain.TxFactoryAuto(),
		txAdmin,
		ibcwasm.IBCTransferWASM,
		integration.InstantiateConfig{
			AccessType: wasmtypes.AccessTypeUnspecified,
			Payload:    ibcwasm.EmptyPayload,
			Amount:     sendToOsmosisCoin,
			Label:      "ibc_transfer",
		},
	)
	requireT.NoError(err)

	// get the contract balance and check total
	contractBalance, err := txBankClient.Balance(ctx,
		&banktypes.QueryBalanceRequest{
			Address: contractAddr,
			Denom:   sendToOsmosisCoin.Denom,
		})
	requireT.NoError(err)
	requireT.Equal(sendToOsmosisCoin.Amount.String(), contractBalance.Balance.Amount.String())

	txChainHeight, err := txChain.GetLatestConsensusHeight(
		ctx,
		ibctransfertypes.PortID,
		txToOsmosisChannelID,
	)
	requireT.NoError(err)

	transferPayload, err := json.Marshal(map[ibcTransferMethod]ibcTransferRequest{
		ibcTransferMethodTransfer: {
			ChannelID: txToOsmosisChannelID,
			ToAddress: osmosisChain.MustConvertToBech32Address(osmosisRecipient),
			Amount:    sendToOsmosisCoin,
			Timeout: ibcTimeout{
				Block: ibcTimeoutBlock{
					Revision: txChainHeight.RevisionNumber,
					Height:   txChainHeight.RevisionHeight + 1000,
				},
			},
		},
	})
	requireT.NoError(err)

	_, err = txChain.Wasm.ExecuteWASMContract(
		ctx,
		txChain.TxFactoryAuto(),
		txAdmin,
		contractAddr,
		transferPayload,
		sdk.Coin{},
	)
	requireT.NoError(err)

	contractBalance, err = txBankClient.Balance(ctx,
		&banktypes.QueryBalanceRequest{
			Address: contractAddr,
			Denom:   sendToOsmosisCoin.Denom,
		})
	requireT.NoError(err)
	requireT.Equal(sdkmath.ZeroInt().String(), contractBalance.Balance.Amount.String())

	expectedOsmosisRecipientBalance := sdk.NewCoin(
		ConvertToIBCDenom(osmosisToTXChannelID, sendToOsmosisCoin.Denom),
		sendToOsmosisCoin.Amount,
	)
	requireT.NoError(osmosisChain.AwaitForBalance(ctx, t, osmosisRecipient, expectedOsmosisRecipientBalance))
}

// TestIBCCallFromSmartContract tests the IBC contract calls.
func TestIBCCallFromSmartContract(t *testing.T) {
	// we don't enable the t.Parallel here since that test uses the config unseal hack because of the cosmos relayer
	// implementation
	restoreSDKConfig := unsealSDKConfig()
	defer restoreSDKConfig()

	// channelIBCVersion is the version defined in the ibc.rs in the smart contract
	const channelIBCVersion = "counter-1"

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	osmosisChain := chains.Osmosis

	txWasmClient := wasmtypes.NewQueryClient(txChain.ClientContext)
	osmosisWasmClient := wasmtypes.NewQueryClient(osmosisChain.ClientContext)

	txCaller := txChain.GenAccount()
	osmosisCaller := osmosisChain.GenAccount()

	txChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: txCaller,
		Amount:  txChain.NewCoin(sdkmath.NewInt(2000000)),
	})

	osmosisChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: osmosisCaller,
		Amount:  osmosisChain.NewCoin(sdkmath.NewInt(2000000)),
	})

	txContractAddr, _, err := txChain.Wasm.DeployAndInstantiateWASMContract(
		ctx,
		txChain.TxFactoryAuto(),
		txCaller,
		ibcwasm.IBCCallWASM,
		integration.InstantiateConfig{
			Admin:      txCaller,
			AccessType: wasmtypes.AccessTypeUnspecified,
			Payload:    ibcwasm.EmptyPayload,
			Label:      "ibc_call",
		},
	)
	requireT.NoError(err)

	osmosisContractAddr, _, err := osmosisChain.Wasm.DeployAndInstantiateWASMContract(
		ctx,
		osmosisChain.TxFactoryAuto(),
		osmosisCaller,
		ibcwasm.IBCCallWASM,
		integration.InstantiateConfig{
			Admin:      osmosisCaller,
			AccessType: wasmtypes.AccessTypeUnspecified,
			Payload:    ibcwasm.EmptyPayload,
			Label:      "ibc_call",
		},
	)
	requireT.NoError(err)

	txContractInfoRes, err := txWasmClient.ContractInfo(ctx, &wasmtypes.QueryContractInfoRequest{
		Address: txContractAddr,
	})
	requireT.NoError(err)
	txIBCPort := txContractInfoRes.IBCPortID
	requireT.NotEmpty(txIBCPort)
	t.Logf("tx-chain contract IBC port:%s", txIBCPort)

	osmosisContractInfoRes, err := osmosisWasmClient.ContractInfo(ctx, &wasmtypes.QueryContractInfoRequest{
		Address: osmosisContractAddr,
	})
	requireT.NoError(err)
	osmosisIBCPort := osmosisContractInfoRes.IBCPortID
	requireT.NotEmpty(osmosisIBCPort)
	t.Logf("Osmisis contract IBC port:%s", osmosisIBCPort)

	txIbcChannelClient := ibcchanneltypes.NewQueryClient(txChain.ClientContext)

	_, srcConnectionID := txChain.AwaitForIBCClientAndConnectionIDs(ctx, t, osmosisChain.ChainSettings.ChainID)
	msgChannelOpenInit := ibcchanneltypes.NewMsgChannelOpenInit(
		txIBCPort,
		channelIBCVersion,
		ibcchanneltypes.UNORDERED,
		[]string{srcConnectionID},
		osmosisIBCPort,
		txChain.MustConvertToBech32Address(txCaller),
	)
	res, err := chains.TXChain.BroadcastTxWithSigner(
		ctx,
		chains.TXChain.TxFactoryAuto(),
		txCaller,
		msgChannelOpenInit,
	)
	requireT.NoError(err)

	txToOsmosisChannelID, err := event.FindStringEventAttribute(
		res.Events, ibcchanneltypes.EventTypeChannelOpenInit, ibcchanneltypes.AttributeKeyChannelID,
	)
	requireT.NoError(err)

	osmosisToTXChannelID := ""

	require.NoError(t, txChain.AwaitState(ctx, func(ctx context.Context) error {
		ibcChanRes, err := txIbcChannelClient.Channel(ctx, &ibcchanneltypes.QueryChannelRequest{
			PortId:    txIBCPort,
			ChannelId: txToOsmosisChannelID,
		})
		if err != nil {
			return retry.Retryable(errors.Errorf(
				"IBC channel is not ready yet, %s",
				err,
			))
		}
		if ibcChanRes.Channel.State != ibcchanneltypes.OPEN {
			return retry.Retryable(errors.Errorf(
				"IBC channel is not open yet, it is still in %s",
				ibcChanRes.Channel.State.String(),
			))
		}
		osmosisToTXChannelID = ibcChanRes.Channel.Counterparty.ChannelId
		return nil
	}))

	t.Logf(
		"Channels are ready tx-chain channel ID:%s, osmosis channel ID:%s",
		txToOsmosisChannelID,
		osmosisToTXChannelID,
	)

	t.Logf("Sending two IBC transactions from tx-chain contract to osmosis contract")
	awaitWasmCounterValue(ctx, t, txChain.Chain, txToOsmosisChannelID, txContractAddr, 0)
	awaitWasmCounterValue(ctx, t, osmosisChain, osmosisToTXChannelID, osmosisContractAddr, 0)

	// execute tx-chain counter twice
	executeWasmIncrement(ctx, requireT, txChain.Chain, txCaller, txToOsmosisChannelID, txContractAddr)
	executeWasmIncrement(ctx, requireT, txChain.Chain, txCaller, txToOsmosisChannelID, txContractAddr)

	// check that current state is expected
	// the order of assertion is important because we are waiting for the expected non-zero counter first to be sure
	// that async operation is completed fully before the second assertion
	awaitWasmCounterValue(ctx, t, osmosisChain, osmosisToTXChannelID, osmosisContractAddr, 2)
	awaitWasmCounterValue(ctx, t, txChain.Chain, txToOsmosisChannelID, txContractAddr, 0)

	t.Logf("Sending three IBC transactions from osmosis contract to tx-chain contract")
	executeWasmIncrement(ctx, requireT, osmosisChain, osmosisCaller, osmosisToTXChannelID, osmosisContractAddr)
	executeWasmIncrement(ctx, requireT, osmosisChain, osmosisCaller, osmosisToTXChannelID, osmosisContractAddr)
	executeWasmIncrement(ctx, requireT, osmosisChain, osmosisCaller, osmosisToTXChannelID, osmosisContractAddr)

	// check that current state is expected, the order of assertion is important
	awaitWasmCounterValue(ctx, t, txChain.Chain, txToOsmosisChannelID, txContractAddr, 3)
	awaitWasmCounterValue(ctx, t, osmosisChain, osmosisToTXChannelID, osmosisContractAddr, 2)
}

// executeWasmIncrement executes increment method on the contract which calls another contract and increments
// the counter.
func executeWasmIncrement(
	ctx context.Context,
	requireT *require.Assertions,
	chain integration.Chain,
	caller sdk.AccAddress,
	channelID, contractAddr string,
) {
	incrementPayload, err := json.Marshal(map[ibcCallMethod]ibcCallChannelRequest{
		ibcCallMethodIncrement: {
			Channel: channelID,
		},
	})
	requireT.NoError(err)

	_, err = chain.Wasm.ExecuteWASMContract(
		ctx,
		chain.TxFactoryAuto(),
		caller,
		contractAddr,
		incrementPayload,
		sdk.Coin{},
	)
	requireT.NoError(err)
}

// awaitWasmCounterValue waits until the count on the counter contract reaches the expectedCount.
func awaitWasmCounterValue(
	ctx context.Context,
	t *testing.T,
	chain integration.Chain,
	channelID, contractAddress string,
	expectedCount uint32,
) {
	t.Helper()

	t.Logf("Awaiting for count:%d, chainID: %s, channel:%s", expectedCount, chain.ChainSettings.ChainID, channelID)

	retryCtx, retryCancel := context.WithTimeout(ctx, time.Minute)
	defer retryCancel()
	require.NoError(t, retry.Do(retryCtx, time.Second, func() error {
		getCountPayload, err := json.Marshal(map[ibcCallMethod]ibcCallChannelRequest{
			ibcCallMethodGetCount: {
				Channel: channelID,
			},
		})
		require.NoError(t, err)
		queryCountOut, err := chain.Wasm.QueryWASMContract(retryCtx, contractAddress, getCountPayload)
		require.NoError(t, err)
		var queryCountRes ibcCallCountResponse
		err = json.Unmarshal(queryCountOut, &queryCountRes)
		require.NoError(t, err)

		if queryCountRes.Count != expectedCount {
			return retry.Retryable(errors.Errorf(
				"counter is still not equal to expected, current:%d, expected:%d",
				queryCountRes.Count,
				expectedCount,
			))
		}

		return nil
	}))

	t.Logf("Received expected count of %d.", expectedCount)
}

func unsealSDKConfig() func() {
	config := sdk.GetConfig()
	// unseal the config
	setField(config, "sealed", false)
	setField(config, "sealedch", make(chan struct{}))

	bech32AccountAddrPrefix := config.GetBech32AccountAddrPrefix()
	bech32AccountPubPrefix := config.GetBech32AccountPubPrefix()
	bech32ValidatorAddrPrefix := config.GetBech32ValidatorAddrPrefix()
	bech32ValidatorPubPrefix := config.GetBech32ValidatorPubPrefix()
	bech32ConsensusAddrPrefix := config.GetBech32ConsensusAddrPrefix()
	bech32ConsensusPubPrefix := config.GetBech32ConsensusPubPrefix()
	coinType := config.GetCoinType()

	return func() {
		config.SetBech32PrefixForAccount(bech32AccountAddrPrefix, bech32AccountPubPrefix)
		config.SetBech32PrefixForValidator(bech32ValidatorAddrPrefix, bech32ValidatorPubPrefix)
		config.SetBech32PrefixForConsensusNode(bech32ConsensusAddrPrefix, bech32ConsensusPubPrefix)
		config.SetCoinType(coinType)

		config.Seal()
	}
}

func setField(object interface{}, fieldName string, value interface{}) {
	rs := reflect.ValueOf(object).Elem()
	field := rs.FieldByName(fieldName)
	// rf can't be read or set.
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).
		Elem().
		Set(reflect.ValueOf(value))
}
