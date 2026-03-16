package keeper

import (
	"context"

	"cosmossdk.io/collections"
	addresscodec "cosmossdk.io/core/address"
	sdkstore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
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
	DelegationTimeEntries collections.Map[
		collections.Triple[uint64, sdk.AccAddress, sdk.ValAddress],
		types.DelegationTimeEntry,
	]
	AccountScoreSnapshot        collections.Map[collections.Pair[uint64, sdk.AccAddress], sdkmath.Int]
	AllocationSchedule          collections.Map[uint64, types.ScheduledDistribution] // Map: ID -> ScheduledDistribution
	TotalScore                  collections.Map[uint64, sdkmath.Int]                 // Map: ID -> total accumulated score
	OngoingDistribution         collections.Item[types.ScheduledDistribution]        // Currently processing distribution
	DistributedAmount           collections.Map[uint64, sdkmath.Int]                 // ID -> distributed amount
	DistributionDisabled        collections.Item[bool]
	LastProcessedDistributionID collections.Item[uint64]
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
			collections.TripleKeyCodec(collections.Uint64Key, sdk.AccAddressKey, sdk.ValAddressKey),
			codec.CollValue[types.DelegationTimeEntry](cdc),
		),
		AccountScoreSnapshot: collections.NewMap(
			sb,
			types.AccountScoreKey,
			"account_score",
			collections.PairKeyCodec(collections.Uint64Key, sdk.AccAddressKey),
			sdk.IntValue,
		),
		AllocationSchedule: collections.NewMap(
			sb,
			types.AllocationScheduleKey,
			"allocation_schedule",
			collections.Uint64Key,
			codec.CollValue[types.ScheduledDistribution](cdc),
		),
		TotalScore: collections.NewMap(
			sb,
			types.TotalScoreKey,
			"total_score",
			collections.Uint64Key,
			sdk.IntValue,
		),
		OngoingDistribution: collections.NewItem(
			sb,
			types.OngoingDistributionKey,
			"ongoing_distribution",
			codec.CollValue[types.ScheduledDistribution](cdc),
		),
		DistributedAmount: collections.NewMap(
			sb,
			types.DistributedAmountKey,
			"distributed_amount",
			collections.Uint64Key,
			sdk.IntValue,
		),
		DistributionDisabled: collections.NewItem(
			sb,
			types.DistributionDisabledKey,
			"distribution_disabled",
			codec.BoolValue,
		),
		LastProcessedDistributionID: collections.NewItem(
			sb,
			types.LastProcessedDistributionIDKey,
			"last_processed_distribution_id",
			collections.Uint64Value,
		),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// StoreService returns the store service used by the keeper.
func (k Keeper) StoreService() sdkstore.KVStoreService {
	return k.storeService
}

// Codec returns the binary codec used by the keeper.
func (k Keeper) Codec() codec.BinaryCodec {
	return k.cdc
}

// GetClearingAccountBalances returns the current balances of all PSE clearing accounts in the bond denom.
func (k Keeper) GetClearingAccountBalances(ctx context.Context) ([]types.ClearingAccountBalance, error) {
	// Get bond denom from staking params
	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	// Get all clearing accounts
	clearingAccounts := types.GetAllClearingAccounts()
	balances := make([]types.ClearingAccountBalance, 0, len(clearingAccounts))

	// Query balance for each clearing account
	for _, account := range clearingAccounts {
		moduleAddr := k.accountKeeper.GetModuleAddress(account)
		if moduleAddr == nil {
			// Module account not found, set balance to zero
			balances = append(balances, types.ClearingAccountBalance{
				ClearingAccount: account,
				Balance:         sdkmath.ZeroInt(),
			})
			continue
		}

		balance := k.bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
		balances = append(balances, types.ClearingAccountBalance{
			ClearingAccount: account,
			Balance:         balance.Amount,
		})
	}

	return balances, nil
}
