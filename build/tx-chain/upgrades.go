package txchain

import "github.com/tokenize-x/tx-chain/build/tools"

// TXdUpgrades returns the mapping from upgrade name to the upgraded version.
func TXdUpgrades() map[string]string {
	return map[string]string{
		"v6": "txd",
		"v5": string(tools.CoredV500),
	}
}
