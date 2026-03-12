package v6paramspatch_test

import (
	"testing"
	"time"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	v6paramspatch "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6-params-patch"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
)

func TestMigrateDenomMetadata(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).
		WithChainID(string(constant.ChainIDDev)).
		WithBlockTime(time.Now())

	// Reproduce the broken on-chain state left by v6: Display, Description, Symbol were
	// rebranded to "tx" but DenomUnits was missed, leaving "devcore" at exponent 6.
	testApp.BankKeeper.SetDenomMetaData(ctx, banktypes.Metadata{
		Description: "devtx coin",
		DenomUnits: []*banktypes.DenomUnit{
			{Denom: constant.DenomDev, Exponent: 0},
			{Denom: constant.DenomDevDisplay, Exponent: 6}, // "devcore" - not updated by v6
		},
		Base:    constant.DenomDev,
		Display: "devtx", // updated by v6
		Name:    constant.DenomDev,
		Symbol:  "udevtx",
	})

	// Verify the inconsistent state before fix
	meta, found := testApp.BankKeeper.GetDenomMetaData(ctx, constant.DenomDev)
	requireT.True(found)
	requireT.Equal("devtx", meta.Display)
	requireT.Equal("devcore", meta.DenomUnits[1].Denom)

	// Run the fix
	err := v6paramspatch.MigrateDenomMetadata(ctx, testApp.BankKeeper)
	requireT.NoError(err)

	// Verify the fix
	meta, found = testApp.BankKeeper.GetDenomMetaData(ctx, constant.DenomDev)
	requireT.True(found)
	requireT.Equal("devtx", meta.Display)
	requireT.Equal("devtx", meta.DenomUnits[1].Denom)
	requireT.Equal(uint32(6), meta.DenomUnits[1].Exponent)

	// Verify base unit unchanged
	requireT.Equal(constant.DenomDev, meta.DenomUnits[0].Denom)
	requireT.Equal(uint32(0), meta.DenomUnits[0].Exponent)

	// Verify the metadata passes validation
	requireT.NoError(meta.Validate())
}
