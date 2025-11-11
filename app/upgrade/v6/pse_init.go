package v6

import (
	"context"
	"time"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

const (
	// DefaultDistributionStartTime is the default start time for the distribution schedule
	// This is Dec 1, 2025, 00:00:00 UTC.
	// TODO: Confirm the start time.
	DefaultDistributionStartTime = 1764547200

	// InitialTotalMint is the total amount to mint during initialization
	// 100 billion tokens (in base denomination units).
	InitialTotalMint = 100_000_000_000_000_000

	// TotalAllocationMonths is the total number of months for the allocation schedule.
	TotalAllocationMonths = 84
)

// InitialFundAllocation defines the token funding allocation for a module account during initialization.
type InitialFundAllocation struct {
	ModuleAccount string
	Percentage    sdkmath.LegacyDec // Percentage of total mint amount (0-1)
}

// DefaultInitialFundAllocations returns the default token funding percentages for module accounts.
// These percentages should sum to 1.0 (100%).
// Non-Community clearing accounts receive tokens but are not included in the distribution schedule.
func DefaultInitialFundAllocations() []InitialFundAllocation {
	return []InitialFundAllocation{
		{
			ModuleAccount: psetypes.ClearingAccountCommunity,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.40"), // 40% - receives tokens but not in schedule
		},
		{
			ModuleAccount: psetypes.ClearingAccountFoundation,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.30"), // 30%
		},
		{
			ModuleAccount: psetypes.ClearingAccountAlliance,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.20"), // 20%
		},
		{
			ModuleAccount: psetypes.ClearingAccountPartnership,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.03"), // 3%
		},
		{
			ModuleAccount: psetypes.ClearingAccountInvestors,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.05"), // 5%
		},
		{
			ModuleAccount: psetypes.ClearingAccountTeam,
			Percentage:    sdkmath.LegacyMustNewDecFromStr("0.02"), // 2%
		},
	}
}

// FilterNonCommunityAllocations returns only the fund allocations for clearing accounts
// all of the clearing accounts except for Community.
func FilterNonCommunityAllocations(fundAllocations []InitialFundAllocation) []InitialFundAllocation {
	var distributionAllocations []InitialFundAllocation
	for _, allocation := range fundAllocations {
		if allocation.ModuleAccount != psetypes.ClearingAccountCommunity {
			distributionAllocations = append(distributionAllocations, allocation)
		}
	}
	return distributionAllocations
}

// DefaultClearingAccountMappings returns the default clearing account mappings.
// Community clearing account is not included in the mappings.
// TODO: Replace placeholder addresses with actual recipient addresses provided by management.
var DefaultClearingAccountMappings = func() []psetypes.ClearingAccountMapping {
	return []psetypes.ClearingAccountMapping{
		{
			ClearingAccount:  psetypes.ClearingAccountFoundation,
			RecipientAddress: "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		},
		{
			ClearingAccount:  psetypes.ClearingAccountAlliance,
			RecipientAddress: "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		},
		{
			ClearingAccount:  psetypes.ClearingAccountPartnership,
			RecipientAddress: "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		},
		{
			ClearingAccount:  psetypes.ClearingAccountInvestors,
			RecipientAddress: "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		},
		{
			ClearingAccount:  psetypes.ClearingAccountTeam,
			RecipientAddress: "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		},
		// Note: Community clearing account is not included in the mappings
	}
}

// InitPSEAllocationsAndSchedule initializes the PSE module by creating a distribution schedule,
// minting tokens, and distributing them to module accounts. The schedule defines
// how tokens will be gradually released over time from module accounts to recipients.
// Should be called once during the software upgrade that introduces the PSE module.
func InitPSEAllocationsAndSchedule(
	ctx context.Context,
	pseKeeper pskeeper.Keeper,
	bankKeeper psetypes.BankKeeper,
	stakingKeeper psetypes.StakingQuerier,
) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Initialize parameters using predefined constants
	fundAllocations := DefaultInitialFundAllocations()
	scheduleStartTime := uint64(DefaultDistributionStartTime)
	totalMintAmount := sdkmath.NewInt(InitialTotalMint)

	// Retrieve the chain's native token denomination from staking params
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to get staking params: %v", err)
	}

	// Ensure fund allocation percentages are valid and sum to exactly 100%
	if err := validateFundAllocations(fundAllocations); err != nil {
		return errorsmod.Wrap(err, "invalid fund allocations")
	}

	// Step 1: Validate all module account names
	// All accounts (including non-Community clearing accounts) can receive tokens
	// but only non-Community clearing accounts will be in the distribution schedule
	for _, allocation := range fundAllocations {
		perms := psetypes.GetClearingAccountPerms()
		if _, exists := perms[allocation.ModuleAccount]; !exists {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "invalid module account: %s", allocation.ModuleAccount)
		}
	}

	// Step 2: Filter to only non-Community clearing accounts (for schedule and mappings)
	distributionAllocations := FilterNonCommunityAllocations(fundAllocations)

	// Step 3: Create clearing account mappings (only for non-Community clearing accounts)
	// TODO: Replace placeholder addresses with actual recipient addresses provided by management.
	mappings := DefaultClearingAccountMappings()

	// Get authority (governance module address)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	if err := pseKeeper.UpdateClearingMappings(ctx, authority, mappings); err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to create clearing account mappings: %v", err)
	}

	// Step 4: Generate the n-month distribution schedule (only for non-Community clearing accounts)
	// This defines when and how much each non-Community clearing account will distribute to recipients
	schedule, err := CreateDistributionSchedule(distributionAllocations, totalMintAmount, scheduleStartTime)
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrScheduleCreationFailed, "%v", err)
	}

	// Step 5: Persist the schedule to blockchain state
	if err := pseKeeper.SaveDistributionSchedule(ctx, schedule); err != nil {
		return errorsmod.Wrapf(psetypes.ErrScheduleCreationFailed, "%v", err)
	}

	// Step 6: Mint and fund clearing accounts (all accounts, including non-Community clearing accounts)
	if err := MintAndFundClearingAccounts(ctx, bankKeeper, fundAllocations, totalMintAmount, bondDenom); err != nil {
		return err
	}

	sdkCtx.Logger().Info("initialization completed",
		"minted", totalMintAmount.String(),
		"denom", bondDenom,
		"fund_allocations", len(fundAllocations),
		"distribution_allocations", len(distributionAllocations),
		"periods", len(schedule),
	)

	return nil
}

// CreateDistributionSchedule generates a periodic distribution schedule over n months.
// Only fund allocations for non-Community clearing accounts are included in the schedule.
// Each distribution period allocates an equal portion (1/n) of each non-Community clearing account's total balance.
// Timestamps are calculated using Go's AddDate for proper Gregorian calendar handling.
// Returns the schedule without persisting it to state, making this a pure, testable function.
func CreateDistributionSchedule(
	distributionFundAllocations []InitialFundAllocation,
	totalMintAmount sdkmath.Int,
	startTime uint64,
) ([]psetypes.ScheduledDistribution, error) {
	if len(distributionFundAllocations) == 0 {
		return nil, psetypes.ErrNoModuleBalances
	}

	// Convert Unix timestamp to time.Time for date arithmetic
	startDateTime := time.Unix(int64(startTime), 0).UTC()

	// Pre-allocate slice with exact capacity for n distribution periods
	schedule := make([]psetypes.ScheduledDistribution, 0, TotalAllocationMonths)

	for month := range TotalAllocationMonths {
		// Calculate distribution timestamp by adding months to start time
		// AddDate handles month length variations and leap years correctly
		distributionDateTime := startDateTime.AddDate(0, month, 0)
		distributionTime := uint64(distributionDateTime.Unix())

		// Build allocations list for this distribution period
		// Only non-Community clearing accounts are included (Community clearing account is already filtered out)
		periodAllocations := make([]psetypes.ClearingAccountAllocation, 0, len(distributionFundAllocations))

		for _, allocation := range distributionFundAllocations {
			// Calculate total balance for this module account from percentage
			totalBalance := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()

			// Divide total balance equally across all distribution periods using integer division
			monthlyAmount := totalBalance.QuoRaw(TotalAllocationMonths)

			// Fail if balance is too small to distribute over n periods
			if monthlyAmount.IsZero() {
				return nil, errorsmod.Wrapf(
					psetypes.ErrInvalidInput,
					"clearing account %s: balance too small to divide into monthly distributions",
					allocation.ModuleAccount,
				)
			}

			periodAllocations = append(periodAllocations, psetypes.ClearingAccountAllocation{
				ClearingAccount: allocation.ModuleAccount,
				Amount:          monthlyAmount,
			})
		}

		if len(periodAllocations) == 0 {
			return nil, errorsmod.Wrapf(psetypes.ErrInvalidInput, "no allocations for distribution period %d", month)
		}

		// Add this distribution period to the schedule
		schedule = append(schedule, psetypes.ScheduledDistribution{
			Timestamp:   distributionTime,
			Allocations: periodAllocations,
		})
	}

	return schedule, nil
}

// MintAndFundClearingAccounts mints the total token supply and distributes it to clearing account modules.
// All accounts receive tokens.
func MintAndFundClearingAccounts(
	ctx context.Context,
	bankKeeper psetypes.BankKeeper,
	fundAllocations []InitialFundAllocation,
	totalMintAmount sdkmath.Int,
	denom string,
) error {
	// Mint the full token supply
	coinsToMint := sdk.NewCoins(sdk.NewCoin(denom, totalMintAmount))
	if err := bankKeeper.MintCoins(ctx, psetypes.ModuleName, coinsToMint); err != nil {
		return errorsmod.Wrap(err, "failed to mint coins")
	}

	// Distribute minted tokens from PSE module to all clearing account modules
	// All accounts receive tokens, including non-Community clearing accounts
	for _, allocation := range fundAllocations {
		// Calculate amount for this module account from percentage
		allocationAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()

		// Validate that allocation is not zero
		if allocationAmount.IsZero() {
			return errorsmod.Wrapf(
				psetypes.ErrInvalidInput,
				"module account %s: allocation rounds to zero",
				allocation.ModuleAccount,
			)
		}

		coinsToTransfer := sdk.NewCoins(sdk.NewCoin(denom, allocationAmount))
		if err := bankKeeper.SendCoinsFromModuleToModule(
			ctx,
			psetypes.ModuleName,
			allocation.ModuleAccount,
			coinsToTransfer,
		); err != nil {
			return errorsmod.Wrapf(psetypes.ErrTransferFailed, "to %s: %v", allocation.ModuleAccount, err)
		}
	}

	return nil
}

// validateFundAllocations verifies that all fund allocation entries are well-formed and percentages sum to exactly 1.0.
func validateFundAllocations(fundAllocations []InitialFundAllocation) error {
	if len(fundAllocations) == 0 {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "no fund allocations provided")
	}

	totalPercentage := sdkmath.LegacyZeroDec()
	for i, allocation := range fundAllocations {
		if allocation.ModuleAccount == "" {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "fund allocation %d: empty module account name", i)
		}

		if allocation.Percentage.IsNegative() {
			return errorsmod.Wrapf(
				psetypes.ErrInvalidInput,
				"fund allocation %d (%s): negative percentage",
				i, allocation.ModuleAccount,
			)
		}

		if allocation.Percentage.GT(sdkmath.LegacyOneDec()) {
			return errorsmod.Wrapf(
				psetypes.ErrInvalidInput,
				"fund allocation %d (%s): percentage %.2f exceeds 100%%",
				i, allocation.ModuleAccount, allocation.Percentage.MustFloat64()*100,
			)
		}

		totalPercentage = totalPercentage.Add(allocation.Percentage)
	}

	// Verify sum is exactly 1.0 (no tolerance - must be precise)
	if !totalPercentage.Equal(sdkmath.LegacyOneDec()) {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "total percentage must equal 1.0, got %s", totalPercentage.String())
	}

	return nil
}
