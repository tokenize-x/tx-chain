package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
)

func TestKeeper_Distribute(t *testing.T) {
	// TODO: add more test cases.
	// test accumulated score (done.)
	// test unaccumulated score
	// test accumulated score + uncaculated score
	// test redelegation, undelegation, cancel unbonding delegation
	// test no delegation for scoring user
	// test score reset
	// test excluded address
	cases := []struct {
		name    string
		actions []func(*runEnv)
	}{
		{
			name: "test accumulated score",
			actions: []func(*runEnv){
				func(r *runEnv) { delegateAction(r, r.delegators[0], r.validators[0], 1_100_000) },
				func(r *runEnv) { delegateAction(r, r.delegators[1], r.validators[0], 900_000) },
				func(r *runEnv) { waitAction(r, time.Second*8) },
				func(r *runEnv) { distributeAction(r, sdkmath.NewInt(1000)) },
				func(r *runEnv) {
					assertDistributionAction(r, map[*sdk.AccAddress]sdkmath.Int{
						&r.delegators[0]: sdkmath.NewInt(1_100_366), // + 1000 * 1.1 m / 3 m
						&r.delegators[1]: sdkmath.NewInt(900_299),   // + 1000 * 0.9 m / 3 m
					})
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)
			testApp := simapp.New()
			ctx, _, err := testApp.BeginNextBlock()
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
				validator, err := addValidator(
					ctx, testApp, validatorOperator, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10),
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
