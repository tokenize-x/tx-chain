package keeper

import (
	"context"

	sdkerrors "cosmossdk.io/errors"
	cosmoserrors "github.com/cosmos/cosmos-sdk/types/errors"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	wdistrtypes "github.com/tokenize-x/tx-chain/v8/x/wdistribution/types"
)

// MsgServer wraps the distribution message server.
type MsgServer struct {
	distrtypes.MsgServer
	stakingKeeper wdistrtypes.StakingKeeper
}

// NewMsgServerImpl returns the wrapped distribution message server.
func NewMsgServerImpl(
	distrMsgSrv distrtypes.MsgServer, stakingKeeper wdistrtypes.StakingKeeper,
) distrtypes.MsgServer {
	return MsgServer{
		MsgServer:     distrMsgSrv,
		stakingKeeper: stakingKeeper,
	}
}

// DepositValidatorRewardsPool only accepts the bond denom.
// The check runs in the message handler, so it applies to every path that dispatches the message.
// That includes group proposals, smart contracts and ICA, which do not pass through the ante handler.
func (s MsgServer) DepositValidatorRewardsPool(
	ctx context.Context, msg *distrtypes.MsgDepositValidatorRewardsPool,
) (*distrtypes.MsgDepositValidatorRewardsPoolResponse, error) {
	bondDenom, err := s.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	for _, coin := range msg.Amount {
		if coin.Denom != bondDenom {
			return nil, sdkerrors.Wrapf(
				cosmoserrors.ErrInvalidRequest,
				"only the bond denom %q can be deposited into validator reward pools, got %q",
				bondDenom, coin.Denom,
			)
		}
	}

	return s.MsgServer.DepositValidatorRewardsPool(ctx, msg)
}
