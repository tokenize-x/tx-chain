package keeper

import (
	"context"
	"time"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

const (
	// TotalDistributionMonths is the total number of months for the distribution schedule.
	TotalDistributionMonths = 84

	// DefaultDistributionStartTime is the default start time for the distribution schedule
	// This is January 1, 2026, 00:00:00 UTC.
	DefaultDistributionStartTime = 1735689600

	// BootstrapTotalMint is the total amount to mint during bootstrap
	// 100 billion tokens (in base denomination units).
	BootstrapTotalMint = 100_000_000_000
)

// BootstrapAllocation defines the initial allocation for a module account during bootstrap.
type BootstrapAllocation struct {
	ModuleAccount string
	Percentage    sdkmath.LegacyDec // Percentage of total mint amount (0-1)
}

// DefaultBootstrapAllocations returns the default allocation percentages for module accounts.
// These percentages should sum to 1.0 (100%).
func DefaultBootstrapAllocations() []BootstrapAllocation {
	return []BootstrapAllocation{
		{
			ModuleAccount: types.ModuleAccountTreasury,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.30"), // 30%
		},
		{
			ModuleAccount: types.ModuleAccountPartnership,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.20"), // 20%
		},
		{
			ModuleAccount: types.ModuleAccountFoundingPartner,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.15"), // 15%
		},
		{
			ModuleAccount: types.ModuleAccountTeam,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.20"), // 20%
		},
		{
			ModuleAccount: types.ModuleAccountInvestors,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.15"), // 15%
		},
	}
}

// PerformBootstrap mints a specified amount of tokens and distributes them to module accounts
// based on predefined percentages. It also creates an 84-month distribution schedule starting from
// a predefined time. This should be called during software upgrade.
//
// Parameters:
//   - ctx: The context for the operation
//   - totalMintAmount: The total amount to mint and distribute
//   - denom: The denomination of tokens to mint
//   - allocations: The allocation percentages for each module account (if nil, uses defaults)
//   - scheduleStartTime: The start time for the distribution schedule (if 0, uses default)
//
// Returns an error if minting, distribution, or schedule creation fails.
func (k Keeper) PerformBootstrap(
	ctx context.Context,
	totalMintAmount sdkmath.Int,
	denom string,
	allocations []BootstrapAllocation,
	scheduleStartTime uint64,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Use default allocations if none provided
	if allocations == nil {
		allocations = DefaultBootstrapAllocations()
	}

	// Use default start time if not provided
	if scheduleStartTime == 0 {
		scheduleStartTime = DefaultDistributionStartTime
	}

	// Validate allocations
	if err := validateAllocations(allocations); err != nil {
		return errorsmod.Wrapf(types.ErrInvalidAllocations, "%v", err)
	}

	// Mint the total amount to the PSE module account
	coinsToMint := sdk.NewCoins(sdk.NewCoin(denom, totalMintAmount))
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coinsToMint); err != nil {
		return errorsmod.Wrapf(types.ErrMintFailed, "%v", err)
	}

	sdkCtx.Logger().Info("bootstrap: minted tokens",
		"amount", totalMintAmount.String(),
		"denom", denom,
	)

	// Track module account balances for schedule creation
	moduleAccountBalances := make(map[string]sdkmath.Int)

	// Distribute to module accounts based on percentages
	for _, allocation := range allocations {
		// Validate module account name
		if !types.IsValidModuleAccountName(allocation.ModuleAccount) {
			return errorsmod.Wrapf(types.ErrInvalidModuleAccount, "module account: %s", allocation.ModuleAccount)
		}

		// Calculate allocation amount
		allocationAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		if allocationAmount.IsZero() {
			continue // Skip zero allocations
		}

		// Store balance for schedule creation
		moduleAccountBalances[allocation.ModuleAccount] = allocationAmount

		// Transfer from PSE module to target module account
		coinsToTransfer := sdk.NewCoins(sdk.NewCoin(denom, allocationAmount))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(
			ctx,
			types.ModuleName,
			allocation.ModuleAccount,
			coinsToTransfer,
		); err != nil {
			return errorsmod.Wrapf(types.ErrTransferFailed, "to %s: %v", allocation.ModuleAccount, err)
		}

		sdkCtx.Logger().Info("bootstrap: allocated tokens",
			"module_account", allocation.ModuleAccount,
			"amount", allocationAmount.String(),
			"percentage", allocation.Percentage.String(),
			"denom", denom,
		)
	}

	// Create 84-month distribution schedule
	if err := k.createDistributionSchedule(ctx, moduleAccountBalances, scheduleStartTime); err != nil {
		return errorsmod.Wrapf(types.ErrScheduleCreationFailed, "%v", err)
	}

	sdkCtx.Logger().Info("bootstrap: completed successfully",
		"total_minted", totalMintAmount.String(),
		"denom", denom,
		"num_allocations", len(allocations),
		"schedule_months", TotalDistributionMonths,
		"schedule_start", time.Unix(int64(scheduleStartTime), 0).UTC().Format(time.RFC3339),
	)

	return nil
}

// validateAllocations checks that all allocations are valid and sum to 1.0 (100%).
func validateAllocations(allocations []BootstrapAllocation) error {
	if len(allocations) == 0 {
		return types.ErrNoAllocations
	}

	totalPercentage := sdkmath.LegacyZeroDec()
	for i, allocation := range allocations {
		if allocation.ModuleAccount == "" {
			return errorsmod.Wrapf(types.ErrEmptyModuleAccount, "allocation %d", i)
		}

		if allocation.Percentage.IsNegative() {
			return errorsmod.Wrapf(types.ErrNegativePercentage, "allocation %d (%s)", i, allocation.ModuleAccount)
		}

		if allocation.Percentage.GT(sdkmath.LegacyOneDec()) {
			return errorsmod.Wrapf(types.ErrPercentageExceedsOne, "allocation %d (%s)", i, allocation.ModuleAccount)
		}

		totalPercentage = totalPercentage.Add(allocation.Percentage)
	}

	// Check that total percentage is exactly 1.0 (with small tolerance for rounding)
	if !totalPercentage.Equal(sdkmath.LegacyOneDec()) {
		return errorsmod.Wrapf(types.ErrInvalidTotalPercentage,
			"total percentage must equal 1.0, got %s", totalPercentage.String())
	}

	return nil
}

// createDistributionSchedule creates an 84-month distribution schedule based on module account balances.
// Each month distributes 1/84th of each module account's balance.
// Uses proper Gregorian calendar calculation respecting all date rules including leap years.
func (k Keeper) createDistributionSchedule(
	ctx context.Context,
	moduleAccountBalances map[string]sdkmath.Int,
	startTime uint64,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if len(moduleAccountBalances) == 0 {
		return types.ErrNoModuleBalances
	}

	// Get current params to append new schedule
	params, err := k.GetParams(ctx)
	if err != nil {
		return errorsmod.Wrapf(types.ErrParamsGet, "%v", err)
	}

	// Convert start time to Go time for proper date arithmetic
	startDateTime := time.Unix(int64(startTime), 0).UTC()

	// Create distribution periods for 84 months
	distributionPeriods := make([]types.DistributionPeriod, 0, TotalDistributionMonths)

	for month := range TotalDistributionMonths {
		// Add months using AddDate which properly handles Gregorian calendar rules
		// including varying month lengths and leap years
		distributionDateTime := startDateTime.AddDate(0, month, 0)
		distributionTime := uint64(distributionDateTime.Unix())

		// Create distributions for each module account
		distributions := make([]types.ModuleDistribution, 0, len(moduleAccountBalances))

		for moduleAccount, totalBalance := range moduleAccountBalances {
			// Calculate monthly distribution amount (1/84th of total)
			monthlyAmount := totalBalance.QuoRaw(TotalDistributionMonths)

			// Skip if amount is zero
			if monthlyAmount.IsZero() {
				continue
			}

			distributions = append(distributions, types.ModuleDistribution{
				ModuleAccount: moduleAccount,
				Amount:        monthlyAmount,
			})
		}

		// Only add period if there are distributions
		if len(distributions) > 0 {
			distributionPeriods = append(distributionPeriods, types.DistributionPeriod{
				DistributionTime: distributionTime,
				Distributions:    distributions,
			})

			// Add timestamp to pending timestamps queue
			if err := k.PendingTimestamps.Set(ctx, distributionTime); err != nil {
				return errorsmod.Wrapf(types.ErrPendingTimestampAdd, "month %d: %v", month, err)
			}
		}
	}

	// Calculate the end time (last distribution time)
	endDateTime := startDateTime.AddDate(0, TotalDistributionMonths-1, 0)

	// Update params with new distribution schedule
	params.DistributionSchedule = distributionPeriods
	if err := k.SetParams(ctx, params); err != nil {
		return errorsmod.Wrapf(types.ErrParamsSet, "%v", err)
	}

	sdkCtx.Logger().Info("bootstrap: created distribution schedule",
		"num_periods", len(distributionPeriods),
		"start_time", startDateTime.Format(time.RFC3339),
		"end_time", endDateTime.Format(time.RFC3339),
		"num_module_accounts", len(moduleAccountBalances),
	)

	return nil
}
