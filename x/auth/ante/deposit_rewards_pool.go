package ante

import (
	"context"

	sdkerrors "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

// StakingKeeper defines the expected staking keeper used to resolve the bond denom.
type StakingKeeper interface {
	BondDenom(ctx context.Context) (string, error)
}

// DepositValidatorRewardsPoolDenomDecorator restricts MsgDepositValidatorRewardsPool
// to the chain's bond denom.
//
// Allowing arbitrary denoms lets an attacker deposit a token with receive-side
// restrictions (e.g. whitelisting) into a validator's reward pool, which can cause
// reward payout fails and freezing all staking operations on that validator.
// The bond denom carries no such features.
type DepositValidatorRewardsPoolDenomDecorator struct {
	stakingKeeper StakingKeeper
}

// NewDepositValidatorRewardsPoolDenomDecorator creates a new DepositValidatorRewardsPoolDenomDecorator.
func NewDepositValidatorRewardsPoolDenomDecorator(
	stakingKeeper StakingKeeper,
) DepositValidatorRewardsPoolDenomDecorator {
	return DepositValidatorRewardsPoolDenomDecorator{stakingKeeper: stakingKeeper}
}

// AnteHandle rejects any MsgDepositValidatorRewardsPool carrying a non-bond denom.
func (d DepositValidatorRewardsPoolDenomDecorator) AnteHandle(
	ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler,
) (sdk.Context, error) {
	deposits := collectDepositValidatorRewardsPoolMsgs(tx.GetMsgs())
	if len(deposits) == 0 {
		return next(ctx, tx, simulate)
	}

	bondDenom, err := d.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return ctx, err
	}

	for _, deposit := range deposits {
		for _, coin := range deposit.Amount {
			if coin.Denom != bondDenom {
				return ctx, sdkerrors.Wrapf(
					cosmoserrors.ErrInvalidRequest,
					"only the bond denom %q can be deposited into validator reward pools, got %q",
					bondDenom, coin.Denom,
				)
			}
		}
	}

	return next(ctx, tx, simulate)
}

// collectDepositValidatorRewardsPoolMsgs returns all MsgDepositValidatorRewardsPool
// messages in the tx, unwrapping authz.MsgExec recursively so the restriction
// cannot be bypassed by nesting the deposit inside an authz exec.
func collectDepositValidatorRewardsPoolMsgs(
	msgs []sdk.Msg,
) []*distributiontypes.MsgDepositValidatorRewardsPool {
	var deposits []*distributiontypes.MsgDepositValidatorRewardsPool
	for _, msg := range msgs {
		switch typedMsg := msg.(type) {
		case *distributiontypes.MsgDepositValidatorRewardsPool:
			deposits = append(deposits, typedMsg)
		case *authz.MsgExec:
			nested, err := typedMsg.GetMessages()
			if err != nil {
				// Malformed nested messages are rejected later by the authz handler.
				continue
			}
			deposits = append(deposits, collectDepositValidatorRewardsPoolMsgs(nested)...)
		}
	}
	return deposits
}
