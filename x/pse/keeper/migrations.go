package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	v2 "github.com/tokenize-x/tx-chain/v7/x/pse/migrations/v2"
)

// Migrator handles in-place store migrations for the PSE module.
type Migrator struct {
	keeper Keeper
}

// NewMigrator returns a new Migrator.
func NewMigrator(keeper Keeper) Migrator {
	return Migrator{keeper: keeper}
}

// Migrate1to2 migrates the store from v1 to v2.
// Key changes:
// - DelegationTimeEntries: Pair[AccAddress, ValAddress] -> Triple[uint64, AccAddress, ValAddress].
// - AccountScoreSnapshot: AccAddress -> Pair[uint64, AccAddress].
func (m Migrator) Migrate1to2(ctx sdk.Context) error {
	return v2.MigrateStore(ctx, m.keeper.storeService, m.keeper.cdc)
}
