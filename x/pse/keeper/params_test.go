package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v7/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v7/x/pse/types"
)

func TestGetSetParams(t *testing.T) {
	requireT := require.New(t)
	assertT := assert.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	// Test getting default params
	params, err := pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Empty(params.ExcludedAddresses)

	// Test setting params with excluded addresses
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	newParams := types.Params{
		ExcludedAddresses: []string{addr1, addr2},
	}

	err = pseKeeper.SetParams(ctx, newParams)
	requireT.NoError(err)

	// Verify params were set correctly
	params, err = pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Len(params.ExcludedAddresses, 2)
	assertT.Contains(params.ExcludedAddresses, addr1)
	assertT.Contains(params.ExcludedAddresses, addr2)

	// Test setting params with empty excluded addresses
	emptyParams := types.Params{
		ExcludedAddresses: []string{},
	}

	err = pseKeeper.SetParams(ctx, emptyParams)
	requireT.NoError(err)

	params, err = pseKeeper.GetParams(ctx)
	requireT.NoError(err)
	assertT.Empty(params.ExcludedAddresses)
}

func TestUpdateExcludedAddresses(t *testing.T) {
	requireT := require.New(t)
	assertT := assert.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	// Use correct authority unless test specifies otherwise
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr2 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr3 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr4 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr5 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	invalidAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	testCases := []struct {
		name             string
		authority        string
		initialAddresses []string
		addressesToAdd   []string
		addressesToRem   []string
		expectedFinal    []string
		expectErr        bool
		errMsg           string
	}{
		{
			name:             "add_to_empty_list",
			initialAddresses: []string{},
			addressesToAdd:   []string{addr1},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1},
			expectErr:        false,
		},
		{
			name:             "add_multiple_addresses",
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2, addr3},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2, addr3},
			expectErr:        false,
		},
		{
			name:             "remove_existing_address",
			initialAddresses: []string{addr1, addr2, addr3},
			addressesToAdd:   []string{},
			addressesToRem:   []string{addr2},
			expectedFinal:    []string{addr1, addr3},
			expectErr:        false,
		},
		{
			name:             "add_and_remove_different_addresses",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr3, addr4},
			addressesToRem:   []string{addr1},
			expectedFinal:    []string{addr2, addr3, addr4},
			expectErr:        false,
		},
		{
			name:             "remove_nonexistent_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{},
			addressesToRem:   []string{addr5},
			expectedFinal:    []string{addr1, addr2},
			expectErr:        false,
		},
		{
			name:             "add_duplicate_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr1},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2}, // Should not add duplicate
			expectErr:        false,
		},
		{
			name:             "remove_then_add_different_address",
			initialAddresses: []string{addr1, addr2},
			addressesToAdd:   []string{addr3},
			addressesToRem:   []string{addr1},
			expectedFinal:    []string{addr2, addr3},
			expectErr:        false,
		},
		{
			name:             "add_multiple_including_duplicate",
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2, addr1, addr3},
			addressesToRem:   []string{},
			expectedFinal:    []string{addr1, addr2, addr3}, // addr1 should not be duplicated
			expectErr:        false,
		},
		{
			name:             "invalid_authority",
			authority:        invalidAuthority,
			initialAddresses: []string{addr1},
			addressesToAdd:   []string{addr2},
			addressesToRem:   []string{},
			expectErr:        true,
			errMsg:           "expected",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.authority != "" {
				authority = tc.authority
			}

			// Set initial params
			initialParams := types.Params{
				ExcludedAddresses: tc.initialAddresses,
			}
			err := pseKeeper.SetParams(ctx, initialParams)
			requireT.NoError(err)

			// Update excluded addresses
			err = pseKeeper.UpdateExcludedAddresses(ctx, authority, tc.addressesToAdd, tc.addressesToRem)

			if tc.expectErr {
				requireT.Error(err)
				if tc.errMsg != "" {
					requireT.Contains(err.Error(), tc.errMsg)
				}
			} else {
				requireT.NoError(err)

				// Verify final state
				params, err := pseKeeper.GetParams(ctx)
				requireT.NoError(err)
				assertT.Len(params.ExcludedAddresses, len(tc.expectedFinal))

				for _, expectedAddr := range tc.expectedFinal {
					assertT.Contains(params.ExcludedAddresses, expectedAddr)
				}
			}
		})
	}
}

func TestUpdateClearingMappings_Authority(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper

	correctAuthority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	wrongAuthority := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()
	addr1 := sdk.AccAddress(ed25519.GenPrivKey().PubKey().Address()).String()

	// Must include all eligible clearing accounts
	var mappings []types.ClearingAccountMapping
	for _, clearingAccount := range types.GetNonCommunityClearingAccounts() {
		mappings = append(mappings, types.ClearingAccountMapping{
			ClearingAccount:    clearingAccount,
			RecipientAddresses: []string{addr1},
		})
	}

	// Test with wrong authority
	err := pseKeeper.UpdateClearingAccountMappings(ctx, wrongAuthority, mappings)
	requireT.Error(err, "should reject wrong authority")
	requireT.Contains(err.Error(), "invalid authority")

	// Test with correct authority
	err = pseKeeper.UpdateClearingAccountMappings(ctx, correctAuthority, mappings)
	requireT.NoError(err, "should accept correct authority")
}

// TestExcludedAddress_ScoreLifecycle verifies the full excluded address score lifecycle:
// score accumulates in ExcludedAddressScore while excluded, is invisible to distributions,
// and is restored to AccountScoreSnapshot on re-inclusion.
func TestExcludedAddress_ScoreLifecycle(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContext(false)
	pseKeeper := testApp.PSEKeeper
	authority := authtypes.NewModuleAddress(govtypes.ModuleName).String()
	// getActiveDistributionID returns 1 when no ongoing distribution and no LastProcessedDistributionID.
	const distID = uint64(1)

	// Create validator.
	validatorOp, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, validatorOp, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1000))),
	))
	validator, err := testApp.AddValidator(ctx, validatorOp, sdk.NewInt64Coin(sdk.DefaultBondDenom, 10), nil)
	requireT.NoError(err)

	// Create delegator with plenty of tokens.
	delegator, _ := testApp.GenAccount(ctx)
	requireT.NoError(testApp.FundAccount(
		ctx, delegator, sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(10000))),
	))

	stakingMsgSrv := stakingkeeper.NewMsgServerImpl(testApp.StakingKeeper)
	delegate := func(amount int64) {
		_, err := stakingMsgSrv.Delegate(ctx, &stakingtypes.MsgDelegate{
			DelegatorAddress: delegator.String(),
			ValidatorAddress: validator.GetOperator(),
			Amount:           sdk.NewInt64Coin(sdk.DefaultBondDenom, amount),
		})
		requireT.NoError(err)
	}

	// Step 1: delegate 100 tokens, wait 10s, delegate again -> score = 100*10 = 1000 in AccountScoreSnapshot.
	delegate(100)
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(10 * time.Second))
	requireT.NoError(err)
	delegate(1) // triggers AfterDelegationModified hook

	snapshot, err := pseKeeper.GetDelegatorScore(ctx, distID, delegator)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1000), snapshot)

	totalScore, err := pseKeeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1000), totalScore)

	// Step 2: exclude delegator -> snapshot moves to ExcludedAddressScore, TotalScore decremented.
	requireT.NoError(pseKeeper.UpdateExcludedAddresses(ctx, authority, []string{delegator.String()}, nil))

	_, err = pseKeeper.GetDelegatorScore(ctx, distID, delegator)
	requireT.ErrorIs(err, collections.ErrNotFound, "snapshot should be cleared on exclusion")

	excludedScore, err := pseKeeper.ExcludedAddressScore.Get(ctx, delegator)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(1000), excludedScore)

	totalScore, err = pseKeeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(0), totalScore)

	// Step 3: while excluded, first delegation creates a fresh entry (removeExcludedAccountData cleared
	// all entries on exclusion), then a second delegation after waiting accumulates score to ExcludedAddressScore.
	delegate(1) // Scenario 3: no entry exists -> creates entry, no score yet
	ctx, _, err = testApp.BeginNextBlockAtTime(ctx.BlockTime().Add(10 * time.Second))
	requireT.NoError(err)
	delegate(1) // Scenario 2: entry exists -> hook accumulates score to ExcludedAddressScore

	_, err = pseKeeper.GetDelegatorScore(ctx, distID, delegator)
	requireT.ErrorIs(err, collections.ErrNotFound, "excluded addr must not appear in AccountScoreSnapshot")

	excludedScore, err = pseKeeper.ExcludedAddressScore.Get(ctx, delegator)
	requireT.NoError(err)
	requireT.True(excludedScore.GT(sdkmath.NewInt(1000)), "excluded score should grow while address is excluded")

	totalScore, err = pseKeeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(0), totalScore, "TotalScore must not include excluded address score")

	savedExcludedScore := excludedScore

	// Step 4: re-include delegator -> ExcludedAddressScore moved back to AccountScoreSnapshot+TotalScore.
	requireT.NoError(pseKeeper.UpdateExcludedAddresses(ctx, authority, nil, []string{delegator.String()}))

	_, err = pseKeeper.ExcludedAddressScore.Get(ctx, delegator)
	requireT.ErrorIs(err, collections.ErrNotFound, "excluded score should be cleared on re-inclusion")

	snapshot, err = pseKeeper.GetDelegatorScore(ctx, distID, delegator)
	requireT.NoError(err)
	requireT.Equal(savedExcludedScore, snapshot, "excluded score should be restored to account snapshot")

	totalScore, err = pseKeeper.TotalScore.Get(ctx, distID)
	requireT.NoError(err)
	requireT.Equal(savedExcludedScore, totalScore, "TotalScore should reflect the restored score")
}
