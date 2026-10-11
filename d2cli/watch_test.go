package d2cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/d2lang/d2/lib/localfile"
)

func TestTrackedFSHonorsLocalFilePolicy(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside.d2")
	if err := os.WriteFile(inside, []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideRoot := t.TempDir()
	outside := filepath.Join(outsideRoot, "outside.d2")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}

	rooted, err := localfile.Rooted(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rooted.Close() })
	tracked := trackedFS{localFiles: rooted}
	file, err := tracked.Open(inside)
	if err != nil {
		t.Fatalf("open rooted regular file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if len(tracked.opened) != 1 || tracked.opened[0] != inside {
		t.Fatalf("tracked opens = %q, want [%q]", tracked.opened, inside)
	}

	if _, err := tracked.Open(outside); !errors.Is(err, localfile.ErrDenied) {
		t.Fatalf("outside-root Open error = %v, want localfile.ErrDenied", err)
	}
	if _, err := tracked.Open(root); err == nil {
		t.Fatal("trackedFS accepted a directory")
	}
	if len(tracked.opened) != 1 {
		t.Fatalf("failed opens were tracked: %q", tracked.opened)
	}

	var denied trackedFS
	if _, err := denied.Open(inside); !errors.Is(err, localfile.ErrDenied) {
		t.Fatalf("zero-policy Open error = %v, want localfile.ErrDenied", err)
	}

	trusted := trackedFS{localFiles: localfile.Unrestricted()}
	file, err = trusted.Open(outside)
	if err != nil {
		t.Fatalf("trusted CLI Open error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if len(trusted.opened) != 1 || trusted.opened[0] != outside {
		t.Fatalf("trusted tracked opens = %q, want [%q]", trusted.opened, outside)
	}
}

// TestHandleRootInvalidatesStaleResultOnBoardNavigation guards against a
// regression where navigating between boards in watch mode (e.g. descending
// into a layer, then returning to the parent board) could momentarily hand a
// newly-connecting websocket client the previous board's cached SVG. The
// frontend locks in its fit-to-window scale from the first message it
// receives, so a stale cross-board SVG left the diagram mis-scaled even
// after the correct one arrived.
func TestHandleRootInvalidatesStaleResultOnBoardNavigation(t *testing.T) {
	w := &watcher{compileCh: make(chan struct{}, 1)}
	w.boardPath = "old"
	w.res = &compileResult{SVG: "<svg>old board</svg>"}

	req := httptest.NewRequest(http.MethodGet, "/new", nil)
	w.handleRoot(httptest.NewRecorder(), req)

	if w.boardPath != "new" {
		t.Fatalf("boardPath = %q, want %q", w.boardPath, "new")
	}
	if got := w.getRes(); got != nil {
		t.Fatalf("getRes() = %+v, want nil after navigating to a different board", got)
	}
}

// TestHandleRootKeepsResultWhenBoardUnchanged ensures the invalidation above
// doesn't also drop the cached result on a plain reload of the same board,
// which would cause the diagram to flicker blank while it recompiles.
func TestHandleRootKeepsResultWhenBoardUnchanged(t *testing.T) {
	w := &watcher{compileCh: make(chan struct{}, 1)}
	w.boardPath = "same"
	res := &compileResult{SVG: "<svg>same board</svg>"}
	w.res = res

	req := httptest.NewRequest(http.MethodGet, "/same", nil)
	w.handleRoot(httptest.NewRecorder(), req)

	if got := w.getRes(); got != res {
		t.Fatalf("getRes() = %+v, want unchanged cached result %+v", got, res)
	}
}

// TestBoardStillCurrentGuardsInFlightCompile covers the remaining half of the
// stale-result race: a compile for the board being navigated away from can
// still be running when handleRoot clears w.res for the new board. Without
// this guard, compileLoop would broadcast that in-flight compile's result
// after the clear, repopulating w.res with the wrong board's SVG.
func TestBoardStillCurrentGuardsInFlightCompile(t *testing.T) {
	w := &watcher{}
	w.boardPath = "top"

	if !w.boardStillCurrent("top") {
		t.Fatal("boardStillCurrent(\"top\") = false, want true before any navigation")
	}

	// Simulate the user navigating away while a compile for "top" is in flight.
	w.boardPath = "top.nested"

	if w.boardStillCurrent("top") {
		t.Fatal("boardStillCurrent(\"top\") = true after navigating to \"top.nested\", want false")
	}
	if !w.boardStillCurrent("top.nested") {
		t.Fatal("boardStillCurrent(\"top.nested\") = false, want true for the newly-selected board")
	}
}
