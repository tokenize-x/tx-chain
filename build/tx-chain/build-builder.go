package txchain

import (
	"context"
	"os"
	"path/filepath"

	"github.com/tokenize-x/tx-tools/pkg/must"

	"github.com/tokenize-x/crust/build/golang"
	"github.com/tokenize-x/crust/build/tools"
	"github.com/tokenize-x/crust/build/types"
)

// BuildBuilder builds building tool in the current repository.
func BuildBuilder(ctx context.Context, deps types.DepsFunc) error {
	return golang.Build(ctx, deps, golang.BinaryBuildConfig{
		TargetPlatform: tools.TargetPlatformLocal,
		PackagePath:    "build/cmd/builder",
		BinOutputPath:  must.String(filepath.EvalSymlinks(must.String(os.Executable()))),
		CGOEnabled:     true,
	})
}
