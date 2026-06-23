package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	testcontracts "github.com/tokenize-x/tx-chain/v8/x/asset/ft/keeper/test-contracts"
	"github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
)

// TestKeeper_Extension_CommissionBurn_RespectsFrozenBalance checks that on the
// extension path the spendable check covers the full debit (amount+commission+burn).
// Otherwise a holder could send their whole unfrozen quota as the bare amount while
// the commission+burn portion is drawn from frozen funds, leaving balance < frozen.
func TestKeeper_Extension_CommissionBurn_RespectsFrozenBalance(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("frozen-overdebit"),
	})

	ftKeeper := testApp.AssetFTKeeper
	bankKeeper := testApp.BankKeeper

	issuer := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	user := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	recipient := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())

	codeID, _, err := testApp.WasmPermissionedKeeper.Create(
		ctx, issuer, testcontracts.AssetExtensionWasm, &wasmtypes.AllowEverybody,
	)
	requireT.NoError(err)

	// Boundary rates: 0.5 burn + 0.5 commission, so the full debit is 2x the send amount.
	settings := types.IssueSettings{
		Issuer:        issuer,
		Symbol:        "FROZ",
		Subunit:       "froz",
		Precision:     1,
		Description:   "extension token with commission and burn",
		InitialAmount: sdkmath.NewInt(10_000),
		Features: []types.Feature{
			types.Feature_freezing,
			types.Feature_extension,
		},
		BurnRate:           sdkmath.LegacyMustNewDecFromStr("0.5"),
		SendCommissionRate: sdkmath.LegacyMustNewDecFromStr("0.5"),
		ExtensionSettings: &types.ExtensionIssueSettings{
			CodeId: codeID,
		},
	}
	denom, err := ftKeeper.Issue(ctx, settings)
	requireT.NoError(err)

	// User holds 1000, of which the admin freezes 500. Spendable (unfrozen) = 500.
	requireT.NoError(bankKeeper.SendCoins(
		ctx, issuer, user, sdk.NewCoins(sdk.NewCoin(denom, sdkmath.NewInt(1000))),
	))
	requireT.NoError(ftKeeper.Freeze(
		ctx, issuer, user, sdk.NewCoin(denom, sdkmath.NewInt(500)),
	))

	assertInvariant := func(stage string) {
		balance := bankKeeper.GetBalance(ctx, user, denom)
		frozen, frozenErr := ftKeeper.GetFrozenBalance(ctx, user, denom)
		requireT.NoError(frozenErr)
		requireT.True(balance.Amount.GTE(frozen.Amount),
			"%s: invariant balance(%s) >= frozen(%s) must hold", stage, balance.Amount, frozen.Amount)
	}

	// Over-debit: sending 500 (the full unfrozen quota as the bare amount) costs
	// 500 + 250 burn + 250 commission = 1000 > spendable 500 → must be rejected.
	err = bankKeeper.SendCoins(
		ctx, user, recipient, sdk.NewCoins(sdk.NewCoin(denom, sdkmath.NewInt(500))),
	)
	requireT.ErrorIs(err, cosmoserrors.ErrInsufficientFunds,
		"over-debit transfer must be rejected: full cost 1000 exceeds spendable 500")

	// State is untouched after the rejected transfer.
	requireT.Equal(sdkmath.NewInt(1000), bankKeeper.GetBalance(ctx, user, denom).Amount,
		"balance must be unchanged after rejected transfer")
	frozenAfterReject, err := ftKeeper.GetFrozenBalance(ctx, user, denom)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(500), frozenAfterReject.Amount,
		"frozen reserve must be untouched after rejected transfer")
	assertInvariant("after rejected over-debit")

	// Within-budget: sending 250 costs 250 + 125 + 125 = 500 == spendable → must succeed,
	// drawing only from the unfrozen portion and preserving the frozen reserve.
	err = bankKeeper.SendCoins(
		ctx, user, recipient, sdk.NewCoins(sdk.NewCoin(denom, sdkmath.NewInt(250))),
	)
	requireT.NoError(err, "within-budget transfer (full cost 500 == spendable) must still succeed")

	requireT.Equal(sdkmath.NewInt(500), bankKeeper.GetBalance(ctx, user, denom).Amount,
		"balance must drop by the full debit (1000 -> 500)")
	frozenAfterSend, err := ftKeeper.GetFrozenBalance(ctx, user, denom)
	requireT.NoError(err)
	requireT.Equal(sdkmath.NewInt(500), frozenAfterSend.Amount,
		"frozen reserve must remain intact (500)")
	requireT.Equal(sdkmath.NewInt(250), bankKeeper.GetBalance(ctx, recipient, denom).Amount,
		"recipient receives the bare amount (250)")
	assertInvariant("after within-budget transfer")
}
