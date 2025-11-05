package types

import (
	sdkerrors "cosmossdk.io/errors"
)

var (
	// ErrNotImplemented is a placeholder.
	// TODO: This error is a temporary placeholder. Remove it when the functionality is implemented.
	// NOTE: Error status code must start from 2.
	ErrNotImplemented = sdkerrors.Register(ModuleName, 2, "not implemented")

	// ErrInvalidAuthority is returned when the authority is invalid.
	ErrInvalidAuthority = sdkerrors.Register(ModuleName, 3, "invalid authority")

	// ErrInitGenesis is returned when there is an error initializing genesis.
	ErrInitGenesis = sdkerrors.Register(ModuleName, 4, "error initializing genesis")

	// ErrExportGenesis is returned when there is an error exporting genesis.
	ErrExportGenesis = sdkerrors.Register(ModuleName, 5, "error exporting genesis")

	// ErrDistributionScheduleNotFound is returned when a distribution schedule for a module account is not found.
	ErrDistributionScheduleNotFound = sdkerrors.Register(ModuleName, 6, "distribution schedule not found")

	// ErrCannotModifyPaidPeriods is returned when attempting to modify periods that have already been paid.
	ErrCannotModifyPaidPeriods = sdkerrors.Register(ModuleName, 7, "cannot modify already paid periods")

	// ErrInvalidInput is returned when input validation fails.
	ErrInvalidInput = sdkerrors.Register(ModuleName, 9, "invalid input")

	// ErrDistributionNotFound is returned when a specific distribution is not found.
	ErrDistributionNotFound = sdkerrors.Register(ModuleName, 10, "distribution not found")

	// ErrNoPendingDistributions is returned when there are no pending distributions scheduled.
	ErrNoPendingDistributions = sdkerrors.Register(ModuleName, 11, "no pending distributions scheduled")

	// ErrInvalidAllocations is returned when bootstrap allocations are invalid.
	ErrInvalidAllocations = sdkerrors.Register(ModuleName, 12, "invalid bootstrap allocations")

	// ErrMintFailed is returned when minting coins fails.
	ErrMintFailed = sdkerrors.Register(ModuleName, 13, "failed to mint coins")

	// ErrInvalidModuleAccount is returned when a module account name is invalid.
	ErrInvalidModuleAccount = sdkerrors.Register(ModuleName, 14, "invalid module account")

	// ErrTransferFailed is returned when transferring coins fails.
	ErrTransferFailed = sdkerrors.Register(ModuleName, 15, "failed to transfer coins")

	// ErrScheduleCreationFailed is returned when creating distribution schedule fails.
	ErrScheduleCreationFailed = sdkerrors.Register(ModuleName, 16, "failed to create distribution schedule")

	// ErrNoAllocations is returned when no allocations are provided.
	ErrNoAllocations = sdkerrors.Register(ModuleName, 17, "no allocations provided")

	// ErrEmptyModuleAccount is returned when module account name is empty.
	ErrEmptyModuleAccount = sdkerrors.Register(ModuleName, 18, "empty module account name")

	// ErrNegativePercentage is returned when allocation percentage is negative.
	ErrNegativePercentage = sdkerrors.Register(ModuleName, 19, "negative percentage")

	// ErrPercentageExceedsOne is returned when allocation percentage exceeds 1.0.
	ErrPercentageExceedsOne = sdkerrors.Register(ModuleName, 20, "percentage exceeds 1.0")

	// ErrInvalidTotalPercentage is returned when total allocation percentages don't sum to 1.0.
	ErrInvalidTotalPercentage = sdkerrors.Register(ModuleName, 21, "invalid total percentage")

	// ErrNoModuleBalances is returned when no module account balances are provided.
	ErrNoModuleBalances = sdkerrors.Register(ModuleName, 22, "no module account balances provided")

	// ErrParamsGet is returned when getting params fails.
	ErrParamsGet = sdkerrors.Register(ModuleName, 23, "failed to get params")

	// ErrParamsSet is returned when setting params fails.
	ErrParamsSet = sdkerrors.Register(ModuleName, 24, "failed to set params")

	// ErrPendingTimestampAdd is returned when adding pending timestamp fails.
	ErrPendingTimestampAdd = sdkerrors.Register(ModuleName, 25, "failed to add pending timestamp")
)
