package keeper

import (
	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// SetDelegationTimeEntry saves DelegationTimeEntry into storages.
func (k Keeper) SetDelegationTimeEntry(
	ctx sdk.Context,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
	entry types.DelegationTimeEntry,
) error {
	key := collections.Join(valAddr, delAddr)
	return k.DelegationTimeEntries.Set(ctx, key, entry)
}

// GetDelegationTimeEntry retrieves DelegationTimeEntry from storages.
func (k Keeper) GetDelegationTimeEntry(
	ctx sdk.Context,
	valAddr sdk.ValAddress,
	delAddr sdk.AccAddress,
) (types.DelegationTimeEntry, error) {
	key := collections.Join(valAddr, delAddr)
	return k.DelegationTimeEntries.Get(ctx, key)
}
