package types

import (
	context "context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// StakingQuerier interface.
type StakingQuerier interface {
	BondDenom(ctx context.Context) (string, error)
	GetDelegation(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (stakingtypes.Delegation, error)
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)

	Delegate(
		ctx context.Context, delAddr sdk.AccAddress, bondAmt sdkmath.Int, tokenSrc stakingtypes.BondStatus,
		validator stakingtypes.Validator, subtractAccount bool,
	) (newShares sdkmath.LegacyDec, err error)

	DelegatorDelegations(
		ctx context.Context,
		req *stakingtypes.QueryDelegatorDelegationsRequest,
	) (*stakingtypes.QueryDelegatorDelegationsResponse, error)
}

// BankKeeper interface.
type BankKeeper interface {
	SendCoinsFromModuleToModule(
		ctx context.Context,
		senderModule string,
		recipientModule string,
		amt sdk.Coins,
	) error
	SendCoinsFromModuleToAccount(
		ctx context.Context,
		senderModule string,
		recipientAddr sdk.AccAddress,
		amt sdk.Coins,
	) error
}

// DistributionKeeper interface.
type DistributionKeeper interface {
	FundCommunityPool(ctx context.Context, amount sdk.Coins, sender sdk.AccAddress) error
}

// AccountKeeper interface.
type AccountKeeper interface {
	GetModuleAddress(moduleName string) sdk.AccAddress
}
