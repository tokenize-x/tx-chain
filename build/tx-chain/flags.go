package txchain

import (
	"context"

	"github.com/spf13/pflag"

	"github.com/tokenize-x/tx-crust/build"
	"github.com/tokenize-x/tx-crust/build/docker"
)

type txdImageOSContextKey struct{}

// RegisterTXdOSFlag is a build.FlagRegistrar that adds the --override-txd-os flag.
// The value is resolved with precedence: CLI flag > $TXD_OS env var > default (alpine).
//
//	builder --override-txd-os ubuntu integration-tests/xrpl
//	TXD_OS=ubuntu builder integration-tests/xrpl
func RegisterTXdOSFlag(fs *pflag.FlagSet) func(context.Context) context.Context {
	fs.String("override-txd-os", string(docker.ImageOSAlpine),
		"Base OS image for the txd Docker build (alpine|ubuntu) [$TXD_OS]")
	return func(ctx context.Context) context.Context {
		value := build.ResolveFlag(fs, "override-txd-os", "TXD_OS")
		return context.WithValue(ctx, txdImageOSContextKey{}, docker.ImageOS(value))
	}
}

// TXdImageOSFromContext returns the txd base OS image selected via --override-txd-os
// or $TXD_OS, falling back to ImageOSAlpine when neither is set.
func TXdImageOSFromContext(ctx context.Context) docker.ImageOS {
	if v, ok := ctx.Value(txdImageOSContextKey{}).(docker.ImageOS); ok {
		return v
	}
	return docker.ImageOSAlpine
}
