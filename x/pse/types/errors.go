package types

import (
	sdkerrors "cosmossdk.io/errors"
)

var (
	// ErrInvalidAuthority is returned when the authority is invalid.
	ErrInvalidAuthority = sdkerrors.Register(ModuleName, 2, "invalid authority")

	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = sdkerrors.Register(ModuleName, 3, "invalid input")

	// ErrTransferFailed is returned when transferring coins fails.
	ErrTransferFailed = sdkerrors.Register(ModuleName, 4, "failed to transfer coins")

	// ErrScheduleCreationFailed is returned when creating distribution schedule fails.
	ErrScheduleCreationFailed = sdkerrors.Register(ModuleName, 5, "failed to create distribution schedule")

	// ErrNoModuleBalances is returned when no clearing account balances are provided.
	ErrNoModuleBalances = sdkerrors.Register(ModuleName, 6, "no clearing account balances provided")

	// ErrInvalidParam is returned when a parameter is invalid.
	ErrInvalidParam = sdkerrors.Register(ModuleName, 7, "invalid parameter")

	// ErrOngoingDistribution is returned when a schedule update is attempted during an ongoing distribution.
	ErrOngoingDistribution = sdkerrors.Register(ModuleName, 8, "distribution is currently in progress")

	// ErrInvariantViolation is returned when an internal invariant is violated, disabling the module.
	ErrInvariantViolation = sdkerrors.Register(ModuleName, 9, "invariant violation")
)
