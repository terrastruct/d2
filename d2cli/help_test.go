package d2cli

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"
)

func TestLayoutCmdRejectsExtraArguments(t *testing.T) {
	opts := xmain.NewOpts(xos.NewEnv(nil), []string{"layout", "dagre", "info"})
	if err := opts.Flags.Parse(opts.Args); err != nil {
		t.Fatal(err)
	}

	err := layoutCmd(context.Background(), &xmain.State{Opts: opts}, nil)
	var usageErr xmain.UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("layoutCmd() error = %v, want xmain.UsageError", err)
	}
	const want = "bad usage: layout subcommand accepts at most one argument"
	if err.Error() != want {
		t.Fatalf("layoutCmd() error = %q, want %q", err, want)
	}
}
