//go:build integrationtests

package ibc

import (
	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
)

// ConvertToIBCDenom returns the IBC denom based on the channelID and denom.
func ConvertToIBCDenom(channelID, denom string) string {
	return ibctransfertypes.NewDenom(denom, ibctransfertypes.NewHop(ibctransfertypes.PortID, channelID)).IBCDenom()
}
