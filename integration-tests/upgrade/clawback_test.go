//go:build integrationtests

package upgrade

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	appupgradev8 "github.com/tokenize-x/tx-chain/v8/app/upgrade/v8"
	integrationtests "github.com/tokenize-x/tx-chain/v8/integration-tests"
	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v8/testutil/integration"
)

// clawbackTest verifies that the v8 upgrade moves the configured balances.
// It funds the devnet source account before the upgrade and checks both balances after.
type clawbackTest struct {
	transfer        appupgradev8.ClawbackTransfer
	fundedAmount    sdkmath.Int
	denom           string
	recipientBefore sdkmath.Int
}

func (c *clawbackTest) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	transfers := appupgradev8.ClawbackTransfers[constant.ChainIDDev]
	requireT.Len(transfers, 1)
	c.transfer = transfers[0]
	c.denom = chain.ChainSettings.Denom

	from, err := sdk.AccAddressFromBech32(c.transfer.From)
	requireT.NoError(err)
	to, err := sdk.AccAddressFromBech32(c.transfer.To)
	requireT.NoError(err)

	c.fundedAmount = sdkmath.NewInt(1_000_000)
	chain.FundAccountWithOptions(ctx, t, from, integration.BalancesOptions{
		Amount: c.fundedAmount,
	})

	bankClient := banktypes.NewQueryClient(chain.ClientContext)

	fromBalance, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: from.String(), Denom: c.denom,
	})
	requireT.NoError(err)
	requireT.True(fromBalance.Balance.Amount.GTE(c.fundedAmount))
	c.fundedAmount = fromBalance.Balance.Amount

	toBalance, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: to.String(), Denom: c.denom,
	})
	requireT.NoError(err)
	c.recipientBefore = toBalance.Balance.Amount
}

func (c *clawbackTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	bankClient := banktypes.NewQueryClient(chain.ClientContext)

	fromBalance, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: c.transfer.From, Denom: c.denom,
	})
	requireT.NoError(err)
	requireT.True(fromBalance.Balance.Amount.IsZero(), "source account must be empty after the upgrade")

	toBalance, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: c.transfer.To, Denom: c.denom,
	})
	requireT.NoError(err)
	requireT.Equal(c.recipientBefore.Add(c.fundedAmount).String(), toBalance.Balance.Amount.String())
}
