package wibctransfer_test

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	channeltypesv2 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/v2/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/x/wibctransfer"
	"github.com/tokenize-x/tx-chain/v8/x/wibctransfer/types"
)

// TestPurposeMiddlewareV2 checks the purpose each IBC v2 callback passes to the wrapped module.
func TestPurposeMiddlewareV2(t *testing.T) {
	requireT := require.New(t)

	module := &purposeRecorder{}
	middleware := wibctransfer.NewPurposeMiddlewareV2(module)
	ctx := sdk.NewContext(nil, tmproto.Header{}, false, nil)
	payload := channeltypesv2.Payload{}

	requireT.NoError(middleware.OnSendPacket(ctx, "", "", 1, payload, nil))
	requireT.Equal(types.PurposeOut, module.purpose)

	middleware.OnRecvPacket(ctx, "", "", 1, payload, nil)
	requireT.Equal(types.PurposeIn, module.purpose)

	requireT.NoError(middleware.OnAcknowledgementPacket(ctx, "", "", 1, nil, payload, nil))
	requireT.Equal(types.PurposeAck, module.purpose)

	requireT.NoError(middleware.OnTimeoutPacket(ctx, "", "", 1, payload, nil))
	requireT.Equal(types.PurposeTimeout, module.purpose)
}

var _ wibctransfer.IBCModuleV2 = &purposeRecorder{}

// purposeRecorder records the purpose found in the context of the last callback.
type purposeRecorder struct {
	purpose types.Purpose
}

func (r *purposeRecorder) OnSendPacket(
	ctx sdk.Context, _, _ string, _ uint64, _ channeltypesv2.Payload, _ sdk.AccAddress,
) error {
	r.record(ctx)
	return nil
}

func (r *purposeRecorder) OnRecvPacket(
	ctx sdk.Context, _, _ string, _ uint64, _ channeltypesv2.Payload, _ sdk.AccAddress,
) channeltypesv2.RecvPacketResult {
	r.record(ctx)
	return channeltypesv2.RecvPacketResult{}
}

func (r *purposeRecorder) OnTimeoutPacket(
	ctx sdk.Context, _, _ string, _ uint64, _ channeltypesv2.Payload, _ sdk.AccAddress,
) error {
	r.record(ctx)
	return nil
}

func (r *purposeRecorder) OnAcknowledgementPacket(
	ctx sdk.Context, _, _ string, _ uint64, _ []byte, _ channeltypesv2.Payload, _ sdk.AccAddress,
) error {
	r.record(ctx)
	return nil
}

func (r *purposeRecorder) UnmarshalPacketData(_ channeltypesv2.Payload) (any, error) {
	return nil, nil //nolint:nilnil // the recorder has no packet data
}

func (r *purposeRecorder) record(ctx sdk.Context) {
	r.purpose, _ = types.GetPurpose(ctx)
}
