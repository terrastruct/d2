package d2raster

import (
	"context"
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestEvenOddPathFillPixelsTransformsAndEffects(t *testing.T) {
	path := nestedRectanglePath(
		d2scene.Box{Width: 8, Height: 8},
		d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4},
		red,
	)
	node := d2scene.NewNode(path)
	node.Transform = d2scene.Translate(4, 3)
	node.Opacity = .5
	document := d2scene.NewDocument(d2scene.Box{Width: 16, Height: 14}, node)

	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, frame.NRGBAAt(5, 4), color.NRGBA{R: 255, A: 128})
	assertPixel(t, frame.NRGBAAt(7, 6), color.NRGBA{})
	assertPixel(t, frame.NRGBAAt(3, 4), color.NRGBA{})

	gradientPath := nestedRectanglePath(
		d2scene.Box{Width: 8, Height: 8},
		d2scene.Box{X: 3, Y: 2, Width: 2, Height: 4},
		d2scene.LinearGradient{
			Units: d2scene.UserSpaceOnUse, Transform: d2scene.Identity(),
			Start: d2scene.Point{}, End: d2scene.Point{X: 8},
			Stops: []d2scene.GradientStop{
				{Color: color.NRGBA{R: 255, A: 255}},
				{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
			},
		},
	)
	gradientFrame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, d2scene.NewNode(gradientPath)), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	left, right := gradientFrame.NRGBAAt(1, 1), gradientFrame.NRGBAAt(7, 1)
	if left.A != 255 || left.R <= left.B {
		t.Fatalf("left gradient pixel = %#v, want opaque red-dominant", left)
	}
	if right.A != 255 || right.B <= right.R {
		t.Fatalf("right gradient pixel = %#v, want opaque blue-dominant", right)
	}
	assertPixel(t, gradientFrame.NRGBAAt(3, 3), color.NRGBA{})
}

func TestEvenOddPathFillResourceBoundaries(t *testing.T) {
	path := nestedRectanglePath(
		d2scene.Box{X: 2, Y: 2, Width: 6, Height: 6},
		d2scene.Box{X: 4, Y: 4, Width: 2, Height: 2},
		red,
	)
	document := testDocument(path)
	options := testOptions()

	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	// The one-pixel antialias fringe gives an 8x8 mask. Its two rectangles
	// have eight edges, each evaluated at all sixteen samples per pixel.
	const wantWork = int64(8 * 8 * 8 * 4 * 4)
	if prepared.resources.evenOddClipWork != wantWork {
		t.Fatalf("even-odd fill work = %d, want %d", prepared.resources.evenOddClipWork, wantWork)
	}
	if prepared.resources.peakOffscreenBytes != 8*8 {
		t.Fatalf("even-odd fill offscreen bytes = %d, want 64", prepared.resources.peakOffscreenBytes)
	}

	options.MaxEvenOddClipWork = wantWork - 1
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "even-odd clip work 8192 exceeds limit 8191") {
		t.Fatalf("below-work-limit Render() error = %v", err)
	}
	options.MaxEvenOddClipWork = wantWork
	options.MaxOffscreenBytes = 8*8 - 1
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage 64 bytes exceeds limit 63") {
		t.Fatalf("below-storage-limit Render() error = %v", err)
	}
	options.MaxOffscreenBytes = 8 * 8
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive limits Render() error = %v", err)
	}
}

func TestEvenOddPathFillCancellationReleasesMask(t *testing.T) {
	path := nestedRectanglePath(
		d2scene.Box{X: 2, Y: 2, Width: 6, Height: 6},
		d2scene.Box{X: 4, Y: 4, Width: 2, Height: 2},
		red,
	)
	prepared, err := prepare(context.Background(), testDocument(path), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	scratch := &rasterScratch{offscreen: offscreenBudget{limit: testOptions().MaxOffscreenBytes}}
	ctx := &evenOddCancelContext{remaining: 3}
	err = drawPrimitive(ctx, image.NewRGBA(image.Rect(0, 0, 10, 10)), prepared.root.primitive, scratch)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("drawPrimitive() error = %v, want context.Canceled", err)
	}
	if scratch.offscreen.live != 0 {
		t.Fatalf("offscreen bytes after cancellation = %d, want 0", scratch.offscreen.live)
	}
}

func TestRetainedVectorAssetEvenOddPathFill(t *testing.T) {
	assetPath := nestedRectanglePath(
		d2scene.Box{Width: 4, Height: 4},
		d2scene.Box{X: 1, Y: 1, Width: 2, Height: 2},
		red,
	)

	t.Run("unused definition is valid and consumes no pixel work", func(t *testing.T) {
		document := d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, d2scene.NewNode(nil))
		document.Assets["icon"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 4, Height: 4},
			Root:    d2scene.NewNode(assetPath),
		}
		prepared, err := prepare(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		if prepared.resources.evenOddClipWork != 0 {
			t.Fatalf("unused retained fill work = %d, want 0", prepared.resources.evenOddClipWork)
		}
	})

	t.Run("visible instance renders and is charged", func(t *testing.T) {
		imageNode := d2scene.NewNode(d2scene.Image{Asset: "icon", Box: d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4}})
		document := d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, imageNode)
		document.Assets["icon"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 4, Height: 4},
			Root:    d2scene.NewNode(assetPath),
		}
		options := testOptions()
		const wantWork = int64(4 * 4 * 8 * 4 * 4)
		options.MaxEvenOddClipWork = wantWork - 1
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "even-odd clip work 2048 exceeds limit 2047") {
			t.Fatalf("below imported-fill limit Render() error = %v", err)
		}
		options.MaxEvenOddClipWork = wantWork
		frame, err := Render(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(2, 2), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(3, 3), color.NRGBA{})
	})

	t.Run("invalid retained rule is rejected", func(t *testing.T) {
		invalid := assetPath
		invalid.FillRule = d2scene.FillRule(255)
		document := d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, d2scene.NewNode(nil))
		document.Assets["invalid"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 4, Height: 4},
			Root:    d2scene.NewNode(invalid),
		}
		if _, err := Render(context.Background(), document, testOptions()); err == nil || !strings.Contains(err.Error(), "invalid fill rule 255") {
			t.Fatalf("invalid retained fill-rule error = %v", err)
		}
	})
}

type evenOddCancelContext struct {
	remaining int
}

func (*evenOddCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*evenOddCancelContext) Done() <-chan struct{}       { return nil }
func (*evenOddCancelContext) Value(any) any               { return nil }
func (c *evenOddCancelContext) Err() error {
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func nestedRectanglePath(outer, inner d2scene.Box, fill d2scene.Paint) d2scene.Path {
	commands := make([]d2scene.PathCommand, 0, 10)
	for _, box := range [...]d2scene.Box{outer, inner} {
		commands = append(commands,
			d2scene.MoveTo(box.X, box.Y),
			d2scene.LineTo(box.X+box.Width, box.Y),
			d2scene.LineTo(box.X+box.Width, box.Y+box.Height),
			d2scene.LineTo(box.X, box.Y+box.Height),
			d2scene.ClosePath(),
		)
	}
	return d2scene.Path{Commands: commands, FillRule: d2scene.EvenOdd, Fill: fill}
}
