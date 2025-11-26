//go:build integrationtests

package upgrade

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

type pseInitialDistribution struct {
	totalSupplyBefore     sdk.Coin
	clearingAccountBefore map[string]sdk.Coin
}

func (pid *pseInitialDistribution) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	// Get staking params to determine bond denom
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	stakingParams, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	requireT.NoError(err)
	bondDenom := stakingParams.Params.BondDenom

	// Get total supply before upgrade
	bankClient := banktypes.NewQueryClient(chain.ClientContext)
	supplyResp, err := bankClient.SupplyOf(ctx, &banktypes.QuerySupplyOfRequest{Denom: bondDenom})
	requireT.NoError(err)
	pid.totalSupplyBefore = supplyResp.Amount

	// Record clearing account balances before upgrade (should be zero or non-existent)
	pid.clearingAccountBefore = make(map[string]sdk.Coin)
	clearingAccounts := psetypes.GetAllClearingAccounts()
	for _, clearingAccount := range clearingAccounts {
		moduleAddr := authtypes.NewModuleAddress(clearingAccount)
		balanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
			Address: moduleAddr.String(),
			Denom:   bondDenom,
		})
		requireT.NoError(err)
		pid.clearingAccountBefore[clearingAccount] = *balanceResp.Balance
		t.Logf("Before upgrade - %s balance: %s", clearingAccount, balanceResp.Balance)
	}
}

func (pid *pseInitialDistribution) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	// Get staking params to determine bond denom
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	stakingParams, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	requireT.NoError(err)
	bondDenom := stakingParams.Params.BondDenom

	bankClient := banktypes.NewQueryClient(chain.ClientContext)

	pid.verifyTotalSupplyIncreaseAfter(ctx, t, bankClient, bondDenom)
	allocations := pid.verifyClearingAccountAllocations(ctx, t, bankClient, bondDenom)

	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	schedule := pid.verifyDistributionScheduleAfter(ctx, t, pseClient)
	pid.verifyDistributionTimestampsAfter(t, schedule)
	pid.verifyPeriodAllocationsAfter(t, allocations, schedule)
	pid.verifyClearingAccountMappingsAfter(ctx, t, pseClient)
	pid.verifyClearingAccountBalancesAfter(ctx, t, pseClient, len(allocations))

	t.Log("All distribution schedule and allocation verifications passed")
}

func (pid *pseInitialDistribution) verifyTotalSupplyIncreaseAfter(
	ctx context.Context,
	t *testing.T,
	bankClient banktypes.QueryClient,
	bondDenom string,
) {
	requireT := require.New(t)
	supplyResp, err := bankClient.SupplyOf(ctx, &banktypes.QuerySupplyOfRequest{Denom: bondDenom})
	requireT.NoError(err)
	totalSupplyAfter := supplyResp.Amount

	actualSupplyIncrease := totalSupplyAfter.Amount.Sub(pid.totalSupplyBefore.Amount)
	requireT.True(actualSupplyIncrease.GTE(sdkmath.NewInt(v6.InitialTotalMint)),
		"total supply should increase by at least InitialTotalMint (%s), actual increase: %s",
		sdkmath.NewInt(v6.InitialTotalMint), actualSupplyIncrease)

	t.Logf("Total supply before: %s", pid.totalSupplyBefore)
	t.Logf("Total supply after: %s", totalSupplyAfter)
	t.Logf("Supply increase: %s (expected at least: %s)", actualSupplyIncrease, sdkmath.NewInt(v6.InitialTotalMint))
}

func (pid *pseInitialDistribution) verifyClearingAccountAllocations(
	ctx context.Context,
	t *testing.T,
	bankClient banktypes.QueryClient,
	bondDenom string,
) []v6.InitialFundAllocation {
	requireT := require.New(t)
	allocations := v6.DefaultInitialFundAllocations()
	allClearingAccounts := psetypes.GetAllClearingAccounts()

	requireT.Len(allocations, len(allClearingAccounts),
		"allocations should match the number of all clearing accounts (%d)", len(allClearingAccounts))

	totalMintAmount := sdkmath.NewInt(v6.InitialTotalMint)
	totalVerified := sdkmath.ZeroInt()
	totalActualIncrease := sdkmath.ZeroInt()

	for _, allocation := range allocations {
		// Verify allocation corresponds to a known clearing account
		found := false
		for _, expectedAccount := range allClearingAccounts {
			if allocation.ClearingAccount == expectedAccount {
				found = true
				break
			}
		}
		requireT.True(found, "allocation for unknown clearing account: %s", allocation.ClearingAccount)

		expectedAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		totalVerified = totalVerified.Add(expectedAmount)

		moduleAddr := authtypes.NewModuleAddress(allocation.ClearingAccount)
		balanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
			Address: moduleAddr.String(),
			Denom:   bondDenom,
		})
		requireT.NoError(err)

		prevBalance := pid.clearingAccountBefore[allocation.ClearingAccount].Amount
		actualIncrease := balanceResp.Balance.Amount.Sub(prevBalance)
		totalActualIncrease = totalActualIncrease.Add(actualIncrease)

		requireT.Equal(expectedAmount.String(), actualIncrease.String(),
			"clearing account %s should have received exactly %s, got increase of %s",
			allocation.ClearingAccount, expectedAmount, actualIncrease)

		t.Logf("Clearing account %s: previous=%s, allocated=%s, current=%s, increase=%s",
			allocation.ClearingAccount, prevBalance, expectedAmount, balanceResp.Balance.Amount, actualIncrease)
	}

	requireT.Equal(totalMintAmount.String(), totalVerified.String(),
		"sum of allocations should equal total mint amount")
	requireT.Equal(totalMintAmount.String(), totalActualIncrease.String(),
		"sum of clearing account increases should equal total mint amount (%s), got %s",
		totalMintAmount, totalActualIncrease)

	t.Logf("Total mint amount: %s", totalMintAmount)
	t.Logf("Total clearing account increase: %s", totalActualIncrease)

	return allocations
}

func (pid *pseInitialDistribution) verifyDistributionScheduleAfter(
	ctx context.Context,
	t *testing.T,
	pseClient psetypes.QueryClient,
) []psetypes.ScheduledDistribution {
	requireT := require.New(t)
	scheduleResp, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.NotNil(scheduleResp)

	schedule := scheduleResp.ScheduledDistributions
	requireT.Len(schedule, v6.TotalAllocationMonths,
		"should have %d monthly distributions", v6.TotalAllocationMonths)

	t.Logf("Distribution schedule created with %d periods", len(schedule))
	return schedule
}

func (pid *pseInitialDistribution) verifyDistributionTimestampsAfter(
	t *testing.T,
	schedule []psetypes.ScheduledDistribution,
) {
	requireT := require.New(t)

	requireT.Equal(uint64(v6.DefaultDistributionStartTime), schedule[0].Timestamp,
		"first distribution should start at DefaultDistributionStartTime")

	firstDistTime := time.Unix(int64(schedule[0].Timestamp), 0).UTC()
	t.Logf("First distribution scheduled for: %s", firstDistTime.Format(time.RFC3339))

	lastDistTime := time.Unix(int64(schedule[v6.TotalAllocationMonths-1].Timestamp), 0).UTC()
	t.Logf("Last distribution scheduled for: %s", lastDistTime.Format(time.RFC3339))

	requireT.Greater(schedule[v6.TotalAllocationMonths-1].Timestamp,
		uint64(v6.DefaultDistributionStartTime),
		"last distribution should be after start time")

	var prevTime time.Time
	for i, period := range schedule {
		currentTime := time.Unix(int64(period.Timestamp), 0).UTC()

		requireT.Equal(1, currentTime.Day(),
			"period %d should be on the first day of the month, got day %d", i, currentTime.Day())

		if i > 0 {
			expectedTime := prevTime.AddDate(0, 1, 0)
			requireT.Equal(expectedTime.Year(), currentTime.Year(),
				"period %d: year should be %d, got %d", i, expectedTime.Year(), currentTime.Year())
			requireT.Equal(expectedTime.Month(), currentTime.Month(),
				"period %d: month should be %s, got %s", i, expectedTime.Month(), currentTime.Month())
		}

		prevTime = currentTime
	}
}

func (pid *pseInitialDistribution) verifyPeriodAllocationsAfter(
	t *testing.T,
	allocations []v6.InitialFundAllocation,
	schedule []psetypes.ScheduledDistribution,
) {
	requireT := require.New(t)
	totalMintAmount := sdkmath.NewInt(v6.InitialTotalMint)

	for i, period := range schedule {
		requireT.Len(period.Allocations, len(allocations),
			"period %d should have allocations for all %d clearing accounts", i, len(allocations))

		periodTotal := sdkmath.ZeroInt()
		for _, periodAlloc := range period.Allocations {
			var expectedTotal sdkmath.Int
			for _, initialAlloc := range allocations {
				if initialAlloc.ClearingAccount == periodAlloc.ClearingAccount {
					expectedTotal = initialAlloc.Percentage.MulInt(totalMintAmount).TruncateInt()
					break
				}
			}
			expectedMonthly := expectedTotal.QuoRaw(v6.TotalAllocationMonths)
			requireT.Equal(expectedMonthly.String(), periodAlloc.Amount.String(),
				"period %d: monthly amount for %s should be 1/%d of total",
				i, periodAlloc.ClearingAccount, v6.TotalAllocationMonths)

			periodTotal = periodTotal.Add(periodAlloc.Amount)
		}

		if i == 0 || i == v6.TotalAllocationMonths-1 {
			t.Logf("Period %d total allocation: %s", i, periodTotal)
		}
	}
}

func (pid *pseInitialDistribution) verifyClearingAccountMappingsAfter(
	ctx context.Context,
	t *testing.T,
	pseClient psetypes.QueryClient,
) {
	requireT := require.New(t)
	paramsResp, err := pseClient.Params(ctx, &psetypes.QueryParamsRequest{})
	requireT.NoError(err)
	requireT.NotNil(paramsResp)

	nonCommunityClearingAccounts := psetypes.GetNonCommunityClearingAccounts()
	expectedMappingCount := len(nonCommunityClearingAccounts)

	mappings := paramsResp.Params.ClearingAccountMappings
	requireT.Len(mappings, expectedMappingCount,
		"should have mappings for %d non-Community clearing accounts", expectedMappingCount)

	t.Logf("Created %d clearing account mappings", len(mappings))

	for _, clearingAccount := range nonCommunityClearingAccounts {
		found := false
		for _, mapping := range mappings {
			if mapping.ClearingAccount == clearingAccount {
				found = true
				requireT.NotEmpty(mapping.RecipientAddresses,
					"mapping for %s should have recipient addresses", clearingAccount)
				t.Logf("Clearing account %s has %d recipient address(es)",
					clearingAccount, len(mapping.RecipientAddresses))
				break
			}
		}
		requireT.True(found, "should have mapping for %s", clearingAccount)
	}

	for _, mapping := range mappings {
		requireT.NotEqual(psetypes.ClearingAccountCommunity, mapping.ClearingAccount,
			"Community should not have a mapping (uses score-based distribution)")
	}
}

func (pid *pseInitialDistribution) verifyClearingAccountBalancesAfter(
	ctx context.Context,
	t *testing.T,
	pseClient psetypes.QueryClient,
	expectedCount int,
) {
	requireT := require.New(t)
	allClearingAccounts := psetypes.GetAllClearingAccounts()

	clearingAccountBalancesResp, err := pseClient.ClearingAccountBalances(ctx,
		&psetypes.QueryClearingAccountBalancesRequest{})
	requireT.NoError(err)
	requireT.NotNil(clearingAccountBalancesResp)
	requireT.Len(clearingAccountBalancesResp.Balances, len(allClearingAccounts),
		"should return balances for all %d clearing accounts", len(allClearingAccounts))
	requireT.Len(clearingAccountBalancesResp.Balances, expectedCount,
		"expected count should match the number of all clearing accounts")

	t.Logf("Clearing account balances:")
	for _, balance := range clearingAccountBalancesResp.Balances {
		// Verify each balance corresponds to a known clearing account
		found := false
		for _, expectedAccount := range allClearingAccounts {
			if balance.ClearingAccount == expectedAccount {
				found = true
				break
			}
		}
		requireT.True(found, "balance for unknown clearing account: %s", balance.ClearingAccount)
		t.Logf("  %s: %s", balance.ClearingAccount, balance.Balance)
	}
}
