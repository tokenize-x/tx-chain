package keeper_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/docker/distribution/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/event"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	testcontracts "github.com/tokenize-x/tx-chain/v8/x/asset/ft/keeper/test-contracts"
	assetfttypes "github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
	"github.com/tokenize-x/tx-chain/v8/x/dex/types"
)

const (
	ExtensionOrderDataWASMAttribute = "order_data"
)

var (
	AmountDEXExpectToSpendTrigger   = sdkmath.NewInt(testcontracts.AmountDEXExpectToSpendTrigger)
	AmountDEXExpectToReceiveTrigger = sdkmath.NewInt(testcontracts.AmountDEXExpectToReceiveTrigger)
)

func TestKeeper_PlaceOrderWithExtension(t *testing.T) {
	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("some-hash"),
	})
	testSet := genTestSet(t, sdkCtx, testApp)

	issuer, _ := testApp.GenAccount(sdkCtx)

	// extension
	codeID, _, err := testApp.WasmPermissionedKeeper.Create(
		sdkCtx, issuer, testcontracts.AssetExtensionWasm, &wasmtypes.AllowEverybody,
	)
	require.NoError(t, err)
	settingsWithExtension := assetfttypes.IssueSettings{
		Issuer:        issuer,
		Symbol:        "DEFEXT",
		Subunit:       "defext",
		Precision:     6,
		InitialAmount: sdkmath.NewIntWithDecimal(1, 10),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_extension,
		},
		ExtensionSettings: &assetfttypes.ExtensionIssueSettings{
			CodeId: codeID,
		},
	}
	denomWithExtension, err := testApp.AssetFTKeeper.Issue(sdkCtx, settingsWithExtension)
	require.NoError(t, err)

	tests := []struct {
		name       string
		order      types.Order
		wantDEXErr bool
	}{
		{
			name: "sell_positive",
			order: types.Order{
				Creator: func() string {
					creator, _ := testApp.GenAccount(sdkCtx)
					return creator.String()
				}(),
				Type:        types.ORDER_TYPE_LIMIT,
				ID:          uuid.Generate().String(),
				BaseDenom:   denomWithExtension,
				QuoteDenom:  testSet.denom2,
				Price:       lo.ToPtr(types.MustNewPriceFromString("1")),
				Quantity:    defaultQuantityStep,
				Side:        types.SIDE_SELL,
				TimeInForce: types.TIME_IN_FORCE_GTC,
			},
			wantDEXErr: false,
		},
		{
			name: "sell_dex_error",
			order: types.Order{
				Creator: func() string {
					creator, _ := testApp.GenAccount(sdkCtx)
					return creator.String()
				}(),
				Type:        types.ORDER_TYPE_LIMIT,
				ID:          uuid.Generate().String(),
				BaseDenom:   denomWithExtension,
				QuoteDenom:  testSet.denom2,
				Price:       lo.ToPtr(types.MustNewPriceFromString("1")),
				Quantity:    AmountDEXExpectToSpendTrigger,
				Side:        types.SIDE_SELL,
				TimeInForce: types.TIME_IN_FORCE_GTC,
			},
			wantDEXErr: true,
		},
		{
			name: "buy_positive",
			order: types.Order{
				Creator: func() string {
					creator, _ := testApp.GenAccount(sdkCtx)
					return creator.String()
				}(),
				Type:        types.ORDER_TYPE_LIMIT,
				ID:          uuid.Generate().String(),
				BaseDenom:   testSet.denom2,
				QuoteDenom:  denomWithExtension,
				Price:       lo.ToPtr(types.MustNewPriceFromString("1")),
				Quantity:    defaultQuantityStep,
				Side:        types.SIDE_BUY,
				TimeInForce: types.TIME_IN_FORCE_GTC,
			},
			wantDEXErr: false,
		},
		{
			name: "buy_dex_error",
			order: types.Order{
				Creator: func() string {
					creator, _ := testApp.GenAccount(sdkCtx)
					return creator.String()
				}(),
				Type:        types.ORDER_TYPE_LIMIT,
				ID:          uuid.Generate().String(),
				BaseDenom:   testSet.denom2,
				QuoteDenom:  denomWithExtension,
				Price:       lo.ToPtr(types.MustNewPriceFromString("1")),
				Quantity:    AmountDEXExpectToReceiveTrigger,
				Side:        types.SIDE_BUY,
				TimeInForce: types.TIME_IN_FORCE_GTC,
			},
			wantDEXErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			creator := sdk.MustAccAddressFromBech32(tt.order.Creator)
			lockedBalance, err := tt.order.ComputeLimitOrderLockedBalance()
			require.NoError(t, err)
			testApp.MintAndSendCoin(t, sdkCtx, creator, sdk.NewCoins(lockedBalance))
			fundOrderReserve(t, testApp, sdkCtx, creator)
			if !tt.wantDEXErr {
				sdkCtx = sdkCtx.WithEventManager(sdk.NewEventManager())
				require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, tt.order))

				// decode wasm events
				orderStr, err := event.FindStringEventAttribute(
					sdkCtx.EventManager().Events().ToABCIEvents(),
					wasmtypes.WasmModuleEventType,
					ExtensionOrderDataWASMAttribute,
				)
				require.NoError(t, err)

				extensionOrderData := assetfttypes.DEXOrder{}
				require.NoError(t, json.Unmarshal([]byte(orderStr), &extensionOrderData))

				order, err := testApp.DEXKeeper.GetOrderByAddressAndID(sdkCtx, creator, tt.order.ID)
				require.NoError(t, err)

				require.Equal(t, assetfttypes.DEXOrder{
					Creator:    sdk.MustAccAddressFromBech32(order.Creator),
					Type:       order.Type.String(),
					ID:         order.ID,
					Sequence:   order.Sequence,
					BaseDenom:  order.BaseDenom,
					QuoteDenom: order.QuoteDenom,
					Price:      lo.ToPtr(order.Price.String()),
					Quantity:   order.Quantity,
					Side:       order.Side.String(),
				}, extensionOrderData)
			} else {
				require.ErrorContains(
					t,
					testApp.DEXKeeper.PlaceOrder(simapp.CopyContextWithMultiStore(sdkCtx), tt.order),
					"wasm error: DEX order placement is failed",
				)
			}
		})
	}
}

func TestKeeper_PlaceOrderWithDEXBlockFeature(t *testing.T) {
	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("some-hash"),
	})
	testSet := genTestSet(t, sdkCtx, testApp)

	acc, _ := testApp.GenAccount(sdkCtx)
	issuer, _ := testApp.GenAccount(sdkCtx)

	settingsWithExtension := assetfttypes.IssueSettings{
		Issuer:        issuer,
		Symbol:        "DEFEXT",
		Subunit:       "defext",
		Precision:     6,
		InitialAmount: sdkmath.NewIntWithDecimal(1, 10),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_dex_block,
		},
	}
	denomWithExtension, err := testApp.AssetFTKeeper.Issue(sdkCtx, settingsWithExtension)
	require.NoError(t, err)

	order := types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denomWithExtension,
		QuoteDenom: testSet.denom2,
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	lockedBalance, err := order.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.BankKeeper.SendCoins(sdkCtx, issuer, acc, sdk.NewCoins(lockedBalance)))
	fundOrderReserve(t, testApp, sdkCtx, acc)
	errStr := fmt.Sprintf("usage of %s is not supported for DEX, the token has dex_block", denomWithExtension)
	require.ErrorContains(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, order), errStr)

	// use the denomWithExtension as quote
	order = types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  testSet.denom2,
		QuoteDenom: denomWithExtension,
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	lockedBalance, err = order.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	testApp.MintAndSendCoin(t, sdkCtx, acc, sdk.NewCoins(lockedBalance))
	fundOrderReserve(t, testApp, sdkCtx, acc)
	require.ErrorContains(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, order), errStr)
}

func TestKeeper_PlaceOrderWithRestrictDEXFeature(t *testing.T) {
	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("some-hash"),
	})
	testSet := genTestSet(t, sdkCtx, testApp)

	acc, _ := testApp.GenAccount(sdkCtx)
	issuer, _ := testApp.GenAccount(sdkCtx)

	issuanceSettings := assetfttypes.IssueSettings{
		Issuer:        issuer,
		Symbol:        "DEFEXT",
		Subunit:       "defext",
		Precision:     6,
		InitialAmount: sdkmath.NewIntWithDecimal(1, 10),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_dex_whitelisted_denoms,
		},
		DEXSettings: &assetfttypes.DEXSettings{
			WhitelistedDenoms: []string{
				testSet.denom3,
			},
		},
	}
	denom, err := testApp.AssetFTKeeper.Issue(sdkCtx, issuanceSettings)
	require.NoError(t, err)

	orderReceiveDenom2 := types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denom,
		QuoteDenom: testSet.denom2, // the denom2 is not allowed
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	lockedBalance, err := orderReceiveDenom2.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.BankKeeper.SendCoins(sdkCtx, issuer, acc, sdk.NewCoins(lockedBalance)))
	fundOrderReserve(t, testApp, sdkCtx, acc)
	require.ErrorContains(
		t, testApp.DEXKeeper.PlaceOrder(sdkCtx, orderReceiveDenom2), "locking coins for DEX is prohibited, denom",
	)

	orderReceiveDenom3 := types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denom,
		QuoteDenom: testSet.denom3, // the denom3 is allowed
		Price:      lo.ToPtr(types.MustNewPriceFromString("7e-4")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	lockedBalance, err = orderReceiveDenom2.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.BankKeeper.SendCoins(sdkCtx, issuer, acc, sdk.NewCoins(lockedBalance)))
	fundOrderReserve(t, testApp, sdkCtx, acc)
	require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, orderReceiveDenom3))

	// now update settings to remove all limit and place orderReceiveDenom2
	require.NoError(t, testApp.AssetFTKeeper.UpdateDEXWhitelistedDenoms(sdkCtx, issuer, denom, nil))
	fundOrderReserve(t, testApp, sdkCtx, acc)
	orderReceiveDenom2.ID = uuid.Generate().String()
	require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, orderReceiveDenom2))
}

func TestKeeper_PlaceOrderWithBurning(t *testing.T) {
	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("some-hash"),
	})
	testSet := genTestSet(t, sdkCtx, testApp)

	acc, _ := testApp.GenAccount(sdkCtx)
	issuer, _ := testApp.GenAccount(sdkCtx)

	settingsWithBurn := assetfttypes.IssueSettings{
		Issuer:        issuer,
		Symbol:        "DEFEXT",
		Subunit:       "defext",
		Precision:     6,
		InitialAmount: sdkmath.NewIntWithDecimal(1, 10),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_burning,
		},
	}
	denomWithBurn, err := testApp.AssetFTKeeper.Issue(sdkCtx, settingsWithBurn)
	require.NoError(t, err)

	order := types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denomWithBurn,
		QuoteDenom: testSet.denom2,
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	lockedBalance, err := order.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.BankKeeper.SendCoins(sdkCtx, issuer, acc, sdk.NewCoins(lockedBalance)))
	fundOrderReserve(t, testApp, sdkCtx, acc)
	require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, order))
	require.ErrorContains(t, testApp.AssetFTKeeper.Burn(sdkCtx, acc, lockedBalance), "coins are not spendable")
}

func TestKeeper_PlaceOrderWithStaking(t *testing.T) {
	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("some-hash"),
	})
	testSet := genTestSet(t, sdkCtx, testApp)

	acc, _ := testApp.GenAccount(sdkCtx)
	validatorOwner, _ := testApp.GenAccount(sdkCtx)
	validatorOwner2, _ := testApp.GenAccount(sdkCtx)

	denomToStake := sdk.DefaultBondDenom

	require.NoError(t, testApp.FundAccount(sdkCtx, validatorOwner, sdk.NewCoins(sdk.NewInt64Coin(denomToStake, 10))))
	_, err := testApp.AddValidator(sdkCtx, validatorOwner, sdk.NewInt64Coin(denomToStake, 10), nil)
	require.NoError(t, err)
	val, err := testApp.StakingKeeper.GetValidators(sdkCtx, 1)
	require.NoError(t, err)

	order := types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denomToStake,
		QuoteDenom: testSet.denom2,
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	orderLockedBalance, err := order.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.FundAccount(sdkCtx, acc, sdk.NewCoins(orderLockedBalance)))

	_, err = testApp.StakingKeeper.Delegate(sdkCtx, acc, orderLockedBalance.Amount, stakingtypes.Unbonded, val[0], true)
	require.NoError(t, err)

	balance := testApp.BankKeeper.GetBalance(sdkCtx, acc, denomToStake)
	require.Equal(t, sdk.NewInt64Coin(denomToStake, 0).String(), balance.String())

	lockedBalance := testApp.AssetFTKeeper.GetDEXLockedBalance(sdkCtx, acc, denomToStake)
	require.Equal(t, sdk.NewInt64Coin(denomToStake, 0).String(), lockedBalance.String())

	fundOrderReserve(t, testApp, sdkCtx, acc)

	require.Error(t, testApp.DEXKeeper.PlaceOrder(
		simapp.CopyContextWithMultiStore(sdkCtx), order), cosmoserrors.ErrInsufficientFunds,
	)
	require.NoError(t, testApp.FundAccount(sdkCtx, acc, sdk.NewCoins(orderLockedBalance)))

	require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, order))

	balance = testApp.BankKeeper.GetBalance(sdkCtx, acc, denomToStake)
	params, err := testApp.DEXKeeper.GetParams(sdkCtx)
	require.NoError(t, err)
	orderReserve := params.OrderReserve
	require.Equal(t, orderLockedBalance.Add(orderReserve).String(), balance.String())

	lockedBalance = testApp.AssetFTKeeper.GetDEXLockedBalance(sdkCtx, acc, denomToStake)
	require.Equal(t, orderLockedBalance.Add(orderReserve).String(), lockedBalance.String())

	_, err = testApp.StakingKeeper.Delegate(
		simapp.CopyContextWithMultiStore(sdkCtx),
		acc,
		orderLockedBalance.Amount,
		stakingtypes.Unbonded,
		val[0],
		true,
	)
	require.Error(t, err, cosmoserrors.ErrInsufficientFunds)

	order = types.Order{
		Creator:    validatorOwner2.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denomToStake,
		QuoteDenom: testSet.denom2,
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	orderLockedBalance, err = order.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.FundAccount(sdkCtx, validatorOwner2, sdk.NewCoins(orderLockedBalance)))
	fundOrderReserve(t, testApp, sdkCtx, validatorOwner2)
	require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, order))
	_, err = testApp.AddValidator(sdkCtx, validatorOwner2, orderLockedBalance, nil)
	require.ErrorContains(t, err, "does not have enough stake tokens to delegate")
}

func TestKeeper_PlaceOrderWithBurnRate(t *testing.T) {
	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("some-hash"),
	})
	testSet := genTestSet(t, sdkCtx, testApp)

	acc, _ := testApp.GenAccount(sdkCtx)
	issuer, _ := testApp.GenAccount(sdkCtx)

	settingsWithBurnRate := assetfttypes.IssueSettings{
		Issuer:        issuer,
		Symbol:        "DEFEXT",
		Subunit:       "defext",
		Precision:     6,
		InitialAmount: sdkmath.NewIntWithDecimal(1, 10),
		Features: []assetfttypes.Feature{
			assetfttypes.Feature_burning,
		},
		BurnRate: sdkmath.LegacyMustNewDecFromStr("0.5"),
	}
	denomWithBurnRate, err := testApp.AssetFTKeeper.Issue(sdkCtx, settingsWithBurnRate)
	require.NoError(t, err)

	order := types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denomWithBurnRate,
		QuoteDenom: testSet.denom2,
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	lockedBalance, err := order.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.BankKeeper.SendCoins(sdkCtx, issuer, acc, sdk.NewCoins(lockedBalance)))
	fundOrderReserve(t, testApp, sdkCtx, acc)
	balanceBeforePlaceOrder := testApp.BankKeeper.GetBalance(sdkCtx, acc, denomWithBurnRate)
	require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, order))
	balanceAfterPlaceOrder := testApp.BankKeeper.GetBalance(sdkCtx, acc, denomWithBurnRate)
	require.Equal(t, balanceBeforePlaceOrder, balanceAfterPlaceOrder)
}

func TestKeeper_PlaceOrderWithCommissionRate(t *testing.T) {
	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Time:    time.Now(),
		AppHash: []byte("some-hash"),
	})
	testSet := genTestSet(t, sdkCtx, testApp)

	acc, _ := testApp.GenAccount(sdkCtx)
	issuer, _ := testApp.GenAccount(sdkCtx)

	settingsWithExtension := assetfttypes.IssueSettings{
		Issuer:             issuer,
		Symbol:             "DEFEXT",
		Subunit:            "defext",
		Precision:          6,
		InitialAmount:      sdkmath.NewIntWithDecimal(1, 10),
		SendCommissionRate: sdkmath.LegacyMustNewDecFromStr("0.5"),
	}
	denomWithCommissionRate, err := testApp.AssetFTKeeper.Issue(sdkCtx, settingsWithExtension)
	require.NoError(t, err)

	order := types.Order{
		Creator:    acc.String(),
		Type:       types.ORDER_TYPE_LIMIT,
		ID:         uuid.Generate().String(),
		BaseDenom:  denomWithCommissionRate,
		QuoteDenom: testSet.denom2,
		Price:      lo.ToPtr(types.MustNewPriceFromString("12e-1")),
		Quantity:   defaultQuantityStep,
		Side:       types.SIDE_SELL,
		GoodTil: &types.GoodTil{
			GoodTilBlockHeight: 390,
		},
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}
	lockedBalance, err := order.ComputeLimitOrderLockedBalance()
	require.NoError(t, err)
	require.NoError(t, testApp.BankKeeper.SendCoins(sdkCtx, issuer, acc, sdk.NewCoins(lockedBalance)))
	balanceBeforePlaceOrder := testApp.BankKeeper.GetBalance(sdkCtx, acc, denomWithCommissionRate)
	fundOrderReserve(t, testApp, sdkCtx, acc)
	require.NoError(t, testApp.DEXKeeper.PlaceOrder(sdkCtx, order))
	balanceAfterPlaceOrder := testApp.BankKeeper.GetBalance(sdkCtx, acc, denomWithCommissionRate)
	require.Equal(t, balanceBeforePlaceOrder, balanceAfterPlaceOrder)
}

// Regression for Immunefi 77114: maker places SELL, issuer freezes maker, taker
// attempts to match. Asserts settlement rejects the match with the spendable-check
// error, maker's balance stays intact, taker receives nothing, and balance >= frozen.
func TestDEXSettlement_RejectsMatchAgainstFrozenMaker_Immunefi77114(t *testing.T) {
	requireT := require.New(t)

	testApp := simapp.New()
	sdkCtx := testApp.NewContextLegacy(false, tmproto.Header{
		Height: 100,
		Time:   time.Now(),
	})

	ftKeeper := testApp.AssetFTKeeper
	bankKeeper := testApp.BankKeeper
	dexKeeper := testApp.DEXKeeper

	issuer, _ := testApp.GenAccount(sdkCtx)
	maker, _ := testApp.GenAccount(sdkCtx)
	taker, _ := testApp.GenAccount(sdkCtx)

	tDenom, err := ftKeeper.Issue(sdkCtx, assetfttypes.IssueSettings{
		Issuer:        issuer,
		Symbol:        "TTT",
		Subunit:       "ttt",
		Precision:     6,
		InitialAmount: sdkmath.NewIntWithDecimal(1, 18),
		Features:      []assetfttypes.Feature{assetfttypes.Feature_freezing},
	})
	requireT.NoError(err)

	qDenom, err := ftKeeper.Issue(sdkCtx, assetfttypes.IssueSettings{
		Issuer:        issuer,
		Symbol:        "QQQ",
		Subunit:       "qqq",
		Precision:     6,
		InitialAmount: sdkmath.NewIntWithDecimal(1, 18),
	})
	requireT.NoError(err)

	const qty int64 = 1_000_000
	price := lo.ToPtr(types.MustNewPriceFromString("1"))

	// Maker places SELL, locking qty T.
	requireT.NoError(bankKeeper.SendCoins(sdkCtx, issuer, maker,
		sdk.NewCoins(sdk.NewCoin(tDenom, sdkmath.NewInt(qty)))))
	fundOrderReserve(t, testApp, sdkCtx, maker)
	requireT.NoError(dexKeeper.PlaceOrder(sdkCtx, types.Order{
		Creator: maker.String(), Type: types.ORDER_TYPE_LIMIT, ID: "sell",
		BaseDenom: tDenom, QuoteDenom: qDenom, Price: price,
		Quantity: sdkmath.NewInt(qty), Side: types.SIDE_SELL,
		TimeInForce: types.TIME_IN_FORCE_GTC,
	}))

	// Issuer freezes maker's full T balance AFTER the order is placed.
	requireT.NoError(ftKeeper.Freeze(sdkCtx, issuer, maker,
		sdk.NewCoin(tDenom, sdkmath.NewInt(qty))))

	// Unprivileged taker attempts to match — must be rejected by settlement check.
	requireT.NoError(bankKeeper.SendCoins(sdkCtx, issuer, taker,
		sdk.NewCoins(sdk.NewCoin(qDenom, sdkmath.NewInt(qty)))))
	fundOrderReserve(t, testApp, sdkCtx, taker)
	err = dexKeeper.PlaceOrder(sdkCtx, types.Order{
		Creator: taker.String(), Type: types.ORDER_TYPE_LIMIT, ID: "buy",
		BaseDenom: tDenom, QuoteDenom: qDenom, Price: price,
		Quantity: sdkmath.NewInt(qty), Side: types.SIDE_BUY,
		TimeInForce: types.TIME_IN_FORCE_GTC,
	})
	requireT.Error(err)
	requireT.Contains(err.Error(), "DEX settlement: sender spendable check failed")

	// Invariant balance >= frozen holds; no tokens leaked to taker.
	makerT := bankKeeper.GetBalance(sdkCtx, maker, tDenom).Amount
	takerT := bankKeeper.GetBalance(sdkCtx, taker, tDenom).Amount
	frozen, err := ftKeeper.GetFrozenBalance(sdkCtx, maker, tDenom)
	requireT.NoError(err)

	requireT.Equal(sdkmath.NewInt(qty), makerT, "maker balance preserved")
	requireT.Equal(sdkmath.ZeroInt(), takerT, "taker received nothing")
	requireT.True(makerT.GTE(frozen.Amount),
		"invariant holds: balance(%s) >= frozen(%s)", makerT, frozen.Amount)
}
