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
	storeService sdkstore.KVStoreService
	authority    string

	// codec
	cdc             codec.BinaryCodec
	addressCodec    addresscodec.Codec
	valAddressCodec addresscodec.Codec

	// keepers
	accountKeeper      types.AccountKeeper
	bankKeeper         types.BankKeeper
	distributionKeeper types.DistributionKeeper
	stakingKeeper      types.StakingQuerier

	// collections
	Schema                collections.Schema
	Params                collections.Item[types.Params]
	DelegationTimeEntries collections.Map[collections.Pair[sdk.AccAddress, sdk.ValAddress], types.DelegationTimeEntry]
	AccountScoreSnapshot  collections.Map[sdk.AccAddress, sdkmath.Int]
	AllocationSchedule    collections.Map[uint64, types.ScheduledDistribution] // Map: timestamp -> ScheduledDistribution
}

// NewKeeper returns a new keeper object providing storage options required by the module.
func NewKeeper(
	storeService sdkstore.KVStoreService,
	cdc codec.BinaryCodec,
	authority string,
	accountKeeper types.AccountKeeper,
	bankKeeper types.BankKeeper,
	distributionKeeper types.DistributionKeeper,
	stakingKeeper types.StakingQuerier,
	addressCodec addresscodec.Codec,
	valAddressCodec addresscodec.Codec,

) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		storeService:       storeService,
		cdc:                cdc,
		addressCodec:       addressCodec,
		valAddressCodec:    valAddressCodec,
		authority:          authority,
		accountKeeper:      accountKeeper,
		bankKeeper:         bankKeeper,
		distributionKeeper: distributionKeeper,
		stakingKeeper:      stakingKeeper,

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
			collections.PairKeyCodec(sdk.AccAddressKey, sdk.ValAddressKey),
			codec.CollValue[types.DelegationTimeEntry](cdc),
		),
		AccountScoreSnapshot: collections.NewMap(
			sb,
			types.AccountScoreKey,
			"account_score",
			sdk.AccAddressKey,
			sdk.IntValue,
		),
		AllocationSchedule: collections.NewMap(
			sb,
			types.AllocationScheduleKey,
			"allocation_schedule",
			collections.Uint64Key,
			codec.CollValue[types.ScheduledDistribution](cdc),
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}
