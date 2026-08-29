package scanline

import (
	"context"
	"errors"
	"image"
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
