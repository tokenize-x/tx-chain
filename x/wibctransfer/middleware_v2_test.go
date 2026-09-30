package wibctransfer_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	channeltypesv2 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/v2/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	assetfttypes "github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
)

// TestIBCV2TransferRoute_EnforcesIBCFeature sends through the transfer route of the app's IBC v2 router.
// A token whose issuer did not enable IBC must not be escrowed, the same as on IBC v1.
func TestIBCV2TransferRoute_EnforcesIBCFeature(t *testing.T) {
	testCases := []struct {
		name        string
		features    []assetfttypes.Feature
		expectedErr error
	}{
		{
			name:        "ibc_disabled",
			expectedErr: cosmoserrors.ErrUnauthorized,
		},
		{
			name:     "ibc_enabled",
			features: []assetfttypes.Feature{assetfttypes.Feature_ibc},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requireT := require.New(t)

			testApp := simapp.New()
			ctx := testApp.NewContextLegacy(false, tmproto.Header{})

			issuer := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
			sender := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())

			denom, err := testApp.AssetFTKeeper.Issue(ctx, assetfttypes.IssueSettings{
				Issuer:        issuer,
				Symbol:        "ABC",
				Subunit:       "abc",
				Precision:     1,
				Description:   "ABC Desc",
				InitialAmount: sdkmath.NewInt(1000),
				Features:      tc.features,
			})
			requireT.NoError(err)
			coin := sdk.NewCoin(denom, sdkmath.NewInt(100))
			requireT.NoError(testApp.BankKeeper.SendCoins(ctx, issuer, sender, sdk.NewCoins(coin)))

			data := ibctransfertypes.NewFungibleTokenPacketData(
				coin.Denom, coin.Amount.String(), sender.String(), "receiver", "",
			)
			bz, err := ibctransfertypes.MarshalPacketData(data, ibctransfertypes.V1, ibctransfertypes.EncodingProtobuf)
			requireT.NoError(err)
			payload := channeltypesv2.NewPayload(
				ibctransfertypes.PortID,
				ibctransfertypes.PortID,
				ibctransfertypes.V1,
				ibctransfertypes.EncodingProtobuf,
				bz,
			)

			route := testApp.IBCKeeper.ChannelKeeperV2.Router.Route(ibctransfertypes.PortID)
			err = route.OnSendPacket(ctx, "07-tendermint-0", "07-tendermint-1", 1, payload, sender)
			if tc.expectedErr != nil {
				requireT.ErrorIs(err, tc.expectedErr)
				requireT.Equal(coin.String(), testApp.BankKeeper.GetBalance(ctx, sender, denom).String())
				return
			}
			requireT.NoError(err)
			requireT.True(testApp.BankKeeper.GetBalance(ctx, sender, denom).IsZero())
		})
	}
}
