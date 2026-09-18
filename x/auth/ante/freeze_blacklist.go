package ante

import (
	sdkerrors "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
)

// BlacklistedSignersDecorator rejects any transaction signed by a frozen account at or above the activation height.
// It is a secondary early reject, not a replacement for the bank BeforeSend hook.
// It only sees tx signers, so it misses authz-wrapped debits, which the bank hook still catches.
type BlacklistedSignersDecorator struct{}

// NewBlacklistedSignersDecorator creates a BlacklistedSignersDecorator.
func NewBlacklistedSignersDecorator() BlacklistedSignersDecorator {
	return BlacklistedSignersDecorator{}
}

// AnteHandle rejects the tx when one of its signers is frozen at the current block height.
func (d BlacklistedSignersDecorator) AnteHandle(
	ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler,
) (sdk.Context, error) {
	sigTx, ok := tx.(authsigning.SigVerifiableTx)
	if !ok {
		return next(ctx, tx, simulate)
	}

	signers, err := sigTx.GetSigners()
	if err != nil {
		return ctx, err
	}

	for _, signer := range signers {
		addr := sdk.AccAddress(signer).String()
		if constant.IsFrozen(ctx.BlockHeight(), addr) {
			return ctx, sdkerrors.Wrapf(cosmoserrors.ErrUnauthorized, "address %s is frozen", addr)
		}
	}

	return next(ctx, tx, simulate)
}
