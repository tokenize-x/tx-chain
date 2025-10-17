package txchain

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/samber/lo"

	"github.com/tokenize-x/tx-chain/v6/testutil/simapp"
	"github.com/tokenize-x/tx-crust/build/golang"
	"github.com/tokenize-x/tx-crust/build/types"
	"github.com/tokenize-x/tx-crust/znet/infra"
	"github.com/tokenize-x/tx-crust/znet/infra/apps"
	"github.com/tokenize-x/tx-crust/znet/infra/apps/txd"
	"github.com/tokenize-x/tx-crust/znet/pkg/znet"
)

// Test names.
const (
	TestIBC     = "ibc"
	TestModules = "modules"
	TestUpgrade = "upgrade"
	TestStress  = "stress"
	// TestExport is a special test that runs after all other tests.
	TestExport = "export"
)

// Test run unit tests in tx-chain repo.
func Test(ctx context.Context, deps types.DepsFunc) error {
	deps(CompileAllSmartContracts)

	return golang.Test(ctx, deps)
}

// RunAllIntegrationTests runs all the tx-chain integration tests.
func RunAllIntegrationTests(runUnsafe bool) types.CommandFunc {
	return func(ctx context.Context, deps types.DepsFunc) error {
		deps(
			RunIntegrationTestsModules(runUnsafe),
			RunIntegrationTestsIBC(runUnsafe),
			RunIntegrationTestsUpgrade(runUnsafe),
		)
		return nil
	}
}

// RunIntegrationTestsModules returns function running modules integration tests.
func RunIntegrationTestsModules(runUnsafe bool) types.CommandFunc {
	return func(ctx context.Context, deps types.DepsFunc) error {
		deps(CompileModulesSmartContracts, CompileAssetExtensionSmartContracts,
			CompileDEXSmartContracts, BuildTXdLocally, BuildTXdDockerImage)

		znetConfig := defaultZNetConfig()
		znetConfig.Profiles = []string{apps.Profile3TXd}
		znetConfig.CoverageOutputFile = "coverage/coreum-integration-tests-modules"

		return runIntegrationTests(ctx, deps, runUnsafe, false, znetConfig, TestModules)
	}
}

// RunIntegrationTestsStress returns function running stress integration tests.
func RunIntegrationTestsStress(runUnsafe bool) types.CommandFunc {
	return func(ctx context.Context, deps types.DepsFunc) error {
		deps(BuildTXdLocally, BuildTXdDockerImage)

		znetConfig := defaultZNetConfig()
		znetConfig.Profiles = []string{apps.Profile3TXd, apps.ProfileDEX}
		znetConfig.CoverageOutputFile = "coverage/coreum-integration-tests-stress"

		return runIntegrationTests(ctx, deps, runUnsafe, false, znetConfig, TestStress)
	}
}

// RunIntegrationTestsIBC returns function running IBC integration tests.
func RunIntegrationTestsIBC(runUnsafe bool) types.CommandFunc {
	return func(ctx context.Context, deps types.DepsFunc) error {
		deps(CompileIBCSmartContracts, CompileAssetExtensionSmartContracts, CompileDEXSmartContracts,
			BuildTXdLocally, BuildTXdDockerImage, BuildGaiaDockerImage, BuildOsmosisDockerImage,
			BuildHermesDockerImage)

		znetConfig := defaultZNetConfig()
		znetConfig.Profiles = []string{apps.Profile3TXd, apps.ProfileIBC}

		return runIntegrationTests(ctx, deps, runUnsafe, false, znetConfig, TestIBC)
	}
}

// RunIntegrationTestsUpgrade returns function running upgrade integration tests.
func RunIntegrationTestsUpgrade(runUnsafe bool) types.CommandFunc {
	return func(ctx context.Context, deps types.DepsFunc) error {
		deps(CompileIBCSmartContracts, CompileAssetExtensionSmartContracts, CompileDEXSmartContracts,
			CompileModulesSmartContracts, BuildTXdLocally, BuildTXdDockerImage,
			BuildGaiaDockerImage, BuildOsmosisDockerImage, BuildHermesDockerImage)

		znetConfig := defaultZNetConfig()
		znetConfig.Profiles = []string{apps.Profile3TXd, apps.ProfileIBC}
		znetConfig.TXdVersion = "v5.0.0"

		return runIntegrationTests(ctx, deps, runUnsafe, true, znetConfig, TestUpgrade, TestIBC, TestModules)
	}
}

// TestFuzz run fuzz tests in tx-chain repo.
func TestFuzz(ctx context.Context, deps types.DepsFunc) error {
	deps(CompileAllSmartContracts)

	return golang.TestFuzz(ctx, deps, time.Minute)
}

func runIntegrationTests(
	ctx context.Context,
	deps types.DepsFunc,
	runUnsafe bool,
	runExport bool,
	znetConfig *infra.ConfigFactory,
	testDirs ...string,
) error {
	// General flags for all tests
	generalFlags := []string{
		"-tags=integrationtests",
		fmt.Sprintf("-parallel=%d", 2*runtime.NumCPU()),
		"-timeout=1h",
	}

	// Start znet for regular tests
	if err := znet.Remove(ctx, znetConfig); err != nil {
		return err
	}
	if err := znet.Start(ctx, znetConfig); err != nil {
		return err
	}

	// Run regular integration tests with general flags (+ --run-unsafe if set)
	regularFlags := append([]string{}, generalFlags...)
	if runUnsafe {
		regularFlags = append(regularFlags, "--run-unsafe")
	}
	for _, testDir := range testDirs {
		if err := golang.RunTests(ctx, deps, golang.TestConfig{
			PackagePath: filepath.Join(integrationTestsDir, testDir),
			Flags:       regularFlags,
		}); err != nil {
			return err
		}
	}

	// Run export test last, with general flags + node-app-dir and exported-genesis-path
	if runExport {
		// Stop znet before export test
		if err := znet.Stop(ctx, znetConfig); err != nil {
			return err
		}

		// Dump the validator application directory
		nodeAppDir, err := znet.DumpAppDir(znetConfig, txd.AppType)
		if err != nil {
			return err
		}

		// Export the genesis state
		exportedGenesisPath, err := znet.ExportGenesis(ctx, znetConfig, simapp.GetModulesToExport())
		if err != nil {
			return err
		}

		exportFlags := append([]string{}, generalFlags...)
		exportFlags = append(exportFlags, "--node-app-dir="+nodeAppDir)
		exportFlags = append(exportFlags, "--exported-genesis-path="+exportedGenesisPath)
		if err := golang.RunTests(ctx, deps, golang.TestConfig{
			PackagePath: filepath.Join(integrationTestsDir, TestExport),
			Flags:       exportFlags,
		}); err != nil {
			return err
		}
	}

	if znetConfig.CoverageOutputFile != "" {
		if err := znet.Stop(ctx, znetConfig); err != nil {
			return err
		}
		if err := znet.CoverageConvert(ctx, znetConfig); err != nil {
			return err
		}
	}

	return znet.Remove(ctx, znetConfig)
}

func defaultZNetConfig() *infra.ConfigFactory {
	return &infra.ConfigFactory{
		EnvName:       "znet",
		TimeoutCommit: 500 * time.Millisecond,
		HomeDir:       filepath.Join(lo.Must(os.UserHomeDir()), ".crust", "znet"),
		RootDir:       ".",
		TXdUpgrades:   TXdUpgrades(),
	}
}
