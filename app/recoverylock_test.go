//go:build recoverylock_test

package app_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/collections"
	coreaddress "cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	evidencetypes "cosmossdk.io/x/evidence/types"
	"cosmossdk.io/x/feegrant"
	"cosmossdk.io/x/nft"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	vestingtypes "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	govv1beta1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1beta1"
	"github.com/cosmos/cosmos-sdk/x/group"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/cosmos/gogoproto/proto"
	icacontrollertypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/controller/types"
	icahosttypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/host/types"
	icatypes "github.com/cosmos/ibc-go/v10/modules/apps/27-interchain-accounts/types"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	clientv2 "github.com/cosmos/ibc-go/v10/modules/core/02-client/v2/types"
	connectiontypes "github.com/cosmos/ibc-go/v10/modules/core/03-connection/types"
	channelv1 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/types"
	channelv2 "github.com/cosmos/ibc-go/v10/modules/core/04-channel/v2/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/app"
	"github.com/tokenize-x/tx-chain/v8/app/recoverylock"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
	assetfttypes "github.com/tokenize-x/tx-chain/v8/x/asset/ft/types"
	assetnfttypes "github.com/tokenize-x/tx-chain/v8/x/asset/nft/types"
	customparamstypes "github.com/tokenize-x/tx-chain/v8/x/customparams/types"
	dextypes "github.com/tokenize-x/tx-chain/v8/x/dex/types"
	feemodeltypes "github.com/tokenize-x/tx-chain/v8/x/feemodel/types"
	psetypes "github.com/tokenize-x/tx-chain/v8/x/pse/types"
)

const lockHeight = 20

var lockTime = time.Unix(1_750_000_000, 0)

// useLockTestConfig enables the lock with the test values in
// recoverylock/testdata for apps and configs built during the test.
func useLockTestConfig(t testing.TB) {
	t.Helper()
	listed, err := os.ReadFile("recoverylock/testdata/listed.txt")
	require.NoError(t, err)
	params, err := os.ReadFile("recoverylock/testdata/params.json")
	require.NoError(t, err)
	t.Cleanup(recoverylock.UseTestConfig(listed, params))
}

func newLockApp(t testing.TB) *simapp.App {
	t.Helper()
	a := simapp.New(simapp.WithStartTime(lockTime.Add(-time.Hour)))
	return a
}

// TestRecoveryLockDisabledByDefault checks that the committed configuration
// leaves the app without a lock.
func TestRecoveryLockDisabledByDefault(t *testing.T) {
	a := newLockApp(t)
	require.Nil(t, a.RecoveryLock)
}

func placeholderKey(seed string) *secp256k1.PrivKey {
	digest := sha256.Sum256([]byte(seed))
	return &secp256k1.PrivKey{Key: digest[:]}
}

func placeholderAddress(seed string) sdk.AccAddress {
	return sdk.AccAddress(placeholderKey(seed).PubKey().Address())
}

type withdrawStore struct {
	addr           sdk.AccAddress
	reads, deletes int
}

func (d *withdrawStore) GetDelegatorWithdrawAddr(_ context.Context, wallet sdk.AccAddress) (sdk.AccAddress, error) {
	d.reads++
	if d.addr == nil {
		return wallet, nil
	}
	return d.addr, nil
}

func (d *withdrawStore) DeleteDelegatorWithdrawAddr(_ context.Context, _, _ sdk.AccAddress) error {
	d.deletes++
	d.addr = nil
	return nil
}

type lockFixture struct {
	a                              *simapp.App
	lock                           *recoverylock.Lock
	ctx                            sdk.Context
	withdraw                       *withdrawStore
	wallet, recovery, payer, other sdk.AccAddress
	ac                             coreaddress.Codec
}

func newLockFixture(t *testing.T) lockFixture {
	t.Helper()
	a := newLockApp(t)
	ac := a.InterfaceRegistry().SigningContext().AddressCodec()
	useLockTestConfig(t)
	cfg := recoverylock.MustConfig(string(app.ChosenNetwork.ChainID()), ac, nil, nil)
	require.NotNil(t, cfg)
	withdraw := &withdrawStore{}
	return lockFixture{
		a:        a,
		lock:     recoverylock.New(cfg, a.AppCodec(), ac, withdraw, func() int64 { return lockHeight - 1 }),
		ctx:      sdk.Context{}.WithContext(context.Background()).WithBlockHeight(lockHeight).WithBlockTime(lockTime),
		withdraw: withdraw,
		wallet:   placeholderAddress("recoverylock-test-listed-0"),
		recovery: placeholderAddress("recoverylock-test-recovery"),
		payer:    placeholderAddress("recoverylock-test-fee-payer-0"),
		other:    placeholderAddress("recoverylock-unlisted"),
		ac:       ac,
	}
}

func (f lockFixture) acc(t *testing.T, a sdk.AccAddress) string {
	t.Helper()
	s, err := f.ac.BytesToString(a)
	require.NoError(t, err)
	return s
}

// lockDenom is the bond and fee denom of the simapp genesis.
func lockDenom() string { return sdk.DefaultBondDenom }

func lockCoins(amount int64) sdk.Coins {
	return sdk.NewCoins(sdk.NewInt64Coin(lockDenom(), amount))
}

func (f lockFixture) tx(t *testing.T, payer, granter sdk.AccAddress, amt int64, msgs ...sdk.Msg) sdk.Tx {
	t.Helper()
	b := f.a.TxConfig().NewTxBuilder()
	require.NoError(t, b.SetMsgs(msgs...))
	if payer != nil {
		b.SetFeePayer(payer)
	}
	if granter != nil {
		b.SetFeeGranter(granter)
	}
	b.SetFeeAmount(lockCoins(amt))
	return b.GetTx()
}

func (f lockFixture) ante(t *testing.T, ctx sdk.Context, tx sdk.Tx) error {
	t.Helper()
	called := false
	_, err := (recoverylock.Decorator{L: f.lock}).AnteHandle(ctx, tx, false,
		func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
			called = true
			return ctx, nil
		})
	require.Equal(t, err == nil, called)
	return err
}

// blockedFixtures returns a message with a listed signer for every message
// type registered on the app that the lock refuses for a listed signer.
func blockedFixtures(t *testing.T, f lockFixture) []sdk.Msg {
	t.Helper()
	w, o, r := f.acc(t, f.wallet), f.acc(t, f.other), f.acc(t, f.recovery)
	validator, err := address.NewBech32Codec(app.ChosenNetwork.Provider.GetAddressPrefix() + "valoper").BytesToString(f.wallet)
	require.NoError(t, err)
	// These are signer fixtures. The lock must deny them before their handlers
	// validate module-specific fields or look up live state.
	return []sdk.Msg{
		&banktypes.MsgSend{FromAddress: w, ToAddress: o},
		&banktypes.MsgSend{FromAddress: w, ToAddress: w},
		&banktypes.MsgMultiSend{Inputs: []banktypes.Input{{Address: w}}, Outputs: []banktypes.Output{{Address: r}, {Address: o}}},
		&banktypes.MsgUpdateParams{Authority: w},
		&banktypes.MsgSetSendEnabled{Authority: w},
		&authtypes.MsgUpdateParams{Authority: w},
		&stakingtypes.MsgCreateValidator{ValidatorAddress: validator},
		&stakingtypes.MsgEditValidator{ValidatorAddress: validator},
		&stakingtypes.MsgDelegate{DelegatorAddress: w},
		&stakingtypes.MsgBeginRedelegate{DelegatorAddress: w},
		&stakingtypes.MsgCancelUnbondingDelegation{DelegatorAddress: w},
		&stakingtypes.MsgUpdateParams{Authority: w},
		&slashingtypes.MsgUnjail{ValidatorAddr: validator},
		&slashingtypes.MsgUpdateParams{Authority: w},
		&distrtypes.MsgSetWithdrawAddress{DelegatorAddress: w, WithdrawAddress: o},
		&distrtypes.MsgFundCommunityPool{Depositor: w},
		&distrtypes.MsgDepositValidatorRewardsPool{Depositor: w},
		&distrtypes.MsgCommunityPoolSpend{Authority: w},
		&distrtypes.MsgUpdateParams{Authority: w},
		&minttypes.MsgUpdateParams{Authority: w},
		&govv1.MsgSubmitProposal{Proposer: w},
		&govv1beta1.MsgSubmitProposal{Proposer: w},
		&govv1.MsgDeposit{Depositor: w},
		&govv1beta1.MsgDeposit{Depositor: w},
		&govv1.MsgCancelProposal{Proposer: w},
		&govv1.MsgExecLegacyContent{Authority: w},
		&govv1.MsgVote{Voter: w},
		&govv1.MsgVoteWeighted{Voter: w},
		&govv1beta1.MsgVote{Voter: w},
		&govv1beta1.MsgVoteWeighted{Voter: w},
		&govv1.MsgUpdateParams{Authority: w},
		&group.MsgCreateGroup{Admin: w},
		&group.MsgUpdateGroupMembers{Admin: w},
		&group.MsgUpdateGroupAdmin{Admin: w},
		&group.MsgUpdateGroupMetadata{Admin: w},
		&group.MsgCreateGroupPolicy{Admin: w},
		&group.MsgCreateGroupWithPolicy{Admin: w},
		&group.MsgUpdateGroupPolicyAdmin{Admin: w},
		&group.MsgUpdateGroupPolicyDecisionPolicy{Admin: w},
		&group.MsgUpdateGroupPolicyMetadata{Admin: w},
		&group.MsgSubmitProposal{Proposers: []string{w}},
		&group.MsgWithdrawProposal{Address: w},
		&group.MsgVote{Voter: w},
		&group.MsgExec{Executor: w},
		&group.MsgLeaveGroup{Address: w},
		&vestingtypes.MsgCreateVestingAccount{FromAddress: w},
		&vestingtypes.MsgCreatePermanentLockedAccount{FromAddress: w},
		&vestingtypes.MsgCreatePeriodicVestingAccount{FromAddress: w},
		&authz.MsgGrant{Granter: w, Grantee: o},
		&authz.MsgExec{Grantee: w},
		&feegrant.MsgGrantAllowance{Granter: w, Grantee: o},
		&feegrant.MsgRevokeAllowance{Granter: w, Grantee: o},
		&feegrant.MsgPruneAllowances{Pruner: w},
		&upgradetypes.MsgSoftwareUpgrade{Authority: w},
		&upgradetypes.MsgCancelUpgrade{Authority: w},
		&consensustypes.MsgUpdateParams{Authority: w},
		&evidencetypes.MsgSubmitEvidence{Submitter: w},
		&nft.MsgSend{Sender: w, Receiver: o, ClassId: "c", Id: "1"},
		&nft.MsgSend{Sender: w, Receiver: w, ClassId: "c", Id: "1"},
		&transfertypes.MsgTransfer{Sender: w, Token: lockCoins(1)[0]},
		&transfertypes.MsgTransfer{Sender: w, Receiver: r, Token: lockCoins(1)[0]},
		&transfertypes.MsgUpdateParams{Signer: w},
		&icacontrollertypes.MsgRegisterInterchainAccount{Owner: w},
		&icacontrollertypes.MsgSendTx{Owner: w},
		&icacontrollertypes.MsgUpdateParams{Signer: w},
		&icahosttypes.MsgUpdateParams{Signer: w},
		&icahosttypes.MsgModuleQuerySafe{Signer: w},
		&clienttypes.MsgCreateClient{Signer: w},
		&clienttypes.MsgUpdateClient{Signer: w},
		&clienttypes.MsgUpgradeClient{Signer: w},
		&clienttypes.MsgSubmitMisbehaviour{Signer: w},
		&clienttypes.MsgRecoverClient{Signer: w},
		&clienttypes.MsgIBCSoftwareUpgrade{Signer: w},
		&clienttypes.MsgUpdateParams{Signer: w},
		&clienttypes.MsgDeleteClientCreator{Signer: w},
		&clientv2.MsgRegisterCounterparty{Signer: w},
		&clientv2.MsgUpdateClientConfig{Signer: w},
		&connectiontypes.MsgConnectionOpenInit{Signer: w},
		&connectiontypes.MsgConnectionOpenTry{Signer: w},
		&connectiontypes.MsgConnectionOpenAck{Signer: w},
		&connectiontypes.MsgConnectionOpenConfirm{Signer: w},
		&connectiontypes.MsgUpdateParams{Signer: w},
		&channelv1.MsgChannelOpenInit{Signer: w},
		&channelv1.MsgChannelOpenTry{Signer: w},
		&channelv1.MsgChannelOpenAck{Signer: w},
		&channelv1.MsgChannelOpenConfirm{Signer: w},
		&channelv1.MsgChannelCloseInit{Signer: w},
		&channelv1.MsgChannelCloseConfirm{Signer: w},
		&channelv1.MsgRecvPacket{Signer: w},
		&channelv1.MsgAcknowledgement{Signer: w},
		&channelv1.MsgTimeout{Signer: w},
		&channelv1.MsgTimeoutOnClose{Signer: w},
		&channelv2.MsgSendPacket{Signer: w},
		&channelv2.MsgRecvPacket{Signer: w},
		&channelv2.MsgAcknowledgement{Signer: w},
		&channelv2.MsgTimeout{Signer: w},
		&wasmtypes.MsgStoreCode{Sender: w},
		&wasmtypes.MsgInstantiateContract{Sender: w},
		&wasmtypes.MsgInstantiateContract2{Sender: w},
		&wasmtypes.MsgMigrateContract{Sender: w},
		&wasmtypes.MsgStoreAndInstantiateContract{Authority: w},
		&wasmtypes.MsgStoreAndMigrateContract{Authority: w},
		&wasmtypes.MsgUpdateAdmin{Sender: w},
		&wasmtypes.MsgClearAdmin{Sender: w},
		&wasmtypes.MsgUpdateContractLabel{Sender: w},
		&wasmtypes.MsgSudoContract{Authority: w},
		&wasmtypes.MsgPinCodes{Authority: w},
		&wasmtypes.MsgUnpinCodes{Authority: w},
		&wasmtypes.MsgUpdateInstantiateConfig{Sender: w},
		&wasmtypes.MsgAddCodeUploadParamsAddresses{Authority: w},
		&wasmtypes.MsgRemoveCodeUploadParamsAddresses{Authority: w},
		&wasmtypes.MsgUpdateParams{Authority: w},
		&assetfttypes.MsgIssue{Issuer: w},
		&assetfttypes.MsgMint{Sender: w},
		&assetfttypes.MsgBurn{Sender: w},
		&assetfttypes.MsgFreeze{Sender: w},
		&assetfttypes.MsgUnfreeze{Sender: w},
		&assetfttypes.MsgSetFrozen{Sender: w},
		&assetfttypes.MsgGloballyFreeze{Sender: w},
		&assetfttypes.MsgGloballyUnfreeze{Sender: w},
		&assetfttypes.MsgClawback{Sender: w},
		&assetfttypes.MsgSetWhitelistedLimit{Sender: w},
		&assetfttypes.MsgTransferAdmin{Sender: w},
		&assetfttypes.MsgClearAdmin{Sender: w},
		&assetfttypes.MsgUpdateParams{Authority: w},
		&assetfttypes.MsgUpdateDEXUnifiedRefAmount{Sender: w},
		&assetfttypes.MsgUpdateDEXWhitelistedDenoms{Sender: w},
		&assetnfttypes.MsgIssueClass{Issuer: w},
		&assetnfttypes.MsgMint{Sender: w},
		&assetnfttypes.MsgUpdateData{Sender: w},
		&assetnfttypes.MsgBurn{Sender: w},
		&assetnfttypes.MsgFreeze{Sender: w},
		&assetnfttypes.MsgUnfreeze{Sender: w},
		&assetnfttypes.MsgAddToWhitelist{Sender: w},
		&assetnfttypes.MsgRemoveFromWhitelist{Sender: w},
		&assetnfttypes.MsgAddToClassWhitelist{Sender: w},
		&assetnfttypes.MsgRemoveFromClassWhitelist{Sender: w},
		&assetnfttypes.MsgClassFreeze{Sender: w},
		&assetnfttypes.MsgClassUnfreeze{Sender: w},
		&assetnfttypes.MsgUpdateParams{Authority: w},
		&dextypes.MsgPlaceOrder{Sender: w},
		&dextypes.MsgCancelOrdersByDenom{Sender: w, Account: w},
		&dextypes.MsgUpdateParams{Authority: w},
		&feemodeltypes.MsgUpdateParams{Authority: w},
		&customparamstypes.MsgUpdateStakingParams{Authority: w},
		&psetypes.MsgUpdateExcludedAddresses{Authority: w},
		&psetypes.MsgUpdateClearingAccountMappings{Authority: w},
		&psetypes.MsgUpdateDistributionSchedule{Authority: w},
		&psetypes.MsgDisableDistributions{Authority: w},
		&psetypes.MsgUpdateMinDistributionGap{Authority: w},
		&psetypes.MsgUpdateDistributionBatchSize{Authority: w},
	}
}

func allowedFixtures(t *testing.T, f lockFixture) []sdk.Msg {
	t.Helper()
	w, r, o := f.acc(t, f.wallet), f.acc(t, f.recovery), f.acc(t, f.other)
	validator, err := address.NewBech32Codec(app.ChosenNetwork.Provider.GetAddressPrefix() + "valoper").BytesToString(f.wallet)
	require.NoError(t, err)
	return []sdk.Msg{
		&banktypes.MsgSend{FromAddress: w, ToAddress: r, Amount: lockCoins(2)},
		&banktypes.MsgMultiSend{Inputs: []banktypes.Input{{Address: w, Coins: lockCoins(2)}}, Outputs: []banktypes.Output{{Address: r, Coins: lockCoins(2)}}},
		&distrtypes.MsgSetWithdrawAddress{DelegatorAddress: w, WithdrawAddress: w},
		&distrtypes.MsgSetWithdrawAddress{DelegatorAddress: w, WithdrawAddress: r},
		&nft.MsgSend{Sender: w, Receiver: r, ClassId: "c", Id: "1"},
		&dextypes.MsgCancelOrder{Sender: w, ID: "1"},
		&authz.MsgRevoke{Granter: w, Grantee: o, MsgTypeUrl: sdk.MsgTypeURL(&stakingtypes.MsgDelegate{})},
		&stakingtypes.MsgUndelegate{DelegatorAddress: w, ValidatorAddress: validator, Amount: lockCoins(2)[0]},
		&distrtypes.MsgWithdrawDelegatorReward{DelegatorAddress: w, ValidatorAddress: validator},
		&distrtypes.MsgWithdrawValidatorCommission{ValidatorAddress: validator},
		&wasmtypes.MsgExecuteContract{Sender: w, Contract: o, Msg: []byte(fmt.Sprintf(`{"transfer":{"recipient":%q,"amount":"1"}}`, r))},
	}
}

func TestRecoveryLockBlockedPaths(t *testing.T) {
	f := newLockFixture(t)
	for _, msg := range blockedFixtures(t, f) {
		t.Run(sdk.MsgTypeURL(msg), func(t *testing.T) {
			tx := f.tx(t, f.payer, nil, 1, msg)
			require.ErrorIs(t, f.ante(t, f.ctx, tx), recoverylock.ErrLocked)
			router := recoverylock.Router{Inner: testRouter{}, L: f.lock}
			_, err := router.Handler(msg)(f.ctx, msg)
			require.ErrorIs(t, err, recoverylock.ErrLocked)
		})
	}
}

// TestRecoveryLockCoversEveryMessage checks that the fixtures above name
// every message type the app routes, so a message added in a later release
// (for example in a newer release) shows up here and is reviewed.
func TestRecoveryLockCoversEveryMessage(t *testing.T) {
	f := newLockFixture(t)
	covered := map[string]bool{}
	for _, msg := range append(blockedFixtures(t, f), allowedFixtures(t, f)...) {
		covered[sdk.MsgTypeURL(msg)] = true
	}
	var missing []string
	for _, url := range f.a.InterfaceRegistry().ListImplementations(sdk.MsgInterfaceProtoName) {
		if f.a.MsgServiceRouter().HandlerByTypeURL(url) != nil && !covered[url] {
			missing = append(missing, url)
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing)
	for url := range covered {
		require.NotNil(t, f.a.MsgServiceRouter().HandlerByTypeURL(url), url)
	}
}

func TestRecoveryLockAllowedPaths(t *testing.T) {
	f := newLockFixture(t)
	w, r, o := f.acc(t, f.wallet), f.acc(t, f.recovery), f.acc(t, f.other)
	for _, msg := range allowedFixtures(t, f) {
		require.NoError(t, f.ante(t, f.ctx, f.tx(t, f.payer, nil, 1, msg)), "%T", msg)
		// allowed only with a configured fee payer
		require.ErrorIs(t, f.ante(t, f.ctx, f.tx(t, f.wallet, nil, 1, msg)), recoverylock.ErrLocked, "%T", msg)
		require.ErrorIs(t, f.ante(t, f.ctx, f.tx(t, nil, nil, 1, msg)), recoverylock.ErrLocked, "%T", msg)
	}
	validator, err := address.NewBech32Codec(app.ChosenNetwork.Provider.GetAddressPrefix() + "valoper").BytesToString(f.wallet)
	require.NoError(t, err)
	for _, msg := range []sdk.Msg{
		&stakingtypes.MsgUndelegate{DelegatorAddress: w, ValidatorAddress: validator, Amount: lockCoins(2)[0]},
		&distrtypes.MsgWithdrawDelegatorReward{DelegatorAddress: w, ValidatorAddress: validator},
		&distrtypes.MsgWithdrawValidatorCommission{ValidatorAddress: validator},
	} {
		tx := f.tx(t, f.payer, nil, 1, msg)
		for _, dest := range []sdk.AccAddress{f.wallet, f.recovery} {
			f.withdraw.addr = dest
			require.NoError(t, f.ante(t, f.ctx, tx), "%T", msg)
		}
		f.withdraw.addr = f.other
		require.ErrorIs(t, f.ante(t, f.ctx, tx), recoverylock.ErrLocked, "%T", msg)
		f.withdraw.addr = nil
	}
	for _, body := range []string{
		fmt.Sprintf(`{"transfer":{"recipient":%q,"amount":"1"}}`, r),
		fmt.Sprintf(`{"transfer_nft":{"recipient":%q,"token_id":"1"}}`, r),
		fmt.Sprintf(`{"decrease_allowance":{"spender":%q,"amount":"1"}}`, o),
		fmt.Sprintf(`{"revoke":{"spender":%q,"token_id":"1"}}`, o),
		fmt.Sprintf(`{"revoke_all":{"operator":%q}}`, o),
	} {
		msg := &wasmtypes.MsgExecuteContract{Sender: w, Contract: o, Msg: []byte(body)}
		require.NoError(t, f.ante(t, f.ctx, f.tx(t, f.payer, nil, 1, msg)))
		msg.Funds = lockCoins(1)
		require.ErrorIs(t, f.ante(t, f.ctx, f.tx(t, f.payer, nil, 1, msg)), recoverylock.ErrLocked)
	}
	for _, body := range []string{
		fmt.Sprintf(`{"transfer":{"recipient":%q,"amount":"1"}}`, o),
		fmt.Sprintf(`{"transfer_nft":{"recipient":%q,"token_id":"1"}}`, o),
		fmt.Sprintf(`{"transfer":{"recipient":%q,"recipient":%q,"amount":"1"}}`, o, r),
		fmt.Sprintf(`{"transfer":{"recipient":%q,"amount":1}}`, r),
		fmt.Sprintf(`{"decrease_allowance":{"spender":%q,"amount":"1","expires":{}}}`, o),
		`{"increase_allowance":{}}`, `{"approve":{}}`, `{"send":{}}`, `{"send_nft":{}}`,
		`{"revoke_all":{}} {}`,
	} {
		msg := &wasmtypes.MsgExecuteContract{Sender: w, Contract: o, Msg: []byte(body)}
		require.ErrorIs(t, f.ante(t, f.ctx, f.tx(t, f.payer, nil, 1, msg)), recoverylock.ErrLocked)
	}
}

type testRouter struct{}

func (testRouter) Handler(sdk.Msg) baseapp.MsgServiceHandler {
	return func(sdk.Context, sdk.Msg) (*sdk.Result, error) { return &sdk.Result{}, nil }
}

func (r testRouter) HandlerByTypeURL(string) baseapp.MsgServiceHandler { return r.Handler(nil) }

func TestRecoveryLockFeesWindowAndDispatch(t *testing.T) {
	f := newLockFixture(t)
	w, r, o := f.acc(t, f.wallet), f.acc(t, f.recovery), f.acc(t, f.other)
	bad := &banktypes.MsgSend{FromAddress: w, ToAddress: o, Amount: lockCoins(2)}
	good := &banktypes.MsgSend{FromAddress: w, ToAddress: r, Amount: lockCoins(2)}
	ordinary := &banktypes.MsgSend{FromAddress: o, ToAddress: w, Amount: lockCoins(2)}
	for _, tx := range []sdk.Tx{
		f.tx(t, nil, nil, 1, good),
		f.tx(t, f.other, nil, 1, good),
		f.tx(t, f.payer, nil, 0, good),
		f.tx(t, f.payer, f.other, 1, good),
		f.tx(t, f.other, f.wallet, 1, ordinary),
		f.tx(t, f.wallet, nil, 1, ordinary),
		f.tx(t, f.payer, nil, 1, ordinary, bad),
	} {
		require.ErrorIs(t, f.ante(t, f.ctx, tx), recoverylock.ErrLocked)
	}
	require.NoError(t, f.ante(t, f.ctx, f.tx(t, nil, f.other, 1, ordinary)))
	tx := f.tx(t, nil, nil, 1, bad)
	before := f.ctx.WithBlockHeight(lockHeight - 1)
	require.NoError(t, f.ante(t, before, tx))
	require.ErrorIs(t, f.ante(t, f.ctx, tx), recoverylock.ErrLocked)
	require.NoError(t, f.ante(t, f.ctx.WithBlockTime(time.Unix(4102444800, 0)), tx))
	require.ErrorIs(t, f.ante(t, before.WithIsCheckTx(true), tx), recoverylock.ErrLocked)
	restart := f.ctx.WithBlockHeight(0).WithBlockTime(time.Time{}).WithIsCheckTx(true)
	require.ErrorIs(t, f.ante(t, restart, tx), recoverylock.ErrLocked)

	wrap := func(msg sdk.Msg) sdk.Msg {
		exec := authz.NewMsgExec(f.other, []sdk.Msg{msg})
		return &exec
	}
	nestedGood, nestedBad := wrap(wrap(good)), wrap(wrap(bad))
	require.NoError(t, f.ante(t, f.ctx, f.tx(t, f.payer, nil, 1, nestedGood)))
	require.ErrorIs(t, f.ante(t, f.ctx, f.tx(t, nil, nil, 1, nestedGood)), recoverylock.ErrLocked)
	require.ErrorIs(t, f.ante(t, f.ctx, f.tx(t, f.payer, nil, 1, nestedBad)), recoverylock.ErrLocked)
	router := recoverylock.Router{Inner: testRouter{}, L: f.lock}
	_, err := router.Handler(nestedGood)(f.ctx, nestedGood)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	_, err = router.HandlerByTypeURL(sdk.MsgTypeURL(good))(f.ctx, good)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	_, err = router.Handler(ordinary)(f.ctx, ordinary)
	require.NoError(t, err)
	_, err = router.Handler(good)(before, good)
	require.NoError(t, err)
	deep := func(msg sdk.Msg) sdk.Msg {
		for i := 0; i <= recoverylock.MaxDepth; i++ {
			msg = wrap(msg)
		}
		return msg
	}
	deepUnlisted := deep(ordinary)
	require.NoError(t, f.ante(t, f.ctx, f.tx(t, nil, nil, 1, deepUnlisted)))
	for _, tx := range []sdk.Tx{
		f.tx(t, f.wallet, nil, 1, deepUnlisted),
		f.tx(t, f.payer, f.wallet, 1, deepUnlisted),
		f.tx(t, f.payer, nil, 1, deepUnlisted, good),
	} {
		err := f.ante(t, f.ctx, tx)
		require.ErrorIs(t, err, recoverylock.ErrLocked)
		require.ErrorContains(t, err, "message nesting deeper")
	}
	deepListed := deep(good)
	_, err = router.Handler(deepListed)(f.ctx, deepListed)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	require.ErrorContains(t, err, "listed accounts may act only")
}

func TestRecoveryLockStakingHooks(t *testing.T) {
	f := newLockFixture(t)
	hooks := recoverylock.StakingHooks{L: f.lock}
	f.withdraw.addr = f.other
	require.NoError(t, hooks.BeforeDelegationSharesModified(f.ctx.WithBlockHeight(lockHeight-1), f.wallet, nil))
	require.NoError(t, hooks.BeforeDelegationRemoved(f.ctx, f.other, nil))
	require.Zero(t, f.withdraw.reads)
	require.NoError(t, hooks.BeforeDelegationSharesModified(f.ctx, f.wallet, nil))
	require.Equal(t, 1, f.withdraw.deletes)
	f.withdraw.addr = f.recovery
	require.NoError(t, hooks.BeforeDelegationRemoved(f.ctx, f.wallet, nil))
	require.Equal(t, 1, f.withdraw.deletes)
	f.withdraw.addr = f.other
	require.NoError(t, hooks.AfterValidatorRemoved(f.ctx, nil, sdk.ValAddress(f.wallet)))
	require.Equal(t, 2, f.withdraw.deletes)
}

// lockAppEnv is an app with the lock wired, funded test accounts and a block
// context at the activation height.
type lockAppEnv struct {
	a                              *simapp.App
	ctx                            sdk.Context
	walletKey, payerKey, otherKey  *secp256k1.PrivKey
	wallet, payer, other, recovery sdk.AccAddress
	valAddr                        sdk.ValAddress
}

func newLockAppEnv(t *testing.T) lockAppEnv {
	t.Helper()
	return newLockAppEnvWithLock(t, true)
}

func newLockAppEnvWithLock(t *testing.T, enabled bool) lockAppEnv {
	t.Helper()
	if enabled {
		useLockTestConfig(t)
	}
	a := newLockApp(t)
	require.Equal(t, enabled, a.RecoveryLock != nil)
	ctx := a.NewContextLegacy(false, cmtproto.Header{Height: lockHeight, Time: lockTime, ChainID: a.ChainID()})
	e := lockAppEnv{
		a: a, ctx: ctx,
		walletKey: placeholderKey("recoverylock-test-listed-0"),
		payerKey:  placeholderKey("recoverylock-test-fee-payer-0"),
		otherKey:  placeholderKey("recoverylock-unlisted"),
		recovery:  placeholderAddress("recoverylock-test-recovery"),
	}
	e.wallet = sdk.AccAddress(e.walletKey.PubKey().Address())
	e.payer = sdk.AccAddress(e.payerKey.PubKey().Address())
	e.other = sdk.AccAddress(e.otherKey.PubKey().Address())
	for _, addr := range []sdk.AccAddress{e.wallet, e.payer, e.other} {
		a.AccountKeeper.SetAccount(ctx, a.AccountKeeper.NewAccountWithAddress(ctx, addr))
		require.NoError(t, a.FundAccount(ctx, addr, lockCoins(100_000_000)))
	}
	// a validator, created before activation, that the wallet delegates to
	before := ctx.WithBlockHeight(lockHeight - 1)
	operator, _ := a.GenAccount(before)
	require.NoError(t, a.FundAccount(before, operator, lockCoins(100_000_000)))
	val, err := a.AddValidator(before, operator, sdk.NewInt64Coin(lockDenom(), 10_000_000), nil)
	require.NoError(t, err)
	e.valAddr, err = sdk.ValAddressFromBech32(val.GetOperator())
	require.NoError(t, err)
	_, err = a.StakingKeeper.Delegate(before, e.wallet, sdkmath.NewInt(5_000_000), stakingtypes.Unbonded, val, true)
	require.NoError(t, err)
	return e
}

func (e lockAppEnv) run(t *testing.T, ctx sdk.Context, payer sdk.AccAddress, msgs []sdk.Msg, keys ...*secp256k1.PrivKey) error {
	t.Helper()
	tx := signedLockTestTxMsgs(t, e.a, ctx, payer, msgs, keys...)
	execCtx, err := e.a.AnteHandler()(ctx, tx, false)
	if err != nil {
		return err
	}
	for _, msg := range tx.GetMsgs() {
		handler := e.a.MsgServiceRouter().Handler(msg)
		require.NotNil(t, handler, sdk.MsgTypeURL(msg))
		if _, err := handler(execCtx, msg); err != nil {
			return err
		}
	}
	return nil
}

func TestRecoveryLockAppSignedDeepAuthz(t *testing.T) {
	wrap := func(grantee sdk.AccAddress, msg sdk.Msg) sdk.Msg {
		// Keep the seven nested wrappers independent of the policy limit.
		for i := 0; i < 7; i++ {
			exec := authz.NewMsgExec(grantee, []sdk.Msg{msg})
			msg = &exec
		}
		return msg
	}
	for _, tc := range []struct {
		name    string
		enabled bool
		height  int64
	}{
		{"upstream", false, lockHeight},
		{"inactive", true, lockHeight - 1},
		{"active", true, lockHeight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newLockAppEnvWithLock(t, tc.enabled)
			ctx := e.ctx.WithBlockHeight(tc.height)
			msg := wrap(e.other, &banktypes.MsgSend{
				FromAddress: e.other.String(), ToAddress: e.recovery.String(), Amount: lockCoins(2),
			})
			require.NoError(t, e.run(t, ctx, e.other, []sdk.Msg{msg}, e.otherKey))
			require.Equal(t, lockCoins(2)[0], e.a.BankKeeper.GetBalance(ctx, e.recovery, lockDenom()))
			require.Equal(t, lockCoins(100_000_000 - lockFee - 2)[0],
				e.a.BankKeeper.GetBalance(ctx, e.other, lockDenom()))
		})
	}

	t.Run("listed leaf", func(t *testing.T) {
		e := newLockAppEnv(t)
		before := e.ctx.WithBlockHeight(lockHeight - 1)
		send := &banktypes.MsgSend{
			FromAddress: e.wallet.String(), ToAddress: e.recovery.String(), Amount: lockCoins(2),
		}
		expiry := before.BlockTime().Add(time.Hour)
		require.NoError(t, e.a.AuthzKeeper.SaveGrant(before, e.other, e.wallet,
			authz.NewGenericAuthorization(sdk.MsgTypeURL(send)), &expiry))
		msgs := []sdk.Msg{wrap(e.other, send)}
		inactive, _ := before.CacheContext()
		require.NoError(t, e.run(t, inactive, e.payer, msgs, e.otherKey, e.payerKey))
		err := e.run(t, e.ctx, e.payer, msgs, e.otherKey, e.payerKey)
		require.ErrorIs(t, err, recoverylock.ErrLocked)
		require.ErrorContains(t, err, "message nesting deeper")
		require.True(t, e.a.BankKeeper.GetBalance(e.ctx, e.recovery, lockDenom()).IsZero())
	})
}

func TestRecoveryLockAppWiring(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	bank := a.BankKeeper
	denom := lockDenom()
	send := func(to sdk.AccAddress) *banktypes.MsgSend {
		return &banktypes.MsgSend{FromAddress: e.wallet.String(), ToAddress: to.String(), Amount: lockCoins(2)}
	}

	payerStart := bank.GetBalance(ctx, e.payer, denom)
	walletStart := bank.GetBalance(ctx, e.wallet, denom)
	ante := a.AnteHandler()
	_, err := ante(ctx, signedLockTestTx(t, a, ctx, e.payer, send(e.other), e.walletKey, e.payerKey), false)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	require.Equal(t, payerStart, bank.GetBalance(ctx, e.payer, denom))
	cache, _ := ctx.CacheContext()
	_, err = ante(cache, signedLockTestTx(t, a, ctx, e.payer, send(e.recovery), e.walletKey), false)
	require.Error(t, err, "the registered payer must sign")
	// the wallet paying its own fee is refused before any fee is deducted
	_, err = ante(ctx, signedLockTestTx(t, a, ctx, e.wallet, send(e.recovery), e.walletKey), false)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	require.Equal(t, walletStart, bank.GetBalance(ctx, e.wallet, denom))

	require.NoError(t, e.run(t, ctx, e.payer, []sdk.Msg{send(e.recovery)}, e.walletKey, e.payerKey))
	require.Equal(t, payerStart.Sub(lockCoins(lockFee)[0]), bank.GetBalance(ctx, e.payer, denom))
	require.Equal(t, walletStart.Sub(lockCoins(2)[0]), bank.GetBalance(ctx, e.wallet, denom))
	require.Equal(t, lockCoins(2)[0], bank.GetBalance(ctx, e.recovery, denom))

	// The coin rule on both bank keepers: the wbank wrapper and module
	// account debits refuse any destination but the recovery address.
	walletNow := bank.GetBalance(ctx, e.wallet, denom)
	require.ErrorIs(t, bank.SendCoins(ctx, e.wallet, e.other, lockCoins(1)), recoverylock.ErrLocked)
	require.ErrorIs(t, bank.SendCoinsFromAccountToModule(ctx, e.wallet, authtypes.FeeCollectorName, lockCoins(1)), recoverylock.ErrLocked)
	require.ErrorIs(t, bank.InputOutputCoins(ctx, banktypes.Input{Address: e.wallet.String(), Coins: lockCoins(1)},
		[]banktypes.Output{{Address: e.other.String(), Coins: lockCoins(1)}}), recoverylock.ErrLocked)
	require.ErrorIs(t, bank.BaseKeeper.SendCoins(ctx, e.wallet, e.other, lockCoins(1)), recoverylock.ErrLocked)
	require.NoError(t, bank.SendCoins(ctx, e.wallet, e.recovery, lockCoins(1)))
	require.NoError(t, bank.SendCoins(ctx, e.other, e.wallet, lockCoins(1)), "receiving is unaffected")
	require.Equal(t, walletNow, bank.GetBalance(ctx, e.wallet, denom))
	before := ctx.WithBlockHeight(lockHeight - 1)
	require.NoError(t, bank.SendCoins(before, e.wallet, e.other, lockCoins(1)), "inactive before the activation height")

	// authz dispatch as the wallet is refused even for an allowed message
	expiry := ctx.BlockTime().Add(time.Hour)
	msg := send(e.recovery)
	require.NoError(t, a.AuthzKeeper.SaveGrant(ctx, e.other, e.wallet, authz.NewGenericAuthorization(sdk.MsgTypeURL(msg)), &expiry))
	_, err = a.AuthzKeeper.DispatchActions(ctx, e.other, []sdk.Msg{msg})
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	exec := authz.NewMsgExec(e.other, []sdk.Msg{msg})
	require.ErrorIs(t, e.run(t, ctx, e.other, []sdk.Msg{&exec}, e.otherKey), recoverylock.ErrLocked)

	for _, absent := range []string{
		"/cosmos.staking.v1beta1.MsgTokenizeShares",
		"/cosmos.staking.v1beta1.MsgRedeemTokensForShares",
		"/cosmos.staking.v1beta1.MsgDisableTokenizeShares",
	} {
		require.Nil(t, a.MsgServiceRouter().HandlerByTypeURL(absent))
	}
}

func TestRecoveryLockAppAuthzRevoke(t *testing.T) {
	e := newLockAppEnv(t)
	before := e.ctx.WithBlockHeight(lockHeight - 1)
	expiry := before.BlockTime().Add(time.Hour)
	authorization, err := stakingtypes.NewStakeAuthorization(
		[]sdk.ValAddress{e.valAddr}, nil, stakingtypes.AuthorizationType_AUTHORIZATION_TYPE_DELEGATE, nil,
	)
	require.NoError(t, err)
	require.NoError(t, e.a.AuthzKeeper.SaveGrant(before, e.other, e.wallet, authorization, &expiry))

	revoke := &authz.MsgRevoke{
		Granter: e.wallet.String(), Grantee: e.other.String(), MsgTypeUrl: authorization.MsgTypeURL(),
	}
	msgs := []sdk.Msg{revoke}
	require.ErrorIs(t, e.run(t, e.ctx, e.wallet, msgs, e.walletKey), recoverylock.ErrLocked)
	cache, _ := e.ctx.CacheContext()
	require.Error(t, e.run(t, cache, e.payer, msgs, e.walletKey))

	require.NoError(t, e.a.AuthzKeeper.SaveGrant(before, e.other, e.wallet,
		authz.NewGenericAuthorization(sdk.MsgTypeURL(revoke)), &expiry))
	nested := authz.NewMsgExec(e.other, msgs)
	nestedTx := signedLockTestTx(t, e.a, e.ctx, e.payer, &nested, e.otherKey, e.payerKey)
	_, err = e.a.AnteHandler()(e.ctx, nestedTx, false)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	_, err = e.a.AuthzKeeper.DispatchActions(e.ctx, e.other, msgs)
	require.ErrorIs(t, err, recoverylock.ErrLocked)

	stored, _ := e.a.AuthzKeeper.GetAuthorization(e.ctx, e.other, e.wallet, revoke.MsgTypeUrl)
	require.Equal(t, authorization, stored)
	require.NoError(t, e.run(t, e.ctx, e.payer, msgs, e.walletKey, e.payerKey))
	stored, _ = e.a.AuthzKeeper.GetAuthorization(e.ctx, e.other, e.wallet, revoke.MsgTypeUrl)
	require.Nil(t, stored)
}

func TestRecoveryLockAppMultiSend(t *testing.T) {
	e := newLockAppEnv(t)
	bank := e.a.BankKeeper
	walletStart := bank.GetBalance(e.ctx, e.wallet, lockDenom())
	payerStart := bank.GetBalance(e.ctx, e.payer, lockDenom())
	recoveryStart := bank.GetBalance(e.ctx, e.recovery, lockDenom())
	otherStart := bank.GetBalance(e.ctx, e.other, lockDenom())

	send := &banktypes.MsgMultiSend{
		Inputs: []banktypes.Input{{Address: e.wallet.String(), Coins: lockCoins(5)}},
		Outputs: []banktypes.Output{
			{Address: e.recovery.String(), Coins: lockCoins(2)},
			{Address: e.other.String(), Coins: lockCoins(3)},
		},
	}
	err := e.run(t, e.ctx, e.payer, []sdk.Msg{send}, e.walletKey, e.payerKey)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	require.Equal(t, walletStart, bank.GetBalance(e.ctx, e.wallet, lockDenom()))
	require.Equal(t, payerStart, bank.GetBalance(e.ctx, e.payer, lockDenom()))
	require.Equal(t, recoveryStart, bank.GetBalance(e.ctx, e.recovery, lockDenom()))
	require.Equal(t, otherStart, bank.GetBalance(e.ctx, e.other, lockDenom()))

	send.Outputs[1].Address = e.recovery.String()
	require.NoError(t, e.run(t, e.ctx, e.payer, []sdk.Msg{send}, e.walletKey, e.payerKey))
	require.Equal(t, walletStart.Sub(lockCoins(5)[0]), bank.GetBalance(e.ctx, e.wallet, lockDenom()))
	require.Equal(t, payerStart.Sub(lockCoins(lockFee)[0]), bank.GetBalance(e.ctx, e.payer, lockDenom()))
	require.Equal(t, recoveryStart.Add(lockCoins(5)[0]), bank.GetBalance(e.ctx, e.recovery, lockDenom()))
	require.Equal(t, otherStart, bank.GetBalance(e.ctx, e.other, lockDenom()))
}

func TestRecoveryLockAppStaking(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	valoper := e.valAddr.String()

	// redelegation and new delegations are refused; undelegation needs a safe
	// withdraw address
	require.ErrorIs(t, e.run(t, ctx, e.payer, []sdk.Msg{&stakingtypes.MsgDelegate{
		DelegatorAddress: e.wallet.String(), ValidatorAddress: valoper, Amount: lockCoins(1)[0],
	}}, e.walletKey, e.payerKey), recoverylock.ErrLocked)
	require.NoError(t, a.DistrKeeper.SetWithdrawAddr(ctx.WithBlockHeight(lockHeight-1), e.wallet, e.other))
	undelegate := &stakingtypes.MsgUndelegate{DelegatorAddress: e.wallet.String(), ValidatorAddress: valoper, Amount: lockCoins(1_000_000)[0]}
	require.ErrorIs(t, e.run(t, ctx, e.payer, []sdk.Msg{undelegate}, e.walletKey, e.payerKey), recoverylock.ErrLocked)
	setWithdraw := &distrtypes.MsgSetWithdrawAddress{DelegatorAddress: e.wallet.String(), WithdrawAddress: e.recovery.String()}
	// The rule reads the withdraw address in ante, before any message of the
	// transaction runs, so the change must come in an earlier transaction.
	require.ErrorIs(t, e.run(t, ctx, e.payer, []sdk.Msg{setWithdraw, undelegate}, e.walletKey, e.payerKey), recoverylock.ErrLocked)
	require.NoError(t, e.run(t, ctx, e.payer, []sdk.Msg{setWithdraw}, e.walletKey, e.payerKey))
	require.NoError(t, e.run(t, ctx, e.payer, []sdk.Msg{undelegate}, e.walletKey, e.payerKey))
	ubd, err := a.StakingKeeper.GetUnbondingDelegation(ctx, e.wallet, e.valAddr)
	require.NoError(t, err)
	require.Len(t, ubd.Entries, 1)

	// The staking hook resets an unsafe withdraw address before automatic
	// payouts (here a PSE style auto-delegation through the staking keeper).
	require.NoError(t, a.DistrKeeper.SetWithdrawAddr(ctx.WithBlockHeight(lockHeight-1), e.wallet, e.other))
	val, err := a.StakingKeeper.GetValidator(ctx, e.valAddr)
	require.NoError(t, err)
	_, err = a.StakingKeeper.Delegate(ctx, e.wallet, sdkmath.NewInt(1_000), stakingtypes.Unbonded, val, true)
	require.NoError(t, err, "keeper delegation (PSE auto-delegate) is not refused")
	withdraw, err := a.DistrKeeper.GetDelegatorWithdrawAddr(ctx, e.wallet)
	require.NoError(t, err)
	require.Equal(t, e.wallet, withdraw)
}

func TestRecoveryLockAppAssets(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	before := ctx.WithBlockHeight(lockHeight - 1)
	issuer, issuerKey := a.GenAccount(before)
	require.NoError(t, a.FundAccount(before, issuer, lockCoins(100_000_000_000)))
	params, err := a.AssetFTKeeper.GetParams(before)
	require.NoError(t, err)
	_ = params

	issue := func(subunit string, burnRate, commission string, features ...assetfttypes.Feature) string {
		denom, err := a.AssetFTKeeper.Issue(before, assetfttypes.IssueSettings{
			Issuer: issuer, Symbol: strings.ToUpper(subunit), Subunit: subunit, Precision: 6,
			InitialAmount: sdkmath.NewInt(1_000_000), Features: features,
			BurnRate: sdkmath.LegacyMustNewDecFromStr(burnRate), SendCommissionRate: sdkmath.LegacyMustNewDecFromStr(commission),
		})
		require.NoError(t, err)
		require.NoError(t, a.BankKeeper.SendCoins(before, issuer, e.wallet, sdk.NewCoins(sdk.NewInt64Coin(denom, 1000))))
		return denom
	}
	plain := issue("plain", "0", "0", assetfttypes.Feature_freezing, assetfttypes.Feature_clawback)
	burnRated := issue("burnrated", "0.1", "0")
	commissionRated := issue("commissionrated", "0", "0.1")

	sendToken := func(denom string, to sdk.AccAddress, amount int64) *banktypes.MsgSend {
		return &banktypes.MsgSend{FromAddress: e.wallet.String(), ToAddress: to.String(), Amount: sdk.NewCoins(sdk.NewInt64Coin(denom, amount))}
	}
	require.ErrorIs(t, e.run(t, ctx, e.payer, []sdk.Msg{sendToken(plain, e.other, 10)}, e.walletKey, e.payerKey), recoverylock.ErrLocked)
	require.NoError(t, e.run(t, ctx, e.payer, []sdk.Msg{sendToken(plain, e.recovery, 10)}, e.walletKey, e.payerKey))
	require.Equal(t, int64(10), a.BankKeeper.GetBalance(ctx, e.recovery, plain).Amount.Int64())

	// Exercise each side debit independently so the commission refusal
	// cannot mask the burn path.
	for _, denom := range []string{burnRated, commissionRated} {
		err = e.run(t, ctx, e.payer, []sdk.Msg{sendToken(denom, e.recovery, 10)}, e.walletKey, e.payerKey)
		require.ErrorIs(t, err, recoverylock.ErrLocked, denom)
		require.Equal(t, int64(1000), a.BankKeeper.GetBalance(ctx, e.wallet, denom).Amount.Int64())
		require.True(t, a.BankKeeper.GetBalance(ctx, e.recovery, denom).IsZero())
	}

	// Issuer clawback from a listed wallet is a debit to the issuer: refused
	// while the lock is active. Issuer freezing moves nothing and works.
	clawback := &assetfttypes.MsgClawback{Sender: issuer.String(), Account: e.wallet.String(), Coin: sdk.NewInt64Coin(plain, 5)}
	require.ErrorIs(t, e.run(t, ctx, issuer, []sdk.Msg{clawback}, issuerKey), recoverylock.ErrLocked)
	freeze := &assetfttypes.MsgFreeze{Sender: issuer.String(), Account: e.wallet.String(), Coin: sdk.NewInt64Coin(plain, 5)}
	require.NoError(t, e.run(t, ctx, issuer, []sdk.Msg{freeze}, issuerKey))
	require.NoError(t, e.run(t, ctx.WithBlockHeight(lockHeight-1), issuer, []sdk.Msg{clawback}, issuerKey), "clawback works before activation")
}

// TestRecoveryLockAppDEX places an order for the listed wallet before
// activation. After activation a taker cannot fill it (the settlement debit
// from the listed maker is refused), placing new orders is refused, and the
// recovery operator can cancel it, which releases the locked balance.
func TestRecoveryLockAppDEX(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	before := ctx.WithBlockHeight(lockHeight - 1)
	issuer, _ := a.GenAccount(before)
	require.NoError(t, a.FundAccount(before, issuer, lockCoins(100_000_000_000)))
	denom, err := a.AssetFTKeeper.Issue(before, assetfttypes.IssueSettings{
		Issuer: issuer, Symbol: "BASE", Subunit: "base", Precision: 6, InitialAmount: sdkmath.NewInt(10_000_000_000),
	})
	require.NoError(t, err)
	require.NoError(t, a.BankKeeper.SendCoins(before, issuer, e.wallet, sdk.NewCoins(sdk.NewInt64Coin(denom, 1_000_000))))
	require.NoError(t, a.BankKeeper.SendCoins(before, issuer, e.other, sdk.NewCoins(sdk.NewInt64Coin(denom, 1_000_000))))

	dexParams, err := a.DEXKeeper.GetParams(before)
	require.NoError(t, err)
	price := dextypes.MustNewPriceFromString("1e-1")
	order := dextypes.Order{
		Creator: e.wallet.String(), Type: dextypes.ORDER_TYPE_LIMIT, ID: "lock-sell",
		BaseDenom: denom, QuoteDenom: lockDenom(), Price: &price, Quantity: sdkmath.NewInt(1_000_000),
		Side: dextypes.SIDE_SELL, TimeInForce: dextypes.TIME_IN_FORCE_GTC,
	}
	require.NoError(t, a.FundAccount(before, e.wallet, sdk.NewCoins(dexParams.OrderReserve)))
	require.NoError(t, a.DEXKeeper.PlaceOrder(before, order))

	place := &dextypes.MsgPlaceOrder{
		Sender: e.wallet.String(), Type: dextypes.ORDER_TYPE_LIMIT, ID: "lock-sell-2",
		BaseDenom: denom, QuoteDenom: lockDenom(), Price: &price, Quantity: sdkmath.NewInt(10_000),
		Side: dextypes.SIDE_SELL, TimeInForce: dextypes.TIME_IN_FORCE_GTC,
	}
	require.ErrorIs(t, e.run(t, ctx, e.payer, []sdk.Msg{place}, e.walletKey, e.payerKey), recoverylock.ErrLocked)

	walletBase := a.BankKeeper.GetBalance(ctx, e.wallet, denom)
	taker := dextypes.Order{
		Creator: e.other.String(), Type: dextypes.ORDER_TYPE_LIMIT, ID: "taker-buy",
		BaseDenom: denom, QuoteDenom: lockDenom(), Price: &price, Quantity: sdkmath.NewInt(1_000_000),
		Side: dextypes.SIDE_BUY, TimeInForce: dextypes.TIME_IN_FORCE_IOC,
	}
	cache, _ := ctx.CacheContext()
	require.ErrorIs(t, a.DEXKeeper.PlaceOrder(cache, taker), recoverylock.ErrLocked)
	require.Equal(t, walletBase, a.BankKeeper.GetBalance(ctx, e.wallet, denom))

	cancel := &dextypes.MsgCancelOrder{Sender: e.wallet.String(), ID: "lock-sell"}
	require.ErrorIs(t, e.run(t, ctx, e.wallet, []sdk.Msg{cancel}, e.walletKey), recoverylock.ErrLocked)
	require.NoError(t, e.run(t, ctx, e.payer, []sdk.Msg{cancel}, e.walletKey, e.payerKey))
	require.True(t, a.AssetFTKeeper.GetDEXLockedBalance(ctx, e.wallet, denom).IsZero())

	order.ID = "lock-expiry"
	order.GoodTil = &dextypes.GoodTil{GoodTilBlockHeight: lockHeight + 1}
	require.NoError(t, a.DEXKeeper.PlaceOrder(before, order))
	require.Equal(t, walletBase, a.AssetFTKeeper.GetDEXLockedBalance(ctx, e.wallet, denom))
	require.NoError(t, a.DelayKeeper.ExecuteAllItems(ctx.WithBlockHeight(lockHeight+2)))
	_, err = a.DEXKeeper.GetOrderByAddressAndID(ctx, e.wallet, order.ID)
	require.ErrorIs(t, err, dextypes.ErrRecordNotFound)
	require.True(t, a.AssetFTKeeper.GetDEXLockedBalance(ctx, e.wallet, denom).IsZero())
	require.Equal(t, walletBase, a.BankKeeper.GetBalance(ctx, e.wallet, denom))
	send := &banktypes.MsgSend{FromAddress: e.wallet.String(), ToAddress: e.recovery.String(), Amount: sdk.NewCoins(walletBase)}
	require.NoError(t, e.run(t, ctx, e.payer, []sdk.Msg{send}, e.walletKey, e.payerKey))
	require.Equal(t, walletBase, a.BankKeeper.GetBalance(ctx, e.recovery, denom))
}

func TestRecoveryLockAppNFT(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	before := ctx.WithBlockHeight(lockHeight - 1)
	issuer, _ := a.GenAccount(before)
	require.NoError(t, a.FundAccount(before, issuer, lockCoins(100_000_000_000)))
	classID, err := a.AssetNFTKeeper.IssueClass(before, assetnfttypes.IssueClassSettings{Issuer: issuer, Symbol: "LOCKNFT"})
	require.NoError(t, err)
	for _, id := range []string{"nft1", "nft2"} {
		require.NoError(t, a.AssetNFTKeeper.Mint(before, assetnfttypes.MintSettings{
			Sender: issuer, Recipient: e.wallet, ClassID: classID, ID: id,
		}))
	}
	toOther := &nft.MsgSend{Sender: e.wallet.String(), Receiver: e.other.String(), ClassId: classID, Id: "nft1"}
	require.ErrorIs(t, e.run(t, ctx, e.payer, []sdk.Msg{toOther}, e.walletKey, e.payerKey), recoverylock.ErrLocked)
	toRecovery := &nft.MsgSend{Sender: e.wallet.String(), Receiver: e.recovery.String(), ClassId: classID, Id: "nft1"}
	require.ErrorIs(t, e.run(t, ctx, e.wallet, []sdk.Msg{toRecovery}, e.walletKey), recoverylock.ErrLocked)
	require.NoError(t, e.run(t, ctx, e.payer, []sdk.Msg{toRecovery}, e.walletKey, e.payerKey))
	require.Equal(t, e.recovery, a.NFTKeeper.GetOwner(ctx, classID, "nft1"))
	// a grantee with an nft SendAuthorization (marketplace style) cannot
	// move the wallet's NFTs while the lock is active
	expiry := ctx.BlockTime().Add(time.Hour)
	other2 := &nft.MsgSend{Sender: e.wallet.String(), Receiver: e.other.String(), ClassId: classID, Id: "nft2"}
	require.NoError(t, a.AuthzKeeper.SaveGrant(ctx, e.other, e.wallet, authz.NewGenericAuthorization(sdk.MsgTypeURL(other2)), &expiry))
	exec := authz.NewMsgExec(e.other, []sdk.Msg{other2})
	require.ErrorIs(t, e.run(t, ctx, e.other, []sdk.Msg{&exec}, e.otherKey), recoverylock.ErrLocked)
	require.Equal(t, e.wallet, a.NFTKeeper.GetOwner(ctx, classID, "nft2"))
}

// TestRecoveryLockAppIBC covers MsgTransfer from the wallet (refused by the
// rules and, below them, by the coin rule on the escrow debit).
func TestRecoveryLockAppIBC(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	walletStart := a.BankKeeper.GetAllBalances(ctx, e.wallet)
	transfer := &transfertypes.MsgTransfer{
		SourcePort: transfertypes.PortID, SourceChannel: "channel-0", Token: lockCoins(10)[0],
		Sender: e.wallet.String(), Receiver: e.recovery.String(), TimeoutTimestamp: uint64(ctx.BlockTime().Add(time.Hour).UnixNano()),
	}
	require.ErrorIs(t, e.run(t, ctx, e.payer, []sdk.Msg{transfer}, e.walletKey, e.payerKey), recoverylock.ErrLocked)
	escrow := transfertypes.GetEscrowAddress(transfertypes.PortID, "channel-0")
	require.ErrorIs(t, a.BankKeeper.SendCoins(ctx, e.wallet, escrow, lockCoins(10)), recoverylock.ErrLocked)
	require.Equal(t, walletStart, a.BankKeeper.GetAllBalances(ctx, e.wallet))
}

func TestRecoveryLockAppIBCRefunds(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	const port, channel = transfertypes.PortID, "channel-0"
	coin := lockCoins(10)[0]
	escrow := transfertypes.GetEscrowAddress(port, channel)
	before := ctx.WithBlockHeight(lockHeight - 1)
	start := a.BankKeeper.GetBalance(ctx, e.wallet, coin.Denom)
	packet := transfertypes.NewInternalTransferRepresentation(
		transfertypes.Token{Denom: transfertypes.NewDenom(coin.Denom), Amount: coin.Amount.String()},
		e.wallet.String(), e.other.String(), "",
	)

	require.NoError(t, a.TransferKeeper.EscrowCoin(before, e.wallet, escrow, coin))
	require.NoError(t, a.TransferKeeper.OnTimeoutPacket(ctx, port, channel, packet))
	require.Equal(t, start, a.BankKeeper.GetBalance(ctx, e.wallet, coin.Denom))
	require.True(t, a.TransferKeeper.GetTotalEscrowForDenom(ctx, coin.Denom).IsZero())

	require.NoError(t, a.TransferKeeper.EscrowCoin(before, e.wallet, escrow, coin))
	ack := channelv1.NewErrorAcknowledgement(fmt.Errorf("counterparty rejected transfer"))
	require.NoError(t, a.TransferKeeper.OnAcknowledgementPacket(ctx, port, channel, packet, ack))
	require.Equal(t, start, a.BankKeeper.GetBalance(ctx, e.wallet, coin.Denom))
	require.True(t, a.TransferKeeper.GetTotalEscrowForDenom(ctx, coin.Denom).IsZero())

	packet.Token.Denom = transfertypes.NewDenom("uatom", transfertypes.NewHop(port, channel))
	voucher := packet.Token.Denom.IBCDenom()
	require.NoError(t, a.TransferKeeper.OnTimeoutPacket(ctx, port, channel, packet))
	require.Equal(t, coin.Amount, a.BankKeeper.GetBalance(ctx, e.wallet, voucher).Amount)
	require.NoError(t, a.TransferKeeper.OnAcknowledgementPacket(ctx, port, channel, packet, ack))
	require.Equal(t, coin.Amount.MulRaw(2), a.BankKeeper.GetBalance(ctx, e.wallet, voucher).Amount)
}

// The synthetic account mapping exercises the host router with a listed signer.
func TestRecoveryLockAppICAHostDispatch(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	port, err := icatypes.NewControllerPortID(e.other.String())
	require.NoError(t, err)
	const channel, connection = "channel-0", "connection-0"
	metadata := icatypes.NewMetadata(icatypes.Version, connection, connection, e.wallet.String(),
		icatypes.EncodingProtobuf, icatypes.TxTypeSDKMultiMsg)
	a.IBCKeeper.ChannelKeeper.SetChannel(ctx, icatypes.HostPortID, channel, channelv1.NewChannel(
		channelv1.OPEN, channelv1.ORDERED, channelv1.NewCounterparty(port, channel),
		[]string{connection}, string(icatypes.ModuleCdc.MustMarshalJSON(&metadata)),
	))
	a.ICAHostKeeper.SetInterchainAccountAddress(ctx, connection, port, e.wallet.String())
	// This destination would pass the bank restriction, so rejection here
	// depends on the ICA host's message router.
	send := &banktypes.MsgSend{FromAddress: e.wallet.String(), ToAddress: e.recovery.String(), Amount: lockCoins(10)}
	data, err := icatypes.SerializeCosmosTx(a.AppCodec(), []proto.Message{send}, icatypes.EncodingProtobuf)
	require.NoError(t, err)
	packetData := icatypes.InterchainAccountPacketData{Type: icatypes.EXECUTE_TX, Data: data}
	packet := channelv1.Packet{
		SourcePort: port, SourceChannel: channel, DestinationPort: icatypes.HostPortID,
		DestinationChannel: channel, Data: packetData.GetBytes(),
	}
	_, err = a.ICAHostKeeper.OnRecvPacket(ctx, packet)
	require.ErrorIs(t, err, recoverylock.ErrLocked)
	require.True(t, a.BankKeeper.GetBalance(ctx, e.recovery, lockDenom()).IsZero())
	_, err = a.ICAHostKeeper.OnRecvPacket(ctx.WithBlockHeight(lockHeight-1), packet)
	require.NoError(t, err)
	require.Equal(t, send.Amount[0], a.BankKeeper.GetBalance(ctx, e.recovery, lockDenom()))
}

func TestRecoveryLockAppModuleCredits(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	before := ctx.WithBlockHeight(lockHeight - 1)
	bank := a.BankKeeper
	coins := lockCoins(20)
	start := bank.GetBalance(ctx, e.wallet, lockDenom())
	require.NoError(t, bank.SendCoinsFromAccountToModule(before, e.wallet, govtypes.ModuleName, coins))
	require.NoError(t, a.GovKeeper.SetDeposit(before, govv1.Deposit{
		ProposalId: 1, Depositor: e.wallet.String(), Amount: coins,
	}))
	require.NoError(t, a.GovKeeper.RefundAndDeleteDeposits(ctx, 1))
	require.Equal(t, start, bank.GetBalance(ctx, e.wallet, lockDenom()))

	require.NoError(t, bank.SendCoinsFromAccountToModule(before, e.wallet, govtypes.ModuleName, coins))
	require.NoError(t, a.GovKeeper.SetDeposit(before, govv1.Deposit{
		ProposalId: 2, Depositor: e.wallet.String(), Amount: coins,
	}))
	supply := bank.GetSupply(ctx, lockDenom())
	require.NoError(t, a.GovKeeper.DeleteAndBurnDeposits(ctx, 2))
	require.Equal(t, supply.Sub(coins[0]), bank.GetSupply(ctx, lockDenom()))
	require.Equal(t, start.Sub(coins[0]), bank.GetBalance(ctx, e.wallet, lockDenom()))

	_, _, err := a.StakingKeeper.Undelegate(before, e.wallet, e.valAddr, sdkmath.LegacyNewDec(100))
	require.NoError(t, err)
	unbonding, err := a.StakingKeeper.GetUnbondingDelegation(ctx, e.wallet, e.valAddr)
	require.NoError(t, err)
	require.Len(t, unbonding.Entries, 1)
	entry := unbonding.Entries[0]
	start = bank.GetBalance(ctx, e.wallet, lockDenom())
	mature := ctx.WithBlockTime(entry.CompletionTime)
	credited, err := a.StakingKeeper.CompleteUnbonding(mature, e.wallet, e.valAddr)
	require.NoError(t, err)
	require.Equal(t, entry.Balance, credited.AmountOf(lockDenom()))
	require.Equal(t, start.Amount.Add(entry.Balance), bank.GetBalance(ctx, e.wallet, lockDenom()).Amount)
}

func TestRecoveryLockAppSlashRedelegation(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	before := ctx.WithBlockHeight(lockHeight - 1)
	require.NoError(t, a.DistrKeeper.SetWithdrawAddr(before, e.wallet, e.other))
	val, err := a.StakingKeeper.GetValidator(ctx, e.valAddr)
	require.NoError(t, err)
	source, _ := a.GenAccount(before)
	require.NoError(t, a.FundAccount(before, source, lockCoins(10_000_000)))
	src, err := a.AddValidator(before, source, lockCoins(10_000_000)[0], nil)
	require.NoError(t, err)

	reward := lockCoins(1_000_000)
	bank := a.BankKeeper
	require.NoError(t, bank.MintCoins(before, minttypes.ModuleName, reward))
	require.NoError(t, bank.SendCoinsFromModuleToModule(before, minttypes.ModuleName, distrtypes.ModuleName, reward))
	require.NoError(t, a.DistrKeeper.AllocateTokensToValidator(ctx, val, sdk.NewDecCoinsFromCoins(reward...)))
	walletStart := bank.GetBalance(ctx, e.wallet, lockDenom())
	otherStart := bank.GetBalance(ctx, e.other, lockDenom())

	// Inject a pending redelegation record for shares held at the destination.
	red := stakingtypes.Redelegation{
		DelegatorAddress: e.wallet.String(), ValidatorSrcAddress: src.GetOperator(), ValidatorDstAddress: val.GetOperator(),
		Entries: []stakingtypes.RedelegationEntry{{
			CreationHeight: lockHeight - 1, CompletionTime: ctx.BlockTime().Add(time.Hour),
			InitialBalance: sdkmath.NewInt(1000), SharesDst: sdkmath.LegacyNewDec(1000),
		}},
	}
	slashed, err := a.StakingKeeper.SlashRedelegation(ctx, src, red, lockHeight-1, sdkmath.LegacyNewDecWithPrec(1, 1))
	require.NoError(t, err)
	require.Equal(t, sdkmath.NewInt(100), slashed)
	require.True(t, bank.GetBalance(ctx, e.wallet, lockDenom()).IsGT(walletStart))
	require.Equal(t, otherStart, bank.GetBalance(ctx, e.other, lockDenom()))
}

func TestRecoveryLockAppValidatorRemovalCommission(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	before := ctx.WithBlockHeight(lockHeight - 1)
	validator, err := a.AddValidator(before, e.wallet, lockCoins(10_000_000)[0], nil)
	require.NoError(t, err)
	// Model a validator whose delegations have already been removed.
	validator.Status = stakingtypes.Unbonded
	validator.Tokens = sdkmath.ZeroInt()
	validator.DelegatorShares = sdkmath.LegacyZeroDec()
	require.NoError(t, a.StakingKeeper.SetValidator(before, validator))
	require.NoError(t, a.DistrKeeper.SetWithdrawAddr(before, e.wallet, e.other))

	commission := lockCoins(100)
	bank := a.BankKeeper
	require.NoError(t, bank.MintCoins(before, minttypes.ModuleName, commission))
	require.NoError(t, bank.SendCoinsFromModuleToModule(before, minttypes.ModuleName, distrtypes.ModuleName, commission))
	operator := sdk.ValAddress(e.wallet)
	accrued := sdk.NewDecCoinsFromCoins(commission...)
	require.NoError(t, a.DistrKeeper.SetValidatorAccumulatedCommission(before, operator,
		distrtypes.ValidatorAccumulatedCommission{Commission: accrued}))
	require.NoError(t, a.DistrKeeper.SetValidatorOutstandingRewards(before, operator,
		distrtypes.ValidatorOutstandingRewards{Rewards: accrued}))

	walletStart := bank.GetBalance(ctx, e.wallet, lockDenom())
	otherStart := bank.GetBalance(ctx, e.other, lockDenom())
	require.NoError(t, a.StakingKeeper.RemoveValidator(ctx, operator))
	require.Equal(t, walletStart.Add(commission[0]), bank.GetBalance(ctx, e.wallet, lockDenom()))
	require.Equal(t, otherStart, bank.GetBalance(ctx, e.other, lockDenom()))
}

func TestRecoveryLockAppPSEDistribution(t *testing.T) {
	e := newLockAppEnv(t)
	a, ctx := e.a, e.ctx
	before := ctx.WithBlockHeight(lockHeight - 1)
	require.NoError(t, a.DistrKeeper.SetWithdrawAddr(before, e.wallet, e.other))

	keeper := a.PSEKeeper
	reward := lockCoins(100)[0]
	intermediary := a.AccountKeeper.GetModuleAddress(psetypes.ClearingAccountCommunityIntermediary)
	require.NoError(t, a.BankKeeper.MintCoins(before, minttypes.ModuleName, sdk.NewCoins(reward)))
	require.NoError(t, a.BankKeeper.SendCoinsFromModuleToModule(before, minttypes.ModuleName,
		psetypes.ClearingAccountCommunityIntermediary, sdk.NewCoins(reward)))
	const distributionID = uint64(1)
	score := sdkmath.NewInt(1)
	require.NoError(t, keeper.TotalScore.Set(ctx, distributionID, score))
	require.NoError(t, keeper.AccountScoreSnapshot.Set(ctx, collections.Join(distributionID, e.wallet), score))
	distribution := psetypes.ScheduledDistribution{
		ID: distributionID, Timestamp: uint64(ctx.BlockTime().Unix()),
		Allocations: []psetypes.ClearingAccountAllocation{{
			ClearingAccount: psetypes.ClearingAccountCommunity, Amount: reward.Amount,
		}},
	}

	delegation, err := a.StakingKeeper.GetDelegation(ctx, e.wallet, e.valAddr)
	require.NoError(t, err)
	bank := a.BankKeeper
	distributionReward := lockCoins(1_000_000)
	require.NoError(t, bank.MintCoins(before, minttypes.ModuleName, distributionReward))
	require.NoError(t, bank.SendCoinsFromModuleToModule(before, minttypes.ModuleName, distrtypes.ModuleName, distributionReward))
	val, err := a.StakingKeeper.GetValidator(ctx, e.valAddr)
	require.NoError(t, err)
	require.NoError(t, a.DistrKeeper.AllocateTokensToValidator(ctx, val, sdk.NewDecCoinsFromCoins(distributionReward...)))
	walletStart := bank.GetBalance(ctx, e.wallet, reward.Denom)
	otherStart := bank.GetBalance(ctx, e.other, reward.Denom)
	_, err = keeper.ProcessOngoingTokenDistribution(ctx, distribution, reward.Denom)
	require.NoError(t, err)
	current, err := a.StakingKeeper.GetDelegation(ctx, e.wallet, e.valAddr)
	require.NoError(t, err)
	require.Equal(t, delegation.Shares.Add(sdkmath.LegacyNewDecFromInt(reward.Amount)), current.Shares)
	require.True(t, bank.GetBalance(ctx, intermediary, reward.Denom).IsZero())
	require.True(t, bank.GetBalance(ctx, e.wallet, reward.Denom).IsGT(walletStart),
		"automatic distribution rewards go back to the listed wallet")
	require.Equal(t, otherStart, bank.GetBalance(ctx, e.other, reward.Denom))
	withdraw, err := a.DistrKeeper.GetDelegatorWithdrawAddr(ctx, e.wallet)
	require.NoError(t, err)
	require.Equal(t, e.wallet, withdraw)
}

const lockFee = 1_000_000

func signedLockTestTx(t testing.TB, a *simapp.App, ctx sdk.Context, payer sdk.AccAddress, msg sdk.Msg, keys ...*secp256k1.PrivKey) sdk.Tx {
	t.Helper()
	return signedLockTestTxMsgs(t, a, ctx, payer, []sdk.Msg{msg}, keys...)
}

func signedLockTestTxMsgs(t testing.TB, a *simapp.App, ctx sdk.Context, payer sdk.AccAddress, msgs []sdk.Msg, keys ...*secp256k1.PrivKey) sdk.Tx {
	t.Helper()
	txConfig := a.TxConfig()
	acct := a.AccountKeeper
	b := txConfig.NewTxBuilder()
	require.NoError(t, b.SetMsgs(msgs...))
	if payer != nil {
		b.SetFeePayer(payer)
	}
	b.SetFeeAmount(lockCoins(lockFee))
	b.SetGasLimit(1_000_000)
	mode := signing.SignMode_SIGN_MODE_DIRECT
	sigs := make([]signing.SignatureV2, len(keys))
	for i, key := range keys {
		addr := sdk.AccAddress(key.PubKey().Address())
		sigs[i] = signing.SignatureV2{PubKey: key.PubKey(), Data: &signing.SingleSignatureData{SignMode: mode},
			Sequence: acct.GetAccount(ctx, addr).GetSequence()}
	}
	require.NoError(t, b.SetSignatures(sigs...))
	for i, key := range keys {
		addr := sdk.AccAddress(key.PubKey().Address())
		account := acct.GetAccount(ctx, addr)
		var err error
		sigs[i], err = clienttx.SignWithPrivKey(ctx, mode, authsigning.SignerData{
			Address: addr.String(), ChainID: ctx.ChainID(), AccountNumber: account.GetAccountNumber(),
			Sequence: account.GetSequence(), PubKey: key.PubKey(),
		}, b, key, txConfig, account.GetSequence())
		require.NoError(t, err)
	}
	require.NoError(t, b.SetSignatures(sigs...))
	encoded, err := txConfig.TxEncoder()(b.GetTx())
	require.NoError(t, err)
	decoded, err := txConfig.TxDecoder()(encoded)
	require.NoError(t, err)
	return decoded
}

// benchLockConfig returns a test configuration with n generated listed
// addresses, each with its own generated recovery address, and the eight fee
// payers of recoverylock/testdata.
func benchLockConfig(tb testing.TB, n int) (listed, params []byte) {
	tb.Helper()
	gen := func(prefix string, i int) string {
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", prefix, i)))
		return sdk.AccAddress(digest[:20]).String()
	}
	var list strings.Builder
	recoveries := make(map[string]string, n)
	for i := 0; i < n; i++ {
		wallet := gen("recoverylock-bench-listed", i)
		list.WriteString(wallet + "\n")
		recoveries[wallet] = gen("recoverylock-bench-recovery", i)
	}
	listed = []byte(list.String())
	payers := make([]string, 8)
	for i := range payers {
		payers[i] = placeholderAddress(fmt.Sprintf("recoverylock-test-fee-payer-%d", i)).String()
	}
	params, err := json.Marshal(map[string]any{
		"enabled": true, "chainId": string(app.ChosenNetwork.ChainID()), "recoveryAddresses": recoveries, "feePayers": payers,
		"activationHeight": lockHeight, "expiryTime": 4102444800,
		"listSha256": fmt.Sprintf("%x", sha256.Sum256(listed)), "listCount": n, "testValues": true,
	})
	require.NoError(tb, err)
	return listed, params
}

// BenchmarkRecoveryLockConfig10k measures the startup load and validation of
// a 10,000 address list.
func BenchmarkRecoveryLockConfig10k(b *testing.B) {
	a := newLockApp(b)
	ac := a.InterfaceRegistry().SigningContext().AddressCodec()
	b.Cleanup(recoverylock.UseTestConfig(benchLockConfig(b, 10_000)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if recoverylock.MustConfig(string(app.ChosenNetwork.ChainID()), ac, nil, nil) == nil {
			b.Fatal("config disabled")
		}
	}
}

// BenchmarkRecoveryLockAnte measures the full ante handler for an ordinary
// bank send between unlisted accounts, with the committed (disabled)
// configuration and with an active 10,000 address list.
func BenchmarkRecoveryLockAnte(b *testing.B) {
	for _, bc := range []struct {
		name   string
		listed int
	}{{"disabled", 0}, {"active-10k", 10_000}} {
		b.Run(bc.name, func(b *testing.B) {
			if bc.listed > 0 {
				b.Cleanup(recoverylock.UseTestConfig(benchLockConfig(b, bc.listed)))
			}
			a := newLockApp(b)
			require.Equal(b, bc.listed > 0, a.RecoveryLock != nil)
			ctx := a.NewContextLegacy(false, cmtproto.Header{Height: lockHeight, Time: lockTime, ChainID: a.ChainID()})
			senderKey := placeholderKey("recoverylock-bench-sender")
			sender := sdk.AccAddress(senderKey.PubKey().Address())
			a.AccountKeeper.SetAccount(ctx, a.AccountKeeper.NewAccountWithAddress(ctx, sender))
			require.NoError(b, a.FundAccount(ctx, sender, lockCoins(1_000_000_000)))
			msg := &banktypes.MsgSend{FromAddress: sender.String(), ToAddress: placeholderAddress("recoverylock-unlisted").String(), Amount: lockCoins(1)}
			tx := signedLockTestTx(b, a, ctx, nil, msg, senderKey)
			ante := a.AnteHandler()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cache, _ := ctx.CacheContext()
				if _, err := ante(cache, tx, false); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
