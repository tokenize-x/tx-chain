package keeper_test

import (
	"errors"
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
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
			ID:        1,
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
			ID:        2,
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
		err = pseKeeper.AllocationSchedule.Set(ctx, scheduledDist.ID, scheduledDist)
		requireT.NoError(err)
	}

	// Process first distribution
	ctx = ctx.WithBlockTime(time.Unix(int64(time1)+10, 0))
	ctx = ctx.WithBlockHeight(100)
	for range 10 {
		err = pseKeeper.ProcessNextDistribution(ctx)
		requireT.NoError(err)
		_, oErr := pseKeeper.OngoingDistribution.Get(ctx)
		if oErr != nil {
			break
		}
	}

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
	allocationSchedule2, err := pseKeeper2.GetDistributionSchedule(ctx2)
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
	// Note: Community is excluded from this test since it has different distribution logic
	// and is tested separately in other tests
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix())
	schedule := []types.ScheduledDistribution{
		{
			ID:        1,
			Timestamp: startTime,
			Allocations: []types.ClearingAccountAllocation{
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
	expectedRemainder := sdkmath.LegacyNewDec(1)
	requireT.Equal(expectedRemainder.String(), communityPoolBalance.String(),
		"community pool should have received the distribution remainders")
}

// TestDistribution_MultiBlockEndBlockerRouting tests the full EndBlocker routing logic
// across multiple calls to ProcessNextDistribution, verifying phase transitions:
//
//	Call 1 (idle -> start): non-community allocations distributed, OngoingDistribution set
//	Call 2 (Phase 1): score conversion batch processed
//	Call 3 (Phase 1 -> done): empty batch, TotalScore computed
//	Call 4 (Phase 2): tokens distributed to delegators
//	Call 5 (Phase 2 -> cleanup): empty batch, cleanup runs, OngoingDistribution removed
//	Call 6 (idle): no ongoing, no due schedule, nothing happens
func TestDistribution_MultiBlockEndBlockerRouting(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	pseKeeper := testApp.PSEKeeper
	bankKeeper := testApp.BankKeeper

	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Create validator
	valOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, valOp, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1000)))))
	val, err := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	// Create two delegators with delegations
	del1, _ := testApp.GenAccount(ctx)
	del2, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, del1, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000)))))
	requireT.NoError(testApp.FundAccount(ctx, del2, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000)))))

	distributionID := uint64(1)
	recipientAddr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Save initial schedule for hooks to find the distribution ID
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: distributionID, Timestamp: distributionID},
	})
	requireT.NoError(err)

	// Delegate
	for _, del := range []sdk.AccAddress{del1, del2} {
		msg := &stakingtypes.MsgDelegate{
			DelegatorAddress: del.String(),
			ValidatorAddress: valAddr.String(),
			Amount:           sdk.NewInt64Coin(bondDenom, 500),
		}
		_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, msg)
		requireT.NoError(err)
	}

	// Advance time for score accumulation
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(10 * time.Second))
	requireT.NoError(err)

	// Set up clearing account mappings
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.ClearingAccountMappings = []types.ClearingAccountMapping{
		{ClearingAccount: types.ClearingAccountFoundation, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountAlliance, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountPartnership, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountInvestors, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountTeam, RecipientAddresses: []string{recipientAddr}},
	}
	err = pseKeeper.SetParams(ctx, params)
	requireT.NoError(err)

	// Fund all clearing accounts
	communityAmount := sdkmath.NewInt(10_000_000)
	nonCommunityAmount := sdkmath.NewInt(100)
	for _, clearingAccount := range types.GetAllClearingAccounts() {
		amount := nonCommunityAmount
		if clearingAccount == types.ClearingAccountCommunity {
			amount = communityAmount
		}
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, amount))
		err = bankKeeper.MintCoins(ctx, minttypes.ModuleName, coins)
		requireT.NoError(err)
		err = bankKeeper.SendCoinsFromModuleToModule(ctx, minttypes.ModuleName, clearingAccount, coins)
		requireT.NoError(err)
	}

	// Update schedule with the actual distribution (due now)
	distTimestamp := uint64(ctx.BlockTime().Unix())
	err = pseKeeper.AllocationSchedule.Remove(ctx, distributionID)
	requireT.NoError(err)
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{
			ID:        distributionID,
			Timestamp: distTimestamp,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountCommunity, Amount: communityAmount},
				{ClearingAccount: types.ClearingAccountFoundation, Amount: nonCommunityAmount},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: nonCommunityAmount},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: nonCommunityAmount},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: nonCommunityAmount},
				{ClearingAccount: types.ClearingAccountTeam, Amount: nonCommunityAmount},
			},
		},
	})
	requireT.NoError(err)

	// --- Call 1: Start distribution ---
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// Verify: OngoingDistribution should be set
	ongoing, err := pseKeeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(distributionID, ongoing.ID)

	// Verify: non-community recipient should have received tokens
	recipientBalance := bankKeeper.GetBalance(ctx, sdk.MustAccAddressFromBech32(recipientAddr), bondDenom)
	requireT.Equal(nonCommunityAmount.MulRaw(5).String(), recipientBalance.Amount.String(),
		"recipient should have received all 5 non-community allocations")

	// Verify: TotalScore should NOT exist yet (Phase 1 hasn't run)
	_, err = pseKeeper.TotalScore.Get(ctx, distributionID)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// --- Call 2: Phase 1 (process score entries) ---
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// TotalScore still not set (entries processed but empty-batch call needed to compute it)
	_, err = pseKeeper.TotalScore.Get(ctx, distributionID)
	requireT.ErrorIs(err, collections.ErrNotFound)

	// Verify entries migrated from distributionID to distributionID+1
	hasEntries := false
	err = pseKeeper.DelegationTimeEntries.Walk(ctx,
		collections.NewPrefixedTripleRange[uint64, sdk.AccAddress, sdk.ValAddress](distributionID+1),
		func(key collections.Triple[uint64, sdk.AccAddress, sdk.ValAddress], value types.DelegationTimeEntry) (bool, error) {
			hasEntries = true
			return true, nil
		})
	requireT.NoError(err)
	requireT.True(hasEntries, "entries should be migrated to next distribution ID")

	// --- Call 3: Phase 1 done (empty batch -> compute TotalScore) ---
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// TotalScore should now exist
	totalScore, err := pseKeeper.TotalScore.Get(ctx, distributionID)
	requireT.NoError(err)
	requireT.True(totalScore.IsPositive(), "TotalScore should be positive")

	// OngoingDistribution should still exist
	_, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)

	// --- Call 4: Phase 2 (distribute tokens) ---
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// OngoingDistribution should still exist (cleanup hasn't run yet)
	_, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)

	// --- Call 5: Phase 2 done (empty batch -> cleanup) ---
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// OngoingDistribution should be removed
	_, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.ErrorIs(err, collections.ErrNotFound, "OngoingDistribution should be removed after cleanup")

	// Schedule entry should be removed
	_, err = pseKeeper.AllocationSchedule.Get(ctx, distributionID)
	requireT.ErrorIs(err, collections.ErrNotFound, "schedule entry should be removed after cleanup")

	// TotalScore should be cleaned up
	_, err = pseKeeper.TotalScore.Get(ctx, distributionID)
	requireT.ErrorIs(err, collections.ErrNotFound, "TotalScore should be removed after cleanup")

	// Delegators should have received community tokens (auto-delegated)
	stakingQuerier := stakingkeeper.NewQuerier(testApp.StakingKeeper)
	for _, del := range []sdk.AccAddress{del1, del2} {
		resp, err := stakingQuerier.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
			DelegatorAddr: del.String(),
		})
		requireT.NoError(err)
		totalDelegated := sdkmath.NewInt(0)
		for _, d := range resp.DelegationResponses {
			totalDelegated = totalDelegated.Add(d.Balance.Amount)
		}
		requireT.True(totalDelegated.GT(sdkmath.NewInt(500)),
			"delegator should have more than initial 500 after community distribution")
	}

	// --- Call 6: Idle (nothing to do) ---
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// Still no ongoing
	_, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.ErrorIs(err, collections.ErrNotFound)
}

// TestDistribution_NonCommunityOnlySingleBlock tests that a distribution with
// no community allocation completes in a single call to ProcessNextDistribution.
func TestDistribution_NonCommunityOnlySingleBlock(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx, _, err := testApp.BeginNextBlock()
	requireT.NoError(err)

	pseKeeper := testApp.PSEKeeper
	bankKeeper := testApp.BankKeeper

	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	recipientAddr := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Set up mappings
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.ClearingAccountMappings = []types.ClearingAccountMapping{
		{ClearingAccount: types.ClearingAccountFoundation, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountAlliance, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountPartnership, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountInvestors, RecipientAddresses: []string{recipientAddr}},
		{ClearingAccount: types.ClearingAccountTeam, RecipientAddresses: []string{recipientAddr}},
	}
	err = pseKeeper.SetParams(ctx, params)
	requireT.NoError(err)

	// Fund non-community clearing accounts only
	amount := sdkmath.NewInt(100)
	for _, clearingAccount := range types.GetNonCommunityClearingAccounts() {
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, amount))
		err = bankKeeper.MintCoins(ctx, minttypes.ModuleName, coins)
		requireT.NoError(err)
		err = bankKeeper.SendCoinsFromModuleToModule(ctx, minttypes.ModuleName, clearingAccount, coins)
		requireT.NoError(err)
	}

	// Schedule with zero community allocation
	distTime := uint64(ctx.BlockTime().Unix()) - 1
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{
			ID:        1,
			Timestamp: distTime,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountCommunity, Amount: sdkmath.NewInt(0)},
				{ClearingAccount: types.ClearingAccountFoundation, Amount: amount},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: amount},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: amount},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: amount},
				{ClearingAccount: types.ClearingAccountTeam, Amount: amount},
			},
		},
	})
	requireT.NoError(err)

	// Single call should complete everything
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// No OngoingDistribution should be set (no community allocation)
	_, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.ErrorIs(err, collections.ErrNotFound, "no OngoingDistribution for non-community-only distribution")

	// Schedule entry should be removed
	_, err = pseKeeper.AllocationSchedule.Get(ctx, 1)
	requireT.ErrorIs(err, collections.ErrNotFound, "schedule should be removed after single-block distribution")

	// Recipient should have received all non-community tokens
	recipientBalance := bankKeeper.GetBalance(ctx, sdk.MustAccAddressFromBech32(recipientAddr), bondDenom)
	requireT.Equal(amount.MulRaw(5).String(), recipientBalance.Amount.String())
}

// TestDistribution_EndBlockerWithScenarios mirrors TestKeeper_Distribute scenarios but routes
// through ProcessNextDistribution (the actual EndBlocker entry point) instead of calling
// Phase1/Phase2 directly. This validates the full EndBlocker routing with real delegation flows.
func TestDistribution_EndBlockerWithScenarios(t *testing.T) {
	cases := []struct {
		name    string
		actions []func(*runEnv)
	}{
		{
			name: "unaccumulated score via EndBlocker",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { endBlockerDistributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_366),
						&r.delegators[1]: sdkmath.NewInt(900_299),
					})
				},
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "accumulated + unaccumulated score via EndBlocker",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 900_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 1_100_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { endBlockerDistributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(2_000_387),
						&r.delegators[1]: sdkmath.NewInt(2_000_362),
					})
				},
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "unbonding delegation via EndBlocker",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[0], 900_000) },
				func(r *runEnv) { undelegateAction(r, r.delegators[1], r.validators[0], 700_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { endBlockerDistributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(200_295),
						&r.delegators[1]: sdkmath.NewInt(200_249),
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "redelegation via EndBlocker",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { redelegateAction(r, r.delegators[0], r.validators[0], r.validators[2], 900_000) },
				func(r *runEnv) { redelegateAction(r, r.delegators[1], r.validators[0], r.validators[2], 700_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { endBlockerDistributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_365),
						&r.delegators[1]: sdkmath.NewInt(900_298),
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "zero score via EndBlocker",
			actions: []func(*runEnv){
				func(r *runEnv) { endBlockerDistributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(0),
						&r.delegators[1]: sdkmath.NewInt(0),
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "multiple distributions via EndBlocker",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { endBlockerDistributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_366),
						&r.delegators[1]: sdkmath.NewInt(900_299),
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { endBlockerDistributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_732),
						&r.delegators[1]: sdkmath.NewInt(900_598),
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(4)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)
			startTime := time.Now().Round(time.Second)
			testApp := simapp.New(simapp.WithStartTime(startTime))
			ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
			requireT.NoError(err)
			runContext := &runEnv{
				testApp:       testApp,
				ctx:           ctx,
				requireT:      requireT,
				currentDistID: firstDistributionID,
			}

			// add validators.
			for range 3 {
				validatorOperator, _ := testApp.GenAccount(ctx)
				requireT.NoError(testApp.FundAccount(
					ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000)))),
				)
				validator, err := testApp.AddValidator(
					ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
				)
				requireT.NoError(err)
				runContext.validators = append(
					runContext.validators,
					sdk.MustValAddressFromBech32(validator.GetOperator()),
				)
			}

			// add delegators.
			for range 3 {
				delegator, _ := testApp.GenAccount(ctx)
				requireT.NoError(testApp.FundAccount(
					ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
				))
				runContext.delegators = append(runContext.delegators, delegator)
			}

			err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
				{
					Timestamp: firstDistributionID,
					ID:        firstDistributionID,
				},
			})
			requireT.NoError(err)

			// run actions.
			for _, action := range tc.actions {
				action(runContext)
			}
		})
	}
}

// TestEdgeCase_DelegationChangeDuringPhase1 verifies that when a delegator changes
// their delegation while Phase 1 (score conversion) is in progress, the hook correctly:
//   - Splits the score at the distribution timestamp (ongoing score into ongoingID, post-dist score into nextID)
//   - Removes the entry from ongoingID to prevent double-scoring by Phase 1 batch
//   - Creates new entry under nextID with updated shares
//
// Test Flow: delegate -> wait 8s -> start distribution -> delegate again (before Phase 1 batch processing)
func TestEdgeCase_DelegationChangeDuringPhase1(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	r := &runEnv{
		testApp:       testApp,
		ctx:           ctx,
		requireT:      requireT,
		currentDistID: firstDistributionID,
	}

	// Add validators.
	for range 3 {
		validatorOperator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		validator, err := testApp.AddValidator(
			ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
		)
		requireT.NoError(err)
		r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))
	}

	// Add delegators.
	for range 3 {
		delegator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		r.delegators = append(r.delegators, delegator)
	}

	err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: firstDistributionID, Timestamp: firstDistributionID},
	})
	requireT.NoError(err)

	// T=0s: Both delegators delegate.
	delegateAction(r, r.delegators[0], r.validators[0], 1_100_000)
	delegateAction(r, r.delegators[1], r.validators[0], 900_000)

	// T=8s: Advance time for score accumulation.
	waitAction(r, time.Second*8)

	// Start ongoing distribution (sets OngoingDistribution with distTimestamp=now).
	// Entries exist under ongoingID=1.
	startOngoingDistributionAction(r, sdkmath.NewInt(1000))

	// delegators[0] changes delegation BEFORE Phase 1 runs any batches.
	// AfterDelegationModified hook should trigger with Scenario-1 (migrateOngoingEntry):
	delegateAction(r, r.delegators[0], r.validators[0], 50_000)

	// Verify: delegators[0] entry removed from ongoingID=1 (migrateOngoingEntry cleaned it up).
	assertEntryNotExistsUnderDistIDAction(r, r.currentDistID, r.delegators[0], r.validators[0])

	// Verify: delegators[0] entry exists under nextID=2 (hook created it with new shares).
	nextID := r.currentDistID + 1
	assertEntryExistsUnderDistIDAction(r, nextID, r.delegators[0], r.validators[0])
	// Verify: delegators[0] entry removed from ongoingID
	assertEntryNotExistsUnderDistIDAction(r, r.currentDistID, r.delegators[0], r.validators[0])

	// Verify: delegators[0] has score snapshot for ongoingID=1 from migrateOngoingEntry.
	score0, err := testApp.PSEKeeper.GetDelegatorScore(r.ctx, r.currentDistID, r.delegators[0])
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1_100_000*8), score0) // 1,100,000 tokens × 8 seconds

	// Verify: delegators[1] entry still under ongoingID=1 (no delegation change, no hook fired).
	assertEntryExistsUnderDistIDAction(r, r.currentDistID, r.delegators[1], r.validators[0])

	// Run Phase 1 to completion to process remaining entries.
	for {
		done := runPhase1BatchAction(r)
		if done {
			break
		}
	}

	// Verify both delegators have scores for ongoingID=1.
	score1, err := testApp.PSEKeeper.GetDelegatorScore(r.ctx, r.currentDistID, r.delegators[1])
	requireT.NoError(err)
	requireT.True(score1.IsPositive(), "delegators[1] should have score from Phase 1 batch")

	// Finish distribution (Phase 2 + cleanup).
	finishDistributionAction(r)

	// Verify rewards were auto-delegated: initial delegation + PSE reward.
	assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
		&r.delegators[0]: sdkmath.NewInt(1_150_366), // 1,100,000 + 50,000 + 366 reward
		&r.delegators[1]: sdkmath.NewInt(900_299),   // 900,000 + 299 reward
	})
	assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2))

	// Verify cleanup completed: scores cleared, entries migrated to nextID.
	assertScoreResetAction(r)
}

// TestEdgeCase_NewDelegatorDuringOngoingDistribution verifies that a new delegator
// who stakes for the first time while a distribution is ongoing:
//   - Gets entry created under nextID (not ongoingID) via hook Scenario 3
//   - Has zero score for the ongoing distribution
//   - Receives zero rewards from the current distribution
//   - Does not affect existing delegators' rewards
//
// Test Flow: delegate (2 users) -> wait 8s -> start distribution -> new user delegates -> finish distribution
func TestEdgeCase_NewDelegatorDuringOngoingDistribution(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	r := &runEnv{
		testApp:       testApp,
		ctx:           ctx,
		requireT:      requireT,
		currentDistID: firstDistributionID,
	}

	// Add validators.
	for range 3 {
		validatorOperator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		validator, err := testApp.AddValidator(
			ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
		)
		requireT.NoError(err)
		r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))
	}

	// Add delegators (3: two existing + one new who will delegate mid-distribution).
	for range 3 {
		delegator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		r.delegators = append(r.delegators, delegator)
	}

	err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: firstDistributionID, Timestamp: firstDistributionID},
	})
	requireT.NoError(err)

	// Only delegators[0] and delegators[1] delegate initially.
	delegateAction(r, r.delegators[0], r.validators[0], 1_100_000)
	delegateAction(r, r.delegators[1], r.validators[0], 900_000)
	waitAction(r, time.Second*8)

	// Start ongoing distribution.
	startOngoingDistributionAction(r, sdkmath.NewInt(1000))

	// delegators[2] delegates for the first time DURING ongoing distribution.
	// Hook Scenario 3: no entry under ongoingID or nextID -> create under nextID.
	delegateAction(r, r.delegators[2], r.validators[0], 500_000)

	// Verify: delegators[2] has NO entry under ongoingID=1.
	assertEntryNotExistsUnderDistIDAction(r, r.currentDistID, r.delegators[2], r.validators[0])

	// Verify: delegators[2] has entry under nextID=2.
	nextID := r.currentDistID + 1
	assertEntryExistsUnderDistIDAction(r, nextID, r.delegators[2], r.validators[0])

	// Verify: delegators[2] has no score snapshot for the ongoing distribution.
	_, err = testApp.PSEKeeper.GetDelegatorScore(r.ctx, r.currentDistID, r.delegators[2])
	requireT.ErrorIs(err, collections.ErrNotFound, "new delegator should have no score for ongoing distribution")

	// Finish distribution (Phase 1 + Phase 2 + cleanup).
	finishDistributionAction(r)

	// Verify: delegators[2] received zero rewards (staking balance = initial delegation only).
	assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
		&r.delegators[0]: sdkmath.NewInt(1_100_366), // 1,100,000 + 366 reward
		&r.delegators[1]: sdkmath.NewInt(900_299),   // 900,000 + 299 reward
		&r.delegators[2]: sdkmath.NewInt(500_000),   // 500,000 only, no reward
	})
	assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2))
	assertScoreResetAction(r)
}

// TestEdgeCase_ConsecutiveDistributions verifies that LastProcessedDistributionID
// advances correctly across consecutive distributions (0 -> 1 -> 2), and that
// entries are properly migrated between distribution IDs after each round.
//
// Test Flow: delegate -> wait -> distribute (ID=1) -> wait -> distribute (ID=2) -> verify state
func TestEdgeCase_ConsecutiveDistributions(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	r := &runEnv{
		testApp:       testApp,
		ctx:           ctx,
		requireT:      requireT,
		currentDistID: firstDistributionID,
	}

	// Add validators.
	for range 3 {
		validatorOperator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		validator, err := testApp.AddValidator(
			ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
		)
		requireT.NoError(err)
		r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))
	}

	// Add delegators.
	for range 2 {
		delegator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		r.delegators = append(r.delegators, delegator)
	}

	err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: firstDistributionID, Timestamp: firstDistributionID},
	})
	requireT.NoError(err)

	// Before any distribution: LastProcessedDistributionID should not be set.
	assertLastProcessedDistributionIDAction(r, 0)

	// T=0s: Delegate.
	delegateAction(r, r.delegators[0], r.validators[0], 1_100_000)
	delegateAction(r, r.delegators[1], r.validators[0], 900_000)

	// Verify entries exist under distID=1.
	assertEntryExistsUnderDistIDAction(r, 1, r.delegators[0], r.validators[0])
	assertEntryExistsUnderDistIDAction(r, 1, r.delegators[1], r.validators[0])

	// T=8s: Accumulate score.
	waitAction(r, time.Second*8)

	// Distribution 1 (ID=1): run full distribution via EndBlocker path.
	endBlockerDistributeAction(r, sdkmath.NewInt(1000))

	// After distribution 1: LastProcessedDistributionID = 1.
	assertLastProcessedDistributionIDAction(r, 1)
	assertOngoingDistributionNotExistsAction(r)

	// Verify rewards from distribution 1.
	assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
		&r.delegators[0]: sdkmath.NewInt(1_100_366), // 1,100,000 + 366 reward
		&r.delegators[1]: sdkmath.NewInt(900_299),   // 900,000 + 299 reward
	})
	assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2))
	assertScoreResetAction(r)

	// Entries should now exist under distID=2 (migrated by Phase 1).
	assertEntryExistsUnderDistIDAction(r, 2, r.delegators[0], r.validators[0])
	assertEntryExistsUnderDistIDAction(r, 2, r.delegators[1], r.validators[0])

	// Schedule distribution 2.
	err = testApp.PSEKeeper.SaveDistributionSchedule(r.ctx, []types.ScheduledDistribution{
		{ID: 2, Timestamp: uint64(r.ctx.BlockTime().Unix())},
	})
	requireT.NoError(err)

	// T=16s: Accumulate more score.
	waitAction(r, time.Second*8)

	// Distribution 2 (ID=2).
	endBlockerDistributeAction(r, sdkmath.NewInt(1000))

	// After distribution 2: LastProcessedDistributionID = 2.
	assertLastProcessedDistributionIDAction(r, 2)
	assertOngoingDistributionNotExistsAction(r)

	// Verify rewards from distribution 2 (cumulative: initial + dist1 reward + dist2 reward).
	assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
		&r.delegators[0]: sdkmath.NewInt(1_100_732), // 1,100,366 + 366
		&r.delegators[1]: sdkmath.NewInt(900_598),   // 900,299 + 299
	})
	assertCommunityPoolBalanceAction(r, sdkmath.NewInt(4))
	assertScoreResetAction(r)

	// Entries should now exist under distID=3.
	assertEntryExistsUnderDistIDAction(r, 3, r.delegators[0], r.validators[0])
	assertEntryExistsUnderDistIDAction(r, 3, r.delegators[1], r.validators[0])
}

// TestEdgeCase_ScheduleUpdateRejectedDuringOngoing verifies that UpdateDistributionSchedule
// rejects schedule updates while a multi-block distribution is in progress.
func TestEdgeCase_ScheduleUpdateRejectedDuringOngoing(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	r := &runEnv{
		testApp:       testApp,
		ctx:           ctx,
		requireT:      requireT,
		currentDistID: firstDistributionID,
	}

	// Add validators.
	for range 3 {
		validatorOperator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		validator, err := testApp.AddValidator(
			ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
		)
		requireT.NoError(err)
		r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))
	}

	// Add delegator.
	delegator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	r.delegators = append(r.delegators, delegator)

	err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: firstDistributionID, Timestamp: firstDistributionID},
	})
	requireT.NoError(err)

	delegateAction(r, r.delegators[0], r.validators[0], 1_000_000)
	waitAction(r, time.Second*8)

	// Start ongoing distribution.
	startOngoingDistributionAction(r, sdkmath.NewInt(1000))

	// Attempt to update schedule while distribution is ongoing.
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	err = testApp.PSEKeeper.UpdateDistributionSchedule(r.ctx, authority, []types.ScheduledDistribution{
		{ID: 2, Timestamp: uint64(r.ctx.BlockTime().Unix()) + 3600},
	})
	requireT.ErrorIs(err, types.ErrOngoingDistribution)

	// Finish distribution and verify schedule update succeeds after.
	finishDistributionAction(r)

	err = testApp.PSEKeeper.UpdateDistributionSchedule(r.ctx, authority, []types.ScheduledDistribution{
		{ID: 2, Timestamp: uint64(r.ctx.BlockTime().Unix()) + 3600},
	})
	requireT.NoError(err)
}

// TestEdgeCase_ScheduleUpdateWithPastIDsRejected verifies that UpdateDistributionSchedule
// rejects schedules with IDs that have already been processed (ID < LastProcessedDistributionID + 1).
func TestEdgeCase_ScheduleUpdateWithPastIDsRejected(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	r := &runEnv{
		testApp:       testApp,
		ctx:           ctx,
		requireT:      requireT,
		currentDistID: firstDistributionID,
	}

	// Add validators.
	for range 3 {
		validatorOperator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		validator, err := testApp.AddValidator(
			ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
		)
		requireT.NoError(err)
		r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))
	}

	// Add delegator.
	delegator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	r.delegators = append(r.delegators, delegator)

	err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: firstDistributionID, Timestamp: firstDistributionID},
	})
	requireT.NoError(err)

	delegateAction(r, r.delegators[0], r.validators[0], 1_000_000)
	waitAction(r, time.Second*8)

	// Run distribution ID=1.
	endBlockerDistributeAction(r, sdkmath.NewInt(1000))
	assertLastProcessedDistributionIDAction(r, 1)

	// Schedule distribution ID=2, run it.
	err = testApp.PSEKeeper.SaveDistributionSchedule(r.ctx, []types.ScheduledDistribution{
		{ID: 2, Timestamp: uint64(r.ctx.BlockTime().Unix())},
	})
	requireT.NoError(err)
	waitAction(r, time.Second*8)
	endBlockerDistributeAction(r, sdkmath.NewInt(1000))
	assertLastProcessedDistributionIDAction(r, 2)

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	// Reject: ID=1 (already processed).
	err = testApp.PSEKeeper.UpdateDistributionSchedule(r.ctx, authority, []types.ScheduledDistribution{
		{ID: 1, Timestamp: uint64(r.ctx.BlockTime().Unix()) + 3600},
	})
	requireT.ErrorIs(err, types.ErrInvalidInput)

	// Reject: ID=2 (already processed).
	err = testApp.PSEKeeper.UpdateDistributionSchedule(r.ctx, authority, []types.ScheduledDistribution{
		{ID: 2, Timestamp: uint64(r.ctx.BlockTime().Unix()) + 3600},
	})
	requireT.ErrorIs(err, types.ErrInvalidInput)

	// Accept: ID=3 (next valid ID).
	err = testApp.PSEKeeper.UpdateDistributionSchedule(r.ctx, authority, []types.ScheduledDistribution{
		{ID: 3, Timestamp: uint64(r.ctx.BlockTime().Unix()) + 3600},
	})
	requireT.NoError(err)
}

// TestEdgeCase_Phase2FairnessBonus verifies that after Phase 2 auto-delegates reward tokens,
// a bonus score of `rewardAmount × (blockTime - distributionTimestamp)` is added to the
// next distribution's score snapshot. This ensures delegators processed in later batches
// aren't disadvantaged compared to those processed earlier.
//
// Test Flow: delegate -> wait 8s -> start distribution -> Phase 1 -> wait 4s -> Phase 2 -> verify bonus score
func TestEdgeCase_Phase2FairnessBonus(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	r := &runEnv{
		testApp:       testApp,
		ctx:           ctx,
		requireT:      requireT,
		currentDistID: firstDistributionID,
	}

	// Add validators.
	for range 3 {
		validatorOperator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		validator, err := testApp.AddValidator(
			ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
		)
		requireT.NoError(err)
		r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))
	}

	// Add delegators.
	for range 2 {
		delegator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		r.delegators = append(r.delegators, delegator)
	}

	err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: firstDistributionID, Timestamp: firstDistributionID},
	})
	requireT.NoError(err)

	// T=0s: Delegate.
	delegateAction(r, r.delegators[0], r.validators[0], 1_100_000)
	delegateAction(r, r.delegators[1], r.validators[0], 900_000)

	// T=8s: Accumulate score.
	waitAction(r, time.Second*8)

	// Start ongoing distribution at T=8s.
	startOngoingDistributionAction(r, sdkmath.NewInt(1000))
	distTimestamp := r.ctx.BlockTime().Unix()

	// Run Phase 1 to completion.
	for {
		done := runPhase1BatchAction(r)
		if done {
			break
		}
	}

	// Advance 4s so elapsed = 4 when Phase 2 runs.
	waitAction(r, time.Second*4)

	// Capture scores BEFORE Phase 2 for nextID.
	nextID := r.currentDistID + 1
	scoreBefore0, err := testApp.PSEKeeper.GetDelegatorScore(r.ctx, nextID, r.delegators[0])
	if errors.Is(err, collections.ErrNotFound) {
		scoreBefore0 = sdkmath.NewInt(0)
	} else {
		requireT.NoError(err)
	}
	scoreBefore1, err := testApp.PSEKeeper.GetDelegatorScore(r.ctx, nextID, r.delegators[1])
	if errors.Is(err, collections.ErrNotFound) {
		scoreBefore1 = sdkmath.NewInt(0)
	} else {
		requireT.NoError(err)
	}

	// Run Phase 2 (distributes rewards and adds fairness bonus).
	ongoing, err := testApp.PSEKeeper.OngoingDistribution.Get(r.ctx)
	requireT.NoError(err)
	bondDenom, err := testApp.StakingKeeper.BondDenom(r.ctx)
	requireT.NoError(err)
	for {
		done, err := testApp.PSEKeeper.ProcessPhase2TokenDistribution(r.ctx, ongoing, bondDenom)
		requireT.NoError(err)
		if done {
			break
		}
	}

	// Verify bonus scores were added to nextID.
	scoreAfter0, err := testApp.PSEKeeper.GetDelegatorScore(r.ctx, nextID, r.delegators[0])
	requireT.NoError(err)
	scoreAfter1, err := testApp.PSEKeeper.GetDelegatorScore(r.ctx, nextID, r.delegators[1])
	requireT.NoError(err)

	scoreAdded0 := scoreAfter0.Sub(scoreBefore0)
	scoreAdded1 := scoreAfter1.Sub(scoreBefore1)

	elapsed := r.ctx.BlockTime().Unix() - distTimestamp

	// Phase 2 auto-delegation triggers AfterDelegationModified hook (Scenario 2):
	// hookScore = oldTokens × (blockTime - entry.lastChanged), where lastChanged = distTimestamp.
	hookScore0 := sdkmath.NewInt(1_100_000).MulRaw(elapsed)
	hookScore1 := sdkmath.NewInt(900_000).MulRaw(elapsed)

	// Fairness bonus = distributedAmount × elapsed.
	// TotalScore = 1,100,000×8 + 900,000×8 + genesis(1,000,000×8) + 3 vals(10×8) = 24,000,240
	// reward[0] = 1000 × 8,800,000 / 24,000,240 = 366
	// reward[1] = 1000 × 7,200,000 / 24,000,240 = 299
	fairnessBonus0 := sdkmath.NewInt(366).MulRaw(elapsed)
	fairnessBonus1 := sdkmath.NewInt(299).MulRaw(elapsed)

	// Total score added = hook score + fairness bonus.
	requireT.Equal(hookScore0.Add(fairnessBonus0), scoreAdded0,
		"delegator[0] score delta should include hook score + fairness bonus")
	requireT.Equal(hookScore1.Add(fairnessBonus1), scoreAdded1,
		"delegator[1] score delta should include hook score + fairness bonus")
}

func TestDistribution_EndBlockFailure(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx, _, err := testApp.BeginNextBlock()
	requireT.NoError(err)
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
	recipients := []string{addr1, addr2, addr3, addr4}

	// Set up mappings with multiple recipients
	mappings := []types.ClearingAccountMapping{
		{ClearingAccount: types.ClearingAccountFoundation, RecipientAddresses: []string{addr1}},
		{ClearingAccount: types.ClearingAccountAlliance, RecipientAddresses: []string{addr2}},
		{ClearingAccount: types.ClearingAccountPartnership, RecipientAddresses: []string{addr3}},
		{ClearingAccount: types.ClearingAccountInvestors, RecipientAddresses: []string{addr4}},
		{ClearingAccount: types.ClearingAccountTeam, RecipientAddresses: []string{addr4}},
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
		// we skip team clearing account, so it will lead to not enough funds error in end block.
		if clearingAccount == types.ClearingAccountTeam {
			continue
		}
		coins := sdk.NewCoins(sdk.NewCoin(bondDenom, allocationAmount))
		err = bankKeeper.MintCoins(ctx, types.ModuleName, coins)
		requireT.NoError(err)
		err = bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, clearingAccount, coins)
		requireT.NoError(err)
	}

	// Create and save distribution schedule
	// Note: Community is excluded from this test since it has different distribution logic
	// and is tested separately in other tests
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix())
	schedule := []types.ScheduledDistribution{
		{
			ID:        1,
			Timestamp: startTime,
			Allocations: []types.ClearingAccountAllocation{
				{ClearingAccount: types.ClearingAccountFoundation, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountAlliance, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountPartnership, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountInvestors, Amount: allocationAmount},
				{ClearingAccount: types.ClearingAccountTeam, Amount: allocationAmount},
			},
		},
	}

	// Save distribution schedule
	err = pseKeeper.SaveDistributionSchedule(ctx, schedule)
	requireT.NoError(err)
	// Process distribution
	err = testApp.FinalizeBlock()
	requireT.NoError(err)

	// Verify disabled distributions is set to true
	disabled, err := pseKeeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.True(disabled, "disabled distributions should be set to true")

	// all recipients should have zero balance because the distribution failed.
	for _, recipient := range recipients {
		recipientBalance := bankKeeper.GetBalance(ctx, sdk.MustAccAddressFromBech32(recipient), bondDenom)
		requireT.True(recipientBalance.Amount.IsZero(),
			"recipient %s should have zero balance because the distribution failed", recipient)
	}

	// Verify clearing accounts balances are unchanged
	for _, mapping := range mappings {
		// we did not fund team clearing account, so it should have zero balance.
		if mapping.ClearingAccount == types.ClearingAccountTeam {
			continue
		}
		moduleAddr := testApp.AccountKeeper.GetModuleAddress(mapping.ClearingAccount)
		moduleBalance := bankKeeper.GetBalance(ctx, moduleAddr, bondDenom)
		requireT.True(moduleBalance.Amount.IsPositive(),
			"clearing account %s should have positive balance after distribution", mapping.ClearingAccount)
	}
}

// TestEdgeCase_ExcludedAddressDuringPhase1 verifies that when an excluded address changes
// delegation while Phase 1 (score conversion) is in progress, the migrateOngoingEntry hook:
//   - Splits the score at the distribution timestamp (ongoing + post-dist portions)
//   - Routes both score portions to ExcludedAddressScore (NOT AccountScoreSnapshot)
//   - Removes the entry from ongoingID (prevents double-counting by Phase 1 batch)
//   - Creates new entry under nextID with updated shares
//   - Phase 1 batch does not produce any AccountScoreSnapshot for the excluded address
//   - Phase 2 distributes nothing to the excluded address
func TestEdgeCase_ExcludedAddressDuringPhase1(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	r := &runEnv{
		testApp:       testApp,
		ctx:           ctx,
		requireT:      requireT,
		currentDistID: firstDistributionID,
	}

	// Add validators.
	for range 3 {
		validatorOperator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, validatorOperator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		validator, err := testApp.AddValidator(
			ctx, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil,
		)
		requireT.NoError(err)
		r.validators = append(r.validators, sdk.MustValAddressFromBech32(validator.GetOperator()))
	}

	// Add delegators: [0] = excluded, [1] = normal (for reward comparison).
	for range 3 {
		delegator, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(
			ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
		))
		r.delegators = append(r.delegators, delegator)
	}

	err = testApp.PSEKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: firstDistributionID, Timestamp: firstDistributionID},
	})
	requireT.NoError(err)

	// T=0s: Both delegators delegate.
	delegateAction(r, r.delegators[0], r.validators[0], 1_100_000)
	delegateAction(r, r.delegators[1], r.validators[0], 900_000)

	// Exclude delegators[0] before any score accumulation (ExcludedAddressScore=0, entries kept).
	err = testApp.PSEKeeper.UpdateExcludedAddresses(r.ctx, authority, []string{r.delegators[0].String()}, nil)
	requireT.NoError(err)

	// T=8s: Advance time for score accumulation.
	waitAction(r, time.Second*8)

	// Start ongoing distribution (sets OngoingDistribution with distTimestamp=now).
	// Both delegators have entries under ongoingID=1.
	startOngoingDistributionAction(r, sdkmath.NewInt(1000))
	distTimestamp := r.ctx.BlockTime().Unix()

	// Excluded delegators[0] changes delegation BEFORE Phase 1 runs any batches.
	// migrateOngoingEntry should fire with isExcluded=true:
	//   - ongoingScore = 1,100,000 × 8s = 8,800,000 -> ExcludedAddressScore
	//   - nextScore = 1,100,000 × 0s = 0 (distTimestamp == blockTime) -> ExcludedAddressScore
	//   - entry removed from ongoingID=1, new entry under nextID=2
	delegateAction(r, r.delegators[0], r.validators[0], 50_000)

	// Verify: delegators[0] entry removed from ongoingID=1.
	assertEntryNotExistsUnderDistIDAction(r, r.currentDistID, r.delegators[0], r.validators[0])

	// Verify: delegators[0] entry exists under nextID=2.
	nextID := r.currentDistID + 1
	assertEntryExistsUnderDistIDAction(r, nextID, r.delegators[0], r.validators[0])

	// Verify: scores went to ExcludedAddressScore, NOT AccountScoreSnapshot.
	excludedScore, err := testApp.PSEKeeper.ExcludedAddressScore.Get(r.ctx, r.delegators[0])
	requireT.NoError(err)
	// ongoingScore = 1,100,000 tokens × 8s = 8,800,000
	// nextScore = 1,100,000 tokens × (blockTime - distTimestamp) = 0 (same time)
	requireT.Equal(sdkmath.NewInt(1_100_000*8), excludedScore,
		"migrateOngoingEntry should route scores to ExcludedAddressScore for excluded address")
	_ = distTimestamp // used in comment above

	// Verify: NO AccountScoreSnapshot for delegators[0] under ongoingID.
	_, err = testApp.PSEKeeper.GetDelegatorScore(r.ctx, r.currentDistID, r.delegators[0])
	requireT.ErrorIs(err, collections.ErrNotFound,
		"excluded address should have no AccountScoreSnapshot entry")

	// Verify: delegators[1] entry still under ongoingID=1 (no delegation change, no hook fired).
	assertEntryExistsUnderDistIDAction(r, r.currentDistID, r.delegators[1], r.validators[0])

	// Run Phase 1 to completion to process remaining entries.
	for {
		done := runPhase1BatchAction(r)
		if done {
			break
		}
	}

	// Verify: delegators[1] has score from Phase 1 batch (normal address).
	score1, err := testApp.PSEKeeper.GetDelegatorScore(r.ctx, r.currentDistID, r.delegators[1])
	requireT.NoError(err)
	requireT.True(score1.IsPositive(), "delegators[1] should have score from Phase 1 batch")

	// Verify: delegators[0] STILL has no AccountScoreSnapshot after Phase 1 batch.
	// (Entry was removed from ongoingID by migrateOngoingEntry, so Phase 1 batch can't process it.)
	_, err = testApp.PSEKeeper.GetDelegatorScore(r.ctx, r.currentDistID, r.delegators[0])
	requireT.ErrorIs(err, collections.ErrNotFound,
		"excluded address should have no AccountScoreSnapshot even after Phase 1 completes")

	// Verify: ExcludedAddressScore unchanged after Phase 1 (Phase 1 doesn't touch it again).
	excludedScoreAfterPhase1, err := testApp.PSEKeeper.ExcludedAddressScore.Get(r.ctx, r.delegators[0])
	requireT.NoError(err)
	requireT.Equal(excludedScore, excludedScoreAfterPhase1,
		"ExcludedAddressScore should not change during Phase 1 batch processing")

	// Finish distribution (Phase 2 + cleanup).
	finishDistributionAction(r)

	// Verify: delegators[1] (normal) got rewards, delegators[0] (excluded) got nothing.
	// delegators[1]: 900,000 original + reward share (genesis validator also has score).
	assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
		&r.delegators[0]: sdkmath.NewInt(1_150_000), // 1,100,000 + 50,000 delegation only, no PSE reward
		&r.delegators[1]: sdkmath.NewInt(900_473),   // 900,000 + reward auto-delegated
	})
}
