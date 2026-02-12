//go:build integrationtests

// IBC v2 integration tests. V2 uses client-based paths and MsgRegisterCounterparty
// instead of the legacy channel handshake.
package ibc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	tmtypes "github.com/cometbft/cometbft/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	gogoproto "github.com/cosmos/gogoproto/proto"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	clientv2types "github.com/cosmos/ibc-go/v10/modules/core/02-client/v2/types"
	channeltypesv2 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/v2/types"
	commitmenttypes "github.com/cosmos/ibc-go/v10/modules/core/23-commitment/types"
	hostv2 "github.com/cosmos/ibc-go/v10/modules/core/24-host/v2"
	ibctmtypes "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
)

const (
	// Relayer and sender funding (in base denom).
	relayerFundAmount = 20_000_000
	senderFundAmount  = 1_500_000
	transferAmount    = 800_000
	// Gas limits for IBC v2 messages.
	createClientGasLimit         = 750_000
	registerCounterpartyGasLimit = 250_000
	// Timeouts: packet expiry, balance await, and height await.
	packetTimeout       = 5 * time.Minute
	awaitBalanceTimeout = 30 * time.Second
	awaitHeightTimeout  = 15 * time.Second
)

// cosmosSDKMerklePath is the Merkle path to the ICS24 provable store on Cosmos SDK chains.
// See IBC core commitment docs (MerklePath in counterparty registration).
var cosmosSDKMerklePath = [][]byte{[]byte("ibc"), {}}

// TestIBCV2TransferTxToGaia exercises the IBC v2 flow end-to-end:
//
//  1. Create Tendermint light clients on both chains and register counterparties
//     (replaces the legacy 4-step channel handshake).
//  2. Send a v2 transfer packet from tx-chain using MsgSendPacket (source client, not channel).
//  3. Wait for the commitment block, update the light client on Gaia, then relay with MsgRecvPacket.
//  4. Assert the recipient receives the expected IBC denom and the packet receipt is recorded.
func TestIBCV2TransferTxToGaia(t *testing.T) {
	t.Parallel()

	ctx, chains := integrationtests.NewChainsTestingContext(t)
	txChain := chains.TXChain
	gaiaChain := chains.Gaia

	// --- Step 1: Relayer accounts ---
	// We use dedicated relayer accounts (not the default faucet) so we control client creation
	// and packet relay. They must be funded to pay for MsgCreateClient, MsgRegisterCounterparty,
	// and MsgRecvPacket.
	txRelayer := txChain.GenAccount()
	gaiaRelayer := gaiaChain.GenAccount()
	fundRelayers(t, ctx, txChain, gaiaChain, txRelayer, gaiaRelayer)

	// --- Step 2: IBC v2 client setup (replaces legacy connection + channel handshake) ---
	// Each chain creates a Tendermint light client that tracks the other chain's consensus.
	// Then we register the counterparty: this tells IBC Core how to route packets between
	// this client and the other chain's client (Merkle path to the provable store).
	txClientID := createTendermintClient(t, ctx, txChain.Chain, gaiaChain, txRelayer)
	gaiaClientID := createTendermintClient(t, ctx, gaiaChain, txChain.Chain, gaiaRelayer)
	registerCounterparty(t, ctx, txChain.Chain, txRelayer, txClientID, gaiaClientID)
	registerCounterparty(t, ctx, gaiaChain, gaiaRelayer, gaiaClientID, txClientID)

	// --- Step 3: Sender and recipient; fund sender ---
	sender := txChain.GenAccount()
	recipient := gaiaChain.GenAccount()
	amount := txChain.NewCoin(sdkmath.NewInt(transferAmount))
	fundSenderForTransfer(t, ctx, txChain, sender)

	// --- Step 4: Build and send v2 packet ---
	// V2 uses MsgSendPacket with source client ID (not channel). The payload is a wrapped
	// transfer packet (port, version, encoding, and serialized FungibleTokenPacketData).
	timeoutTS := uint64(time.Now().Add(packetTimeout).Unix())
	transferPayload := buildTransferPayload(t, buildTransferMsg(txChain, gaiaChain, sender, recipient, amount, timeoutTS))

	sendMsg := channeltypesv2.NewMsgSendPacket(
		txClientID,
		timeoutTS,
		txChain.MustConvertToBech32Address(sender),
		transferPayload,
	)
	// MsgSendPacket is nondeterministic gas; use TxFactoryAuto() to estimate via simulation.
	txResp, err := txChain.BroadcastTxWithSigner(
		ctx,
		txChain.TxFactoryAuto(),
		sender,
		sendMsg,
	)
	require.NoError(t, err)
	require.Greater(t, txResp.Height, int64(0), "tx height must be set for proof query")
	t.Logf("v2 transfer sent, tx hash: %s", txResp.TxHash)

	// --- Step 5: Obtain packet commitment proof ---
	// The commitment was written at txResp.Height. We query that height with Prove=true to get
	// a merkle proof. The destination chain will verify this proof against its light client's
	// consensus state at proofHeight (height+1 per ibc-go convention).
	sequence := nextSequenceSend(t, ctx, txChain.Chain, txClientID) - 1
	proofBz, proofHeight := packetCommitmentProof(t, ctx, txChain.Chain, txClientID, sequence, txResp.Height)

	// --- Step 6: Sync light client on Gaia and relay packet ---
	// Gaia's client must have a consensus state at proofHeight before it can verify the proof.
	// We wait until the source chain has committed that block, then submit a header update
	// so the client stores the root at proofHeight. Then MsgRecvPacket verifies the proof
	// against that root and executes the transfer.
	waitForHeight(t, ctx, txChain.Chain, int64(proofHeight.GetRevisionHeight()))
	updateTendermintClient(t, ctx, gaiaChain, txChain.Chain, gaiaRelayer, gaiaClientID, proofHeight)

	packet := channeltypesv2.Packet{
		Sequence:          sequence,
		SourceClient:      txClientID,
		DestinationClient: gaiaClientID,
		TimeoutTimestamp:  timeoutTS,
		Payloads:          []channeltypesv2.Payload{transferPayload},
	}
	recvMsg := &channeltypesv2.MsgRecvPacket{
		Packet:          packet,
		ProofCommitment: proofBz,
		ProofHeight:     proofHeight,
		Signer:          gaiaChain.MustConvertToBech32Address(gaiaRelayer),
	}
	recvTxResp, err := gaiaChain.BroadcastTxWithSigner(ctx, gaiaChain.TxFactoryAuto(), gaiaRelayer, recvMsg)
	require.NoError(t, err)
	t.Logf("v2 recv packet relayed, tx hash: %s", recvTxResp.TxHash)

	// --- Step 7: Assert recipient balance and packet receipt ---
	// IBC denom in v2 is derived from (port, clientID, base denom); either source or dest
	// client ID may be used depending on how the receiving module mints. We check the
	// receipt to confirm the packet was processed.
	expectedDenoms := expectedIBCDenoms(amount.Denom, txClientID, gaiaClientID)
	ibcDenom := awaitIBCDenom(ctx, t, gaiaChain, recipient, amount.Amount, expectedDenoms)
	require.NotEmpty(t, ibcDenom)

	receiptRes, err := channeltypesv2.NewQueryClient(gaiaChain.ClientContext).PacketReceipt(
		ctx, channeltypesv2.NewQueryPacketReceiptRequest(gaiaClientID, sequence),
	)
	require.NoError(t, err)
	require.True(t, receiptRes.Received, "packet receipt not recorded on destination")
}

// fundRelayers credits both relayer accounts so they can pay for client creation and packet relay.
func fundRelayers(t *testing.T, ctx context.Context, txChain integration.TXChain, gaiaChain integration.Chain, txRelayer, gaiaRelayer sdk.AccAddress) {
	t.Helper()
	txChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: txRelayer,
		Amount:  txChain.NewCoin(sdkmath.NewInt(relayerFundAmount)),
	})
	gaiaChain.Faucet.FundAccounts(ctx, t, integration.FundedAccount{
		Address: gaiaRelayer,
		Amount:  gaiaChain.NewCoin(sdkmath.NewInt(relayerFundAmount)),
	})
}

// fundSenderForTransfer funds the sender and registers MsgTransfer for gas estimation.
func fundSenderForTransfer(t *testing.T, ctx context.Context, txChain integration.TXChain, sender sdk.AccAddress) {
	t.Helper()
	txChain.FundAccountWithOptions(ctx, t, sender, integration.BalancesOptions{
		Messages: []sdk.Msg{&ibctransfertypes.MsgTransfer{}},
		Amount:   sdkmath.NewInt(senderFundAmount),
	})
}

// buildTransferMsg builds a MsgTransfer used only to construct the v2 payload (denom, amount, sender, receiver).
// SourceChannel is unused in v2; the payload carries port and application data.
func buildTransferMsg(
	txChain integration.TXChain,
	gaiaChain integration.Chain,
	sender, recipient sdk.AccAddress,
	amount sdk.Coin,
	timeoutTS uint64,
) *ibctransfertypes.MsgTransfer {
	return &ibctransfertypes.MsgTransfer{
		SourcePort:       ibctransfertypes.PortID,
		SourceChannel:    "",
		Token:            amount,
		Sender:           txChain.MustConvertToBech32Address(sender),
		Receiver:         gaiaChain.MustConvertToBech32Address(recipient),
		TimeoutHeight:    clienttypes.ZeroHeight(),
		TimeoutTimestamp: timeoutTS,
	}
}

// expectedIBCDenoms returns the two possible IBC denom hashes for a single-hop transfer
// (source-client path or destination-client path; receiving chain may use either).
func expectedIBCDenoms(baseDenom, sourceClientID, destClientID string) []string {
	return []string{
		ibctransfertypes.NewDenom(baseDenom, ibctransfertypes.NewHop(ibctransfertypes.PortID, sourceClientID)).IBCDenom(),
		ibctransfertypes.NewDenom(baseDenom, ibctransfertypes.NewHop(ibctransfertypes.PortID, destClientID)).IBCDenom(),
	}
}

// --- Client helpers ---

// createTendermintClient creates a Tendermint light client on chain that tracks counterparty.
// It uses the counterparty's latest header and staking params (unbonding/trusting period).
// Returns the new client ID (e.g. "07-tendermint-1").
func createTendermintClient(
	t *testing.T,
	ctx context.Context,
	chain integration.Chain,
	counterparty integration.Chain,
	signer sdk.AccAddress,
) string {
	t.Helper()

	before := listClientIDs(t, ctx, chain)
	header, err := counterparty.LatestBlockHeader(ctx)
	require.NoError(t, err)

	// Trusting period must be less than unbonding period so the client can detect misbehaviour.
	stakingResp, err := stakingtypes.NewQueryClient(counterparty.ClientContext).Params(ctx, &stakingtypes.QueryParamsRequest{})
	require.NoError(t, err)
	unbonding := stakingResp.Params.UnbondingTime
	trusting := unbonding / 2
	revision := clienttypes.ParseChainID(counterparty.ChainSettings.ChainID)
	latestHeight := clienttypes.NewHeight(revision, uint64(header.Height))

	clientState := ibctmtypes.NewClientState(
		counterparty.ChainSettings.ChainID,
		ibctmtypes.Fraction{Numerator: 1, Denominator: 3},
		trusting,
		unbonding,
		time.Minute,
		latestHeight,
		commitmenttypes.GetSDKSpecs(),
		[]string{"upgrade", "upgradedIBCState"},
	)
	consensusState := ibctmtypes.NewConsensusState(
		header.Time,
		commitmenttypes.NewMerkleRoot(header.AppHash),
		header.NextValidatorsHash,
	)

	msg, err := clienttypes.NewMsgCreateClient(clientState, consensusState, chain.MustConvertToBech32Address(signer))
	require.NoError(t, err)
	_, err = chain.BroadcastTxWithSigner(ctx, chain.TxFactory().WithGas(createClientGasLimit), signer, msg)
	require.NoError(t, err)

	after := listClientIDs(t, ctx, chain)
	clientID := findNewClientID(before, after)
	require.NotEmpty(t, clientID, "client id not found after creation")
	return clientID
}

// registerCounterparty registers the other chain's client as the counterparty for clientID.
// This creates the association needed for IBC Core to route and verify packets. The Merkle
// path tells the verifier where the provable store lives (Cosmos SDK: "ibc" store).
func registerCounterparty(
	t *testing.T,
	ctx context.Context,
	chain integration.Chain,
	signer sdk.AccAddress,
	clientID, counterpartyClientID string,
) {
	t.Helper()
	msg := clientv2types.NewMsgRegisterCounterparty(
		clientID,
		cosmosSDKMerklePath,
		counterpartyClientID,
		chain.MustConvertToBech32Address(signer),
	)
	_, err := chain.BroadcastTxWithSigner(ctx, chain.TxFactory().WithGas(registerCounterpartyGasLimit), signer, msg)
	require.NoError(t, err)
}

// listClientIDs returns all IBC client IDs on the chain (e.g. 07-tendermint-0, 07-tendermint-1).
func listClientIDs(t *testing.T, ctx context.Context, chain integration.Chain) []string {
	t.Helper()
	res, err := clienttypes.NewQueryClient(chain.ClientContext).ClientStates(ctx, &clienttypes.QueryClientStatesRequest{
		Pagination: &query.PageRequest{Limit: query.PaginationMaxLimit},
	})
	require.NoError(t, err)
	ids := make([]string, 0, len(res.ClientStates))
	for _, cs := range res.ClientStates {
		ids = append(ids, cs.ClientId)
	}
	return ids
}

// findNewClientID returns the single client ID that appears in after but not in before.
func findNewClientID(before, after []string) string {
	seen := make(map[string]struct{}, len(before))
	for _, id := range before {
		seen[id] = struct{}{}
	}
	for _, id := range after {
		if _, ok := seen[id]; !ok {
			return id
		}
	}
	return ""
}

// updateTendermintClient submits a header from counterpartyChain to update the light client
// on clientChain to targetHeight. The client can then verify merkle proofs against the
// consensus state at that height (used before relaying a packet).
func updateTendermintClient(
	t *testing.T,
	ctx context.Context,
	clientChain integration.Chain, // chain that stores the client state
	counterpartyChain integration.Chain, // chain we fetch the header from
	signer sdk.AccAddress,
	clientID string,
	targetHeight clienttypes.Height,
) {
	t.Helper()

	csRes, err := clienttypes.NewQueryClient(clientChain.ClientContext).ClientState(ctx, &clienttypes.QueryClientStateRequest{ClientId: clientID})
	require.NoError(t, err)
	var clientState ibctmtypes.ClientState
	require.Equal(t, "/ibc.lightclients.tendermint.v1.ClientState", csRes.ClientState.TypeUrl)
	require.NoError(t, gogoproto.Unmarshal(csRes.ClientState.Value, &clientState))

	trustedHeight := clientState.LatestHeight
	trustedHeightInt := int64(trustedHeight.GetRevisionHeight())
	height := int64(targetHeight.GetRevisionHeight())

	// Fetch signed header and validator set at target height for the update.
	commitRes, err := counterpartyChain.ClientContext.RPCClient().Commit(ctx, &height)
	require.NoError(t, err)
	require.NotNil(t, commitRes.SignedHeader)
	signedHeaderProto := commitRes.SignedHeader.ToProto()

	valsRes, err := counterpartyChain.ClientContext.RPCClient().Validators(ctx, &height, nil, nil)
	require.NoError(t, err)
	valSet := tmtypes.NewValidatorSet(valsRes.Validators)
	valSetProto, err := valSet.ToProto()
	require.NoError(t, err)

	trustedValsRes, err := counterpartyChain.ClientContext.RPCClient().Validators(ctx, &trustedHeightInt, nil, nil)
	require.NoError(t, err)
	trustedValSetProto, err := tmtypes.NewValidatorSet(trustedValsRes.Validators).ToProto()
	require.NoError(t, err)

	header := &ibctmtypes.Header{
		SignedHeader:      signedHeaderProto,
		ValidatorSet:      valSetProto,
		TrustedHeight:     trustedHeight,
		TrustedValidators: trustedValSetProto,
	}
	// Tendermint header must match the validator set we fetched.
	require.True(t, bytes.Equal(header.SignedHeader.Header.ValidatorsHash, valSet.Hash()), "validator hash mismatch")

	anyHeader, err := codectypes.NewAnyWithValue(header)
	require.NoError(t, err)
	msg := &clienttypes.MsgUpdateClient{
		ClientId:      clientID,
		ClientMessage: anyHeader,
		Signer:        clientChain.MustConvertToBech32Address(signer),
	}
	_, err = clientChain.BroadcastTxWithSigner(ctx, clientChain.TxFactoryAuto(), signer, msg)
	require.NoError(t, err)
}

// --- Channel / proof helpers ---

// nextSequenceSend returns the next sequence number to be used for sending on the given client.
// After a send, the used sequence is nextSequenceSend - 1.
func nextSequenceSend(t *testing.T, ctx context.Context, chain integration.Chain, clientID string) uint64 {
	t.Helper()
	res, err := channeltypesv2.NewQueryClient(chain.ClientContext).NextSequenceSend(
		ctx, channeltypesv2.NewQueryNextSequenceSendRequest(clientID),
	)
	require.NoError(t, err)
	return res.NextSequenceSend
}

// packetCommitmentProof queries the IBC store at commitmentBlockHeight with Prove=true and
// returns the serialized merkle proof plus the proofHeight to pass to MsgRecvPacket.
// The destination chain verifies the proof against its light client's consensus state at
// proofHeight. ibc-go expects proofHeight = commitmentBlockHeight + 1.
func packetCommitmentProof(
	t *testing.T,
	ctx context.Context,
	chain integration.Chain,
	clientID string,
	sequence uint64,
	commitmentBlockHeight int64,
) (proofBz []byte, proofHeight clienttypes.Height) {
	t.Helper()

	key := hostv2.PacketCommitmentKey(clientID, sequence)
	abciRes, err := chain.ClientContext.RPCClient().ABCIQueryWithOptions(
		ctx, "/store/ibc/key", key,
		rpcclient.ABCIQueryOptions{Prove: true, Height: commitmentBlockHeight},
	)
	require.NoError(t, err)
	require.Zero(t, abciRes.Response.Code, "abci query error: %d %s", abciRes.Response.Code, abciRes.Response.Log)
	require.NotNil(t, abciRes.Response.ProofOps)

	merkleProof, err := commitmenttypes.ConvertProofs(abciRes.Response.ProofOps)
	require.NoError(t, err)
	proofBz, err = gogoproto.Marshal(&merkleProof)
	require.NoError(t, err)

	// IBC verifier expects proofHeight to be the height of the block that contains the commitment.
	// Query at commitmentBlockHeight returns state at end of that block; reporting height+1 matches ibc-go.
	revision := clienttypes.ParseChainID(chain.ChainSettings.ChainID)
	proofHeight = clienttypes.NewHeight(uint64(revision), uint64(commitmentBlockHeight+1))
	return proofBz, proofHeight
}

// --- Transfer helpers ---

// buildTransferPayload wraps FungibleTokenPacketData in a v2 Payload (port, version, encoding, data).
func buildTransferPayload(t *testing.T, msg *ibctransfertypes.MsgTransfer) channeltypesv2.Payload {
	t.Helper()
	data := ibctransfertypes.NewFungibleTokenPacketData(
		msg.Token.Denom,
		msg.Token.Amount.String(),
		msg.Sender,
		msg.Receiver,
		msg.Memo,
	)
	bz, err := ibctransfertypes.MarshalPacketData(data, ibctransfertypes.V1, ibctransfertypes.EncodingProtobuf)
	require.NoError(t, err)
	return channeltypesv2.NewPayload(
		ibctransfertypes.PortID,
		ibctransfertypes.PortID,
		ibctransfertypes.V1,
		ibctransfertypes.EncodingProtobuf,
		bz,
	)
}

// awaitIBCDenom waits until addr has an IBC denom balance equal to amount, then returns that denom.
// It asserts the denom is one of expectedDenoms (v2 can mint with source or dest client ID in the path).
func awaitIBCDenom(ctx context.Context, t *testing.T, chain integration.Chain, addr sdk.AccAddress, amount sdkmath.Int, expectedDenoms []string) string {
	t.Helper()

	var denom string
	err := chain.AwaitState(ctx, func(checkCtx context.Context) error {
		balances, err := banktypes.NewQueryClient(chain.ClientContext).AllBalances(
			checkCtx, &banktypes.QueryAllBalancesRequest{Address: chain.MustConvertToBech32Address(addr)},
		)
		if err != nil {
			return err
		}
		for _, coin := range balances.Balances {
			if strings.HasPrefix(coin.Denom, "ibc/") && coin.Amount.Equal(amount) {
				denom = coin.Denom
				return nil
			}
		}
		return fmt.Errorf("awaiting IBC balance (expected denoms %v)", expectedDenoms)
	}, integration.WithAwaitStateTimeout(awaitBalanceTimeout))
	require.NoError(t, err)
	require.Contains(t, expectedDenoms, denom, "unexpected IBC denom for received amount")
	return denom
}

// --- Chain helpers ---

// waitForHeight blocks until the chain's latest block height is at least targetHeight.
// Used to ensure the commitment block is committed before we update the light client.
func waitForHeight(t *testing.T, ctx context.Context, chain integration.Chain, targetHeight int64) {
	t.Helper()
	require.NoError(t, chain.AwaitState(ctx, func(checkCtx context.Context) error {
		status, err := chain.ClientContext.RPCClient().Status(checkCtx)
		if err != nil {
			return err
		}
		if status.SyncInfo.LatestBlockHeight < targetHeight {
			return errors.New("waiting for target height")
		}
		return nil
	}, integration.WithAwaitStateTimeout(awaitHeightTimeout)))
}
