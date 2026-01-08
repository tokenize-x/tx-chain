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

	// DistributionIntervalSeconds is the interval between distributions
	// Set to 60 seconds (1 minute) for demo purposes
	// In production, this should be based on calendar months
	DistributionIntervalSeconds = 60 // 1 minute for demo
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
// Different clearing accounts have different numbers of recipients:
// - Foundation: 2 recipients
// - Team: 3 recipients
// - Alliance, Partnership, Investors: 1 recipient each
// TODO: Replace placeholder addresses with actual recipient addresses provided by management.
func DefaultClearingAccountMappings(chainID string) ([]psetypes.ClearingAccountMapping, error) {
	// Define base recipient addresses for each chain
	var baseAddresses map[string][]string
	switch chainID {
	case string(constant.ChainIDMain):
		baseAddresses = map[string][]string{
			"foundation":  {"core1foundation1address000000000000000000000000", "core1foundation2address000000000000000000000000"},
			"team":        {"core1team1address00000000000000000000000000000", "core1team2address00000000000000000000000000000", "core1team3address00000000000000000000000000000"},
			"alliance":    {"core1alliance1address000000000000000000000000"},
			"partnership": {"core1partnership1address0000000000000000000"},
			"investors":   {"core1investors1address00000000000000000000000"},
		}
	case string(constant.ChainIDTest):
		baseAddresses = map[string][]string{
			"foundation":  {"testcore1foundation1address00000000000000000", "testcore1foundation2address00000000000000000"},
			"team":        {"testcore1team1address0000000000000000000000", "testcore1team2address0000000000000000000000", "testcore1team3address0000000000000000000000"},
			"alliance":    {"testcore1alliance1address0000000000000000000"},
			"partnership": {"testcore1partnership1address000000000000000"},
			"investors":   {"testcore1investors1address000000000000000000"},
		}
	case string(constant.ChainIDDev):
		baseAddresses = map[string][]string{
			"foundation":  {"devcore1rm6twjcqvgf2guc54gvas2860q3zcd9pwmumz4", "devcore1r2fuv00ll8mku5y927nl2k3c4r2a3vzx9ekzrs"},
			"team":        {"devcore1myu0eua9zzng92gw0jyjy2p9pftlmug24tm6pm", "devcore17rprm85xvpxup87wg9kl9yj6v0amp5pdk7w689", "devcore1x2sjwtjhva6huvanyl895g4gks6s7wxty60wlf"},
			"alliance":    {"devcore1py6vj4acvypw24e7f2gs4pck7rh9s8vh9s9uys"},
			"partnership": {"devcore1jp8j2avp4hlm9nhxxmwzqwa4hzf5ryjq4vss77"},
			"investors":   {"devcore19veu5j9t005yquwg9gk3r4hgevemjphl2aclnv"},
		}
	default:
		return nil, errorsmod.Wrapf(psetypes.ErrInvalidInput, "unknown chain id: %s", chainID)
	}

	// Create mappings for all non-Community clearing accounts
	var mappings []psetypes.ClearingAccountMapping
	for _, clearingAccount := range psetypes.GetNonCommunityClearingAccounts() {
		var recipients []string

		// Map clearing account module name to base address key
		switch clearingAccount {
		case psetypes.ClearingAccountFoundation:
			recipients = baseAddresses["foundation"]
		case psetypes.ClearingAccountTeam:
			recipients = baseAddresses["team"]
		case psetypes.ClearingAccountAlliance:
			recipients = baseAddresses["alliance"]
		case psetypes.ClearingAccountPartnership:
			recipients = baseAddresses["partnership"]
		case psetypes.ClearingAccountInvestors:
			recipients = baseAddresses["investors"]
		default:
			return nil, errorsmod.Wrapf(psetypes.ErrInvalidInput, "unknown clearing account: %s", clearingAccount)
		}

		mappings = append(mappings, psetypes.ClearingAccountMapping{
			ClearingAccount:    clearingAccount,
			RecipientAddresses: recipients,
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
	// Start distributions immediately from current block time
	scheduleStartTime := uint64(sdkCtx.BlockTime().Unix()) + 60
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
	// DISABLED FOR DEMO: znet already handles minting and funding
	// if err := MintAndFundClearingAccounts(ctx, bankKeeper, allocations, totalMintAmount, bondDenom); err != nil {
	// 	return err
	// }

	sdkCtx.Logger().Info("PSE initialization completed (minting disabled - handled by znet)",
		"denom", bondDenom,
		"fund_allocations", len(allocations),
		"periods", len(schedule),
		"interval_seconds", DistributionIntervalSeconds,
	)

	return nil
}

// CreateDistributionSchedule generates a distribution schedule over TotalAllocationMonths periods.
// Each period is separated by DistributionIntervalSeconds.
// All clearing accounts (including Community) are included in the schedule.
// Each distribution allocates an equal portion (1/n) of each clearing account's total balance.
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

	// Pre-allocate slice with exact capacity
	schedule := make([]psetypes.ScheduledDistribution, 0, TotalAllocationMonths)

	for i := int64(0); i < TotalAllocationMonths; i++ {
		// Add i intervals to start time
		distributionDateTime := startDateTime.Add(time.Duration(i*DistributionIntervalSeconds) * time.Second)
		distributionTime := uint64(distributionDateTime.Unix())

		// Build allocations list for this distribution period
		// All clearing accounts (including Community) are included
		periodAllocations := make([]psetypes.ClearingAccountAllocation, 0, len(distributionFundAllocations))

		for _, allocation := range distributionFundAllocations {
			// Calculate total balance for this module account from percentage
			totalBalance := allocation.Percentage.MulInt(totalMintAmount).TruncateInt()

			// Divide total balance equally across all distribution periods using integer division
			periodAmount := totalBalance.QuoRaw(TotalAllocationMonths)

			// Fail if balance is too small to distribute over n periods
			if periodAmount.IsZero() {
				return nil, errorsmod.Wrapf(
					psetypes.ErrInvalidInput,
					"clearing account %s: balance too small to divide into distribution periods",
					allocation.ClearingAccount,
				)
			}

			periodAllocations = append(periodAllocations, psetypes.ClearingAccountAllocation{
				ClearingAccount: allocation.ClearingAccount,
				Amount:          periodAmount,
			})
		}

		if len(periodAllocations) == 0 {
			return nil, errorsmod.Wrapf(psetypes.ErrInvalidInput, "no allocations for distribution period %d", i)
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
