//go:build recoverylock_test

package recoverylock

import "sync"

// testBuild is true only when compiled with -tags recoverylock_test, which is
// used for tests and local test chains only. Such builds accept test values
// and the known test addresses in the embedded configuration.
const testBuild = true

// testOverride holds a configuration injected by UseTestConfig.
var testOverride struct {
	sync.Mutex
	listed, params []byte
}

// UseTestConfig makes MustConfig use the given list and params, validated with
// the test rules, until the returned restore function runs. It exists only in
// builds with the recoverylock_test tag. Tests that build the app with the
// lock enabled call it before app construction.
func UseTestConfig(listed, paramsJSON []byte) (restore func()) {
	testOverride.Lock()
	defer testOverride.Unlock()
	prevListed, prevParams := testOverride.listed, testOverride.params
	testOverride.listed, testOverride.params = listed, paramsJSON
	return func() {
		testOverride.Lock()
		defer testOverride.Unlock()
		testOverride.listed, testOverride.params = prevListed, prevParams
	}
}

func testConfigOverride() (listed, paramsJSON []byte, ok bool) {
	testOverride.Lock()
	defer testOverride.Unlock()
	if testOverride.params == nil {
		return nil, nil, false
	}
	return testOverride.listed, testOverride.params, true
}
