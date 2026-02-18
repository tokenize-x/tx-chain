package txchain

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"

	txchaintools "github.com/tokenize-x/tx-chain/build/tools"
	"github.com/tokenize-x/tx-crust/build/config"
	"github.com/tokenize-x/tx-crust/build/docker"
	dockerbasic "github.com/tokenize-x/tx-crust/build/docker/basic"
	"github.com/tokenize-x/tx-crust/build/git"
	"github.com/tokenize-x/tx-crust/build/golang"
	"github.com/tokenize-x/tx-crust/build/lint"
	txcrusttools "github.com/tokenize-x/tx-crust/build/tools"
	"github.com/tokenize-x/tx-crust/build/types"
	"github.com/tokenize-x/tx-tools/pkg/libexec"
)

const (
	blockchainName    = "tx-chain"
	binaryName        = "txd"
	gaiaBinaryName    = "gaiad"
	hermesBinaryName  = "hermes"
	osmosisBinaryName = "osmosisd"
	repoPath          = "."

	binaryPath          = "bin/" + binaryName
	gaiaBinaryPath      = "bin/" + gaiaBinaryName
	hermesBinaryPath    = "bin/" + hermesBinaryName
	osmosisBinaryPath   = "bin/" + osmosisBinaryName
	integrationTestsDir = repoPath + "/integration-tests"

	cosmovisorBinaryPath = "bin/cosmovisor"
	goCoverFlag          = "-cover"
)

var defaultBuildTags = []string{"netgo", "ledger"}

// BuildTXd builds all the versions of txd binary.
func BuildTXd(ctx context.Context, deps types.DepsFunc) error {
	deps(BuildTXdLocally, BuildTXdInDocker)
	return nil
}

// BuildTXdLocally builds txd locally.
func BuildTXdLocally(ctx context.Context, deps types.DepsFunc) error {
	ldFlags, err := txdVersionLDFlags(ctx, defaultBuildTags)
	if err != nil {
		return err
	}

	return golang.Build(ctx, deps, golang.BinaryBuildConfig{
		TargetPlatform: txcrusttools.TargetPlatformLocal,
		PackagePath:    "cmd/txd",
		BinOutputPath:  binaryPath,
		CGOEnabled:     true,
		Tags:           defaultBuildTags,
		LDFlags:        ldFlags,
	})
}

// copyLocalBinary copies the binary to the cache dir.
func copyLocalBinary(src, dst string) error {
	// create dir from path
	err := os.MkdirAll(filepath.Dir(dst), os.ModePerm)
	if err != nil {
		return errors.WithStack(err)
	}

	// copy the file we need
	fr, err := os.Open(src)
	if err != nil {
		return errors.WithStack(err)
	}
	defer fr.Close()
	fw, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return errors.WithStack(err)
	}
	defer fw.Close()
	if _, err = io.Copy(fw, fr); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// BuildTXdInDocker builds txd in docker.
func BuildTXdInDocker(ctx context.Context, deps types.DepsFunc) error {
	return buildTXdInDocker(ctx, deps, txcrusttools.TargetPlatformLinuxLocalArchInDocker, []string{goCoverFlag}, false)
}

// BuildGaiaDockerImage builds docker image of the gaia.
func BuildGaiaDockerImage(ctx context.Context, deps types.DepsFunc) error {
	if err := txcrusttools.Ensure(ctx, txchaintools.Gaia, txcrusttools.TargetPlatformLinuxLocalArchInDocker); err != nil {
		return err
	}

	gaiaLocalPath := filepath.Join(
		"bin", ".cache", gaiaBinaryName, txcrusttools.TargetPlatformLinuxLocalArchInDocker.String(),
	)
	if err := txcrusttools.CopyToolBinaries(
		txchaintools.Gaia,
		txcrusttools.TargetPlatformLinuxLocalArchInDocker,
		gaiaLocalPath,
		gaiaBinaryPath,
	); err != nil {
		return err
	}

	dockerfile, err := dockerbasic.Execute(dockerbasic.Data{
		From:   docker.AlpineImage,
		Binary: gaiaBinaryPath,
	})
	if err != nil {
		return err
	}

	return docker.BuildImage(ctx, docker.BuildImageConfig{
		ContextDir: gaiaLocalPath,
		ImageName:  gaiaBinaryName,
		Dockerfile: dockerfile,
		Versions:   []string{config.ZNetVersion},
	})
}

// BuildHermesDockerImage builds docker image of the ibc relayer.
func BuildHermesDockerImage(ctx context.Context, deps types.DepsFunc) error {
	if err := txcrusttools.Ensure(
		ctx, txchaintools.Hermes, txcrusttools.TargetPlatformLinuxLocalArchInDocker,
	); err != nil {
		return err
	}

	hermesLocalPath := filepath.Join(
		"bin", ".cache", hermesBinaryName, txcrusttools.TargetPlatformLinuxLocalArchInDocker.String(),
	)
	if err := txcrusttools.CopyToolBinaries(
		txchaintools.Hermes,
		txcrusttools.TargetPlatformLinuxLocalArchInDocker,
		hermesLocalPath,
		hermesBinaryPath,
	); err != nil {
		return err
	}

	dockerfile, err := dockerbasic.Execute(dockerbasic.Data{
		From:   docker.UbuntuImage,
		Binary: hermesBinaryPath,
		Run:    "apt update && apt install curl jq -y",
	})
	if err != nil {
		return err
	}

	return docker.BuildImage(ctx, docker.BuildImageConfig{
		ContextDir: hermesLocalPath,
		ImageName:  hermesBinaryName,
		Dockerfile: dockerfile,
		Versions:   []string{config.ZNetVersion},
	})
}

// BuildOsmosisDockerImage builds docker image of the osmosis.
func BuildOsmosisDockerImage(ctx context.Context, deps types.DepsFunc) error {
	if err := txcrusttools.Ensure(ctx,
		txchaintools.Osmosis, txcrusttools.TargetPlatformLinuxLocalArchInDocker); err != nil {
		return err
	}

	binaryLocalPath := filepath.Join(
		"bin", ".cache", osmosisBinaryName, txcrusttools.TargetPlatformLinuxLocalArchInDocker.String(),
	)
	if err := txcrusttools.CopyToolBinaries(
		txchaintools.Osmosis,
		txcrusttools.TargetPlatformLinuxLocalArchInDocker,
		binaryLocalPath,
		osmosisBinaryPath,
	); err != nil {
		return err
	}

	dockerfile, err := dockerbasic.Execute(dockerbasic.Data{
		From:   docker.AlpineImage,
		Binary: osmosisBinaryPath,
	})
	if err != nil {
		return err
	}

	return docker.BuildImage(ctx, docker.BuildImageConfig{
		ContextDir: binaryLocalPath,
		ImageName:  osmosisBinaryName,
		Dockerfile: dockerfile,
		Versions:   []string{config.ZNetVersion},
	})
}

// linuxMuslToolchain holds paths for a Linux muslc build (same toolchain for local and in-Docker).
type linuxMuslToolchain struct {
	hostCCDirPath            string
	ccRelativePath           string
	wasmHostDirPath          string
	wasmCCLibRelativeLibPath string
}

// linuxMuslToolchainFor returns the musl toolchain config for the given Linux platform.
// targetPlatform is used for all Path() lookups so local and in-Docker builds use the same toolchain.
func linuxMuslToolchainFor(targetPlatform txcrusttools.TargetPlatform) (linuxMuslToolchain, error) {
	switch targetPlatform {
	case txcrusttools.TargetPlatformLinuxAMD64InDocker,
		txcrusttools.TargetPlatformLinuxARM64InDocker,
		txcrusttools.TargetPlatformLinuxLocalArchInDocker:
		// fall through to arch switch
	default:
		return linuxMuslToolchain{}, errors.Errorf("building is not possible for platform %s", targetPlatform)
	}

	var gccBin, wasmLib, wasmSubdir string
	switch targetPlatform.Arch {
	case txcrusttools.ArchAMD64:
		gccBin, wasmLib, wasmSubdir = "bin/x86_64-linux-musl-gcc", "lib/libwasmvm_muslc.x86_64.a", "/x86_64-linux-musl/lib/libwasmvm_muslc.x86_64.a"
	case txcrusttools.ArchARM64:
		gccBin, wasmLib, wasmSubdir = "bin/aarch64-linux-musl-gcc", "lib/libwasmvm_muslc.aarch64.a", "/aarch64-linux-musl/lib/libwasmvm_muslc.aarch64.a"
	default:
		return linuxMuslToolchain{}, errors.Errorf("building is not possible for platform %s", targetPlatform)
	}

	hostCCDirPath := filepath.Dir(filepath.Dir(txcrusttools.Path(gccBin, targetPlatform)))
	ccRelativePath := "/" + gccBin
	wasmHostDirPath := txcrusttools.Path(wasmLib, targetPlatform)
	return linuxMuslToolchain{
		hostCCDirPath:            hostCCDirPath,
		ccRelativePath:           ccRelativePath,
		wasmHostDirPath:          wasmHostDirPath,
		wasmCCLibRelativeLibPath: wasmSubdir,
	}, nil
}

func buildTXdInDocker(
	ctx context.Context,
	deps types.DepsFunc,
	targetPlatform txcrusttools.TargetPlatform,
	extraFlags []string,
	release bool,
) error {
	if err := txcrusttools.Ensure(ctx, txchaintools.LibWASM, targetPlatform); err != nil {
		return err
	}

	ldFlags := make([]string, 0)
	var cc string
	buildTags := defaultBuildTags
	envs := make([]string, 0)
	dockerVolumes := make([]string, 0)
	switch targetPlatform.OS {
	case txcrusttools.OSLinux:
		// Linux builds must use muslc + static linking so the binary runs in Alpine (txd Docker image).
		// Using glibc or dynamic wasmvm leads to SIGABRT in wasmvm/cgo when the binary runs in-container.
		if err := txcrusttools.Ensure(ctx, txchaintools.MuslCC, targetPlatform); err != nil {
			return err
		}
		buildTags = append(buildTags, "muslc")
		ldFlags = append(ldFlags, "-extldflags '-static'")

		tc, err := linuxMuslToolchainFor(targetPlatform)
		if err != nil {
			return err
		}

		if !release && runtime.GOOS == txcrusttools.OSLinux {
			// Local Linux build: same muslc/static config as in-Docker, output to cache for Docker image.
			targetPlatform = txcrusttools.TargetPlatformLocal
			if err := copyLocalBinary(tc.wasmHostDirPath, tc.hostCCDirPath+tc.wasmCCLibRelativeLibPath); err != nil {
				return err
			}
			cc = tc.hostCCDirPath + tc.ccRelativePath
		} else {
			const ccDockerDir = "/musl-gcc"
			dockerVolumes = append(
				dockerVolumes,
				fmt.Sprintf("%s:%s", tc.hostCCDirPath, ccDockerDir),
				// put the libwasmvm to the lib folder of the compiler
				fmt.Sprintf("%s:%s%s", tc.wasmHostDirPath, ccDockerDir, tc.wasmCCLibRelativeLibPath),
			)
			cc = ccDockerDir + tc.ccRelativePath
		}
	case txcrusttools.OSDarwin:
		buildTags = append(buildTags, "static_wasm")
		switch targetPlatform {
		case txcrusttools.TargetPlatformDarwinAMD64InDocker:
			cc = "o64-clang"
		case txcrusttools.TargetPlatformDarwinARM64InDocker:
			cc = "oa64-clang"
		default:
			return errors.Errorf("building is not possible for platform %s", targetPlatform)
		}
		wasmHostDirPath := txcrusttools.Path("lib/libwasmvmstatic_darwin.a", targetPlatform)
		dockerVolumes = append(dockerVolumes, fmt.Sprintf("%s:%s", wasmHostDirPath, "/lib/libwasmvmstatic_darwin.a"))
		envs = append(envs, "CGO_LDFLAGS=-L/lib")
	default:
		return errors.Errorf("building is not possible for platform %s", targetPlatform)
	}

	envs = append(envs, "CC="+cc)

	versionLDFlags, err := txdVersionLDFlags(ctx, buildTags)
	if err != nil {
		return err
	}
	ldFlags = append(ldFlags, versionLDFlags...)

	binOutputPath := filepath.Join("bin", ".cache", binaryName, targetPlatform.String(), "bin", binaryName)
	return golang.Build(ctx, deps, golang.BinaryBuildConfig{
		TargetPlatform: targetPlatform,
		PackagePath:    "cmd/txd",
		BinOutputPath:  binOutputPath,
		CGOEnabled:     true,
		Tags:           buildTags,
		LDFlags:        ldFlags,
		Flags:          extraFlags,
		Envs:           envs,
		DockerVolumes:  dockerVolumes,
	})
}

// Lint lints tx-chain repo.
func Lint(ctx context.Context, deps types.DepsFunc) error {
	deps(
		Generate,
		CompileAllSmartContracts,
		formatProto,
		lintProto,
		// breakingProto, TODO(Restore breaking proto)
	)
	return lint.Lint(ctx, deps)
}

func txdVersionLDFlags(ctx context.Context, buildTags []string) ([]string, error) {
	hash, err := git.DirtyHeadHash(ctx)
	if err != nil {
		return nil, err
	}

	version, err := git.VersionFromTag(ctx)
	if err != nil {
		return nil, err
	}
	if version == "" {
		version = hash
	}

	ps := map[string]string{
		"github.com/cosmos/cosmos-sdk/version.Name":    blockchainName,
		"github.com/cosmos/cosmos-sdk/version.AppName": binaryName,
		"github.com/cosmos/cosmos-sdk/version.Version": version,
		"github.com/cosmos/cosmos-sdk/version.Commit":  hash,
	}

	if len(buildTags) > 0 {
		ps["github.com/cosmos/cosmos-sdk/version.BuildTags"] = strings.Join(buildTags, ",")
	}

	var values []string
	for k, v := range ps {
		values = append(values, fmt.Sprintf("-X %s=%s", k, v))
	}

	return values, nil
}

func formatProto(ctx context.Context, deps types.DepsFunc) error {
	deps(txchaintools.EnsureBuf)

	cmd := exec.Command(txcrusttools.Path("bin/buf", txcrusttools.TargetPlatformLocal), "format", "-w")
	cmd.Dir = filepath.Join(repoPath, "proto", "coreum")
	return libexec.Exec(ctx, cmd)
}
