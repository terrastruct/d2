package log

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

var (
	colorReset  = "\033[0m"
	colorFaded  = "\033[2m" // Dim for faded text (timestamp)
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
)

type PrettyHandler struct {
	handler slog.Handler
}

func NewPrettyHandler(h slog.Handler) *PrettyHandler {
	return &PrettyHandler{handler: h}
}

func (h *PrettyHandler) Handle(ctx context.Context, r slog.Record) error {
	timestamp := getFadedTimestamp(r.Time)

	levelColor := getColorForLevel(r.Level)
	level := fmt.Sprintf("%s%-5s%s", levelColor, r.Level.String(), colorReset)

	var msg string
	msg += fmt.Sprintf("%s %s %s", timestamp, level, r.Message)

	r.Attrs(func(attr slog.Attr) bool {
		msg += fmt.Sprintf(" %s=%v", attr.Key, attr.Value)
		return true
	})

	fmt.Println(msg)
	return nil
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &PrettyHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	return &PrettyHandler{handler: h.handler.WithGroup(name)}
}

func getFadedTimestamp(t time.Time) string {
	return fmt.Sprintf("%s%s%s", colorFaded, t.Format(time.RFC3339), colorReset)
}

func getColorForLevel(level slog.Level) string {
	switch {
	case level == slog.LevelError:
		return colorRed
	case level == slog.LevelWarn:
		return colorYellow
	case level == slog.LevelInfo:
		return colorGreen
	case level == slog.LevelDebug:
		return colorCyan
	default:
		return colorReset
	}
}

func (h *PrettyHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}
