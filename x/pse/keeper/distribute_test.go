package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v7/x/pse/keeper"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

func TestKeeper_Distribute(t *testing.T) {
	cases := []struct {
		name    string
		actions []func(*runEnv)
	}{
		{
			name: "test unaccumulated score",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_366), // + 1000 * 1.1 / 3
						&r.delegators[1]: sdkmath.NewInt(900_299),   // + 1000 * 0.9 / 3
					})
				},
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "test accumulated score + unaccumulated score",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 900_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 1_100_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(2_000_387), // + (1100 * 8 + 2000 * 8) / 64
						&r.delegators[1]: sdkmath.NewInt(2_000_362), // + (900 * 8 + 2000 * 8) / 64
					})
				},
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "test accumulated score + unaccumulated score + multiple validators",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[1], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 900_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[1], 900_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[1], 1_100_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(4_000_442), // + (1100 * 8 + 2000 * 8) * 2 / 112
						&r.delegators[1]: sdkmath.NewInt(4_000_414), // + (900 * 8 + 2000 * 8) * 2 / 112
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "test unbonding delegation",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[0], 900_000) },
				func(r *runEnv) { undelegateAction(r, r.delegators[1], r.validators[0], 700_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(200_295), // + (1100 * 8 + 200 * 8) / 35.2
						&r.delegators[1]: sdkmath.NewInt(200_249), // + (900 * 8 + 200 * 8) / 35.2
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "test redelegation",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { redelegateAction(r, r.delegators[0], r.validators[0], r.validators[2], 900_000) },
				func(r *runEnv) { redelegateAction(r, r.delegators[1], r.validators[0], r.validators[2], 700_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_365), // + 1000 * 1.1 / 3
						&r.delegators[1]: sdkmath.NewInt(900_298),   // + 1000 * 0.9 / 3
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "test no delegation with scoring user",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { undelegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					// delegators[0] fully undelegated — no active delegations, tokens go to community pool
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(0),       // no active delegation -> tokens go to community pool
						&r.delegators[1]: sdkmath.NewInt(900_299), // 900k original + 1000 * 0.9 / 2.4 ≈ 299 auto-delegated
					})
					// delegators[0] only has original 1000 funded amount (no PSE reward)
					balance := r.testApp.BankKeeper.GetBalance(r.ctx, r.delegators[0], sdk.DefaultBondDenom)
					r.requireT.Equal(sdkmath.NewInt(1000), balance.Amount)
				},
				// delegators[0]'s share (366) + rounding (2) goes to community pool
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(368)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "no eligible recipients finalizes to community pool",
			actions: []func(*runEnv){
				func(r *runEnv) {
					distributeExpectFinalizeToCommunityPool(r, sdkmath.NewInt(1000))
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(1000)) },
			},
		},
		{
			name: "test multiple distributions",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_366), // + 1000 * 1.1 / 3
						&r.delegators[1]: sdkmath.NewInt(900_299),   // + 1000 * 0.9 / 3
					})
				},
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_732), // + 366 * 2
						&r.delegators[1]: sdkmath.NewInt(900_598),   // + 299 * 2
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
					Timestamp: uint64(ctx.BlockTime().Unix()),
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

// Test_ExcludedAddress_FullLifecycle validates the complete lifecycle of excluded addresses.
func Test_ExcludedAddress_FullLifecycle(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper
	queryService := keeper.NewQueryService(pseKeeper)

	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	// Create validator
	valOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, valOp, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1_000))),
	))
	val, err := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	// Create delegator
	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, delAddr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1_000))),
	))

	distributionID := firstDistributionID

	// Step 1: Address accumulates score - delegate and wait for score to build up
	msg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 100),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msg)
	requireT.NoError(err)

	// Advance time to accumulate score
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(10 * time.Second))

	// Trigger score calculation by making another delegation change
	msg2 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 1),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msg2)
	requireT.NoError(err)

	// Verify score accumulated (should be positive)
	resp1, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	scoreBeforeExclusion := resp1.Score
	requireT.True(scoreBeforeExclusion.IsPositive(), "Score should be positive after delegation and time passing")
	t.Logf("Score after 10 seconds: %s", scoreBeforeExclusion.String())

	// Step 2: Address added to excluded_list
	err = pseKeeper.UpdateExcludedAddresses(ctx, authority, []string{delAddr.String()}, nil)
	requireT.NoError(err)

	// Step 3: Verify exclusion impact - score snapshot cleared and DelegationTimeEntry removed
	_, err = queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	// Note: Query returns zero score when no snapshot exists, not an error

	// Verify delegation still exists
	delegation, err := stakingKeeper.GetDelegation(ctx, delAddr, valAddr)
	requireT.NoError(err)
	requireT.NotNil(delegation, "Delegation should still exist")

	// Verify DelegationTimeEntry was removed
	_, err = pseKeeper.GetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr)
	requireT.ErrorIs(err, collections.ErrNotFound, "DelegationTimeEntry should be removed for excluded address")

	// Step 4: Make delegation change while excluded - should NOT accumulate score
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(5 * time.Second))
	msg3 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 1),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msg3)
	requireT.NoError(err)

	// Verify still no score
	resp, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	requireT.True(resp.Score.IsZero(), "Excluded address should still have zero score after delegation change")

	// Step 5: Run distribution while address is excluded - should receive nothing
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)
	amount := sdkmath.NewInt(1_000)
	macc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, sdk.NewCoins(sdk.NewCoin(bondDenom, amount))))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc.GetName(), sdk.NewCoins(sdk.NewCoin(bondDenom, amount)),
	))
	scheduledDistribution := types.ScheduledDistribution{
		ID:        distributionID,
		Timestamp: uint64(ctx.BlockTime().Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          amount,
		}},
	}
	err = pseKeeper.BeginCommunityDistribution(ctx, scheduledDistribution, bondDenom)
	requireT.NoError(err)
	balanceBefore := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom)
	for {
		done, err := pseKeeper.ConsumeOngoingDelegationTimeEntries(ctx, scheduledDistribution)
		requireT.NoError(err)
		if done {
			break
		}
	}
	for {
		done, err := pseKeeper.ProcessOngoingTokenDistribution(ctx, scheduledDistribution, bondDenom)
		requireT.NoError(err)
		if done {
			break
		}
	}
	balanceAfter := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom)
	requireT.Equal(
		balanceBefore.Amount.String(), balanceAfter.Amount.String(),
		"Excluded address should receive no rewards",
	)

	// ExcludedAddressScore must be purged after distribution cleanup.
	_, err = pseKeeper.ExcludedAddressScore.Get(ctx, delAddr)
	requireT.ErrorIs(err, collections.ErrNotFound,
		"ExcludedAddressScore must be purged after distribution — each period is isolated")

	// DelegationTimeEntry must survive cleanup — migrated to nextID with reset timestamp.
	nextID := distributionID + 1
	entry, err := pseKeeper.GetDelegationTimeEntry(ctx, nextID, valAddr, delAddr)
	requireT.NoError(err, "DelegationTimeEntry must be migrated to nextID after distribution")
	requireT.Equal(ctx.BlockTime().Unix(), entry.LastChangedUnixSec,
		"Migrated entry must have reset timestamp")

	// After distribution, entries migrated from distributionID to distributionID+1.
	// Save a new schedule so hooks and UpdateExcludedAddresses can find it.
	distributionID++
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{Timestamp: uint64(ctx.BlockTime().Unix()), ID: distributionID},
	})
	requireT.NoError(err)

	// Step 6: Verify excluded delegator can fully undelegate after distribution
	msgUndel := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 102), // full amount (100 + 1 + 1 from earlier)
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Undelegate(ctx, msgUndel)
	requireT.NoError(err, "Excluded delegator should be able to fully undelegate after distribution")

	// Step 7: Re-delegate while still excluded and accumulate excluded score in the new period.
	requireT.NoError(testApp.BankKeeper.MintCoins(
		ctx, minttypes.ModuleName, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(200))),
	))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToAccount(
		ctx, minttypes.ModuleName, delAddr, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(200))),
	))
	msgDelegate := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 50),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msgDelegate)
	requireT.NoError(err)

	// Advance time and trigger another delegation change to accumulate excluded score in the new period.
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(5 * time.Second))
	msgDelegate2 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 1),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msgDelegate2)
	requireT.NoError(err)

	// Verify fresh excluded score accumulated in the new period.
	currentPeriodExcludedScore, err := pseKeeper.ExcludedAddressScore.Get(ctx, delAddr)
	requireT.NoError(err)
	// 50 tokens * 5 seconds = 250.
	requireT.Equal(sdkmath.NewInt(250), currentPeriodExcludedScore,
		"Excluded score must be fresh for new period, not carried over from Distribution 1")

	// Step 8: Remove from exclude_list (re-include)
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(1 * time.Second))
	err = pseKeeper.UpdateExcludedAddresses(ctx, authority, nil, []string{delAddr.String()})
	requireT.NoError(err)

	// Verify DelegationTimeEntry was recreated with current state
	entry, err = pseKeeper.GetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr)
	requireT.NoError(err, "DelegationTimeEntry should be recreated on re-inclusion")
	requireT.Equal(ctx.BlockTime().Unix(), entry.LastChangedUnixSec, "Entry should have current block time")

	// Verify restored score is the fresh score from the current period only.
	restoredSnapshot, err := pseKeeper.GetDelegatorScore(ctx, distributionID, delAddr)
	requireT.NoError(err)
	requireT.Equal(currentPeriodExcludedScore, restoredSnapshot,
		"restored score must equal fresh excluded score from current period only")

	_, err = pseKeeper.GetDelegatorScore(ctx, firstDistributionID, delAddr)
	requireT.ErrorIs(err, collections.ErrNotFound,
		"score must not exist at the already-processed distID=1 after re-inclusion")

	// ExcludedAddressScore must be fully cleared after re-inclusion.
	_, err = pseKeeper.ExcludedAddressScore.Get(ctx, delAddr)
	requireT.ErrorIs(err, collections.ErrNotFound,
		"ExcludedAddressScore must be cleared after re-inclusion")

	// TotalScore[distributionID] must include the restored score.
	totalScoreAtReinclusion, err := pseKeeper.TotalScore.Get(ctx, distributionID)
	requireT.NoError(err)
	requireT.Equal(restoredSnapshot, totalScoreAtReinclusion,
		"TotalScore must equal the restored excluded score after re-inclusion")

	// Step 9: Score after re-inclusion = restored excluded score + new accumulation.
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(3 * time.Second))

	// Query score directly - no delegation needed because DelegationTimeEntry exists
	resp2, err := queryService.Score(ctx, &types.QueryScoreRequest{
		Address: delAddr.String(),
	})
	requireT.NoError(err)
	scoreAfterReinclusion := resp2.Score
	requireT.True(scoreAfterReinclusion.IsPositive(), "Score should be positive after re-inclusion")
	// currentPeriodExcludedScore (250) + fresh accumulation (51 tokens * 3s = 153) = 403
	requireT.True(scoreAfterReinclusion.GT(currentPeriodExcludedScore),
		"Score after re-inclusion should exceed restored score (restored + new accumulation)")
	t.Logf("Score after re-inclusion and 3 seconds: %s (restored was %s)",
		scoreAfterReinclusion.String(), currentPeriodExcludedScore.String())

	// Step 10: Run Distribution 2 and verify the re-included delegator receives proportional rewards.
	t.Log("=== Distribution 2: re-included delegator receives rewards ===")

	// Use a large enough amount so that delegator's share won't be downed to zero
	// due to genesis/simapp delegators huge shares.
	amount2 := sdkmath.NewInt(10_000_000)
	macc2 := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(
		ctx, minttypes.ModuleName, sdk.NewCoins(sdk.NewCoin(bondDenom, amount2)),
	))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc2.GetName(), sdk.NewCoins(sdk.NewCoin(bondDenom, amount2)),
	))

	scheduledDist2 := types.ScheduledDistribution{
		ID:        distributionID,
		Timestamp: uint64(ctx.BlockTime().Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          amount2,
		}},
	}
	requireT.NoError(pseKeeper.BeginCommunityDistribution(ctx, scheduledDist2, bondDenom))

	// Capture delegation before Distribution 2.
	delDelegBefore2 := sdkmath.NewInt(0)
	{
		q := stakingkeeper.NewQuerier(stakingKeeper)
		resp, err2 := q.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
			DelegatorAddr: delAddr.String(),
		})
		requireT.NoError(err2)
		for _, d := range resp.DelegationResponses {
			delDelegBefore2 = delDelegBefore2.Add(d.Balance.Amount)
		}
	}

	for {
		done, err2 := pseKeeper.ConsumeOngoingDelegationTimeEntries(ctx, scheduledDist2)
		requireT.NoError(err2)
		if done {
			break
		}
	}
	for {
		done, err2 := pseKeeper.ProcessOngoingTokenDistribution(ctx, scheduledDist2, bondDenom)
		requireT.NoError(err2)
		if done {
			break
		}
	}

	// Re-included delegator must receive auto-staked rewards in Distribution 2.
	delDelegAfter2 := sdkmath.NewInt(0)
	{
		q := stakingkeeper.NewQuerier(stakingKeeper)
		resp, err2 := q.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
			DelegatorAddr: delAddr.String(),
		})
		requireT.NoError(err2)
		for _, d := range resp.DelegationResponses {
			delDelegAfter2 = delDelegAfter2.Add(d.Balance.Amount)
		}
	}
	requireT.True(delDelegAfter2.GT(delDelegBefore2),
		"re-included delegator must receive auto-staked rewards in Distribution 2 (got %s, had %s)",
		delDelegAfter2.String(), delDelegBefore2.String())

	// LastProcessedDistributionID advances to distributionID after Distribution 2 completes.
	lastProcessed2, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(distributionID, lastProcessed2,
		"LastProcessedDistributionID must advance to distributionID=%d after Distribution 2", distributionID)
}

// TestDistribution_FairnessBonus verifies that distribution adds a fairness bonus score to
// AccountScoreSnapshot[nextID] proportional to the distributed amount and elapsed time since
// the distribution started.
// formula: bonusScore = distributedAmount * (blockTime - StartedAt)
func TestDistribution_FairnessBonus(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	now := time.Now().Round(time.Second)
	ctx := testApp.NewContext(false).WithBlockTime(now)
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Create validator and delegator.
	valOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, valOp, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))
	val, err := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delAddr, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))

	distributionID := firstDistributionID

	// Delegate and accumulate score over 10 seconds.
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 500),
	})
	requireT.NoError(err)
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(10 * time.Second))
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 1),
	})
	requireT.NoError(err)

	// Save schedule so hooks can find distribution ID.
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: distributionID, Timestamp: uint64(ctx.BlockTime().Unix())},
	})
	requireT.NoError(err)

	// Fund community clearing account with large amount to prevent integer division to zero.
	communityAmount := sdkmath.NewInt(10_000_000)
	communityCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, communityAmount))
	macc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, communityCoins))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc.GetName(), communityCoins,
	))

	// D = simulated elapsed seconds since distribution started.
	const D = int64(7)
	blockTime := ctx.BlockTime().Unix()

	// distTimestamp == blockTime so Phase 1 gap score = shares * (blockTime - distTimestamp) = 0.
	// StartedAt is D seconds before blockTime so processingElapsedSec = D.
	scheduledDistribution := types.ScheduledDistribution{
		ID:        distributionID,
		Timestamp: uint64(blockTime),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          communityAmount,
		}},
		StartedAt: blockTime - D,
	}
	requireT.NoError(pseKeeper.BeginCommunityDistribution(ctx, scheduledDistribution, bondDenom))

	// Phase 1: convert DelegationTimeEntries to score snapshots.
	for {
		done, err := pseKeeper.ConsumeOngoingDelegationTimeEntries(ctx, scheduledDistribution)
		requireT.NoError(err)
		if done {
			break
		}
	}

	// Capture TotalScore and delegator score before Phase 2 (cleanup removes these).
	totalScore, err := pseKeeper.TotalScore.Get(ctx, distributionID)
	requireT.NoError(err)
	requireT.True(totalScore.IsPositive(), "TotalScore must be positive after Phase 1")

	delegatorScore, err := pseKeeper.GetDelegatorScore(ctx, distributionID, delAddr)
	requireT.NoError(err)
	requireT.True(delegatorScore.IsPositive(), "delegator score must be positive after Phase 1")

	// expectedUserAmount = communityAmount * delegatorScore / totalScore (same formula as Phase 2).
	expectedUserAmount := communityAmount.Mul(delegatorScore).Quo(totalScore)
	requireT.True(expectedUserAmount.IsPositive(),
		"expectedUserAmount must be positive (increase communityAmount if this fails); "+
			"delegatorScore=%s totalScore=%s", delegatorScore, totalScore)
	expectedBonus := expectedUserAmount.MulRaw(D)

	// Phase 2: distribute tokens and add fairness bonus to nextID snapshot.
	for {
		done, err := pseKeeper.ProcessOngoingTokenDistribution(ctx, scheduledDistribution, bondDenom)
		requireT.NoError(err)
		if done {
			break
		}
	}

	// AccountScoreSnapshot[nextID][delAddr] must equal the fairness bonus.
	// Gap score = 0 (distTimestamp == blockTime), hook score from auto-delegate = 0.
	// Only the added fairness bonus contributes in next ID score.
	nextID := distributionID + 1
	bonusScore, err := pseKeeper.GetDelegatorScore(ctx, nextID, delAddr)
	requireT.NoError(err)
	requireT.Equal(expectedBonus.String(), bonusScore.String(),
		"fairness bonus in AccountScoreSnapshot[nextID] must equal distributedAmount * processingElapsedSec")

	t.Logf("distributedAmount=%s D=%d bonusScore=%s", expectedUserAmount, D, bonusScore)
}

// TestDistribution_FairnessBonus_SkippedWhenStartedAtZero verifies that no fairness bonus
// is added when StartedAt is zero. This preserves backward compatibility for distributions
// that predate the StartedAt field.
func TestDistribution_FairnessBonus_SkippedWhenStartedAtZero(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now().Round(time.Second))
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	valOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, valOp, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))
	val, err := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delAddr, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))

	distributionID := firstDistributionID

	// Delegate and accumulate score.
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 500),
	})
	requireT.NoError(err)
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(10 * time.Second))
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 1),
	})
	requireT.NoError(err)

	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{ID: distributionID, Timestamp: uint64(ctx.BlockTime().Unix())},
	})
	requireT.NoError(err)

	communityAmount := sdkmath.NewInt(10_000_000)
	communityCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, communityAmount))
	macc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, communityCoins))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc.GetName(), communityCoins,
	))

	// StartedAt = 0: fairness bonus must be skipped (backward-compatible default).
	scheduledDistribution := types.ScheduledDistribution{
		ID:        distributionID,
		Timestamp: uint64(ctx.BlockTime().Unix()), // distTimestamp == blockTime -> gap score = 0
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          communityAmount,
		}},
		StartedAt: 0,
	}
	requireT.NoError(pseKeeper.BeginCommunityDistribution(ctx, scheduledDistribution, bondDenom))

	for {
		done, err := pseKeeper.ConsumeOngoingDelegationTimeEntries(ctx, scheduledDistribution)
		requireT.NoError(err)
		if done {
			break
		}
	}
	for {
		done, err := pseKeeper.ProcessOngoingTokenDistribution(ctx, scheduledDistribution, bondDenom)
		requireT.NoError(err)
		if done {
			break
		}
	}

	// No bonus -> no AccountScoreSnapshot entry at nextID for this delegator.
	// Gap score = 0 (distTimestamp == blockTime), hook score from auto-delegate = 0.
	nextID := distributionID + 1
	_, err = pseKeeper.GetDelegatorScore(ctx, nextID, delAddr)
	requireT.ErrorIs(err, collections.ErrNotFound,
		"no fairness bonus must be added when StartedAt is zero")
}

// TestCommunityIntermediary_AccountInitialized verifies that InitCommunityIntermediary creates the
// pse_community_intermediary module account in state.
func TestCommunityIntermediary_AccountInitialized(t *testing.T) {
	requireT := require.New(t)
	startTime := time.Now()
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	testApp.PSEKeeper.InitCommunityIntermediary(ctx)

	acc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunityIntermediary)
	requireT.NotNil(acc, "pse_community_intermediary module account must exist after InitCommunityIntermediary")
	requireT.Equal(types.ClearingAccountCommunityIntermediary, acc.GetName())
}

// TestCommunityIntermediary_FundIsolation is the core safety test for per-distribution intermediary.
// Verifies that only the current round's funds are at risk during distribution.
// The remaining pse_community balance (future rounds) must never be touched.
// Scenario:
//   - pse_community holds 3x communityAmount
//   - BeginCommunityDistribution moves exactly 1x into pse_community_intermediary
//   - After full distribution: intermediary is drained to zero, pse_community still holds 2x
func TestCommunityIntermediary_FundIsolation(t *testing.T) {
	requireT := require.New(t)
	startTime := time.Now()
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Set up a validator and delegator.
	validatorOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, validatorOp,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000))),
	))
	validator, err := testApp.AddValidator(ctx, validatorOp, sdk.NewInt64Coin(bondDenom, 500_000), nil)
	requireT.NoError(err)

	delegator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delegator,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(500_000))),
	))
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator.OperatorAddress,
		Amount:           sdk.NewInt64Coin(bondDenom, 400_000),
	})
	requireT.NoError(err)

	// Mint 3x communityAmount into pse_community to simulate a multi-round treasury.
	communityAmount := sdkmath.NewInt(10_000_000)
	treasuryTotal := communityAmount.MulRaw(3)
	macc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName,
		sdk.NewCoins(sdk.NewCoin(bondDenom, treasuryTotal)),
	))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc.GetName(), sdk.NewCoins(sdk.NewCoin(bondDenom, treasuryTotal)),
	))

	communityAddr := testApp.AccountKeeper.GetModuleAddress(types.ClearingAccountCommunity)
	intermediaryAddr := testApp.AccountKeeper.GetModuleAddress(types.ClearingAccountCommunityIntermediary)

	// Sanity: full treasury in pse_community, intermediary empty before distribution starts.
	requireT.Equal(treasuryTotal, testApp.BankKeeper.GetBalance(ctx, communityAddr, bondDenom).Amount)
	requireT.True(testApp.BankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount.IsZero())

	// Start distribution — only 1× moves from pse_community into the intermediary.
	// Timestamp is set 10 seconds after the delegation so score = shares * 10 > 0.
	const distributionID = uint64(1)
	scheduledDistribution := types.ScheduledDistribution{
		ID:        distributionID,
		Timestamp: uint64(startTime.Add(10 * time.Second).Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          communityAmount,
		}},
	}
	requireT.NoError(pseKeeper.BeginCommunityDistribution(ctx, scheduledDistribution, bondDenom))

	remaining := treasuryTotal.Sub(communityAmount)
	requireT.Equal(remaining, testApp.BankKeeper.GetBalance(ctx, communityAddr, bondDenom).Amount,
		"pse_community must retain funds for future rounds — only this round's amount must leave")
	requireT.Equal(communityAmount, testApp.BankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount,
		"pse_community_intermediary must hold exactly this round's funds")

	// Run Phase 1.
	for {
		done, err := pseKeeper.ConsumeOngoingDelegationTimeEntries(ctx, scheduledDistribution)
		requireT.NoError(err)
		if done {
			break
		}
	}

	// Run Phase 2 to completion.
	for {
		done, err := pseKeeper.ProcessOngoingTokenDistribution(ctx, scheduledDistribution, bondDenom)
		requireT.NoError(err)
		if done {
			break
		}
	}

	// After full distribution: intermediary is drained, pse_community is untouched.
	requireT.True(testApp.BankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount.IsZero(),
		"pse_community_intermediary must be fully drained after distribution completes")
	requireT.Equal(remaining, testApp.BankKeeper.GetBalance(ctx, communityAddr, bondDenom).Amount,
		"pse_community must still hold future rounds' funds untouched after distribution")
}

// TestDistribution_SlashedValidator_RedirectsRewardToHealthy verifies that when a delegator has
// stake on one healthy and one fully-slashed validator, PSE Phase 2 skips the slashed validator
// and routes the delegator's full reward to the healthy validator(s) instead. Without the fix,
// Phase 2 calls Delegate(slashedVal, 0) which returns ErrDelegatorShareExRateInvalid and disables PSE.
func TestDistribution_SlashedValidator_RedirectsRewardToHealthy(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	// Use explicit timing so Phase 1 produces a positive score.
	t0 := time.Now().UTC().Round(time.Second)
	ctx, _, err := testApp.BeginNextBlockAtTime(t0)
	requireT.NoError(err)
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Two validators (slashed + healthy) and a delegator with stake on both.
	makeValidator := func() sdk.ValAddress {
		op, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(ctx, op, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))
		val, err := testApp.AddValidator(ctx, op, sdk.NewInt64Coin(bondDenom, 10), nil)
		requireT.NoError(err)
		return sdk.MustValAddressFromBech32(val.GetOperator())
	}
	valSlashedAddr := makeValidator()
	valHealthyAddr := makeValidator()

	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delAddr, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000)))))

	const delegationAmt = int64(1_000)
	delegate := func(val sdk.ValAddress, amt int64) {
		_, err := stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
			DelegatorAddress: delAddr.String(),
			ValidatorAddress: val.String(),
			Amount:           sdk.NewInt64Coin(bondDenom, amt),
		})
		requireT.NoError(err)
	}
	delegate(valSlashedAddr, delegationAmt)
	delegate(valHealthyAddr, delegationAmt)

	// Schedule timestamp 5 seconds after delegations so Phase 1 produces score.
	scheduleTimestamp := t0.Add(5 * time.Second)
	requireT.NoError(pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{{
		ID:        firstDistributionID,
		Timestamp: uint64(scheduleTimestamp.Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          sdkmath.NewInt(10_000_000),
		}},
	}}))

	// Post-100%-slash state on valSlashed: Tokens=0, DelegatorShares unchanged.
	valSlashed, err := stakingKeeper.GetValidator(ctx, valSlashedAddr)
	requireT.NoError(err)
	valSlashed.Tokens = sdkmath.ZeroInt()
	requireT.NoError(stakingKeeper.SetValidator(ctx, valSlashed))

	// Fund the community clearing account.
	communityCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000_000)))
	macc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, communityCoins))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc.GetName(), communityCoins,
	))

	// Capture wallet + healthy delegation balances right before the distribution starts.
	walletBefore := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount
	healthyBefore := func() sdkmath.Int {
		d, derr := stakingKeeper.GetDelegation(ctx, delAddr, valHealthyAddr)
		requireT.NoError(derr)
		val, verr := stakingKeeper.GetValidator(ctx, valHealthyAddr)
		requireT.NoError(verr)
		return val.TokensFromShares(d.Shares).TruncateInt()
	}()

	// Trigger block at t0+10s which will set OngoingDistribution.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(10 * time.Second)))

	// Phase 1 batch + Phase 2 first batch (pays the delegator).
	// Slashed validator should be skipped and entire userAmount should be paid to the healthy one.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(15 * time.Second)))

	// Phase 2 second batch is empty. cleanup -> lastProcessed advances.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(20 * time.Second)))

	ctx, _, err = testApp.BeginNextBlockAtTime(t0.Add(25 * time.Second))
	requireT.NoError(err)

	// PSE must NOT be disabled — the fix kept the failing path from firing.
	disabled, err := pseKeeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.False(disabled, "PSE must not be disabled — the slashed validator's slice was skipped")

	// LastProcessedDistributionID must have advanced — distribution finalized successfully.
	lastProcessed, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(firstDistributionID, lastProcessed, "distribution must have finalized")

	// Verify the slashed validator's slice was fully redirected to the healthy validator:
	walletAfter := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount
	walletLeftover := walletAfter.Sub(walletBefore)
	requireT.True(walletLeftover.LTE(sdkmath.NewInt(1)),
		"wallet leftover=%s exceeds 1 subunit — reward was not fully auto-delegated to the healthy validator",
		walletLeftover)

	stakingQuerier := stakingkeeper.NewQuerier(stakingKeeper)
	delResp, err := stakingQuerier.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delAddr.String(),
	})
	requireT.NoError(err)
	requireT.Len(delResp.DelegationResponses, 2)
	for _, d := range delResp.DelegationResponses {
		switch d.Delegation.ValidatorAddress {
		case valHealthyAddr.String():
			healthyGrowth := d.Balance.Amount.Sub(healthyBefore)
			requireT.True(healthyGrowth.IsPositive(),
				"healthy validator delegation must have grown from auto-delegate (got growth=%s)", healthyGrowth)
		case valSlashedAddr.String():
			requireT.True(d.Balance.Amount.IsZero(),
				"slashed validator delegation Balance must remain zero (validator.Tokens=0)")
		}
	}
}

// TestProcessOngoingTokenDistribution_ErrorContext asserts that Phase 2
// failures surface the full wrap chain (delegator, distID, score, amounts)
// in the returned error.
func TestProcessOngoingTokenDistribution_ErrorContext(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false).WithBlockTime(time.Now().Round(time.Second))
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	valOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, valOp, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000)))))
	val, err := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(bondDenom, 100_000), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delAddr, sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000_000)))))
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 100_000),
	})
	requireT.NoError(err)

	// Allocation larger than the intermediary's funded balance → Phase 2 fails.
	const distID = uint64(1)
	allocationAmount := sdkmath.NewInt(1_000_000)
	ongoing := types.ScheduledDistribution{
		ID:        distID,
		Timestamp: uint64(ctx.BlockTime().Unix()),
		Allocations: []types.ClearingAccountAllocation{
			{ClearingAccount: types.ClearingAccountCommunity, Amount: allocationAmount},
		},
	}
	requireT.NoError(pseKeeper.OngoingDistribution.Set(ctx, ongoing))

	// One entry so the single delegator's userAmount equals the full allocation.
	score := sdkmath.NewInt(100)
	requireT.NoError(pseKeeper.AccountScoreSnapshot.Set(ctx, collections.Join(distID, delAddr), score))
	requireT.NoError(pseKeeper.TotalScore.Set(ctx, distID, score))

	// Underfund the intermediary so the first send fails.
	intermediaryCoins := sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1)))
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, intermediaryCoins))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, types.ClearingAccountCommunityIntermediary, intermediaryCoins,
	))

	_, err = pseKeeper.ProcessOngoingTokenDistribution(ctx, ongoing, bondDenom)
	requireT.Error(err)

	errStr := err.Error()
	t.Logf("wrapped error: %s", errStr)
	for _, want := range []string{
		"phase 2:",
		"distribution_id=1",
		"user_amount=1000000",
		"total_score=100",
		"send reward from intermediary:",
		delAddr.String(),
	} {
		requireT.Contains(errStr, want, "missing %q from error chain", want)
	}
}

// TestDistribution_JailedValidator_SkipsReward verifies that when a delegator has stake on both
// jailed and healthy validators:
//   - The jailed validator's proportional share is routed to the Community Pool.
//   - The healthy validator receives auto-delegation for its proportional share only.
//   - The delegator's wallet remains unchanged (eligible portion fully auto-delegated,
//     jailed portion never sent to wallet).
//   - The jailed validator's token balance does not grow.
//   - PSE is not disabled.
func TestDistribution_JailedValidator_SkipsReward(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	t0 := time.Now().UTC().Round(time.Second)
	ctx, _, err := testApp.BeginNextBlockAtTime(t0)
	requireT.NoError(err)
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Create two validators: one to jail, one to remain healthy.
	createValidator := func() sdk.ValAddress {
		op, _ := testApp.GenAccount(ctx)
		requireT.NoError(testApp.FundAccount(ctx, op,
			sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))
		val, err := testApp.AddValidator(ctx, op, sdk.NewInt64Coin(bondDenom, 10), nil)
		requireT.NoError(err)
		return sdk.MustValAddressFromBech32(val.GetOperator())
	}
	valJailedAddr := createValidator()
	valHealthyAddr := createValidator()

	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(20_000)))))

	// Delegate equal amounts to both validators (50/50 split).
	// This means: eligibleShare = userAmount × 1000/2000 = userAmount/2
	// and jailed portion = userAmount/2 → Community Pool.
	delegateTo := func(val sdk.ValAddress, amount int64) {
		_, err := stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
			DelegatorAddress: delAddr.String(),
			ValidatorAddress: val.String(),
			Amount:           sdk.NewInt64Coin(bondDenom, amount),
		})
		requireT.NoError(err)
	}
	delegateTo(valJailedAddr, 1_000)
	delegateTo(valHealthyAddr, 1_000)

	// Capture pre-jail token balances for later assertions.
	valJailedBefore, err := stakingKeeper.GetValidator(ctx, valJailedAddr)
	requireT.NoError(err)
	jailedTokensBefore := valJailedBefore.Tokens

	valHealthyBefore, err := stakingKeeper.GetValidator(ctx, valHealthyAddr)
	requireT.NoError(err)
	healthyTokensBefore := valHealthyBefore.Tokens

	// Jail the first validator.
	jailValidator(t, requireT, ctx, stakingKeeper, valJailedAddr)

	// Capture pre-distribution state.
	communityBefore := communityPoolBalance(requireT, testApp, ctx, bondDenom)
	walletBefore := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount

	// Schedule and fund the distribution.
	scheduleTimestamp := t0.Add(5 * time.Second)
	const distributionAmount = int64(10_000_000)
	requireT.NoError(pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{{
		ID:        1,
		Timestamp: uint64(scheduleTimestamp.Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          sdkmath.NewInt(distributionAmount),
		}},
	}}))
	fundCommunityAccount(requireT, testApp, ctx, bondDenom, sdkmath.NewInt(distributionAmount))

	// Run three-block distribution sequence.
	runDistribution(requireT, testApp, t0)

	ctx, _, err = testApp.BeginNextBlockAtTime(t0.Add(25 * time.Second))
	requireT.NoError(err)

	// PSE must not be disabled; distribution must have finalized.
	assertDistributionFinalized(requireT, pseKeeper, ctx, 1)

	// Wallet must be unchanged: eligible portion was fully auto-delegated (never left
	// as loose tokens), and the jailed portion was never sent to the wallet at all.
	walletAfter := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount
	requireT.Equal(walletBefore, walletAfter,
		"delegator wallet must be unchanged: eligible portion auto-delegated, "+
			"jailed portion never sent to wallet")

	// Community pool must have grown by the jailed validator's proportional share.
	// With a 50/50 split, the community pool receives roughly half the delegator's
	// PSE userAmount (plus any rounding from other delegators).
	communityAfter := communityPoolBalance(requireT, testApp, ctx, bondDenom)
	communityGrowth := communityAfter.Sub(communityBefore)
	requireT.True(communityGrowth.IsPositive(),
		"jailed validator's proportional share must be routed to the Community Pool")

	// Healthy validator must have received auto-delegation from its proportional share.
	valHealthyAfter, err := stakingKeeper.GetValidator(ctx, valHealthyAddr)
	requireT.NoError(err)
	healthyGrowth := valHealthyAfter.Tokens.Sub(healthyTokensBefore)
	requireT.True(healthyGrowth.IsPositive(),
		"healthy validator tokens must grow from auto-delegation (growth=%s)", healthyGrowth)

	// Critical: jailed validator must NOT receive any auto-delegation.
	// This catches regressions where the implementation sends tokens to jailed validators.
	valJailedAfter, err := stakingKeeper.GetValidator(ctx, valJailedAddr)
	requireT.NoError(err)
	requireT.Equal(jailedTokensBefore, valJailedAfter.Tokens,
		"jailed validator tokens must not change — no auto-delegation must target a jailed validator")

	// Sanity: community growth and healthy growth together should approximately
	// equal the delegator's total userAmount (within rounding from other delegators).
	// Both should be roughly equal given the 50/50 delegation split.
	requireT.True(communityGrowth.GT(sdkmath.ZeroInt()))
	requireT.True(healthyGrowth.GT(sdkmath.ZeroInt()))
}

// TestDistribution_AllJailedValidators verifies that when every delegation is to a jailed
// validator:
//   - No bank-send occurs to the delegator (wallet unchanged).
//   - The full reward remains in the intermediary until finalization.
//   - The intermediary is fully drained after finalization.
//   - The entire reward is routed to the Community Pool.
//   - PSE is not disabled.
func TestDistribution_AllJailedValidators(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	t0 := time.Now().UTC().Round(time.Second)
	ctx, _, err := testApp.BeginNextBlockAtTime(t0)
	requireT.NoError(err)
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Single validator — will be jailed before distribution.
	op, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, op,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))
	val, err := testApp.AddValidator(ctx, op, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	valJailedAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(20_000)))))

	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valJailedAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 1_000),
	})
	requireT.NoError(err)

	// Advance time so the delegator accumulates a positive score before jailing.
	// This is important: we want to confirm the score exists (and is used in the
	// proportion calculation) even though the reward ultimately goes to Community Pool.
	ctx, _, err = testApp.BeginNextBlockAtTime(t0.Add(2 * time.Second))
	requireT.NoError(err)

	// Trigger score flush by making a small delegation change.
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valJailedAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 1),
	})
	requireT.NoError(err)

	// Capture score before distribution — must be positive to make the test meaningful.
	// If score is zero, community pool growth could come from other delegators, not
	// from this delegator's jailed portion, and the test would be vacuous.
	scoreBeforeDistribution, err := pseKeeper.CalculateDelegatorScore(ctx, delAddr)
	requireT.NoError(err)
	requireT.True(scoreBeforeDistribution.IsPositive(),
		"delegator must have positive score before distribution for this test to be meaningful")

	// Now jail the validator.
	jailValidator(t, requireT, ctx, stakingKeeper, valJailedAddr)

	// Capture pre-distribution state.
	communityBefore := communityPoolBalance(requireT, testApp, ctx, bondDenom)
	walletBefore := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount

	// Schedule and fund the distribution.
	scheduleTimestamp := t0.Add(5 * time.Second)
	const distributionAmount = int64(10_000_000)
	requireT.NoError(pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{{
		ID:        1,
		Timestamp: uint64(scheduleTimestamp.Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          sdkmath.NewInt(distributionAmount),
		}},
	}}))
	fundCommunityAccount(requireT, testApp, ctx, bondDenom, sdkmath.NewInt(distributionAmount))

	// Run three-block distribution sequence.
	runDistribution(requireT, testApp, t0)

	ctx, _, err = testApp.BeginNextBlockAtTime(t0.Add(25 * time.Second))
	requireT.NoError(err)

	// PSE must not be disabled; distribution must have finalized.
	assertDistributionFinalized(requireT, pseKeeper, ctx, 1)

	// Bank-send must be exactly zero — no tokens sent to the delegator's wallet.
	walletAfter := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount
	requireT.Equal(walletBefore, walletAfter,
		"bank-send must be zero when all validators are jailed — "+
			"full reward must stay in intermediary until finalization")

	// Full reward must be routed to the Community Pool after finalization.
	communityAfter := communityPoolBalance(requireT, testApp, ctx, bondDenom)
	communityGrowth := communityAfter.Sub(communityBefore)
	requireT.True(communityGrowth.IsPositive(),
		"full reward must be routed to the Community Pool when all validators are jailed")

	// Intermediary must be fully drained after finalizeCommunityDistribution runs.
	// This confirms the leftover sweep (totalPSEAmount - distributedSoFar) worked correctly.
	intermediaryAddr := testApp.AccountKeeper.GetModuleAccount(
		ctx, types.ClearingAccountCommunityIntermediary,
	).GetAddress()
	intermediaryBalance := testApp.BankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount
	requireT.True(intermediaryBalance.IsZero(),
		"intermediary account must be fully drained at finalization (balance=%s)", intermediaryBalance)
}

// TestDistribution_JailedValidator_ScorePreserved verifies the core design decision:
// score accumulated while the validator was bonded is preserved and contributes to the
// PSE proportion calculation even after the validator is jailed.
//
// Scenario:
//  1. Delegate to a validator.
//  2. Advance time — score accumulates while validator is bonded.
//  3. Jail the validator.
//  4. Run distribution — delegator has positive score, but all validators are jailed.
//  5. Expected: delegator's score is non-zero (used in proportion), but since all
//     validators are jailed, no bank-send occurs and the Community Pool receives
//     the delegator's proportional share.
func TestDistribution_JailedValidator_ScorePreserved(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	t0 := time.Now().UTC().Round(time.Second)
	ctx, _, err := testApp.BeginNextBlockAtTime(t0)
	requireT.NoError(err)
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Single validator that will accumulate score while bonded, then get jailed.
	op, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, op,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))
	val, err := testApp.AddValidator(ctx, op, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	delAddr, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, delAddr,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(20_000)))))

	// Delegate while validator is bonded.
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 1_000),
	})
	requireT.NoError(err)

	// Advance time to accumulate score while validator is healthy.
	// Score accumulated here = 1000 tokens × 8 seconds = 8000.
	ctx, _, err = testApp.BeginNextBlockAtTime(t0.Add(8 * time.Second))
	requireT.NoError(err)

	// Flush accumulated score to snapshot by triggering a delegation change.
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(bondDenom, 1),
	})
	requireT.NoError(err)

	// Capture score after bonded-period accumulation — this is the score that must
	// survive jailing and still be used in the distribution proportion calculation.
	scoreAfterBondedPeriod, err := pseKeeper.CalculateDelegatorScore(ctx, delAddr)
	requireT.NoError(err)
	requireT.True(scoreAfterBondedPeriod.IsPositive(),
		"score must have accumulated during the bonded period")

	// Jail the validator — score must not be affected.
	jailValidator(t, requireT, ctx, stakingKeeper, valAddr)

	// Verify score is unchanged after jailing (Phase-1 scoring is not modified).
	scoreAfterJail, err := pseKeeper.CalculateDelegatorScore(ctx, delAddr)
	requireT.NoError(err)
	requireT.Equal(scoreAfterBondedPeriod, scoreAfterJail,
		"score must be unchanged after validator is jailed — "+
			"Phase-1 scoring is not affected by jailing")

	// Capture pre-distribution Community Pool balance.
	communityBefore := communityPoolBalance(requireT, testApp, ctx, bondDenom)
	walletBefore := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount

	// Schedule and fund the distribution.
	scheduleTimestamp := t0.Add(5 * time.Second) // already past, so it's due immediately
	const distributionAmount = int64(10_000_000)
	requireT.NoError(pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{{
		ID:        1,
		Timestamp: uint64(scheduleTimestamp.Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          sdkmath.NewInt(distributionAmount),
		}},
	}}))
	fundCommunityAccount(requireT, testApp, ctx, bondDenom, sdkmath.NewInt(distributionAmount))

	// Run distribution.
	runDistribution(requireT, testApp, t0)

	ctx, _, err = testApp.BeginNextBlockAtTime(t0.Add(25 * time.Second))
	requireT.NoError(err)

	// PSE must not be disabled; distribution must have finalized.
	assertDistributionFinalized(requireT, pseKeeper, ctx, 1)

	// Verify outcomes.
	//
	// Wallet unchanged: validator is jailed so no bank-send occurred.
	walletAfter := testApp.BankKeeper.GetBalance(ctx, delAddr, bondDenom).Amount
	requireT.Equal(walletBefore, walletAfter,
		"wallet must be unchanged — validator is jailed so no bank-send occurred")

	// Community pool must have grown: the delegator's bonded-period score contributed
	// to the proportion calculation, so their userAmount was computed and routed to
	// the Community Pool (since all validators are jailed).
	communityAfter := communityPoolBalance(requireT, testApp, ctx, bondDenom)
	communityGrowth := communityAfter.Sub(communityBefore)
	requireT.True(communityGrowth.IsPositive(),
		"delegator's bonded-period score must produce a positive userAmount "+
			"that is routed to the Community Pool when all validators are jailed; "+
			"community growth=%s", communityGrowth)

	// Intermediary must be fully drained.
	intermediaryAddr := testApp.AccountKeeper.GetModuleAccount(
		ctx, types.ClearingAccountCommunityIntermediary,
	).GetAddress()
	intermediaryBalance := testApp.BankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount
	requireT.True(intermediaryBalance.IsZero(),
		"intermediary must be fully drained after finalization")
}

// TestDistribution_JailedValidator_BatchTransition documents the accepted behavior when a
// validator is jailed between Phase-2 batches: earlier batches may treat it as healthy while
// later batches treat it as jailed. The test verifies no panic and no PSE disable occur.
//
// This scenario is intentionally reproduced using a small batch size (1) so that
// two delegators span two separate Phase-2 batches.
func TestDistribution_JailedValidator_BatchTransition(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	pseKeeper := testApp.PSEKeeper
	stakingKeeper := testApp.StakingKeeper

	t0 := time.Now().UTC().Round(time.Second)
	ctx, _, err := testApp.BeginNextBlockAtTime(t0)
	requireT.NoError(err)
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	requireT.NoError(err)

	// Set batch size to 1 so each delegator is processed in its own Phase-2 block.
	// This forces the batch-transition scenario where the validator state can change
	// between batch 1 (block N) and batch 2 (block N+1).
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.DistributionBatchSize = 1
	requireT.NoError(pseKeeper.SetParams(ctx, params))

	// Single shared validator used by both delegators.
	op, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, op,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(1_000)))))
	sharedVal, err := testApp.AddValidator(ctx, op, sdk.NewInt64Coin(bondDenom, 10), nil)
	requireT.NoError(err)
	sharedValAddr := sdk.MustValAddressFromBech32(sharedVal.GetOperator())

	// Two delegators — each will occupy one Phase-2 batch.
	del1, _ := testApp.GenAccount(ctx)
	del2, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, del1,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000)))))
	requireT.NoError(testApp.FundAccount(ctx, del2,
		sdk.NewCoins(sdk.NewCoin(bondDenom, sdkmath.NewInt(10_000)))))

	delegateTo := func(del sdk.AccAddress, amount int64) {
		_, err := stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, &stakingtypes.MsgDelegate{
			DelegatorAddress: del.String(),
			ValidatorAddress: sharedValAddr.String(),
			Amount:           sdk.NewInt64Coin(bondDenom, amount),
		})
		requireT.NoError(err)
	}
	delegateTo(del1, 1_000)
	delegateTo(del2, 1_000)

	// Schedule and fund the distribution.
	scheduleTimestamp := t0.Add(5 * time.Second)
	const distributionAmount = int64(10_000_000)
	requireT.NoError(pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{{
		ID:        1,
		Timestamp: uint64(scheduleTimestamp.Unix()),
		Allocations: []types.ClearingAccountAllocation{{
			ClearingAccount: types.ClearingAccountCommunity,
			Amount:          sdkmath.NewInt(distributionAmount),
		}},
	}}))
	fundCommunityAccount(requireT, testApp, ctx, bondDenom, sdkmath.NewInt(distributionAmount))

	// t0+10s: non-community allocations + BeginCommunityDistribution.
	// Phase-1 runs with batchSize=1 so it may take multiple blocks.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(10 * time.Second)))

	// FIX: With batchSize=1, Phase 1 needs more blocks (4 DTE entries to process).
	// Adding extra Phase 1 blocks to ensure completion.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(11 * time.Second)))
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(11 * time.Second).Add(500 * time.Millisecond)))

	// t0+12s: Phase-2 batch 1 — del1 is processed while validator is healthy.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(12 * time.Second)))

	// Jail the validator between batch 1 and batch 2 to trigger the transition scenario.
	ctx, _, err = testApp.BeginNextBlockAtTime(t0.Add(13 * time.Second))
	requireT.NoError(err)
	jailValidator(t, requireT, ctx, stakingKeeper, sharedValAddr)

	// t0+15s: Phase-2 batch 2 — del2 is processed while validator is now jailed.
	// Earlier batch (del1) processed as healthy; this batch (del2) processes as jailed.
	// This is accepted behavior per spec: batch-ordering bias is documented, not fixed here.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(15 * time.Second)))

	// Need additional Phase-2 blocks to process remaining delegators and finalize.
	// With batchSize=1 and 4 delegators, need 4 blocks + 1 empty batch.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(16 * time.Second)))
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(17 * time.Second)))
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(18 * time.Second)))
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(19 * time.Second)))

	// t0+20s: finalize + cleanup.
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(20 * time.Second)))

	// Core assertion: no panic and no PSE disable despite mid-distribution jailing.
	assertDistributionFinalized(requireT, pseKeeper, ctx, 1)

	// Intermediary must be fully drained regardless of batch-transition state.
	intermediaryAddr := testApp.AccountKeeper.GetModuleAccount(
		ctx, types.ClearingAccountCommunityIntermediary,
	).GetAddress()
	intermediaryBalance := testApp.BankKeeper.GetBalance(ctx, intermediaryAddr, bondDenom).Amount
	requireT.True(intermediaryBalance.IsZero(),
		"intermediary must be drained even after a mid-distribution jail event")
}

// jailValidator is a test helper that jails the given validator and asserts
// the jailed flag is visible through GetAllValidators.
func jailValidator(
	t *testing.T,
	requireT *require.Assertions,
	ctx sdk.Context,
	stakingKeeper *stakingkeeper.Keeper,
	valAddr sdk.ValAddress,
) {
	t.Helper()
	val, err := stakingKeeper.GetValidator(ctx, valAddr)
	requireT.NoError(err)
	consAddr, err := val.GetConsAddr()
	requireT.NoError(err)
	requireT.NoError(stakingKeeper.Jail(ctx, sdk.ConsAddress(consAddr)))

	all, err := stakingKeeper.GetAllValidators(ctx)
	requireT.NoError(err)
	found := false
	for _, v := range all {
		if v.OperatorAddress == valAddr.String() {
			requireT.True(v.IsJailed(), "validator must be jailed after Jail() call")
			found = true
			break
		}
	}
	requireT.True(found, "jailed validator must appear in GetAllValidators")
}

// fundCommunityAccount mints amount into pse_community and returns the funded amount.
func fundCommunityAccount(
	requireT *require.Assertions,
	testApp *simapp.App,
	ctx sdk.Context,
	bondDenom string,
	amount sdkmath.Int,
) {
	coins := sdk.NewCoins(sdk.NewCoin(bondDenom, amount))
	macc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, coins))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc.GetName(), coins,
	))
}

// runDistribution runs the three-block EndBlocker sequence used in all jailed-validator tests:
//   - t0+10s: Phase-1 score conversion + BeginCommunityDistribution
//   - t0+15s: Phase-2 first batch (pays delegators)
//   - t0+20s: Phase-2 empty batch → finalize + cleanup
func runDistribution(
	requireT *require.Assertions,
	testApp *simapp.App,
	t0 time.Time,
) {
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(10 * time.Second)))
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(15 * time.Second)))
	requireT.NoError(testApp.FinalizeBlockAtTime(t0.Add(20 * time.Second)))
}

// assertDistributionFinalized checks PSE is not disabled and distribution ID advanced.
//
//nolint:unparam
func assertDistributionFinalized(
	requireT *require.Assertions,
	pseKeeper keeper.Keeper,
	ctx sdk.Context,
	expectedID uint64,
) {
	disabled, err := pseKeeper.DistributionDisabled.Get(ctx)
	requireT.NoError(err)
	requireT.False(disabled, "PSE must not be disabled")

	lastProcessed, err := pseKeeper.LastProcessedDistributionID.Get(ctx)
	requireT.NoError(err)
	requireT.Equal(expectedID, lastProcessed, "LastProcessedDistributionID must advance")
}

// communityPoolBalance returns the community pool balance for bondDenom.
func communityPoolBalance(
	requireT *require.Assertions,
	testApp *simapp.App,
	ctx sdk.Context,
	bondDenom string,
) sdkmath.Int {
	feePool, err := testApp.DistrKeeper.FeePool.Get(ctx)
	requireT.NoError(err)
	return feePool.CommunityPool.AmountOf(bondDenom).TruncateInt()
}
