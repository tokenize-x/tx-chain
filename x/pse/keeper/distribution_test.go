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
			ClearingAccount:    clearingAccount,
			RecipientAddresses: []string{addrs[i%len(addrs)]},
		})
	}

	// Fund all clearing accounts
	for _, clearingAccount := range types.GetAllClearingAccounts() {
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

	// Create and store allocation schedule with all clearing accounts
	schedule := []types.ScheduledDistribution{
		{
			Timestamp: time1,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountCommunity, Amount: sdkmath.NewInt(5000)},
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
				{ClearingAccount: types.ClearingAccountCommunity, Amount: sdkmath.NewInt(10000)},
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
	// Verify the remaining allocation has all 6 clearing accounts
	requireT.Len(
		genesisState.ScheduledDistributions[0].Allocations, 6,
		"should have allocations for all 6 clearing accounts",
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

func TestDistribution_PrecisionWithMultipleRecipients(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now())
	pseKeeper := testApp.PSEKeeper
	bankKeeper := testApp.BankKeeper

	// Get bond denom
	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Create multiple recipient addresses
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Set up mappings with multiple recipients
	mappings := []types.ClearingAccountMapping{
		// 3 recipients - will test remainder handling
		{ClearingAccount: types.ClearingAccountFoundation, RecipientAddresses: []string{addr1, addr2, addr3}},
		// 2 recipients
		{ClearingAccount: types.ClearingAccountAlliance, RecipientAddresses: []string{addr1, addr4}},
		// Single recipient (baseline)
		{ClearingAccount: types.ClearingAccountPartnership, RecipientAddresses: []string{addr1}},
		{ClearingAccount: types.ClearingAccountInvestors, RecipientAddresses: []string{addr1}},
		{ClearingAccount: types.ClearingAccountTeam, RecipientAddresses: []string{addr1}},
	}

	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.ClearingAccountMappings = mappings
	err = pseKeeper.SetParams(ctx, params)
	requireT.NoError(err)

	// Use amount that doesn't divide evenly by 3
	allocationAmount := sdkmath.NewInt(1000) // 1000 / 3 = 333 remainder 1

	// Fund the clearing accounts
	for _, clearingAccount := range types.GetAllClearingAccounts() {
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, allocationAmount))
		err = bankKeeper.MintCoins(ctx, types.ModuleName, coins)
		requireT.NoError(err)
		err = bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, clearingAccount, coins)
		requireT.NoError(err)
	}

	// Create and save distribution schedule
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix())
	schedule := []types.ScheduledDistribution{
		{
			Timestamp: startTime,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountCommunity, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountFoundation, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountTeam, Amount: allocationAmount},
			},
		},
	}

	err = pseKeeper.SaveDistributionSchedule(ctx, schedule)
	requireT.NoError(err)

	// Process distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(startTime)+10, 0))
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// Test Case 1: Foundation with 3 recipients (1000 / 3 = 333 remainder 1)
	// Each recipient gets equal amount (333), remainder (1) goes to community pool
	recipient1Balance := bankKeeper.GetBalance(ctx, sdk.MustAccAddressFromBech32(addr1), bondDenom)
	recipient2Balance := bankKeeper.GetBalance(ctx, sdk.MustAccAddressFromBech32(addr2), bondDenom)
	recipient3Balance := bankKeeper.GetBalance(ctx, sdk.MustAccAddressFromBech32(addr3), bondDenom)

	// addr1 gets distributions from Foundation (333), Alliance (500), Partnership (1000), Investors (1000), Team (1000)
	// = 333 + 500 + 1000 + 1000 + 1000 = 3833
	expectedAddr1 := sdkmath.NewInt(333 + 500 + 1000 + 1000 + 1000)
	requireT.Equal(expectedAddr1.String(), recipient1Balance.Amount.String(),
		"addr1 should get correct total without remainders")

	// addr2 gets only from Foundation (333)
	requireT.Equal("333", recipient2Balance.Amount.String(),
		"addr2 (Foundation recipient 2) should get base amount")

	// addr3 gets only from Foundation (333)
	requireT.Equal("333", recipient3Balance.Amount.String(),
		"addr3 (Foundation recipient 3) should get base amount")

	// addr4 gets only from Alliance (500)
	recipient4Balance := bankKeeper.GetBalance(ctx, sdk.MustAccAddressFromBech32(addr4), bondDenom)
	requireT.Equal("500", recipient4Balance.Amount.String(),
		"addr4 (Alliance recipient 2) should get base amount")

	// Verify total distributed from Foundation to recipients = 999 (333 * 3)
	// Remainder of 1 goes to community pool, not to recipients
	totalFoundationDistributed := sdkmath.NewInt(333 + 333 + 333)
	requireT.Equal("999", totalFoundationDistributed.String(),
		"total Foundation distribution to recipients should be 999 (remainder goes to community pool)")

	// Verify clearing accounts are empty (all distributed: recipients + remainder to community pool)
	for _, mapping := range mappings {
		if mapping.ClearingAccount == types.ClearingAccountCommunity {
			continue // Community doesn't distribute
		}
		moduleAddr := testApp.AccountKeeper.GetModuleAddress(mapping.ClearingAccount)
		moduleBalance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
		requireT.True(moduleBalance.Amount.IsZero(),
			"clearing account %s should be empty after distribution", mapping.ClearingAccount)
	}

	// Verify community pool received the remainders
	// Foundation: 1000 / 3 = 333 remainder 1
	// Alliance: 1000 / 2 = 500 remainder 0
	// Total expected remainder = 1
	communityPoolCoins, err := testApp.DistrKeeper.FeePool.Get(ctx)
	requireT.NoError(err)
	communityPoolBalance := communityPoolCoins.CommunityPool.AmountOf(bondDenom)
	// Only Foundation has remainder of 1 + CommunityClearingAccount
	expectedRemainder := sdkmath.LegacyNewDec(1001)
	requireT.Equal(expectedRemainder.String(), communityPoolBalance.String(),
		"community pool should have received the distribution remainders")
}
