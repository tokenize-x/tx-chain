package txchain

import "github.com/tokenize-x/tx-chain/build/tools"

// TXdUpgrades returns the mapping from upgrade name to the upgraded version.
func TXdUpgrades() map[string]string {
	return map[string]string{
		"v7patch1": "txd",
		"v7":       "txd",
		"v6":       string(tools.TXdV600),
	}
}
