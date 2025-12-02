package cli_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	txchainclitestutil "github.com/tokenize-x/tx-chain/v6/testutil/cli"
	"github.com/tokenize-x/tx-chain/v6/testutil/network"
	"github.com/tokenize-x/tx-chain/v6/x/pse/client/cli"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestQueryParams(t *testing.T) {
	requireT := require.New(t)

	testNetwork := network.New(t)

	ctx := testNetwork.Validators[0].ClientCtx

	var resp types.QueryParamsResponse
	txchainclitestutil.ExecQueryCmd(t, ctx, cli.CmdQueryParams(), []string{}, &resp)
	requireT.NotNil(resp.Params)
}

func TestQueryScore_NoScore(t *testing.T) {
	requireT := require.New(t)

	testNetwork := network.New(t)

	ctx := testNetwork.Validators[0].ClientCtx
	// Query score for validator address (which should have zero score)
	addr := testNetwork.Validators[0].Address

	var resp types.QueryScoreResponse
	txchainclitestutil.ExecQueryCmd(t, ctx, cli.CmdQueryScore(), []string{addr.String()}, &resp)
	requireT.NotNil(resp)
	requireT.True(resp.Score.IsZero() || resp.Score.IsPositive(), "score should be zero or positive")
}

func TestQueryScheduledDistributions_Empty(t *testing.T) {
	requireT := require.New(t)

	testNetwork := network.New(t)
	ctx := testNetwork.Validators[0].ClientCtx

	var resp types.QueryScheduledDistributionsResponse
	txchainclitestutil.ExecQueryCmd(t, ctx, cli.CmdQueryScheduledDistributions(), []string{}, &resp)
	requireT.NotNil(resp)
	// The response should be a valid slice (could be empty or have scheduled distributions)
	requireT.NotNil(resp.ScheduledDistributions)
}

func TestQueryClearingAccountBalances(t *testing.T) {
	requireT := require.New(t)

	testNetwork := network.New(t)
	ctx := testNetwork.Validators[0].ClientCtx

	var resp types.QueryClearingAccountBalancesResponse
	txchainclitestutil.ExecQueryCmd(t, ctx, cli.CmdQueryClearingAccountBalances(), []string{}, &resp)
	requireT.NotNil(resp)
	requireT.NotNil(resp.Balances)

	// Should return all clearing accounts
	requireT.Len(resp.Balances, 6, "should return all 6 clearing accounts")

	// Verify all clearing accounts are present
	expectedAccounts := types.GetAllClearingAccounts()
	accountsFound := make(map[string]bool)

	for _, balance := range resp.Balances {
		requireT.NotEmpty(balance.ClearingAccount, "clearing account name should not be empty")
		requireT.NotNil(balance.Balance, "balance should not be nil")
		requireT.True(balance.Balance.GTE(sdkmath.ZeroInt()), "balance should be >= 0")
		accountsFound[balance.ClearingAccount] = true
	}

	// Verify all expected accounts are present
	for _, expectedAccount := range expectedAccounts {
		requireT.True(accountsFound[expectedAccount], "expected clearing account %s not found", expectedAccount)
	}
}
