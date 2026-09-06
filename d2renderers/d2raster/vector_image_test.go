package d2raster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestVectorImageTwoStageMappingViewBoxAndPaintOrder(t *testing.T) {
	t.Run("import viewport then image viewport", func(t *testing.T) {
		content := d2scene.NewNode(nil)
		content.Transform = d2scene.Scale(2, 2)
		content.Children = []*d2scene.Node{
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: red}),
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 10, Width: 10, Height: 10}, Fill: green}),
		}
		assetRoot := d2scene.NewNode(nil)
		assetRoot.Clip = &d2scene.Clip{Path: clipRect(0, 0, 40, 20, d2scene.NonZero), Transform: d2scene.Identity()}
		assetRoot.Children = []*d2scene.Node{content}

		imageNode := d2scene.NewNode(d2scene.Image{
			Asset: "vector",
			Box:   d2scene.Box{X: 10, Y: 10, Width: 100, Height: 100},
			Aspect: d2scene.AspectRatio{
				Align: d2scene.AlignXMidYMid,
				Fit:   d2scene.AspectMeet,
			},
		})
		// Children of an Image node paint after the image primitive.
		imageNode.Children = []*d2scene.Node{
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 40, Y: 45, Width: 20, Height: 20}, Fill: blue}),
		}
		document := d2scene.NewDocument(d2scene.Box{Width: 120, Height: 120}, imageNode)
		document.Assets["vector"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 40, Height: 20},
			Root:    assetRoot,
		}

		frame, err := Render(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(20, 40), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(90, 40), color.NRGBA{G: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(45, 50), color.NRGBA{B: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(50, 20), color.NRGBA{})
		assertPixel(t, frame.NRGBAAt(50, 90), color.NRGBA{})
	})

	t.Run("nonzero source origin", func(t *testing.T) {
		assetRoot := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{X: 10, Y: 20, Width: 20, Height: 10},
			Fill: red,
		})
		imageNode := d2scene.NewNode(d2scene.Image{
			Asset: "offset",
			Box:   d2scene.Box{X: 5, Y: 7, Width: 40, Height: 20},
			Aspect: d2scene.AspectRatio{
				Align: d2scene.AlignNone,
			},
		})
		document := d2scene.NewDocument(d2scene.Box{Width: 50, Height: 35}, imageNode)
		document.Assets["offset"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{X: 10, Y: 20, Width: 20, Height: 10},
			Root:    assetRoot,
		}
		frame, err := Render(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		assertPixel(t, frame.NRGBAAt(6, 8), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(44, 26), color.NRGBA{R: 255, A: 255})
		assertPixel(t, frame.NRGBAAt(4, 8), color.NRGBA{})
		assertPixel(t, frame.NRGBAAt(45, 8), color.NRGBA{})
	})
}

func TestVectorImageSliceOuterClipAndAffineTransform(t *testing.T) {
	left := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	right := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 1, Width: 1, Height: 1}, Fill: blue})
	assetRoot := d2scene.NewNode(nil)
	assetRoot.Children = []*d2scene.Node{left, right}

	renderSlice := func(t *testing.T, align d2scene.AspectAlign) *image.NRGBA {
		t.Helper()
		imageNode := d2scene.NewNode(d2scene.Image{
			Asset: "wide",
			Box:   d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4},
			Aspect: d2scene.AspectRatio{
				Align: align,
				Fit:   d2scene.AspectSlice,
			},
		})
		document := d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, imageNode)
		document.Assets["wide"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 2, Height: 1}, Root: assetRoot}
		frame, err := Render(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		return frame
	}
	minSlice := renderSlice(t, d2scene.AlignXMinYMin)
	maxSlice := renderSlice(t, d2scene.AlignXMaxYMin)
	assertPixel(t, minSlice.NRGBAAt(4, 4), color.NRGBA{R: 255, A: 255})
	assertPixel(t, maxSlice.NRGBAAt(3, 4), color.NRGBA{B: 255, A: 255})
	for _, point := range []image.Point{{X: 1, Y: 3}, {X: 6, Y: 3}, {X: 3, Y: 1}, {X: 3, Y: 6}} {
		assertPixel(t, minSlice.NRGBAAt(point.X, point.Y), color.NRGBA{})
		assertPixel(t, maxSlice.NRGBAAt(point.X, point.Y), color.NRGBA{})
	}

	full := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: red})
	imageNode := d2scene.NewNode(d2scene.Image{Asset: "square", Box: d2scene.Box{Width: 4, Height: 4}})
	imageNode.Transform = d2scene.Matrix{A: 1, C: 1, D: 1, E: 2, F: 2}
	document := d2scene.NewDocument(d2scene.Box{Width: 12, Height: 9}, imageNode)
	document.Assets["square"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 4, Height: 4}, Root: full}
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, frame.NRGBAAt(6, 4), color.NRGBA{R: 255, A: 255})
	// This point is inside the transformed box's axis-aligned bounds but
	// outside its sheared viewport polygon.
	assertPixel(t, frame.NRGBAAt(2, 5), color.NRGBA{})
}

func TestVectorImageCounterScaledExtremeTransformKeepsViewport(t *testing.T) {
	assetRoot := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 1, Height: 1}, Fill: red,
	})
	tests := []struct {
		name      string
		box       d2scene.Box
		transform d2scene.Matrix
	}{
		{
			name:      "diagonal",
			box:       d2scene.Box{Width: 1e-300, Height: 1e300},
			transform: d2scene.Matrix{A: 1e300, D: 1e-300},
		},
		{
			name:      "off-diagonal reflection",
			box:       d2scene.Box{Width: 1e300, Height: 1e-300},
			transform: d2scene.Matrix{B: 1e-300, C: 1e300},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := d2scene.NewNode(d2scene.Image{
				Asset: "counter-scaled", Box: test.box,
				Aspect: d2scene.AspectRatio{Align: d2scene.AlignNone},
			})
			host.Transform = test.transform
			document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, host)
			document.Assets["counter-scaled"] = d2scene.VectorAsset{
				ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot,
			}
			frame, err := Render(context.Background(), document, testOptions())
			if err != nil {
				t.Fatalf("counter-scaled vector image: %v", err)
			}
			assertPixel(t, frame.NRGBAAt(0, 0), color.NRGBA{R: 255, A: 255})
		})
	}
}

func TestVectorImageSingularTransformIsTransparentBeforeGradientPreparation(t *testing.T) {
	assetRoot := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: 1, Height: 1},
		Fill: retainedTestLinearGradient(d2scene.Identity(), d2scene.UserSpaceOnUse),
	})
	host := d2scene.NewNode(d2scene.Image{
		Asset: "singular", Box: d2scene.Box{Width: 1, Height: 1},
	})
	host.Transform = d2scene.Scale(0, 1)
	document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, host)
	document.Assets["singular"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot,
	}
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatalf("singular vector image gradient: %v", err)
	}
	assertPixel(t, frame.NRGBAAt(0, 0), color.NRGBA{})
}

func TestVectorImageExtremeGradientTransformsRemainRenderable(t *testing.T) {
	transforms := map[string]d2scene.Matrix{
		"large": {A: 1e300, D: 1e300},
		"small": {A: 1e-300, D: 1e-300},
	}
	for name, transform := range transforms {
		t.Run(name, func(t *testing.T) {
			assetRoot := d2scene.NewNode(d2scene.Rect{
				Box:  d2scene.Box{Width: 1, Height: 1},
				Fill: retainedTestLinearGradient(transform, d2scene.UserSpaceOnUse),
			})
			document := vectorImageDocument("gradient", d2scene.Box{Width: 1, Height: 1})
			document.Assets["gradient"] = d2scene.VectorAsset{
				ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot,
			}
			frame, err := Render(context.Background(), document, testOptions())
			if err != nil {
				t.Fatalf("extreme visible gradient transform: %v", err)
			}
			if pixel := frame.NRGBAAt(0, 0); pixel.A != 255 {
				t.Fatalf("extreme visible gradient pixel = %#v, want opaque", pixel)
			}
		})
	}
}

func TestVectorImageRejectsSingularNonemptyAspectMappingBeforePaint(t *testing.T) {
	paints := map[string]d2scene.Paint{
		"solid":    red,
		"gradient": retainedTestLinearGradient(d2scene.Identity(), d2scene.UserSpaceOnUse),
	}
	for name, paint := range paints {
		t.Run(name, func(t *testing.T) {
			assetRoot := d2scene.NewNode(d2scene.Rect{
				Box: d2scene.Box{Width: 1e300, Height: 1}, Fill: paint,
			})
			host := d2scene.NewNode(d2scene.Image{
				Asset: "underflow", Box: d2scene.Box{Width: 1e-300, Height: 1},
				Aspect: d2scene.AspectRatio{Align: d2scene.AlignNone},
			})
			document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, host)
			document.Assets["underflow"] = d2scene.VectorAsset{
				ViewBox: d2scene.Box{Width: 1e300, Height: 1}, Root: assetRoot,
			}
			_, err := Render(context.Background(), document, testOptions())
			if err == nil || !strings.Contains(err.Error(), "aspect ratio: mapping is singular in the finite numeric domain") {
				t.Fatalf("singular visible aspect mapping error = %v", err)
			}
		})
	}
}

func TestVectorImageNestedAssetsEffectsAndDeterminism(t *testing.T) {
	raster := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	raster.SetNRGBA(0, 0, color.NRGBA{G: 255, A: 255})

	rasterImage := d2scene.NewNode(d2scene.Image{Asset: "pixel", Box: d2scene.Box{Width: 2, Height: 2}})
	innerRoot := d2scene.NewNode(nil)
	innerRoot.Children = []*d2scene.Node{rasterImage}
	outerRoot := d2scene.NewNode(d2scene.Image{Asset: "inner", Box: d2scene.Box{Width: 4, Height: 4}})
	outerRoot.Opacity = .5
	outerRoot.Clip = &d2scene.Clip{Path: clipRect(0, 0, 4, 2, d2scene.NonZero), Transform: d2scene.Identity()}
	outerRoot.Mask = &d2scene.Mask{
		Type: d2scene.MaskAlpha,
		Root: d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 2, Height: 4}, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
		}),
		Transform: d2scene.Identity(),
	}
	host := d2scene.NewNode(d2scene.Image{Asset: "outer", Box: d2scene.Box{Width: 8, Height: 8}})
	host.Opacity = .5
	document := d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, host)
	document.Assets["pixel"] = d2scene.RasterAsset{
		MIMEType: "image/png", Data: encodeRasterPNG(t, raster), PixelWidth: 1, PixelHeight: 1,
	}
	document.Assets["inner"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 2, Height: 2}, Root: innerRoot}
	document.Assets["outer"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 4, Height: 4}, Root: outerRoot}

	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	inside := frame.NRGBAAt(1, 1)
	if inside.G != 255 || inside.A != 64 {
		t.Fatalf("nested vector/raster effects pixel = %#v, want green at alpha 64", inside)
	}
	assertPixel(t, frame.NRGBAAt(5, 1), color.NRGBA{})
	assertPixel(t, frame.NRGBAAt(1, 5), color.NRGBA{})

	want, err := renderTestPNG(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	const renders = 12
	results := make(chan []byte, renders)
	errors := make(chan error, renders)
	var group sync.WaitGroup
	for range renders {
		group.Add(1)
		go func() {
			defer group.Done()
			got, renderErr := renderTestPNG(context.Background(), document, testOptions())
			if renderErr != nil {
				errors <- renderErr
				return
			}
			results <- got
		}()
	}
	group.Wait()
	close(results)
	close(errors)
	for renderErr := range errors {
		t.Fatal(renderErr)
	}
	for got := range results {
		if !bytes.Equal(got, want) {
			t.Fatal("concurrent vector render was not deterministic")
		}
	}
	if outerRoot.Transform != d2scene.Identity() || innerRoot.Transform != d2scene.Identity() || rasterImage.Transform != d2scene.Identity() {
		t.Fatal("vector rendering mutated shared scene nodes")
	}
}

func TestVectorImageBlendIsolationAndHostBlendOrder(t *testing.T) {
	assetRoot := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 1, Height: 1}, Fill: red,
	})
	assetRoot.Blend = d2scene.BlendMultiply
	render := func(t *testing.T, hostBlend d2scene.BlendMode) color.NRGBA {
		t.Helper()
		root := d2scene.NewNode(nil)
		host := d2scene.NewNode(d2scene.Image{Asset: "blend", Box: d2scene.Box{Width: 1, Height: 1}})
		host.Blend = hostBlend
		root.Children = []*d2scene.Node{
			d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: blue}),
			host,
		}
		document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, root)
		document.Assets["blend"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot,
		}
		frame, err := Render(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		return frame.NRGBAAt(0, 0)
	}
	assertPixel(t, render(t, d2scene.BlendNormal), color.NRGBA{R: 255, A: 255})
	assertPixel(t, render(t, d2scene.BlendMultiply), color.NRGBA{A: 255})
}

func TestVectorImageCyclesImportDepthAndInstantiatedBudgets(t *testing.T) {
	t.Run("indirect cycle", func(t *testing.T) {
		aRoot := d2scene.NewNode(d2scene.Image{Asset: "b", Box: d2scene.Box{Width: 1, Height: 1}})
		bRoot := d2scene.NewNode(d2scene.Image{Asset: "a", Box: d2scene.Box{Width: 1, Height: 1}})
		document := vectorImageDocument("a", d2scene.Box{Width: 1, Height: 1})
		document.Assets["a"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: aRoot}
		document.Assets["b"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: bRoot}
		_, err := Render(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), "cyclic vector asset reference") || !strings.Contains(err.Error(), `"a"`) {
			t.Fatalf("cycle error = %v", err)
		}
	})

	t.Run("import depth boundary", func(t *testing.T) {
		inner := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
		outer := d2scene.NewNode(d2scene.Image{Asset: "inner", Box: d2scene.Box{Width: 1, Height: 1}})
		document := vectorImageDocument("outer", d2scene.Box{Width: 1, Height: 1})
		document.Assets["outer"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: outer}
		document.Assets["inner"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: inner}
		options := testOptions()
		options.MaxImportDepth = 1
		_, err := Render(context.Background(), document, options)
		if err == nil || !strings.Contains(err.Error(), "import depth 2 exceeds limit 1") {
			t.Fatalf("depth limit error = %v", err)
		}
		options.MaxImportDepth = 2
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("depth boundary Render: %v", err)
		}
	})

	t.Run("nodes depth and paths count each instance", func(t *testing.T) {
		path := d2scene.Path{Fill: red, Commands: []d2scene.PathCommand{
			d2scene.MoveTo(0, 0), d2scene.LineTo(1, 0), d2scene.LineTo(0, 1), d2scene.ClosePath(),
		}}
		assetRoot := d2scene.NewNode(path)
		root := d2scene.NewNode(nil)
		root.Children = []*d2scene.Node{
			d2scene.NewNode(d2scene.Image{Asset: "triangle", Box: d2scene.Box{Width: 2, Height: 2}}),
			d2scene.NewNode(d2scene.Image{Asset: "triangle", Box: d2scene.Box{X: 2, Width: 2, Height: 2}}),
		}
		document := d2scene.NewDocument(d2scene.Box{Width: 4, Height: 2}, root)
		document.Assets["triangle"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot}

		options := testOptions()
		// The retained asset root is validated once, then each of the two
		// visible instances is charged again: one retained root + document
		// group + two Image nodes + two instantiated roots = six nodes, and
		// three traversals of the four-command path = twelve commands.
		options.MaxNodes = 5
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "node count") {
			t.Fatalf("node limit error = %v", err)
		}
		options.MaxNodes = 6
		options.MaxDepth = 2
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "node depth 3 exceeds limit 2") {
			t.Fatalf("depth limit error = %v", err)
		}
		options.MaxDepth = 3
		options.MaxPathCommands = 11
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "path command count") {
			t.Fatalf("path limit error = %v", err)
		}
		options.MaxPathCommands = 12
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("instantiated budget boundary Render: %v", err)
		}
	})

	t.Run("imported clip work and viewport-scoped storage", func(t *testing.T) {
		assetRoot := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: red})
		assetRoot.Clip = &d2scene.Clip{Path: clipRect(0, 0, 4, 4, d2scene.EvenOdd), Transform: d2scene.Identity()}
		imageNode := d2scene.NewNode(d2scene.Image{
			Asset: "small", Box: d2scene.Box{X: 500, Y: 500, Width: 4, Height: 4},
		})
		document := d2scene.NewDocument(d2scene.Box{Width: 1_000, Height: 1_000}, imageNode)
		document.Assets["small"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 4, Height: 4}, Root: assetRoot}
		options := testOptions()
		options.MaxWidth, options.MaxHeight, options.MaxPixels = 1_000, 1_000, 1_000_000
		options.MaxEvenOddClipWork = 1_023
		if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "even-odd clip work 1024 exceeds limit 1023") {
			t.Fatalf("imported even-odd work error = %v", err)
		}
		options.MaxEvenOddClipWork = 1_024
		prepared, err := prepare(context.Background(), document, options)
		if err != nil {
			t.Fatal(err)
		}
		scanlineScratch, ok := scanline.RetainedBytes(4, 4, 2)
		if !ok {
			t.Fatal("scanline retained-byte calculation overflowed")
		}
		want := int64(144) + scanlineScratch
		if prepared.resources.peakOffscreenBytes != want {
			t.Fatalf("small vector image planned %d offscreen bytes, want viewport-scoped %d", prepared.resources.peakOffscreenBytes, want)
		}
		if _, err := Render(context.Background(), document, options); err != nil {
			t.Fatalf("imported clip work boundary Render: %v", err)
		}
	})
}

func TestUnusedVectorAssetsStrictPreflight(t *testing.T) {
	newUnusedDocument := func() *d2scene.Document {
		return d2scene.NewDocument(d2scene.Box{Width: 2, Height: 2}, d2scene.NewNode(nil))
	}

	t.Run("forward reference", func(t *testing.T) {
		document := newUnusedDocument()
		document.Assets["a"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root:    d2scene.NewNode(d2scene.Image{Asset: "z", Box: d2scene.Box{Width: 1, Height: 1}}),
		}
		document.Assets["z"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root:    d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red}),
		}
		if _, err := prepare(context.Background(), document, testOptions()); err != nil {
			t.Fatalf("forward-referenced retained asset: %v", err)
		}
	})

	t.Run("invalid filter root", func(t *testing.T) {
		root := d2scene.NewNode(nil)
		root.ID = "unused-root"
		var blur *d2scene.GaussianBlur
		root.Filters = []d2scene.Filter{blur}
		document := newUnusedDocument()
		document.Assets["unused"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: root}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), `vector asset "unused" retained validation`) || !strings.Contains(err.Error(), `node "unused-root" filter 0 is a nil Gaussian blur`) {
			t.Fatalf("unused invalid-filter-root error = %v", err)
		}
	})

	t.Run("singular intrinsic gradient transform", func(t *testing.T) {
		transforms := map[string]d2scene.Matrix{
			"zero row":           d2scene.Scale(0, 1),
			"exact dependent":    {A: 1, B: 1, C: 1, D: 1},
			"mixed proportional": {A: 1e300, B: 1, C: 1e300, D: 1},
		}
		for name, transform := range transforms {
			t.Run(name, func(t *testing.T) {
				document := newUnusedDocument()
				document.Assets["unused"] = d2scene.VectorAsset{
					ViewBox: d2scene.Box{Width: 1, Height: 1},
					Root: d2scene.NewNode(d2scene.Rect{
						Box:  d2scene.Box{Width: 1, Height: 1},
						Fill: retainedTestLinearGradient(transform, d2scene.UserSpaceOnUse),
					}),
				}
				_, err := prepare(context.Background(), document, testOptions())
				if err == nil || !strings.Contains(err.Error(), `vector asset "unused" retained validation`) || !strings.Contains(err.Error(), "singular gradient transform") {
					t.Fatalf("unused singular-gradient error = %v", err)
				}
			})
		}
	})

	t.Run("singular gradient ancestor transforms", func(t *testing.T) {
		gradientRect := func() *d2scene.Node {
			return d2scene.NewNode(d2scene.Rect{
				Box:  d2scene.Box{Width: 1, Height: 1},
				Fill: retainedTestLinearGradient(d2scene.Identity(), d2scene.UserSpaceOnUse),
			})
		}
		tests := []struct {
			name string
			root *d2scene.Node
		}{
			{name: "static node", root: func() *d2scene.Node {
				root := gradientRect()
				root.Transform = d2scene.Scale(0, 1)
				return root
			}()},
			{name: "animated node", root: func() *d2scene.Node {
				root := gradientRect()
				root.Animations = []d2scene.Track{animationTrack(
					d2scene.AnimateTransform,
					d2scene.TransformValue(d2scene.Scale(0, 1)),
					d2scene.TransformValue(d2scene.Scale(0, 1)),
				)}
				return root
			}()},
			{name: "mask", root: func() *d2scene.Node {
				root := d2scene.NewNode(nil)
				root.Mask = &d2scene.Mask{
					Type: d2scene.MaskAlpha, Transform: d2scene.Scale(0, 1), Root: gradientRect(),
				}
				return root
			}()},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				document := newUnusedDocument()
				document.Assets["unused"] = d2scene.VectorAsset{
					ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: test.root,
				}
				_, err := prepare(context.Background(), document, testOptions())
				if err == nil || !strings.Contains(err.Error(), "singular gradient transform") {
					t.Fatalf("unused singular gradient ancestor error = %v", err)
				}
			})
		}
	})

	t.Run("extreme finite invertible gradient transform", func(t *testing.T) {
		document := newUnusedDocument()
		document.Assets["unused"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root: d2scene.NewNode(d2scene.Rect{
				Box:  d2scene.Box{Width: 1, Height: 1},
				Fill: retainedTestLinearGradient(d2scene.Matrix{A: 1e300, D: 1e-300}, d2scene.UserSpaceOnUse),
			}),
		}
		if _, err := prepare(context.Background(), document, testOptions()); err != nil {
			t.Fatalf("unused extreme invertible gradient: %v", err)
		}
	})

	t.Run("zero-area object bounding box", func(t *testing.T) {
		document := newUnusedDocument()
		document.Assets["unused"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root: d2scene.NewNode(d2scene.Rect{
				Box:  d2scene.Box{Height: 1},
				Fill: retainedTestLinearGradient(d2scene.Identity(), d2scene.ObjectBoundingBox),
			}),
		}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), "object bounding box has zero width or height") {
			t.Fatalf("unused zero-area objectBoundingBox error = %v", err)
		}
	})

	t.Run("missing nested asset", func(t *testing.T) {
		document := newUnusedDocument()
		document.Assets["unused"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root:    d2scene.NewNode(d2scene.Image{Asset: "missing", Box: d2scene.Box{Width: 1, Height: 1}}),
		}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), `vector asset "unused" retained validation`) || !strings.Contains(err.Error(), `missing asset "missing"`) {
			t.Fatalf("unused missing-reference error = %v", err)
		}
	})

	t.Run("wrong nested asset type", func(t *testing.T) {
		fontBytes, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
		if !ok {
			t.Fatal("embedded Source Sans Pro font is not loaded")
		}
		document := newUnusedDocument()
		document.Assets["font"] = d2scene.FontAsset{MIMEType: "font/ttf", Data: fontBytes}
		document.Assets["unused"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root:    d2scene.NewNode(d2scene.Image{Asset: "font", Box: d2scene.Box{Width: 1, Height: 1}}),
		}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), `image asset "font" is not a raster or vector asset`) {
			t.Fatalf("unused wrong-reference error = %v", err)
		}
	})

	t.Run("non-finite nested aspect mapping", func(t *testing.T) {
		document := newUnusedDocument()
		document.Assets["inner"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1e-300, Height: 1},
			Root: d2scene.NewNode(d2scene.Rect{
				Box: d2scene.Box{Width: 1e-300, Height: 1}, Fill: red,
			}),
		}
		document.Assets["outer"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root: d2scene.NewNode(d2scene.Image{
				Asset: "inner", Box: d2scene.Box{Width: 1e300, Height: 1},
				Aspect: d2scene.AspectRatio{Align: d2scene.AlignNone},
			}),
		}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), `vector asset "inner" aspect ratio`) {
			t.Fatalf("unused nested aspect-ratio error = %v", err)
		}
	})

	t.Run("singular nested aspect mapping", func(t *testing.T) {
		document := newUnusedDocument()
		document.Assets["inner"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1e300, Height: 1},
			Root: d2scene.NewNode(d2scene.Rect{
				Box: d2scene.Box{Width: 1e300, Height: 1}, Fill: red,
			}),
		}
		document.Assets["outer"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root: d2scene.NewNode(d2scene.Image{
				Asset: "inner", Box: d2scene.Box{Width: 1e-300, Height: 1},
				Aspect: d2scene.AspectRatio{Align: d2scene.AlignNone},
			}),
		}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), `vector asset "inner" aspect ratio: mapping is singular in the finite numeric domain`) {
			t.Fatalf("unused singular nested aspect-ratio error = %v", err)
		}
	})

	t.Run("indirect retained cycle", func(t *testing.T) {
		document := newUnusedDocument()
		document.Assets["a"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root:    d2scene.NewNode(d2scene.Image{Asset: "z", Box: d2scene.Box{Width: 1, Height: 1}}),
		}
		document.Assets["z"] = d2scene.VectorAsset{
			ViewBox: d2scene.Box{Width: 1, Height: 1},
			Root:    d2scene.NewNode(d2scene.Image{Asset: "a", Box: d2scene.Box{Width: 1, Height: 1}}),
		}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), "cyclic vector asset reference") {
			t.Fatalf("unused retained-cycle error = %v", err)
		}
	})

	t.Run("nil child cannot be amplified by repeated instances", func(t *testing.T) {
		assetRoot := d2scene.NewNode(nil)
		assetRoot.ID = "retained-root"
		assetRoot.Children = make([]*d2scene.Node, 4_096)
		documentRoot := d2scene.NewNode(nil)
		documentRoot.Children = []*d2scene.Node{
			d2scene.NewNode(d2scene.Image{Asset: "bad", Box: d2scene.Box{Width: 1, Height: 1}}),
			d2scene.NewNode(d2scene.Image{Asset: "bad", Box: d2scene.Box{X: 1, Width: 1, Height: 1}}),
		}
		document := d2scene.NewDocument(d2scene.Box{Width: 2, Height: 1}, documentRoot)
		document.Assets["bad"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot}
		_, err := prepare(context.Background(), document, testOptions())
		if err == nil || !strings.Contains(err.Error(), `vector asset "bad" retained validation`) || !strings.Contains(err.Error(), `node "retained-root" child 0 is nil`) {
			t.Fatalf("repeated nil-child vector error = %v", err)
		}
	})
}

func TestRetainedVectorValidationLimitBoundaries(t *testing.T) {
	newUnusedDocument := func(root *d2scene.Node) *d2scene.Document {
		document := d2scene.NewDocument(d2scene.Box{Width: 2, Height: 2}, d2scene.NewNode(nil))
		document.Assets["unused"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: root}
		return document
	}

	pathRoot := d2scene.NewNode(d2scene.Path{Fill: red, Commands: []d2scene.PathCommand{
		d2scene.MoveTo(0, 0), d2scene.LineTo(1, 0), d2scene.LineTo(0, 1), d2scene.ClosePath(),
	}})
	options := testOptions()
	options.MaxNodes = 1
	if _, err := prepare(context.Background(), newUnusedDocument(pathRoot), options); err == nil || !strings.Contains(err.Error(), "node count") {
		t.Fatalf("retained+document node limit error = %v", err)
	}
	options.MaxNodes = 2
	options.MaxPathCommands = 3
	if _, err := prepare(context.Background(), newUnusedDocument(pathRoot), options); err == nil || !strings.Contains(err.Error(), "path command count") {
		t.Fatalf("retained path limit error = %v", err)
	}
	options.MaxPathCommands = 4
	if _, err := prepare(context.Background(), newUnusedDocument(pathRoot), options); err != nil {
		t.Fatalf("retained node/path boundary: %v", err)
	}

	deepRoot := d2scene.NewNode(nil)
	deepRoot.Children = []*d2scene.Node{d2scene.NewNode(nil)}
	options = testOptions()
	options.MaxDepth = 1
	if _, err := prepare(context.Background(), newUnusedDocument(deepRoot), options); err == nil || !strings.Contains(err.Error(), "node depth 2 exceeds limit 1") {
		t.Fatalf("retained depth limit error = %v", err)
	}
	options.MaxDepth = 2
	if _, err := prepare(context.Background(), newUnusedDocument(deepRoot), options); err == nil || !strings.Contains(err.Error(), "composed node depth 3 exceeds limit 2") {
		t.Fatalf("unplaceable retained depth error = %v", err)
	}
	options.MaxDepth = 3
	if _, err := prepare(context.Background(), newUnusedDocument(deepRoot), options); err != nil {
		t.Fatalf("retained depth boundary: %v", err)
	}

	outer := d2scene.NewNode(d2scene.Image{Asset: "z", Box: d2scene.Box{Width: 1, Height: 1}})
	inner := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	importDocument := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
	importDocument.Assets["a"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: outer}
	importDocument.Assets["z"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: inner}
	options = testOptions()
	options.MaxImportDepth = 1
	if _, err := prepare(context.Background(), importDocument, options); err == nil || !strings.Contains(err.Error(), "import depth 2 exceeds limit 1") {
		t.Fatalf("retained import-depth limit error = %v", err)
	}
	options.MaxImportDepth = 2
	if _, err := prepare(context.Background(), importDocument, options); err != nil {
		t.Fatalf("retained import-depth boundary: %v", err)
	}
}

func TestRetainedSingleNodeVectorIncludesUnavoidableHostDepth(t *testing.T) {
	document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
	document.Assets["unused"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 1, Height: 1},
		Root: d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 1, Height: 1}, Fill: red,
		}),
	}
	options := testOptions()
	options.MaxDepth = 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "composed node depth 2 exceeds limit 1") {
		t.Fatalf("single-node retained host-depth error = %v", err)
	}
	options.MaxDepth = 2
	if _, err := prepare(context.Background(), document, options); err != nil {
		t.Fatalf("single-node retained host-depth boundary: %v", err)
	}
}

func TestRetainedVectorValidationMemoizesDefinitionsAndLongestImportChain(t *testing.T) {
	const assets = 40
	document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
	for index := 0; index < assets; index++ {
		id := d2scene.AssetID(fmt.Sprintf("a%02d", index))
		var root *d2scene.Node
		if index == 0 {
			root = d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
		} else {
			root = d2scene.NewNode(d2scene.Image{
				Asset: d2scene.AssetID(fmt.Sprintf("a%02d", index-1)),
				Box:   d2scene.Box{Width: 1, Height: 1},
			})
		}
		document.Assets[id] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: root}
	}

	options := testOptions()
	options.MaxImportDepth = assets - 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "import depth 40 exceeds limit 39") {
		t.Fatalf("memoized longest-chain limit error = %v", err)
	}
	options.MaxImportDepth = assets
	options.MaxDepth = assets
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "composed node depth 41 exceeds limit 40") {
		t.Fatalf("memoized composed-depth limit error = %v", err)
	}
	options.MaxDepth = assets + 1
	options.MaxNodes = assets // Forty definitions plus the document root need 41.
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "node count") {
		t.Fatalf("memoized retained-node boundary error = %v", err)
	}
	options.MaxNodes = assets + 1
	if _, err := prepare(context.Background(), document, options); err != nil {
		t.Fatalf("linear retained graph validation boundary: %v", err)
	}
}

func TestRetainedVectorDepthCombinesOuterSubtreeAndImportedAsset(t *testing.T) {
	innerRoot := d2scene.NewNode(nil)
	innerRoot.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red}),
	}
	outerImage := d2scene.NewNode(d2scene.Image{Asset: "inner", Box: d2scene.Box{Width: 1, Height: 1}})
	outerGroup := d2scene.NewNode(nil)
	outerGroup.Children = []*d2scene.Node{outerImage}
	outerRoot := d2scene.NewNode(nil)
	outerRoot.Children = []*d2scene.Node{outerGroup}
	document := d2scene.NewDocument(d2scene.Box{Width: 1, Height: 1}, d2scene.NewNode(nil))
	document.Assets["inner"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: innerRoot}
	document.Assets["outer"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: outerRoot}

	options := testOptions()
	// Shallowest placement: host Image 1, outer root 2, outer group 3,
	// outer Image 4, inner root 5, inner child 6.
	options.MaxDepth = 5
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "composed node depth 6 exceeds limit 5") {
		t.Fatalf("outer plus imported subtree depth error = %v", err)
	}
	options.MaxDepth = 6
	if _, err := prepare(context.Background(), document, options); err != nil {
		t.Fatalf("outer plus imported subtree depth boundary: %v", err)
	}
}

func TestRetainedVectorLargeIntrinsicCoordinatesUseVisibleMapping(t *testing.T) {
	const intrinsic = 1e40 // Finite, but outside the rasterizer's float32 device domain.
	assetRoot := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: intrinsic, Height: intrinsic},
		Fill: red,
	})
	document := vectorImageDocument("huge", d2scene.Box{Width: 4, Height: 4})
	document.Assets["huge"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: intrinsic, Height: intrinsic},
		Root:    assetRoot,
	}
	frame, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatalf("safely scaled intrinsic coordinates: %v", err)
	}
	assertPixel(t, frame.NRGBAAt(1, 1), color.NRGBA{R: 255, A: 255})

	emptyDocument := vectorImageDocument("huge", d2scene.Box{Height: 4})
	emptyDocument.Assets["huge"] = document.Assets["huge"]
	emptyFrame, err := Render(context.Background(), emptyDocument, testOptions())
	if err != nil {
		t.Fatalf("zero-width safely scaled intrinsic coordinates: %v", err)
	}
	for index, alpha := range emptyFrame.Pix[3:] {
		if index%4 == 0 && alpha != 0 {
			t.Fatalf("zero-width huge vector image painted alpha %d", alpha)
		}
	}

	tiny := math.SmallestNonzeroFloat64
	tinyDocument := vectorImageDocument("tiny", d2scene.Box{Height: 1})
	tinyDocument.Assets["tiny"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: tiny, Height: 1},
		Root: d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: tiny, Height: 1}, Fill: red,
		}),
	}
	if _, err := Render(context.Background(), tinyDocument, testOptions()); err != nil {
		t.Fatalf("zero-width subnormal-viewBox vector image: %v", err)
	}
}

func TestRepeatedVectorAnimationBudgetsAreAggregateAndInclusive(t *testing.T) {
	assetRoot := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	assetRoot.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateOpacity,
		d2scene.NumberValue(1),
		d2scene.NumberValue(1),
	)}
	documentRoot := d2scene.NewNode(nil)
	documentRoot.Children = []*d2scene.Node{
		d2scene.NewNode(d2scene.Image{Asset: "animated", Box: d2scene.Box{Width: 1, Height: 1}}),
		d2scene.NewNode(d2scene.Image{Asset: "animated", Box: d2scene.Box{X: 1, Width: 1, Height: 1}}),
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 2, Height: 1}, documentRoot)
	document.Assets["animated"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot}

	options := testOptions()
	// One retained definition plus two visible instances consume three tracks
	// and six keyframes.
	options.MaxAnimationTracks = 2
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "animation track count") {
		t.Fatalf("repeated vector track limit error = %v", err)
	}
	options.MaxAnimationTracks = 3
	options.MaxAnimationKeyframes = 5
	if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "keyframe count") {
		t.Fatalf("repeated vector keyframe limit error = %v", err)
	}
	options.MaxAnimationKeyframes = 6
	frame, err := Render(context.Background(), document, options)
	if err != nil {
		t.Fatalf("inclusive repeated animation budgets: %v", err)
	}
	assertPixel(t, frame.NRGBAAt(0, 0), color.NRGBA{R: 255, A: 255})
	assertPixel(t, frame.NRGBAAt(1, 0), color.NRGBA{R: 255, A: 255})
}

func TestAnimationLimitsAndKeyframeCancellation(t *testing.T) {
	document := testDocument(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	options := testOptions()
	options.MaxAnimationTracks = 0
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "every frame resource limit") {
		t.Fatalf("zero animation-track limit error = %v", err)
	}
	options = testOptions()
	options.MaxAnimationKeyframes = 0
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "every frame resource limit") {
		t.Fatalf("zero animation-keyframe limit error = %v", err)
	}

	keyframes := make([]d2scene.Keyframe, 1_024)
	for index := range keyframes {
		keyframes[index] = d2scene.Keyframe{
			Offset: float64(index) / float64(len(keyframes)-1),
			Value:  d2scene.NumberValue(1),
		}
	}
	animated := d2scene.NewNode(nil)
	animated.Animations = []d2scene.Track{{
		Property: d2scene.AnimateOpacity, Duration: time.Second, Keyframes: keyframes,
	}}
	ctx := &vectorCancelAfterErrChecks{remaining: 4}
	p := &preflight{
		ctx:          ctx,
		options:      testOptions(),
		frameBounds:  image.Rect(0, 0, 1, 1),
		active:       make(map[*d2scene.Node]bool),
		activeAssets: make(map[d2scene.AssetID]bool),
	}
	_, err := p.node(animated, d2scene.Identity(), 1, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("keyframe validation cancellation error = %v, want context.Canceled", err)
	}
}

func TestZeroSizeVectorImageStillChargesInstantiatedWork(t *testing.T) {
	path := d2scene.Path{Fill: red, Commands: []d2scene.PathCommand{
		d2scene.MoveTo(0, 0), d2scene.LineTo(1, 0), d2scene.LineTo(0, 1), d2scene.ClosePath(),
	}}
	assetRoot := d2scene.NewNode(path)
	assetRoot.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateOpacity, d2scene.NumberValue(1), d2scene.NumberValue(1),
	)}
	document := vectorImageDocument("empty", d2scene.Box{Height: 1})
	document.Assets["empty"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: assetRoot,
	}

	tests := []struct {
		name  string
		short func(*FrameOptions)
		exact func(*FrameOptions)
		want  string
	}{
		{
			name:  "nodes",
			short: func(options *FrameOptions) { options.MaxNodes = 2 },
			exact: func(options *FrameOptions) { options.MaxNodes = 3 },
			want:  "node count",
		},
		{
			name:  "paths",
			short: func(options *FrameOptions) { options.MaxPathCommands = 7 },
			exact: func(options *FrameOptions) { options.MaxPathCommands = 8 },
			want:  "path command count",
		},
		{
			name:  "animation tracks",
			short: func(options *FrameOptions) { options.MaxAnimationTracks = 1 },
			exact: func(options *FrameOptions) { options.MaxAnimationTracks = 2 },
			want:  "animation track count",
		},
		{
			name:  "animation keyframes",
			short: func(options *FrameOptions) { options.MaxAnimationKeyframes = 3 },
			exact: func(options *FrameOptions) { options.MaxAnimationKeyframes = 4 },
			want:  "keyframe count",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := testOptions()
			test.short(&options)
			if _, err := Render(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("one-short zero-size work error = %v, want %q", err, test.want)
			}
			options = testOptions()
			test.exact(&options)
			frame, err := Render(context.Background(), document, options)
			if err != nil {
				t.Fatalf("exact zero-size work boundary: %v", err)
			}
			assertPixel(t, frame.NRGBAAt(0, 0), color.NRGBA{})
		})
	}
}

func TestNodeChildIterationHonorsCancellationBeforeNilValidation(t *testing.T) {
	ctx := &vectorCancelAfterErrChecks{remaining: 1}
	root := d2scene.NewNode(nil)
	root.Children = []*d2scene.Node{nil}
	p := &preflight{
		ctx:          ctx,
		options:      testOptions(),
		frameBounds:  image.Rect(0, 0, 1, 1),
		active:       make(map[*d2scene.Node]bool),
		activeAssets: make(map[d2scene.AssetID]bool),
	}
	_, err := p.node(root, d2scene.Identity(), 1, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("child-iteration cancellation error = %v, want context.Canceled", err)
	}
}

type vectorCancelAfterErrChecks struct {
	remaining int
}

func (c *vectorCancelAfterErrChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *vectorCancelAfterErrChecks) Done() <-chan struct{}       { return nil }
func (c *vectorCancelAfterErrChecks) Value(any) any               { return nil }
func (c *vectorCancelAfterErrChecks) Err() error {
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func TestVectorImageZeroSizeAndMalformedAssets(t *testing.T) {
	zero := vectorImageDocument("vector", d2scene.Box{X: 3, Y: 2, Height: 6})
	zero.ViewBox = d2scene.Box{Width: 10, Height: 10}
	zero.LogicalWidth, zero.LogicalHeight = 10, 10
	zero.Assets["vector"] = d2scene.VectorAsset{
		ViewBox: d2scene.Box{Width: 2, Height: 2},
		Root: d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{Width: 2, Height: 2},
			Fill: d2scene.LinearGradient{
				Start: d2scene.Point{}, End: d2scene.Point{X: 2}, Units: d2scene.UserSpaceOnUse,
				Transform: d2scene.Identity(),
				Stops: []d2scene.GradientStop{
					{Color: color.NRGBA{R: 255, A: 255}},
					{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
				},
			},
		}),
	}
	frame, err := Render(context.Background(), zero, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	for index, alpha := range frame.Pix[3:] {
		if index%4 == 0 && alpha != 0 {
			t.Fatalf("zero-width vector image painted alpha %d", alpha)
		}
	}

	validRoot := d2scene.NewNode(nil)
	tests := []struct {
		name  string
		asset d2scene.Asset
		want  string
	}{
		{name: "nil pointer", asset: (*d2scene.VectorAsset)(nil), want: "is nil"},
		{name: "zero viewbox", asset: d2scene.VectorAsset{ViewBox: d2scene.Box{}, Root: validRoot}, want: "invalid viewbox"},
		{name: "negative viewbox", asset: d2scene.VectorAsset{ViewBox: d2scene.Box{Width: -1, Height: 1}, Root: validRoot}, want: "invalid viewbox"},
		{name: "nonfinite viewbox", asset: d2scene.VectorAsset{ViewBox: d2scene.Box{Width: math.NaN(), Height: 1}, Root: validRoot}, want: "invalid viewbox"},
		{name: "missing root", asset: d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}}, want: "no root node"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := vectorImageDocument("bad", d2scene.Box{Width: 1, Height: 1})
			document.Assets["bad"] = test.asset
			_, err := Render(context.Background(), document, testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), `"bad"`) {
				t.Fatalf("malformed vector error = %v, want asset ID and %q", err, test.want)
			}
		})
	}

	badRoot := d2scene.NewNode(nil)
	badRoot.ID = "bad-root"
	badRoot.Opacity = 2
	malformedRoot := vectorImageDocument("bad", d2scene.Box{Width: 1, Height: 1})
	malformedRoot.Assets["bad"] = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 1, Height: 1}, Root: badRoot}
	_, err = Render(context.Background(), malformedRoot, testOptions())
	if err == nil || !strings.Contains(err.Error(), `vector asset "bad"`) || !strings.Contains(err.Error(), `node "bad-root"`) {
		t.Fatalf("malformed root error = %v, want asset and node context", err)
	}
}

func vectorImageDocument(asset d2scene.AssetID, box d2scene.Box) *d2scene.Document {
	return d2scene.NewDocument(
		d2scene.Box{Width: math.Max(1, box.X+box.Width), Height: math.Max(1, box.Y+box.Height)},
		d2scene.NewNode(d2scene.Image{Asset: asset, Box: box}),
	)
}

func retainedTestLinearGradient(transform d2scene.Matrix, units d2scene.PaintUnits) d2scene.LinearGradient {
	return d2scene.LinearGradient{
		Start: d2scene.Point{}, End: d2scene.Point{X: 1},
		Units: units, Transform: transform,
		Stops: []d2scene.GradientStop{
			{Color: color.NRGBA{R: 255, A: 255}},
			{Offset: 1, Color: color.NRGBA{B: 255, A: 255}},
		},
	}
}
