package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v8/x/customparams/types"
)

func TestKeeper_InitAndExportGenesis(t *testing.T) {
	testApp := simapp.New()
	keeper := testApp.CustomParamsKeeper
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	genState := types.GenesisState{
		StakingParams: types.StakingParams{
			MinSelfDelegation: sdkmath.OneInt(),
			MaxVotingPower:    sdkmath.LegacyOneDec(),
		},
	}
	keeper.InitGenesis(ctx, genState)

	requireT := require.New(t)
	params, err := keeper.GetStakingParams(ctx)
	requireT.NoError(err)
	requireT.Equal(sdkmath.OneInt().String(), params.MinSelfDelegation.String())

	exportedGetState := keeper.ExportGenesis(ctx)
	requireT.Equal(genState, *exportedGetState)
}

func TestKeeper_InitGenesis_InvalidParams(t *testing.T) {
	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{})

	// a genesis without max_voting_power would start with a nil cap and fail every capped staking message
	require.Panics(t, func() {
		testApp.CustomParamsKeeper.InitGenesis(ctx, types.GenesisState{
			StakingParams: types.StakingParams{MinSelfDelegation: sdkmath.OneInt()},
		})
	})
	require.Panics(t, func() {
		testApp.CustomParamsKeeper.InitGenesis(ctx, types.GenesisState{
			StakingParams: types.StakingParams{
				MinSelfDelegation: sdkmath.OneInt(),
				MaxVotingPower:    sdkmath.LegacyMustNewDecFromStr("0.04"),
			},
		})
	})
}
