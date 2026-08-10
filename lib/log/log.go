// Package log is a context wrapper around slog.Logger
package log

import (
	"context"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/d2lang/d2/internal/testlog"
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

// WithTB returns a context whose logger writes to tb.
//
// Deprecated: WithTB is a test-only convenience helper and will be removed
// after one compatibility release. Downstream tests should create an slog
// logger backed by their test logger and pass it to With.
func WithTB(ctx context.Context, tb interface {
	Helper()
	Log(args ...any)
}) context.Context {
	return With(ctx, testlog.New(tb))
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
