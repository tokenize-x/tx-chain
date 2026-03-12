package v6patch1testnet_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	v6patch1testnet "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6patch1testnet"
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
	err := v6patch1testnet.MigrateMintParams(ctx, testApp.MintKeeper)
	requireT.NoError(err)

	// Verify params
	params, err := testApp.MintKeeper.Params.Get(ctx)
	requireT.NoError(err)
	requireT.True(params.InflationRateChange.Equal(math.LegacyMustNewDecFromStr("0.005")))
	requireT.True(params.InflationMax.Equal(math.LegacyMustNewDecFromStr("0.02")))
	requireT.Equal(uint64(33_000_000), params.BlocksPerYear)

	// Verify minter
	minter, err := testApp.MintKeeper.Minter.Get(ctx)
	requireT.NoError(err)
	requireT.True(minter.Inflation.Equal(math.LegacyMustNewDecFromStr("0.00075")))
}
