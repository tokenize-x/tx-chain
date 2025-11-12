package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

func TestDistribution_GenesisRebuild(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	ctx = ctx.WithBlockTime(time.Now()) // Set proper block time
	pseKeeper := testApp.PSEKeeper

	// Get bond denom
	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Set up mappings and fund modules for all eligible accounts
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr5 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	addrs := []string{addr1, addr2, addr3, addr4, addr5}
	var mappings []types.ClearingAccountMapping
	for i, clearingAccount := range types.GetNonCommunityClearingAccounts() {
		mappings = append(mappings, types.ClearingAccountMapping{
			ClearingAccount:  clearingAccount,
			RecipientAddress: addrs[i%len(addrs)],
		})
	}

	// Fund all non-Community clearing accounts
	for _, clearingAccount := range types.GetNonCommunityClearingAccounts() {
		fundAmount := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(5000)))
		err = testApp.BankKeeper.MintCoins(ctx, types.ModuleName, fundAmount)
		requireT.NoError(err)
		err = testApp.BankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, clearingAccount, fundAmount)
		requireT.NoError(err)
	}

	time1 := uint64(time.Now().Add(1 * time.Hour).Unix())
	time2 := uint64(time.Now().Add(2 * time.Hour).Unix())

	// Set up params with mappings
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.ClearingAccountMappings = mappings
	err = pseKeeper.SetParams(ctx, params)
	requireT.NoError(err)

	// Create and store allocation schedule with all eligible accounts
	schedule := []types.ScheduledDistribution{
		{
			Timestamp: time1,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(1000)},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(200)},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(300)},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(400)},
				{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(500)},
			},
		},
		{
			Timestamp: time2,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Amount: sdkmath.NewInt(2000)},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: sdkmath.NewInt(400)},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: sdkmath.NewInt(600)},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: sdkmath.NewInt(800)},
				{ClearingAccount: types.ClearingAccountTeam, Amount: sdkmath.NewInt(1000)},
			},
		},
	}

	// Store in allocation schedule map
	for _, scheduledDist := range schedule {
		err = pseKeeper.AllocationSchedule.Set(ctx, scheduledDist.Timestamp, scheduledDist)
		requireT.NoError(err)
	}

	// Process first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(time1)+10, 0))
	ctx = ctx.WithBlockHeight(100)
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// Export genesis
	genesisState, err := pseKeeper.ExportGenesis(ctx)
	requireT.NoError(err)

	// Verify export contains:
	// - 1 allocation in schedule (time2 only, since time1 was processed and removed)
	requireT.Len(genesisState.ScheduledDistributions, 1, "should have 1 remaining allocation (time2)")
	requireT.Equal(time2, genesisState.ScheduledDistributions[0].Timestamp)
	// Verify the remaining allocation has all 5 eligible accounts
	requireT.Len(
		genesisState.ScheduledDistributions[0].Allocations, 5,
		"should have allocations for all 5 eligible accounts",
	)

	// Create new app and import genesis
	testApp2 := simapp.New()
	ctx2 := testApp2.NewContext(false)
	ctx2 = ctx2.WithBlockTime(time.Unix(int64(time1)+10, 0)) // Set to same time as when we exported
	pseKeeper2 := testApp2.PSEKeeper

	// InitGenesis should restore allocation schedule from genesis state
	err = pseKeeper2.InitGenesis(ctx2, *genesisState)
	requireT.NoError(err)

	// Verify allocation schedule only contains time2 since time1 was already processed
	allocationSchedule2, err := pseKeeper2.GetAllocationSchedule(ctx2)
	requireT.NoError(err)
	requireT.Len(allocationSchedule2, 1, "should have 1 remaining allocation (time2)")
	requireT.Equal(time2, allocationSchedule2[0].Timestamp)
}
