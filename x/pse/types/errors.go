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
)
