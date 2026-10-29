package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v8/x/customparams/keeper"
	"github.com/tokenize-x/tx-chain/v8/x/customparams/types"
)

func TestMigrator_Migrate2to3(t *testing.T) {
	requireT := require.New(t)
	testApp := simapp.New()
	ctx := testApp.NewContext(false)

	// params encoded before max_voting_power existed: only field 1, min_self_delegation = "10000"
	minSelfDelegation := "10000"
	ctx.KVStore(testApp.GetKey(types.StoreKey)).Set(
		types.StakingParamsKey,
		append([]byte{0x0a, byte(len(minSelfDelegation))}, minSelfDelegation...),
	)
	params, err := testApp.CustomParamsKeeper.GetStakingParams(ctx)
	requireT.NoError(err)
	requireT.True(params.MaxVotingPower.IsNil())

	requireT.NoError(keeper.NewMigrator(testApp.CustomParamsKeeper, testApp.ParamsKeeper).Migrate2to3(ctx))

	params, err = testApp.CustomParamsKeeper.GetStakingParams(ctx)
	requireT.NoError(err)
	requireT.Equal(minSelfDelegation, params.MinSelfDelegation.String())
	requireT.Equal(sdkmath.LegacyOneDec().String(), params.MaxVotingPower.String())
	requireT.NoError(params.ValidateBasic())
}
