package txchain

import (
	"context"

	"github.com/pkg/errors"

	"github.com/tokenize-x/crust/build/config"
	"github.com/tokenize-x/crust/build/docker"
	"github.com/tokenize-x/crust/build/git"
	"github.com/tokenize-x/crust/build/tools"
	"github.com/tokenize-x/crust/build/types"
)

// ReleaseTXd releases txd binary for amd64 and arm64 to be published inside the release.
func ReleaseTXd(ctx context.Context, deps types.DepsFunc) error {
	clean, _, err := git.StatusClean(ctx)
	if err != nil {
		return err
	}
	if !clean {
		return errors.New("released commit contains uncommitted changes")
	}

	version, err := git.VersionFromTag(ctx)
	if err != nil {
		return err
	}
	if version == "" {
		return errors.New("no version present on released commit")
	}

	if err := buildTXdInDocker(
		ctx, deps, tools.TargetPlatformLinuxAMD64InDocker, []string{},
	); err != nil {
		return err
	}

	if err := buildTXdInDocker(
		ctx, deps, tools.TargetPlatformLinuxARM64InDocker, []string{},
	); err != nil {
		return err
	}

	if err := buildTXdInDocker(
		ctx, deps, tools.TargetPlatformDarwinAMD64InDocker, []string{},
	); err != nil {
		return err
	}

	return buildTXdInDocker(ctx, deps, tools.TargetPlatformDarwinARM64InDocker, []string{})
}

// ReleaseTXdImage releases txd docker images for amd64 and arm64.
func ReleaseTXdImage(ctx context.Context, deps types.DepsFunc) error {
	deps(ReleaseTXd)

	return buildTXdDockerImage(ctx, imageConfig{
		BinaryPath: binaryPath,
		TargetPlatforms: []tools.TargetPlatform{
			tools.TargetPlatformLinuxAMD64InDocker,
			tools.TargetPlatformLinuxARM64InDocker,
		},
		Action:   docker.ActionPush,
		Username: config.DockerHubUsername,
	})
}
