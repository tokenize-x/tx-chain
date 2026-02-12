package cosmoscmd

import (
	"fmt"

	"github.com/99designs/keyring"
	sdkversion "github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
)

// legacyServiceName is the OS keyring service name used by cored.
const legacyServiceName = "coreum"

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

	if legacyServiceName == targetName {
		cmd.Println("Source and destination namespaces are the same, nothing to migrate.")
		return nil
	}

	srcKr, err := openOSKeyring(legacyServiceName)
	if err != nil {
		return fmt.Errorf("failed to open source keyring %q: %w", legacyServiceName, err)
	}

	destKr, err := openOSKeyring(targetName)
	if err != nil {
		return fmt.Errorf("failed to open destination keyring %q: %w", targetName, err)
	}

	keys, err := srcKr.Keys()
	if err != nil {
		return fmt.Errorf("failed to list keys from %q: %w", legacyServiceName, err)
	}

	if len(keys) == 0 {
		cmd.Printf("No keys found in %q namespace.\n", legacyServiceName)
		return nil
	}

	migrated := 0
	for _, key := range keys {
		if _, err := destKr.Get(key); err == nil {
			cmd.Printf("  Skipping %q (already exists in %q)\n", key, targetName)
			continue
		}

		item, err := srcKr.Get(key)
		if err != nil {
			cmd.PrintErrf("  Warning: could not read %q from %q: %v\n", key, legacyServiceName, err)
			continue
		}

		if err := destKr.Set(item); err != nil {
			cmd.PrintErrf("  Warning: could not write %q to %q: %v\n", key, targetName, err)
			continue
		}

		migrated++
	}

	if migrated == 0 {
		cmd.Println("No keys needed migration.")
	} else {
		cmd.Printf("Migrated %d key(s) from %q to %q.\n", migrated, legacyServiceName, targetName)
	}

	return nil
}
