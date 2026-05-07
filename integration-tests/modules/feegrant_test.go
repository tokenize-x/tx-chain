//go:build integrationtests

package modules

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/x/feegrant"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v8/integration-tests"
	"github.com/tokenize-x/tx-chain/v8/pkg/client"
	"github.com/tokenize-x/tx-chain/v8/testutil/integration"
)

func TestFeeGrant(t *testing.T) {
	t.Parallel()
	requireT := require.New(t)
	ctx, chain := integrationtests.NewTXChainTestingContext(t)

	granter := chain.GenAccount()
	grantee := chain.GenAccount()
	recipient := chain.GenAccount()
	feegrantClient := feegrant.NewQueryClient(chain.ClientContext)

	chain.FundAccountsWithOptions(ctx, t, []integration.AccWithBalancesOptions{
		{
			Acc: granter,
			Options: integration.BalancesOptions{
				Messages: []sdk.Msg{
					&banktypes.MsgSend{},
					&banktypes.MsgSend{},
					&feegrant.MsgRevokeAllowance{},
				},
				Amount: sdkmath.NewInt(500_000),
			},
		}, {
			Acc: grantee,
			Options: integration.BalancesOptions{
				Amount: sdkmath.NewInt(1),
			},
		},
	})

	basicAllowance, err := codectypes.NewAnyWithValue(&feegrant.BasicAllowance{
		SpendLimit: nil, // empty means no limit
		Expiration: nil, // empty means no limit
	})
	requireT.NoError(err)

	grantMsg := &feegrant.MsgGrantAllowance{
		Granter:   granter.String(),
		Grantee:   grantee.String(),
		Allowance: basicAllowance,
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(granter),
		chain.TxFactoryAuto(),
		grantMsg,
	)
	requireT.NoError(err)

	latestBlock, err := chain.LatestBlockHeader(ctx)
	requireT.NoError(err)

	allowanceExpiration := latestBlock.Time.Add(10 * time.Second)
	expiringAllowance, err := codectypes.NewAnyWithValue(&feegrant.BasicAllowance{
		SpendLimit: nil, // empty means no limit
		Expiration: lo.ToPtr(allowanceExpiration),
	})
	requireT.NoError(err)

	expiringGrantMsg := &feegrant.MsgGrantAllowance{
		Granter:   granter.String(),
		Grantee:   recipient.String(),
		Allowance: expiringAllowance,
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(granter),
		chain.TxFactoryAuto(),
		expiringGrantMsg,
	)
	requireT.NoError(err)

	allowancesRes, err := feegrantClient.AllowancesByGranter(ctx, &feegrant.QueryAllowancesByGranterRequest{
		Granter: granter.String(),
	})
	requireT.NoError(err)
	requireT.Len(allowancesRes.Allowances, 2)

	// await until chain time passes the allowance expiration
	requireT.NoError(chain.AwaitState(ctx, func(ctx context.Context) error {
		header, err := chain.LatestBlockHeader(ctx)
		if err != nil {
			return err
		}
		if !header.Time.After(allowanceExpiration) {
			return errors.Errorf(
				"chain time %s has not passed expiration %s yet",
				header.Time, allowanceExpiration,
			)
		}
		return nil
	}))

	pruneAllowancesMsg := &feegrant.MsgPruneAllowances{
		Pruner: granter.String(),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(granter),
		chain.TxFactory().WithGas(200_000),
		pruneAllowancesMsg,
	)
	requireT.NoError(err)

	allowancesRes, err = feegrantClient.AllowancesByGranter(ctx, &feegrant.QueryAllowancesByGranterRequest{
		Granter: granter.String(),
	})
	requireT.NoError(err)
	requireT.Len(allowancesRes.Allowances, 1)

	sendMsg := &banktypes.MsgSend{
		FromAddress: grantee.String(),
		ToAddress:   recipient.String(),
		Amount:      sdk.NewCoins(chain.NewCoin(sdkmath.NewInt(1))),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(grantee).WithFeeGranterAddress(granter),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(sendMsg)),
		sendMsg,
	)
	requireT.NoError(err)

	revokeMsg := &feegrant.MsgRevokeAllowance{
		Granter: granter.String(),
		Grantee: grantee.String(),
	}

	res, err := client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(granter),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(revokeMsg)),
		revokeMsg,
	)
	requireT.NoError(err)
	requireT.EqualValues(chain.GasLimitByMsgs(revokeMsg), res.GasUsed)

	sendMsg = &banktypes.MsgSend{
		FromAddress: grantee.String(),
		ToAddress:   recipient.String(),
		Amount:      sdk.NewCoins(chain.NewCoin(sdkmath.NewInt(1))),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(grantee).WithFeeGranterAddress(granter),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(sendMsg)),
		sendMsg,
	)
	requireT.Error(err)
	requireT.True(cosmoserrors.ErrNotFound.Is(err))
}
