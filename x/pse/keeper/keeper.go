package keeper

import (
	"cosmossdk.io/collections"
	addresscodec "cosmossdk.io/core/address"
	sdkstore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// Keeper of the module.
type Keeper struct {
	addressCodec    addresscodec.Codec
	authority       string
	cdc             codec.BinaryCodec
	valAddressCodec addresscodec.Codec
	storeService    sdkstore.KVStoreService

	// keepers
	accountKeeper      types.AccountKeeper
	bankKeeper         types.BankKeeper
	distributionKeeper types.DistributionKeeper
	stakingKeeper      types.StakingQuerier

	// collections
	Schema                collections.Schema
	Params                collections.Item[types.Params]
	DelegationTimeEntries collections.Map[collections.Pair[sdk.ValAddress, sdk.AccAddress], types.DelegationTimeEntry]
	AccountScoreSnapshot  collections.Map[sdk.AccAddress, sdkmath.Int]
}

// NewKeeper returns a new keeper object providing storage options required by the module.
func NewKeeper(
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
	stakingKeeper types.StakingQuerier,
	distributionKeeper types.DistributionKeeper,
	accountKeeper types.AccountKeeper,
	bankKeeper types.BankKeeper,
	addressCodec addresscodec.Codec,
	valAddressCodec addresscodec.Codec,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		storeService:    storeService,
		cdc:             cdc,
		authority:       authority,
		addressCodec:    addressCodec,
		valAddressCodec: valAddressCodec,

		stakingKeeper:      stakingKeeper,
		distributionKeeper: distributionKeeper,
		accountKeeper:      accountKeeper,
		bankKeeper:         bankKeeper,

		Params: collections.NewItem(
			sb,
			types.ParamsKey,
			"params",
			codec.CollValue[types.Params](cdc),
		),
		DelegationTimeEntries: collections.NewMap(
			sb,
			types.StakingTimeKey,
			"delegation_time_entries",
			collections.PairKeyCodec(sdk.ValAddressKey, sdk.AccAddressKey),
			codec.CollValue[types.DelegationTimeEntry](cdc),
		),
		AccountScoreSnapshot: collections.NewMap(
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
