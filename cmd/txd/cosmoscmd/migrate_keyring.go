package cosmoscmd

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/99designs/keyring"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdkversion "github.com/cosmos/cosmos-sdk/version"
	"github.com/spf13/cobra"
)

// legacyServiceName is the OS keyring service name used by cored.
const legacyServiceName = "coreum"

// MigrateKeyringCmd migrates OS keyring keys from legacy service namespaces into the current one.
func MigrateKeyringCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-keyring [key-names...]",
		Short: "Migrate OS keyring keys from coreum namespace into tx-chain",
		Long: `Copies OS keyring keys from "coreum" Keychain service into "tx-chain".
Only affects the "os" keyring backend.

When key names are provided, only matching keys are migrated.
Without arguments, all keys are migrated.

Usage:
  txd keys migrate-keyring              # migrate all keys
  txd keys migrate-keyring alice bob    # migrate only alice and bob`,
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

func runMigrateKeyring(cmd *cobra.Command, args []string) error {
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

	// If key names provided, filter to only matching entries.
	if len(args) > 0 {
		keys = filterKeys(keys, args)
		if len(keys) == 0 {
			cmd.Println("No matching keys found in source namespace.")
			return nil
		}
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

		// After migrating a .info entry, also migrate its .address reverse-lookup entry.
		if strings.HasSuffix(key, ".info") {
			migrated += migrateAddressEntry(cmd, srcKr, destKr, item.Data)
		}
	}

	if migrated == 0 {
		cmd.Println("No keys needed migration.")
	} else {
		cmd.Printf("Migrated %d key(s) from %q to %q.\n", migrated, legacyServiceName, targetName)
	}

	return nil
}

// migrateAddressEntry extracts and migrates the corresponding <hex>.address reverse-lookup entry.
func migrateAddressEntry(cmd *cobra.Command, srcKr, destKr keyring.Keyring, infoData []byte) int {
	ir := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(ir)
	cdc := codec.NewProtoCodec(ir)

	var record sdkkeyring.Record
	if err := cdc.Unmarshal(infoData, &record); err != nil {
		cmd.PrintErrf("  Warning: could not unmarshal record: %v\n", err)
		return 0
	}

	pubKey, err := record.GetPubKey()
	if err != nil {
		cmd.PrintErrf("  Warning: could not extract public key: %v\n", err)
		return 0
	}

	addrKey := hex.EncodeToString(pubKey.Address()) + ".address"

	if _, err := destKr.Get(addrKey); err == nil {
		return 0 // already exists
	}

	addrItem, err := srcKr.Get(addrKey)
	if err != nil {
		cmd.PrintErrf("  Warning: could not read %q: %v\n", addrKey, err)
		return 0
	}

	if err := destKr.Set(addrItem); err != nil {
		cmd.PrintErrf("  Warning: could not write %q: %v\n", addrKey, err)
		return 0
	}

	return 1
}

// filterKeys returns keyring entries whose key name matches any of the given names.
func filterKeys(allKeys []string, names []string) []string {
	selected := make(map[string]bool, len(names))
	for _, n := range names {
		selected[n] = true
	}

	var result []string
	for _, key := range allKeys {
		// Match exact key or bare name prefix (e.g. "alice" matches "alice.info").
		if selected[key] {
			result = append(result, key)
			continue
		}
		baseName := strings.SplitN(key, ".", 2)[0]
		if selected[baseName] {
			result = append(result, key)
		}
	}
	return result
}
