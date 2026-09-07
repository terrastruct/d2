package d2cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/util-go/xmain"
)

func TestRunReportsPartialRasterStdout(t *testing.T) {
	for _, test := range []struct {
		format    string
		prefix    string
		isometric bool
	}{
		{format: "png", prefix: "\x89PNG\r\n\x1a\n"},
		{format: "gif", prefix: "GIF89a"},
		{format: "svg", prefix: "<?xml", isometric: true},
	} {
		t.Run(test.format, func(t *testing.T) {
			directory := t.TempDir()
			inputPath := filepath.Join(directory, "input.d2")
			if err := os.WriteFile(inputPath, []byte("a\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			wantErr := errors.New("raster stdout failed")
			stdout := &controlledWriteCloser{limit: 8, writeErr: wantErr}
			args := []string{"d2", inputPath, "--stdout-format", test.format, "-"}
			if test.isometric {
				args = append(args, "--isometric")
			}
			state := &xmain.TestState{
				Run: Run, Args: args,
				PWD: directory, Stdout: stdout,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			state.Start(t, ctx)
			defer state.Cleanup(t)
			err := state.Wait(ctx)
			if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "partial render written") {
				t.Fatalf("Run() error = %v, want partial-render error wrapping %v", err, wantErr)
			}
			if !strings.HasPrefix(stdout.String(), test.prefix) {
				t.Fatalf("stdout prefix = %q, want %q", stdout.String(), test.prefix)
			}
		})
	}
}

func TestWriteStdoutReportsPartialErrorWrite(t *testing.T) {
	wantErr := errors.New("partial write failed")
	output := &controlledWriteCloser{limit: 5, writeErr: wantErr}
	written, err := writeStdout(output, []byte("0123456789"))
	if !written {
		t.Fatal("writeStdout reported no output after bytes escaped")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeStdout error = %v, want %v", err, wantErr)
	}
	if output.Len() != output.limit || output.writeCalls != 1 {
		t.Fatalf("writeStdout accepted %d bytes in %d calls, want %d bytes in one call", output.Len(), output.writeCalls, output.limit)
	}
}

func TestWriteStdoutReportsCloseErrorAfterOutput(t *testing.T) {
	wantErr := errors.New("close failed")
	output := &controlledWriteCloser{limit: -1, closeErr: wantErr}
	written, err := writeStdout(output, []byte("complete output"))
	if !written {
		t.Fatal("writeStdout reported no output after a successful write")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("writeStdout error = %v, want %v", err, wantErr)
	}
	if output.writeCalls != 1 || output.closeCalls != 1 {
		t.Fatalf("writeStdout calls = write %d, close %d; want 1, 1", output.writeCalls, output.closeCalls)
	}
}

func TestWriteStdoutReportsShortWrite(t *testing.T) {
	output := &controlledWriteCloser{limit: 3}
	written, err := writeStdout(output, []byte("short output"))
	if !written {
		t.Fatal("writeStdout reported no output after a short write")
	}
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writeStdout error = %v, want %v", err, io.ErrShortWrite)
	}
	if output.closeCalls != 0 {
		t.Fatalf("writeStdout closed output after a short write %d times, want 0", output.closeCalls)
	}
}

func TestWriteDoesNotRetryStdoutAfterPartialError(t *testing.T) {
	wantErr := errors.New("partial write failed")
	output := &controlledWriteCloser{limit: 4, writeErr: wantErr}
	err := Write(&xmain.State{Stdout: output}, "-", []byte("do not replay"))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Write error = %v, want %v", err, wantErr)
	}
	if output.writeCalls != 1 {
		t.Fatalf("Write called stdout %d times, want exactly once", output.writeCalls)
	}
}

type controlledWriteCloser struct {
	bytes.Buffer
	limit      int
	writeErr   error
	closeErr   error
	writeCalls int
	closeCalls int
}

func (w *controlledWriteCloser) Write(p []byte) (int, error) {
	w.writeCalls++
	if w.limit >= 0 && len(p) > w.limit {
		p = p[:w.limit]
	}
	n, _ := w.Buffer.Write(p)
	return n, w.writeErr
}

func (w *controlledWriteCloser) Close() error {
	w.closeCalls++
	return w.closeErr
}
