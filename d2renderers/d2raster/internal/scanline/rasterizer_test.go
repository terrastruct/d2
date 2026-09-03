package scanline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
	"time"
)

func addWorstCaseCubic(rasterizer *Rasterizer) {
	rasterizer.MoveTo(0, 1e8)
	rasterizer.CubeTo(0, 9e8, 1e20, 9e8, 1e20, 1e8)
}

func addLargeCubic(rasterizer *Rasterizer) {
	rasterizer.MoveTo(0, 10)
	rasterizer.CubeTo(0, 1000, 1000, 1000, 1000, 10)
}

func TestCounterRejectsUnresolvedCubic(t *testing.T) {
	rasterizer := NewCounter(1, 1<<30, 1<<maxCurveDepth)
	addWorstCaseCubic(rasterizer)
	if !errors.Is(rasterizer.Err(), ErrCurveLimit) {
		t.Fatalf("error = %v, want ErrCurveLimit", rasterizer.Err())
	}
}

func TestCounterStopsAtEdgeLimitWithoutAllocating(t *testing.T) {
	const limit = 7
	rasterizer := NewCounter(1001, 2000, limit)
	addLargeCubic(rasterizer)
	if got := rasterizer.EdgeCount(); got != limit {
		t.Fatalf("edges = %d, want capped count %d", got, limit)
	}
	if !errors.Is(rasterizer.Err(), ErrEdgeLimit) {
		t.Fatalf("error = %v, want ErrEdgeLimit", rasterizer.Err())
	}
	rasterizer.LineTo(0, 20)
	addLargeCubic(rasterizer)
	if got := rasterizer.EdgeCount(); got != limit {
		t.Fatalf("edges after additional geometry = %d, want capped count %d", got, limit)
	}
	if cap(rasterizer.edges) != 0 || cap(rasterizer.scanEdges) != 0 || cap(rasterizer.active) != 0 {
		t.Fatalf("counter allocated raster storage: edges=%d scanEdges=%d active=%d", cap(rasterizer.edges), cap(rasterizer.scanEdges), cap(rasterizer.active))
	}
	if allocs := testing.AllocsPerRun(100, func() {
		rasterizer.Reset(1001, 2000)
		addLargeCubic(rasterizer)
	}); allocs != 0 {
		t.Fatalf("counter allocations = %g, want 0", allocs)
	}
}

func TestCubeFlatnessRejectsChordOvershootAndRetracing(t *testing.T) {
	p0 := point{x: 0, y: 0}
	p3 := point{x: 10, y: 0}
	if cubeFlatEnough(p0, point{x: -100, y: 0.1}, point{x: 110, y: -0.1}, p3) {
		t.Fatal("overshooting retraced cubic was treated as its finite chord")
	}
	if !cubeFlatEnough(p0, point{x: 3, y: 0.1}, point{x: 7, y: -0.1}, p3) {
		t.Fatal("monotone cubic inside the finite-chord tolerance was rejected")
	}
}

func TestRetainedBytesCombinesIndependentDimensionsAndEdges(t *testing.T) {
	wide, wideOK := RetainedBytes(1000, 2, 2)
	tall, tallOK := RetainedBytes(2, 1000, 2)
	combined, combinedOK := RetainedBytes(1000, 1000, 2)
	if !wideOK || !tallOK || !combinedOK {
		t.Fatal("retained-byte calculation overflowed")
	}
	if combined <= wide || combined <= tall {
		t.Fatalf("combined storage = %d, want greater than wide %d and tall %d", combined, wide, tall)
	}
	base, baseOK := RetainedBytes(17, 23, 0)
	withEdges, edgesOK := RetainedBytes(17, 23, 7)
	if !baseOK || !edgesOK {
		t.Fatal("edge retained-byte calculation overflowed")
	}
	edgeBytes := withEdges - base
	if got := MaxEdgesForBytes(edgeBytes); got != 7 {
		t.Fatalf("inclusive edge capacity = %d, want 7", got)
	}
	if got := MaxEdgesForBytes(edgeBytes - 1); got != 6 {
		t.Fatalf("below-limit edge capacity = %d, want 6", got)
	}
}

func TestWorkBoundCoversRuntimeAccounting(t *testing.T) {
	const width, height = 64, 32
	newRasterizer := func() *Rasterizer {
		rasterizer := NewRasterizer(width, height)
		rasterizer.MoveTo(0, 0)
		rasterizer.LineTo(width, height)
		rasterizer.LineTo(0, height)
		rasterizer.ClosePath()
		return rasterizer
	}
	rasterizer := newRasterizer()
	bound, ok := rasterizer.WorkBound()
	if !ok || bound <= 0 {
		t.Fatalf("Rasterizer.WorkBound = %d, %v; want a positive bound", bound, ok)
	}
	worst, ok := WorkBound(width, height, rasterizer.EdgeCount())
	if !ok || bound > worst {
		t.Fatalf("geometry work bound %d exceeds worst-case bound %d", bound, worst)
	}

	budget := NewWorkBudget(bound)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, image.NewAlpha(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	spent := bound - budget.Remaining()
	if spent <= 0 || spent > bound {
		t.Fatalf("runtime work = %d, bound = %d", spent, bound)
	}

	below := NewWorkBudget(spent - 1)
	if err := newRasterizer().WriteAlpha(context.Background(), &below, image.NewAlpha(image.Rect(0, 0, width, height))); !errors.Is(err, ErrWorkLimit) {
		t.Fatalf("below-limit error = %v, want ErrWorkLimit", err)
	}
	exact := NewWorkBudget(spent)
	if err := newRasterizer().WriteAlpha(context.Background(), &exact, image.NewAlpha(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("inclusive runtime work limit: %v", err)
	}
	if exact.Remaining() != 0 {
		t.Fatalf("remaining exact budget = %d, want 0", exact.Remaining())
	}

	if _, ok := WorkBound(math.MaxInt, math.MaxInt, math.MaxInt); ok {
		t.Fatal("WorkBound accepted overflowing dimensions and edges")
	}
}

func TestGeometryWorkBoundAvoidsWholeTargetPerSmallPath(t *testing.T) {
	const width, height = 16_384, 16_384
	rasterizer := NewCounter(width, height, 10)
	addTestRectangle(rasterizer, 10, 10, 20, 20)
	bound, ok := rasterizer.WorkBound()
	if !ok {
		t.Fatal("small-path work bound overflowed")
	}
	if bound >= 100_000 {
		t.Fatalf("small-path work bound = %d, want below 100000", bound)
	}
	worst, ok := WorkBound(width, height, rasterizer.EdgeCount())
	if !ok || worst <= 500_000_000 {
		t.Fatalf("worst-case work bound = %d, %v; want a full-target safety bound above 500M", worst, ok)
	}
	adversarial, ok := WorkBound(width, height, 50_000)
	if !ok || adversarial <= 4_000_000_000 {
		t.Fatalf("adversarial work bound = %d, %v; want the 4B operation ceiling to remain meaningful", adversarial, ok)
	}
}

func TestRasterizeCancellationDuringScan(t *testing.T) {
	t.Run("wide painted row", func(t *testing.T) {
		const width, height = 8_192, 2
		rasterizer := NewRasterizer(width, height)
		addTestRectangle(rasterizer, 0, 0, width, height)
		bound, ok := rasterizer.WorkBound()
		if !ok {
			t.Fatal("work bound overflowed")
		}
		budget := NewWorkBudget(bound)
		destination := image.NewAlpha(image.Rect(0, 0, width, height))
		ctx := &cancelAfterErrContext{remaining: 3}
		err := rasterizer.WriteAlpha(ctx, &budget, destination)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WriteAlpha error = %v, want context.Canceled", err)
		}
		if destination.AlphaAt(100, 0).A == 0 || destination.AlphaAt(7_000, 0).A != 0 {
			t.Fatalf("cancellation was not observed mid-row: early=%d late=%d", destination.AlphaAt(100, 0).A, destination.AlphaAt(7_000, 0).A)
		}
	})

	t.Run("wide opaque RGBA span", func(t *testing.T) {
		const width, height = 8_192, 2
		rasterizer := NewRasterizer(width, height)
		addTestRectangle(rasterizer, 0, 0, width, height)
		bound, ok := rasterizer.WorkBound()
		if !ok {
			t.Fatal("work bound overflowed")
		}
		budget := NewWorkBudget(bound)
		destination := image.NewRGBA(image.Rect(0, 0, width, height))
		ctx := &cancelAfterErrContext{remaining: 3}
		err := rasterizer.DrawRGBA(ctx, &budget, destination, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("DrawRGBA error = %v, want context.Canceled", err)
		}
		if destination.RGBAAt(100, 0).A == 0 || destination.RGBAAt(7_000, 0).A != 0 {
			t.Fatalf("cancellation was not observed between opaque spans: early=%d late=%d", destination.RGBAAt(100, 0).A, destination.RGBAAt(7_000, 0).A)
		}
	})

	t.Run("large active edge set", func(t *testing.T) {
		const rectangles = 5_000
		rasterizer := NewRasterizer(2, 2)
		for range rectangles {
			addTestRectangle(rasterizer, 0, 0, 1, 2)
		}
		bound, ok := rasterizer.WorkBound()
		if !ok {
			t.Fatal("work bound overflowed")
		}
		budget := NewWorkBudget(bound)
		ctx := &cancelAfterErrContext{remaining: 7}
		err := rasterizer.WriteAlpha(ctx, &budget, image.NewAlpha(image.Rect(0, 0, 2, 2)))
		if !errors.Is(err, context.Canceled) || ctx.checks < 7 {
			t.Fatalf("WriteAlpha error/checks = %v/%d, want mid-active context cancellation", err, ctx.checks)
		}
	})
}

func TestResetAfterCanceledRasterizeMatchesFreshRasterizer(t *testing.T) {
	t.Parallel()

	const width, height = 8_192, 2
	rasterizer := NewRasterizer(width, height)
	addTestRectangle(rasterizer, .25, .25, width-.25, height-.25)
	budget := NewWorkBudget(math.MaxInt64)
	err := rasterizer.WriteAlpha(&cancelAfterErrContext{remaining: 3}, &budget, image.NewAlpha(image.Rect(0, 0, width, height)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted WriteAlpha error = %v, want context.Canceled", err)
	}

	// Exercise both shrinking and regrowing within the retained capacity.
	rasterizer.Reset(64, height)
	rasterizer.Reset(width, height)
	addTestRectangle(rasterizer, .25, .25, width-.25, height-.25)
	got := image.NewAlpha(image.Rect(0, 0, width, height))
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, got); err != nil {
		t.Fatal(err)
	}

	fresh := NewRasterizer(width, height)
	addTestRectangle(fresh, .25, .25, width-.25, height-.25)
	want := image.NewAlpha(got.Bounds())
	budget = NewWorkBudget(math.MaxInt64)
	if err := fresh.WriteAlpha(context.Background(), &budget, want); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("reset after interruption retained scanline accumulation")
	}
}

func TestWalkCoverageMatchesWriteAlphaRandomized(t *testing.T) {
	t.Parallel()

	const width, height = 73, 59
	state := uint64(0x9e3779b97f4a7c15)
	next := func() uint64 {
		state ^= state >> 12
		state ^= state << 25
		state ^= state >> 27
		return state * 0x2545f4914f6cdd1d
	}
	coordinate := func(limit int) float32 {
		// Include fractional geometry and points just outside every target edge.
		value := float64(int64(next()%uint64(limit*8+32))-16) / 8
		return float32(value)
	}

	for iteration := range 250 {
		rasterizer := NewRasterizer(width, height)
		for range int(next()%9) + 1 {
			points := int(next()%8) + 2
			rasterizer.MoveTo(coordinate(width), coordinate(height))
			for range points - 1 {
				rasterizer.LineTo(coordinate(width), coordinate(height))
			}
			rasterizer.ClosePath()
		}

		want := image.NewAlpha(image.Rect(0, 0, width, height))
		budget := NewWorkBudget(math.MaxInt64)
		if err := rasterizer.WriteAlpha(context.Background(), &budget, want); err != nil {
			t.Fatalf("iteration %d WriteAlpha: %v", iteration, err)
		}

		got := image.NewAlpha(want.Bounds())
		budget = NewWorkBudget(math.MaxInt64)
		err := rasterizer.WalkCoverage(context.Background(), &budget, func(y, minX int, partial, difference []float32) error {
			row := got.PixOffset(minX, y)
			var winding float32
			for index := range partial {
				winding += difference[index]
				got.Pix[row+index] = QuantizeCoverage(partial[index] + winding)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("iteration %d WalkCoverage: %v", iteration, err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			for index := range got.Pix {
				if got.Pix[index] != want.Pix[index] {
					t.Fatalf("iteration %d first differing pixel %d: WalkCoverage=%d WriteAlpha=%d", iteration, index, got.Pix[index], want.Pix[index])
				}
			}
			t.Fatalf("iteration %d output lengths differ: WalkCoverage=%d WriteAlpha=%d", iteration, len(got.Pix), len(want.Pix))
		}
	}
}

func TestRasterizerReuseAfterCanceledPreparationMatchesFresh(t *testing.T) {
	t.Parallel()

	const width, height = 32, 512
	addPaths := func(rasterizer *Rasterizer) {
		// More than one context-check interval ensures cancellation happens
		// after prepareScanEdges has populated some row heads.
		for index := range 48 {
			y := float32(index*10) + .25
			addTestRectangle(rasterizer, 2.25, y, 29.75, y+6.5)
		}
	}

	rasterizer := NewRasterizer(width, height)
	addPaths(rasterizer)
	budget := NewWorkBudget(math.MaxInt64)
	err := rasterizer.WriteAlpha(&cancelAfterErrContext{remaining: 1}, &budget, image.NewAlpha(image.Rect(0, 0, width, height)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted WriteAlpha error = %v, want context.Canceled", err)
	}

	got := image.NewAlpha(image.Rect(0, 0, width, height))
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, got); err != nil {
		t.Fatal(err)
	}

	fresh := NewRasterizer(width, height)
	addPaths(fresh)
	want := image.NewAlpha(got.Bounds())
	budget = NewWorkBudget(math.MaxInt64)
	if err := fresh.WriteAlpha(context.Background(), &budget, want); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("reuse after canceled scan-edge preparation differs from a fresh rasterizer")
	}
}

func TestRasterizerCancellationWhilePreparingOnlyInvisibleEdges(t *testing.T) {
	t.Parallel()

	const width, height = 8, 8
	rasterizer := NewRasterizer(width, height)
	// Each rectangle contributes four raw edges. Cross a preparation context
	// checkpoint without ever publishing a visible row head.
	for index := 0; index < contextCheckInterval/4+1; index++ {
		y := -float32(index*2 + 2)
		addTestRectangle(rasterizer, 1, y-1, 7, y)
	}
	budget := NewWorkBudget(math.MaxInt64)
	err := rasterizer.WriteAlpha(&cancelAfterErrContext{remaining: 1}, &budget, image.NewAlpha(image.Rect(0, 0, width, height)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted WriteAlpha error = %v, want context.Canceled", err)
	}

	rasterizer.Reset(width, height)
	addTestRectangle(rasterizer, 1, 1, 7, 7)
	destination := image.NewAlpha(image.Rect(0, 0, width, height))
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, destination); err != nil {
		t.Fatal(err)
	}
	if destination.AlphaAt(4, 4).A == 0 {
		t.Fatal("rasterizer was not reusable after canceled invisible-edge preparation")
	}
}

func TestRasterizerRowHeadsSurviveHeightShrinkAndRegrow(t *testing.T) {
	t.Parallel()

	const width, height = 32, 16_384
	rasterizer := NewRasterizer(width, height)
	addTestRectangle(rasterizer, 2.25, 8.25, 29.75, 55.75)
	budget := NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, image.NewAlpha(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}

	rasterizer.Reset(width, 16)
	addTestRectangle(rasterizer, 2.25, 1.25, 29.75, 14.75)
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, image.NewAlpha(image.Rect(0, 0, width, 16))); err != nil {
		t.Fatal(err)
	}

	rasterizer.Reset(width, height)
	addTestRectangle(rasterizer, 2.25, height-55.75, 29.75, height-8.25)
	got := image.NewAlpha(image.Rect(0, 0, width, height))
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, got); err != nil {
		t.Fatal(err)
	}

	fresh := NewRasterizer(width, height)
	addTestRectangle(fresh, 2.25, height-55.75, 29.75, height-8.25)
	want := image.NewAlpha(got.Bounds())
	budget = NewWorkBudget(math.MaxInt64)
	if err := fresh.WriteAlpha(context.Background(), &budget, want); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("height shrink and regrow differs from a fresh rasterizer")
	}
}

func TestDrawRGBAOpaqueFullCoverageMatchesReferenceArithmeticExhaustively(t *testing.T) {
	t.Parallel()

	// Full coverage from an opaque NRGBA paint makes source-over independent
	// of the destination. Pin that fast-path identity against the exact integer
	// arithmetic of the general source-over path for every byte value.
	const full = uint32(0xffff)
	for source := uint32(0); source <= 0xff; source++ {
		source16 := source * 0x101
		for destination := uint32(0); destination <= 0xff; destination++ {
			inverse := (full - full*full/full) * 0x101
			reference := uint8((destination*inverse + source16*full) / full >> 8)
			if reference != uint8(source) {
				t.Fatalf("reference opaque channel source=%d destination=%d produced %d", source, destination, reference)
			}
		}
	}
	for destinationAlpha := uint32(0); destinationAlpha <= 0xff; destinationAlpha++ {
		inverse := (full - full*full/full) * 0x101
		reference := uint8((destinationAlpha*inverse + full*full) / full >> 8)
		if reference != 0xff {
			t.Fatalf("reference opaque alpha destination=%d produced %d", destinationAlpha, reference)
		}
	}

	rasterizer := NewRasterizer(256, 1)
	addTestRectangle(rasterizer, 0, 0, 256, 1)
	destination := image.NewRGBA(image.Rect(0, 0, 256, 1))
	for source := range 256 {
		for x := range 256 {
			offset := destination.PixOffset(x, 0)
			destination.Pix[offset], destination.Pix[offset+1], destination.Pix[offset+2], destination.Pix[offset+3] = uint8(x), uint8(255-x), uint8(x^0x55), uint8(x)
		}
		budget := NewWorkBudget(math.MaxInt64)
		paint := color.NRGBA{R: uint8(source), G: uint8(255 - source), B: uint8(source ^ 0xa5), A: 255}
		if err := rasterizer.DrawRGBA(context.Background(), &budget, destination, paint); err != nil {
			t.Fatal(err)
		}
		for x := range 256 {
			if got := destination.RGBAAt(x, 0); got != (color.RGBA{R: paint.R, G: paint.G, B: paint.B, A: 255}) {
				t.Fatalf("opaque full-coverage source=%d destination=%d: got %#v, want paint %#v", source, x, got, paint)
			}
		}
	}
}

func TestDrawRGBAOpaqueFastPathsMatchCoverageReference(t *testing.T) {
	t.Parallel()

	const width, height = 257, 73
	optimized := image.NewRGBA(image.Rect(0, 0, width, height))
	reference := image.NewRGBA(optimized.Bounds())
	state := uint32(42)
	for index := range optimized.Pix {
		state = state*1664525 + 1013904223
		optimized.Pix[index] = uint8(state >> 24)
	}
	copy(reference.Pix, optimized.Pix)

	paint := color.NRGBA{R: 0x37, G: 0xa9, B: 0xe2, A: 0xff}
	rasterizer := NewRasterizer(width, height)
	addFastPathTestGeometry(rasterizer, width, height)
	budget := NewWorkBudget(math.MaxInt64)
	if err := rasterizer.DrawRGBA(context.Background(), &budget, optimized, paint); err != nil {
		t.Fatal(err)
	}

	mask := image.NewAlpha(optimized.Bounds())
	rasterizer = NewRasterizer(width, height)
	addFastPathTestGeometry(rasterizer, width, height)
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, mask); err != nil {
		t.Fatal(err)
	}
	sr, sg, sb, sa := paint.RGBA()
	for y := range height {
		for x := range width {
			coverage := uint32(mask.AlphaAt(x, y).A)
			if coverage == 0 {
				continue
			}
			coverage |= coverage << 8
			inverse := (uint32(0xffff) - sa*coverage/0xffff) * 0x101
			offset := reference.PixOffset(x, y)
			reference.Pix[offset] = uint8((uint32(reference.Pix[offset])*inverse + sr*coverage) / 0xffff >> 8)
			reference.Pix[offset+1] = uint8((uint32(reference.Pix[offset+1])*inverse + sg*coverage) / 0xffff >> 8)
			reference.Pix[offset+2] = uint8((uint32(reference.Pix[offset+2])*inverse + sb*coverage) / 0xffff >> 8)
			reference.Pix[offset+3] = uint8((uint32(reference.Pix[offset+3])*inverse + sa*coverage) / 0xffff >> 8)
		}
	}
	if !bytes.Equal(optimized.Pix, reference.Pix) {
		t.Fatal("opaque fast paths differ from coverage-composited reference")
	}
}

func TestDrawRGBAOpaqueDensePathMatchesCoverageReference(t *testing.T) {
	t.Parallel()

	const width, height = 257, 73
	bounds := image.Rect(-11, 7, -11+width, 7+height)
	stride := width*4 + 13
	optimized := &image.RGBA{Pix: make([]byte, stride*height+5), Stride: stride, Rect: bounds}
	reference := &image.RGBA{Pix: make([]byte, len(optimized.Pix)), Stride: stride, Rect: bounds}
	state := uint32(42)
	for index := range optimized.Pix {
		state = state*1664525 + 1013904223
		optimized.Pix[index] = uint8(state >> 24)
	}
	copy(reference.Pix, optimized.Pix)

	addDenseGeometry := func(rasterizer *Rasterizer) {
		for index := range 128 {
			x := float32(index%32)*7.75 - 2.25
			y := float32(index%16)*4.125 - 1.75
			addTestRectangle(rasterizer, x, y, x+19.5, y+11.25)
		}
	}
	paint := color.NRGBA{R: 0x37, G: 0xa9, B: 0xe2, A: 0xff}
	rasterizer := NewRasterizer(width, height)
	addDenseGeometry(rasterizer)
	if rasterizer.EdgeCount() <= height {
		t.Fatalf("dense geometry has %d edges, want more than height %d", rasterizer.EdgeCount(), height)
	}
	budget := NewWorkBudget(math.MaxInt64)
	if err := rasterizer.DrawRGBA(context.Background(), &budget, optimized, paint); err != nil {
		t.Fatal(err)
	}

	mask := &image.Alpha{Pix: make([]byte, (width+7)*height), Stride: width + 7, Rect: bounds}
	rasterizer = NewRasterizer(width, height)
	addDenseGeometry(rasterizer)
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, mask); err != nil {
		t.Fatal(err)
	}
	sr, sg, sb, sa := paint.RGBA()
	for y := range height {
		for x := range width {
			coverage := uint32(mask.AlphaAt(bounds.Min.X+x, bounds.Min.Y+y).A)
			if coverage == 0 {
				continue
			}
			coverage |= coverage << 8
			inverse := (uint32(0xffff) - sa*coverage/0xffff) * 0x101
			offset := reference.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)
			reference.Pix[offset] = uint8((uint32(reference.Pix[offset])*inverse + sr*coverage) / 0xffff >> 8)
			reference.Pix[offset+1] = uint8((uint32(reference.Pix[offset+1])*inverse + sg*coverage) / 0xffff >> 8)
			reference.Pix[offset+2] = uint8((uint32(reference.Pix[offset+2])*inverse + sb*coverage) / 0xffff >> 8)
			reference.Pix[offset+3] = uint8((uint32(reference.Pix[offset+3])*inverse + sa*coverage) / 0xffff >> 8)
		}
	}
	if !bytes.Equal(optimized.Pix, reference.Pix) {
		t.Fatal("dense opaque path differs from coverage-composited reference")
	}
}

func TestWriteAlphaSparseSpansMatchScalarReference(t *testing.T) {
	t.Parallel()

	const width, height = 257, 73
	bounds := image.Rect(-11, 7, -11+width, 7+height)
	stride := width + 13
	optimized := &image.Alpha{Pix: make([]byte, stride*height), Stride: stride, Rect: bounds}
	reference := &image.Alpha{Pix: make([]byte, stride*height), Stride: stride, Rect: bounds}
	for y := range height {
		for offset := width; offset < stride; offset++ {
			optimized.Pix[y*stride+offset] = 0xa5
			reference.Pix[y*stride+offset] = 0xa5
		}
	}

	rasterizer := NewRasterizer(width, height)
	addFastPathTestGeometry(rasterizer, width, height)
	budget := NewWorkBudget(math.MaxInt64)
	if err := rasterizer.WriteAlpha(context.Background(), &budget, optimized); err != nil {
		t.Fatal(err)
	}

	rasterizer = NewRasterizer(width, height)
	addFastPathTestGeometry(rasterizer, width, height)
	budget = NewWorkBudget(math.MaxInt64)
	if err := rasterizer.rasterize(context.Background(), &budget, func(y, minX, maxX int) error {
		row := reference.PixOffset(reference.Rect.Min.X+minX, reference.Rect.Min.Y+y)
		var winding float32
		for x := minX; x < maxX; x++ {
			winding += rasterizer.difference[x]
			reference.Pix[row] = referenceCoverage(rasterizer.partial[x] + winding)
			row++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(optimized.Pix, reference.Pix) {
		t.Fatal("sparse Alpha spans differ from scalar coverage, including row padding")
	}
}

func TestSparseSpanFastPathsMatchScalarReference(t *testing.T) {
	t.Parallel()

	const width, height = 2*contextCheckInterval + 257, 7
	bounds := image.Rect(-23, 11, -23+width, 11+height)

	t.Run("Alpha", func(t *testing.T) {
		stride := width + 17
		optimized := &image.Alpha{Pix: make([]byte, stride*height+9), Stride: stride, Rect: bounds}
		fillDeterministicBytes(optimized.Pix, 0x13579bdf)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			clear(optimized.Pix[optimized.PixOffset(bounds.Min.X, y):optimized.PixOffset(bounds.Max.X, y)])
		}
		reference := &image.Alpha{Pix: append([]byte(nil), optimized.Pix...), Stride: stride, Rect: bounds}

		rasterizer := NewRasterizer(width, height)
		addSparseSpanTestGeometry(rasterizer, width, height)
		if !isSparseSpanPath(rasterizer, height) {
			t.Fatalf("geometry has %d edges, does not select the sparse span path", rasterizer.EdgeCount())
		}
		budget := NewWorkBudget(math.MaxInt64)
		if err := rasterizer.WriteAlpha(context.Background(), &budget, optimized); err != nil {
			t.Fatal(err)
		}

		rasterizer = NewRasterizer(width, height)
		addSparseSpanTestGeometry(rasterizer, width, height)
		budget = NewWorkBudget(math.MaxInt64)
		if err := writeAlphaScalarReference(context.Background(), rasterizer, &budget, reference); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(optimized.Pix, reference.Pix) {
			t.Fatal("sparse Alpha span output differs from the scalar reference, including padding and trailing storage")
		}
		assertSpanCoverageClasses(t, optimized)
	})

	for _, test := range []struct {
		name  string
		paint color.NRGBA
	}{
		{name: "Opaque", paint: color.NRGBA{R: 0x19, G: 0xa7, B: 0xe3, A: 0xff}},
		{name: "Translucent", paint: color.NRGBA{R: 0xed, G: 0x42, B: 0x76, A: 0x9d}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			stride := width*4 + 29
			optimized := &image.RGBA{Pix: make([]byte, stride*height+13), Stride: stride, Rect: bounds}
			fillDeterministicBytes(optimized.Pix, 0x2468ace0)
			reference := &image.RGBA{Pix: append([]byte(nil), optimized.Pix...), Stride: stride, Rect: bounds}

			rasterizer := NewRasterizer(width, height)
			addSparseSpanTestGeometry(rasterizer, width, height)
			if !isSparseSpanPath(rasterizer, height) {
				t.Fatalf("geometry has %d edges, does not select the sparse span path", rasterizer.EdgeCount())
			}
			budget := NewWorkBudget(math.MaxInt64)
			if err := rasterizer.DrawRGBA(context.Background(), &budget, optimized, test.paint); err != nil {
				t.Fatal(err)
			}

			rasterizer = NewRasterizer(width, height)
			addSparseSpanTestGeometry(rasterizer, width, height)
			budget = NewWorkBudget(math.MaxInt64)
			if err := drawRGBAScalarReference(context.Background(), rasterizer, &budget, reference, test.paint); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(optimized.Pix, reference.Pix) {
				t.Fatal("sparse RGBA span output differs from the scalar reference, including arbitrary destination pixels, padding, and trailing storage")
			}
		})
	}
}

func TestSparseSpanFastPathsMatchScalarReferenceRandomized(t *testing.T) {
	t.Parallel()

	type rectangle struct {
		x0, y0, x1, y1 float32
	}
	widths := [...]int{8, 16, 31, 64, 255, 256, 257, 513, contextCheckInterval + 17}
	heights := [...]int{1, 7, 16, 73}
	state := uint32(0x3c6ef372)
	next := func() uint32 {
		state = state*1664525 + 1013904223
		return state
	}
	coordinate := func(extent int) float32 {
		whole := int(next()%uint32(extent*2+9)) - extent/2 - 4
		return float32(whole) + float32(next()&7)/8
	}

	for iteration := range 128 {
		width := widths[next()%uint32(len(widths))]
		height := heights[next()%uint32(len(heights))]
		bounds := image.Rect(-int(next()%17), -int(next()%13), 0, 0)
		bounds.Max = bounds.Min.Add(image.Pt(width, height))
		rectangles := make([]rectangle, 1+next()%2)
		for index := range rectangles {
			rectangles[index] = rectangle{
				x0: coordinate(width),
				y0: coordinate(height),
				x1: coordinate(width),
				y1: coordinate(height),
			}
			if rectangles[index].x0 == rectangles[index].x1 {
				rectangles[index].x1 += .5
			}
			if rectangles[index].y0 == rectangles[index].y1 {
				rectangles[index].y1 += .5
			}
		}
		addGeometry := func(rasterizer *Rasterizer) {
			for _, rectangle := range rectangles {
				addTestRectangle(rasterizer, rectangle.x0, rectangle.y0, rectangle.x1, rectangle.y1)
			}
		}

		for _, paint := range []color.NRGBA{
			{R: uint8(next()), G: uint8(next()), B: uint8(next()), A: 0xff},
			{R: uint8(next()), G: uint8(next()), B: uint8(next()), A: uint8(next()%254 + 1)},
		} {
			stride := width*4 + int(next()%19)
			optimized := &image.RGBA{Pix: make([]byte, stride*height+int(next()%11)), Stride: stride, Rect: bounds}
			fillDeterministicBytes(optimized.Pix, next())
			reference := &image.RGBA{Pix: append([]byte(nil), optimized.Pix...), Stride: stride, Rect: bounds}

			rasterizer := NewRasterizer(width, height)
			addGeometry(rasterizer)
			if !isSparseSpanPath(rasterizer, height) {
				t.Fatalf("iteration %d geometry has %d edges, does not select sparse RGBA path", iteration, rasterizer.EdgeCount())
			}
			budget := NewWorkBudget(math.MaxInt64)
			if err := rasterizer.DrawRGBA(context.Background(), &budget, optimized, paint); err != nil {
				t.Fatalf("iteration %d optimized RGBA: %v", iteration, err)
			}

			rasterizer = NewRasterizer(width, height)
			addGeometry(rasterizer)
			budget = NewWorkBudget(math.MaxInt64)
			if err := drawRGBAScalarReference(context.Background(), rasterizer, &budget, reference, paint); err != nil {
				t.Fatalf("iteration %d scalar RGBA: %v", iteration, err)
			}
			if !bytes.Equal(optimized.Pix, reference.Pix) {
				t.Fatalf("iteration %d sparse RGBA output differs from scalar reference (width=%d height=%d alpha=%d)", iteration, width, height, paint.A)
			}
		}

		stride := width + int(next()%19)
		optimized := &image.Alpha{Pix: make([]byte, stride*height+int(next()%11)), Stride: stride, Rect: bounds}
		fillDeterministicBytes(optimized.Pix, next())
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			clear(optimized.Pix[optimized.PixOffset(bounds.Min.X, y):optimized.PixOffset(bounds.Max.X, y)])
		}
		reference := &image.Alpha{Pix: append([]byte(nil), optimized.Pix...), Stride: stride, Rect: bounds}

		rasterizer := NewRasterizer(width, height)
		addGeometry(rasterizer)
		budget := NewWorkBudget(math.MaxInt64)
		if err := rasterizer.WriteAlpha(context.Background(), &budget, optimized); err != nil {
			t.Fatalf("iteration %d optimized Alpha: %v", iteration, err)
		}
		rasterizer = NewRasterizer(width, height)
		addGeometry(rasterizer)
		budget = NewWorkBudget(math.MaxInt64)
		if err := writeAlphaScalarReference(context.Background(), rasterizer, &budget, reference); err != nil {
			t.Fatalf("iteration %d scalar Alpha: %v", iteration, err)
		}
		if !bytes.Equal(optimized.Pix, reference.Pix) {
			t.Fatalf("iteration %d sparse Alpha output differs from scalar reference (width=%d height=%d)", iteration, width, height)
		}
	}
}

func TestSparseSpanFastPathsMatchScalarCancellationCadenceAndPrefix(t *testing.T) {
	const width, height = 2*contextCheckInterval + 257, 7
	bounds := image.Rect(19, -31, 19+width, -31+height)

	compareAlphaCancellation := func(t *testing.T, cancelAt int) (int, error) {
		t.Helper()
		stride := width + 11
		initial := make([]byte, stride*height+7)
		fillDeterministicBytes(initial, 0x9e3779b9)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			clear(initial[(y-bounds.Min.Y)*stride : (y-bounds.Min.Y)*stride+width])
		}
		optimized, reference := append([]byte(nil), initial...), append([]byte(nil), initial...)
		optimizedContext, referenceContext := &cancelAtErrContext{cancelAt: cancelAt}, &cancelAtErrContext{cancelAt: cancelAt}

		optimizedRasterizer := NewRasterizer(width, height)
		addSparseSpanTestGeometry(optimizedRasterizer, width, height)
		optimizedBudget := NewWorkBudget(math.MaxInt64)
		optimizedErr := optimizedRasterizer.WriteAlpha(optimizedContext, &optimizedBudget, &image.Alpha{Pix: optimized, Stride: stride, Rect: bounds})

		referenceRasterizer := NewRasterizer(width, height)
		addSparseSpanTestGeometry(referenceRasterizer, width, height)
		referenceBudget := NewWorkBudget(math.MaxInt64)
		referenceErr := writeAlphaScalarReference(referenceContext, referenceRasterizer, &referenceBudget, &image.Alpha{Pix: reference, Stride: stride, Rect: bounds})

		assertCancellationEquivalence(t, cancelAt, optimizedContext, referenceContext, optimizedErr, referenceErr, optimized, reference)
		return referenceContext.checks, referenceErr
	}

	compareRGBACancellation := func(t *testing.T, paint color.NRGBA, cancelAt int) (int, error) {
		t.Helper()
		stride := width*4 + 23
		initial := make([]byte, stride*height+5)
		fillDeterministicBytes(initial, 0x7f4a7c15)
		optimized, reference := append([]byte(nil), initial...), append([]byte(nil), initial...)
		optimizedContext, referenceContext := &cancelAtErrContext{cancelAt: cancelAt}, &cancelAtErrContext{cancelAt: cancelAt}

		optimizedRasterizer := NewRasterizer(width, height)
		addSparseSpanTestGeometry(optimizedRasterizer, width, height)
		optimizedBudget := NewWorkBudget(math.MaxInt64)
		optimizedErr := optimizedRasterizer.DrawRGBA(optimizedContext, &optimizedBudget, &image.RGBA{Pix: optimized, Stride: stride, Rect: bounds}, paint)

		referenceRasterizer := NewRasterizer(width, height)
		addSparseSpanTestGeometry(referenceRasterizer, width, height)
		referenceBudget := NewWorkBudget(math.MaxInt64)
		referenceErr := drawRGBAScalarReference(referenceContext, referenceRasterizer, &referenceBudget, &image.RGBA{Pix: reference, Stride: stride, Rect: bounds}, paint)

		assertCancellationEquivalence(t, cancelAt, optimizedContext, referenceContext, optimizedErr, referenceErr, optimized, reference)
		return referenceContext.checks, referenceErr
	}

	for _, test := range []struct {
		name string
		run  func(*testing.T, int) (int, error)
	}{
		{name: "Alpha", run: compareAlphaCancellation},
		{name: "OpaqueRGBA", run: func(t *testing.T, cancelAt int) (int, error) {
			return compareRGBACancellation(t, color.NRGBA{R: 0x21, G: 0x83, B: 0xd4, A: 0xff}, cancelAt)
		}},
		{name: "TranslucentRGBA", run: func(t *testing.T, cancelAt int) (int, error) {
			return compareRGBACancellation(t, color.NRGBA{R: 0xd7, G: 0x34, B: 0x65, A: 0xa3}, cancelAt)
		}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			checks, err := test.run(t, 0)
			if err != nil {
				t.Fatalf("uncanceled reference error = %v", err)
			}
			if checks < 2*(width/contextCheckInterval) {
				t.Fatalf("only %d context checks for width %d; test did not exercise multiple mid-row boundaries", checks, width)
			}
			for cancelAt := 1; cancelAt <= checks+1; cancelAt++ {
				t.Run(fmt.Sprintf("CancelAt%d", cancelAt), func(t *testing.T) {
					test.run(t, cancelAt)
				})
			}
		})
	}
}

func addSparseSpanTestGeometry(rasterizer *Rasterizer, width, height int) {
	// An outer contour, reversed hole, and island produce long full, zero, and
	// fractional-coverage spans while staying under the sparse-path edge limit.
	addTestRectangle(rasterizer, 16.25, .25, float32(width)-16.25, float32(height)-.25)
	addTestRectangle(rasterizer, float32(width)-1024.25, .75, 1024.25, float32(height)-.75)
	addTestRectangle(rasterizer, 4094.25, 1.25, 4102.75, float32(height)-1.25)
}

func isSparseSpanPath(rasterizer *Rasterizer, height int) bool {
	return len(rasterizer.edges) <= max(height, 8)
}

func fillDeterministicBytes(pixels []byte, state uint32) {
	for index := range pixels {
		state = state*1664525 + 1013904223
		pixels[index] = uint8(state >> 24)
	}
}

func assertSpanCoverageClasses(t *testing.T, mask *image.Alpha) {
	t.Helper()
	classes := [3]bool{}
	longRuns := [3]bool{}
	for y := mask.Rect.Min.Y; y < mask.Rect.Max.Y; y++ {
		lastClass, run := -1, 0
		for x := mask.Rect.Min.X; x < mask.Rect.Max.X; x++ {
			value := mask.AlphaAt(x, y).A
			class := 1
			if value == 0 {
				class = 0
			} else if value == 0xff {
				class = 2
			}
			classes[class] = true
			if class == lastClass {
				run++
			} else {
				lastClass, run = class, 1
			}
			if run >= 32 {
				longRuns[class] = true
			}
		}
	}
	if classes != [3]bool{true, true, true} || longRuns != [3]bool{true, true, true} {
		t.Fatalf("coverage classes present=%v long-runs=%v, want zero/partial/full", classes, longRuns)
	}
}

func assertCancellationEquivalence(t *testing.T, cancelAt int, optimizedContext, referenceContext *cancelAtErrContext, optimizedErr, referenceErr error, optimized, reference []byte) {
	t.Helper()
	if !errors.Is(optimizedErr, referenceErr) || !errors.Is(referenceErr, optimizedErr) {
		t.Fatalf("cancelAt=%d errors differ: optimized=%v reference=%v", cancelAt, optimizedErr, referenceErr)
	}
	if optimizedContext.checks != referenceContext.checks {
		t.Fatalf("cancelAt=%d context checks differ: optimized=%d reference=%d", cancelAt, optimizedContext.checks, referenceContext.checks)
	}
	if !bytes.Equal(optimized, reference) {
		for index := range optimized {
			if optimized[index] != reference[index] {
				t.Fatalf("cancelAt=%d first differing byte %d: optimized=%#02x reference=%#02x", cancelAt, index, optimized[index], reference[index])
			}
		}
		t.Fatalf("cancelAt=%d output lengths differ: optimized=%d reference=%d", cancelAt, len(optimized), len(reference))
	}
}

func drawRGBAScalarReference(ctx context.Context, rasterizer *Rasterizer, budget *WorkBudget, dst *image.RGBA, paint color.NRGBA) error {
	if err := rasterizer.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dst == nil || rasterizer.width == 0 || rasterizer.height == 0 || len(rasterizer.edges) == 0 || paint.A == 0 {
		return nil
	}
	sr, sg, sb, sa := paint.RGBA()
	return rasterizer.rasterize(ctx, budget, func(y, minX, maxX int) error {
		row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
		var winding float32
		for x := minX; x < maxX; {
			end := min(x+contextCheckInterval, maxX)
			if x != minX {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			for ; x < end; x++ {
				winding += rasterizer.difference[x]
				pixelCoverage := uint32(referenceCoverage(rasterizer.partial[x] + winding))
				if pixelCoverage == 0 {
					row += 4
					continue
				}
				pixelCoverage |= pixelCoverage << 8
				inverse := (uint32(0xffff) - sa*pixelCoverage/0xffff) * 0x101
				pixel := dst.Pix[row : row+4 : row+4]
				pixel[0] = uint8((uint32(pixel[0])*inverse + sr*pixelCoverage) / 0xffff >> 8)
				pixel[1] = uint8((uint32(pixel[1])*inverse + sg*pixelCoverage) / 0xffff >> 8)
				pixel[2] = uint8((uint32(pixel[2])*inverse + sb*pixelCoverage) / 0xffff >> 8)
				pixel[3] = uint8((uint32(pixel[3])*inverse + sa*pixelCoverage) / 0xffff >> 8)
				row += 4
			}
		}
		return nil
	})
}

func writeAlphaScalarReference(ctx context.Context, rasterizer *Rasterizer, budget *WorkBudget, dst *image.Alpha) error {
	if err := rasterizer.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dst == nil || rasterizer.width == 0 || rasterizer.height == 0 || len(rasterizer.edges) == 0 {
		return nil
	}
	return rasterizer.rasterize(ctx, budget, func(y, minX, maxX int) error {
		row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
		var winding float32
		for x := minX; x < maxX; {
			end := min(x+contextCheckInterval, maxX)
			if x != minX {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			for ; x < end; x++ {
				winding += rasterizer.difference[x]
				dst.Pix[row] = referenceCoverage(rasterizer.partial[x] + winding)
				row++
			}
		}
		return nil
	})
}

type cancelAtErrContext struct {
	cancelAt int
	checks   int
	canceled bool
}

func (ctx *cancelAtErrContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *cancelAtErrContext) Done() <-chan struct{}       { return nil }
func (ctx *cancelAtErrContext) Value(any) any               { return nil }
func (ctx *cancelAtErrContext) Err() error {
	ctx.checks++
	if ctx.canceled || ctx.cancelAt > 0 && ctx.checks >= ctx.cancelAt {
		ctx.canceled = true
		return context.Canceled
	}
	return nil
}

func TestCoverageMatchesFloat64AbsReference(t *testing.T) {
	t.Parallel()

	check := func(value float32) {
		t.Helper()
		if got, want := coverage(value), referenceCoverage(value); got != want {
			t.Fatalf("coverage(%08x) = %d, want %d", math.Float32bits(value), got, want)
		}
	}
	for _, value := range []float32{
		0, float32(math.Copysign(0, -1)), 1, -1, .5, -.5,
		math.SmallestNonzeroFloat32, -math.SmallestNonzeroFloat32,
		math.MaxFloat32, -math.MaxFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)),
		math.Float32frombits(0x7fc00001), math.Float32frombits(0xffc00001),
	} {
		check(value)
	}
	state := uint32(0x243f6a88)
	for range 1 << 20 {
		state = state*1664525 + 1013904223
		check(math.Float32frombits(state))
	}
}

func TestZeroCoverageValuesMatchesFloatEquality(t *testing.T) {
	t.Parallel()

	positiveZero := float32(0)
	negativeZero := float32(math.Copysign(0, -1))
	values := []float32{
		positiveZero,
		negativeZero,
		math.SmallestNonzeroFloat32,
		-math.SmallestNonzeroFloat32,
		1,
		-1,
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
		math.Float32frombits(0x7fc00001),
		math.Float32frombits(0xffc00001),
	}
	for _, partial := range values {
		for _, difference := range values {
			if got, want := zeroCoverageValues(partial, difference), partial == 0 && difference == 0; got != want {
				t.Fatalf("zeroCoverageValues(%08x, %08x) = %v, want %v", math.Float32bits(partial), math.Float32bits(difference), got, want)
			}
		}
	}
}

func TestZeroCoverageRunEndMatchesScalarReference(t *testing.T) {
	t.Parallel()

	reference := func(partial, difference []float32, start, end int) int {
		for start < end && partial[start] == 0 && difference[start] == 0 {
			start++
		}
		return start
	}
	check := func(partial, difference []float32, start, end int) {
		t.Helper()
		if got, want := zeroCoverageRunEnd(partial, difference, start, end), reference(partial, difference, start, end); got != want {
			t.Fatalf("zeroCoverageRunEnd(start=%d, end=%d) = %d, want %d", start, end, got, want)
		}
	}

	positiveZero := float32(0)
	negativeZero := float32(math.Copysign(0, -1))
	for length := 0; length <= 64; length++ {
		for start := 0; start <= length; start++ {
			for stop := start; stop <= length; stop++ {
				partial := make([]float32, length)
				difference := make([]float32, length)
				for index := range length {
					partial[index] = []float32{positiveZero, negativeZero}[index&1]
					difference[index] = []float32{negativeZero, positiveZero}[index&1]
				}
				if stop < length {
					if stop&1 == 0 {
						partial[stop] = math.SmallestNonzeroFloat32
					} else {
						difference[stop] = -math.SmallestNonzeroFloat32
					}
				}
				check(partial, difference, start, length)
			}
		}
	}

	state := uint32(0x6a09e667)
	for range 1 << 15 {
		state = state*1664525 + 1013904223
		length := int(state % 129)
		partial := make([]float32, length)
		difference := make([]float32, length)
		for index := range length {
			state = state*1664525 + 1013904223
			if state&3 == 0 {
				partial[index] = math.Float32frombits(state)
			} else if state&4 != 0 {
				partial[index] = negativeZero
			}
			state = state*1664525 + 1013904223
			if state&3 == 0 {
				difference[index] = math.Float32frombits(state)
			} else if state&4 != 0 {
				difference[index] = negativeZero
			}
		}
		state = state*1664525 + 1013904223
		start := 0
		if length != 0 {
			start = int(state % uint32(length+1))
		}
		state = state*1664525 + 1013904223
		end := start
		if start < length {
			end += int(state % uint32(length-start+1))
		}
		check(partial, difference, start, end)
	}
}

func referenceCoverage(area float32) uint8 {
	area = float32(math.Abs(float64(area)))
	if area <= 0 {
		return 0
	}
	if area >= 1 {
		return 255
	}
	return uint8(area*255 + 0.5)
}

func addFastPathTestGeometry(rasterizer *Rasterizer, width, height int) {
	// Mix fractional, overlapping, reversed, and self-intersecting paths so the
	// output contains empty, partial, singly wound, and multiply wound spans.
	for index := range 19 {
		x0 := float32((index*37)%(width-18)) + .125
		y0 := float32((index*23)%(height-12)) + .25
		x1 := min(x0+float32(7+(index*29)%83), float32(width)-.125)
		y1 := min(y0+float32(5+(index*17)%41), float32(height)-.25)
		if index&1 == 0 {
			addTestRectangle(rasterizer, x0, y0, x1, y1)
		} else {
			addTestRectangle(rasterizer, x1, y0, x0, y1)
		}
	}
	rasterizer.MoveTo(.375, 7.125)
	rasterizer.LineTo(float32(width)-.625, float32(height)-8.375)
	rasterizer.LineTo(13.25, float32(height)-4.625)
	rasterizer.LineTo(float32(width)-17.75, 3.875)
	rasterizer.ClosePath()
}

func addTestRectangle(rasterizer *Rasterizer, x0, y0, x1, y1 float32) {
	rasterizer.MoveTo(x0, y0)
	rasterizer.LineTo(x1, y0)
	rasterizer.LineTo(x1, y1)
	rasterizer.LineTo(x0, y1)
	rasterizer.ClosePath()
}

type cancelAfterErrContext struct {
	remaining int
	checks    int
}

func (ctx *cancelAfterErrContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *cancelAfterErrContext) Done() <-chan struct{}       { return nil }
func (ctx *cancelAfterErrContext) Value(any) any               { return nil }
func (ctx *cancelAfterErrContext) Err() error {
	ctx.checks++
	ctx.remaining--
	if ctx.remaining <= 0 {
		return context.Canceled
	}
	return nil
}
