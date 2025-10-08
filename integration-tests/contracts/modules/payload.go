package modules

import (
	"encoding/json"

	"github.com/tokenize-x/tx-tools/pkg/must"
)

// EmptyPayload represents empty payload.
var EmptyPayload = must.Bytes(json.Marshal(struct{}{}))
