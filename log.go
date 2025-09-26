package svcinit

import (
	"io"
	"log/slog"
	"sync"

	slog2 "github.com/rrgmc/svcinit/v3/slog"
)

// defaultLogger is the default logger to be used internally.
func defaultLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level:       slog2.LevelTrace,
		ReplaceAttr: slog2.ReplaceAttr,
	}))
}

var (
	nullLoggerDefault *slog.Logger
	nullLoggerOnce    sync.Once
)

func nullLogger() *slog.Logger {
	nullLoggerOnce.Do(func() {
		nullLoggerDefault = slog.New(slog.DiscardHandler)
	})
	return nullLoggerDefault
}
