package recoverylock

import (
	"context"

	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// SendRestriction returns the coin rule for bank.AppendSendRestriction: coins
// leaving a listed address may only go to its registered recovery address.
// On this chain the app installs it on both bank keepers: the plain keeper
// that the assetft and assetnft keepers use, through which every account
// debit of the wbank wrapper ends (bank sends, fees, module deposits, IBC
// escrow and burns, contract funds, DEX settlement, assetft commission, burn
// and clawback), and the keeper inside the wrapper.
func (l *Lock) SendRestriction() banktypes.SendRestrictionFn {
	return func(ctx context.Context, from, to sdk.AccAddress, _ sdk.Coins) (sdk.AccAddress, error) {
		sctx := sdk.UnwrapSDKContext(ctx)
		if !l.active(sctx) || !l.cfg.isListed(from) || l.cfg.isRecovery(from, to) {
			return to, nil
		}
		return nil, lockedf("coins of %s may only go to the recovery address", from)
	}
}

// Router wraps the message router held by the authz, CosmWasm, group and ICA
// host keepers. BaseApp runs top-level messages on its original router after
// ante applies the rules and fee payer requirement.
//
// While the lock is active, the wrapper refuses every dispatched message that
// has a listed signer at any depth, even one the rules would allow. Dispatch
// has no fee payer: with an authz grant made before activation, a contract or
// any grantee could otherwise run allowed messages as a listed wallet without
// the recovery operator (for example fill all unbonding entries with tiny
// undelegations so the operator's undelegation fails). Listed wallets
// therefore act only in top level messages of transactions that pass the
// Decorator. Messages without a listed signer at any depth pass if their
// signers and wrapped messages can be decoded.
type Router struct {
	Inner baseapp.MessageRouter
	L     *Lock
}

// Handler returns the inner handler wrapped with the lock check.
func (r Router) Handler(msg sdk.Msg) baseapp.MsgServiceHandler { return r.wrap(r.Inner.Handler(msg)) }

// HandlerByTypeURL returns the inner handler wrapped with the lock check.
func (r Router) HandlerByTypeURL(typeURL string) baseapp.MsgServiceHandler {
	return r.wrap(r.Inner.HandlerByTypeURL(typeURL))
}

func (r Router) wrap(h baseapp.MsgServiceHandler) baseapp.MsgServiceHandler {
	if h == nil {
		return nil
	}
	return func(ctx sdk.Context, msg sdk.Msg) (*sdk.Result, error) {
		if r.L.active(ctx) {
			var deepest int
			involved, err := r.L.checkMsg(ctx, msg, 0, &deepest)
			if err != nil {
				return nil, err
			}
			if involved {
				return nil, lockedf("listed accounts may act only in transactions paid by a fee payer")
			}
		}
		return h(ctx, msg)
	}
}

// Decorator is the ante decorator. Place it before any fee deduction and
// before signature verification. BaseApp discards ante state on error.
type Decorator struct{ L *Lock }

// AnteHandle rejects a transaction that involves a listed address (as signer
// of a message at any depth, as fee payer or as fee granter) unless every
// message with a listed signer is allowed and the fee is paid by the
// configured fee payer set, without a fee granter, and is not zero.
func (d Decorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	l := d.L
	if !l.active(ctx) {
		return next(ctx, tx, simulate)
	}
	msgs := tx.GetMsgs()
	if len(msgs) == 0 {
		// Nothing to check, and FeePayer() would panic on an empty signer
		// list. baseapp refuses transactions without messages before the
		// ante handler runs.
		return next(ctx, tx, simulate)
	}
	involved := false
	var deepest int
	for _, m := range msgs {
		sub, err := l.checkMsg(ctx, m, 0, &deepest)
		if err != nil {
			return ctx, err
		}
		involved = involved || sub
	}
	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		if involved {
			return ctx, lockedf("transaction without fee information")
		}
		return next(ctx, tx, simulate)
	}
	payer, granter := feeTx.FeePayer(), feeTx.FeeGranter()
	if l.cfg.isListed(payer) || l.cfg.isListed(granter) {
		involved = true
	}
	if involved {
		if deepest > MaxDepth {
			return ctx, lockedf("message nesting deeper than %d", MaxDepth)
		}
		if !l.cfg.isFeePayer(payer) {
			return ctx, lockedf("fee must be paid by one of the configured fee payers")
		}
		if len(granter) != 0 {
			return ctx, lockedf("fee grants are not allowed")
		}
		if feeTx.GetFee().IsZero() {
			return ctx, lockedf("fee must not be zero")
		}
	}
	return next(ctx, tx, simulate)
}
