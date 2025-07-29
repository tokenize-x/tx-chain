package coreum

import "github.com/CoreumFoundation/coreum/build/tools"

// TXdUpgrades returns the mapping from upgrade name to the upgraded version.
func TXdUpgrades() map[string]string {
	return map[string]string{
		"v6": "txd",
		"v5": string(tools.CoredV500),
	}
}
