package v6

import (
	"context"

	addresscodec "cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// SnapshotPSEStaking snapshots the staking data into pse keeper.
func SnapshotPSEStaking(
	ctx context.Context,
	stakingKeeper stakingkeeper.Querier,
	pseKeeper pskeeper.Keeper,
	addressCodec addresscodec.Codec,
	valAddressCodec addresscodec.Codec,
) error {
	blockTimeUnixSeconds := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()
	delegations, err := stakingKeeper.GetAllDelegations(ctx)
	if err != nil {
		return err
	}

	for _, delegation := range delegations {
		valAddress, err := valAddressCodec.StringToBytes(delegation.ValidatorAddress)
		if err != nil {
			return err
		}
		delAddress, err := addressCodec.StringToBytes(delegation.DelegatorAddress)
		if err != nil {
			return err
		}
		err = pseKeeper.SetDelegationTimeEntry(ctx, valAddress, delAddress, psetypes.DelegationTimeEntry{
			LastChangedUnixSec: blockTimeUnixSeconds,
			Shares:             delegation.Shares,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
