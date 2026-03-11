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
						&r.delegators[0]: sdkmath.NewInt(1_100_366), // + 1000 * 1.1 / 2
						&r.delegators[1]: sdkmath.NewInt(900_299),   // + 1000 * 0.9 / 2
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
					// delegators[0] fully undelegated — no auto-delegation, but earned reward sent as liquid tokens
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(0),       // staking balance 0 (no active delegation for auto-delegate)
						&r.delegators[1]: sdkmath.NewInt(900_299), // 900k original + 1000 * 0.9 / 2.4 ≈ 299 auto-delegated
					})
					// delegators[0] receives 366 as liquid: 1000 (FundAccount) + 366 (PSE reward) = 1366
					// undelegated 1,100,000 tokens are in unbonding queue, not liquid
					balance := r.testApp.BankKeeper.GetBalance(r.ctx, r.delegators[0], sdk.DefaultBondDenom)
					r.requireT.Equal(sdkmath.NewInt(1366), balance.Amount)
				},
				// only rounding leftover goes to community pool (no forfeited rewards)
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(2)) },
				func(r *runEnv) { assertScoreResetAction(r) },
			},
		},
		{
			name: "zero score",
			actions: []func(*runEnv){
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
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

	distributionID := uint64(1)

	// Step 1: Delegate 100 tokens at t=0, wait 10s, delegate 1 more to trigger score calc.
	// Score snapshot = 100 * 10 = 1000.
	msg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 100),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msg)
	requireT.NoError(err)

	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(10 * time.Second))

	msg2 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 1),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msg2)
	requireT.NoError(err)

	// Verify score = 1000
	scoreBeforeExclusion, err := pseKeeper.GetDelegatorScore(ctx, distributionID, delAddr)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1000), scoreBeforeExclusion)
	t.Logf("Score before exclusion: %s", scoreBeforeExclusion.String())

	// Step 2: Add to exclusion list.
	// AccountScoreSnapshot(1000) should move to ExcludedAddressScore(1000).
	// DelegationTimeEntries should be preserved.
	err = pseKeeper.UpdateExcludedAddresses(ctx, authority, []string{delAddr.String()}, nil)
	requireT.NoError(err)

	// Verify: score query returns 0 (grpc filter for excluded).
	// Advance time so currentPeriodScore would be non-zero without the filter.
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(2 * time.Second))
	resp, err := queryService.Score(ctx, &types.QueryScoreRequest{Address: delAddr.String()})
	requireT.NoError(err)
	requireT.True(resp.Score.IsZero(), "Score query should return 0 for excluded address even after time passes")

	// Verify: AccountScoreSnapshot cleared
	_, err = pseKeeper.GetDelegatorScore(ctx, distributionID, delAddr)
	requireT.ErrorIs(err, collections.ErrNotFound, "AccountScoreSnapshot should be cleared on exclusion")

	// Verify: ExcludedAddressScore has the accumulated score
	excludedScore, err := pseKeeper.ExcludedAddressScore.Get(ctx, delAddr)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1000), excludedScore, "ExcludedAddressScore should have the original score")

	// Verify: DelegationTimeEntry still exists (NOT removed)
	_, err = pseKeeper.GetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr)
	requireT.NoError(err, "DelegationTimeEntry should be preserved for excluded address")

	// Step 3: Wait 3s more (2s already passed in query check above), make delegation change while excluded.
	// Score from entry: 101 * 5 = 505 -> added to ExcludedAddressScore.
	// ExcludedAddressScore = 1000 + 505 = 1505.
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(3 * time.Second))
	msg3 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 1),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Delegate(ctx, msg3)
	requireT.NoError(err)

	excludedScore, err = pseKeeper.ExcludedAddressScore.Get(ctx, delAddr)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1505), excludedScore, "ExcludedAddressScore should accumulate during exclusion")
	t.Logf("ExcludedAddressScore after delegation change: %s", excludedScore.String())

	// Score query still returns 0
	resp, err = queryService.Score(ctx, &types.QueryScoreRequest{Address: delAddr.String()})
	requireT.NoError(err)
	requireT.True(resp.Score.IsZero(), "Excluded address score query should still return 0")

	// Step 4: Run distribution while excluded — should receive nothing.
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
		done, err := pseKeeper.ProcessPhase1ScoreConversion(ctx, scheduledDistribution)
		requireT.NoError(err)
		if done {
			break
		}
	}
	for {
		done, err := pseKeeper.ProcessPhase2TokenDistribution(ctx, scheduledDistribution, bondDenom)
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

	// After distribution, entries migrated from distributionID=1 to distributionID=2.
	distributionID++
	err = pseKeeper.SaveDistributionSchedule(ctx, []types.ScheduledDistribution{
		{Timestamp: distributionID, ID: distributionID},
	})
	requireT.NoError(err)

	// Verify entry migrated to new distID
	_, err = pseKeeper.GetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr)
	requireT.NoError(err, "Entry should be migrated to distID=2 after Phase 1")

	// Step 5: Fully undelegate while excluded.
	msgUndel := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delAddr.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 102),
	}
	_, err = stakingkeeper.NewMsgServerImpl(stakingKeeper).Undelegate(ctx, msgUndel)
	requireT.NoError(err, "Excluded delegator should be able to fully undelegate")

	// Step 6: Re-delegate 50 tokens while still excluded.
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

	// Step 7: Wait 1s and remove from exclusion list (re-include).
	// ExcludedAddressScore should be moved to AccountScoreSnapshot.
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(1 * time.Second))
	excludedScoreBeforeReinclusion, err := pseKeeper.ExcludedAddressScore.Get(ctx, delAddr)
	requireT.NoError(err)
	t.Logf("ExcludedAddressScore before re-inclusion: %s", excludedScoreBeforeReinclusion.String())

	err = pseKeeper.UpdateExcludedAddresses(ctx, authority, nil, []string{delAddr.String()})
	requireT.NoError(err)

	// Verify: ExcludedAddressScore cleared
	_, err = pseKeeper.ExcludedAddressScore.Get(ctx, delAddr)
	requireT.ErrorIs(err, collections.ErrNotFound, "ExcludedAddressScore should be cleared on re-inclusion")

	// Verify: AccountScoreSnapshot restored with full accumulated score
	restoredScore, err := pseKeeper.GetDelegatorScore(ctx, distributionID, delAddr)
	requireT.NoError(err)
	requireT.Equal(excludedScoreBeforeReinclusion, restoredScore,
		"AccountScoreSnapshot should contain full ExcludedAddressScore after re-inclusion")

	// Verify: DelegationTimeEntry exists
	entry, err := pseKeeper.GetDelegationTimeEntry(ctx, distributionID, valAddr, delAddr)
	requireT.NoError(err, "DelegationTimeEntry should exist after re-inclusion")
	t.Logf("Entry lastChanged after re-inclusion: %d, blockTime: %d", entry.LastChangedUnixSec, ctx.BlockTime().Unix())

	// Step 8: Wait 3s and verify exact score = restored + currentPeriod.
	// currentPeriod = 50 tokens × (blockTime - entry.LastChangedUnixSec).
	// Entry lastChanged is from the delegate-50 at Step 6 (not updated by un-exclusion).
	ctx = ctx.WithBlockTime(ctx.BlockTime().Add(3 * time.Second))
	currentPeriodDuration := ctx.BlockTime().Unix() - entry.LastChangedUnixSec
	expectedTotal := restoredScore.Add(sdkmath.NewInt(50).MulRaw(currentPeriodDuration))
	resp2, err := queryService.Score(ctx, &types.QueryScoreRequest{Address: delAddr.String()})
	requireT.NoError(err)
	requireT.Equal(expectedTotal, resp2.Score,
		"exact score: restored(%s) + 50*%ds = %s", restoredScore, currentPeriodDuration, expectedTotal)
	t.Logf("Total score after re-inclusion + 3s: %s (restored=%s + currentPeriod=50*%ds=%d)",
		resp2.Score, restoredScore, currentPeriodDuration, 50*currentPeriodDuration)
}
