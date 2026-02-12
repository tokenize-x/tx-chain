package cosmoscmd

import (
	"fmt"

	"github.com/99designs/keyring"
	sdkversion "github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
)

// Legacy OS keyring service names from previous binary versions.
var legacyServiceNames = []string{"coreum"}

// MigrateKeyringCmd migrates OS keyring keys from legacy service namespaces into the current one.
func MigrateKeyringCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-keyring",
		Short: "Migrate OS keyring keys from coreum namespace into tx-chain",
		Long: `Copies OS keyring keys from "coreum" Keychain service into "tx-chain".
Only affects the "os" keyring backend.`,
		Args: cobra.NoArgs,
		RunE: runMigrateKeyring,
	}
	return cmd
}

func openOSKeyring(serviceName string) (keyring.Keyring, error) {
	return keyring.Open(keyring.Config{
		ServiceName:              serviceName,
		KeychainTrustApplication: true,
		FilePasswordFunc:         func(_ string) (string, error) { return "", nil },
	})
}

func runMigrateKeyring(cmd *cobra.Command, _ []string) error {
	targetName := sdkversion.Name // "tx-chain"

	destKr, err := openOSKeyring(targetName)
	if err != nil {
		return fmt.Errorf("failed to open destination keyring %q: %w", targetName, err)
	}

	totalMigrated := 0

	for _, srcName := range legacyServiceNames {
		if srcName == targetName {
			continue
		}

		srcKr, err := openOSKeyring(srcName)
		if err != nil {
			cmd.PrintErrf("Warning: could not open keyring %q: %v\n", srcName, err)
			continue
		}

		keys, err := srcKr.Keys()
		if err != nil {
			cmd.PrintErrf("Warning: could not list keys from %q: %v\n", srcName, err)
			continue
		}

		if len(keys) == 0 {
			cmd.Printf("No keys found in %q namespace.\n", srcName)
			continue
		}

		migrated := 0
		for _, key := range keys {
			if _, err := destKr.Get(key); err == nil {
				cmd.Printf("  Skipping %q (already exists in %q)\n", key, targetName)
				continue
			}

			item, err := srcKr.Get(key)
			if err != nil {
				cmd.PrintErrf("  Warning: could not read %q from %q: %v\n", key, srcName, err)
				continue
			}

			if err := destKr.Set(item); err != nil {
				cmd.PrintErrf("  Warning: could not write %q to %q: %v\n", key, targetName, err)
				continue
			}

			migrated++
		}

		if migrated > 0 {
			cmd.Printf("Migrated %d key(s) from %q to %q.\n", migrated, srcName, targetName)
		}
		totalMigrated += migrated
	}

	if totalMigrated == 0 {
		cmd.Println("No keys needed migration.")
	} else {
		cmd.Printf("Total: %d key(s) migrated to %q.\n", totalMigrated, targetName)
	}

	return nil
}
