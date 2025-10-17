package types

import (
	sdkerrors "cosmossdk.io/errors"
)

// TODO: This error is a temporary placeholder. Change it to a more specific error.
// NOTE: Error status code must start from 2.
var ErrNotImplemented = sdkerrors.Register(ModuleName, 2, "not implemented")
