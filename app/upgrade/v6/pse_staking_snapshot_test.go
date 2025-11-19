package v6_test

import (
	"testing"

	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/stretchr/testify/require"

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
)

func TestSnapshotPSEStaking(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)

	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper
	addressCodec := testApp.AccountKeeper.AddressCodec()
	valAddressCodec := testApp.StakingKeeper.ValidatorAddressCodec()
	err := v6.SnapshotPSEStaking(ctx, stakingkeeper.NewQuerier(stakingKeeper), pseKeeper, addressCodec, valAddressCodec)
	requireT.NoError(err)
	// This is not an interesting test case, the real scenario can only be tested in integration tests.
}
