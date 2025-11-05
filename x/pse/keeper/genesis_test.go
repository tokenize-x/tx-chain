package keeper_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestGenesis(t *testing.T) {
	// Use DefaultGenesisState to ensure all slices are properly initialized
	genesisState := *types.DefaultGenesisState()

	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	genesisState.Params.ExcludedAddresses = []string{
		addr1,
		addr2,
	}

	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)

	pseKeeper := testApp.PSEKeeper

	err := pseKeeper.InitGenesis(ctx, genesisState)
	requireT.NoError(err)
	got, err := pseKeeper.ExportGenesis(ctx)
	requireT.NoError(err)
	requireT.NotNil(got)

	requireT.EqualExportedValues(&genesisState, got)
}
