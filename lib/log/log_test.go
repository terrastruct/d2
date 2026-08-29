package log_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	d2log "github.com/d2lang/d2/lib/log"
)

func TestWithTBCompatibility(t *testing.T) {
	t.Parallel()

	ctx := d2log.WithTB(context.Background(), t)
	d2log.Info(ctx, "compatibility message")
}

type fakeTB struct {
	logs []string
}

func (*fakeTB) Helper() {}

func (tb *fakeTB) Log(args ...any) {
	for _, arg := range args {
		if s, ok := arg.(string); ok {
			tb.logs = append(tb.logs, s)
		}
	}
}

func TestWithTBAcceptsMinimalLogger(t *testing.T) {
	t.Parallel()

	tb := new(fakeTB)
	ctx := d2log.WithTB(context.Background(), tb)
	d2log.Info(ctx, "minimal logger")

	assert.Len(t, tb.logs, 1)
	assert.Contains(t, strings.Join(tb.logs, "\n"), "minimal logger")
}
