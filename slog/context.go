package slog

import (
	"context"
	"log/slog"
	"sync"
)

type loggerContextKey struct{}

func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerContextKey{}).(*slog.Logger); ok {
		return logger
	}
	nullLoggerOnce.Do(func() {
		nullLogger = slog.New(slog.DiscardHandler)
	})
	return nullLogger
}

func LoggerToContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

var (
	nullLogger     *slog.Logger
	nullLoggerOnce sync.Once
)
