package platform

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// Runner is a long-lived loop such as the worker or scheduler.
type Runner interface {
	Name() string
	Run(ctx context.Context) error
}

// RunAll starts every runner and blocks until ctx is cancelled or one fails.
func RunAll(ctx context.Context, log *slog.Logger, runners ...Runner) error {
	errCh := make(chan error, len(runners))
	for _, r := range runners {
		r := r
		go func() {
			log.Info("runner started", "runner", r.Name())
			errCh <- r.Run(ctx)
		}()
	}
	var firstErr error
	for range runners {
		if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Every runs fn immediately and then on a ticker until ctx is done.
func Every(ctx context.Context, interval time.Duration, fn func(context.Context) error) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := fn(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}
