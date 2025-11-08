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
	storeService  sdkstore.KVStoreService
	cdc           codec.BinaryCodec
	authority     string
	accountKeeper types.AccountKeeper
	bankKeeper    types.BankKeeper
	stakingKeeper types.StakingKeeper

	// collections
	Schema                collections.Schema
	Params                collections.Item[types.Params]
	DelegationTimeEntries collections.Map[collections.Pair[sdk.ValAddress, sdk.AccAddress], types.DelegationTimeEntry]
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
	stakingKeeper types.StakingKeeper,
) Keeper {
	sb := collections.NewSchemaBuilder(storeService)
	k := Keeper{
		storeService:  storeService,
		cdc:           cdc,
		authority:     authority,
		accountKeeper: accountKeeper,
		bankKeeper:    bankKeeper,
		stakingKeeper: stakingKeeper,

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

// GetBondDenom returns the bond denomination from staking params.
// This is used as the distribution denom for all PSE distributions.
func (k Keeper) GetBondDenom(ctx sdk.Context) (string, error) {
	stakingParams, err := k.stakingKeeper.GetParams(ctx)
	if err != nil {
		return "", err
	}
	return stakingParams.BondDenom, nil
}
