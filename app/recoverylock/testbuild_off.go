//go:build !recoverylock_test

package recoverylock

// testBuild is false in every build without -tags recoverylock_test,
// including release builds and plain go test runs. The embedded configuration
// is then validated with the release rules, which refuse test values and the
// known test addresses.
const testBuild = false

// testConfigOverride never overrides the embedded configuration in a build
// without the recoverylock_test tag.
func testConfigOverride() (listed, paramsJSON []byte, ok bool) {
	return nil, nil, false
}
