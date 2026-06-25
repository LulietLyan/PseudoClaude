package filelock

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"time"
)

type Options struct {
	Attempts int
	MinDelay time.Duration
	MaxDelay time.Duration
	StaleAge time.Duration
}

func Acquire(ctx context.Context, path string, opts Options) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opts = normalize(opts)
	for attempt := 0; attempt < opts.Attempts; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, _ = file.WriteString(time.Now().Format(time.RFC3339Nano))
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		_ = removeIfStale(path, opts.StaleAge)
		if attempt == opts.Attempts-1 {
			break
		}
		delay := jitter(opts.MinDelay, opts.MaxDelay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, os.ErrExist
}

func normalize(opts Options) Options {
	if opts.Attempts <= 0 {
		opts.Attempts = 10
	}
	if opts.MinDelay <= 0 {
		opts.MinDelay = 5 * time.Millisecond
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 100 * time.Millisecond
	}
	if opts.MaxDelay < opts.MinDelay {
		opts.MaxDelay = opts.MinDelay
	}
	if opts.StaleAge <= 0 {
		opts.StaleAge = 10 * time.Second
	}
	return opts
}

func removeIfStale(path string, staleAge time.Duration) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if time.Since(info.ModTime()) > staleAge {
		return os.Remove(path)
	}
	return nil
}

func jitter(minDelay, maxDelay time.Duration) time.Duration {
	if maxDelay <= minDelay {
		return minDelay
	}
	delta := int64(maxDelay - minDelay)
	return minDelay + time.Duration(rand.Int64N(delta+1))
}
