//go:build integrationtests

package upgrade

import (
	"context"
	"fmt"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	tmtypes "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govtypesv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	v6 "github.com/tokenize-x/tx-chain/v6/app/upgrade/v6"
	integrationtests "github.com/tokenize-x/tx-chain/v6/integration-tests"
	"github.com/tokenize-x/tx-chain/v6/pkg/client"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	"github.com/tokenize-x/tx-chain/v6/testutil/event"
	"github.com/tokenize-x/tx-chain/v6/testutil/integration"
	psetypes "github.com/tokenize-x/tx-chain/v6/x/pse/types"
)

// Hardcoded recipient addresses for clearing accounts (devnet)
var (
	foundationAddresses = []string{
		"devcore1kfm48yfnh965ypssfp490aany73jst05fhfapz",
		"devcore1ct5esnd49w7tjtcn9wktjdc4q02a6skpq5tkfz",
		"devcore1yw2urtycl5lfqhtr8lj4qk7fnayqmvhv5fjw8m",
		"devcore1j382ydyytj3tpmjwpx4f3t59vg4f046rluzng5",
		"devcore1splxgwp434jse6gt3ajqfv8vx2fftkj862wvxa",
		"devcore18jmpgu5lgremx9nqua2xa2l4j7l5n3jr24gy4g",
		"devcore10qraeygcztmzqwdk6d2c5rwhkw683az9e9ln0k",
		"devcore1pf3dgr53edq282p452p42amzw9ga2s3398853h",
		"devcore1cj7qkawquelsf2ugcceuehqmkatnyrrr4de5yd",
		"devcore14ewuhx7qwx5gpeh5lzq56a8lkjte46ssgxtl8s",
	}
	allianceAddresses = []string{
		"devcore1ku77sw8vhr3gkegnsnjpzfg5etvwp9l60dy00x",
		"devcore1v9g9eam7ulyhc8mgf458q047f4g05d3wpmx707",
	}
	partnershipAddresses = []string{
		"devcore1m5yzpc3sn5vp96eufa8fxcl0fs44yecl0znvl7",
	}
	investorsAddresses = []string{
		"devcore1cq8p07unjvhnhjmu8v99n8632n29sn5h29tthd",
	}
	teamAddresses = []string{
		"devcore1cx39e5mv8plhs75ls0uweufqr6senymra0rlza",
	}

	foundationMnemonics = []string{
		"summer real powder salmon sudden cash grain bench inspire paddle turtle matrix fine syrup shoulder enforce camp bounce climb appear zoo fade fluid opinion",
		"width wage grace peace mango shock inspire cool abuse slab spread absorb today trial little essence yard artist safe raven siren credit deny boss",
		"message loud strike give round attitude imitate crash donate upon unaware pass winter melody horse mixed spirit swap flush chunk tongue alpha onion release",
		"attend document beach double total gym local style exact scan slush employ maximum neutral leg reunion together key globe coil top tunnel inch zone",
		"during pave bubble crash just myth rubber radar disorder sun zoo nuclear tuition burden tower acoustic scrap shock open crowd fix dawn aim voice",
		"gorilla online record earn duck bean forget submit split fuel number pulse switch jazz surprise hand cliff tissue stove grow aware void toy nice",
		"cliff spin army letter test exhibit decline polar strategy melody win select source detect carbon wrist degree category mandate foot crater settle luxury acquire",
		"divorce truck approve dolphin glad urge fabric start music goose season stairs dizzy mystery start mass tornado script eye federal miss emotion wine claim",
		"wing park remain mind abuse yard distance critic science lady situate ranch sphere soccer brand never problem citizen nuclear rug title misery farm fetch",
		"piece crawl upset smoke outdoor stand pact payment key hawk shrimp wage coffee buzz manual ice chimney praise apart best series today evidence odor",
	}
	allianceMnemonics = []string{
		"paper flat vital boy truth woman cattle vicious license notable stem beyond among arena push drive train tell tell prize sleep answer music timber",
		"layer neglect melt exotic market glare security pool rib force gadget burden act issue force copy pair badge script level husband ripple share sauce",
	}
	partnershipMnemonics = []string{
		"domain cancel lady hammer use shine end jealous orbit drastic pluck shop umbrella blue helmet fly hair delay toilet seven obtain foil soap cloud",
	}
	investorsMnemonics = []string{
		"win nurse differ square omit truck mass cake claw hover cloth leisure blade umbrella ready remove hurt current response fancy buyer link doll fluid",
	}
	teamMnemonics = []string{
		"honey range choose loop ketchup ramp tornado wrestle friend asset frown predict equip paddle network come hint glare they empower assist tool assault scrap",
	}
)

type mintAdditionalSupplyTest struct {
	binanceAddressBefore *sdk.Coin
	soloAddressBefore    *sdk.Coin
	binanceAddress       sdk.AccAddress
	soloAddress          sdk.AccAddress
}

func (mast *mintAdditionalSupplyTest) Before(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	// test mint additional supply in the dev chain only
	if chain.ChainSettings.ChainID != string(constant.ChainIDDev) {
		t.Skip("Skipping mint additional supply test in non-dev chain")
		return
	}

	// Get staking params to determine bond denom
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	stakingParams, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	requireT.NoError(err)
	bondDenom := stakingParams.Params.BondDenom

	// Get addresses based on chain ID
	chainID := chain.ChainSettings.ChainID
	binanceAddressStr := map[string]string{
		string(constant.ChainIDMain): "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		string(constant.ChainIDTest): "testcore1c6y9nwvu9jxx468qx3js620zq34c6hnpg9qgqu8rz3krjrxqmk9s5vzxkj",
		string(constant.ChainIDDev):  "devcore1za98kfjq6pma30l5u2x9pu6w9castcs934xyw6",
	}[chainID]
	soloAddressStr := map[string]string{
		string(constant.ChainIDMain): "core17pmq7hp4upvmmveqexzuhzu64v36re3w3447n7dt46uwp594wtps97qlm5",
		string(constant.ChainIDTest): "testcore19gcp0mkgml3l9pmm269000f6kxacpus0x5ru9pg95tt37dxjx0ksd30rx9",
		string(constant.ChainIDDev):  "devcore1dk2ger49pmqcw062hl09typhjrhxq392qd4rah",
	}[chainID]

	mast.binanceAddress, err = sdk.AccAddressFromBech32(binanceAddressStr)
	requireT.NoError(err, "failed to parse binance address")
	mast.soloAddress, err = sdk.AccAddressFromBech32(soloAddressStr)
	requireT.NoError(err, "failed to parse solo address")

	// Get balances before upgrade
	bankClient := banktypes.NewQueryClient(chain.ClientContext)
	binanceBalanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: binanceAddressStr,
		Denom:   bondDenom,
	})
	requireT.NoError(err)
	mast.binanceAddressBefore = binanceBalanceResp.Balance

	soloBalanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: soloAddressStr,
		Denom:   bondDenom,
	})
	requireT.NoError(err)
	mast.soloAddressBefore = soloBalanceResp.Balance

	t.Logf("Before upgrade - Binance balance: %s, SOLO balance: %s",
		mast.binanceAddressBefore, mast.soloAddressBefore)
}

func (mast *mintAdditionalSupplyTest) After(t *testing.T) {
	ctx, chain := integrationtests.NewTXChainTestingContext(t)
	requireT := require.New(t)

	// test mint additional supply in the dev chain only
	if chain.ChainSettings.ChainID != string(constant.ChainIDDev) {
		t.Skip("Skipping mint additional supply test in non-dev chain")
		return
	}

	// Get staking params to determine bond denom
	stakingClient := stakingtypes.NewQueryClient(chain.ClientContext)
	stakingParams, err := stakingClient.Params(ctx, &stakingtypes.QueryParamsRequest{})
	requireT.NoError(err)
	bondDenom := stakingParams.Params.BondDenom

	bankClient := banktypes.NewQueryClient(chain.ClientContext)

	// Step 1: Verify exact amounts received after upgrade
	mast.verifyExactAmountsReceived(ctx, t, chain, bankClient, bondDenom)

	// Step 2: Delegate tokens from both accounts
	mast.delegateTokens(ctx, t, chain, stakingClient, bondDenom)

	// Step 3: Wait for first PSE distribution and verify Community distributions
	mast.verifyPSEDistributions(ctx, t, chain, bankClient, bondDenom)
}

func (mast *mintAdditionalSupplyTest) verifyExactAmountsReceived(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
	bankClient banktypes.QueryClient,
	bondDenom string,
) {
	requireT := require.New(t)

	// Expected amounts from v6 upgrade
	expectedBinanceAmount := sdkmath.NewInt(v6.TxSupplyForBinance)
	expectedSOLOAmount := sdkmath.NewInt(v6.TxSupplyForSOLOHolders)

	// Get balances after upgrade
	binanceBalanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: mast.binanceAddress.String(),
		Denom:   bondDenom,
	})
	requireT.NoError(err)
	binanceBalanceAfter := binanceBalanceResp.Balance

	soloBalanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: mast.soloAddress.String(),
		Denom:   bondDenom,
	})
	requireT.NoError(err)
	soloBalanceAfter := soloBalanceResp.Balance

	// Calculate actual increases
	binanceIncrease := binanceBalanceAfter.Amount.Sub(mast.binanceAddressBefore.Amount)
	soloIncrease := soloBalanceAfter.Amount.Sub(mast.soloAddressBefore.Amount)

	// Verify exact amounts
	requireT.Equal(expectedBinanceAmount.String(), binanceIncrease.String(),
		"Binance address should receive exactly %s, got increase of %s",
		expectedBinanceAmount, binanceIncrease)
	requireT.Equal(expectedSOLOAmount.String(), soloIncrease.String(),
		"SOLO address should receive exactly %s, got increase of %s",
		expectedSOLOAmount, soloIncrease)

	t.Logf("Verified exact amounts received:")
	t.Logf("Binance: received %s (expected %s)", binanceIncrease, expectedBinanceAmount)
	t.Logf("SOLO: received %s (expected %s)", soloIncrease, expectedSOLOAmount)
}

func (mast *mintAdditionalSupplyTest) delegateTokens(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
	stakingClient stakingtypes.QueryClient,
	bondDenom string,
) {
	requireT := require.New(t)

	// Get validator
	validatorsResponse, err := stakingClient.Validators(ctx, &stakingtypes.QueryValidatorsRequest{
		Status: stakingtypes.Bonded.String(),
	})
	requireT.NoError(err)
	requireT.NotEmpty(validatorsResponse.Validators, "should have at least one validator")
	validator := validatorsResponse.Validators[0]

	kr := chain.ClientContext.Keyring()
	chainID := chain.ChainSettings.ChainID

	// Only delegate on devnet (where we have mnemonics)
	if chainID != string(constant.ChainIDDev) {
		t.Logf("Skipping delegation - mnemonics only available for devnet")
		return
	}

	// Fund all recipient accounts so they can delegate and receive Community distributions
	mast.fundRecipientAccounts(ctx, t, chain, bondDenom)

	// Delegate from Foundation accounts
	mast.delegateFromAccounts(ctx, t, chain, stakingClient, kr, validator, bondDenom,
		foundationAddresses, foundationMnemonics, "Foundation")

	// Delegate from Alliance accounts
	mast.delegateFromAccounts(ctx, t, chain, stakingClient, kr, validator, bondDenom,
		allianceAddresses, allianceMnemonics, "Alliance")

	// Delegate from Partnership accounts
	mast.delegateFromAccounts(ctx, t, chain, stakingClient, kr, validator, bondDenom,
		partnershipAddresses, partnershipMnemonics, "Partnership")

	// Delegate from Investors accounts
	mast.delegateFromAccounts(ctx, t, chain, stakingClient, kr, validator, bondDenom,
		investorsAddresses, investorsMnemonics, "Investors")

	// Delegate from Team accounts
	mast.delegateFromAccounts(ctx, t, chain, stakingClient, kr, validator, bondDenom,
		teamAddresses, teamMnemonics, "Team")

	// Wait for blocks to accumulate score
	requireT.NoError(client.AwaitNextBlocks(ctx, chain.ClientContext, 3))
	t.Logf("Delegations completed, waiting for score accumulation")
}

func (mast *mintAdditionalSupplyTest) delegateFromAccounts(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
	stakingClient stakingtypes.QueryClient,
	keyring keyring.Keyring,
	validator stakingtypes.Validator,
	bondDenom string,
	addresses []string,
	mnemonics []string,
	accountType string,
) {
	requireT := require.New(t)

	// Addresses and mnemonics are hardcoded test data - they must match
	requireT.Equal(len(addresses), len(mnemonics),
		"%s addresses (%d) and mnemonics (%d) count must match", accountType, len(addresses), len(mnemonics))

	bankClient := banktypes.NewQueryClient(chain.ClientContext)
	delegatedCount := 0

	for i, addrStr := range addresses {
		// Addresses are hardcoded test data - they must be valid
		addr, err := sdk.AccAddressFromBech32(addrStr)
		requireT.NoError(err, "Invalid %s address %s", accountType, addrStr)

		// Check balance
		balanceResp, err := bankClient.Balance(ctx, &banktypes.QueryBalanceRequest{
			Address: addrStr,
			Denom:   bondDenom,
		})
		requireT.NoError(err, "Could not check balance for %s account %s", accountType, addrStr)

		balance := balanceResp.Balance.Amount
		if balance.IsZero() {
			t.Logf("Skipping %s account %s - zero balance", accountType, addrStr)
			continue
		}

		// Import key - mnemonics are hardcoded test data - they must be valid
		keyName := fmt.Sprintf("%s-%d", accountType, i)
		_, err = keyring.NewAccount(
			keyName,
			mnemonics[i],
			"",
			sdk.GetConfig().GetFullBIP44Path(),
			hd.Secp256k1,
		)
		requireT.NoError(err, "Could not import %s key %d for address %s", accountType, i, addrStr)

		// Delegate 50% of balance (keep rest for fees)
		delegateAmount := balance.QuoRaw(2)
		if delegateAmount.IsZero() {
			t.Logf("Skipping %s account %s - balance too small to delegate", accountType, addrStr)
			continue
		}

		delegateMsg := &stakingtypes.MsgDelegate{
			DelegatorAddress: addrStr,
			ValidatorAddress: validator.OperatorAddress,
			Amount:           sdk.NewCoin(bondDenom, delegateAmount),
		}

		_, err = client.BroadcastTx(
			ctx,
			chain.ClientContext.WithFromAddress(addr),
			chain.TxFactory().WithGas(chain.GasLimitByMsgs(delegateMsg)),
			delegateMsg,
		)
		requireT.NoError(err, "Failed to delegate from %s account %s", accountType, addrStr)

		delegatedCount++
		t.Logf("%s account %s delegated %s", accountType, addrStr, delegateAmount)
	}

	if delegatedCount > 0 {
		t.Logf("Delegated from %d %s accounts", delegatedCount, accountType)
	}
}

// fundRecipientAccounts funds all recipient accounts so they can delegate and receive Community distributions
func (mast *mintAdditionalSupplyTest) fundRecipientAccounts(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
	bondDenom string,
) {
	// Collect all addresses to fund
	allAddresses := make([]sdk.AccAddress, 0)
	allAddresses = append(allAddresses, mast.collectAddresses(foundationAddresses)...)
	allAddresses = append(allAddresses, mast.collectAddresses(allianceAddresses)...)
	allAddresses = append(allAddresses, mast.collectAddresses(partnershipAddresses)...)
	allAddresses = append(allAddresses, mast.collectAddresses(investorsAddresses)...)
	allAddresses = append(allAddresses, mast.collectAddresses(teamAddresses)...)

	// Fund each account with enough to delegate and pay fees
	// Use a reasonable amount - enough to delegate and cover gas fees
	fundAmount := sdkmath.NewInt(10_000_000) // 10 tokens should be enough for delegation + fees

	accountsToFund := make([]integration.FundedAccount, 0, len(allAddresses))
	for _, addr := range allAddresses {
		accountsToFund = append(accountsToFund, integration.FundedAccount{
			Address: addr,
			Amount:  chain.NewCoin(fundAmount),
		})
	}

	if len(accountsToFund) > 0 {
		chain.Faucet.FundAccounts(ctx, t, accountsToFund...)
		t.Logf("Funded %d recipient accounts with %s each", len(accountsToFund), fundAmount)
	}
}

// collectAddresses converts address strings to AccAddresses, skipping invalid ones
func (mast *mintAdditionalSupplyTest) collectAddresses(addresses []string) []sdk.AccAddress {
	result := make([]sdk.AccAddress, 0, len(addresses))
	for _, addrStr := range addresses {
		addr, err := sdk.AccAddressFromBech32(addrStr)
		if err != nil {
			continue // Skip invalid addresses
		}
		result = append(result, addr)
	}
	return result
}

func (mast *mintAdditionalSupplyTest) verifyPSEDistributions(
	ctx context.Context,
	t *testing.T,
	chain integration.TXChain,
	bankClient banktypes.QueryClient,
	bondDenom string,
) {
	requireT := require.New(t)

	// Get PSE schedule created by upgrade
	pseClient := psetypes.NewQueryClient(chain.ClientContext)
	scheduleResp, err := pseClient.ScheduledDistributions(ctx, &psetypes.QueryScheduledDistributionsRequest{})
	requireT.NoError(err)
	requireT.NotEmpty(scheduleResp.ScheduledDistributions, "should have scheduled distributions")

	// Verify the upgrade created the schedule correctly
	firstDistribution := scheduleResp.ScheduledDistributions[0]
	firstDistTime := time.Unix(int64(firstDistribution.Timestamp), 0)
	t.Logf("Upgrade created schedule - first distribution scheduled for: %s", firstDistTime.Format(time.RFC3339))

	// Get Community allocation amount from the first distribution
	communityAllocation := sdkmath.ZeroInt()
	for _, alloc := range firstDistribution.Allocations {
		if alloc.ClearingAccount == psetypes.ClearingAccountCommunity {
			communityAllocation = alloc.Amount
			break
		}
	}
	requireT.True(communityAllocation.IsPositive(), "should have Community allocation")
	t.Logf("Community allocation for first distribution: %s", communityAllocation)

	// Create a new schedule with near-future timestamps for testing (like module test does)
	govParams, err := chain.Governance.QueryGovParams(ctx)
	requireT.NoError(err)
	distributionStartTime := time.Now().Add(10 * time.Second).Add(*govParams.ExpeditedVotingPeriod)

	// Get all allocations from the first distribution to reuse
	testAllocations := firstDistribution.Allocations

	// Create a test schedule with 3 distributions: 30s, 60s, 90s from now
	testSchedule := []psetypes.ScheduledDistribution{
		{Timestamp: uint64(distributionStartTime.Add(30 * time.Second).Unix()), Allocations: testAllocations},
		{Timestamp: uint64(distributionStartTime.Add(60 * time.Second).Unix()), Allocations: testAllocations},
		{Timestamp: uint64(distributionStartTime.Add(90 * time.Second).Unix()), Allocations: testAllocations},
	}

	// Update schedule via governance (like module test does)
	chain.Governance.ExpeditedProposalFromMsgAndVote(
		ctx, t, nil, "-", "-", "-", govtypesv1.OptionYes,
		&psetypes.MsgUpdateDistributionSchedule{
			Authority: authtypes.NewModuleAddress(govtypes.ModuleName).String(),
			Schedule:  testSchedule,
		},
	)

	t.Logf("Updated schedule for testing - first distribution in ~30 seconds")

	// Wait for first distribution
	header, err := chain.LatestBlockHeader(ctx)
	requireT.NoError(err)
	height := header.Height

	distHeight, events, err := mast.awaitScheduledDistributionEvent(ctx, chain, height)
	requireT.NoError(err)

	t.Logf("Distribution processed at height: %d", distHeight)

	// Collect all recipient addresses that should receive distributions
	allRecipientAddresses := make(map[string]string) // address -> account type
	for _, addr := range foundationAddresses {
		allRecipientAddresses[addr] = "Foundation"
	}
	for _, addr := range allianceAddresses {
		allRecipientAddresses[addr] = "Alliance"
	}
	for _, addr := range partnershipAddresses {
		allRecipientAddresses[addr] = "Partnership"
	}
	for _, addr := range investorsAddresses {
		allRecipientAddresses[addr] = "Investors"
	}
	for _, addr := range teamAddresses {
		allRecipientAddresses[addr] = "Team"
	}

	// First, assert that all events have the same TotalPseScore (consistency check)
	expectedTotalPseScore := sdkmath.ZeroInt()
	totalDistributedAll := sdkmath.ZeroInt()
	for _, ev := range events {
		if expectedTotalPseScore.IsZero() {
			expectedTotalPseScore = ev.TotalPseScore
		} else {
			requireT.True(expectedTotalPseScore.Equal(ev.TotalPseScore),
				"All events should have the same TotalPseScore. Expected: %s, Got: %s for delegator %s",
				expectedTotalPseScore, ev.TotalPseScore, ev.DelegatorAddress)
		}
		totalDistributedAll = totalDistributedAll.Add(ev.Amount)
	}

	requireT.True(expectedTotalPseScore.IsPositive(), "TotalPseScore should be positive")
	t.Logf("Total delegators who received distributions: %d", len(events))
	t.Logf("Total distributed to all delegators: %s", totalDistributedAll)

	// Check which recipient accounts received Community distributions
	receivedAccounts := make(map[string]*psetypes.EventCommunityDistributed)
	for _, ev := range events {
		if accountType, exists := allRecipientAddresses[ev.DelegatorAddress]; exists {
			receivedAccounts[ev.DelegatorAddress] = ev
			t.Logf("%s account %s received Community distribution: %s (score: %s, totalScore: %s)",
				accountType, ev.DelegatorAddress, ev.Amount, ev.Score, ev.TotalPseScore)
		}
	}

	requireT.Greaterf(len(receivedAccounts), 0, "No recipient accounts received Community distributions.")

	t.Logf("%d recipient accounts received Community PSE distributions", len(receivedAccounts))

	// Verify proportionality for recipient accounts
	for addr, ev := range receivedAccounts {
		expectedAmount := communityAllocation.Mul(ev.Score).Quo(expectedTotalPseScore)
		diff := ev.Amount.Sub(expectedAmount).Abs()

		// Allow small rounding differences (within 1 token)
		requireT.True(diff.LTE(sdkmath.NewInt(1)),
			"%s account %s: distribution should be proportional. Expected: %s, Got: %s, Diff: %s, Score: %s, TotalScore: %s",
			allRecipientAddresses[addr], addr, expectedAmount, ev.Amount, diff, ev.Score, expectedTotalPseScore)
	}

	// Verify the sum of all distributed amounts equals the community allocation
	// (accounting for rounding/leftover sent to community pool)
	// The leftover should be at most the number of ALL delegators (due to rounding down)
	leftover := communityAllocation.Sub(totalDistributedAll)
	requireT.False(leftover.IsNegative(),
		"Total distributed (%s) should not exceed community allocation (%s)",
		totalDistributedAll, communityAllocation)
	requireT.True(leftover.LTE(sdkmath.NewInt(int64(len(events)))),
		"Leftover (%s) should be at most %d tokens (one per delegator due to rounding)",
		leftover, len(events))

	t.Logf("Verified distributions are proportional to scores")
	t.Logf("Total distributed to all delegators: %s, Community allocation: %s, Leftover: %s",
		totalDistributedAll, communityAllocation, leftover)
}

// Helper type for community distributed events
type communityDistributedEvent []*psetypes.EventCommunityDistributed

func (e communityDistributedEvent) find(addr string) *psetypes.EventCommunityDistributed {
	for _, evt := range e {
		if evt.DelegatorAddress == addr {
			return evt
		}
	}
	return nil
}

// awaitScheduledDistributionEvent waits for a PSE distribution event to occur
func (mast *mintAdditionalSupplyTest) awaitScheduledDistributionEvent(
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
	// Remove the mode attribute from the events because it is not part of the typed event
	// and is added by cosmos-sdk, otherwise parsing the events will fail.
	events := removeAttributeFromEvent(results.FinalizeBlockEvents, "mode")
	communityDistributedEvents, err := event.FindTypedEvents[*psetypes.EventCommunityDistributed](events)
	if err != nil {
		return 0, nil, err
	}
	return observedHeight, communityDistributedEvent(communityDistributedEvents), nil
}

func removeAttributeFromEvent(events []tmtypes.Event, key string) []tmtypes.Event {
	newEvents := make([]tmtypes.Event, 0, len(events))
	for _, evt := range events {
		for i, attribute := range evt.Attributes {
			if attribute.Key == key {
				evt.Attributes = append(evt.Attributes[:i], evt.Attributes[i+1:]...)
			}
		}
		newEvents = append(newEvents, evt)
	}
	return newEvents
}
