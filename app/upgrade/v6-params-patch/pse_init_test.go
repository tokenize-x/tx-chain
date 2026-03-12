package v6paramspatch_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v6paramspatch "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6-params-patch"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestV6Patch(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).
		WithChainID(string(constant.ChainIDDev)).
		WithBlockTime(time.Now())

	// Run the patch: set PSE module params to testnet defaults
	err := v6paramspatch.V6ParamsPatch(ctx, testApp.PSEKeeper)
	requireT.NoError(err)

	// Verify params match testnet defaults
	params, err := testApp.PSEKeeper.GetParams(ctx)
	requireT.NoError(err)

	// Should have 5 clearing account mappings (Foundation, Alliance, Partnership, Investors, Team)
	requireT.Len(params.ClearingAccountMappings, 5, "should have 5 clearing account mappings")

	expectedAccounts := []string{
		psetypes.ClearingAccountFoundation,
		psetypes.ClearingAccountAlliance,
		psetypes.ClearingAccountPartnership,
		psetypes.ClearingAccountInvestors,
		psetypes.ClearingAccountTeam,
	}
	for i, mapping := range params.ClearingAccountMappings {
		requireT.Equal(expectedAccounts[i], mapping.ClearingAccount, "mapping %d clearing account", i)
		requireT.NotEmpty(mapping.RecipientAddresses, "mapping %s should have recipient addresses", mapping.ClearingAccount)
	}

	requireT.Len(params.ClearingAccountMappings[0].RecipientAddresses, 3, "Foundation should have 3 recipients")
	requireT.Len(params.ClearingAccountMappings[1].RecipientAddresses, 2, "Alliance should have 2 recipients")
	requireT.Len(params.ClearingAccountMappings[2].RecipientAddresses, 1, "Partnership should have 1 recipient")
	requireT.Len(params.ClearingAccountMappings[3].RecipientAddresses, 1, "Investors should have 1 recipient")
	requireT.Len(params.ClearingAccountMappings[4].RecipientAddresses, 1, "Team should have 1 recipient")

	// ExcludedAddresses = all mapping recipients + 3 other foundation addresses (11 unique for dev)
	requireT.Len(params.ExcludedAddresses, 11, "ExcludedAddresses should have 11 unique addresses")

	// Spot-check known dev addresses are excluded
	requireT.Contains(params.ExcludedAddresses, "devcore17cak5uy6k70l0hqqr3zrkrr960whz6jaqyey0d")
	requireT.Contains(params.ExcludedAddresses, "devcore1ma6a84s25n9q2f3wlsdwg22a84qknn2fggtrqn")
}
