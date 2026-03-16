package keeper

import (
	"context"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	wstakingtypes "github.com/tokenize-x/tx-chain/v7/x/wstaking/types"
)

// MsgServer is the wrapped staking module message server.
type MsgServer struct {
	stakingtypes.MsgServer
	customParamsKeeper wstakingtypes.CustomParamsKeeper
	stakingKeeper      wstakingtypes.StakingKeeper
}

// NewMsgServerImpl returns an implementation of the staking MsgServer with voting power cap enforcement.
func NewMsgServerImpl(
	stakingMsgSrv stakingtypes.MsgServer,
	customParamsKeeper wstakingtypes.CustomParamsKeeper,
	stakingKeeper wstakingtypes.StakingKeeper,
) stakingtypes.MsgServer {
	return MsgServer{
		MsgServer:          stakingMsgSrv,
		customParamsKeeper: customParamsKeeper,
		stakingKeeper:      stakingKeeper,
	}
}

func formatVPPercent(vp sdkmath.LegacyDec) string {
	// Multiply by 100 for percentage, truncate to 2 decimal places.
	pct := vp.MulInt64(10000).TruncateInt()
	whole := pct.Quo(sdkmath.NewInt(100))
	frac := pct.Mod(sdkmath.NewInt(100)).Abs()
	if frac.IsZero() {
		return whole.String()
	}
	return fmt.Sprintf("%s.%02d", whole, frac.Int64())
}

func (s MsgServer) checkVotingPowerCap(
	ctx sdk.Context,
	additionalTokens sdkmath.LegacyDec,
	totalBondedDelta sdkmath.LegacyDec,
	action string,
	existingTokens sdkmath.LegacyDec,
) error {
	params, err := s.customParamsKeeper.GetStakingParams(ctx)
	if err != nil {
		return err
	}

	maxVP := params.MaxVotingPower
	if maxVP.IsNil() || !maxVP.IsPositive() || maxVP.GTE(sdkmath.LegacyOneDec()) {
		return nil
	}

	totalBonded, err := s.stakingKeeper.TotalBondedTokens(ctx)
	if err != nil {
		return err
	}
	totalBondedDec := totalBonded.ToLegacyDec()

	projectedTotal := totalBondedDec.Add(totalBondedDelta)
	if !projectedTotal.IsPositive() {
		return nil
	}

	projectedValidator := existingTokens.Add(additionalTokens)
	projectedVP := projectedValidator.Quo(projectedTotal)

	if projectedVP.GT(maxVP) {
		return errorsmod.Wrapf(
			cosmoserrors.ErrInvalidRequest,
			"%s would result in voting power of %s%%, which exceeds the max allowed %s%%",
			action,
			formatVPPercent(projectedVP),
			formatVPPercent(maxVP),
		)
	}

	return nil
}

// CreateValidator enforces min self delegation and voting power cap before creating a validator.
func (s MsgServer) CreateValidator(
	goCtx context.Context, msg *stakingtypes.MsgCreateValidator,
) (*stakingtypes.MsgCreateValidatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params, err := s.customParamsKeeper.GetStakingParams(ctx)
	if err != nil {
		return nil, err
	}
	if params.MinSelfDelegation.GT(msg.MinSelfDelegation) {
		return nil, errorsmod.Wrapf(
			stakingtypes.ErrSelfDelegationBelowMinimum,
			"min self delegation must be greater than or equal to global min self delegation: %s",
			msg.MinSelfDelegation,
		)
	}

	amount := msg.Value.Amount.ToLegacyDec()
	err = s.checkVotingPowerCap(ctx, amount, amount, "creating validator", sdkmath.LegacyZeroDec())
	if err != nil {
		return nil, err
	}

	return s.MsgServer.CreateValidator(goCtx, msg)
}

// Delegate enforces the voting power cap before delegating to a validator.
func (s MsgServer) Delegate(
	goCtx context.Context, msg *stakingtypes.MsgDelegate,
) (*stakingtypes.MsgDelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}
	val, err := s.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return nil, err
	}

	amount := msg.Amount.Amount.ToLegacyDec()
	err = s.checkVotingPowerCap(ctx, amount, amount, "delegation", val.GetTokens().ToLegacyDec())
	if err != nil {
		return nil, err
	}

	return s.MsgServer.Delegate(goCtx, msg)
}

// BeginRedelegate enforces the voting power cap on the destination validator before redelegating.
func (s MsgServer) BeginRedelegate(
	goCtx context.Context, msg *stakingtypes.MsgBeginRedelegate,
) (*stakingtypes.MsgBeginRedelegateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if msg.ValidatorSrcAddress == msg.ValidatorDstAddress {
		return s.MsgServer.BeginRedelegate(goCtx, msg)
	}

	dstValAddr, err := sdk.ValAddressFromBech32(msg.ValidatorDstAddress)
	if err != nil {
		return nil, err
	}
	dstVal, err := s.stakingKeeper.GetValidator(ctx, dstValAddr)
	if err != nil {
		return nil, err
	}

	amount := msg.Amount.Amount.ToLegacyDec()
	err = s.checkVotingPowerCap(ctx, amount, sdkmath.LegacyZeroDec(), "redelegation", dstVal.GetTokens().ToLegacyDec())
	if err != nil {
		return nil, err
	}

	return s.MsgServer.BeginRedelegate(goCtx, msg)
}

// CancelUnbondingDelegation enforces the voting power cap before canceling an unbonding delegation.
func (s MsgServer) CancelUnbondingDelegation(
	goCtx context.Context, msg *stakingtypes.MsgCancelUnbondingDelegation,
) (*stakingtypes.MsgCancelUnbondingDelegationResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, err
	}
	val, err := s.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return nil, err
	}

	amount := msg.Amount.Amount.ToLegacyDec()
	delta := sdkmath.LegacyZeroDec()
	if val.IsBonded() {
		delta = amount
	}

	err = s.checkVotingPowerCap(ctx, amount, delta, "cancel unbonding delegation", val.GetTokens().ToLegacyDec())
	if err != nil {
		return nil, err
	}

	return s.MsgServer.CancelUnbondingDelegation(goCtx, msg)
}
