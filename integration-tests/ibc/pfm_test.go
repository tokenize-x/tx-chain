//go:build integrationtests

package ibc

import (
	"encoding/json"
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	packetforwardtypes "github.com/cosmos/ibc-apps/middleware/packet-forward-middleware/v10/packetforward/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v7/integration-tests"
	"github.com/tokenize-x/tx-chain/v7/testutil/integration"
)

const (
	// It is recommended to use an invalid bech32 string (such as "pfm") for the receiver on intermediate chains.
	// More details here:
	//nolint:lll // https://github.com/cosmos/ibc-apps/tree/middleware/packet-forward-middleware/v7.1.3/middleware/packet-forward-middleware#intermediate-receivers
	pfmRecipient = "pfm"
)

// Forward metadata example:
//
//	{
//	 "forward": {
//	   "receiver": "chain-c-bech32-address",
//	   "port": "transfer",
//	   "channel": "channel-123" // this is the chain C on chain B channel.
//	 }
//	}
type pfmForwardMetadata struct {
	Forward packetforwardtypes.ForwardMetadata `json:"forward"`
}

// TestPFMViaTxForOsmosisToken tests the packet
// forwarding middleware integration into TX by sending Osmosis native token:
// Osmosis -> TX -> Gaia IBC transfer.
func TestPFMViaTxForOsmosisToken(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	osmosisChain := chains.Osmosis
	gaiaChain := chains.Gaia

	osmosisSender := osmosisChain.GenAccount()
	txSender := txChain.GenAccount()

	gaiaReceiver := gaiaChain.GenAccount()

	osmosisChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: osmosisSender,
			Amount:  osmosisChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)
	txChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: txSender,
			Amount:  txChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)

	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx,
		t,
		ibctransfertypes.PortID,
		gaiaChain.ChainContext,
	)
	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx,
		t,
		ibctransfertypes.PortID,
		txChain.ChainContext,
	)
	txToOsmosiChannelID := txChain.AwaitForIBCChannelID(
		ctx,
		t,
		ibctransfertypes.PortID,
		osmosisChain.ChainContext,
	)

	forwardMetadata := pfmForwardMetadata{
		Forward: packetforwardtypes.ForwardMetadata{
			Receiver: gaiaChain.MustConvertToBech32Address(gaiaReceiver),
			Port:     ibctransfertypes.PortID,
			Channel:  txToGaiaChannelID,
		},
	}

	pfmMemo, err := json.Marshal(forwardMetadata)
	requireT.NoError(err)

	sendToGaiaCoin := osmosisChain.NewCoin(sdkmath.NewInt(10_000_000))
	_, err = osmosisChain.ExecuteIBCTransferWithMemo(
		ctx,
		t,
		osmosisChain.TxFactoryAuto(),
		osmosisSender,
		sendToGaiaCoin,
		txChain.ChainContext,
		pfmRecipient,
		string(pfmMemo),
	)
	requireT.NoError(err)

	// Packet denom is the IBC denom sent from tx to gaia in raw format (without bech32 encoding).
	// Example: "transfer/channel-1/stake"
	packetDenom := fmt.Sprintf("%s/%s/%s", ibctransfertypes.PortID, txToOsmosiChannelID, sendToGaiaCoin.Denom)
	// So a received packet on gaia looks like this:
	// port: "transfer"
	// channel: "channel-0"
	// denom: "transfer/channel-1/stake"
	receivedDenomOnGaia := ConvertToIBCDenom(gaiaToTXChannelID, packetDenom)

	expectedGaiaReceiverBalance := sdk.NewCoin(receivedDenomOnGaia, sendToGaiaCoin.Amount)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaReceiver, expectedGaiaReceiverBalance))
}

// TestPFMViaTXforTXToken tests the packet forwarding middleware integration into TX
// by sending TX native token to Osmosis and then sending it to gaia via TX:
// tx1: TX -> Osmosis, tx2: Osmosis -> TX -> Gaia.
func TestPFMViaTXforTXToken(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	requireT := require.New(t)
	txChain := chains.TXChain
	osmosisChain := chains.Osmosis
	gaiaChain := chains.Gaia

	osmosisSender := osmosisChain.GenAccount()
	txSender := txChain.GenAccount()

	gaiaReceiver := gaiaChain.GenAccount()

	osmosisChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: osmosisSender,
			Amount:  osmosisChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)
	txChain.Faucet.FundAccounts(ctx, t,
		integration.FundedAccount{
			Address: txSender,
			Amount:  txChain.NewCoin(sdkmath.NewInt(20_000_000)),
		},
	)

	txToGaiaChannelID := txChain.AwaitForIBCChannelID(
		ctx,
		t,
		ibctransfertypes.PortID,
		gaiaChain.ChainContext,
	)
	gaiaToTXChannelID := gaiaChain.AwaitForIBCChannelID(
		ctx,
		t,
		ibctransfertypes.PortID,
		txChain.ChainContext,
	)
	osmosisToTXChannelID := osmosisChain.AwaitForIBCChannelID(
		ctx,
		t,
		ibctransfertypes.PortID,
		txChain.ChainContext,
	)

	// ********** Send funds to Osmosis **********

	sendToOsmosisCoin := txChain.NewCoin(sdkmath.NewInt(10_000_000))
	_, err := txChain.ExecuteIBCTransfer(
		ctx,
		t,
		txChain.TxFactory().WithGas(txChain.GasLimitByMsgs(&ibctransfertypes.MsgTransfer{})),
		txSender,
		sendToOsmosisCoin,
		osmosisChain.ChainContext,
		osmosisSender,
	)
	requireT.NoError(err)

	expectedOsmosisRecipientBalance := sdk.NewCoin(
		ConvertToIBCDenom(osmosisToTXChannelID, sendToOsmosisCoin.Denom),
		sendToOsmosisCoin.Amount,
	)
	requireT.NoError(osmosisChain.AwaitForBalance(ctx, t, osmosisSender, expectedOsmosisRecipientBalance))

	// ********** Send funds to Gaia via TX using PFM **********

	forwardMetadata := pfmForwardMetadata{
		Forward: packetforwardtypes.ForwardMetadata{
			Receiver: gaiaChain.MustConvertToBech32Address(gaiaReceiver),
			Port:     ibctransfertypes.PortID,
			Channel:  txToGaiaChannelID,
		},
	}

	pfmMemo, err := json.Marshal(forwardMetadata)
	requireT.NoError(err)

	sendToGaiaCoin := expectedOsmosisRecipientBalance
	_, err = osmosisChain.ExecuteIBCTransferWithMemo(
		ctx,
		t,
		osmosisChain.TxFactoryAuto(),
		osmosisSender,
		sendToGaiaCoin,
		txChain.ChainContext,
		pfmRecipient,
		string(pfmMemo),
	)
	requireT.NoError(err)

	// Note that denom is resolved in the same way as if was sent from TX to Gaia directly.
	expectedGaiaReceiverBalance := sdk.NewCoin(
		ConvertToIBCDenom(gaiaToTXChannelID, txChain.ChainSettings.Denom),
		sendToGaiaCoin.Amount,
	)
	requireT.NoError(gaiaChain.AwaitForBalance(ctx, t, gaiaReceiver, expectedGaiaReceiverBalance))
}
