package keeper

import (
	"context"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// QueryService serves grpc requests for the module.
type QueryService struct {
	keeper Keeper
}

// NewQueryService creates query service.
func NewQueryService(keeper Keeper) QueryService {
	return QueryService{
		keeper: keeper,
	}
}

// Params returns params of the module.
func (qs QueryService) Params(ctx context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	return nil, types.ErrNotImplemented
}
