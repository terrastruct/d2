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

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestGaussianBlurPixelsAndDegenerateIdentity(t *testing.T) {
	t.Parallel()

	makeNode := func(filter d2scene.Filter) *d2scene.Node {
		node := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{X: 6, Y: 6, Width: 12, Height: 12},
			Fill: red,
		})
		if filter != nil {
			node.Filters = []d2scene.Filter{filter}
		}
		return node
	}
	document := func(node *d2scene.Node) *d2scene.Document {
		return d2scene.NewDocument(d2scene.Box{Width: 24, Height: 24}, node)
	}

	baseline, err := renderTestPNG(context.Background(), document(makeNode(nil)), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	zero := &d2scene.GaussianBlur{}
	identity, err := renderTestPNG(context.Background(), document(makeNode(zero)), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(identity, baseline) {
		t.Fatal("zero-deviation Gaussian blur changed the frame")
	}

	blurred, err := Render(context.Background(), document(makeNode(d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1})), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, blurred.NRGBAAt(12, 12), color.NRGBA{R: 255, A: 255})
	if got := blurred.NRGBAAt(5, 12); got.R != 255 || got.A == 0 || got.A == 255 {
		t.Fatalf("Gaussian fringe pixel = %#v, want translucent red", got)
	}
	assertPixel(t, blurred.NRGBAAt(2, 12), color.NRGBA{})

	horizontal, err := Render(context.Background(), document(makeNode(d2scene.GaussianBlur{SigmaX: 1})), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if horizontal.NRGBAAt(5, 12).A == 0 {
		t.Fatal("horizontal Gaussian blur did not spread horizontally")
	}
	assertPixel(t, horizontal.NRGBAAt(12, 5), color.NRGBA{})

	edgeNode := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 12, Height: 12, Y: 6}, Fill: red,
	})
	edgeNode.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1}}
	edge, err := Render(context.Background(), document(edgeNode), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := edge.NRGBAAt(0, 12), blurred.NRGBAAt(6, 12); got != want {
		t.Fatalf("viewport-edge Gaussian pixel = %#v, want translation-invariant %#v", got, want)
	}
}

func TestDropShadowPixelsAndTransparentIdentity(t *testing.T) {
	t.Parallel()

	makeNode := func(shadow d2scene.DropShadow) *d2scene.Node {
		node := d2scene.NewNode(d2scene.Rect{
			Box:  d2scene.Box{X: 4, Y: 4, Width: 4, Height: 4},
			Fill: red,
		})
		node.Filters = []d2scene.Filter{shadow}
		return node
	}
	document := func(node *d2scene.Node) *d2scene.Document {
		return d2scene.NewDocument(d2scene.Box{Width: 16, Height: 12}, node)
	}

	shadow := d2scene.DropShadow{OffsetX: 6, Color: color.NRGBA{A: 128}}
	frame, err := Render(context.Background(), document(makeNode(shadow)), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, frame.NRGBAAt(5, 5), color.NRGBA{R: 255, A: 255})
	assertPixel(t, frame.NRGBAAt(11, 5), color.NRGBA{A: 128})
	assertPixel(t, frame.NRGBAAt(15, 5), color.NRGBA{})

	baselineNode := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{X: 4, Y: 4, Width: 4, Height: 4}, Fill: red})
	baseline, err := renderTestPNG(context.Background(), document(baselineNode), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	transparent, err := renderTestPNG(context.Background(), document(makeNode(d2scene.DropShadow{
		OffsetX: math.MaxFloat64, SigmaX: math.MaxFloat64, SigmaY: math.MaxFloat64, Color: color.NRGBA{R: 255},
	})), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(transparent, baseline) {
		t.Fatal("transparent drop shadow changed the frame")
	}
	_, err = renderTestPNG(context.Background(), document(makeNode(d2scene.DropShadow{
		OffsetX: math.MaxFloat64, Color: color.NRGBA{A: 255},
	})), testOptions())
	if err == nil || !strings.Contains(err.Error(), "translated filter bounds exceed") {
		t.Fatalf("unrepresentable opaque shadow error = %v", err)
	}

	offscreenSource := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: -6, Y: 4, Width: 4, Height: 4}, Fill: red,
	})
	offscreenSource.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 10, Color: color.NRGBA{A: 255},
	}}
	broughtIntoView, err := Render(context.Background(), document(offscreenSource), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, broughtIntoView.NRGBAAt(5, 5), color.NRGBA{A: 255})

	patternSource := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{X: -6, Y: 4, Width: 4, Height: 4},
		Fill: stripedPattern(d2scene.UserSpaceOnUse, d2scene.Box{Width: 2, Height: 1}, d2scene.Identity()),
	})
	patternSource.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 10, Color: color.NRGBA{A: 255},
	}}
	patternShadow, err := Render(context.Background(), document(patternSource), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, patternShadow.NRGBAAt(5, 5), color.NRGBA{A: 255})
}

func TestFiltersRespectDeclaredOrder(t *testing.T) {
	t.Parallel()

	render := func(filters ...d2scene.Filter) ([]byte, *preparedDocument) {
		t.Helper()
		node := d2scene.NewNode(d2scene.Rect{
			Box: d2scene.Box{X: 5, Y: 5, Width: 6, Height: 6}, Fill: red,
		})
		node.Filters = filters
		document := d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, node)
		prepared, err := prepare(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		png, err := renderTestPNG(context.Background(), document, testOptions())
		if err != nil {
			t.Fatal(err)
		}
		return png, prepared
	}
	blur := d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1}
	shadow := d2scene.DropShadow{OffsetX: 1, OffsetY: 1, SigmaX: 1, SigmaY: 1, Color: color.NRGBA{A: 220}}
	blurThenShadow, first := render(blur, shadow)
	shadowThenBlur, second := render(shadow, blur)
	if len(first.root.filters) != 2 || first.root.filters[0].kind != preparedGaussianBlur || first.root.filters[1].kind != preparedDropShadow {
		t.Fatalf("prepared blur-then-shadow order = %+v", first.root.filters)
	}
	if len(second.root.filters) != 2 || second.root.filters[0].kind != preparedDropShadow || second.root.filters[1].kind != preparedGaussianBlur {
		t.Fatalf("prepared shadow-then-blur order = %+v", second.root.filters)
	}
	if bytes.Equal(blurThenShadow, shadowThenBlur) {
		t.Fatal("reversing non-commuting filters did not change rendered pixels")
	}
}

func TestFilterParametersUseComposedDeviceTransform(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 1, Y: 1, Width: 2, Height: 2}, Fill: red,
	})
	node.Transform = d2scene.Scale(2, 3)
	node.Filters = []d2scene.Filter{
		&d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1},
		&d2scene.DropShadow{OffsetX: 2, OffsetY: 2, Color: color.NRGBA{A: 255}},
	}
	prepared, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 20, Height: 20}, node), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 2 {
		t.Fatalf("prepared filters = %d, want 2", len(prepared.root.filters))
	}
	blur, shadow := prepared.root.filters[0], prepared.root.filters[1]
	if blur.kind != preparedGaussianBlur || len(blur.passes) != 6 {
		t.Fatalf("scaled Gaussian preparation = %+v", blur)
	}
	horizontal, vertical := 0, 0
	for _, pass := range blur.passes {
		if pass.axis == blurHorizontal {
			horizontal += pass.radius
		} else {
			vertical += pass.radius
		}
	}
	if horizontal != 6 || vertical != 9 {
		t.Fatalf("scaled Gaussian support = (%d,%d), want (6,9)", horizontal, vertical)
	}
	if shadow.kind != preparedDropShadow || shadow.offsetX != 4 || shadow.offsetY != 6 {
		t.Fatalf("scaled drop-shadow offset = %+v, want (4,6)", shadow)
	}
}

func TestFilterOrderPrecedesClipMaskOpacity(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 2, Y: 2, Width: 4, Height: 4}, Fill: red,
	})
	node.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 4, Color: color.NRGBA{A: 255},
	}}
	node.Clip = &d2scene.Clip{Path: clipRect(0, 0, 9, 12, d2scene.NonZero), Transform: d2scene.Identity()}
	maskRoot := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{Width: 12, Height: 12},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	})
	maskRoot.Opacity = .5
	node.Mask = &d2scene.Mask{Type: d2scene.MaskAlpha, Root: maskRoot, Transform: d2scene.Identity()}
	node.Opacity = .5

	frame, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 12, Height: 12}, node), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	shadow := frame.NRGBAAt(8, 4)
	if shadow.R != 0 || shadow.G != 0 || shadow.B != 0 || shadow.A < 63 || shadow.A > 64 {
		t.Fatalf("filtered shadow after mask and opacity = %#v, want black alpha 63/64", shadow)
	}
	assertPixel(t, frame.NRGBAAt(9, 4), color.NRGBA{})
}

func TestAnimatedDropShadowTargetIndexAndConcurrentImmutability(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 2, Y: 4, Width: 4, Height: 4}, Fill: red,
	})
	node.Filters = []d2scene.Filter{
		d2scene.DropShadow{OffsetX: 1, Color: color.NRGBA{G: 255, A: 255}},
		d2scene.GaussianBlur{},
		d2scene.DropShadow{OffsetX: 4, Color: color.NRGBA{A: 64}},
	}
	track := animationTrack(
		d2scene.AnimateDropShadow,
		d2scene.ShadowValue(d2scene.DropShadow{OffsetX: 4, Color: color.NRGBA{A: 64}}),
		d2scene.ShadowValue(d2scene.DropShadow{OffsetX: 8, Color: color.NRGBA{B: 255, A: 192}}),
	)
	track.TargetIndex = 2
	node.Animations = []d2scene.Track{track}
	document := d2scene.NewDocument(d2scene.Box{Width: 20, Height: 12}, node)
	original := fmt.Sprintf("%#v", document)

	options := testOptions()
	options.Time = 500 * time.Millisecond
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 2 {
		t.Fatalf("prepared active filters = %d, want two drop shadows and omitted identity blur", len(prepared.root.filters))
	}
	first, second := prepared.root.filters[0], prepared.root.filters[1]
	if first.kind != preparedDropShadow || first.offsetX != 1 || first.shadowColor != (color.NRGBA{G: 255, A: 255}) {
		t.Fatalf("static target zero changed: %+v", first)
	}
	if second.kind != preparedDropShadow || second.offsetX != 6 || second.shadowColor != (color.NRGBA{B: 128, A: 128}) {
		t.Fatalf("animated target two = %+v, want midpoint shadow", second)
	}

	times := []time.Duration{0, 250 * time.Millisecond, 500 * time.Millisecond, 750 * time.Millisecond, time.Second}
	want := make([][]byte, len(times))
	for index, timestamp := range times {
		frameOptions := testOptions()
		frameOptions.Time = timestamp
		want[index], err = renderTestPNG(context.Background(), document, frameOptions)
		if err != nil {
			t.Fatal(err)
		}
	}
	var wait sync.WaitGroup
	errs := make(chan error, len(times)*4)
	for range 4 {
		for index, timestamp := range times {
			wait.Add(1)
			go func(index int, timestamp time.Duration) {
				defer wait.Done()
				frameOptions := testOptions()
				frameOptions.Time = timestamp
				got, err := renderTestPNG(context.Background(), document, frameOptions)
				if err != nil {
					errs <- err
					return
				}
				if !bytes.Equal(got, want[index]) {
					errs <- errors.New("concurrent animated drop-shadow frame changed")
				}
			}(index, timestamp)
		}
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if fmt.Sprintf("%#v", document) != original {
		t.Fatal("animated drop-shadow rendering mutated the document")
	}
}

func TestDropShadowAnimationRejectsInvalidTargetIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		index  int
		filter d2scene.Filter
		want   string
	}{
		{name: "outside", index: 1, filter: d2scene.DropShadow{}, want: "outside 1 filters"},
		{name: "wrong kind", filter: d2scene.GaussianBlur{SigmaX: 1}, want: "does not identify a drop shadow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: red})
			node.Filters = []d2scene.Filter{test.filter}
			track := animationTrack(
				d2scene.AnimateDropShadow,
				d2scene.ShadowValue(d2scene.DropShadow{}),
				d2scene.ShadowValue(d2scene.DropShadow{}),
			)
			track.TargetIndex = test.index
			node.Animations = []d2scene.Track{track}
			_, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, node), testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Render() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDropShadowAnimationDoesNotHideInvalidStaticFilter(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 4, Height: 4}, Fill: red})
	node.Filters = []d2scene.Filter{d2scene.DropShadow{SigmaX: -1}}
	node.Animations = []d2scene.Track{animationTrack(
		d2scene.AnimateDropShadow,
		d2scene.ShadowValue(d2scene.DropShadow{}),
		d2scene.ShadowValue(d2scene.DropShadow{}),
	)}
	_, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, node), testOptions())
	if err == nil || !strings.Contains(err.Error(), "invalid Gaussian deviation") {
		t.Fatalf("Render() error = %v, want invalid static filter rejection", err)
	}
}

func TestFilterPreflightValidationAndStructuralLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter d2scene.Filter
		want   string
	}{
		{name: "negative sigma", filter: d2scene.GaussianBlur{SigmaX: -1}, want: "invalid Gaussian deviation"},
		{name: "NaN sigma", filter: d2scene.GaussianBlur{SigmaY: math.NaN()}, want: "invalid Gaussian deviation"},
		{name: "infinite offset", filter: d2scene.DropShadow{OffsetX: math.Inf(1), Color: color.NRGBA{A: 255}}, want: "invalid drop-shadow offset"},
		{name: "nil Gaussian pointer", filter: (*d2scene.GaussianBlur)(nil), want: "nil Gaussian blur"},
		{name: "nil shadow pointer", filter: (*d2scene.DropShadow)(nil), want: "nil drop shadow"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := d2scene.NewNode(nil)
			node.ID = "filtered"
			node.Filters = []d2scene.Filter{test.filter}
			_, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, node), testOptions())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepare() error = %v, want substring %q", err, test.want)
			}
		})
	}

	tooWide := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	tooWide.ID = "wide"
	tooWide.Transform = d2scene.Scale(float64(maxBlurSupportRadius), 1)
	tooWide.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 2}}
	_, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, tooWide), testOptions())
	if err == nil || !strings.Contains(err.Error(), "three-sigma support exceeds") {
		t.Fatalf("large transformed deviation error = %v", err)
	}

	tooMany := d2scene.NewNode(nil)
	tooMany.Filters = make([]d2scene.Filter, 2)
	for index := range tooMany.Filters {
		tooMany.Filters[index] = d2scene.GaussianBlur{}
	}
	options := testOptions()
	options.MaxNodes = 1
	_, err = prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, tooMany), options)
	if err == nil || !strings.Contains(err.Error(), "filter count to exceed structural limit 1") {
		t.Fatalf("filter structural-limit error = %v", err)
	}

	tooDistant := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 1, Height: 1}, Fill: red})
	tooDistant.Filters = []d2scene.Filter{d2scene.DropShadow{
		OffsetX: 20_000_000, Color: color.NRGBA{A: 255},
	}}
	_, err = prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, tooDistant), testOptions())
	if err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
		t.Fatalf("distant-shadow resource error = %v", err)
	}

	empty := d2scene.NewNode(nil)
	empty.Filters = []d2scene.Filter{
		d2scene.GaussianBlur{SigmaX: math.MaxFloat64, SigmaY: math.MaxFloat64},
		d2scene.DropShadow{OffsetX: 2, SigmaX: 1, SigmaY: 1, Color: color.NRGBA{A: 255}},
	}
	prepared, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 8, Height: 8}, empty), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 0 || !prepared.root.bounds.Empty() || prepared.resources.peakOffscreenBytes != 0 {
		t.Fatalf("empty filtered node retained work: filters=%d bounds=%v resources=%+v", len(prepared.root.filters), prepared.root.bounds, prepared.resources)
	}
}

func TestFilterResourcePlanLimitsAndNodeLocalLayers(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 45, Y: 45, Width: 10, Height: 10}, Fill: red,
	})
	node.Filters = []d2scene.Filter{
		d2scene.GaussianBlur{SigmaX: 1, SigmaY: 1},
		d2scene.DropShadow{OffsetX: 3, OffsetY: 2, SigmaX: 1, SigmaY: 1, Color: color.NRGBA{A: 180}},
	}
	document := d2scene.NewDocument(d2scene.Box{Width: 100, Height: 100}, node)
	options := testOptions()
	prepared, err := prepare(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	filterPeak, finalBytes, err := planFilterResources(prepared.root.filters, prepared.root.contentBounds)
	if err != nil {
		t.Fatal(err)
	}
	if finalBytes == 0 || filterPeak == 0 {
		t.Fatalf("filter resource plan = peak %d final %d", filterPeak, finalBytes)
	}
	want, ok := checkedAdd(filterPeak, prepared.resources.rasterizerBytes)
	if !ok || prepared.resources.peakOffscreenBytes != want {
		t.Fatalf("planned peak = %d, want filter %d + rasterizer %d = %d", prepared.resources.peakOffscreenBytes, filterPeak, prepared.resources.rasterizerBytes, want)
	}
	if prepared.resources.peakOffscreenBytes >= 100*100*4 {
		t.Fatalf("small filtered node planned %d bytes, unexpectedly at least one full-document RGBA layer", prepared.resources.peakOffscreenBytes)
	}

	options.MaxOffscreenBytes = want - 1
	if _, err := prepare(context.Background(), document, options); err == nil || !strings.Contains(err.Error(), "peak offscreen pixel storage") {
		t.Fatalf("below-limit prepare() error = %v", err)
	}
	options.MaxOffscreenBytes = want
	if _, err := Render(context.Background(), document, options); err != nil {
		t.Fatalf("inclusive filter limit Render() error = %v", err)
	}

	scratch := &rasterScratch{offscreen: offscreenBudget{limit: want - 1}}
	rasterizerBytes, err := scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "test rasterizer")
	if err != nil {
		t.Fatal(err)
	}
	err = renderNode(context.Background(), image.NewRGBA(image.Rect(0, 0, 100, 100)), prepared.root, scratch)
	if err == nil || !strings.Contains(err.Error(), "exceeding limit") {
		t.Fatalf("runtime below-limit error = %v", err)
	}
	if scratch.offscreen.live != rasterizerBytes {
		t.Fatalf("failed filter render retained %d bytes, want rasterizer-only %d", scratch.offscreen.live, rasterizerBytes)
	}
	scratch.offscreen.release(rasterizerBytes)

	scratch = &rasterScratch{offscreen: offscreenBudget{limit: want}}
	rasterizerBytes, err = scratch.offscreen.reserveBytes(prepared.resources.rasterizerBytes, "test rasterizer")
	if err != nil {
		t.Fatal(err)
	}
	if err := renderNode(context.Background(), image.NewRGBA(image.Rect(0, 0, 100, 100)), prepared.root, scratch); err != nil {
		t.Fatalf("runtime exact-limit render: %v", err)
	}
	if scratch.offscreen.live != rasterizerBytes || scratch.offscreen.peak != want {
		t.Fatalf("runtime accounting live=%d peak=%d, want live=%d peak=%d", scratch.offscreen.live, scratch.offscreen.peak, rasterizerBytes, want)
	}
	scratch.offscreen.release(rasterizerBytes)
}

func TestGaussianBlurCancellationReleasesLayers(t *testing.T) {
	t.Parallel()

	node := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{Width: 128, Height: 128}, Fill: red,
	})
	node.Filters = []d2scene.Filter{d2scene.GaussianBlur{SigmaX: 4, SigmaY: 4}}
	prepared, err := prepare(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 128, Height: 128}, node), testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.root.filters) != 1 {
		t.Fatalf("prepared filters = %d, want 1", len(prepared.root.filters))
	}
	filterPeak, _, err := planFilterResources(prepared.root.filters, prepared.root.contentBounds)
	if err != nil {
		t.Fatal(err)
	}
	scratch := &rasterScratch{offscreen: offscreenBudget{limit: filterPeak}}
	input, err := reserveRGBA(scratch, prepared.root.contentBounds, "test filter input")
	if err != nil {
		t.Fatal(err)
	}
	for offset := 3; offset < len(input.image.Pix); offset += 4 {
		input.image.Pix[offset] = 255
	}
	_, err = applyPreparedFilter(&cancelAfterContext{after: 2}, input, prepared.root.filters[0], scratch)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Gaussian error = %v, want context.Canceled", err)
	}
	if scratch.offscreen.live != 0 {
		t.Fatalf("canceled Gaussian retained %d offscreen bytes", scratch.offscreen.live)
	}

	input, err = reserveRGBA(scratch, prepared.root.contentBounds, "retry filter input")
	if err != nil {
		t.Fatal(err)
	}
	output, err := applyPreparedFilter(context.Background(), input, prepared.root.filters[0], scratch)
	if err != nil {
		t.Fatalf("retry Gaussian filter: %v", err)
	}
	if scratch.offscreen.live != output.reservation {
		t.Fatalf("retry live bytes = %d, want output reservation %d", scratch.offscreen.live, output.reservation)
	}
	output.release()
	if scratch.offscreen.live != 0 {
		t.Fatalf("released retry retained %d bytes", scratch.offscreen.live)
	}
}
