package keeper_test

import (
	"bytes"
	"testing"

	sdkmath "cosmossdk.io/math"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	"github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
	cwasmtypes "github.com/tokenize-x/tx-chain/v8/x/wasm/types"
	wibctransfertypes "github.com/tokenize-x/tx-chain/v8/x/wibctransfer/types"
)

// TestKeeper_IBCPurposeIgnoredForSmartContracts verifies that the IBC purpose exemptions
// (refunds on ack/timeout, incoming transfers) do not apply to sends triggered by a smart contract.
// Contracts run inside the IBC stack (ibc-hooks, ibc-callbacks), so they see the purpose set for the packet.
func TestKeeper_IBCPurposeIgnoredForSmartContracts(t *testing.T) {
	contractAddr := sdk.AccAddress(bytes.Repeat([]byte{1}, wasmtypes.ContractAddrLen))

	testCases := []struct {
		name            string
		purpose         wibctransfertypes.Purpose
		triggeredByWasm bool
		globalFreeze    bool
		freeze          bool
		whitelisted     bool
		expectedErr     error
	}{
		{
			name:            "ack_contract_frozen_balance",
			purpose:         wibctransfertypes.PurposeAck,
			triggeredByWasm: true,
			freeze:          true,
			whitelisted:     true,
			expectedErr:     cosmoserrors.ErrInsufficientFunds,
		},
		{
			name:            "timeout_contract_globally_frozen",
			purpose:         wibctransfertypes.PurposeTimeout,
			triggeredByWasm: true,
			globalFreeze:    true,
			whitelisted:     true,
			expectedErr:     types.ErrGloballyFrozen,
		},
		{
			name:            "ack_contract_recipient_not_whitelisted",
			purpose:         wibctransfertypes.PurposeAck,
			triggeredByWasm: true,
			expectedErr:     types.ErrWhitelistedLimitExceeded,
		},
		{
			name:            "in_contract_frozen_balance",
			purpose:         wibctransfertypes.PurposeIn,
			triggeredByWasm: true,
			freeze:          true,
			whitelisted:     true,
			expectedErr:     cosmoserrors.ErrInsufficientFunds,
		},
		{
			// MsgTransfer sent by a contract keeps the outgoing exemption: the escrow is not whitelisted.
			name:            "out_contract_recipient_not_whitelisted",
			purpose:         wibctransfertypes.PurposeOut,
			triggeredByWasm: true,
		},
		{
			// The refund made by the transfer module itself keeps its exemption.
			name:    "ack_refund_frozen_balance",
			purpose: wibctransfertypes.PurposeAck,
			freeze:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			testApp := simapp.New()
			ctx := testApp.NewContextLegacy(false, tmproto.Header{})
			ftKeeper := testApp.AssetFTKeeper
			bankKeeper := testApp.BankKeeper

			issuer := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
			holder := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
			recipient := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())

			denom, err := ftKeeper.Issue(ctx, types.IssueSettings{
				Issuer:        issuer,
				Symbol:        "ABC",
				Subunit:       "abc",
				Precision:     1,
				Description:   "ABC Desc",
				InitialAmount: sdkmath.NewInt(1000),
				Features: []types.Feature{
					types.Feature_freezing,
					types.Feature_whitelisting,
					types.Feature_ibc,
				},
			})
			requireT.NoError(err)
			coin := sdk.NewCoin(denom, sdkmath.NewInt(100))

			requireT.NoError(ftKeeper.SetWhitelistedBalance(ctx, issuer, holder, coin))
			requireT.NoError(bankKeeper.SendCoins(ctx, issuer, holder, sdk.NewCoins(coin)))
			if tc.whitelisted {
				requireT.NoError(ftKeeper.SetWhitelistedBalance(ctx, issuer, recipient, coin))
			}
			if tc.freeze {
				requireT.NoError(ftKeeper.Freeze(ctx, issuer, holder, coin))
			}
			if tc.globalFreeze {
				requireT.NoError(ftKeeper.GloballyFreeze(ctx, issuer, denom))
			}

			ctx = wibctransfertypes.WithPurpose(ctx, tc.purpose)
			if tc.triggeredByWasm {
				ctx = cwasmtypes.WithSmartContractSender(ctx, contractAddr.String())
			}

			err = bankKeeper.SendCoins(ctx, holder, recipient, sdk.NewCoins(coin))
			if tc.expectedErr != nil {
				requireT.ErrorIs(err, tc.expectedErr)
				return
			}
			requireT.NoError(err)
		})
	}
}
