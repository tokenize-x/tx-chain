//go:build integrationtests

package ibc

import (
	_ "embed"
	"encoding/json"

	"github.com/tokenize-x/tx-tools/pkg/must"
)

// Smart contracts bytecode.
var (
	//go:embed ibc-transfer/artifacts/ibc_transfer.wasm
	IBCTransferWASM []byte
	//go:embed ibc-call/artifacts/ibc_call.wasm
	IBCCallWASM []byte
	//go:embed ibc-hooks-counter/artifacts/ibc_hooks_counter.wasm
	IBCHooksCounter []byte
	//go:embed ibc-callbacks-counter/artifacts/ibc_callbacks_counter.wasm
	IBCCallbacksCounter []byte
)

// EmptyPayload represents empty payload.
var EmptyPayload = must.Bytes(json.Marshal(struct{}{}))
