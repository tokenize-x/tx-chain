package txchain

import (
	"context"
	"path/filepath"

	crusttools "github.com/tokenize-x/crust/build/tools"
	txchaintools "github.com/tokenize-x/tx-chain/build/tools"
)

func ensureCosmovisorWithInstalledBinary(
	ctx context.Context, platform crusttools.TargetPlatform, binaryName string,
) error {
	if err := crusttools.Ensure(ctx, txchaintools.Cosmovisor, platform); err != nil {
		return err
	}

	return crusttools.CopyToolBinaries(txchaintools.Cosmovisor,
		platform,
		filepath.Join("bin", ".cache", binaryName, platform.String()),
		cosmovisorBinaryPath)
}
