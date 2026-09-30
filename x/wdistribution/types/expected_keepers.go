package types

import (
	"context"

	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
)

// StakingKeeper is the staking keeper used by the wrapped distribution module.
type StakingKeeper interface {
	distrtypes.StakingKeeper
	BondDenom(ctx context.Context) (string, error)
}
