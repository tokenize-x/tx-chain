package app_test

import (
	"fmt"
	"testing"
	"time"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	wasmvm "github.com/CosmWasm/wasmvm/v2"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	"github.com/tokenize-x/tx-chain/v8/app"
	"github.com/tokenize-x/tx-chain/v8/testutil/simapp"
)

// TestLibwasmvmVersion ensures txd links the libwasmvm carrying the CosmWasm (Wasmer) security fix.
func TestLibwasmvmVersion(t *testing.T) {
	version, err := wasmvm.LibwasmvmVersion()
	require.NoError(t, err)
	require.Equal(t, "2.3.5", version)
}

// TestWasmMaxFunctionLocals ensures stored code is validated against the raised per-function locals limit.
func TestWasmMaxFunctionLocals(t *testing.T) {
	testApp := simapp.New()
	ctx := testApp.NewContextLegacy(false, tmproto.Header{Time: time.Now()})
	creator, _ := testApp.GenAccount(ctx)

	_, _, err := testApp.WasmPermissionedKeeper.Create(
		ctx, creator, wasmWithFunctionLocals(t, app.MaxWasmFunctionLocals), &wasmtypes.AllowEverybody,
	)
	require.NoError(t, err)

	_, _, err = testApp.WasmPermissionedKeeper.Create(
		ctx, creator, wasmWithFunctionLocals(t, app.MaxWasmFunctionLocals+1), &wasmtypes.AllowEverybody,
	)
	require.ErrorContains(t, err, fmt.Sprintf(
		"more than %d locals: %d", app.MaxWasmFunctionLocals, app.MaxWasmFunctionLocals+1,
	))
}

// wasmWithFunctionLocals builds the smallest module passing CosmWasm static validation.
// It has one extra function declaring the given number of i32 locals.
func wasmWithFunctionLocals(t *testing.T, locals uint32) []byte {
	t.Helper()
	// Keeps the LEB128 encoding of the locals count a single byte.
	require.Less(t, locals, uint32(128))

	section := func(id byte, content ...byte) []byte {
		return append([]byte{id, byte(len(content))}, content...)
	}
	name := func(s string) []byte {
		return append([]byte{byte(len(s))}, s...)
	}

	wasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	// Types: 0 = (i32) -> i32, 1 = (i32) -> (), 2 = () -> ().
	wasm = append(wasm, section(0x01,
		0x03,
		0x60, 0x01, 0x7f, 0x01, 0x7f,
		0x60, 0x01, 0x7f, 0x00,
		0x60, 0x00, 0x00,
	)...)
	// Functions: allocate, deallocate, interface_version_8, locals holder.
	wasm = append(wasm, section(0x03, 0x04, 0x00, 0x01, 0x02, 0x02)...)
	// One memory with 1 initial page and no maximum.
	wasm = append(wasm, section(0x05, 0x01, 0x00, 0x01)...)

	exports := []byte{0x04}
	exports = append(append(exports, name("memory")...), 0x02, 0x00)
	exports = append(append(exports, name("allocate")...), 0x00, 0x00)
	exports = append(append(exports, name("deallocate")...), 0x00, 0x01)
	exports = append(append(exports, name("interface_version_8")...), 0x00, 0x02)
	wasm = append(wasm, section(0x07, exports...)...)

	wasm = append(wasm, section(0x0a,
		0x04,
		0x04, 0x00, 0x20, 0x00, 0x0b, // allocate: local.get 0
		0x02, 0x00, 0x0b, // deallocate
		0x02, 0x00, 0x0b, // interface_version_8
		0x04, 0x01, byte(locals), 0x7f, 0x0b, // one group of `locals` i32 locals
	)...)

	return wasm
}
