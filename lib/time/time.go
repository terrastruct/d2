package time

import (
	"context"
	"time"

	"oss.terrastruct.com/d2/lib/env"
)

func HumanDate(t time.Time) string {
	local := t.Local()
	return local.Format(time.RFC822)
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
