package types

import (
	sdkerrors "cosmossdk.io/errors"
)

// ErrNotImplemented is a placeholder.
// TODO: This error is a temporary placeholder. Remove it when the functionality is implemented.
// NOTE: Error status code must start from 2.
var ErrNotImplemented = sdkerrors.Register(ModuleName, 2, "not implemented")

// ErrInvalidAuthority is returned when the authority is invalid.
var ErrInvalidAuthority = sdkerrors.Register(ModuleName, 1, "invalid authority")
