package recoverylock

import (
	"fmt"

	"cosmossdk.io/core/address"
	errorsmod "cosmossdk.io/errors"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
)

// MaxDepth is the maximum authz MsgExec nesting for a transaction that
// involves a listed account.
const MaxDepth = 6

// ErrLocked identifies an operation refused by the recovery lock.
var ErrLocked = errorsmod.Register("recoverylock", 1, "recovery lock")

// Lock applies recovery rules to listed wallets during the configured window.
type Lock struct {
	cfg        *Config
	cdc        codec.Codec
	addr       address.Codec
	distr      WithdrawAddrKeeper
	lastHeight func() int64
}

// New returns the lock for cfg, or nil when cfg is nil (disabled
// configuration). The app wires no lock component for a nil Lock.
func New(cfg *Config, cdc codec.Codec, addr address.Codec, distr WithdrawAddrKeeper, lastHeight func() int64) *Lock {
	if cfg == nil {
		return nil
	}
	return &Lock{cfg: cfg, cdc: cdc, addr: addr, distr: distr, lastHeight: lastHeight}
}

// active uses the block header in consensus and the next height in CheckTx.
// The last committed height is needed when CheckTx has no header after restart.
func (l *Lock) active(ctx sdk.Context) bool {
	return l.inWindow(ctx, l.cfg.activationHeight, l.cfg.expiryTime)
}

func (l *Lock) inWindow(ctx sdk.Context, activationHeight, expiryTime int64) bool {
	height := ctx.BlockHeight()
	if ctx.IsCheckTx() {
		if height == 0 && l.lastHeight != nil {
			height = l.lastHeight()
		}
		height++
		if ctx.BlockTime().IsZero() {
			return height >= activationHeight
		}
	}
	return height >= activationHeight && (expiryTime == 0 || ctx.BlockTime().Unix() < expiryTime)
}

// checkMsg checks listed signers and records the deepest message in the tree.
// Scan past MaxDepth so a listed signer below the limit still counts.
func (l *Lock) checkMsg(ctx sdk.Context, msg sdk.Msg, depth int, deepest *int) (bool, error) {
	if depth > *deepest {
		*deepest = depth
	}
	signers, _, err := l.cdc.GetMsgV1Signers(msg)
	if err != nil {
		return false, sdkerrors.ErrInvalidRequest.Wrap(err.Error())
	}
	involved := false
	for _, signer := range signers {
		if !l.cfg.isListed(signer) {
			continue
		}
		involved = true
		if err := l.checkRule(ctx, msg, signer, depth); err != nil {
			return true, ErrLocked.Wrap(err.Error())
		}
	}
	if exec, ok := msg.(*authz.MsgExec); ok {
		inner, err := exec.GetMessages()
		if err != nil {
			return involved, sdkerrors.ErrInvalidRequest.Wrap(err.Error())
		}
		for _, nested := range inner {
			listed, err := l.checkMsg(ctx, nested, depth+1, deepest)
			involved = involved || listed
			if err != nil {
				return involved, err
			}
		}
	}
	return involved, nil
}

func lockedf(format string, args ...any) error {
	return ErrLocked.Wrap(fmt.Sprintf(format, args...))
}
