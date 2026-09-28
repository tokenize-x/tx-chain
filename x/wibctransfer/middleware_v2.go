package wibctransfer

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	channeltypesv2 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/v2/types"
	ibcapi "github.com/cosmos/ibc-go/v10/modules/core/api"

	"github.com/tokenize-x/tx-chain/v8/x/wibctransfer/types"
)

var (
	_ ibcapi.IBCModule             = PurposeMiddlewareV2{}
	_ ibcapi.PacketDataUnmarshaler = PurposeMiddlewareV2{}
)

// IBCModuleV2 is an IBC v2 application which can unmarshal its packet data.
// The IBC v2 callbacks middleware wrapping the transfer stack requires both.
type IBCModuleV2 interface {
	ibcapi.IBCModule
	ibcapi.PacketDataUnmarshaler
}

// PurposeMiddlewareV2 adds information about IBC v2 transfer purpose to the context.
// On IBC v1 the outgoing purpose is set by the transfer keeper wrapper, but IBC v2 sends
// go straight to the transfer keeper, so this middleware sets it on send as well.
type PurposeMiddlewareV2 struct {
	IBCModuleV2
}

// NewPurposeMiddlewareV2 returns middleware adding purpose to the context.
func NewPurposeMiddlewareV2(module IBCModuleV2) PurposeMiddlewareV2 {
	return PurposeMiddlewareV2{
		IBCModuleV2: module,
	}
}

// OnSendPacket adds purpose-out to the context and calls the upper implementation.
func (im PurposeMiddlewareV2) OnSendPacket(
	ctx sdk.Context,
	sourceClient string,
	destinationClient string,
	sequence uint64,
	payload channeltypesv2.Payload,
	signer sdk.AccAddress,
) error {
	ctx = types.WithPurpose(ctx, types.PurposeOut)
	return im.IBCModuleV2.OnSendPacket(ctx, sourceClient, destinationClient, sequence, payload, signer)
}

// OnRecvPacket adds purpose-in to the context and calls the upper implementation.
func (im PurposeMiddlewareV2) OnRecvPacket(
	ctx sdk.Context,
	sourceClient string,
	destinationClient string,
	sequence uint64,
	payload channeltypesv2.Payload,
	relayer sdk.AccAddress,
) channeltypesv2.RecvPacketResult {
	ctx = types.WithPurpose(ctx, types.PurposeIn)
	return im.IBCModuleV2.OnRecvPacket(ctx, sourceClient, destinationClient, sequence, payload, relayer)
}

// OnAcknowledgementPacket adds purpose-ack to the context and calls the upper implementation.
func (im PurposeMiddlewareV2) OnAcknowledgementPacket(
	ctx sdk.Context,
	sourceClient string,
	destinationClient string,
	sequence uint64,
	acknowledgement []byte,
	payload channeltypesv2.Payload,
	relayer sdk.AccAddress,
) error {
	ctx = types.WithPurpose(ctx, types.PurposeAck)
	return im.IBCModuleV2.OnAcknowledgementPacket(
		ctx, sourceClient, destinationClient, sequence, acknowledgement, payload, relayer,
	)
}

// OnTimeoutPacket adds purpose-timeout to the context and calls the upper implementation.
func (im PurposeMiddlewareV2) OnTimeoutPacket(
	ctx sdk.Context,
	sourceClient string,
	destinationClient string,
	sequence uint64,
	payload channeltypesv2.Payload,
	relayer sdk.AccAddress,
) error {
	ctx = types.WithPurpose(ctx, types.PurposeTimeout)
	return im.IBCModuleV2.OnTimeoutPacket(ctx, sourceClient, destinationClient, sequence, payload, relayer)
}
