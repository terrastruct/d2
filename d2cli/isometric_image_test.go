package d2cli

import (
	"bytes"
	"context"
	"errors"
	"image/gif"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"
)

func imageCLI(t *testing.T, dir, source string, args ...string) ([]byte, error) {
	t.Helper()
	input := filepath.Join(dir, "input.d2")
	if err := os.WriteFile(input, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	state := &xmain.TestState{Run: Run, Args: append([]string{"d2", input}, args...), PWD: dir, Stdout: &stdout, Env: xos.NewEnv([]string{"PATH="})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	state.Start(t, ctx)
	defer state.Cleanup(t)
	err := state.Wait(ctx)
	return stdout.Bytes(), err
}

func TestIsometricImageOptionsAndFailures(t *testing.T) {
	for _, scale := range []float64{0, -2, .001, 100, math.NaN(), math.Inf(1)} {
		if _, err := isometricImageOptions(PNG, scale); err == nil {
			t.Fatalf("scale %v accepted", scale)
		}
	}
	for _, format := range []exportExtension{SVG, PNG, GIF} {
		o, err := isometricImageOptions(format, .5)
		if err != nil {
			t.Fatal(err)
		}
		if o.Width != 800 && o.Width != 500 {
			t.Fatal(o)
		}
	}
	for _, test := range []struct {
		name, source, want string
		args               []string
	}{
		{"wrong format", "a", "SVG, PNG, GIF, PDF or PPTX", []string{"--isometric", "out.txt"}},
		{"HTML filename", "a", "SVG, PNG, GIF, PDF or PPTX", []string{"--isometric", "out.html"}},
		{"SVG slideshow", "a", "--animate-interval", []string{"--isometric", "--animate-interval=100", "out.svg"}},
		{"multiple SVG stdout", "a\nlayers: {detail: {b}}", "one board", []string{"--isometric", "--stdout-format=svg", "-"}},
		{"PNG slideshow", "a", "--animate-interval", []string{"--isometric", "--animate-interval=100", "out.png"}},
		{"negative slideshow", "a", "--animate-interval", []string{"--isometric", "--animate-interval=-1", "out.gif"}},
		{"appendix", "a", "--force-appendix", []string{"--isometric", "--force-appendix", "out.png"}},
		{"tiny size", "a", "64 pixels", []string{"--isometric", "--scale=.001", "out.png"}},
		{"source sketch", "vars: {d2-config: {sketch: true}}\na", "sketch cannot", []string{"--isometric", "out.png"}},
		{"multiple PNG stdout", "a\nlayers: {detail: {b}}", "one board", []string{"--isometric", "--stdout-format=png", "-"}},
		{"board path traversal", "a\nlayers: {\"../escaped\": {b}}", "output directory", []string{"--isometric", "--scale=.1", "out.png"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "out.png")
			sentinel := []byte("previous private output")
			if err := os.WriteFile(output, sentinel, 0600); err != nil {
				t.Fatal(err)
			}
			data, err := imageCLI(t, dir, test.source, test.args...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got%v, want %q", err, test.want)
			}
			if len(data) != 0 {
				t.Fatal("failure wrote stdout")
			}
			got, err := os.ReadFile(output)
			if err != nil || !bytes.Equal(got, sentinel) {
				t.Fatalf("clobbered output %q %v", got, err)
			}
		})
	}
}

func TestIsometricImageMultiboardExports(t *testing.T) {
	dir := t.TempDir()
	source := "a\nlayers: {detail: {b}}\nscenarios: {alternate: {c}}\nsteps: {next: {d}}"
	if _, err := imageCLI(t, dir, source, "--isometric", "--scale=.1", "views.png"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.png", "layers/detail.png", "scenarios/alternate.png", "steps/next.png"} {
		data, err := os.ReadFile(filepath.Join(dir, "views", name))
		if err != nil {
			t.Fatal(err)
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Width > 160 || config.Height < 1 || config.Height > 100 {
			t.Fatalf("board %s: %+v %v", name, config, err)
		}
	}
	data, err := imageCLI(t, t.TempDir(), source, "--isometric", "--scale=.2", "--animate-interval=370", "--stdout-format=gif", "-")
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 4 || g.LoopCount != 0 {
		t.Fatalf("multiboard GIF: %d frames, loop %d", len(g.Image), g.LoopCount)
	}
	for _, delay := range g.Delay {
		if delay != 37 {
			t.Fatalf("board interval %dcs", delay)
		}
	}
	data, err = imageCLI(t, t.TempDir(), source, "--isometric", "--scale=.2", "--stdout-format=gif", "-")
	if err != nil {
		t.Fatal(err)
	}
	g, err = gif.DecodeAll(bytes.NewReader(data))
	if err != nil || len(g.Image) != 4 || g.Delay[0] != 100 {
		t.Fatalf("default multiboard interval: %v %v", g, err)
	}
}

func TestIsometricImageFolderTraversalAndCycleAdmission(t *testing.T) {
	a, b := &d2target.Diagram{Name: "a"}, &d2target.Diagram{Name: "b"}
	folder := &d2target.Diagram{IsFolderOnly: true, Layers: []*d2target.Diagram{a}, Steps: []*d2target.Diagram{b}}
	boards, err := collectGIFBoards(context.Background(), folder, 128)
	if err != nil || len(boards) != 2 || boards[0] != a || boards[1] != b {
		t.Fatalf("folder traversal: %v %v", boards, err)
	}
	a.Layers = []*d2target.Diagram{folder}
	if _, err := collectGIFBoards(context.Background(), folder, 128); err == nil {
		t.Fatal("cyclic tree admitted before export")
	}
	// An empty source root is a folder in D2: do not produce index.png.
	dir := t.TempDir()
	if _, err := imageCLI(t, dir, "layers: {detail: {b}}", "--isometric", "--scale=.1", "views.png"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "views", "index.png")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("folder-only root produced an empty board")
	}
	if _, err := os.Stat(filepath.Join(dir, "views", "detail.png")); err != nil {
		t.Fatal(err)
	}
	if _, err := imageCLI(t, dir, "layers: {detail: {b}}", "--isometric", "--scale=.2", "views.gif"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "views.gif"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil || len(g.Image) != 1 || g.Delay[0] != 100 {
		t.Fatalf("folder-only GIF should hold its one child at the requested file: %v %v", g, err)
	}
}

func TestIsometricImageAtomicReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "view.png")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := atomicIsometricImage(ctx, path, []byte("new")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "old" {
		t.Fatal("canceled write replaced old file")
	}
	if err := atomicIsometricImage(context.Background(), path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Fatal("not replaced")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("private permissions broadened")
	}
	newPath := filepath.Join(dir, "new.png")
	if err := atomicIsometricImage(context.Background(), newPath, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, _ = os.Stat(newPath)
	if info.Mode().Perm() != 0600 {
		t.Fatal("new file not private")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("temporary output leaked")
	}
}

func TestIsometricImageOptInKeepsNativePNG(t *testing.T) {
	data, err := imageCLI(t, t.TempDir(), "a", "--isometric=false", "--stdout-format=png", "-")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.DecodeConfig(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}

func TestIsometricImageNativeCLI(t *testing.T) {
	dir := t.TempDir()
	if _, err := imageCLI(t, dir, "a -> b", "--isometric", "--scale=.2", "input.png"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "input.png"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Width > 320 || config.Height < 1 || config.Height > 200 {
		t.Fatalf("native PNG %v %v", config, err)
	}
	data, err = imageCLI(t, t.TempDir(), "a\nlayers: {detail: {x -> y}}", "--isometric", "--target=layers.detail", "--scale=.2", "--stdout-format=gif", "-")
	if err != nil {
		t.Fatal(err)
	}
	animated, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(animated.Image) != 100 || animated.LoopCount != 0 || animated.Config.Width < 1 || animated.Config.Width > 200 || animated.Config.Height < 1 || animated.Config.Height > 125 {
		t.Fatal("GIF frame/canvas/loop contract")
	}
	total := 0
	for _, d := range animated.Delay {
		total += d
	}
	if total != 833 {
		t.Fatalf("GIF %dcs", total)
	}
}
