package txchain

import (
	"context"
	"fmt"
	"path/filepath"

	txchaintools "github.com/tokenize-x/tx-chain/build/tools"
	"github.com/tokenize-x/tx-chain/build/tx-chain/image"
	"github.com/tokenize-x/tx-chain/v6/pkg/config/constant"
	"github.com/tokenize-x/tx-crust/build/config"
	"github.com/tokenize-x/tx-crust/build/docker"
	txcrusttools "github.com/tokenize-x/tx-crust/build/tools"
	"github.com/tokenize-x/tx-crust/build/types"
)

type imageConfig struct {
	BinaryPath      string
	TargetPlatforms []txcrusttools.TargetPlatform
	Action          docker.Action
	Username        string
	Versions        []string
}

// BuildTXdDockerImage builds txd docker image.
func BuildTXdDockerImage(ctx context.Context, deps types.DepsFunc) error {
	deps(BuildTXdInDocker, ensureReleasedBinaries)

	return buildTXdDockerImage(ctx, imageConfig{
		BinaryPath:      binaryPath,
		TargetPlatforms: []txcrusttools.TargetPlatform{txcrusttools.TargetPlatformLinuxLocalArchInDocker},
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
