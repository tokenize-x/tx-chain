package txchain

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"

	txchaintools "github.com/tokenize-x/tx-chain/build/tools"
	"github.com/tokenize-x/tx-chain/build/tx-chain/image"
	"github.com/tokenize-x/tx-chain/v8/pkg/config/constant"
	"github.com/tokenize-x/tx-crust/build/config"
	"github.com/tokenize-x/tx-crust/build/docker"
	txcrusttools "github.com/tokenize-x/tx-crust/build/tools"
	"github.com/tokenize-x/tx-crust/build/types"
)

type imageConfig struct {
	BinaryPath        string
	TargetPlatforms   []txcrusttools.TargetPlatform
	Action            docker.Action
	ContainerRegistry string
	OrgName           string
	Versions          []string
	UseLocalBinary    bool
	// BaseImage selects the Docker base OS and its linking strategy.
	// Zero value defaults to docker.ImageOSAlpine (musl/static).
	//   docker.ImageOSAlpine — musl static linking. Smallest. Works on Linux/CI.
	//   docker.ImageOSUbuntu — glibc dynamic linking. Works on Mac Apple Silicon Docker Desktop.
	BaseImage docker.ImageOS
}

// BuildTXdDockerImage builds the txd Docker image, using the base image from the
// context (set via --override-os) and falling back to Alpine when absent.
func BuildTXdDockerImage(ctx context.Context, deps types.DepsFunc) error {
	return BuildTXdDockerImageFor(TXdImageOSFromContext(ctx))(ctx, deps)
}

// BuildTXdDockerImageFor returns a CommandFunc that builds the txd Docker image using the
// given base image. The Linux link mode (musl/static vs glibc/dynamic) is derived automatically.
func BuildTXdDockerImageFor(baseImage docker.ImageOS) types.CommandFunc {
	return func(ctx context.Context, deps types.DepsFunc) error {
		deps(BuildTXdInDockerFor(baseImage), ensureReleasedBinaries)

		// skip building TXd in docker for Linux builds to avoid using the large GoReleaser when unnecessary
		useLocalBinary := runtime.GOOS == txcrusttools.OSLinux

		return buildTXdDockerImage(ctx, imageConfig{
			BinaryPath:      binaryPath,
			TargetPlatforms: []txcrusttools.TargetPlatform{txcrusttools.TargetPlatformLinuxLocalArchInDocker},
			Action:          docker.ActionLoad,
			Versions:        []string{config.ZNetVersion},
			UseLocalBinary:  useLocalBinary,
			BaseImage:       baseImage,
		})
	}
}

func buildTXdDockerImage(ctx context.Context, cfg imageConfig) error {
	baseImage := cfg.BaseImage
	if baseImage == "" {
		baseImage = docker.ImageOSAlpine
	}

	binaryName := filepath.Base(cfg.BinaryPath)
	for _, platform := range cfg.TargetPlatforms {
		if err := ensureCosmovisorWithInstalledBinary(ctx, platform, binaryName); err != nil {
			return err
		}
		if err := ensureWASMLibForDockerContext(ctx, platform, binaryName, linuxLinkModeForImage(baseImage)); err != nil {
			return err
		}
	}

	dockerfile, err := image.Execute(image.Data{
		From:             baseImage.String(),
		TXdBinary:        cfg.BinaryPath,
		CosmovisorBinary: cosmovisorBinaryPath,
		IncludeWASMLib:   linuxLinkModeForImage(baseImage) == linuxLinkModeDynamicGlibc,
		Networks: []string{
			string(constant.ChainIDDev),
			string(constant.ChainIDTest),
		},
		InDocker: !cfg.UseLocalBinary,
	})
	if err != nil {
		return err
	}

	return docker.BuildImage(ctx, docker.BuildImageConfig{
		ContextDir:        filepath.Join("bin", ".cache", binaryName),
		ImageName:         binaryName,
		TargetPlatforms:   cfg.TargetPlatforms,
		Action:            cfg.Action,
		Versions:          cfg.Versions,
		ContainerRegistry: cfg.ContainerRegistry,
		OrgName:           cfg.OrgName,
		Dockerfile:        dockerfile,
	})
}

// ensureWASMLibForDockerContext copies libwasmvm into the Docker build context when
// building for glibc/Ubuntu. For musl/Alpine the binary is statically linked, so this is a no-op.
func ensureWASMLibForDockerContext(
	ctx context.Context,
	platform txcrusttools.TargetPlatform,
	binaryName string,
	mode linuxLinkMode,
) error {
	if mode == linuxLinkModeStaticMusl {
		return nil
	}
	if err := txcrusttools.Ensure(ctx, txchaintools.LibWASMGlibc, platform); err != nil {
		return err
	}
	arch, err := linuxArchName(platform)
	if err != nil {
		return err
	}
	return txcrusttools.CopyToolBinaries(
		txchaintools.LibWASMGlibc,
		platform,
		filepath.Join("bin", ".cache", binaryName, platform.String()),
		fmt.Sprintf("lib/libwasmvm.%s.so", arch),
	)
}

// ensureReleasedBinaries ensures that all previous txd versions are installed.
func ensureReleasedBinaries(ctx context.Context, deps types.DepsFunc) error {
	const binaryTool = txchaintools.TXdV700
	if err := txcrusttools.Ensure(ctx, binaryTool, txcrusttools.TargetPlatformLinuxLocalArchInDocker); err != nil {
		return err
	}
	if err := txcrusttools.CopyToolBinaries(
		binaryTool,
		txcrusttools.TargetPlatformLinuxLocalArchInDocker,
		filepath.Join("bin", ".cache", binaryName, txcrusttools.TargetPlatformLinuxLocalArchInDocker.String()),
		fmt.Sprintf("bin/%s", binaryTool)); err != nil {
		return err
	}
	// copy the release binary for the local platform to use for the genesis generation
	if err := txcrusttools.Ensure(ctx, binaryTool, txcrusttools.TargetPlatformLocal); err != nil {
		return err
	}
	return txcrusttools.CopyToolBinaries(
		binaryTool,
		txcrusttools.TargetPlatformLocal,
		filepath.Join("bin", ".cache", binaryName, txcrusttools.TargetPlatformLocal.String()),
		fmt.Sprintf("bin/%s", binaryTool),
	)
}
