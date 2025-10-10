package txchain

import (
	"context"
	"path/filepath"

	txchaintools "github.com/tokenize-x/tx-chain/build/tools"
	txcrusttools "github.com/tokenize-x/tx-crust/build/tools"
)

func ensureCosmovisorWithInstalledBinary(
	ctx context.Context, platform txcrusttools.TargetPlatform, binaryName string,
) error {
	if err := txcrusttools.Ensure(ctx, txchaintools.Cosmovisor, platform); err != nil {
		return err
	}

	return txcrusttools.CopyToolBinaries(txchaintools.Cosmovisor,
		platform,
		filepath.Join("bin", ".cache", binaryName, platform.String()),
		cosmovisorBinaryPath)
}
