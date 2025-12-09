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
	params, err := qs.keeper.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryParamsResponse{
		Params: params,
	}, nil
}

// Score returns the current total score for a delegator.
// This includes both the accumulated score snapshot and any uncalculated score
// from active delegations since the last distribution.
func (qs QueryService) Score(ctx context.Context, req *types.QueryScoreRequest) (*types.QueryScoreResponse, error) {
	// Convert address string to account address
	delAddr, err := qs.keeper.addressCodec.StringToBytes(req.Address)
	if err != nil {
		return nil, err
	}

	// Calculate current score
	score, err := qs.keeper.CalculateDelegatorScore(ctx, delAddr)
	if err != nil {
		return nil, err
	}

	return &types.QueryScoreResponse{
		Score: score,
	}, nil
}

// ScheduledDistributions returns all future allocation schedules.
// Past scheduled distributions are automatically removed after processing,
// so all scheduled distributions in storage are future scheduled distributions.
func (qs QueryService) ScheduledDistributions(
	ctx context.Context, req *types.QueryScheduledDistributionsRequest,
) (*types.QueryScheduledDistributionsResponse, error) {
	scheduledDistributions, err := qs.keeper.GetDistributionSchedule(ctx)
	if err != nil {
		return nil, err
	}

	disabled, err := qs.keeper.DistributionDisabled.Get(ctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryScheduledDistributionsResponse{
		ScheduledDistributions: scheduledDistributions,
		DisableDistributions:   disabled,
	}, nil
}

// ClearingAccountBalances returns the current balances of all PSE clearing accounts.
func (qs QueryService) ClearingAccountBalances(
	ctx context.Context,
	req *types.QueryClearingAccountBalancesRequest,
) (*types.QueryClearingAccountBalancesResponse, error) {
	balances, err := qs.keeper.GetClearingAccountBalances(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryClearingAccountBalancesResponse{
		Balances: balances,
	}, nil
}
