package keeper

import (
	"context"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

var _ types.MsgServer = MsgServer{}

// MsgServer serves grpc tx requests for the module.
type MsgServer struct {
	keeper Keeper
}

// NewMsgServer returns a new instance of the MsgServer.
func NewMsgServer(keeper Keeper) MsgServer {
	return MsgServer{
		keeper: keeper,
	}
}

// UpdateParams is a governance operation that sets parameters of the module.
func (ms MsgServer) UpdateParams(goCtx context.Context, req *types.MsgUpdateParams) (*types.EmptyResponse, error) {
	return nil, types.ErrNotImplemented
}
