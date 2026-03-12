package v6paramspatch_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	v6paramspatch "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6-params-patch"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
)

func TestMigrateMintParams(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).
		WithChainID(string(constant.ChainIDDev)).
		WithBlockTime(time.Now())

	// Run the migration
	err := v6paramspatch.MigrateMintParams(ctx, testApp.MintKeeper)
	requireT.NoError(err)

	// Verify params
	params, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.True(params.InflationRateChange.Equal(math.LegacyMustNewDecFromStr("0.005")), "InflationRateChange should be 0.500%%")
	requireT.True(params.InflationMax.Equal(math.LegacyMustNewDecFromStr("0.02")), "InflationMax should be 2.000%%")
	requireT.Equal(uint64(33_000_000), params.BlocksPerYear, "BlocksPerYear should be 33M")

	// Verify minter
	minter, err := testApp.MintKeeper.Minter.Get(ctx)
	requireT.NoError(err)
	requireT.True(minter.Inflation.Equal(math.LegacyMustNewDecFromStr("0.00075")), "Inflation should be 0.075%%")
}
