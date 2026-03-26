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
			name: "zero score triggers invariant violation",
			actions: []func(*runEnv){
				func(r *runEnv) {
					distributeExpectInvariantViolation(r, sdkmath.NewInt(1000))
				},
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
