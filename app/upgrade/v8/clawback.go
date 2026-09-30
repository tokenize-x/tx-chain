package v8

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	wbankkeeper "github.com/tokenize-x/tx-chain/v8/x/wbank/keeper"
)

// EventTypeClawback is emitted for every transfer that moves or fails to move funds.
const EventTypeClawback = "clawback"

// recipient of the recovered funds on mainnet.
const recipient = "core1z02frlf8lj0v9l755cdjmjsjpfgt5psuxwjqa7zd2e8hgja208nswqel20"

// ClawbackTransfer moves every coin held by From to To.
// The amount is whatever the account holds at the upgrade height.
type ClawbackTransfer struct {
	From string
	To   string
}

// ClawbackTransfers lists the transfers the v8 upgrade executes, keyed by chain ID.
// An address is only decoded on its own chain, so an address is never parsed under a foreign prefix.
// On mainnet these are the accounts frozen at height 83,530,000.
var ClawbackTransfers = map[constant.ChainID][]ClawbackTransfer{
	constant.ChainIDMain: {
		{From: "core1e7y6qwktg7l6ajr8e2eal5j4dnc2jyceftnjce", To: recipient},
		{From: "core12acz3gw3aluu4dhvtz404dqac0mjmv08pjpunl", To: recipient},
	},
	// Testnet runs the same code path on seeded accounts, so the upgrade is rehearsed before mainnet.
	constant.ChainIDTest: {
		{
			From: "testcore1et6wwhk285ynns5gvh55808c03vr95cfr0dg5e",
			To:   "testcore1w555d8xjg497g77jvwkv0req66ck6tn7vu4jw2",
		},
	},
	// Devnet accounts are funded by the upgrade integration test, which asserts the transfer happened.
	constant.ChainIDDev: {
		{
			From: "devcore1zwfawn7wc66ucvmjkzc75j033hpvw933f04alp",
			To:   "devcore14c7zt6cehtehpkzh0gaqz93kwp0yue7ppxxtgk",
		},
	},
}

// ClawbackFrozenFunds moves the full balance of every configured account to its recipient.
// It calls the embedded BaseKeeper, because the wrapper's SendCoins runs the hook that freezes these accounts.
// A failed transfer is rolled back, logged and emitted, and the remaining transfers still run.
func ClawbackFrozenFunds(
	ctx context.Context,
	bankKeeper wbankkeeper.BaseKeeperWrapper,
	transfers []ClawbackTransfer,
) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	logger := sdkCtx.Logger().With("upgrade", Name, "step", "clawback")

	for _, transfer := range transfers {
		// The bank keeper debits denom by denom, so a failed send must be rolled back as a whole.
		cacheCtx, writeCache := sdkCtx.CacheContext()
		amount, err := clawback(cacheCtx, bankKeeper, transfer) //nolint:contextcheck // this is correct context passing
		if err != nil {
			logger.Error("clawback failed",
				"from", transfer.From, "to", transfer.To, "amount", amount, "error", err.Error())
			sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
				EventTypeClawback,
				sdk.NewAttribute("from", transfer.From),
				sdk.NewAttribute("to", transfer.To),
				sdk.NewAttribute("amount", amount),
				sdk.NewAttribute("error", err.Error()),
			))

			continue
		}
		writeCache()

		logger.Info("clawback done", "from", transfer.From, "to", transfer.To, "amount", amount)
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			EventTypeClawback,
			sdk.NewAttribute("from", transfer.From),
			sdk.NewAttribute("to", transfer.To),
			sdk.NewAttribute("amount", amount),
		))
	}
}

// clawback performs one transfer and returns the amount moved.
func clawback(
	ctx context.Context,
	bankKeeper wbankkeeper.BaseKeeperWrapper,
	transfer ClawbackTransfer,
) (string, error) {
	from, err := sdk.AccAddressFromBech32(transfer.From)
	if err != nil {
		return "", err
	}

	to, err := sdk.AccAddressFromBech32(transfer.To)
	if err != nil {
		return "", err
	}

	balances := bankKeeper.GetAllBalances(ctx, from)
	if balances.IsZero() {
		return balances.String(), nil
	}

	if err := bankKeeper.BaseKeeper.SendCoins(ctx, from, to, balances); err != nil {
		return balances.String(), err
	}

	return balances.String(), nil
}
