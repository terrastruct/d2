package d2raster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestRenderWorkspaceMatchesOwnedFramesAndReusesCanvas(t *testing.T) {
	t.Parallel()
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 1,
	})
	var workspace RenderWorkspace

	tests := []struct {
		name       string
		width      int
		height     int
		background color.Color
		root       *d2scene.Node
	}{
		{
			name: "transparent content", width: 80, height: 60,
			root: d2scene.NewNode(d2scene.Rect{
				Box: d2scene.Box{X: 7, Y: 9, Width: 47, Height: 31}, RadiusX: 6, RadiusY: 6,
				Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 211, G: 83, B: 29, A: 137}},
			}),
		},
		{
			// This follows nonzero transparent pixels in the reused backing store
			// and therefore proves that a transparent frame is cleared completely.
			name: "transparent empty", width: 32, height: 24,
			root: d2scene.NewNode(nil),
		},
		{
			name: "partial background", width: 55, height: 37,
			background: color.NRGBA{R: 31, G: 71, B: 151, A: 93},
			root: d2scene.NewNode(d2scene.Ellipse{
				Center: d2scene.Point{X: 27.5, Y: 18.5}, RadiusX: 13, RadiusY: 9,
				Fill: d2scene.SolidPaint{Color: color.NRGBA{G: 217, A: 193}},
			}),
		},
		{
			name: "opaque background", width: 63, height: 41,
			background: color.NRGBA{R: 229, G: 223, B: 197, A: 255},
			root: d2scene.NewNode(d2scene.Rect{
				Box:  d2scene.Box{X: 12, Y: 8, Width: 31, Height: 19},
				Fill: d2scene.SolidPaint{Color: color.NRGBA{B: 199, A: 127}},
			}),
		},
	}

	var firstBacking *byte
	for index, test := range tests {
		document := d2scene.NewDocument(d2scene.Box{Width: float64(test.width), Height: float64(test.height)}, test.root)
		options := testOptions()
		options.Background = test.background
		owned, err := session.Render(context.Background(), document, options)
		if err != nil {
			t.Fatalf("%s owned render: %v", test.name, err)
		}
		ownedPixels := bytes.Clone(owned.Pix)

		err = workspace.Render(context.Background(), session, document, options, func(frame *image.NRGBA) error {
			if frame.Bounds() != owned.Bounds() || frame.Stride != owned.Stride {
				return fmt.Errorf("geometry = %v/%d, want %v/%d", frame.Bounds(), frame.Stride, owned.Bounds(), owned.Stride)
			}
			if !bytes.Equal(frame.Pix, owned.Pix) {
				return fmt.Errorf("pixels differ from independently owned render")
			}
			if len(frame.Pix) == 0 {
				return fmt.Errorf("empty frame backing")
			}
			if index == 0 {
				firstBacking = &frame.Pix[0]
			} else if &frame.Pix[0] != firstBacking {
				return fmt.Errorf("workspace replaced sufficient frame backing")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s workspace render: %v", test.name, err)
		}
		if !bytes.Equal(owned.Pix, ownedPixels) {
			t.Fatalf("%s workspace render aliased an owned frame", test.name)
		}
	}
}

func TestRenderWorkspaceErrorsCancellationAndRecovery(t *testing.T) {
	t.Parallel()
	document := dimensionDocument(8, 6)
	options := testOptions()
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 1,
	})
	consume := func(*image.NRGBA) error { return nil }

	var nilWorkspace *RenderWorkspace
	if err := nilWorkspace.Render(context.Background(), session, document, options, consume); err == nil || !bytes.Contains([]byte(err.Error()), []byte("nil render workspace")) {
		t.Fatalf("nil workspace error = %v", err)
	}
	var workspace RenderWorkspace
	if err := workspace.Render(context.Background(), nil, document, options, consume); err == nil || !bytes.Contains([]byte(err.Error()), []byte("nil render session")) {
		t.Fatalf("nil session error = %v", err)
	}
	if err := workspace.Render(context.Background(), session, document, options, nil); err == nil || !bytes.Contains([]byte(err.Error()), []byte("nil render workspace consumer")) {
		t.Fatalf("nil consumer error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if err := workspace.Render(canceled, session, document, options, func(*image.NRGBA) error {
		called = true
		return nil
	}); !errors.Is(err, context.Canceled) || called {
		t.Fatalf("pre-canceled render = called %v, error %v", called, err)
	}

	consumerError := errors.New("consumer failed")
	if err := workspace.Render(context.Background(), session, document, options, func(*image.NRGBA) error {
		return consumerError
	}); !errors.Is(err, consumerError) {
		t.Fatalf("consumer error = %v", err)
	}

	lateCanceled, cancelLate := context.WithCancel(context.Background())
	if err := workspace.Render(lateCanceled, session, document, options, func(*image.NRGBA) error {
		cancelLate()
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("late cancellation error = %v", err)
	}

	limited := options
	limited.MaxWidth = 7
	called = false
	if err := workspace.Render(context.Background(), session, document, limited, func(*image.NRGBA) error {
		called = true
		return nil
	}); err == nil || called {
		t.Fatalf("over-limit render = called %v, error %v", called, err)
	}

	// All failure paths must release the workspace lock and leave its canvas
	// safe for a later successful transparent render.
	if err := workspace.Render(nil, session, document, options, func(frame *image.NRGBA) error {
		if frame.Bounds() != image.Rect(0, 0, 8, 6) {
			return fmt.Errorf("recovery bounds = %v", frame.Bounds())
		}
		return nil
	}); err != nil {
		t.Fatalf("workspace recovery: %v", err)
	}
}

func TestRenderWorkspaceReusesOnlyExactRasterizerPlan(t *testing.T) {
	t.Parallel()
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 1,
	})
	var workspace RenderWorkspace
	document := d2scene.NewDocument(d2scene.Box{Width: 96, Height: 64}, d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(3, 4), d2scene.CubicTo(21, 61, 73, 3, 92, 59),
			d2scene.LineTo(7, 53), d2scene.ClosePath(),
		},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 191, G: 53, B: 109, A: 217}},
	}))
	consume := func(*image.NRGBA) error { return nil }
	if err := workspace.Render(context.Background(), session, document, testOptions(), consume); err != nil {
		t.Fatal(err)
	}
	first := workspace.rasterizer
	plan := workspace.rasterizerPlan
	if first == nil || plan.bytes <= 0 || plan.bytes > testOptions().MaxOffscreenBytes {
		t.Fatalf("initial retained rasterizer = %p, plan %+v", first, plan)
	}
	if err := workspace.Render(context.Background(), session, document, testOptions(), consume); err != nil {
		t.Fatal(err)
	}
	if workspace.rasterizer != first || workspace.rasterizerPlan != plan {
		t.Fatalf("same plan replaced rasterizer: %p/%+v, want %p/%+v", workspace.rasterizer, workspace.rasterizerPlan, first, plan)
	}

	lowerLimit := plan.bytes - 1
	if lowerLimit <= 0 {
		t.Fatalf("rasterizer plan is too small for a lower positive limit: %+v", plan)
	}
	lowerOptions := testOptions()
	lowerOptions.MaxOffscreenBytes = lowerLimit
	if err := workspace.Render(context.Background(), session, nil, lowerOptions, consume); err == nil {
		t.Fatal("invalid lower-limit render unexpectedly succeeded")
	}
	if workspace.rasterizer != nil {
		t.Fatalf("failed lower-limit render retained %d-byte prior rasterizer above limit %d", plan.bytes, lowerLimit)
	}

	empty := dimensionDocument(17, 13)
	if err := workspace.Render(context.Background(), session, empty, lowerOptions, consume); err != nil {
		t.Fatal(err)
	}
	if workspace.rasterizer != nil || workspace.rasterizerPlan.bytes != 0 {
		t.Fatalf("zero-rasterizer plan retained prior scratch: %p/%+v", workspace.rasterizer, workspace.rasterizerPlan)
	}
	workspace.Reset()
	if workspace.canvas.Pix != nil || workspace.rasterizer != nil || workspace.rasterizerPlan != (renderWorkspaceRasterizerPlan{}) {
		t.Fatal("Reset retained workspace storage")
	}
	var nilWorkspace *RenderWorkspace
	nilWorkspace.Reset()
}

func TestRenderWorkspaceRecoversExactPixelsAfterMidRasterCancellation(t *testing.T) {
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 1,
	})
	var workspace RenderWorkspace
	const width, height = 512, 512
	document := d2scene.NewDocument(d2scene.Box{Width: width, Height: height}, d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: width, Height: height},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 193, G: 71, B: 29, A: 255}},
	}))
	options := testOptions()
	options.MaxWidth, options.MaxHeight, options.MaxPixels = width, height, width*height

	var expected []byte
	countingContext := &cancelAfterContext{after: int(^uint(0) >> 1)}
	if err := workspace.Render(countingContext, session, document, options, func(frame *image.NRGBA) error {
		expected = bytes.Clone(frame.Pix)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	retained := workspace.rasterizer
	if retained == nil || countingContext.calls < 2 {
		t.Fatalf("primed rasterizer/checkpoints = %p/%d", retained, countingContext.calls)
	}

	foundMidRasterCancellation := false
	for cancelAfter := 1; cancelAfter < countingContext.calls; cancelAfter++ {
		clear(workspace.canvas.Pix)
		ctx := &cancelAfterContext{after: cancelAfter}
		called := false
		err := workspace.Render(ctx, session, document, options, func(*image.NRGBA) error {
			called = true
			return nil
		})
		if !errors.Is(err, context.Canceled) || called {
			continue
		}
		painted := 0
		for offset := 3; offset < len(workspace.canvas.Pix); offset += 4 {
			if workspace.canvas.Pix[offset] != 0 {
				painted++
			}
		}
		if painted == 0 || painted == width*height {
			continue
		}
		if workspace.rasterizer != retained {
			t.Fatalf("mid-render cancellation replaced retained rasterizer: %p, want %p", workspace.rasterizer, retained)
		}
		foundMidRasterCancellation = true
		break
	}
	if !foundMidRasterCancellation {
		t.Fatalf("no partial raster output found across %d cancellation checkpoints", countingContext.calls)
	}

	if err := workspace.Render(context.Background(), session, document, options, func(frame *image.NRGBA) error {
		if !bytes.Equal(frame.Pix, expected) {
			return fmt.Errorf("recovered pixels differ from pre-cancellation oracle")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRenderWorkspaceSerializesConcurrentCalls(t *testing.T) {
	t.Parallel()
	session := newTestRenderSession(t, RenderSessionOptions{
		MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 4,
	})
	var workspace RenderWorkspace
	var active atomic.Int32
	const renders = 16
	errorsChannel := make(chan error, renders)
	var wait sync.WaitGroup
	for index := range renders {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			marker := uint8(index*13 + 7)
			document := testDocument(d2scene.Rect{
				Box:  d2scene.Box{Width: 10, Height: 10},
				Fill: d2scene.SolidPaint{Color: color.NRGBA{R: marker, G: 91, B: 37, A: 255}},
			})
			err := workspace.Render(context.Background(), session, document, testOptions(), func(frame *image.NRGBA) error {
				if active.Add(1) != 1 {
					return fmt.Errorf("workspace callbacks overlapped")
				}
				defer active.Add(-1)
				if pixel := frame.NRGBAAt(5, 5); pixel.R != marker || pixel.A != 255 {
					return fmt.Errorf("render %d marker = %#v", index, pixel)
				}
				return nil
			})
			errorsChannel <- err
		}(index)
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func BenchmarkRenderSequentialCanvasStorage(b *testing.B) {
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{X: 20, Y: 20, Width: 180, Height: 100}, RadiusX: 12, RadiusY: 12,
			Fill: red, Stroke: &d2scene.Stroke{Paint: black, Width: 3, Cap: d2scene.CapRound, Join: d2scene.JoinRound},
		}),
		d2scene.NewNode(d2scene.Ellipse{
			Center: d2scene.Point{X: 330, Y: 70}, RadiusX: 85, RadiusY: 50,
			Fill: green, Stroke: &d2scene.Stroke{Paint: black, Width: 3, Cap: d2scene.CapRound, Join: d2scene.JoinRound},
		}),
		d2scene.NewNode(d2scene.Path{
			Commands: []d2scene.PathCommand{
				d2scene.MoveTo(110, 150), d2scene.CubicTo(175, 115, 280, 205, 350, 160),
			},
			Stroke: &d2scene.Stroke{
				Paint: blue, Width: 5, Cap: d2scene.CapRound, Join: d2scene.JoinRound,
				Dashes: []float64{12, 7}, DashOffset: 3,
			},
		}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 488, Height: 272}, root)
	options := testOptions()
	options.Scale = 2
	options.MaxWidth, options.MaxHeight, options.MaxPixels = 2_000, 2_000, 4_000_000
	ctx := context.Background()
	const frameBytes = int64(976 * 544 * 4)

	b.Run("Owned", func(b *testing.B) {
		session := newTestRenderSession(b, RenderSessionOptions{
			MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 1,
		})
		frame, err := session.Render(ctx, document, options)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkFrame = frame
		b.SetBytes(frameBytes)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			frame, err = session.Render(ctx, document, options)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkFrame = frame
		}
	})

	b.Run("Workspace", func(b *testing.B) {
		session := newTestRenderSession(b, RenderSessionOptions{
			MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 1,
		})
		var workspace RenderWorkspace
		consume := func(frame *image.NRGBA) error {
			benchmarkWorkspaceByte = frame.Pix[len(frame.Pix)/2]
			return nil
		}
		if err := workspace.Render(ctx, session, document, options, consume); err != nil {
			b.Fatal(err)
		}
		b.SetBytes(frameBytes)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if err := workspace.Render(ctx, session, document, options, consume); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkRenderSequentialWorkspaceFilters(b *testing.B) {
	root := d2scene.NewNode(nil)
	shadow := color.NRGBA{R: 24, G: 32, B: 48, A: 180}
	for index := range 8 {
		node := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{Width: 64, Height: 64},
			Fill: red,
		})
		node.Transform = d2scene.Translate(float64((index%4)*40+12), float64((index/4)*96+12))
		node.Filters = []d2scene.Filter{
			d2scene.GaussianBlur{SigmaX: 2, SigmaY: 2},
			d2scene.DropShadow{OffsetX: 3, OffsetY: 3, SigmaX: 2, SigmaY: 2, Color: shadow},
		}
		root.Children = append(root.Children, node)
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 256, Height: 256}, root)
	options := testOptions()
	ctx := context.Background()

	for _, test := range []struct {
		name      string
		workspace bool
	}{
		{name: "Owned"},
		{name: "Workspace", workspace: true},
	} {
		b.Run(test.name, func(b *testing.B) {
			session := newTestRenderSession(b, RenderSessionOptions{
				MaxCacheEntries: 1, MaxCacheBytes: 1024, MaxConcurrentLoads: 1,
			})
			var workspace RenderWorkspace
			render := func() error {
				if !test.workspace {
					frame, err := session.Render(ctx, document, options)
					benchmarkFrame = frame
					return err
				}
				return workspace.Render(ctx, session, document, options, func(frame *image.NRGBA) error {
					benchmarkWorkspaceByte = frame.Pix[len(frame.Pix)/2]
					return nil
				})
			}
			if err := render(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := render(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var benchmarkWorkspaceByte byte
