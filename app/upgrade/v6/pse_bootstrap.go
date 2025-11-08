package v6

import (
	"context"
	"time"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

const (
	// DefaultDistributionStartTime is the default start time for the distribution schedule
	// This is Dec 1, 2025, 00:00:00 UTC.
	DefaultDistributionStartTime = 1764547200

	// BootstrapTotalMint is the total amount to mint during bootstrap
	// 100 billion tokens (in base denomination units).
	BootstrapTotalMint = 100_000_000_000_000_000
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
			ModuleAccount: psetypes.ModuleAccountTreasury,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.30"), // 30%
		},
		{
			ModuleAccount: psetypes.ModuleAccountPartnership,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.20"), // 20%
		},
		{
			ModuleAccount: psetypes.ModuleAccountFoundingPartner,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.15"), // 15%
		},
		{
			ModuleAccount: psetypes.ModuleAccountTeam,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.20"), // 20%
		},
		{
			ModuleAccount: psetypes.ModuleAccountInvestors,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.15"), // 15%
		},
	}
}

// PerformBootstrap initializes the PSE module by creating a distribution schedule,
// minting tokens, and distributing them to module accounts. The schedule defines
// how tokens will be gradually released over time from module accounts to recipients.
// Should be called once during the software upgrade that introduces the PSE module.
func PerformBootstrap(
	ctx context.Context,
	pseKeeper pskeeper.Keeper,
	bankKeeper psetypes.BankKeeper,
	stakingKeeper psetypes.StakingKeeper,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Initialize bootstrap parameters using predefined constants
	allocations := DefaultBootstrapAllocations()
	scheduleStartTime := uint64(DefaultDistributionStartTime)
	totalMintAmount := sdkmath.NewInt(BootstrapTotalMint)

	// Retrieve the chain's native token denomination from staking params
	stakingParams, err := stakingKeeper.GetParams(ctx)
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to get staking params: %v", err)
	}
	denom := stakingParams.BondDenom

	// Ensure allocation percentages are valid and sum to exactly 100%
	if err := validateAllocations(allocations); err != nil {
		return errorsmod.Wrap(err, "invalid bootstrap allocations")
	}

	// Step 1: Convert allocation percentages to absolute token amounts
	moduleAccountBalances := make(map[string]sdkmath.Int)
	for _, allocation := range allocations {
		// Verify module account name is recognized by the PSE module
		if !psetypes.IsValidModuleAccountName(allocation.ModuleAccount) {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "invalid module account: %s", allocation.ModuleAccount)
		}

		// Apply percentage to total mint amount (truncates to integer)
		allocationAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()
		if allocationAmount.IsZero() {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "module account %s: allocation rounds to zero", allocation.ModuleAccount)
		}

		moduleAccountBalances[allocation.ModuleAccount] = allocationAmount
	}

	// Step 2: Verify sum of calculated amounts equals the mint constant
	// This catches rounding errors from percentage-to-integer conversion
	sumOfAllocations := sdkmath.ZeroInt()
	for _, amount := range moduleAccountBalances {
		sumOfAllocations = sumOfAllocations.Add(amount)
	}

	if !sumOfAllocations.Equal(totalMintAmount) {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput,
			"sum of module account allocations (%s) does not equal total mint amount (%s)",
			sumOfAllocations.String(), totalMintAmount.String())
	}

	// Step 3: Generate the n-month distribution schedule
	// This defines when and how much each module account will distribute to recipients
	schedule, err := pskeeper.CreateDistributionSchedule(moduleAccountBalances, scheduleStartTime)
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrScheduleCreationFailed, "%v", err)
	}

	// Step 4: Persist the schedule to blockchain state
	if err := pseKeeper.SaveDistributionSchedule(ctx, schedule); err != nil {
		return errorsmod.Wrapf(psetypes.ErrScheduleCreationFailed, "%v", err)
	}

	sdkCtx.Logger().Info("bootstrap: created and saved distribution schedule",
		"num_periods", len(schedule),
		"schedule_start", time.Unix(int64(scheduleStartTime), 0).UTC().Format(time.RFC3339),
	)

	// Step 5: Mint the total token supply for PSE allocations
	coinsToMint := sdk.NewCoins(sdk.NewCoin(denom, totalMintAmount))
	if err := bankKeeper.MintCoins(ctx, psetypes.ModuleName, coinsToMint); err != nil {
		return errorsmod.Wrap(err, "failed to mint coins")
	}

	sdkCtx.Logger().Info("bootstrap: minted tokens",
		"amount", totalMintAmount.String(),
		"denom", denom,
	)

	// Step 6: Distribute minted tokens from PSE module to clearing account modules
	for moduleAccount, amount := range moduleAccountBalances {
		// Transfer tokens to the module account that will gradually distribute to recipients
		coinsToTransfer := sdk.NewCoins(sdk.NewCoin(denom, amount))
		if err := bankKeeper.SendCoinsFromModuleToModule(
			ctx,
			psetypes.ModuleName,
			moduleAccount,
			coinsToTransfer,
		); err != nil {
			return errorsmod.Wrapf(psetypes.ErrTransferFailed, "to %s: %v", moduleAccount, err)
		}

		sdkCtx.Logger().Info("bootstrap: allocated tokens to module account",
			"module_account", moduleAccount,
			"amount", amount.String(),
			"denom", denom,
		)
	}

	// Calculate when the last distribution will occur
	endDateTime := time.Unix(int64(scheduleStartTime), 0).UTC().AddDate(0, psetypes.TotalAllocationMonths-1, 0)

	sdkCtx.Logger().Info("bootstrap: completed successfully",
		"total_minted", totalMintAmount.String(),
		"denom", denom,
		"num_allocations", len(allocations),
		"num_periods", len(schedule),
		"schedule_end", endDateTime.Format(time.RFC3339),
	)

	return nil
}

// validateAllocations verifies that all allocation entries are well-formed and percentages sum to exactly 1.0.
func validateAllocations(allocations []BootstrapAllocation) error {
	if len(allocations) == 0 {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "no allocations provided")
	}

	totalPercentage := sdkmath.LegacyZeroDec()
	for i, allocation := range allocations {
		if allocation.ModuleAccount == "" {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "allocation %d: empty module account name", i)
		}

		if allocation.Percentage.IsNegative() {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "allocation %d (%s): negative percentage", i, allocation.ModuleAccount)
		}

		if allocation.Percentage.GT(sdkmath.LegacyOneDec()) {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "allocation %d (%s): percentage %.2f exceeds 100%%", i, allocation.ModuleAccount, allocation.Percentage.MustFloat64()*100)
		}

		totalPercentage = totalPercentage.Add(allocation.Percentage)
	}

	// Verify sum is exactly 1.0 (no tolerance - must be precise)
	if !totalPercentage.Equal(sdkmath.LegacyOneDec()) {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "total percentage must equal 1.0, got %s", totalPercentage.String())
	}

	return nil
}
