package recoverylock

import (
	"bytes"
	"fmt"

	"cosmossdk.io/x/nft"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	dextypes "github.com/tokenize-x/tx-chain/v8/x/dex/types"
)

// checkRule runs only for a listed signer. The codec derives signer bytes from
// the same fields used to authenticate the transaction, including validator
// operator addresses for commission withdrawals.
func (l *Lock) checkRule(ctx sdk.Context, msg sdk.Msg, signer []byte) error {
	switch m := msg.(type) {
	case *banktypes.MsgSend:
		return l.checkDestination(m.ToAddress, signer, false)
	case *banktypes.MsgMultiSend:
		for _, output := range m.Outputs {
			if err := l.checkDestination(output.Address, signer, false); err != nil {
				return err
			}
		}
		return nil
	case *stakingtypes.MsgUndelegate, *distrtypes.MsgWithdrawDelegatorReward, *distrtypes.MsgWithdrawValidatorCommission:
		withdraw, err := l.distr.GetDelegatorWithdrawAddr(ctx, signer)
		if err != nil {
			return err
		}
		if !bytes.Equal(withdraw, signer) && !l.cfg.isRecovery(signer, withdraw) {
			return fmt.Errorf("set the withdraw address to the wallet or its recovery address first")
		}
		return nil
	case *distrtypes.MsgSetWithdrawAddress:
		return l.checkDestination(m.WithdrawAddress, signer, true)
	case *nft.MsgSend:
		// Native NFTs (x/nft wrapped by x/wnft, with the assetnft class
		// rules) leave an owner only through this message, signed by the
		// owner. The class rules (soulbound, disabled sending, whitelisting,
		// freezing) still apply to the transfer.
		return l.checkDestination(m.Receiver, signer, false)
	case *dextypes.MsgCancelOrder:
		// Cancelling the wallet's own order only releases its DEX locked
		// balance and reserve in the same account; nothing is transferred.
		return nil
	case *wasmtypes.MsgExecuteContract:
		if len(m.Funds) != 0 {
			return fmt.Errorf("contract execute with funds is not allowed")
		}
		recovery, err := l.addr.BytesToString(l.cfg.recoveries[string(signer)])
		if err != nil {
			return err
		}
		return checkContractExit(m.Msg, recovery, func(addr string) error {
			_, err := l.addr.StringToBytes(addr)
			return err
		})
	default:
		return fmt.Errorf("message %s is not allowed for a listed signer", sdk.MsgTypeURL(msg))
	}
}

func (l *Lock) checkDestination(destination string, signer []byte, allowSelf bool) error {
	addr, err := l.addr.StringToBytes(destination)
	if err != nil {
		return err
	}
	if l.cfg.isRecovery(signer, addr) || (allowSelf && bytes.Equal(signer, addr)) {
		return nil
	}
	return fmt.Errorf("destination %s is outside the allowed addresses for this message", destination)
}
