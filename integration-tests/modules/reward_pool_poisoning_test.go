//go:build integrationtests

package modules

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	integrationtests "github.com/tokenize-x/tx-chain/v8/integration-tests"
	"github.com/tokenize-x/tx-chain/v8/pkg/client"
	"github.com/tokenize-x/tx-chain/v8/testutil/integration"
	assetfttypes "github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
	customparamstypes "github.com/tokenize-x/tx-chain/v8/x/customparams/types"
)

// TestRewardPoolPoisoning verifies the fix that restricts validator reward-pool
// deposits to the bond denom.
//
// The attack it guards against: depositing a whitelisting-enabled token into a
// validator's reward pool poisons OutstandingRewards, so every later reward payout
// fails and freezes all staking operations on that validator. The test asserts:
//   - the poison deposit is rejected (directly and when wrapped in authz.MsgExec),
//   - no poison denom enters OutstandingRewards,
//   - all staking operations keep working (no freeze),
//   - legitimate bond-denom deposits still succeed (no regression).
//
// On a vulnerable build the Phase 1 rejection assertions fail, so this test fails
// without the fix and passes with it.
func TestRewardPoolPoisoning(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)
	bankClient := banktypes.NewQueryClient(chain.ClientContext)
	distrClient := distributiontypes.NewQueryClient(chain.ClientContext)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	issueFee := chain.QueryAssetFTParams(ctx, t).IssueFee
	customParamsClient := customparamstypes.NewQueryClient(chain.ClientContext)
	customStakingParams, err := customParamsClient.StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)
	validatorStakingAmount := customStakingParams.Params.MinSelfDelegation.Mul(sdkmath.NewInt(2))

	// --- Phase 0: Setup ---

	attacker := chain.GenAccount()
	delegator := chain.GenAccount()
	// Delegate ~the validator self-stake so the delegator earns a non-zero reward share;
	// a tiny delegation would round to zero against the large MinSelfDelegation.
	delegateAmount := validatorStakingAmount
	rewardSeedAmount := sdkmath.NewInt(10_000_000)

	validator1AccAddr, validator1Addr, _, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)

	// Second validator is the redelegation destination used in Phase 2.
	_, validator2Addr, _, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)

	chain.FundAccountWithOptions(ctx, t, delegator, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&stakingtypes.MsgDelegate{},                     // initial delegation
			&distributiontypes.MsgWithdrawDelegatorReward{}, // Phase 0 baseline withdraw
			&distributiontypes.MsgWithdrawDelegatorReward{}, // Phase 2 withdraw
			&stakingtypes.MsgDelegate{},                     // Phase 2 delegate more
			&stakingtypes.MsgUndelegate{},                   // Phase 2 undelegate
		},
		// MsgBeginRedelegate (Phase 2) has non-deterministic gas, so it is funded here
		// and broadcast with TxFactoryAuto rather than priced via GasLimitByMsgs.
		NondeterministicMessagesGas: 500_000,
		Amount:                      delegateAmount.MulRaw(3),
	})

	// Seeder makes the legitimate bond-denom deposits (Phase 0 seed + Phase 3 regression).
	rewardSeeder := chain.GenAccount()
	chain.FundAccountWithOptions(ctx, t, rewardSeeder, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&distributiontypes.MsgDepositValidatorRewardsPool{},
			&distributiontypes.MsgDepositValidatorRewardsPool{},
		},
		Amount: rewardSeedAmount.MulRaw(2),
	})

	delegateMsg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator1Addr.String(),
		Amount:           chain.NewCoin(delegateAmount),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg)),
		delegateMsg)
	requireT.NoError(err)

	// Seed a legitimate bond-denom reward so the delegator has rewards to withdraw.
	depositBondDenomMsg := &distributiontypes.MsgDepositValidatorRewardsPool{
		Depositor:        rewardSeeder.String(),
		ValidatorAddress: validator1Addr.String(),
		Amount:           sdk.NewCoins(chain.NewCoin(rewardSeedAmount)),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(rewardSeeder),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(depositBondDenomMsg)),
		depositBondDenomMsg)
	requireT.NoError(err)

	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 2))

	// Baseline: delegator can withdraw a non-zero reward before the attack attempt.
	delegatorBalBefore, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: delegator.String(), Denom: chain.ChainSettings.Denom,
	})
	requireT.NoError(err)

	baselineWithdrawMsg := &distributiontypes.MsgWithdrawDelegatorReward{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator1Addr.String(),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(baselineWithdrawMsg)),
		baselineWithdrawMsg)
	requireT.NoError(err)

	delegatorBalAfter, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: delegator.String(), Denom: chain.ChainSettings.Denom,
	})
	requireT.NoError(err)
	gasCost := chain.ComputeNeededBalanceFromOptions(integration.BalancesOptions{
		Messages: []sdk.Msg{baselineWithdrawMsg},
	})
	baselineReward := delegatorBalAfter.Balance.Amount.Sub(delegatorBalBefore.Balance.Amount.Sub(gasCost))
	requireT.True(baselineReward.IsPositive(), "baseline: delegator must have received non-zero rewards")
	t.Logf("Baseline: withdrawal succeeded, reward: %s", baselineReward)

	// --- Phase 1: the poison deposit must be REJECTED ---

	chain.FundAccountWithOptions(ctx, t, attacker, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&assetfttypes.MsgIssue{},
			&assetfttypes.MsgSetWhitelistedLimit{},
		},
		Amount: issueFee.Amount,
	})

	issueMsg := &assetfttypes.MsgIssue{
		Issuer:        attacker.String(),
		Symbol:        "POISON",
		Subunit:       "upoison",
		Precision:     6,
		Description:   "Reward pool poisoning token",
		InitialAmount: sdkmath.NewInt(1_000_000_000),
		Features:      []assetfttypes.Feature{assetfttypes.Feature_whitelisting},
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(attacker),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(issueMsg)),
		issueMsg)
	requireT.NoError(err)
	poisonDenom := assetfttypes.BuildDenom(issueMsg.Subunit, attacker)

	// Whitelisting the distribution module is what let the poison settle in pre-fix.
	distributionModuleAddr := authtypes.NewModuleAddress(distributiontypes.ModuleName)
	whitelistMsg := &assetfttypes.MsgSetWhitelistedLimit{
		Sender:  attacker.String(),
		Account: distributionModuleAddr.String(),
		Coin:    sdk.NewInt64Coin(poisonDenom, 1_000_000_000),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(attacker),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(whitelistMsg)),
		whitelistMsg)
	requireT.NoError(err)

	poisonDeposit := &distributiontypes.MsgDepositValidatorRewardsPool{
		Depositor:        attacker.String(),
		ValidatorAddress: validator1Addr.String(),
		Amount:           sdk.NewCoins(sdk.NewInt64Coin(poisonDenom, 1_000_000)),
	}

	// 1a. Direct poison deposit is rejected by the ante decorator.
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(attacker),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(poisonDeposit)),
		poisonDeposit)
	requireT.Error(err)
	requireT.ErrorContains(err, "only the bond denom")
	t.Log("Direct poison deposit rejected: only the bond denom is accepted")

	// 1b. Poison deposit wrapped in authz.MsgExec is ALSO rejected (bypass closed).
	execMsg := authztypes.NewMsgExec(attacker, []sdk.Msg{poisonDeposit})
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(attacker),
		chain.TxFactory().WithGas(400_000),
		&execMsg)
	requireT.Error(err)
	requireT.ErrorContains(err, "only the bond denom")
	t.Log("authz-wrapped poison deposit rejected: bypass closed")

	// No poison denom ever entered the validator's outstanding rewards.
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 1))
	outstandingResp, err := distrClient.ValidatorOutstandingRewards(ctx,
		&distributiontypes.QueryValidatorOutstandingRewardsRequest{
			ValidatorAddress: validator1Addr.String(),
		})
	requireT.NoError(err)
	requireT.True(outstandingResp.Rewards.Rewards.AmountOf(poisonDenom).IsZero(),
		"no poison denom must be present in outstanding rewards")
	t.Log("OutstandingRewards clean: no poison denom present")

	// --- Phase 2: all staking operations keep working (no freeze) ---

	// 1. Delegator can withdraw rewards.
	withdrawMsg := &distributiontypes.MsgWithdrawDelegatorReward{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator1Addr.String(),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(withdrawMsg)),
		withdrawMsg)
	requireT.NoError(err)

	// 2. Delegator can delegate more.
	delegateMoreMsg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator1Addr.String(),
		Amount:           chain.NewCoin(sdkmath.NewInt(1)),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMoreMsg)),
		delegateMoreMsg)
	requireT.NoError(err)

	// 3. Delegator can redelegate to validator 2.
	redelegateMsg := &stakingtypes.MsgBeginRedelegate{
		DelegatorAddress:    delegator.String(),
		ValidatorSrcAddress: validator1Addr.String(),
		ValidatorDstAddress: validator2Addr.String(),
		Amount:              chain.NewCoin(sdkmath.NewInt(100)),
	}
	// MsgBeginRedelegate is non-deterministic gas on tx-chain → use TxFactoryAuto.
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactoryAuto(),
		redelegateMsg)
	requireT.NoError(err)

	// 4. Delegator can undelegate.
	undelegateMsg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator1Addr.String(),
		Amount:           chain.NewCoin(sdkmath.NewInt(100)),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(undelegateMsg)),
		undelegateMsg)
	requireT.NoError(err)

	// 5. Validator can withdraw commission.
	chain.FundAccountWithOptions(ctx, t, validator1AccAddr, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&distributiontypes.MsgWithdrawValidatorCommission{},
			&stakingtypes.MsgUndelegate{},
		},
		Amount: sdkmath.NewInt(1),
	})
	commissionMsg := &distributiontypes.MsgWithdrawValidatorCommission{
		ValidatorAddress: validator1Addr.String(),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(validator1AccAddr),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(commissionMsg)),
		commissionMsg)
	requireT.NoError(err)

	// 6. Validator can self-undelegate.
	selfUndelegateMsg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: validator1AccAddr.String(),
		ValidatorAddress: validator1Addr.String(),
		Amount:           chain.NewCoin(sdkmath.NewInt(1)),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(validator1AccAddr),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(selfUndelegateMsg)),
		selfUndelegateMsg)
	requireT.NoError(err)
	t.Log("All 6 operations succeeded: withdraw, delegate, redelegate, undelegate, commission, self-undelegate")

	// --- Phase 3: legitimate bond-denom deposits still work (no regression) ---

	depositBondDenomMsg2 := &distributiontypes.MsgDepositValidatorRewardsPool{
		Depositor:        rewardSeeder.String(),
		ValidatorAddress: validator1Addr.String(),
		Amount:           sdk.NewCoins(chain.NewCoin(rewardSeedAmount)),
	}
	_, err = client.BroadcastTx(ctx,
		chain.ClientContext.WithFromAddress(rewardSeeder),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(depositBondDenomMsg2)),
		depositBondDenomMsg2)
	requireT.NoError(err)
	t.Log("Legitimate bond-denom reward-pool deposit still succeeds")

	// Sanity: validator still queryable with positive tokens.
	val1Resp, err := stakingClient.Validator(ctx, &stakingtypes.QueryValidatorRequest{
		ValidatorAddr: validator1Addr.String(),
	})
	requireT.NoError(err)
	requireT.True(val1Resp.Validator.Tokens.IsPositive(), "validator must retain its tokens")
}
