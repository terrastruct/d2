// Package log is a context wrapper around slog.Logger
package log

import (
	"context"
	"log"
	stdlog "log"
	"os"
	"runtime/debug"
	"testing"
	"time"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogtest"

	"oss.terrastruct.com/d2/lib/env"
)

var _default = slog.Make(sloghuman.Sink(os.Stderr)).Named("default")

func init() {
	stdlib := slog.Stdlib(context.Background(), _default, slog.LevelInfo)
	log.SetOutput(stdlib.Writer())
}

type loggerKey struct{}

func from(ctx context.Context) slog.Logger {
	l, ok := ctx.Value(loggerKey{}).(slog.Logger)
	if !ok {
		_default.Warn(ctx, "missing slog.Logger in context, see lib/log.With", slog.F("stack", string(debug.Stack())))
		return _default
	}
	return l
}

func With(ctx context.Context, l slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// WithTB calls With with the result of slogtest.Make.
func WithTB(ctx context.Context, t testing.TB, opts *slogtest.Options) context.Context {
	l := slogtest.Make(t, opts)
	if env.Debug() {
		l = l.Leveled(slog.LevelDebug)
	}
	return With(ctx, l)
}

func Debug(ctx context.Context, msg string, fields ...slog.Field) {
	slog.Helper()
	from(ctx).Debug(ctx, msg, fields...)
}

func Info(ctx context.Context, msg string, fields ...slog.Field) {
	slog.Helper()
	from(ctx).Info(ctx, msg, fields...)
}

func Warn(ctx context.Context, msg string, fields ...slog.Field) {
	slog.Helper()
	from(ctx).Warn(ctx, msg, fields...)
}

func Error(ctx context.Context, msg string, fields ...slog.Field) {
	slog.Helper()
	from(ctx).Error(ctx, msg, fields...)
}

func Critical(ctx context.Context, msg string, fields ...slog.Field) {
	slog.Helper()
	from(ctx).Critical(ctx, msg, fields...)
}

func Fatal(ctx context.Context, msg string, fields ...slog.Field) {
	slog.Helper()
	from(ctx).Fatal(ctx, msg, fields...)
}

func Named(ctx context.Context, name string) context.Context {
	return With(ctx, from(ctx).Named(name))
}

func Leveled(ctx context.Context, level slog.Level) context.Context {
	return With(ctx, from(ctx).Leveled(level))
}

func AppendSinks(ctx context.Context, s ...slog.Sink) context.Context {
	return With(ctx, from(ctx).AppendSinks(s...))
}

func Sync(ctx context.Context) {
	from(ctx).Sync()
}

func Stdlib(ctx context.Context, level slog.Level) *log.Logger {
	return slog.Stdlib(ctx, from(ctx), level)
}

func Fork(ctx, loggerCtx context.Context) context.Context {
	return With(ctx, from(loggerCtx))
}

func Stderr(ctx context.Context) context.Context {
	l := slog.Make(sloghuman.Sink(os.Stderr))
	if os.Getenv("DEBUG") == "1" {
		l = l.Leveled(slog.LevelDebug)
	}

	sl := slog.Stdlib(ctx, l, slog.LevelInfo)
	stdlog.SetOutput(sl.Writer())

	return With(ctx, l)
}

// WithTimeout returns context.WithTimeout(ctx, timeout) but timeout is overridden with D2_TIMEOUT if set
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	t := timeout
	if seconds, has := env.Timeout(); has {
		t = time.Duration(seconds) * time.Second
	}
	if t <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, t)
}
