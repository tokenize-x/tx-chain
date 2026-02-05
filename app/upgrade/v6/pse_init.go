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
func DefaultClearingAccountMappings(chainID string) ([]psetypes.ClearingAccountMapping, error) {
	// Create mappings for all non-Community clearing accounts
	// Each starts with a single default recipient (can be modified via governance)
	var mappings []psetypes.ClearingAccountMapping
	if chainID == string(constant.ChainIDMain) {
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
	} else if chainID == string(constant.ChainIDTest) {
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
	} else {
		recipientAddress := "devcore17we2jgjyxexcz8rg29dn622axt7s9l263fl0zt"
		for _, clearingAccount := range psetypes.GetNonCommunityClearingAccounts() {
			mappings = append(mappings, psetypes.ClearingAccountMapping{
				ClearingAccount:    clearingAccount,
				RecipientAddresses: []string{recipientAddress},
			})
		}
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
	// Start distributions one month after the upgrade at 12:00:00 GMT
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
