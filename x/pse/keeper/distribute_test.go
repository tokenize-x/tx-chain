package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
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
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(0),       // + 1000 * 1.1 / 3
						&r.delegators[1]: sdkmath.NewInt(900_299), // + 1000 * 0.9 / 3
					})
				},
				// + 1000 * 1.1 / 3 (from user's share) + 2 (from rounding)
				func(r *runEnv) { assertCommunityPoolBalanceAction(r, sdkmath.NewInt(366+2)) },
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
				testApp:  testApp,
				ctx:      ctx,
				requireT: requireT,
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

			// run actions.
			for _, action := range tc.actions {
				action(runContext)
			}
		})
	}
}

// Test_ExcludedAddress_SnapshotPreservationAndUndelegation validates two critical behaviors for excluded addresses:
//
// Scenario A: Snapshot Preservation
// When an address is excluded from PSE distribution via governance parameters, its score snapshot
// must be preserved during distribution cycles. This ensures that if the address is later re-included,
// its historical score accumulation remains intact for accurate future reward calculations.
//
// Scenario B: Full Undelegation Post-Distribution
// Excluded delegators must be able to fully undelegate their tokens after a distribution occurs.
// This test ensures delegation time entries are properly maintained for all addresses, allowing
// successful undelegation even for excluded accounts.
func Test_ExcludedAddress_SnapshotPreservationAndUndelegation(t *testing.T) {
	requireT := require.New(t)

	startTime := time.Now().Round(time.Second)
	testApp := simapp.New(simapp.WithStartTime(startTime))
	ctx, _, err := testApp.BeginNextBlockAtTime(startTime)
	requireT.NoError(err)

	// one validator
	valOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, valOp, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1_000))),
	))
	val, err := testApp.AddValidator(ctx, valOp, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil)
	requireT.NoError(err)
	valAddr := sdk.MustValAddressFromBech32(val.GetOperator())

	// one delegator
	del, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(ctx, del, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1_000)))))

	// delegate some, wait, delegate again to accumulate non-zero snapshot via hooks
	delegate := func(amount int64) {
		msg := &stakingtypes.MsgDelegate{
			DelegatorAddress: del.String(),
			ValidatorAddress: valAddr.String(),
			Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, amount),
		}
		_, err := stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Delegate(ctx, msg)
		requireT.NoError(err)
	}

	// mint coins for delegation
	requireT.NoError(testApp.BankKeeper.MintCoins(
		ctx, minttypes.ModuleName, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(200))),
	))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToAccount(
		ctx, minttypes.ModuleName, del, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(200))),
	))
	delegate(100)

	// advance time to accrue score on next modification
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(5 * time.Second))
	requireT.NoError(err)
	delegate(1) // triggers hook to store snapshot

	// capture snapshot before exclusion
	prevSnap, err := testApp.PSEKeeper.AccountScoreSnapshot.Get(ctx, del)
	requireT.NoError(err)
	requireT.True(prevSnap.GT(sdkmath.ZeroInt()), "snapshot should be non-zero before exclusion")

	// exclude the delegator via governance params
	params, err := testApp.PSEKeeper.GetParams(ctx)
	requireT.NoError(err)
	params.ExcludedAddresses = append(params.ExcludedAddresses, del.String())
	err = testApp.PSEKeeper.SetParams(ctx, params)
	requireT.NoError(err)

	// fund community clearing and run distribution
	bondDenom, err := testApp.StakingKeeper.BondDenom(ctx)
	requireT.NoError(err)
	amount := sdkmath.NewInt(1_000)
	macc := testApp.AccountKeeper.GetModuleAccount(ctx, types.ClearingAccountCommunity)
	requireT.NoError(testApp.BankKeeper.MintCoins(ctx, minttypes.ModuleName, sdk.NewCoins(sdk.NewCoin(bondDenom, amount))))
	requireT.NoError(testApp.BankKeeper.SendCoinsFromModuleToModule(
		ctx, minttypes.ModuleName, macc.GetName(), sdk.NewCoins(sdk.NewCoin(bondDenom, amount)),
	))
	scheduledAt := uint64(ctx.BlockTime().Unix())
	err = testApp.PSEKeeper.DistributeCommunityPSE(ctx, bondDenom, amount, scheduledAt)
	requireT.NoError(err)

	// Verify Scenario A: The excluded address's score snapshot must still exist and match its pre-exclusion value.
	// This confirms the selective clearing logic preserves excluded address snapshots.
	snap, err := testApp.PSEKeeper.AccountScoreSnapshot.Get(ctx, del)
	requireT.NoError(err, "excluded snapshot should still exist after distribution")
	requireT.Equal(prevSnap, snap, "excluded snapshot should be unchanged after distribution")

	// Verify Scenario B: The excluded delegator can successfully undelegate all tokens after distribution.
	// This confirms delegation time entries were reset (not cleared), allowing the hook to succeed.
	msgUndel := &stakingtypes.MsgUndelegate{
		DelegatorAddress: del.String(),
		ValidatorAddress: valAddr.String(),
		Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, 101), // full amount (100 + 1 from earlier)
	}
	_, err = stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper).Undelegate(ctx, msgUndel)
	requireT.NoError(err, "excluded delegator should be able to fully undelegate after distribution")
}
