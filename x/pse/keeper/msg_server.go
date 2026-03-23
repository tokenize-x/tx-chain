package keeper

import (
	"context"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
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
	err := ms.keeper.UpdateClearingAccountMappings(
		goCtx,
		req.Authority,
		req.Mappings,
	)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}

// UpdateDistributionSchedule is a governance operation that updates the distribution schedule.
func (ms MsgServer) UpdateDistributionSchedule(
	goCtx context.Context,
	req *types.MsgUpdateDistributionSchedule,
) (*types.EmptyResponse, error) {
	err := ms.keeper.UpdateDistributionSchedule(
		goCtx,
		req.Authority,
		req.Schedule,
	)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}

// UpdateMinDistributionGap is a governance operation that updates the minimum distribution gap.
func (ms MsgServer) UpdateMinDistributionGap(
	goCtx context.Context,
	req *types.MsgUpdateMinDistributionGap,
) (*types.EmptyResponse, error) {
	err := ms.keeper.UpdateMinDistributionGap(
		goCtx,
		req.Authority,
		req.MinDistributionGapSeconds,
	)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}

// UpdateDistributionBatchSize is a governance operation that updates the distribution batch size.
func (ms MsgServer) UpdateDistributionBatchSize(
	goCtx context.Context,
	req *types.MsgUpdateDistributionBatchSize,
) (*types.EmptyResponse, error) {
	err := ms.keeper.UpdateDistributionBatchSize(
		goCtx,
		req.Authority,
		req.DistributionBatchSize,
	)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}

// DisableDistributions is a governance operation that disables distributions.
func (ms MsgServer) DisableDistributions(
	goCtx context.Context,
	req *types.MsgDisableDistributions,
) (*types.EmptyResponse, error) {
	err := ms.keeper.DisableDistributions(goCtx, req.Authority)
	if err != nil {
		return nil, err
	}
	return &types.EmptyResponse{}, nil
}
