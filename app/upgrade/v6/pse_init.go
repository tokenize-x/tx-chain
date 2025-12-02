package v6

import (
	"context"
	"time"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/samber/lo"

	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	pskeeper "github.com/tokenize-x/tx-chain/v6/x/pse/keeper"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

const (
	// InitialTotalMint is the total amount to mint during initialization
	// 100 billion tokens (in base denomination units).
	InitialTotalMint = 100_000_000_000_000_000

	// TotalAllocationMonths is the total number of distribution months for the allocation schedule.
	// Each month is one calendar month (same day each month).
	// 84 months = exactly 7 years.
	TotalAllocationMonths = 84

	// MaxDistributionDay caps the day of month for distributions to ensure consistency
	// across all months. Set to 28 to guarantee all months (including February) have this day.
	MaxDistributionDay = 28
)

// InitialFundAllocation defines the token funding allocation for a module account during initialization.
type InitialFundAllocation struct {
	ClearingAccount string
	Percentage      sdkmath.LegacyDec // Percentage of total mint amount (0-1)
}

// DefaultInitialFundAllocations returns the default token funding percentages for module accounts.
// These percentages should sum to 1.0 (100%).
// All clearing accounts receive tokens and are included in the distribution schedule.
// Community uses score-based distribution, while others use direct recipient transfers.
func DefaultInitialFundAllocations() []InitialFundAllocation {
	return []InitialFundAllocation{
		{
			ClearingAccount: psetypes.ClearingAccountCommunity,
			Percentage:      sdkmath.LegacyMustNewDecFromStr("0.40"), // 40% - uses score-based distribution
		},
		{
			ClearingAccount: psetypes.ClearingAccountFoundation,
			Percentage:      sdkmath.LegacyMustNewDecFromStr("0.30"), // 30%
		},
		{
			ClearingAccount: psetypes.ClearingAccountAlliance,
			Percentage:      sdkmath.LegacyMustNewDecFromStr("0.20"), // 20%
		},
		{
			ClearingAccount: psetypes.ClearingAccountPartnership,
			Percentage:      sdkmath.LegacyMustNewDecFromStr("0.03"), // 3%
		},
		{
			ClearingAccount: psetypes.ClearingAccountInvestors,
			Percentage:      sdkmath.LegacyMustNewDecFromStr("0.05"), // 5%
		},
		{
			ClearingAccount: psetypes.ClearingAccountTeam,
			Percentage:      sdkmath.LegacyMustNewDecFromStr("0.02"), // 2%
		},
	}
}

// DefaultClearingAccountMappings returns the default clearing account mappings for the given chain ID.
// Community clearing account is not included in the mappings.
// Each clearing account has a single default recipient address.
// TODO: Replace placeholder addresses with actual recipient addresses provided by management.
func DefaultClearingAccountMappings(chainID string) ([]psetypes.ClearingAccountMapping, error) {
	// Determine the recipient address based on chain ID
	var recipientAddress string
	switch chainID {
	case string(constant.ChainIDMain):
		recipientAddress = "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5"
	case string(constant.ChainIDTest):
		recipientAddress = "testcore1dm4x48jqunpdh9h8sud30cwmtsghfuqascgqam"
	case string(constant.ChainIDDev):
		recipientAddress = "devcore17we2jgjyxexcz8rg29dn622axt7s9l263fl0zt"
	default:
		return nil, errorsmod.Wrapf(psetypes.ErrInvalidInput, "unknown chain id: %s", chainID)
	}

	// Create mappings for all non-Community clearing accounts
	// Each starts with a single default recipient (can be modified via governance)
	var mappings []psetypes.ClearingAccountMapping
	for _, clearingAccount := range psetypes.GetNonCommunityClearingAccounts() {
		mappings = append(mappings, psetypes.ClearingAccountMapping{
			ClearingAccount:    clearingAccount,
			RecipientAddresses: []string{recipientAddress},
		})
	}

	return mappings, nil
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
	allocations := DefaultInitialFundAllocations()
	// Use the upgrade date at 12:00:00 GMT as the distribution start time
	// This ensures distributions happen at noon GMT on the same day every month
	// Day is capped at 28 to ensure consistency across all months (including February)
	upgradeBlockTime := sdkCtx.BlockTime()
	distributionDay := upgradeBlockTime.Day()
	if distributionDay > 28 {
		distributionDay = 28
	}
	scheduleStartTime := uint64(time.Date(
		upgradeBlockTime.Year(),
		upgradeBlockTime.Month()+1,
		distributionDay,
		12, 0, 0, 0,
		time.UTC,
	).Unix())
	totalMintAmount := sdkmath.NewInt(InitialTotalMint)

	// Retrieve the chain's native token denomination from staking params
	bondDenom, err := stakingKeeper.BondDenom(ctx)
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to get staking params: %v", err)
	}

	// Ensure fund allocation percentages are valid and sum to exactly 100%
	if err := validateFundAllocations(allocations); err != nil {
		return errorsmod.Wrap(err, "invalid fund allocations")
	}

	// Step 1: Validate all module account names
	// All clearing accounts receive tokens and are included in the distribution schedule
	for _, allocation := range allocations {
		perms := psetypes.GetAllClearingAccounts()
		if !lo.Contains(perms, allocation.ClearingAccount) {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "invalid module account: %s", allocation.ClearingAccount)
		}
	}

	// Step 2: Create clearing account mappings (only for non-Community clearing accounts)
	// Get chain-specific mappings based on chain ID
	// TODO: Replace placeholder addresses with actual recipient addresses provided by management.
	mappings, err := DefaultClearingAccountMappings(sdkCtx.ChainID())
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to get clearing account mappings: %v", err)
	}

	// Get authority (governance module address)
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	if err := pseKeeper.UpdateClearingAccountMappings(ctx, authority, mappings); err != nil {
		return errorsmod.Wrapf(psetypes.ErrInvalidInput, "failed to create clearing account mappings: %v", err)
	}

	// Step 3: Generate the n-month distribution schedule for all clearing accounts
	// This defines when and how much each clearing account will distribute
	// Community uses score-based distribution, others use direct recipient transfers
	schedule, err := CreateDistributionSchedule(allocations, totalMintAmount, scheduleStartTime)
	if err != nil {
		return errorsmod.Wrapf(psetypes.ErrScheduleCreationFailed, "%v", err)
	}

	// Step 4: Persist the schedule to blockchain state
	if err := pseKeeper.SaveDistributionSchedule(ctx, schedule); err != nil {
		return errorsmod.Wrapf(psetypes.ErrScheduleCreationFailed, "%v", err)
	}

	// Step 5: Mint and fund all clearing accounts
	if err := MintAndFundClearingAccounts(ctx, bankKeeper, allocations, totalMintAmount, bondDenom); err != nil {
		return err
	}

	sdkCtx.Logger().Info("initialization completed",
		"minted", totalMintAmount.String(),
		"denom", bondDenom,
		"fund_allocations", len(allocations),
		"periods", len(schedule),
	)

	return nil
}

// CreateDistributionSchedule generates a monthly distribution schedule over n calendar months.
// All clearing accounts (including Community) are included in the schedule.
// Each distribution month allocates an equal portion (1/n) of each clearing account's total balance.
// Timestamps are calculated by adding one calendar month for each month, maintaining the same day of month.
// The day of month is capped at MaxDistributionDay (28) to ensure all months have this day.
// Returns the schedule without persisting it to state, making this a pure, testable function.
// Community clearing account uses score-based distribution, others use direct recipient transfers.
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

	// Cap the day to MaxDistributionDay to ensure consistency across all months
	distributionDay := startDateTime.Day()
	if distributionDay > MaxDistributionDay {
		distributionDay = MaxDistributionDay
	}

	// Pre-allocate slice with exact capacity for n distribution months
	schedule := make([]psetypes.ScheduledDistribution, 0, TotalAllocationMonths)

	for month := range TotalAllocationMonths {
		// Calculate distribution timestamp by adding calendar months
		// AddDate(0, month, 0) adds 'month' months while maintaining the same day
		distributionDateTime := time.Date(
			startDateTime.Year(),
			startDateTime.Month(),
			distributionDay,
			startDateTime.Hour(),
			startDateTime.Minute(),
			startDateTime.Second(),
			startDateTime.Nanosecond(),
			time.UTC,
		).AddDate(0, month, 0)
		distributionTime := uint64(distributionDateTime.Unix())

		// Build allocations list for this distribution month
		// All clearing accounts (including Community) are included
		monthAllocations := make([]psetypes.ClearingAccountAllocation, 0, len(distributionFundAllocations))

		for _, allocation := range distributionFundAllocations {
			// Calculate total balance for this module account from percentage
			totalBalance := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()

			// Divide total balance equally across all distribution months using integer division
			monthAmount := totalBalance.QuoRaw(TotalAllocationMonths)

			// Fail if balance is too small to distribute over n months
			if monthAmount.IsZero() {
				return nil, errorsmod.Wrapf(
					psetypes.ErrInvalidInput,
					"clearing account %s: balance too small to divide into distribution months",
					allocation.ClearingAccount,
				)
			}

			monthAllocations = append(monthAllocations, psetypes.ClearingAccountAllocation{
				ClearingAccount: allocation.ClearingAccount,
				Amount:          monthAmount,
			})
		}

		if len(monthAllocations) == 0 {
			return nil, errorsmod.Wrapf(psetypes.ErrInvalidInput, "no allocations for distribution month %d", month)
		}

		// Add this distribution month to the schedule
		schedule = append(schedule, psetypes.ScheduledDistribution{
			Timestamp:   distributionTime,
			Allocations: monthAllocations,
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
	for _, allocation := range fundAllocations {
		// Calculate amount for this module account from percentage
		allocationAmount := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()

		// Validate that allocation is not zero
		if allocationAmount.IsZero() {
			return errorsmod.Wrapf(
				psetypes.ErrInvalidInput,
				"module account %s: allocation rounds to zero",
				allocation.ClearingAccount,
			)
		}

		coinsToTransfer := sdk.NewCoins(sdk.NewCoin(denom, allocationAmount))
		if err := bankKeeper.SendCoinsFromModuleToModule(
			ctx,
			psetypes.ModuleName,
			allocation.ClearingAccount,
			coinsToTransfer,
		); err != nil {
			return errorsmod.Wrapf(psetypes.ErrTransferFailed, "to %s: %v", allocation.ClearingAccount, err)
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
		if allocation.ClearingAccount == "" {
			return errorsmod.Wrapf(psetypes.ErrInvalidInput, "fund allocation %d: empty module account name", i)
		}

		if allocation.Percentage.IsNegative() {
			return errorsmod.Wrapf(
				psetypes.ErrInvalidInput,
				"fund allocation %d (%s): negative percentage",
				i, allocation.ClearingAccount,
			)
		}

		if allocation.Percentage.GT(sdkmath.LegacyOneDec()) {
			return errorsmod.Wrapf(
				psetypes.ErrInvalidInput,
				"fund allocation %d (%s): percentage %.2f exceeds 100%%",
				i, allocation.ClearingAccount, allocation.Percentage.MustFloat64()*100,
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
