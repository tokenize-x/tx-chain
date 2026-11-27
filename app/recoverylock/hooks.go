package recoverylock

import (
	"bytes"
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// WithdrawAddrKeeper is the part of the distribution keeper the hooks need.
type WithdrawAddrKeeper interface {
	GetDelegatorWithdrawAddr(ctx context.Context, delAddr sdk.AccAddress) (sdk.AccAddress, error)
	DeleteDelegatorWithdrawAddr(ctx context.Context, delAddr, withdrawAddr sdk.AccAddress) error
}

// StakingHooks must be registered FIRST in the app's multi staking hooks,
// before the distribution hooks. Rewards are paid to the withdraw address
// whenever delegation shares change, including paths that no transaction
// controls (for example SlashRedelegation calls Unbond). For a listed
// delegator whose withdraw address is neither the wallet nor the recovery
// address, these hooks reset it to the wallet first, so the payout lands in
// the wallet where the coin rule applies. The same applies to a listed
// validator operator when its validator is deleted (commission payout). Only
// while the lock is active; otherwise they do nothing for listed wallets.
type StakingHooks struct {
	L *Lock
}

var _ stakingtypes.StakingHooks = StakingHooks{}

// BeforeDelegationSharesModified resets an unsafe withdraw address before the
// distribution hook withdraws rewards.
func (h StakingHooks) BeforeDelegationSharesModified(ctx context.Context, del sdk.AccAddress, _ sdk.ValAddress) error {
	return h.reset(ctx, del)
}

// BeforeDelegationRemoved does the same in case a later hook withdraws there.
func (h StakingHooks) BeforeDelegationRemoved(ctx context.Context, del sdk.AccAddress, _ sdk.ValAddress) error {
	return h.reset(ctx, del)
}

// AfterValidatorCreated leaves the withdraw address unchanged.
func (StakingHooks) AfterValidatorCreated(context.Context, sdk.ValAddress) error { return nil }

// BeforeValidatorModified leaves the withdraw address unchanged.
func (StakingHooks) BeforeValidatorModified(context.Context, sdk.ValAddress) error { return nil }

// AfterValidatorRemoved resets an unsafe withdraw address for a listed
// operator before the distribution hook pays the validator commission.
func (h StakingHooks) AfterValidatorRemoved(ctx context.Context, _ sdk.ConsAddress, val sdk.ValAddress) error {
	return h.reset(ctx, sdk.AccAddress(val))
}

// AfterValidatorBonded leaves the withdraw address unchanged.
func (StakingHooks) AfterValidatorBonded(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}

// AfterValidatorBeginUnbonding leaves the withdraw address unchanged.
func (StakingHooks) AfterValidatorBeginUnbonding(context.Context, sdk.ConsAddress, sdk.ValAddress) error {
	return nil
}

// BeforeDelegationCreated leaves the withdraw address unchanged.
func (StakingHooks) BeforeDelegationCreated(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}

// AfterDelegationModified leaves the withdraw address unchanged.
func (StakingHooks) AfterDelegationModified(context.Context, sdk.AccAddress, sdk.ValAddress) error {
	return nil
}

// BeforeValidatorSlashed leaves the withdraw address unchanged.
func (StakingHooks) BeforeValidatorSlashed(context.Context, sdk.ValAddress, math.LegacyDec) error {
	return nil
}

// AfterUnbondingInitiated leaves the withdraw address unchanged.
func (StakingHooks) AfterUnbondingInitiated(context.Context, uint64) error { return nil }

func (h StakingHooks) reset(ctx context.Context, del sdk.AccAddress) error {
	sctx := sdk.UnwrapSDKContext(ctx)
	if !h.L.active(sctx) || !h.L.cfg.isListed(del) {
		return nil
	}
	w, err := h.L.distr.GetDelegatorWithdrawAddr(ctx, del)
	if err != nil {
		return err
	}
	if bytes.Equal(w, del) || h.L.cfg.isRecovery(del, w) {
		return nil
	}
	return h.L.distr.DeleteDelegatorWithdrawAddr(ctx, del, w)
}
