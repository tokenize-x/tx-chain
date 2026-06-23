package ante

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	"github.com/stretchr/testify/require"
)

// TestCollectDepositValidatorRewardsPoolMsgs checks deposits are found whether
// submitted directly or nested (recursively) inside authz.MsgExec, so the denom
// restriction cannot be bypassed by wrapping the deposit in an authz exec.
func TestCollectDepositValidatorRewardsPoolMsgs(t *testing.T) {
	addr := sdk.AccAddress("test_address________")

	deposit := &distributiontypes.MsgDepositValidatorRewardsPool{
		Depositor:        addr.String(),
		ValidatorAddress: sdk.ValAddress(addr).String(),
		Amount:           sdk.NewCoins(sdk.NewInt64Coin("upoison", 1_000_000)),
	}
	otherMsg := &banktypes.MsgSend{FromAddress: addr.String(), ToAddress: addr.String()}

	authzWrap := func(msgs ...sdk.Msg) sdk.Msg {
		exec := authz.NewMsgExec(addr, msgs)
		return &exec
	}

	testCases := []struct {
		name      string
		msgs      []sdk.Msg
		wantCount int
	}{
		{
			name:      "direct deposit",
			msgs:      []sdk.Msg{deposit},
			wantCount: 1,
		},
		{
			name:      "no deposit",
			msgs:      []sdk.Msg{otherMsg},
			wantCount: 0,
		},
		{
			name:      "deposit wrapped in authz.MsgExec",
			msgs:      []sdk.Msg{authzWrap(deposit)},
			wantCount: 1,
		},
		{
			name:      "deposit double-wrapped in authz.MsgExec",
			msgs:      []sdk.Msg{authzWrap(authzWrap(deposit))},
			wantCount: 1,
		},
		{
			name:      "deposit alongside unrelated msg inside authz.MsgExec",
			msgs:      []sdk.Msg{authzWrap(otherMsg, deposit)},
			wantCount: 1,
		},
		{
			name:      "multiple deposits direct and nested",
			msgs:      []sdk.Msg{deposit, authzWrap(deposit), otherMsg},
			wantCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := collectDepositValidatorRewardsPoolMsgs(tc.msgs)
			require.Len(t, got, tc.wantCount)
		})
	}
}
