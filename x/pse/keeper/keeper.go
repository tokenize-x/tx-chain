package keeper

import (
	"cosmossdk.io/collections"
	sdkstore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// Keeper of the module.
type Keeper struct {
	storeService sdkstore.KVStoreService
	cdc          codec.BinaryCodec
	authority    string

	// collections
	Schema                collections.Schema
	DelegationTimeEntries collections.Map[collections.Pair[sdk.ValAddress, sdk.AccAddress], types.DelegationTimeEntry]
	AccountScore          collections.Map[sdk.AccAddress, sdkmath.Int]
}

// NewKeeper returns a new keeper object providing storage options required by the module.
func NewKeeper(
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		storeService: storeService,
		cdc:          cdc,
		authority:    authority,

		DelegationTimeEntries: collections.NewMap(
			sb,
			types.StakingTimeKey,
			"delegation_time_entries",
			collections.PairKeyCodec(sdk.ValAddressKey, sdk.AccAddressKey),
			codec.CollValue[types.DelegationTimeEntry](cdc),
		),
		AccountScore: collections.NewMap(
			sb,
			types.AccountScoreKey,
			"account_score",
			sdk.AccAddressKey,
			sdk.IntValue,
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}
