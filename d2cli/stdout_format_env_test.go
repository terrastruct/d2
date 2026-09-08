package d2cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"
)

func TestRunStdoutFormatEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name      string
		env       string
		flags     []string
		wantPDF   bool
		wantError string
	}{
		{name: "environment_selects_pdf", env: "pdf", wantPDF: true},
		{name: "flag_overrides_environment", env: "pdf", flags: []string{"--stdout-format=svg"}},
		{name: "flag_overrides_invalid_environment", env: "not-a-format", flags: []string{"--stdout-format=svg"}},
		{name: "empty_environment_preserves_default"},
		{name: "invalid_environment_is_reported", env: "not-a-format", wantError: "not-a-format is not a supported format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			inputPath := filepath.Join(dir, "input.d2")
			if err := os.WriteFile(inputPath, []byte("a -> b\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			env := xos.NewEnv(nil)
			env.Setenv("D2_STDOUT_FORMAT", tc.env)
			stdout := &bytes.Buffer{}
			args := append([]string{"d2"}, tc.flags...)
			state := &xmain.TestState{
				Run: Run, Env: env, Args: append(args, inputPath, "-"),
				PWD: dir, Stdout: stdout,
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			state.Start(t, ctx)
			defer state.Cleanup(t)
			err := state.Wait(ctx)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("Run() error = %v, want %q", err, tc.wantError)
				}
				if stdout.Len() != 0 {
					t.Fatalf("invalid format wrote %d bytes to stdout", stdout.Len())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantPDF {
				if !bytes.HasPrefix(stdout.Bytes(), []byte("%PDF-")) {
					t.Fatal("expected PDF output on stdout")
				}
			} else if !bytes.Contains(stdout.Bytes(), []byte("<svg")) {
				t.Fatal("expected SVG output on stdout")
			}
		})
	}
}
