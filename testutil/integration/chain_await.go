package integration

import (
	"context"
	"time"

	"github.com/tokenize-x/tx-tools/pkg/retry"
)

const (
	// DefaultAwaitStateTimeout is duration to await for account to have a specific balance.
	DefaultAwaitStateTimeout = 30 * time.Second
)

type awaitStateOptions struct {
	timeout      time.Duration
	recheckDelay time.Duration
	checkTimeout time.Duration
}

func defaultAwaitStateOptions() awaitStateOptions {
	return awaitStateOptions{
		timeout:      DefaultAwaitStateTimeout,
		recheckDelay: 100 * time.Millisecond,
		checkTimeout: 5 * time.Second,
	}
}

// AwaitStateOptionsFunc is an option for AwaitState.
type AwaitStateOptionsFunc = func(options *awaitStateOptions)

// WithAwaitStateTimeout sets the timeout for the await state.
func WithAwaitStateTimeout(timeout time.Duration) AwaitStateOptionsFunc {
	return func(options *awaitStateOptions) {
		options.timeout = timeout
	}
}

// WithAwaitStateRecheckDelay sets the recheck delay for the await state.
func WithAwaitStateRecheckDelay(recheckDelay time.Duration) AwaitStateOptionsFunc {
	return func(options *awaitStateOptions) {
		options.recheckDelay = recheckDelay
	}
}

// WithAwaitStateCheckTimeout sets the check timeout for the await state.
func WithAwaitStateCheckTimeout(checkTimeout time.Duration) AwaitStateOptionsFunc {
	return func(options *awaitStateOptions) {
		options.checkTimeout = checkTimeout
	}
}

// AwaitState waits for stateChecker function to rerun nil and retires in case of failure.
func (c ChainContext) AwaitState(
	ctx context.Context,
	stateChecker func(ctx context.Context) error,
	opts ...AwaitStateOptionsFunc) error {
	options := defaultAwaitStateOptions()
	for _, optFunc := range opts {
		optFunc(&options)
	}
	retryCtx, retryCancel := context.WithTimeout(ctx, options.timeout)
	defer retryCancel()
	err := retry.Do(retryCtx, options.recheckDelay, func() error {
		checkCtx, checkCtxCancel := context.WithTimeout(retryCtx, options.checkTimeout)
		defer checkCtxCancel()
		if err := stateChecker(checkCtx); err != nil {
			return retry.Retryable(err)
		}

		return nil
	})
	return err
}
