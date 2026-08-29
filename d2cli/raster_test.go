package d2cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d2lang/util-go/cmdlog"
	"github.com/d2lang/util-go/go2"
	"github.com/d2lang/util-go/xmain"
	"github.com/d2lang/util-go/xos"

	"github.com/d2lang/d2/d2plugin"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/pptx"
)

func TestPagedCLIProducesOutput(t *testing.T) {
	tests := []struct {
		ext      string
		validate func([]byte) error
	}{
		{ext: ".pdf", validate: func(data []byte) error {
			if !bytes.HasPrefix(data, []byte("%PDF-")) {
				return fmt.Errorf("missing PDF signature")
			}
			return nil
		}},
		{ext: ".pptx", validate: func(data []byte) error { return pptx.Validate(data, 1) }},
	}
	for _, test := range tests {
		t.Run(test.ext, func(t *testing.T) {
			directory := t.TempDir()
			inputPath := filepath.Join(directory, "input.d2")
			outputPath := filepath.Join(directory, "output"+test.ext)
			if err := os.WriteFile(inputPath, []byte("a -> b\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			state := &xmain.TestState{
				Run:  Run,
				Args: []string{"d2", inputPath, outputPath}, PWD: directory,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			state.Start(t, ctx)
			defer state.Cleanup(t)
			if err := state.Wait(ctx); err != nil {
				t.Fatalf("%s Run() failed: %v", test.ext, err)
			}
			output, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.validate(output); err != nil {
				t.Fatalf("%s validation: %v", test.ext, err)
			}
		})
	}
}

func TestPNGCLIProducesOutput(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input.d2")
	outputPath := filepath.Join(directory, "output.png")
	if err := os.WriteFile(inputPath, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &xmain.TestState{
		Run:  Run,
		Args: []string{"d2", inputPath, outputPath},
		PWD:  directory,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state.Start(t, ctx)
	defer state.Cleanup(t)
	if err := state.Wait(ctx); err != nil {
		t.Fatalf("PNG Run() failed: %v", err)
	}
	output, err := os.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	image, err := png.Decode(output)
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() <= 0 || image.Bounds().Dy() <= 0 {
		t.Fatalf("CLI produced empty PNG bounds %v", image.Bounds())
	}
}

func TestGIFCLIProducesOutput(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input.d2")
	outputPath := filepath.Join(directory, "output.gif")
	if err := os.WriteFile(inputPath, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &xmain.TestState{
		Run:  Run,
		Args: []string{"d2", inputPath, outputPath},
		PWD:  directory,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	state.Start(t, ctx)
	defer state.Cleanup(t)
	if err := state.Wait(ctx); err != nil {
		t.Fatalf("GIF Run() failed: %v", err)
	}
	encoded, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	animation, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(animation.Image) != 30 {
		t.Fatalf("GIF frames = %d, want 30", len(animation.Image))
	}
	var delay int
	for _, frameDelay := range animation.Delay {
		delay += frameDelay
	}
	if delay != 100 {
		t.Fatalf("GIF duration = %d centiseconds, want 100", delay)
	}
}

func TestRasterFinalizersObserveCancellationBeforeSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := runFinalizer(ctx, func() error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("finalizer cancellation = %v, want context.Canceled", err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	touched, err := runStatusFinalizer(ctx, func() (bool, error) {
		cancel()
		return true, nil
	})
	if !touched || !errors.Is(err, context.Canceled) {
		t.Fatalf("status finalizer = %v/%v, want true/context.Canceled", touched, err)
	}
}

func renderGIFFramesForTest(ctx context.Context, plugin d2plugin.Plugin, inputPath string, cacheImages bool, diagram *d2target.Diagram, opts d2svg.RenderOpts, intervalMs int, wantPreview bool) ([]image.Image, []byte, error) {
	session, err := newGIFRenderSession()
	if err != nil {
		return nil, nil, err
	}
	return renderGIFFramesWithSessionForTest(ctx, plugin, inputPath, cacheImages, diagram, opts, intervalMs, session, wantPreview)
}

func renderGIFFramesWithSessionForTest(ctx context.Context, plugin d2plugin.Plugin, inputPath string, cacheImages bool, diagram *d2target.Diagram, opts d2svg.RenderOpts, intervalMs int, session *d2raster.RenderSession, wantPreview bool) ([]image.Image, []byte, error) {
	frames := make([]image.Image, 0)
	summary, err := renderGIFWithSession(
		ctx, plugin, inputPath, cacheImages, diagram, opts, intervalMs, session, wantPreview,
		func(_ int, frame image.Image) error {
			frames = append(frames, frame)
			return nil
		},
	)
	if err != nil {
		return nil, nil, err
	}
	return frames, summary.previewSVG, nil
}

func TestRenderGIFFramesUsesSharedSamplingAndLogicalScale(t *testing.T) {
	frames, _, err := renderGIFFramesForTest(
		context.Background(),
		nil,
		"-",
		false,
		simpleRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0))},
		101,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("GIF frames = %d, want four samples for 101ms", len(frames))
	}
	for index, frame := range frames {
		if frame.Bounds().Dx() != 10 || frame.Bounds().Dy() != 10 {
			t.Fatalf("GIF frame %d bounds = %v, want 10x10 at device scale 1", index, frame.Bounds())
		}
	}
}

func TestRenderGIFBoardFramesConsumesInOrderAndStopsOnError(t *testing.T) {
	const frameCount = 8
	frameLive := false
	consumed := make([]int, 0, frameCount)
	err := renderGIFBoardFrames(
		context.Background(), 0, frameCount, frameCount,
		func(_ context.Context, frameIndex int, timestamp time.Duration, options d2raster.FrameOptions) (image.Image, error) {
			if frameLive {
				return nil, fmt.Errorf("frame %d rendered before its predecessor was consumed", frameIndex)
			}
			if want := time.Duration(frameIndex) * time.Second / 30; timestamp != want {
				return nil, fmt.Errorf("frame %d timestamp = %s, want %s", frameIndex, timestamp, want)
			}
			if options.MaxOffscreenBytes != gifMaxFrameOffscreenBytes {
				return nil, fmt.Errorf("frame %d offscreen bytes = %d, want %d", frameIndex, options.MaxOffscreenBytes, gifMaxFrameOffscreenBytes)
			}
			frameLive = true
			frame := image.NewNRGBA(image.Rect(0, 0, 1, 1))
			frame.SetNRGBA(0, 0, color.NRGBA{R: uint8(frameIndex), A: 0xff})
			return frame, nil
		},
		func(frameIndex int, candidate image.Image) error {
			frame, ok := candidate.(*image.NRGBA)
			if !ok || frame.NRGBAAt(0, 0).R != uint8(frameIndex) {
				return fmt.Errorf("frame %d marker/type = %T/%v", frameIndex, candidate, candidate.At(0, 0))
			}
			consumed = append(consumed, frameIndex)
			frameLive = false
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for frameIndex, got := range consumed {
		if got != frameIndex {
			t.Fatalf("consumed frame %d at position %d", got, frameIndex)
		}
	}

	err = renderGIFBoardFrames(
		context.Background(), 1, frameCount-1, frameCount,
		func(_ context.Context, frameIndex int, _ time.Duration, _ d2raster.FrameOptions) (image.Image, error) {
			if frameIndex == 2 {
				return nil, fmt.Errorf("indexed failure %d", frameIndex)
			}
			return image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil
		},
		func(_ int, _ image.Image) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "frame 2: indexed failure 2") {
		t.Fatalf("indexed frame error = %v", err)
	}
	if err := renderGIFBoardFrames(context.Background(), 0, 1, 1, nil, func(_ int, _ image.Image) error { return nil }); err == nil {
		t.Fatal("GIF frame scheduler accepted a nil renderer")
	}
	if err := renderGIFBoardFrames(context.Background(), 0, 1, 1, func(context.Context, int, time.Duration, d2raster.FrameOptions) (image.Image, error) {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil
	}, nil); err == nil {
		t.Fatal("GIF frame scheduler accepted a nil consumer")
	}
}

func TestRenderGIFFramesReusesBoundedAssetSession(t *testing.T) {
	session, err := d2raster.NewRenderSession(d2raster.RenderSessionOptions{
		MaxCacheEntries: gifMaxAssets, MaxCacheBytes: gifRenderCacheBytes, MaxConcurrentLoads: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	frames, _, err := renderGIFFramesWithSessionForTest(
		context.Background(), nil, "-", false, simpleRasterDiagramWithLabel(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0))}, 101, session, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("GIF frames = %d, want 4", len(frames))
	}
	stats := session.Stats()
	if stats.Misses == 0 || stats.Hits+stats.Waits == 0 || stats.MemoHits+stats.MemoWaits == 0 || stats.Hashes == 0 {
		t.Fatalf("GIF render cache did not reuse assets: %+v", stats)
	}
	if stats.SkippedOversize != 0 || stats.MemoSkipped != 0 {
		t.Fatalf("GIF render cache skipped state: %+v", stats)
	}
}

func TestRenderGIFRejectsRenderCacheAdmissionSkip(t *testing.T) {
	session, err := d2raster.NewRenderSession(d2raster.RenderSessionOptions{
		MaxCacheEntries: 1, MaxCacheBytes: 1, MaxConcurrentLoads: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = renderGIFFramesWithSessionForTest(
		context.Background(), nil, "-", false, simpleRasterDiagramWithLabel(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0))}, 101, session, false,
	)
	if err == nil || !strings.Contains(err.Error(), "render cache rejected bounded state") {
		t.Fatalf("GIF render-cache admission error = %v", err)
	}
}

func TestRenderGIFSamplesAnimatedConnectionPixels(t *testing.T) {
	frames, _, err := renderGIFFramesForTest(
		context.Background(), nil, "-", false, animatedConnectionRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0))}, 101, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 4 {
		t.Fatalf("animated-connection frames = %d, want 4", len(frames))
	}
	first, ok := frames[0].(*image.NRGBA)
	if !ok {
		t.Fatalf("animated-connection frame type = %T, want *image.NRGBA", frames[0])
	}
	changed := false
	for _, candidate := range frames[1:] {
		frame, ok := candidate.(*image.NRGBA)
		if !ok {
			t.Fatalf("animated-connection frame type = %T, want *image.NRGBA", candidate)
		}
		if frame.Bounds() != first.Bounds() {
			t.Fatalf("animated-connection bounds changed from %v to %v", first.Bounds(), frame.Bounds())
		}
		if !bytes.Equal(first.Pix, frame.Pix) {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("animated-connection frames are pixel-identical")
	}
}

func TestRenderGIFPreservesExplicitScaleAndBoardOrder(t *testing.T) {
	scaled, preview, err := renderGIFFramesForTest(
		context.Background(), nil, "-", false, simpleRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(0.5)}, 1, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(scaled) != 1 || scaled[0].Bounds().Dx() != 5 || scaled[0].Bounds().Dy() != 5 {
		t.Fatalf("scaled GIF frames/bounds = %d/%v, want 1/5x5", len(scaled), scaled[0].Bounds())
	}
	if len(preview) != 0 {
		t.Fatalf("unrequested GIF preview = %q", preview)
	}

	root := simpleRasterDiagram()
	layer := simpleRasterDiagram()
	scenario := simpleRasterDiagram()
	step := simpleRasterDiagram()
	root.Shapes[0].Fill = "#ff0000"
	layer.Shapes[0].Fill = "#00ff00"
	scenario.Shapes[0].Fill = "#0000ff"
	step.Shapes[0].Fill = "#ffff00"
	root.Layers = []*d2target.Diagram{layer}
	root.Scenarios = []*d2target.Diagram{scenario}
	root.Steps = []*d2target.Diagram{step}
	encoded, _, err := renderGIF(
		context.Background(), nil, "-", false, root,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}, 34, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	animation, err := gif.DecodeAll(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if len(animation.Image) != 8 {
		t.Fatalf("ordered GIF frames = %d, want 8", len(animation.Image))
	}
	wants := []color.NRGBA{
		{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}, {R: 255, G: 255, A: 255},
	}
	for board, want := range wants {
		got := color.NRGBAModel.Convert(animation.Image[board*2].At(5, 5)).(color.NRGBA)
		if got != want {
			t.Fatalf("GIF board %d color = %#v, want %#v", board, got, want)
		}
		delay := animation.Delay[board*2] + animation.Delay[board*2+1]
		if delay != 4 {
			t.Fatalf("GIF board %d delay = %d centiseconds, want 4", board, delay)
		}
	}
}

func TestGIFLinkBudgetDividesAggregateWork(t *testing.T) {
	one, err := gifLinkBudget(1)
	if err != nil {
		t.Fatal(err)
	}
	if one.MaxRegions != rasterMaxLinkRegions || one.MaxStringBytes != rasterMaxLinkStringBytes {
		t.Fatalf("single-board GIF link budget = %+v", one)
	}
	many, err := gifLinkBudget(gifMaxFrames)
	if err != nil {
		t.Fatal(err)
	}
	if many.MaxRegions != rasterMaxLinkRegions/gifMaxFrames || many.MaxStringBytes != rasterMaxLinkStringBytes/gifMaxFrames {
		t.Fatalf("max-board GIF link budget = %+v", many)
	}
	if _, err := gifLinkBudget(0); err == nil {
		t.Fatal("GIF link budget accepted zero boards")
	}
}

func TestRenderGIFAllowsMetadataAcrossBoards(t *testing.T) {
	root := simpleRasterDiagram()
	layer := simpleRasterDiagram()
	root.Shapes[0].Link = "https://example.com/root"
	root.Shapes[0].Tooltip = "root tooltip"
	layer.Shapes[0].Link = "https://example.com/layer"
	layer.Shapes[0].Tooltip = "layer tooltip"
	root.Layers = []*d2target.Diagram{layer}

	frames, _, err := renderGIFFramesForTest(
		context.Background(), nil, "-", false, root,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}, 1, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("GIF metadata frames = %d, want 2", len(frames))
	}
}

func TestRenderGIFReturnsRootSVGForWatchPreview(t *testing.T) {
	encoded, preview, err := renderGIF(
		context.Background(),
		nil,
		"-",
		false,
		simpleRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0))},
		1,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) == 0 || !bytes.Contains(preview, []byte("<svg")) {
		t.Fatalf("GIF encoded/preview lengths = %d/%d", len(encoded), len(preview))
	}
}

func TestRenderRasterSVGIsOptional(t *testing.T) {
	svg, err := renderRasterSVG(context.Background(), nil, nil, d2svg.RenderOpts{}, false, true)
	if err != nil || len(svg) != 0 {
		t.Fatalf("unrequested raster SVG = %q, %v", svg, err)
	}
	svg, err = renderRasterSVG(
		context.Background(), nil, simpleRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0))}, true, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(svg, []byte("<svg")) {
		t.Fatalf("requested raster SVG = %q", svg)
	}

	plugin := &pluginWithPostProcess{}
	svg, err = renderRasterSVG(
		context.Background(), plugin, simpleRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0))}, true, true,
	)
	if err == nil || !strings.Contains(err.Error(), "postprocessor") {
		t.Fatalf("mutating postprocessor error = %v", err)
	}
	if !bytes.Contains(svg, []byte("<svg")) {
		t.Fatalf("partial raster preview = %q", svg)
	}
}

func TestFolderOnlyPNGReturnsFirstBoardPreview(t *testing.T) {
	directory := t.TempDir()
	child := simpleRasterDiagram()
	child.Name = "one"
	root := &d2target.Diagram{IsFolderOnly: true, Layers: []*d2target.Diagram{child}}
	env := xos.NewEnv(nil)
	state := &xmain.State{Env: env, Log: cmdlog.NewTB(env, t), PWD: directory}

	boards, written, err := render(
		context.Background(), state, 0, nil,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		"input.d2", filepath.Join(directory, "output.png"), false, false, nil,
		root, PNG, "", true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !written {
		t.Fatal("folder-only PNG render reported no written board")
	}
	if len(boards) != 1 || !bytes.Contains(boards[0], []byte("<svg")) {
		t.Fatalf("folder-only PNG preview = %q", boards)
	}
}

func TestBundleGIFPreviewEmbedsLocalImages(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input.d2")
	if err := os.WriteFile(inputPath, []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(directory, "asset.png")
	if err := os.WriteFile(assetPath, testSolidPNG(t, color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}), 0o644); err != nil {
		t.Fatal(err)
	}
	assetURL, err := url.Parse("asset.png")
	if err != nil {
		t.Fatal(err)
	}
	diagram := rasterImageDiagram(assetURL)
	opts := d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}
	_, rawPreview, err := renderGIF(context.Background(), nil, inputPath, false, diagram, opts, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rawPreview, []byte(`href="asset.png"`)) || !bytes.Contains(rawPreview, []byte("data:image/png;base64,")) {
		t.Fatalf("GIF watch preview did not bundle local image: %s", rawPreview)
	}
}

func TestBundleGIFPreviewUsesResolvedSnapshotAndUnescapesHref(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input.d2")
	assetPath := filepath.Join(directory, "asset&one.png")
	first := testSolidPNG(t, color.NRGBA{R: 0xe1, G: 0x22, B: 0x33, A: 0xff})
	second := testSolidPNG(t, color.NRGBA{R: 0x11, G: 0x88, B: 0xcc, A: 0xff})
	if err := os.WriteFile(assetPath, first, 0o644); err != nil {
		t.Fatal(err)
	}
	assetOptions, err := gifSceneAssetOptions(inputPath, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assetOptions.Resolver.Resolve(context.Background(), "asset&one.png"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(assetPath, second, 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := bundleRasterPreview(
		context.Background(), assetOptions.Resolver,
		[]byte(`<svg><image href="asset&amp;one.png"/></svg>`),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(first)
	if !bytes.Contains(preview, []byte(want)) || bytes.Contains(preview, []byte(base64.StdEncoding.EncodeToString(second))) {
		t.Fatal("GIF preview did not preserve the resolver's validated asset snapshot")
	}
}

func TestBundleGIFPreviewBoundsReferencesAndExpandedOutput(t *testing.T) {
	assetOptions, err := gifSceneAssetOptions("-", false, 1)
	if err != nil {
		t.Fatal(err)
	}
	tooMany := []byte("<svg>" + strings.Repeat(`<image href="data:image/png;base64,AA=="/>`, rasterPreviewMaxImageReferences+1) + "</svg>")
	if _, err := bundleRasterPreview(context.Background(), assetOptions.Resolver, tooMany); err == nil || !strings.Contains(err.Error(), "image references") {
		t.Fatalf("preview image-reference limit error = %v", err)
	}

	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input.d2")
	largeSVG := `<svg xmlns="http://www.w3.org/2000/svg"><!--` + strings.Repeat("x", 60<<10) + `--><path d="M0 0"/></svg>`
	if err := os.WriteFile(filepath.Join(directory, "asset.svg"), []byte(largeSVG), 0o644); err != nil {
		t.Fatal(err)
	}
	assetOptions, err = gifSceneAssetOptions(inputPath, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	expanding := []byte("<svg>" + strings.Repeat(`<image href="asset.svg"/>`, 3_000) + "</svg>")
	if _, err := bundleRasterPreview(context.Background(), assetOptions.Resolver, expanding); err == nil || !strings.Contains(err.Error(), "expanded output bytes") {
		t.Fatalf("preview expanded-output limit error = %v", err)
	}
}

func TestRenderGIFRejectsMutatingPostProcessorAndCyclicBoards(t *testing.T) {
	plugin := &pluginWithPostProcess{}
	_, _, err := renderGIFFramesForTest(
		context.Background(),
		plugin,
		"-",
		false,
		simpleRasterDiagram(),
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		1,
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "postprocessor") || !plugin.called {
		t.Fatalf("mutating GIF postprocessor error/call = %v/%v", err, plugin.called)
	}

	cyclic := simpleRasterDiagram()
	cyclic.Layers = []*d2target.Diagram{cyclic}
	if _, err := collectGIFBoards(context.Background(), cyclic, 10); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cyclic GIF board tree error = %v", err)
	}
}

func TestPostProcessorValidationRejectsInPlaceMutation(t *testing.T) {
	plugin := &inPlacePostProcessPlugin{}
	source := []byte("<svg/>")
	wantSource := bytes.Clone(source)
	if err := validateRasterPostProcessor(context.Background(), plugin, source); err == nil || !strings.Contains(err.Error(), "postprocessor") {
		t.Fatalf("in-place postprocessor validation error = %v", err)
	}
	if !plugin.called {
		t.Fatal("in-place postprocessor was not called")
	}
	if !bytes.Equal(source, wantSource) {
		t.Fatalf("postprocessor validation let the postprocessor mutate source: %q", source)
	}
}

func TestRenderGIFRejectsCrossedNormalizedDimensions(t *testing.T) {
	wide := simpleRasterDiagramWithSize(rasterMaxDimension, 1)
	tall := simpleRasterDiagramWithSize(1, rasterMaxDimension)
	wide.Layers = []*d2target.Diagram{tall}
	_, _, err := renderGIFFramesForTest(
		context.Background(),
		nil,
		"-",
		false,
		wide,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		1,
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "normalized frame footprint") {
		t.Fatalf("crossed-dimension GIF error = %v", err)
	}
}

func TestRenderGIFAppliesFrameLimitBeforeCanvasAllocation(t *testing.T) {
	tooLarge := simpleRasterDiagramWithSize(4_096, 4_096)
	_, _, err := renderGIFFramesForTest(
		context.Background(),
		nil,
		"-",
		false,
		tooLarge,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		1,
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "GIF frame pixels exceed") || !strings.Contains(err.Error(), fmt.Sprint(gifMaxFramePixels)) {
		t.Fatalf("oversized GIF frame error = %v", err)
	}
}

func TestGIFPixelAdmissionsAcceptBoundariesAndRejectOverflow(t *testing.T) {
	exactFrame := &d2scene.Document{LogicalWidth: 2_048, LogicalHeight: 1_024}
	if _, err := gifDocumentFrameBounds(exactFrame); err != nil {
		t.Fatalf("exact frame-pixel boundary rejected: %v", err)
	}
	overFrame := &d2scene.Document{LogicalWidth: 2_049, LogicalHeight: 1_024}
	if _, err := gifDocumentFrameBounds(overFrame); err == nil {
		t.Fatal("frame-pixel boundary accepted one row-width over the limit")
	}
	fractional := &d2scene.Document{LogicalWidth: 1.5, LogicalHeight: 2.25}
	if bounds, err := gifDocumentFrameBounds(fractional); err != nil || bounds != image.Rect(0, 0, 2, 3) {
		t.Fatalf("fractional GIF frame bounds = %v/%v, want (0,0)-(2,3)", bounds, err)
	}

	// 1280 * 1024 * 128 is exactly 160 Mi-pixels.
	if err := validateGIFNormalizedFootprint(1_280, 1_024, 128); err != nil {
		t.Fatalf("exact normalized-pixel boundary rejected: %v", err)
	}
	if err := validateGIFNormalizedFootprint(1_281, 1_024, 128); err == nil {
		t.Fatal("normalized-pixel boundary accepted an over-limit canvas")
	}
	for _, dimensions := range [][3]int{{0, 1, 1}, {1, 0, 1}, {1, 1, 0}} {
		if err := validateGIFNormalizedFootprint(dimensions[0], dimensions[1], dimensions[2]); err == nil {
			t.Fatalf("normalized-pixel admission accepted %v", dimensions)
		}
	}

	invalidDocuments := []*d2scene.Document{
		nil,
		{LogicalWidth: math.NaN(), LogicalHeight: 1},
		{LogicalWidth: rasterMaxDimension + 1, LogicalHeight: 1},
	}
	for index, document := range invalidDocuments {
		if _, err := gifDocumentFrameBounds(document); err == nil {
			t.Fatalf("GIF frame admission accepted invalid document %d", index)
		}
	}
}

func TestCollectGIFBoardsBoundsFolderOnlyNodes(t *testing.T) {
	root := &d2target.Diagram{IsFolderOnly: true}
	root.Layers = make([]*d2target.Diagram, gifMaxBoardNodes)
	for index := range root.Layers {
		root.Layers[index] = &d2target.Diagram{IsFolderOnly: true}
	}
	if _, err := collectGIFBoards(context.Background(), root, 1); err == nil || !strings.Contains(err.Error(), "total nodes") {
		t.Fatalf("folder-only GIF board tree error = %v", err)
	}
}

func TestGIFSharesOperationAssetResolverAcrossBoards(t *testing.T) {
	asset := testSolidPNG(t, color.NRGBA{R: 0x45, G: 0x67, B: 0x89, A: 0xff})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(asset)
	}))
	defer server.Close()
	assetURL, err := url.Parse(server.URL + "/shared.png")
	if err != nil {
		t.Fatal(err)
	}
	root := rasterImageDiagram(assetURL)
	root.Layers = []*d2target.Diagram{rasterImageDiagram(assetURL)}
	frames, _, err := renderGIFFramesForTest(
		context.Background(),
		nil,
		"-",
		false,
		root,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		1,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || requests.Load() != 1 {
		t.Fatalf("shared GIF frames/asset requests = %d/%d, want 2/1", len(frames), requests.Load())
	}
}

func TestGIFDividesOperationWorkBudgets(t *testing.T) {
	total := svgImportBudget()
	divided, err := divideSVGImportBudget(total, 2)
	if err != nil {
		t.Fatal(err)
	}
	if divided.MaxSourceBytes != total.MaxSourceBytes/2 || divided.MaxPathCommands != total.MaxPathCommands/2 ||
		divided.MaxDeclaredResources != total.MaxDeclaredResources/2 {
		t.Fatalf("divided SVG import budget = %+v, total %+v", divided, total)
	}
	if _, err := divideSVGImportBudget(total, total.MaxDeclaredResources+1); err == nil {
		t.Fatal("SVG import budget accepted more boards than its resource domain")
	}
	options := gifFrameOptions(0, 30)
	if options.MaxPixels != gifMaxFramePixels || options.MaxNodes != rasterMaxNodes/30 ||
		options.MaxPathCommands != rasterMaxPathCommands/30 || options.MaxAssets != gifMaxAssets ||
		options.MaxTextCoverageChecks != rasterMaxTextCoverageChecks/30 ||
		options.MaxTextShapingRuns != rasterMaxTextShapingRuns/30 ||
		options.MaxAssetBytes != gifMaxAssetBytes || options.MaxDecodedAssetBytes != gifMaxDecodedAssetBytes ||
		options.MaxOffscreenBytes != gifMaxFrameOffscreenBytes ||
		options.MaxEvenOddClipWork != gifMaxEvenOddClipWork/30 ||
		options.MaxScanlineWork != gifMaxScanlineWork/30 {
		t.Fatalf("GIF frame options = %+v", options)
	}
	fonts, err := newFontFallbackOptions(4)
	if err != nil {
		t.Fatal(err)
	}
	if fonts.MaxTotalRunes != fontMaxTotalRunes/4 || fonts.MaxCoverageChecks != fontMaxCoverageChecks/4 ||
		fonts.MaxFontFacesPerText != rasterMaxFontFacesPerText ||
		fonts.MaxShapingRuns != rasterMaxTextShapingRuns/4 || fonts.MaxShapedGlyphs != rasterMaxPathCommands/4 {
		t.Fatalf("font fallback options = %+v", fonts)
	}
	underflow, err := newFontFallbackOptions(int(fontMaxCoverageChecks) + 1)
	if err != nil {
		t.Fatal(err)
	}
	if underflow.MaxTotalRunes != 1 || underflow.MaxCoverageChecks != 1 ||
		underflow.MaxShapingRuns != 1 || underflow.MaxShapedGlyphs != 1 {
		t.Fatalf("underflow-safe font fallback options = %+v", underflow)
	}
	if _, err := newFontFallbackOptions(0); err == nil {
		t.Fatal("font fallback options accepted zero boards")
	}
}

func TestRasterRejectsMutatingPostProcessorBeforeDestination(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "output.png")
	plugin := &pluginWithPostProcess{}
	_, written, err := _render(
		context.Background(),
		&xmain.State{},
		plugin,
		d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)},
		"input.d2",
		outputPath,
		true,
		false,
		nil,
		simpleRasterDiagram(),
		PNG,
		"",
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "postprocessor") || !strings.Contains(err.Error(), "disable") {
		t.Fatalf("_render() error = %v, want actionable postprocessor error", err)
	}
	if written {
		t.Fatal("postprocessor rejection reported a touched destination")
	}
	if !plugin.called {
		t.Fatal("postprocessor validation did not call the postprocessor")
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("postprocessor rejection created destination: %v", statErr)
	}
}

func TestRenderPNGPreservesDPRAndLogicalScale(t *testing.T) {
	tests := []struct {
		name  string
		scale *float64
		size  int
	}{
		{name: "default", size: 20},
		{name: "half", scale: go2.Pointer(0.5), size: 10},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := renderPNG(context.Background(), "-", false, simpleRasterDiagram(), d2svg.RenderOpts{
				Pad:   go2.Pointer(int64(0)),
				Scale: test.scale,
			})
			if err != nil {
				t.Fatal(err)
			}
			image, err := png.Decode(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			if image.Bounds().Dx() != test.size || image.Bounds().Dy() != test.size {
				t.Fatalf("PNG size = %dx%d, want %dx%d", image.Bounds().Dx(), image.Bounds().Dy(), test.size, test.size)
			}
		})
	}
}

func TestRenderPNGPreservesUniformMeetScaleAfterRounding(t *testing.T) {
	encoded, err := renderPNG(context.Background(), "-", false, simpleRasterDiagramWithSize(10, 20), d2svg.RenderOpts{
		Pad:   go2.Pointer(int64(0)),
		Scale: go2.Pointer(1.25),
	})
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 26 || image.Bounds().Dy() != 50 {
		t.Fatalf("PNG size = %dx%d, want 26x50", image.Bounds().Dx(), image.Bounds().Dy())
	}
	insideR, insideG, insideB, _ := image.At(24, 25).RGBA()
	outsideR, outsideG, outsideB, _ := image.At(25, 25).RGBA()
	if insideR <= insideG || insideR <= insideB {
		t.Fatalf("pixel inside uniformly scaled shape is not red: %v", image.At(24, 25))
	}
	if outsideR != outsideG || outsideG != outsideB {
		t.Fatalf("rounded xMinYMin meet gutter is not neutral: %v", image.At(25, 25))
	}
}

func TestRenderPNGResolvesLocalImagesRelativeToInput(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "nested", "input.d2")
	if err := os.MkdirAll(filepath.Dir(inputPath), 0o755); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Join(filepath.Dir(inputPath), "asset.png")
	if err := os.WriteFile(assetPath, testSolidPNG(t, color.NRGBA{R: 0x23, G: 0x91, B: 0xd0, A: 0xff}), 0o644); err != nil {
		t.Fatal(err)
	}
	assetURL, err := url.Parse("asset.png")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := renderPNG(context.Background(), inputPath, false, rasterImageDiagram(assetURL), d2svg.RenderOpts{
		Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPNGPixel(t, encoded, 10, 10, color.NRGBA{R: 0x23, G: 0x91, B: 0xd0, A: 0xff})
}

func TestRenderPNGUsesPlaceholderForUnavailableImage(t *testing.T) {
	valid := testSolidPNG(t, color.NRGBA{R: 0xd4, G: 0x42, B: 0x31, A: 0xff})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(valid)
	}))
	defer server.Close()
	assetURL, err := url.Parse(server.URL + "/missing.png")
	if err != nil {
		t.Fatal(err)
	}
	diagram := rasterImageDiagram(assetURL)
	diagram.Shapes[0].Width = 64
	diagram.Shapes[0].Height = 64
	encoded, err := renderPNG(context.Background(), "-", true, diagram, d2svg.RenderOpts{
		Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPNGPixel(t, encoded, 32, 56, color.NRGBA{R: 0xb8, G: 0xd7, B: 0xf2, A: 0xff})
	second, err := renderPNG(context.Background(), "-", true, diagram, d2svg.RenderOpts{
		Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPNGPixel(t, second, 32, 32, color.NRGBA{R: 0xd4, G: 0x42, B: 0x31, A: 0xff})
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want unavailable response retried once", requests.Load())
	}
}

func TestRenderPNGPreservesRemoteImageCacheBehavior(t *testing.T) {
	asset := testSolidPNG(t, color.NRGBA{R: 0xd4, G: 0x42, B: 0x31, A: 0xff})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(asset)
	}))
	assetURL, err := url.Parse(server.URL + "/asset.png")
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	diagram := rasterImageDiagram(assetURL)
	options := d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}
	first, err := renderPNG(context.Background(), "-", true, diagram, options)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	server.Close()
	second, err := renderPNG(context.Background(), "-", true, diagram, options)
	if err != nil {
		t.Fatalf("cached render after origin shutdown: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("cached remote image changed deterministic PNG bytes")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("remote requests = %d, want 1", got)
	}
}

func TestRenderPNGReusesRemoteConnectionsWithoutImageCache(t *testing.T) {
	asset := testSolidPNG(t, color.NRGBA{R: 0x19, G: 0x8b, B: 0x64, A: 0xff})
	var requests atomic.Int32
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(asset)
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.Start()
	defer server.Close()

	assetURL, err := url.Parse(server.URL + "/asset.png")
	if err != nil {
		t.Fatal(err)
	}
	diagram := rasterImageDiagram(assetURL)
	options := d2svg.RenderOpts{Pad: go2.Pointer(int64(0)), Scale: go2.Pointer(1.0)}
	for render := 0; render < 3; render++ {
		if _, err := renderPNG(context.Background(), "-", false, diagram, options); err != nil {
			t.Fatalf("uncached render %d: %v", render+1, err)
		}
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("remote requests = %d, want 3 with the image cache disabled", got)
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("remote connections = %d, want one process-owned transport connection", got)
	}
}

func TestAssetBudgetsReserveFontsAndBoundEverySVGDimension(t *testing.T) {
	if imageAssetMaxCount+fontAssetReserve != rasterMaxAssets {
		t.Fatalf("asset count budget = images %d + fonts %d, renderer %d", imageAssetMaxCount, fontAssetReserve, rasterMaxAssets)
	}
	if imageAssetMaxCumulativeEncodedBytes+fontAssetByteReserve != rasterMaxAssetBytes {
		t.Fatalf("asset byte budget = images %d + fonts %d, renderer %d", imageAssetMaxCumulativeEncodedBytes, fontAssetByteReserve, rasterMaxAssetBytes)
	}
	if fontSearchResolvedBytes != 2*fontAssetByteReserve || fontSearchResolvedBytes > fontSearchScannedBytes {
		t.Fatalf("font resolver byte budget = resolved %d retained %d scanned %d", fontSearchResolvedBytes, fontAssetByteReserve, fontSearchScannedBytes)
	}
	if fontBundledCopyBytes != 8*1024*1024 || fontBundledResolvedBytes != fontBundledCopyBytes+fontSearchResolvedBytes {
		t.Fatalf("bundled font resolver byte budget = copies %d + downstream %d = %d", fontBundledCopyBytes, fontSearchResolvedBytes, fontBundledResolvedBytes)
	}
	if gifRenderCacheBytes <= gifMaxAssetBytes+gifMaxDecodedAssetBytes+fontAssetByteReserve {
		t.Fatalf("GIF cache budget %d does not exceed assets %d + decoded %d + fonts %d", gifRenderCacheBytes, gifMaxAssetBytes, gifMaxDecodedAssetBytes, fontAssetByteReserve)
	}
	if pagedRenderCacheBytes <= rasterMaxDecodedBytes+fontAssetByteReserve+imageAssetMaxBytes {
		t.Fatalf("paged cache budget %d does not exceed decoded %d + fonts %d + image key %d", pagedRenderCacheBytes, rasterMaxDecodedBytes, fontAssetByteReserve, imageAssetMaxBytes)
	}
	options, err := sceneAssetOptions("-", false)
	if err != nil {
		t.Fatal(err)
	}
	limits := options.SVGImportLimits
	budget := options.SVGImportBudget
	if limits.MaxBytes != svgMaxBytes || limits.MaxElements != svgMaxElements || limits.MaxAttributes != svgMaxAttributes ||
		limits.MaxAttributeBytes != svgMaxAttributeBytes || limits.MaxPathCommands != svgMaxPathCommands ||
		limits.MaxTransformFunctions != svgMaxTransformFunctions || limits.MaxResources != svgMaxResources {
		t.Fatalf("per-import SVG limits = %+v", limits)
	}
	if budget.MaxSourceBytes != svgDocumentMaxSourceBytes || budget.MaxElements != svgDocumentMaxElements || budget.MaxAttributes != svgDocumentMaxAttributes ||
		budget.MaxAttributeBytes != svgDocumentMaxAttributeBytes || budget.MaxPathCommands != svgDocumentMaxPathCommands ||
		budget.MaxTransformFunctions != svgDocumentMaxTransformFunctions ||
		budget.MaxDeclaredResources != svgDocumentMaxDeclaredResources ||
		budget.MaxExpandedUseInstances != svgDocumentMaxExpandedUseInstances {
		t.Fatalf("document SVG budget = %+v", budget)
	}
	if limits.MaxBytes > budget.MaxSourceBytes || limits.MaxElements > budget.MaxElements || limits.MaxAttributes > budget.MaxAttributes ||
		limits.MaxAttributeBytes > budget.MaxAttributeBytes || limits.MaxPathCommands > budget.MaxPathCommands ||
		limits.MaxTransformFunctions > budget.MaxTransformFunctions || limits.MaxResources > budget.MaxDeclaredResources ||
		limits.MaxResources > budget.MaxExpandedUseInstances {
		t.Fatalf("per-import limits exceed document budget: limits=%+v budget=%+v", limits, budget)
	}
}

func simpleRasterDiagram() *d2target.Diagram {
	return simpleRasterDiagramWithSize(10, 10)
}

func simpleRasterDiagramWithLabel() *d2target.Diagram {
	diagram := simpleRasterDiagram()
	diagram.Shapes[0].Text = d2target.Text{
		Label: "x", FontSize: 8, FontFamily: "default", LabelWidth: 5, LabelHeight: 8,
	}
	return diagram
}

func animatedConnectionRasterDiagram() *d2target.Diagram {
	diagram := simpleRasterDiagram()
	diagram.Root.Fill = "#ffffff"
	diagram.Root.Stroke = "none"
	diagram.Shapes = []d2target.Shape{
		{ID: "a", Type: d2target.ShapeRectangle, Pos: d2target.Point{}, Width: 20, Height: 20, Fill: "#ffffff", Stroke: "none", Opacity: 1},
		{ID: "b", Type: d2target.ShapeRectangle, Pos: d2target.Point{X: 80}, Width: 20, Height: 20, Fill: "#ffffff", Stroke: "none", Opacity: 1},
	}
	diagram.Connections = []d2target.Connection{{
		ID: "a-b", Src: "a", Dst: "b", Animated: true,
		SrcArrow: d2target.NoArrowhead, DstArrow: d2target.NoArrowhead,
		Route:  []*geo.Point{{X: 20, Y: 10}, {X: 80, Y: 10}},
		Stroke: "#000000", StrokeWidth: 2, BorderRadius: 10, Opacity: 1,
	}}
	return diagram
}

type inPlacePostProcessPlugin struct {
	pluginWithoutPostProcess
	called bool
}

func (p *inPlacePostProcessPlugin) PostProcess(_ context.Context, input []byte) ([]byte, error) {
	p.called = true
	if len(input) != 0 {
		input[0] ^= 0xff
	}
	return input, nil
}

func simpleRasterDiagramWithSize(width, height int) *d2target.Diagram {
	diagram := d2target.NewDiagram()
	fontFamily := d2fonts.SourceSansPro
	monoFontFamily := d2fonts.SourceCodePro
	diagram.FontFamily = &fontFamily
	diagram.MonoFontFamily = &monoFontFamily
	diagram.Root.Fill = "#ffffff"
	diagram.Root.Stroke = "none"
	diagram.Shapes = []d2target.Shape{{
		ID:      "box",
		Type:    d2target.ShapeRectangle,
		Pos:     d2target.Point{},
		Width:   width,
		Height:  height,
		Fill:    "#ff0000",
		Stroke:  "none",
		Opacity: 1,
	}}
	return diagram
}

func rasterImageDiagram(asset *url.URL) *d2target.Diagram {
	diagram := simpleRasterDiagram()
	diagram.Shapes = []d2target.Shape{{
		ID: "image", Type: d2target.ShapeImage,
		Pos: d2target.Point{}, Width: 10, Height: 10,
		Icon: asset, Opacity: 1, Fill: "none", Stroke: "none",
	}}
	return diagram
}

func testSolidPNG(t *testing.T, fill color.NRGBA) []byte {
	t.Helper()
	image := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			image.SetNRGBA(x, y, fill)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func assertPNGPixel(t *testing.T, encoded []byte, x, y int, want color.NRGBA) {
	t.Helper()
	decoded, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	got := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA)
	if got != want {
		t.Fatalf("PNG pixel (%d,%d) = %#v, want %#v", x, y, got, want)
	}
}
