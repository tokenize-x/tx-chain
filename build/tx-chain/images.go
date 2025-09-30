package txchain

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tokenize-x/crust/build/config"
	"github.com/tokenize-x/crust/build/docker"
	crusttools "github.com/tokenize-x/crust/build/tools"
	"github.com/tokenize-x/crust/build/types"
	txchaintools "github.com/tokenize-x/tx-chain/build/tools"
	"github.com/tokenize-x/tx-chain/build/tx-chain/image"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
)

type imageConfig struct {
	BinaryPath      string
	TargetPlatforms []crusttools.TargetPlatform
	Action          docker.Action
	Username        string
	Versions        []string
}

// BuildTXdDockerImage builds txd docker image.
func BuildTXdDockerImage(ctx context.Context, deps types.DepsFunc) error {
	deps(BuildTXdInDocker, ensureReleasedBinaries)

	return buildTXdDockerImage(ctx, imageConfig{
		BinaryPath:      binaryPath,
		TargetPlatforms: []crusttools.TargetPlatform{crusttools.TargetPlatformLinuxLocalArchInDocker},
		Action:          docker.ActionLoad,
		Versions:        []string{config.ZNetVersion},
	})
}

func buildTXdDockerImage(ctx context.Context, cfg imageConfig) error {
	binaryName := filepath.Base(cfg.BinaryPath)
	for _, platform := range cfg.TargetPlatforms {
		if err := ensureCosmovisorWithInstalledBinary(ctx, platform, binaryName); err != nil {
			return err
		}
	}
	dockerfile, err := image.Execute(image.Data{
		From:             docker.AlpineImage,
		TXdBinary:        cfg.BinaryPath,
		CosmovisorBinary: cosmovisorBinaryPath,
		Networks: []string{
			string(constant.ChainIDDev),
			string(constant.ChainIDTest),
		},
	})
	if err != nil {
		return err
	}

	return docker.BuildImage(ctx, docker.BuildImageConfig{
		ContextDir:      filepath.Join("bin", ".cache", binaryName),
		ImageName:       binaryName,
		TargetPlatforms: cfg.TargetPlatforms,
		Action:          cfg.Action,
		Versions:        cfg.Versions,
		Username:        cfg.Username,
		Dockerfile:      dockerfile,
	})
}

// ensureReleasedBinaries ensures that all previous cored versions are installed.
// TODO (v7): Rename all cored to txd.
func ensureReleasedBinaries(ctx context.Context, deps types.DepsFunc) error {
	const binaryTool = txchaintools.CoredV500
	if err := crusttools.Ensure(ctx, binaryTool, crusttools.TargetPlatformLinuxLocalArchInDocker); err != nil {
		return err
	}
	if err := crusttools.CopyToolBinaries(
		binaryTool,
		crusttools.TargetPlatformLinuxLocalArchInDocker,
		filepath.Join("bin", ".cache", binaryName, crusttools.TargetPlatformLinuxLocalArchInDocker.String()),
		fmt.Sprintf("bin/%s", binaryTool)); err != nil {
		return err
	}
	// copy the release binary for the local platform to use for the genesis generation
	if err := crusttools.Ensure(ctx, binaryTool, crusttools.TargetPlatformLocal); err != nil {
		return err
	}
	return crusttools.CopyToolBinaries(
		binaryTool,
		crusttools.TargetPlatformLocal,
		filepath.Join("bin", ".cache", binaryName, crusttools.TargetPlatformLocal.String()),
		fmt.Sprintf("bin/%s", binaryTool),
	)
}
