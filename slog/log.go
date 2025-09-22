package slog

import "log/slog"

const (
	LevelTrace = slog.Level(-8)
)

// ReplaceAttr is a callback for [slog.HandlerOptions.ReplaceAttr] which outputs the trace log level.
func ReplaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.LevelKey {
		level := a.Value.Any().(slog.Level)
		switch level {
		case LevelTrace:
			a.Value = slog.StringValue("TRACE")
		}
	}
	return a
}

const (
	ErrorKey = "error"
)
