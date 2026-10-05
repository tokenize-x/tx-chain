package wdistribution

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/module"
	distr "github.com/cosmos/cosmos-sdk/x/distribution"
	distrexported "github.com/cosmos/cosmos-sdk/x/distribution/exported"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"

	"github.com/tokenize-x/tx-chain/v8/x/wdistribution/keeper"
	wdistrtypes "github.com/tokenize-x/tx-chain/v8/x/wdistribution/types"
)

// AppModule implements an application module for the wrapped distribution module.
type AppModule struct {
	distr.AppModule

	keeper         distrkeeper.Keeper
	stakingKeeper  wdistrtypes.StakingKeeper
	legacySubspace distrexported.Subspace
}

// NewAppModule creates a new AppModule object.
func NewAppModule(
	cdc codec.Codec,
	keeper distrkeeper.Keeper,
	ak distrtypes.AccountKeeper,
	bk distrtypes.BankKeeper,
	sk wdistrtypes.StakingKeeper,
	ss distrexported.Subspace,
) AppModule {
	return AppModule{
		AppModule:      distr.NewAppModule(cdc, keeper, ak, bk, sk, ss),
		keeper:         keeper,
		stakingKeeper:  sk,
		legacySubspace: ss,
	}
}

// RegisterServices registers module services.
// It mirrors the distribution module, with the message server wrapped.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	distrMsgSrv := distrkeeper.NewMsgServerImpl(am.keeper)
	distrtypes.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(distrMsgSrv, am.stakingKeeper))
	distrtypes.RegisterQueryServer(cfg.QueryServer(), distrkeeper.NewQuerier(am.keeper))

	m := distrkeeper.NewMigrator(am.keeper, am.legacySubspace)
	if err := cfg.RegisterMigration(distrtypes.ModuleName, 1, m.Migrate1to2); err != nil {
		panic(fmt.Sprintf("failed to migrate x/%s from version 1 to 2: %v", distrtypes.ModuleName, err))
	}
	if err := cfg.RegisterMigration(distrtypes.ModuleName, 2, m.Migrate2to3); err != nil {
		panic(fmt.Sprintf("failed to migrate x/%s from version 2 to 3: %v", distrtypes.ModuleName, err))
	}
}
