package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v8/x/customparams/types"
)

func TestKeeper_UpdateStakingParams(t *testing.T) {
	requireT := require.New(t)
	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	// a proposal submitted before max_voting_power existed carries no value for it
	requireT.Error(testApp.CustomParamsKeeper.UpdateStakingParams(ctx, authority, types.StakingParams{
		MinSelfDelegation: sdkmath.NewInt(10_000),
	}))

	params := types.DefaultStakingParams()
	params.MaxVotingPower = sdkmath.LegacyMustNewDecFromStr("0.1")
	requireT.NoError(testApp.CustomParamsKeeper.UpdateStakingParams(ctx, authority, params))

	stored, err := testApp.CustomParamsKeeper.GetStakingParams(ctx)
	requireT.NoError(err)
	requireT.Equal(params.MaxVotingPower.String(), stored.MaxVotingPower.String())
}
