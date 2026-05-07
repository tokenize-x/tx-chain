package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

func TestKeeper_Delegation(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)

	pseKeeper := testApp.PSEKeeper

	delAddr := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	valAddr := sdk.ValAddress(secp256k1.GenPrivKey().PubKey().Address())
	distributionID := uint64(1)
	entry := types.DelegationTimeEntry{
		LastChangedUnixSec: time.Now().Unix(),
		Shares:             sdkmath.LegacyNewDec(10),
	}

	err := pseKeeper.SetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr, entry)
	requireT.NoError(err)

	gotEntry, err := pseKeeper.GetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr)
	requireT.NoError(err)
	requireT.Equal(entry, gotEntry)
}
