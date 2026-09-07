package d2cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/util-go/go2"
	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"
)

func isometricWatchState(t *testing.T) *xmain.State {
	t.Helper()
	env := xos.NewEnv([]string{"PATH=", "BROWSER=0"})
	opts := xmain.NewOpts(env, nil)
	if _, err := opts.Bool("", "isometric", "", false, ""); err != nil {
		t.Fatal(err)
	}
	return &xmain.State{Env: env, Opts: opts, PWD: t.TempDir()}
}

func TestIsometricWatchPreviewPreservesBoardsAndNavigation(t *testing.T) {
	root := simpleRasterDiagramWithSize(100, 80)
	root.Name = "root"
	root.Shapes[0].Link = "root.layers.first"
	first := simpleRasterDiagramWithSize(150, 80)
	first.Name = "first"
	first.Shapes[0].Link = "root"
	for i, link := range []string{"root.layers.second", "https://example.test/docs?q=a&b=c"} {
		shape := first.Shapes[0]
		shape.ID = []string{"sibling", "external"}[i]
		shape.Pos.X += (i + 1) * 180
		shape.Link = link
		first.Shapes = append(first.Shapes, shape)
	}
	second := simpleRasterDiagramWithSize(80, 150)
	second.Name = "second"
	second.Shapes[0].Link = "https://example.test/second"
	root.Layers = []*d2target.Diagram{first, second}

	state := isometricWatchState(t)
	render := d2svg.RenderOpts{Scale: go2.Pointer(.2)}
	for _, test := range []struct {
		name   string
		board  *d2target.Diagram
		folder bool
		want   []string
	}{
		{"root first", root, false, []string{"/layers/first.svg"}},
		{"selected leaf", first, false, []string{"/index.svg", "/layers/second.svg", "https://example.test/docs?q=a&b=c"}},
		{"folder first descendant", root, true, []string{"/index.svg", "/layers/second.svg", "https://example.test/docs?q=a&b=c"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root.IsFolderOnly = test.folder
			before, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			data, err := renderIsometricPreview(context.Background(), state, test.board, render, "input.d2", "root.layers.first", root)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(data, []byte("D2 isometric diagram")) {
				t.Fatal("watch preview is not native isometric SVG")
			}
			_, links := inspectIsometricSVG(t, data)
			for _, want := range test.want {
				if !containsIsometricString(links, want) {
					t.Fatalf("missing preview navigation %q: %v", want, links)
				}
			}
			if len(links) != len(test.want) {
				t.Fatalf("preview rendered more than its first drawable board: %v", links)
			}
			after, err := json.Marshal(root)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("preview relinking changed the source board tree")
			}
		})
	}
}

func TestIsometricWatchPreviewRejectsInvalidTrees(t *testing.T) {
	state := isometricWatchState(t)
	empty := &d2target.Diagram{IsFolderOnly: true}
	if _, err := renderIsometricPreview(context.Background(), state, empty, d2svg.RenderOpts{}, "input.d2", "root"); err == nil {
		t.Fatal("empty folder produced a preview")
	}
	root := simpleRasterDiagram()
	leaf := simpleRasterDiagram()
	root.Layers = []*d2target.Diagram{leaf}
	root.Steps = []*d2target.Diagram{root}
	if _, err := renderIsometricPreview(context.Background(), state, leaf, d2svg.RenderOpts{}, "input.d2", "root.layers.leaf", root); err == nil {
		t.Fatal("selected leaf bypassed full-tree cycle validation")
	}
}

func TestIsometricWatchGIFPreviewUsesStaticSingleTheme(t *testing.T) {
	state, diagram := isometricWatchState(t), simpleRasterDiagram()
	base := d2svg.RenderOpts{Scale: go2.Pointer(.2)}
	want, err := renderIsometricPreview(context.Background(), state, diagram, base, "input.d2", "root")
	if err != nil {
		t.Fatal(err)
	}
	animated := base
	animated.MasterID, animated.DarkThemeID = "gif-slideshow", go2.Pointer(int64(200))
	got, err := renderIsometricPreview(context.Background(), state, diagram, animated, "input.d2", "root")
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("GIF preview must remain the same static selected board: %v", err)
	}
	if animated.MasterID != "gif-slideshow" || animated.DarkThemeID == nil {
		t.Fatal("preview changed the file export options")
	}
}

func TestIsometricWatchCLIRecompilesAndReloadsSourceMode(t *testing.T) {
	dir := t.TempDir()
	input, output := filepath.Join(dir, "input.d2"), filepath.Join(dir, "output.svg")
	write := func(source string) {
		t.Helper()
		if err := os.WriteFile(input, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("vars: {d2-config: {isometric: true}}\na -> b\n")
	logPath := filepath.Join(dir, "watch.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	state := &xmain.TestState{
		Run: Run, PWD: dir,
		Args:   []string{"d2", "--watch", "--host=127.0.0.1", "--port=0", "--scale=.2", input, output},
		Env:    xos.NewEnv([]string{"PATH=", "BROWSER=0"}),
		Stderr: logFile,
	}
	state.Start(t, ctx)
	t.Cleanup(func() {
		cancel()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		err := state.Wait(shutdown)
		if errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("watch failed to stop after context cancellation: %v", err)
			return
		}
		state.Cleanup(t)
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Errorf("watch exited unexpectedly: %v", err)
		}
	})
	waitFor := func(previous []byte, isometric bool) []byte {
		t.Helper()
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		deadline := time.NewTimer(12 * time.Second)
		defer deadline.Stop()
		for {
			data, err := os.ReadFile(output)
			if err == nil && !bytes.Equal(data, previous) && bytes.Contains(data, []byte("</svg>")) && bytes.Contains(data, []byte("D2 isometric diagram")) == isometric {
				return data
			}
			select {
			case <-ticker.C:
			case <-deadline.C:
				log, _ := os.ReadFile(logPath)
				t.Fatalf("watch did not produce changed SVG with isometric=%t (output bytes=%d, read error=%v):\n%s", isometric, len(data), err, log)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	first := waitFor(nil, true)
	inspectIsometricSVG(t, first)
	write("vars: {d2-config: {isometric: true}}\na -> b -> c\n")
	changed := waitFor(first, true)
	inspectIsometricSVG(t, changed)
	write("vars: {d2-config: {isometric: false}}\na -> b -> c\n")
	regular := waitFor(changed, false)
	write("vars: {d2-config: {isometric: true}}\na -> b -> c\n")
	restored := waitFor(regular, true)
	if !bytes.Equal(changed, restored) {
		t.Fatal("re-enabling source isometric mode did not restore the same render")
	}
}
