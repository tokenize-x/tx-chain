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
	err = pseKeeper.OngoingDistribution.Set(ctx, scheduledDistribution)
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

	// Step 7: Re-delegate before re-inclusion (simulating an excluded address that still has delegation)
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

	// Step 8: Remove from exclude_list (re-include)
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(1 * time.Second))
	err = pseKeeper.UpdateExcludedAddresses(ctx, authority, nil, []string{delAddr.String()})
	requireT.NoError(err)

	// Verify DelegationTimeEntry was recreated with current state
	entry, err := pseKeeper.GetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr)
	requireT.NoError(err, "DelegationTimeEntry should be recreated on re-inclusion")
	requireT.Equal(ctx.BlockTime().Unix(), entry.LastChangedUnixSec, "Entry should have current block time")

	// Verify restored score is stored under the current active distID (distributionID=2),
	restoredSnapshot, err := pseKeeper.GetDelegatorScore(ctx, distributionID, delAddr)
	requireT.NoError(err)
	requireT.True(restoredSnapshot.IsPositive(),
		"restored score must be in AccountScoreSnapshot at active distID after re-inclusion")

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
	// Restored excluded score + fresh accumulation (50 tokens * 3s = 150)
	requireT.True(scoreAfterReinclusion.GT(scoreBeforeExclusion), "Score after re-inclusion should exceed original (restored + new accumulation)")
	t.Logf("Score after re-inclusion and 3 seconds: %s (original was %s)", scoreAfterReinclusion.String(), scoreBeforeExclusion.String())

	// Verify fresh accumulation component: at least 50 tokens * 3 seconds = 150 beyond the restored score.
	expectedMinScore := scoreBeforeExclusion.Add(sdkmath.NewInt(50 * 3))
	requireT.True(scoreAfterReinclusion.GTE(expectedMinScore),
		"Score should be at least %s (got %s)", expectedMinScore.String(), scoreAfterReinclusion.String())

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
	requireT.NoError(pseKeeper.OngoingDistribution.Set(ctx, scheduledDist2))

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
