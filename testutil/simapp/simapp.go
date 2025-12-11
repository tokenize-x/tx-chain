// Package simapp contains utils to bootstrap the chain.
package simapp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	pruningtypes "cosmossdk.io/store/pruning/types"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	abci "github.com/cometbft/cometbft/abci/types"
	cmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/proto/tendermint/crypto"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/server"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v6/app"
	"github.com/tokenize-x/tx-chain/v6/pkg/config"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
)

const appHash = "sim-app-hash"

// IgnoredModulesForExport defines module names that should be ignored in entire process.
var IgnoredModulesForExport = map[string]struct{}{
	upgradetypes.ModuleName: {}, // Upgrade exports empty genesis.
	// TODO (v7): fix Error calling the VM: Cache error: Error opening Wasm file for reading
	"wasm": {},
}

// Settings for the simapp initialization.
type Settings struct {
	db        dbm.DB
	logger    log.Logger
	startTime time.Time
}

var sdkConfigOnce = &sync.Once{}

// Option represents simapp customisations.
type Option func(settings Settings) Settings

// WithCustomDB returns the simapp Option to run with different DB.
func WithCustomDB(db dbm.DB) Option {
	return func(s Settings) Settings {
		s.db = db
		return s
	}
}

// WithCustomLogger returns the simapp Option to run with different logger.
func WithCustomLogger(logger log.Logger) Option {
	return func(s Settings) Settings {
		s.logger = logger
		return s
	}
}

// WithStartTime returns the simapp Option to run with different start time.
func WithStartTime(startTime time.Time) Option {
	return func(s Settings) Settings {
		s.startTime = startTime
		return s
	}
}

// App is a simulation app wrapper.
type App struct {
	app.App
}

// New creates application instance with in-memory database and disabled logging.
func New(options ...Option) *App {
	settings := Settings{
		db:        dbm.NewMemDB(),
		logger:    log.NewNopLogger(),
		startTime: time.Now(),
	}

	for _, option := range options {
		settings = option(settings)
	}

	sdkConfigOnce.Do(func() {
		network, err := config.NetworkConfigByChainID(constant.ChainIDDev)
		if err != nil {
			panic(err)
		}

		app.ChosenNetwork = network
		network.SetSDKConfig()
	})

	coreApp := app.New(settings.logger, settings.db, nil, true, simtestutil.NewAppOptionsWithFlagHome(tempDir()))
	pubKey, err := cryptocodec.ToCmtPubKeyInterface(ed25519.GenPrivKey().PubKey())
	if err != nil {
		panic(fmt.Sprintf("can't generate validator pub key genesisState: %v", err))
	}
	validator := tmtypes.NewValidator(pubKey, 1)
	valSet := tmtypes.NewValidatorSet([]*tmtypes.Validator{validator})
	senderPrivateKey := secp256k1.GenPrivKey()
	acc := authtypes.NewBaseAccount(senderPrivateKey.PubKey().Address().Bytes(), senderPrivateKey.PubKey(), 0, 0)

	defaultGenesis := coreApp.DefaultGenesis()
	genesisState, err := simtestutil.GenesisStateWithValSet(
		coreApp.AppCodec(),
		defaultGenesis,
		valSet,
		[]authtypes.GenesisAccount{acc},
	)
	if err != nil {
		panic(fmt.Sprintf("can't generate genesis state with wallet, err: %s", err))
	}

	stateBytes, err := cmjson.MarshalIndent(genesisState, "", " ")
	if err != nil {
		panic(errors.Errorf("can't Marshal genesisState: %s", err))
	}

	_, err = coreApp.InitChain(&abci.RequestInitChain{
		ConsensusParams: simtestutil.DefaultConsensusParams,
		AppStateBytes:   stateBytes,
		Time:            settings.startTime,
	})
	if err != nil {
		panic(errors.Errorf("can't init chain: %s", err))
	}

	simApp := &App{*coreApp}

	return simApp
}

// NewWithGenesis creates application instance with in-memory database and disabled logging,
// using provided genesis bytes.
func NewWithGenesis(
	genesisBytes []byte,
	options ...Option,
) (App, string, map[string]json.RawMessage, *abci.RequestInitChain, *abci.ResponseInitChain) {
	homeDir := tempDir()

	settings := Settings{
		db:     dbm.NewMemDB(),
		logger: log.NewNopLogger(),
	}

	for _, option := range options {
		settings = option(settings)
	}

	initChainReq, appState, err := convertExportedGenesisToInitChain(genesisBytes)
	if err != nil {
		panic(errors.Errorf("can't convert genesis bytes to init chain: %s", err))
	}

	coreApp := app.New(
		settings.logger,
		settings.db,
		nil,
		true,
		simtestutil.NewAppOptionsWithFlagHome(homeDir),
		baseapp.SetChainID(initChainReq.ChainId),
		baseapp.SetPruning(pruningtypes.NewPruningOptions(pruningtypes.PruningNothing)),
	)

	initChainRes, err := coreApp.InitChain(initChainReq)
	if err != nil {
		panic(errors.Errorf("can't init chain: %s", err))
	}

	return App{*coreApp}, homeDir, appState, initChainReq, initChainRes
}

// BeginNextBlock begins new SimApp block and returns the ctx of the new block.
func (s *App) BeginNextBlock() (sdk.Context, sdk.BeginBlock, error) {
	header := tmproto.Header{
		Height:  s.LastBlockHeight() + 1,
		Time:    time.Now(),
		AppHash: []byte(appHash),
	}
	ctx := s.NewContextLegacy(false, header)
	beginBlock, err := s.BeginBlocker(ctx)
	return ctx, beginBlock, err
}

// BeginNextBlockAtTime begins new SimApp block and returns the ctx of the new block with given time.
func (s *App) BeginNextBlockAtTime(blockTime time.Time) (sdk.Context, sdk.BeginBlock, error) {
	header := tmproto.Header{
		Height:  s.LastBlockHeight() + 1,
		Time:    blockTime,
		AppHash: []byte(appHash),
	}
	ctx := s.NewContextLegacy(false, header)
	beginBlock, err := s.BeginBlocker(ctx)
	return ctx, beginBlock, err
}

// BeginNextBlockAtHeight begins new SimApp block and returns the ctx of the new block with given height.
func (s *App) BeginNextBlockAtHeight(height int64) (sdk.Context, sdk.BeginBlock, error) {
	header := tmproto.Header{
		Height:  height,
		Time:    time.Now(),
		AppHash: []byte(appHash),
	}
	ctx := s.NewContextLegacy(false, header)
	beginBlock, err := s.BeginBlocker(ctx)
	return ctx, beginBlock, err
}

// FinalizeBlock ends the current block and commit the state and creates a new block.
func (s *App) FinalizeBlock() error {
	_, err := s.App.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: s.LastBlockHeight() + 1,
		Hash:   s.LastCommitID().Hash,
		Time:   time.Now(),
	})
	return err
}

// FinalizeBlockAtTime ends the current block and commit the state and creates a new block at specified time.
func (s *App) FinalizeBlockAtTime(blockTime time.Time) error {
	_, err := s.App.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: s.LastBlockHeight() + 1,
		Hash:   s.LastCommitID().Hash,
		Time:   blockTime,
	})
	return err
}

// GenAccount creates a new account and registers it in the App.
func (s *App) GenAccount(ctx sdk.Context) (sdk.AccAddress, *secp256k1.PrivKey) {
	privateKey := secp256k1.GenPrivKey()
	accountAddress := sdk.AccAddress(privateKey.PubKey().Address())
	account := s.AccountKeeper.NewAccountWithAddress(ctx, accountAddress)
	s.AccountKeeper.SetAccount(ctx, account)

	return accountAddress, privateKey
}

// FundAccount mints and sends the coins to the provided App account.
func (s *App) FundAccount(ctx sdk.Context, address sdk.AccAddress, balances sdk.Coins) error {
	if err := s.BankKeeper.MintCoins(ctx, minttypes.ModuleName, balances); err != nil {
		return errors.Wrap(err, "can't mint in simapp")
	}

	if err := s.BankKeeper.SendCoinsFromModuleToAccount(ctx, minttypes.ModuleName, address, balances); err != nil {
		return errors.Wrap(err, "can't send funding coins in simapp")
	}

	return nil
}

// AddValidator creates a new validator in the simapp and returns the validator object.
// Commission is optional - if nil, defaults to 10% rate, 20% max rate, 1% max change rate.
// Individual fields within CommissionRates that are zero will also use defaults.
func (s *App) AddValidator(
	ctx sdk.Context,
	operator sdk.AccAddress,
	value sdk.Coin,
	commission *stakingtypes.CommissionRates,
) (stakingtypes.Validator, error) {
	privKey := secp256k1.GenPrivKey()
	pubKey := privKey.PubKey()
	valAddr := sdk.ValAddress(operator)

	pkAny, err := codectypes.NewAnyWithValue(pubKey)
	if err != nil {
		return stakingtypes.Validator{}, err
	}

	// Default commission rates
	commissionRates := stakingtypes.CommissionRates{
		Rate:          sdkmath.LegacyNewDecWithPrec(10, 2), // 10%
		MaxRate:       sdkmath.LegacyNewDecWithPrec(20, 2), // 20%
		MaxChangeRate: sdkmath.LegacyNewDecWithPrec(1, 2),  // 1%
	}
	if commission != nil {
		if !commission.Rate.IsNil() && !commission.Rate.IsZero() {
			commissionRates.Rate = commission.Rate
		}
		if !commission.MaxRate.IsNil() && !commission.MaxRate.IsZero() {
			commissionRates.MaxRate = commission.MaxRate
		}
		if !commission.MaxChangeRate.IsNil() && !commission.MaxChangeRate.IsZero() {
			commissionRates.MaxChangeRate = commission.MaxChangeRate
		}
	}

	msg := &stakingtypes.MsgCreateValidator{
		Description: stakingtypes.Description{
			Moniker: "Validator power",
		},
		Commission:        commissionRates,
		MinSelfDelegation: sdkmath.OneInt(),
		DelegatorAddress:  operator.String(),
		ValidatorAddress:  valAddr.String(),
		Pubkey:            pkAny,
		Value:             value,
	}

	_, err = stakingkeeper.NewMsgServerImpl(s.StakingKeeper).CreateValidator(ctx, msg)
	if err != nil {
		return stakingtypes.Validator{}, err
	}

	return s.StakingKeeper.GetValidator(ctx, valAddr)
}

// SendTx sends the tx to the simApp.
func (s *App) SendTx(
	ctx sdk.Context,
	feeAmt sdk.Coin,
	gas uint64,
	priv cryptotypes.PrivKey,
	messages ...sdk.Msg,
) (sdk.GasInfo, *sdk.Result, error) {
	tx, err := s.GenTx(
		ctx, feeAmt, gas, priv, messages...,
	)
	if err != nil {
		return sdk.GasInfo{}, nil, err
	}

	txCfg := s.TxConfig()
	return s.SimDeliver(txCfg.TxEncoder(), tx)
}

// GenTx generates a tx from messages.
func (s *App) GenTx(
	ctx sdk.Context,
	feeAmt sdk.Coin,
	gas uint64,
	priv cryptotypes.PrivKey,
	messages ...sdk.Msg,
) (sdk.Tx, error) {
	signerAddress := sdk.AccAddress(priv.PubKey().Address())
	account := s.AccountKeeper.GetAccount(ctx, signerAddress)
	if account == nil {
		return nil, errors.Errorf(
			"the account %s doesn't exist, check that it's created or state committed",
			signerAddress,
		)
	}
	accountNum := account.GetAccountNumber()
	accountSeq := account.GetSequence()

	txCfg := s.TxConfig()

	tx, err := simtestutil.GenSignedMockTx(
		rand.New(rand.NewSource(time.Now().UnixNano())),
		txCfg,
		messages,
		sdk.NewCoins(feeAmt),
		gas,
		s.ChainID(),
		[]uint64{accountNum},
		[]uint64{accountSeq},
		priv,
	)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// MintAndSendCoin mints coin to the mint module and sends them to the recipient.
func (s *App) MintAndSendCoin(
	t *testing.T,
	sdkCtx sdk.Context,
	recipient sdk.AccAddress,
	coins sdk.Coins,
) {
	require.NoError(
		t, s.BankKeeper.MintCoins(sdkCtx, minttypes.ModuleName, coins),
	)
	require.NoError(
		t, s.BankKeeper.SendCoinsFromModuleToAccount(sdkCtx, minttypes.ModuleName, recipient, coins),
	)
}

// GetModulesToExport returns the list of modules to export, it filters out ignored modules.
func GetModulesToExport() []string {
	sdkConfigOnce.Do(func() {
		network, err := config.NetworkConfigByChainID(constant.ChainIDDev)
		if err != nil {
			panic(err)
		}

		app.ChosenNetwork = network
		network.SetSDKConfig()
	})

	settings := Settings{
		db:     dbm.NewMemDB(),
		logger: log.NewNopLogger(),
	}

	tmpApp := app.New(settings.logger, settings.db, nil, false, simtestutil.AppOptionsMap{
		flags.FlagHome:            os.TempDir(),
		server.FlagInvCheckPeriod: time.Millisecond * 100,
	})

	// Filter out ignored modules
	var modulesToExport []string
	for _, m := range tmpApp.ModuleManager.ModuleNames() {
		if _, ignored := IgnoredModulesForExport[m]; !ignored {
			modulesToExport = append(modulesToExport, m)
		}
	}

	return modulesToExport
}

// ParseExportedGenesisAndApp parses the exported genesis and application state from a val/full node
// and returns the application instance and the exported genesis as a bytes buffer.
func ParseExportedGenesisAndApp(nodeAppDir, exportedGenesisPath string) (*app.App, bytes.Buffer, error) {
	sdkConfigOnce.Do(func() {
		network, err := config.NetworkConfigByChainID(constant.ChainIDDev)
		if err != nil {
			panic(err)
		}

		app.ChosenNetwork = network
		network.SetSDKConfig()
	})

	settings := Settings{
		logger: log.NewNopLogger(),
	}

	nodeDbDir := filepath.Join(nodeAppDir, "data")
	var err error
	settings.db, err = dbm.NewDB("application", dbm.GoLevelDBBackend, nodeDbDir)
	if err != nil {
		return nil, bytes.Buffer{}, errors.Wrapf(err, "failed to open node DB at %s", nodeDbDir)
	}

	var exportBuf bytes.Buffer
	// Read the exported genesis file
	exportedGenesis, err := os.ReadFile(exportedGenesisPath)
	if err != nil {
		return nil, bytes.Buffer{}, errors.Wrap(err, "failed to read exported genesis file")
	}

	_, err = exportBuf.Write(exportedGenesis)
	if err != nil {
		return nil, bytes.Buffer{}, errors.Wrap(err, "failed to write exported genesis to buffer")
	}

	// this is a temporary app equivalent to the actual running chain exported app
	chainNodeApp := app.New(settings.logger, settings.db, nil, false, simtestutil.AppOptionsMap{
		flags.FlagHome:            nodeAppDir,
		server.FlagInvCheckPeriod: time.Millisecond * 100,
	})

	return chainNodeApp, exportBuf, err
}

func tempDir() string {
	dir, err := os.MkdirTemp("", "txd")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	defer os.RemoveAll(dir) //nolint:errcheck // we don't care

	return dir
}

// CopyContextWithMultiStore returns a sdk.Context with a copied MultiStore.
func CopyContextWithMultiStore(sdkCtx sdk.Context) sdk.Context {
	return sdkCtx.WithMultiStore(sdkCtx.MultiStore().CacheWrap().(storetypes.MultiStore))
}

func convertExportedGenesisToInitChain(jsonBytes []byte) (*abci.RequestInitChain, map[string]json.RawMessage, error) {
	var export struct {
		InitialHeight int64                      `json:"initial_height"` //nolint:tagliatelle
		GenesisTime   string                     `json:"genesis_time"`   //nolint:tagliatelle
		ChainID       string                     `json:"chain_id"`       //nolint:tagliatelle
		AppState      map[string]json.RawMessage `json:"app_state"`      //nolint:tagliatelle
		Consensus     struct {
			Params struct {
				Block struct {
					MaxBytes string `json:"max_bytes"` //nolint:tagliatelle
					MaxGas   string `json:"max_gas"`   //nolint:tagliatelle
				} `json:"block"`
				Evidence struct {
					MaxAgeNumBlocks string `json:"max_age_num_blocks"` //nolint:tagliatelle
					MaxAgeDuration  string `json:"max_age_duration"`   //nolint:tagliatelle
					MaxBytes        string `json:"max_bytes"`          //nolint:tagliatelle
				} `json:"evidence"`
				Validator struct {
					PubKeyTypes []string `json:"pub_key_types"` //nolint:tagliatelle
				} `json:"validator"`
				Version struct {
					App string `json:"app"`
				} `json:"version"`
				ABCI struct {
					VoteExtensionsEnableHeight string `json:"vote_extensions_enable_height"` //nolint:tagliatelle
				} `json:"abci"`
			} `json:"params"`
			Validators []struct {
				Address string `json:"address"`
				PubKey  struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"pub_key"` //nolint:tagliatelle
				Power string `json:"power"`
				Name  string `json:"name"`
			} `json:"validators"`
		} `json:"consensus"`
	}
	if err := json.Unmarshal(jsonBytes, &export); err != nil {
		return nil, nil, err
	}

	// Marshal app_state to bytes
	appStateBytes, err := json.Marshal(export.AppState)
	if err != nil {
		return nil, nil, err
	}

	// Parse genesis_time
	genesisTime, err := time.Parse(time.RFC3339Nano, export.GenesisTime)
	if err != nil {
		return nil, nil, err
	}

	// Build ConsensusParams
	consensusParams := &tmproto.ConsensusParams{
		Block: &tmproto.BlockParams{
			MaxBytes: mustParseInt64(export.Consensus.Params.Block.MaxBytes),
			MaxGas:   mustParseInt64(export.Consensus.Params.Block.MaxGas),
		},
		Evidence: &tmproto.EvidenceParams{
			MaxAgeNumBlocks: mustParseInt64(export.Consensus.Params.Evidence.MaxAgeNumBlocks),
			MaxAgeDuration:  lo.Must(time.ParseDuration(export.Consensus.Params.Evidence.MaxAgeDuration + "ns")),
			MaxBytes:        mustParseInt64(export.Consensus.Params.Evidence.MaxBytes),
		},
		Validator: &tmproto.ValidatorParams{
			PubKeyTypes: export.Consensus.Params.Validator.PubKeyTypes,
		},
		Version: &tmproto.VersionParams{
			App: lo.Must(strconv.ParseUint(export.Consensus.Params.Version.App, 10, 64)),
		},
		Abci: &tmproto.ABCIParams{
			VoteExtensionsEnableHeight: mustParseInt64(export.Consensus.Params.ABCI.VoteExtensionsEnableHeight),
		},
	}

	// Build Validators
	var validators []abci.ValidatorUpdate
	for _, v := range export.Consensus.Validators {
		pubKey, err := base64.StdEncoding.DecodeString(v.PubKey.Value)
		if err != nil {
			return nil, nil, err
		}
		validators = append(validators, abci.ValidatorUpdate{
			PubKey: crypto.PublicKey{
				Sum: &crypto.PublicKey_Ed25519{Ed25519: pubKey},
			},
			Power: mustParseInt64(v.Power),
		})
	}

	return &abci.RequestInitChain{
		Time:            genesisTime,
		ChainId:         export.ChainID,
		ConsensusParams: consensusParams,
		Validators:      validators,
		AppStateBytes:   appStateBytes,
		InitialHeight:   export.InitialHeight,
	}, export.AppState, nil
}

// Helper functions.
func mustParseInt64(s string) int64 {
	return lo.Must(strconv.ParseInt(s, 10, 64))
}
