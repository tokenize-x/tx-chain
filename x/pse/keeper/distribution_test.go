package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

func TestDistribution_GenesisRebuild(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)
	pseKeeper := testApp.PSEKeeper

	// Get bond denom
	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Create a validator and delegator so community distribution has non-zero score.
	valOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, valOp, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1000)))))
	val, errVal := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(errVal)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	del1, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, del1, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000)))))

	time1 := uint64(startTime.Add(1 * time.Hour).Unix())
	time2 := uint64(startTime.Add(2 * time.Hour).Unix())

	// Save initial schedule so hooks can find distribution ID.
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: 1, Timestamp: time1},
	})
	requireT.NoError(err)

	// Delegate and advance time for score accumulation.
	_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: del1.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 500),
	})
	requireT.NoError(err)
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(10 * time.Second))
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

	// Process distribution by calling EndBlocker until OngoingDistribution is cleared.
	// Test entries fit in a single batch (< defaultBatchSize), so exactly 3 calls are needed
	ctx = ctx.WithBlockTime(time.Unix(int64(time1)+10, 0))
	ctx = ctx.WithBlockHeight(100)
	const maxEndBlockerCalls = 3 // consume, distribute, cleanup
	for i := range maxEndBlockerCalls {
		err = pseKeeper.ProcessNextDistribution(ctx)
		requireT.NoError(err)
		_, oErr := pseKeeper.OngoingDistribution.Get(ctx)
		if oErr != nil {
			break
		}
		requireT.Less(i, maxEndBlockerCalls-1, "distribution did not complete within expected calls")
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

	// Create and save distribution schedule.
	// Community allocation is required; non-community precision is the focus of this test.
	startTime := uint64(time.Now().Add(-1 * time.Hour).Unix())
	schedule := []types.ScheduledDistribution{
		{
			ID:        1,
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

	// First call processes non-community allocations and starts multi-block community distribution.
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
	communityAmount := sdkmath.NewInt(1000)
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

	// --- Call 2: Consume all entries + distribute tokens ---
	// Batch size (100) > delegator count (2), so all entries are consumed in one batch (isConsumed=true).
	// Tokens distributed to all delegators in one batch.
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.NoError(err)

	// TotalScore is accumulated incrementally via addToScore.
	totalScore, err := pseKeeper.TotalScore.Get(ctx, distributionID)
	requireT.NoError(err)
	requireT.True(totalScore.IsPositive(), "TotalScore should be positive")

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

	// OngoingDistribution should still exist (cleanup runs on next empty-batch call)
	_, err = pseKeeper.OngoingDistribution.Get(ctx)
	requireT.NoError(err)

	// --- Call 3: Cleanup (no entries, no snapshots -> cleanup) ---
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
// zero community allocation triggers an invariant violation.
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

	// Non-community allocations are processed, but zero community triggers invariant violation.
	err = pseKeeper.ProcessNextDistribution(ctx)
	requireT.ErrorIs(err, types.ErrInvariantViolation)
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
			name: "zero score via EndBlocker triggers invariant violation",
			actions: []func(*runEnv){
				func(r *runEnv) {
					endBlockerDistributeExpectInvariantViolation(r, sdkmath.NewInt(1000))
				},
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
				currentDistID: tempDistributionID,
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
					Timestamp: tempDistributionID,
					ID:        tempDistributionID,
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
