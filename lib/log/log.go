// Package log is a context wrapper around slog.Logger
package log

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"
	"testing"
)

var _default = slog.New(NewPrettyHandler(NewLevelHandler(slog.LevelInfo, slog.NewTextHandler(os.Stderr, nil)))).With(slog.String("logger", "default"))

func Init() {
	slog.SetDefault(_default)
}

type loggerKey struct{}

func from(ctx context.Context) *slog.Logger {
	l, ok := ctx.Value(loggerKey{}).(*slog.Logger)
	if !ok {
		_default.WarnContext(ctx, "missing slog.Logger in context, see lib/log.With", slog.String("stack", string(debug.Stack())))
		return _default
	}
	return l
}

func With(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

func WithDefault(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey{}, _default)
}

func Leveled(ctx context.Context, level slog.Level) context.Context {
	logger := from(ctx)
	handler := logger.Handler()
	leveledHandler := NewLevelHandler(level, handler)
	prettyHandler := NewPrettyHandler(leveledHandler)
	return With(ctx, slog.New(prettyHandler))
}

func WithTB(ctx context.Context, tb testing.TB) context.Context {
	writer := &tbWriter{tb: tb}
	handler := slog.NewTextHandler(writer, nil)
	logger := slog.New(handler)
	return With(ctx, logger)
}

func Debug(ctx context.Context, msg string, attrs ...slog.Attr) {
	from(ctx).LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

func Info(ctx context.Context, msg string, attrs ...slog.Attr) {
	from(ctx).LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

func Warn(ctx context.Context, msg string, attrs ...slog.Attr) {
	from(ctx).LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}

func Error(ctx context.Context, msg string, attrs ...slog.Attr) {
	from(ctx).LogAttrs(ctx, slog.LevelError, msg, attrs...)
}
