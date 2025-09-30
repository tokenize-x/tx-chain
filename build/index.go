package build

import (
	"context"

	"github.com/tokenize-x/crust/build/crust"
	"github.com/tokenize-x/crust/build/golang"
	"github.com/tokenize-x/crust/build/tools"
	"github.com/tokenize-x/crust/build/types"
	txchain "github.com/tokenize-x/tx-chain/build/tx-chain"
)

// Commands is a definition of commands available in build system.
var Commands = map[string]types.Command{
	"build/me":   {Fn: txchain.BuildBuilder, Description: "Builds the builder"},
	"build/znet": {Fn: crust.BuildZNet, Description: "Builds znet binary"},
	"build": {Fn: func(ctx context.Context, deps types.DepsFunc) error {
		deps(
			txchain.BuildTXd,
		)
		return nil
	}, Description: "Builds txd binaries"},
	"build/txd": {Fn: txchain.BuildTXd, Description: "Builds txd binary"},
	"generate":  {Fn: txchain.Generate, Description: "Generates artifacts"},
	"setup":     {Fn: tools.InstallAll, Description: "Installs all the required tools"},
	"images": {Fn: func(ctx context.Context, deps types.DepsFunc) error {
		deps(
			txchain.BuildTXdDockerImage,
			txchain.BuildGaiaDockerImage,
			txchain.BuildHermesDockerImage,
			txchain.BuildOsmosisDockerImage,
		)
		return nil
	}, Description: "Builds txd docker images"},
	"images/txd":     {Fn: txchain.BuildTXdDockerImage, Description: "Builds txd docker image"},
	"images/gaiad":   {Fn: txchain.BuildGaiaDockerImage, Description: "Builds gaia docker image"},
	"images/hermes":  {Fn: txchain.BuildHermesDockerImage, Description: "Builds hermes docker image"},
	"images/osmosis": {Fn: txchain.BuildOsmosisDockerImage, Description: "Builds osmosis docker image"},
	"integration-tests": {
		Fn:          txchain.RunAllIntegrationTests(false),
		Description: "Runs all safe integration tests",
	},
	"integration-tests-unsafe": {
		Fn:          txchain.RunAllIntegrationTests(true),
		Description: "Runs all the integration tests including unsafe",
	},
	"integration-tests/ibc": {
		Fn:          txchain.RunIntegrationTestsIBC(false),
		Description: "Runs safe IBC integration tests",
	},
	"integration-tests-unsafe/ibc": {
		Fn:          txchain.RunIntegrationTestsIBC(true),
		Description: "Runs all IBC integration tests including unsafe",
	},
	"integration-tests/modules": {
		Fn:          txchain.RunIntegrationTestsModules(false),
		Description: "Runs safe modules integration tests",
	},
	"integration-tests-unsafe/modules": {
		Fn:          txchain.RunIntegrationTestsModules(true),
		Description: "Runs all modules integration tests including unsafe",
	},
	"integration-tests-unsafe/stress": {
		Fn:          txchain.RunIntegrationTestsStress(true),
		Description: "Runs unsafe stress integration tests",
	},
	"integration-tests/upgrade": {
		Fn:          txchain.RunIntegrationTestsUpgrade(false),
		Description: "Runs safe upgrade integration tests",
	},
	"integration-tests-unsafe/upgrade": {
		Fn:          txchain.RunIntegrationTestsUpgrade(true),
		Description: "Runs all upgrade integration tests including unsafe",
	},
	"lint":           {Fn: txchain.Lint, Description: "Lints code"},
	"release":        {Fn: txchain.ReleaseTXd, Description: "Releases txd binary"},
	"release/images": {Fn: txchain.ReleaseTXdImage, Description: "Releases txd docker images"},
	"test":           {Fn: txchain.Test, Description: "Runs unit tests"},
	"test-fuzz":      {Fn: txchain.TestFuzz, Description: "Runs fuzz tests"},
	"tidy":           {Fn: golang.Tidy, Description: "Runs go mod tidy"},
	"wasm": {
		Fn:          txchain.CompileAllSmartContracts,
		Description: "Builds smart contracts required by integration tests",
	},
}
