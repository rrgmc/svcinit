package svcinit

import (
	"io"
	"log/slog"

	slog2 "github.com/rrgmc/svcinit/v3/slog"
)

// defaultLogger is the default logger to be used internally.
func defaultLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{
		Level:       slog2.LevelTrace,
		ReplaceAttr: slog2.ReplaceAttr,
	}))
}

// nullLogger is a logger that discards all output.
var nullLogger = slog.New(slog.DiscardHandler)
