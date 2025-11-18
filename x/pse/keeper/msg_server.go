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

// UpdateExcludedAddresses is a governance operation that updates excluded addresses.
func (ms MsgServer) UpdateExcludedAddresses(
	goCtx context.Context,
	req *types.MsgUpdateExcludedAddresses,
) (*types.EmptyResponse, error) {
	err := ms.keeper.UpdateExcludedAddresses(
		goCtx,
		req.Authority,
		req.AddressesToAdd,
		req.AddressesToRemove,
	)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}

// UpdateClearingAccountMappings is a governance operation that updates clearing account mappings.
func (ms MsgServer) UpdateClearingAccountMappings(
	goCtx context.Context,
	req *types.MsgUpdateClearingAccountMappings,
) (*types.EmptyResponse, error) {
	err := ms.keeper.UpdateClearingMappings(
		goCtx,
		req.Authority,
		req.Mappings,
	)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}

// UpdateAllocationSchedule is a governance operation that updates the allocation schedule.
func (ms MsgServer) UpdateAllocationSchedule(
	goCtx context.Context,
	req *types.MsgUpdateAllocationSchedule,
) (*types.EmptyResponse, error) {
	err := ms.keeper.UpdateAllocationSchedule(
		goCtx,
		req.Authority,
		req.Schedule,
	)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}
