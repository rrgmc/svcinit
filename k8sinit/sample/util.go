package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	slog2 "github.com/rrgmc/svcinit/v3/slog"
)

var (
	initLogTime time.Time
	initLogOnce sync.Once
)

// defaultLogger is the default logger to be used internally.
func defaultLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: slog2.LevelTrace,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			initLogOnce.Do(func() {
				initLogTime = time.Now()
			})

			if a.Key == slog.TimeKey {
				t := a.Value.Time()
				return slog.String(slog.TimeKey, formatDuration(t.Sub(initLogTime)))
			} else {
				return slog2.ReplaceAttr(groups, a)
			}
		},
	}))
}

func formatDuration(d time.Duration) string {
	minute := int(d.Minutes()) % 60
	second := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d.%03d", minute, second, ms)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-time.After(duration):
		return nil
	}
}
