//go:build integrationtests

package upgrade

import (
	"testing"

	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v7/integration-tests"
)

type mint struct{}

func (m *mint) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	client := minttypes.NewQueryClient(chain.ClientContext)
	params, err := client.Params(ctx, &minttypes.QueryParamsRequest{})
	requireT.NoError(err)
	oldMaxInflation, err := params.Params.InflationMax.Float64()
	requireT.NoError(err)
	requireT.InDelta(float64(0.20), oldMaxInflation, 0.01)
	oldInflationRateChange, err := params.Params.InflationRateChange.Float64()
	requireT.NoError(err)
	requireT.InDelta(float64(0.13), oldInflationRateChange, 0.0001)
	requireT.EqualValues(17_900_000, params.Params.BlocksPerYear)

	inflation, err := client.Inflation(ctx, &minttypes.QueryInflationRequest{})
	requireT.NoError(err)
	oldInflation, err := inflation.Inflation.Float64()
	requireT.NoError(err)
	requireT.InDelta(float64(0.10), oldInflation, 0.01)
}

func (m *mint) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	client := minttypes.NewQueryClient(chain.ClientContext)
	params, err := client.Params(ctx, &minttypes.QueryParamsRequest{})
	requireT.NoError(err)
	newMaxInflation, err := params.Params.InflationMax.Float64()
	requireT.NoError(err)
	requireT.InDelta(float64(0.20), newMaxInflation, 0.01)
	newInflationRateChange, err := params.Params.InflationRateChange.Float64()
	requireT.NoError(err)
	requireT.InDelta(float64(0.04), newInflationRateChange, 0.0001)

	inflation, err := client.Inflation(ctx, &minttypes.QueryInflationRequest{})
	requireT.NoError(err)
	inflationFloat, err := inflation.Inflation.Float64()
	requireT.NoError(err)
	requireT.InDelta(float64(0.001), inflationFloat, 0.0001)
	requireT.EqualValues(33_000_000, params.Params.BlocksPerYear)
}
