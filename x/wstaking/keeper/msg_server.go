package keeper

import (
	"context"

	sdkerrors "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	wstakingtypes "github.com/tokenize-x/tx-chain/v8/x/wstaking/types"
)

// MsgServer is wrapper staking customParamsKeeper message server.
type MsgServer struct {
	stakingtypes.MsgServer

	customParamsKeeper wstakingtypes.CustomParamsKeeper
	stakingKeeper      wstakingtypes.StakingKeeper
}

// NewMsgServerImpl returns an implementation of the staking wrapped MsgServer.
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

// CreateValidator defines wrapped method for creating a new validator.
func (s MsgServer) CreateValidator(
	goCtx context.Context, msg *stakingtypes.MsgCreateValidator,
) (*stakingtypes.MsgCreateValidatorResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	params, err := s.customParamsKeeper.GetStakingParams(ctx)
	if err != nil {
		return nil, err
	}
	expectedMinSelfDelegation := params.MinSelfDelegation
	if expectedMinSelfDelegation.GT(msg.MinSelfDelegation) {
		return nil, sdkerrors.Wrapf(
			stakingtypes.ErrSelfDelegationBelowMinimum,
			"min self delegation must be greater than or equal to global min self delegation: %s",
			msg.MinSelfDelegation,
		)
	}

	res, err := s.MsgServer.CreateValidator(goCtx, msg)
	if err != nil {
		return nil, err
	}
	if err := s.checkMaxVotingPower(goCtx, msg.ValidatorAddress); err != nil {
		return nil, err
	}

	return res, nil
}

// Delegate defines wrapped method for delegating to a validator.
func (s MsgServer) Delegate(
	goCtx context.Context, msg *stakingtypes.MsgDelegate,
) (*stakingtypes.MsgDelegateResponse, error) {
	res, err := s.MsgServer.Delegate(goCtx, msg)
	if err != nil {
		return nil, err
	}
	if err := s.checkMaxVotingPower(goCtx, msg.ValidatorAddress); err != nil {
		return nil, err
	}

	return res, nil
}

// BeginRedelegate defines wrapped method for redelegating to a validator.
func (s MsgServer) BeginRedelegate(
	goCtx context.Context, msg *stakingtypes.MsgBeginRedelegate,
) (*stakingtypes.MsgBeginRedelegateResponse, error) {
	res, err := s.MsgServer.BeginRedelegate(goCtx, msg)
	if err != nil {
		return nil, err
	}
	if err := s.checkMaxVotingPower(goCtx, msg.ValidatorDstAddress); err != nil {
		return nil, err
	}

	return res, nil
}

// CancelUnbondingDelegation defines wrapped method for returning unbonding tokens to a validator.
func (s MsgServer) CancelUnbondingDelegation(
	goCtx context.Context, msg *stakingtypes.MsgCancelUnbondingDelegation,
) (*stakingtypes.MsgCancelUnbondingDelegationResponse, error) {
	res, err := s.MsgServer.CancelUnbondingDelegation(goCtx, msg)
	if err != nil {
		return nil, err
	}
	if err := s.checkMaxVotingPower(goCtx, msg.ValidatorAddress); err != nil {
		return nil, err
	}

	return res, nil
}

// checkMaxVotingPower runs after the wrapped handler has updated the state. It fails if the validator
// now exceeds max_voting_power, so the message and its state changes are reverted.
// A validator that is not bonded is measured as if it joined the bonded set.
func (s MsgServer) checkMaxVotingPower(goCtx context.Context, validatorAddress string) error {
	params, err := s.customParamsKeeper.GetStakingParams(sdk.UnwrapSDKContext(goCtx))
	if err != nil {
		return err
	}
	if params.MaxVotingPower.GTE(sdkmath.LegacyOneDec()) {
		return nil
	}

	valAddr, err := sdk.ValAddressFromBech32(validatorAddress)
	if err != nil {
		return err
	}
	validator, err := s.stakingKeeper.GetValidator(goCtx, valAddr)
	if err != nil {
		return err
	}
	totalBonded, err := s.stakingKeeper.TotalBondedTokens(goCtx)
	if err != nil {
		return err
	}
	// nothing is bonded before the first validator set exists (gentxs at genesis), so there is no voting power to cap
	if totalBonded.IsZero() {
		return nil
	}
	if !validator.IsBonded() {
		totalBonded = totalBonded.Add(validator.Tokens)
	}

	votingPower := sdkmath.LegacyNewDecFromInt(validator.Tokens).QuoInt(totalBonded)
	if votingPower.GT(params.MaxVotingPower) {
		return sdkerrors.Wrapf(
			cosmoserrors.ErrInvalidRequest,
			"validator voting power %s exceeds max voting power %s",
			votingPower, params.MaxVotingPower,
		)
	}

	return nil
}
