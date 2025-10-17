package keeper

import (
	sdkstore "cosmossdk.io/core/store"
	"github.com/cosmos/cosmos-sdk/codec"
)

// Keeper is a fee model keeper.
type Keeper struct {
	storeService sdkstore.KVStoreService
	cdc          codec.BinaryCodec
	authority    string
}

// NewKeeper returns a new keeper object providing storage options required by fee model.
func NewKeeper(
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
) Keeper {
	return Keeper{
		storeService: storeService,
		cdc:          cdc,
		authority:    authority,
	}
}
