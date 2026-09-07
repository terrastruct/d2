package layoutgraph

import (
	"math"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/lib/geo"
)

// Keep the pre-optimization clipping implementation as a test oracle. In
// particular, corner tangencies and non-finite input follow its comparisons.
func legacySegmentIntersectsBox(p1, p2 *geo.Point, box *geo.Box) bool {
	if p1 == nil || p2 == nil || box == nil || box.TopLeft == nil {
		return false
	}

	left := math.Min(box.TopLeft.X, box.TopLeft.X+box.Width)
	right := math.Max(box.TopLeft.X, box.TopLeft.X+box.Width)
	top := math.Min(box.TopLeft.Y, box.TopLeft.Y+box.Height)
	bottom := math.Max(box.TopLeft.Y, box.TopLeft.Y+box.Height)

	// Quickly rule out segments whose axis-aligned bounds do not overlap the box.
	if math.Max(p1.X, p2.X) < left || right < math.Min(p1.X, p2.X) ||
		math.Max(p1.Y, p2.Y) < top || bottom < math.Min(p1.Y, p2.Y) {
		return false
	}

	contains := func(p *geo.Point) bool {
		return left <= p.X && p.X <= right && top <= p.Y && p.Y <= bottom
	}
	if contains(p1) || contains(p2) {
		return true
	}

	// Clip the segment against the closed box. Both endpoints are outside here,
	// so a zero-length clipped interval is a single-point tangency rather than a
	// crossover. We allow that case, while a positive-length interval means the
	// segment crosses the box or overlaps one of its boundaries.
	tEnter, tExit := 0.0, 1.0
	clipAxis := func(start, delta, minCoord, maxCoord float64) bool {
		if delta == 0 {
			return minCoord <= start && start <= maxCoord
		}
		t1 := (minCoord - start) / delta
		t2 := (maxCoord - start) / delta
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		tEnter = math.Max(tEnter, t1)
		tExit = math.Min(tExit, t2)
		return tEnter <= tExit
	}

	if !clipAxis(p1.X, p2.X-p1.X, left, right) ||
		!clipAxis(p1.Y, p2.Y-p1.Y, top, bottom) {
		return false
	}
	return tEnter < tExit
}

func TestSegmentIntersectsBoxReferenceParity(t *testing.T) {
	random := rand.New(rand.NewSource(718))
	special := []float64{math.NaN(), math.Inf(-1), math.Inf(1), math.Copysign(0, -1), 0,
		math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64, math.MaxFloat64, -math.MaxFloat64,
		-10, -1, 1, 10, math.Nextafter(10, 0), math.Nextafter(10, math.Inf(1))}
	check := func(p1, p2 *geo.Point, box *geo.Box) {
		t.Helper()
		got, want := segmentIntersectsBox(p1, p2, box), legacySegmentIntersectsBox(p1, p2, box)
		if got != want {
			t.Fatalf("segment (%+v, %+v), box %+v: got %v, want %v", p1, p2, box, got, want)
		}
	}
	for range 100_000 {
		value := func() float64 {
			if random.Intn(2) == 0 {
				return special[random.Intn(len(special))]
			}
			return (random.Float64() - 0.5) * 10_000
		}
		p1, p2 := &geo.Point{X: value(), Y: value()}, &geo.Point{X: value(), Y: value()}
		box := &geo.Box{TopLeft: &geo.Point{X: value(), Y: value()}, Width: value(), Height: value()}
		check(p1, p2, box)
		check(p2, p1, box)
	}
	// Exercise every endpoint combination around a closed box, including zero
	// size and negative dimensions, with both segment directions.
	for _, width := range special {
		for _, height := range special {
			box := &geo.Box{TopLeft: &geo.Point{}, Width: width, Height: height}
			for _, x1 := range special {
				for _, x2 := range special {
					p1, p2 := &geo.Point{X: x1, Y: -10}, &geo.Point{X: x2, Y: 10}
					check(p1, p2, box)
					check(p2, p1, box)
					p1.X, p1.Y, p2.X, p2.Y = p1.Y, p1.X, p2.Y, p2.X
					check(p1, p2, box)
				}
			}
		}
	}
	check(nil, &geo.Point{}, &geo.Box{TopLeft: &geo.Point{}})
	check(&geo.Point{}, nil, &geo.Box{TopLeft: &geo.Point{}})
	check(&geo.Point{}, &geo.Point{}, nil)
	check(&geo.Point{}, &geo.Point{}, &geo.Box{})
}

var segmentBoxBenchmarkResult bool

func BenchmarkSegmentIntersectsBox(b *testing.B) {
	const count = 256
	random := rand.New(rand.NewSource(718))
	boxes := make([]geo.Box, count)
	points := make([]geo.Point, count)
	for i := range boxes {
		points[i] = geo.Point{X: random.Float64() * 10_000, Y: random.Float64() * 10_000}
		boxes[i] = geo.Box{TopLeft: &points[i], Width: 100, Height: 100}
	}
	p1, p2 := &geo.Point{X: 100, Y: 500}, &geo.Point{X: 5000, Y: 500}
	for _, implementation := range []struct {
		name string
		run  func(*geo.Point, *geo.Point, *geo.Box) bool
	}{
		{"legacy", legacySegmentIntersectsBox}, {"optimized", segmentIntersectsBox},
	} {
		b.Run(implementation.name, func(b *testing.B) {
			b.ReportAllocs()
			result := false
			for b.Loop() {
				for i := range boxes {
					result = implementation.run(p1, p2, &boxes[i])
				}
			}
			segmentBoxBenchmarkResult = result
		})
	}
}
