package types

import (
	sdkerrors "cosmossdk.io/errors"
)

// codespace is the codespace of the wstaking errors.
const codespace = "wstaking"

// ErrMaxVotingPowerExceeded is returned when a staking message pushes a validator over max voting power.
var ErrMaxVotingPowerExceeded = sdkerrors.Register(codespace, 1, "max voting power exceeded")
