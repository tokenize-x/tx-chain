package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/testutil/simapp"
)

func TestKeeper_AccountScore(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)

	pseKeeper := testApp.PSEKeeper

	acc := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	score := sdkmath.NewInt(111)

	err := pseKeeper.AccountScoreSnapshot.Set(ctx, acc, score)
	requireT.NoError(err)

	gotScore, err := pseKeeper.AccountScoreSnapshot.Get(ctx, acc)
	requireT.NoError(err)
	requireT.Equal(score, gotScore)
}
