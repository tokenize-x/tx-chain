package cli_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/assert"

	txchainclitestutil "github.com/tokenize-x/tx-chain/v7/testutil/cli"
	"github.com/tokenize-x/tx-chain/v7/testutil/network"
	"github.com/tokenize-x/tx-chain/v7/x/feemodel/client/cli"
	"github.com/tokenize-x/tx-chain/v7/x/feemodel/types"
)

func TestMinGasPrice(t *testing.T) {
	testNetwork := network.New(t)

	ctx := testNetwork.Validators[0].ClientCtx
	var resp sdk.DecCoin
	txchainclitestutil.ExecQueryCmd(t, ctx, cli.GetQueryCmd(), []string{"min-gas-price"}, &resp)

	assert.Equal(t, testNetwork.Config.BondDenom, resp.Denom)
	assert.True(t, resp.Amount.GT(sdkmath.LegacyZeroDec()))
}

func TestRecommendedGasPrice(t *testing.T) {
	testNetwork := network.New(t)

	ctx := testNetwork.Validators[0].ClientCtx
	cmd := cli.GetQueryCmd()

	var resp types.QueryRecommendedGasPriceResponse
	txchainclitestutil.ExecQueryCmd(t, ctx, cmd, []string{"recommended-gas-price", "--after", "10"}, &resp)

	assert.Greater(t, resp.Low.Amount.MustFloat64(), sdkmath.LegacyZeroDec().MustFloat64())
	assert.Greater(t, resp.Med.Amount.MustFloat64(), sdkmath.LegacyZeroDec().MustFloat64())
	assert.Greater(t, resp.High.Amount.MustFloat64(), sdkmath.LegacyZeroDec().MustFloat64())
}
