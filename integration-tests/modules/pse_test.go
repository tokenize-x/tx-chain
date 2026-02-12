//go:build integrationtests

package modules

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	tmtypes "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govtypesv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	integrationtests "github.com/tokenize-x/tx-chain/v7/integration-tests"
	"github.com/tokenize-x/tx-chain/v7/pkg/client"
	"github.com/tokenize-x/tx-chain/v7/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v7/testutil/event"
	"github.com/tokenize-x/tx-chain/v7/testutil/integration"
	customparamstypes "github.com/tokenize-x/tx-chain/v7/x/customparams/types"
	psetypes "github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

// defaultClearingAccountMappings returns the default clearing account mappings for the given chain ID.
// Community clearing account is not included in the mappings.
// Each clearing account has a single default recipient address.
//
//nolint:funlen // large switch with chain-specific mapping literals
func defaultClearingAccountMappings(chainID string) ([]psetypes.ClearingAccountMapping, error) {
	// Create mappings for all non-Community clearing accounts
	// Each starts with a single default recipient (can be modified via governance)
	var mappings []psetypes.ClearingAccountMapping

	switch chainID {
	case string(constant.ChainIDMain):
		mappings = []psetypes.ClearingAccountMapping{
			{
				ClearingAccount: psetypes.ClearingAccountFoundation,
				RecipientAddresses: []string{
					"core142498n8sya3k3s5jftp7dujuqfw3ag4tpzc2ve45ykpwx6zmng8skcw5nw",
					"core1ys0dhh6x5s55h2g37zrnc7kh630jfq5p77as8pwyn60ax9zzqh9qvpwc0e",
					"core1wgjpjh42cr7t5sp5hgty4yrzww496a6yaznc9u4wsv9ac3xccu8smqaann",
					"core1rkml5878l2daw3a7xvg48wqecnh9u9dn2dtl8g57rsctq5pnc00sl0nwak",
					"core17l6djqrztw0ux668vkw7ff7d2602jvml52w9fyrvryusp7djnhfq7sg29r",
					"core10ezj2lmcj3flaacqwrzv278aled0pen8cnx257sggeng2fdel53q5gtudn",
					"core1wfse3z8akyw3pmn8x0htzq6l5wwfgqmc2jgnhxtzm96h4ywhhr0q63uvwl",
					"core10w37pqels7ya404xdlfkc9vdfemejmc0e6hjlerknys3xjj9xnasuk9uy2",
					"core13cwsdsetcrhcyd3jeed0mgteg35qaju0q5s0u0drfylagahygwwsj2eanz",
					"core1jc4mtk0g8ulmvhwmpfy5rrj7rwn85ual4p3w0tlwnp2rsauvf5eq58zdmw",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountAlliance,
				RecipientAddresses: []string{
					"core1cfey705ssf6ysclm9u47mvcgr5l6q6q86lk5dtq4jwdu6yjce6ds2tgy6j",
					"core15629hwdy7rd7satqzffn4f80ftg2sln982xvwcalppg36td7jvuq3pqevw",
					"core15lch5glk7deu9tk8wrcfcup4tdpz2l8zhhqn4r2zzsr46dfv849qetkah4",
					"core19rrgcsw8gu8c3rthucqnf6nyyg6q9pq79tt60pvahfsnfu4p5hrsuqajru",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountPartnership,
				RecipientAddresses: []string{
					"core12s5tahy3850k3r3080en0pwhuk4l3my5l2cl8vxrsg6kx48de24q7ygamd",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountInvestors,
				RecipientAddresses: []string{
					"core1mqevjln5hxv3qgd3c4m5zjeeand5hkc7r33ty82fjukw9shxjh6sr0zafz",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountTeam,
				RecipientAddresses: []string{
					"core12xyww2vucfufyzknvyameh5v25cn6gxzzagwgpzhwdq8v35zdmgqd6t6c7",
				},
			},
		}
	case string(constant.ChainIDTest):
		mappings = []psetypes.ClearingAccountMapping{
			{
				ClearingAccount: psetypes.ClearingAccountFoundation,
				RecipientAddresses: []string{
					"testcore17rzcx6c37ypp8m6hrl6pyhhl3mfp2s5d6xhyyl23vsj3laclhpxqx89alr",
					"testcore19kswr87wtx95gphrmkr785595untfmf9fd4dag4chthl5fxnkuhsc3v7gk",
					"testcore16vth8ad0anjqpqqmwpfzc09c3w2tj4492vz6zzwr0xk9st6ca0tsm3nyv4",
					"testcore1hmgca4jxfuxmg8lja9sdet307cldcpm4f6ttacurx8d4d03jz2aq5jgzwm",
					"testcore1c67vg6kueqn5wd78vu0drfqtq7rurhulngyulc9qc0glk9l36vsq4v8h44",
					"testcore1590eujlxwl7qsllu77xeu9v8ryuupkn6s0q5tlyp2e8ea6wa39tqpjy9sx",
					"testcore1xc505dp7agzg7rnzzfzmllmqckw32et0rdnpwck3cplylgplj9hqwnnnvp",
					"testcore13qrxcrsj69kztezt8pepmjeemen5tzxyx3wkg8mtllg2sexwgp2qs9rg2g",
					"testcore1kxsc00mvmhx4mqklzhzze3nr56d0ejclpcda3nf8e6cqcap9mvzq2v6gzk",
					"testcore120xxdn7hydfc8j2aak902zwlmuh9px465ft5jraj7l6qy5ksws4se0ucz7",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountAlliance,
				RecipientAddresses: []string{
					"testcore1csd2z5ycyvfumnjdr7qsgw2r0y9uc7nsk4a4596ej275rg0lzwrqr5g4yy",
					"testcore13egmenzagvcfnldcupxg5zfx5rgjrq44ugzewugku4l7e4jtmvns28sja8",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountPartnership,
				RecipientAddresses: []string{
					"testcore1ludesr02ls9gjv4ufzg9kwypdn8uxvxmk65hqznxnf46hkfcsffqx4ktqv",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountInvestors,
				RecipientAddresses: []string{
					"testcore16hu0xamesjwemrw4u3tpp23dkv3y2htgxvd2k942v3ekus2gsj5qsenwy3",
				},
			},
			{
				ClearingAccount: psetypes.ClearingAccountTeam,
				RecipientAddresses: []string{
					"testcore1lurev2l3g5pecey8lgywxw8wqvs4zupxqvmw4twmr9s8jlll6pgscmsu38",
				},
			},
		}
	case string(constant.ChainIDDev):
		recipientAddress := "devcore17we2jgjyxexcz8rg29dn622axt7s9l263fl0zt"
		for _, clearingAccount := range psetypes.GetNonCommunityClearingAccounts() {
			mappings = append(mappings, psetypes.ClearingAccountMapping{
				ClearingAccount:    clearingAccount,
				RecipientAddresses: []string{recipientAddress},
			})
		}
	default:
		return nil, errorsmod.Wrapf(psetypes.ErrInvalidInput, "unknown chain id: %s", chainID)
	}

	return mappings, nil
}

func TestPSEDistribution(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)

	// Epsilon tolerances for distribution verification
	const (
		epsilonNormal     = 0.05 // Normal delegators
		epsilonReIncluded = 0.06 // Re-included delegators have higher variance due to validator earnings
	)

	// ============================================================
	// SETUP: Create 4 delegators with progressive amounts
	// ============================================================
	delegators := make([]sdk.AccAddress, 4)
	var validatorAddress string

	validatorsResponse, err := stakingClient.Validators(
		ctx, &stakingtypes.QueryValidatorsRequest{Status: stakingtypes.Bonded.String()},
	)
	requireT.NoError(err)
	validator := validatorsResponse.Validators[0]
	validatorAddress = validator.OperatorAddress

	for i := range 4 {
		delegateAmount := sdkmath.NewInt(100_000_000 * int64(i+1))
		acc := chain.GenAccount()
		delegators[i] = acc

		chain.FundAccountWithOptions(ctx, t, acc, integration.BalancesOptions{
			Messages: []sdk.Msg{&stakingtypes.MsgDelegate{}, &stakingtypes.MsgUndelegate{}},
			Amount:   delegateAmount,
		})

		_, err = client.BroadcastTx(
			ctx,
			chain.ClientContext.WithFromAddress(acc),
			chain.TxFactory().WithGas(chain.GasLimitByMsgs(&stakingtypes.MsgDelegate{})),
			&stakingtypes.MsgDelegate{
				DelegatorAddress: acc.String(),
				ValidatorAddress: validator.OperatorAddress,
				Amount:           sdk.NewCoin(chain.ChainSettings.Denom, delegateAmount),
			},
		)
		requireT.NoError(err)
	}

	// ============================================================
	// SETUP: Configure 3 distributions and exclude 4th delegator
	// ============================================================
	allocationAmount := sdkmath.NewInt(100_000_000_000_000)
	allocations := make([]psetypes.ClearingAccountAllocation, 0)
	for _, clearingAccount := range psetypes.GetAllClearingAccounts() {
		allocations = append(allocations, psetypes.ClearingAccountAllocation{
			ClearingAccount: clearingAccount,
			Amount:          allocationAmount,
		})
	}

	govParams, err := chain.Governance.QueryGovParams(ctx)
	requireT.NoError(err)
	distributionStartTime := time.Now().Add(10 * time.Second).Add(*govParams.ExpeditedVotingPeriod)

	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil, "-", "-", "-", govtypesv1.OptionYes,
		&psetypes.MsgUpdateDistributionSchedule{
			Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			Schedule: []psetypes.ScheduledDistribution{
				{Timestamp: uint64(distributionStartTime.Add(30 * time.Second).Unix()), Allocations: allocations},
				{Timestamp: uint64(distributionStartTime.Add(60 * time.Second).Unix()), Allocations: allocations},
				{Timestamp: uint64(distributionStartTime.Add(90 * time.Second).Unix()), Allocations: allocations},
			},
		},
		&psetypes.MsgUpdateClearingAccountMappings{
			Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			Mappings: func() []psetypes.ClearingAccountMapping {
				m, err := defaultClearingAccountMappings(chain.ChainSettings.ChainID)
				requireT.NoError(err)
				return m
			}(),
		},
		&psetypes.MsgUpdateExcludedAddresses{
			Authority:      authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			AddressesToAdd: []string{delegators[3].String()},
		},
	)

	excludedDelegator := delegators[3].String()

	// ============================================================
	// DISTRIBUTION 1: Verify excluded delegator receives NO rewards
	// ============================================================
	t.Log("=== Distribution 1: Excluded delegator should receive nothing ===")

	header, err := chain.LatestBlockHeader(ctx)
	requireT.NoError(err)
	height := header.Height

	height, events, err := awaitScheduledDistributionEvent(ctx, chain, height)
	requireT.NoError(err)
	t.Logf("Distribution 1 at height: %d", height)

	scheduledDistributions, err := getScheduledDistribution(ctx, chain)
	requireT.NoError(err)
	requireT.Len(scheduledDistributions, 2)

	balancesBefore, scoresBefore, totalScore := getAllDelegatorInfo(ctx, t, chain, height-1)
	balancesAfter, _, _ := getAllDelegatorInfo(ctx, t, chain, height)

	// Excluded delegator should receive nothing
	requireT.Equal(balancesBefore[excludedDelegator], balancesAfter[excludedDelegator],
		"Excluded delegator should NOT receive rewards")
	requireT.Nil(events.find(excludedDelegator), "Excluded delegator should NOT have event")
	t.Logf("Excluded delegator correctly received no rewards")

	// Other delegators should receive correct rewards
	for _, delegator := range delegators[:3] { // First 3 delegators
		addr := delegator.String()
		increased := balancesAfter[addr].Sub(balancesBefore[addr])
		requireT.True(increased.IsPositive())

		expected := allocationAmount.Mul(scoresBefore[addr]).Quo(totalScore)
		requireT.InEpsilon(expected.Int64(), increased.Int64(), epsilonNormal)
		requireT.NotNil(events.find(addr))
	}

	// ============================================================
	// RE-INCLUSION: Remove delegator from exclusion list
	// ============================================================
	t.Log("=== Re-including previously excluded delegator ===")

	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil, "-", "-", "-", govtypesv1.OptionYes,
		&psetypes.MsgUpdateExcludedAddresses{
			Authority:         authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			AddressesToRemove: []string{excludedDelegator},
		},
	)
	t.Logf("Delegator re-included, should receive rewards in next distribution")

	// ============================================================
	// DISTRIBUTION 2: Verify re-included delegator receives rewards
	// ============================================================
	t.Log("=== Distribution 2: Re-included delegator should receive rewards ===")

	height, events, err = awaitScheduledDistributionEvent(ctx, chain, height)
	requireT.NoError(err)
	t.Logf("Distribution 2 at height: %d", height)

	scheduledDistributions, err = getScheduledDistribution(ctx, chain)
	requireT.NoError(err)
	requireT.Len(scheduledDistributions, 1)

	balancesBefore, scoresBefore, totalScore = getAllDelegatorInfo(ctx, t, chain, height-1)
	balancesAfter, _, _ = getAllDelegatorInfo(ctx, t, chain, height)

	// Re-included delegator should now receive rewards
	reIncludedIncrease := balancesAfter[excludedDelegator].Sub(balancesBefore[excludedDelegator])
	if reIncludedIncrease.IsPositive() {
		expected := allocationAmount.Mul(scoresBefore[excludedDelegator]).Quo(totalScore)
		requireT.InEpsilon(expected.Int64(), reIncludedIncrease.Int64(), epsilonReIncluded)
		requireT.NotNil(events.find(excludedDelegator))
		t.Logf("Re-included delegator received rewards: %s", reIncludedIncrease.String())
	} else {
		t.Logf("Re-included delegator received no rewards yet (score may not have accumulated)")
	}

	// Other delegators should still receive correct rewards
	for _, delegator := range delegators[:3] {
		addr := delegator.String()
		increased := balancesAfter[addr].Sub(balancesBefore[addr])
		requireT.True(increased.IsPositive())

		expected := allocationAmount.Mul(scoresBefore[addr]).Quo(totalScore)
		requireT.InEpsilon(expected.Int64(), increased.Int64(), epsilonNormal)
		requireT.NotNil(events.find(addr))
	}

	// ============================================================
	// DISTRIBUTION 3: Verify continued rewards
	// ============================================================
	t.Log("=== Distribution 3: All delegators receive rewards ===")

	height, events, err = awaitScheduledDistributionEvent(ctx, chain, height)
	requireT.NoError(err)
	t.Logf("Distribution 3 at height: %d", height)

	scheduledDistributions, err = getScheduledDistribution(ctx, chain)
	requireT.NoError(err)
	requireT.Len(scheduledDistributions, 0)

	balancesBefore, scoresBefore, totalScore = getAllDelegatorInfo(ctx, t, chain, height-1)
	balancesAfter, _, _ = getAllDelegatorInfo(ctx, t, chain, height)

	// All delegators (including re-included) should receive rewards
	for _, delegator := range delegators {
		addr := delegator.String()
		increased := balancesAfter[addr].Sub(balancesBefore[addr])
		requireT.True(increased.IsPositive(), "Delegator %s should receive rewards", addr)

		expected := allocationAmount.Mul(scoresBefore[addr]).Quo(totalScore)
		epsilon := epsilonNormal
		if addr == excludedDelegator {
			epsilon = epsilonReIncluded
		}
		requireT.InEpsilon(expected.Int64(), increased.Int64(), epsilon)
		requireT.NotNil(events.find(addr))
	}

	// ============================================================
	// UNDELEGATION TEST: Re-included delegator can fully undelegate
	// ============================================================
	t.Log("=== Testing full undelegation for re-included delegator ===")

	delResp, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: excludedDelegator,
	})
	requireT.NoError(err)
	requireT.Len(delResp.DelegationResponses, 1)

	currentDelegation := delResp.DelegationResponses[0].Balance.Amount
	t.Logf("Current delegation: %s", currentDelegation.String())

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegators[3]),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(&stakingtypes.MsgUndelegate{})),
		&stakingtypes.MsgUndelegate{
			DelegatorAddress: excludedDelegator,
			ValidatorAddress: validatorAddress,
			Amount:           sdk.NewCoin(chain.ChainSettings.Denom, currentDelegation),
		},
	)
	requireT.NoError(err, "Re-included delegator should be able to undelegate full amount")

	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 1))

	delRespAfter, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: excludedDelegator,
	})
	requireT.NoError(err)
	requireT.Len(delRespAfter.DelegationResponses, 0, "Should have zero delegations after full undelegation")

	t.Logf("Re-included delegator successfully undelegated full amount (%s)", currentDelegation.String())
}

func TestPSEDisableDistributions(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	msgDisableDistributions := &psetypes.MsgDisableDistributions{
		Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
	}
	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil,
		"-", "-", "-", govtypesv1.OptionYes,
		msgDisableDistributions,
	)

	scheduledDistributions, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.True(scheduledDistributions.DisableDistributions, "distributions should be disabled")
}

// TestPSEScore_DelegationFlow tests the end-to-end delegation flow and score accumulation.
func TestPSEScore_DelegationFlow(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	customParamsClient := customparamstypes.NewQueryClient(chain.ClientContext)

	// Get staking params
	customStakingParams, err := customParamsClient.StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)

	validatorStakingAmount := customStakingParams.Params.MinSelfDelegation.Mul(sdkmath.NewInt(2))

	// Create validator
	_, validatorAddress, deactivateValidator, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator()

	// Create delegator
	delegator := chain.GenAccount()
	delegateAmount := sdkmath.NewInt(1_000_000)

	chain.FundAccountWithOptions(ctx, t, delegator, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&stakingtypes.MsgDelegate{},
		},
		Amount: delegateAmount,
	})

	// Query initial score (should be zero)
	initialResp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.True(initialResp.Score.IsZero(), "initial score should be zero")

	// Delegate coins
	delegateMsg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(delegateAmount),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg)),
		delegateMsg,
	)
	requireT.NoError(err)

	t.Logf("Delegation executed, delegated %s to validator %s", delegateAmount.String(), validatorAddress.String())

	// Wait for blocks to accumulate score
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 2))

	// Query score after delegation
	finalResp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(finalResp)

	t.Logf("Score after delegation: %s", finalResp.Score.String())

	// Score should be positive after delegation and block accumulation
	requireT.True(finalResp.Score.IsPositive(), "score should be positive after delegation")
}

// TestPSEScore_MultipleDelegations tests score calculation with multiple delegation transactions.
// This test verifies that scores accumulate correctly when delegating to multiple validators.
func TestPSEScore_MultipleDelegations(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	customParamsClient := customparamstypes.NewQueryClient(chain.ClientContext)

	// Get staking params
	customStakingParams, err := customParamsClient.StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)

	validatorStakingAmount := customStakingParams.Params.MinSelfDelegation.Mul(sdkmath.NewInt(2))

	// Create two validators
	_, validator1Address, deactivateValidator1, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator1()

	_, validator2Address, deactivateValidator2, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator2()

	// Create delegator
	delegator := chain.GenAccount()
	delegateAmount1 := sdkmath.NewInt(500_000)
	delegateAmount2 := sdkmath.NewInt(300_000)
	totalAmount := delegateAmount1.Add(delegateAmount2)

	chain.FundAccountWithOptions(ctx, t, delegator, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&stakingtypes.MsgDelegate{},
			&stakingtypes.MsgDelegate{},
		},
		Amount: totalAmount,
	})

	// Delegate to first validator
	delegateMsg1 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator1Address.String(),
		Amount:           chain.NewCoin(delegateAmount1),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg1)),
		delegateMsg1,
	)
	requireT.NoError(err)

	t.Logf("First delegation executed: %s to validator %s", delegateAmount1.String(), validator1Address.String())

	// Delegate to second validator
	delegateMsg2 := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validator2Address.String(),
		Amount:           chain.NewCoin(delegateAmount2),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg2)),
		delegateMsg2,
	)
	requireT.NoError(err)

	t.Logf("Second delegation executed: %s to validator %s", delegateAmount2.String(), validator2Address.String())

	// Wait for some blocks to accumulate score
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 2))

	// Query score - should account for both delegations
	resp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(resp)

	t.Logf("Score with multiple delegations: %s", resp.Score.String())

	// Score should be positive after delegations and block accumulation
	requireT.True(resp.Score.IsPositive(), "score should be positive after delegations")
}

// TestPSEScore_UndelegationFlow tests the end-to-end undelegation flow and score behavior.
func TestPSEScore_UndelegationFlow(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	customParamsClient := customparamstypes.NewQueryClient(chain.ClientContext)

	// Get staking params
	customStakingParams, err := customParamsClient.StakingParams(ctx, &customparamstypes.QueryStakingParamsRequest{})
	requireT.NoError(err)

	validatorStakingAmount := customStakingParams.Params.MinSelfDelegation.Mul(sdkmath.NewInt(2))

	// Create validator
	_, validatorAddress, deactivateValidator, err := chain.CreateValidator(
		ctx, t, validatorStakingAmount, validatorStakingAmount,
	)
	requireT.NoError(err)
	defer deactivateValidator()

	// Create delegator
	delegator := chain.GenAccount()
	delegateAmount := sdkmath.NewInt(1_000_000)
	undelegateAmount := sdkmath.NewInt(500_000)

	chain.FundAccountWithOptions(ctx, t, delegator, integration.BalancesOptions{
		Messages: []sdk.Msg{
			&stakingtypes.MsgDelegate{},
			&stakingtypes.MsgUndelegate{},
			&stakingtypes.MsgUndelegate{},
		},
		Amount: delegateAmount,
	})

	// Delegate coins
	delegateMsg := &stakingtypes.MsgDelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(delegateAmount),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg)),
		delegateMsg,
	)
	requireT.NoError(err)

	t.Logf("Delegation executed: %s", delegateAmount.String())

	// Wait for blocks to accumulate some score
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 2))

	// Query score after delegation
	scoreAfterDelegate, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	t.Logf("Score after delegation: %s", scoreAfterDelegate.Score.String())

	// Verify delegation exists
	delResp, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delegator.String(),
	})
	requireT.NoError(err)
	requireT.Len(delResp.DelegationResponses, 1)
	requireT.Equal(delegateAmount, delResp.DelegationResponses[0].Balance.Amount)

	// Undelegate some coins
	undelegateMsg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(undelegateAmount),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(undelegateMsg)),
		undelegateMsg,
	)
	requireT.NoError(err)

	t.Logf("Undelegation executed: %s", undelegateAmount.String())

	// Wait for a block
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 1))

	// Query score after undelegation
	scoreAfterUndelegate, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)
	requireT.NotNil(scoreAfterUndelegate)

	t.Logf("Score after undelegation: %s", scoreAfterUndelegate.Score.String())

	// Score should still be positive and should have increased
	requireT.True(scoreAfterUndelegate.Score.IsPositive(), "score should be positive")
	requireT.True(scoreAfterUndelegate.Score.GT(scoreAfterDelegate.Score), "score should increase after more blocks")

	// Verify remaining delegation
	delRespAfter, err := stakingClient.DelegatorDelegations(ctx, &stakingtypes.QueryDelegatorDelegationsRequest{
		DelegatorAddr: delegator.String(),
	})
	requireT.NoError(err)
	requireT.Len(delRespAfter.DelegationResponses, 1)
	expectedRemaining := delegateAmount.Sub(undelegateAmount)
	requireT.Equal(expectedRemaining, delRespAfter.DelegationResponses[0].Balance.Amount)

	// Now undelegate the remaining amount
	undelegateRemainingMsg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: delegator.String(),
		ValidatorAddress: validatorAddress.String(),
		Amount:           chain.NewCoin(expectedRemaining),
	}

	_, err = client.BroadcastTx(
		ctx,
		chain.ClientContext.WithFromAddress(delegator),
		chain.TxFactory().WithGas(chain.GasLimitByMsgs(undelegateRemainingMsg)),
		undelegateRemainingMsg,
	)
	requireT.NoError(err)

	t.Logf("Full undelegation executed: %s", expectedRemaining.String())

	// Wait for a block
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 1))

	// Query score after full undelegation
	scoreAfterFullUndelegate, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)

	t.Logf("Score after full undelegation: %s", scoreAfterFullUndelegate.Score.String())

	// Wait for more blocks
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 3))

	// Query score again - it should not have increased since there's no active delegation
	scoreAfterWaiting, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
		Address: delegator.String(),
	})
	requireT.NoError(err)

	t.Logf("Score after waiting with no delegation: %s", scoreAfterWaiting.Score.String())

	// Score should not increase after full undelegation
	requireT.Equal(
		scoreAfterFullUndelegate.Score.String(), scoreAfterWaiting.Score.String(),
		"score should not increase after full undelegation")
}

// TestPSEQueryScore_AddressWithoutDelegation tests querying scores for addresses that have no delegation.
func TestPSEQueryScore_AddressWithoutDelegation(t *testing.T) {
	t.Parallel()

	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)

	// Create multiple accounts that have never delegated
	addresses := []sdk.AccAddress{
		chain.GenAccount(),
		chain.GenAccount(),
		chain.GenAccount(),
	}

	// Query score for each address - all should be zero
	for i, addr := range addresses {
		scoreResp, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
			Address: addr.String(),
		})
		requireT.NoError(err)
		requireT.NotNil(scoreResp)

		t.Logf("Address %d (%s): Score = %s", i, addr.String(), scoreResp.Score.String())

		// Score should be zero for addresses with no delegation
		requireT.True(scoreResp.Score.IsZero(), "score should be zero for address with no delegation")
	}
}

func getAllDelegatorInfo(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
	height int64,
) (map[string]sdkmath.Int, map[string]sdkmath.Int, sdkmath.Int) {
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	requireT := require.New(t)

	ctx = metadata.AppendToOutgoingContext(ctx, grpctypes.GRPCBlockHeightHeader, strconv.FormatInt(height, 10))

	validatorsResponse, err := stakingClient.Validators(
		ctx, &stakingtypes.QueryValidatorsRequest{Status: stakingtypes.Bonded.String()},
	)
	requireT.NoError(err)

	allDelegatorScores := make(map[string]sdkmath.Int)
	allDelegatorAmounts := make(map[string]sdkmath.Int)
	totalScore := sdkmath.NewInt(0)
	for _, val := range validatorsResponse.Validators {
		delegationsResp, err := stakingClient.ValidatorDelegations(ctx, &stakingtypes.QueryValidatorDelegationsRequest{
			ValidatorAddr: val.OperatorAddress,
		})
		requireT.NoError(err)
		for _, delegation := range delegationsResp.DelegationResponses {
			_, exists := allDelegatorScores[delegation.Delegation.DelegatorAddress]
			if exists {
				continue
			}

			pseScore, err := pseClient.Score(ctx, &psetypes.QueryScoreRequest{
				Address: delegation.Delegation.DelegatorAddress,
			})
			requireT.NoError(err)
			allDelegatorScores[delegation.Delegation.DelegatorAddress] = pseScore.Score
			allDelegatorAmounts[delegation.Delegation.DelegatorAddress] = delegation.Balance.Amount
			totalScore = totalScore.Add(pseScore.Score)
		}
	}

	return allDelegatorAmounts, allDelegatorScores, totalScore
}

type communityDistributedEvent []*psetypes.EventCommunityDistributed

func (e communityDistributedEvent) find(delegatorAddress string) *psetypes.EventCommunityDistributed {
	for _, event := range e {
		if event.DelegatorAddress == delegatorAddress {
			return event
		}
	}
	return nil
}

func awaitScheduledDistributionEvent(
	ctx context.Context,
	chain integration.TXChain,
	startHeight int64,
) (int64, communityDistributedEvent, error) {
	var observedHeight int64
	err := chain.AwaitState(ctx, func(ctx context.Context) error {
		query := fmt.Sprintf("tx.pse.v1.EventAllocationDistributed.mode='EndBlock' AND block.height>%d", startHeight)
		blocks, err := chain.ClientContext.RPCClient().BlockSearch(ctx, query, nil, nil, "")
		if err != nil {
			return err
		}
		if blocks.TotalCount == 0 {
			return errors.New("no blocks found")
		}

		observedHeight = blocks.Blocks[0].Block.Height
		return nil
	},
		integration.WithAwaitStateTimeout(40*time.Second),
	)
	if err != nil {
		return 0, nil, err
	}

	results, err := chain.ClientContext.RPCClient().BlockResults(ctx, &observedHeight)
	if err != nil {
		return 0, nil, err
	}
	// we have to remove the mode attribute from the events because it is not part of the typed event and
	// is added by cosmos-sdk, otherwise parsing the events will fail.
	events := removeAttributeFromEvent(results.FinalizeBlockEvents, "mode")
	communityDistributedEvents, err := event.FindTypedEvents[*psetypes.EventCommunityDistributed](events)
	if err != nil {
		return 0, nil, err
	}
	return observedHeight, communityDistributedEvents, nil
}

func getScheduledDistribution(
	ctx context.Context,
	chain integration.TXChain,
) ([]psetypes.ScheduledDistribution, error) {
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	pseResponse, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	if err != nil {
		return nil, err
	}
	return pseResponse.ScheduledDistributions, nil
}

func removeAttributeFromEvent(events []tmtypes.Event, key string) []tmtypes.Event {
	newEvents := make([]tmtypes.Event, 0, len(events))
	for _, event := range events {
		for i, attribute := range event.Attributes {
			if attribute.Key == key {
				event.Attributes = append(event.Attributes[:i], event.Attributes[i+1:]...)
			}
		}
		newEvents = append(newEvents, event)
	}
	return newEvents
}
