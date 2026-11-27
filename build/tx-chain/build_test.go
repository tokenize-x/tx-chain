package txchain

import (
	"context"
	"slices"
	"testing"

	"github.com/tokenize-x/tx-tools/pkg/logger"
)

func TestTXdVersionLinkerFlagsAreSorted(t *testing.T) {
	ctx := logger.WithLogger(context.Background(), logger.New(logger.Config{Format: "json"}))
	for range 16 {
		flags, err := txdVersionLDFlags(ctx, defaultBuildTags)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.IsSorted(flags) {
			t.Fatalf("version linker flags must be sorted: %v", flags)
		}
	}
}
