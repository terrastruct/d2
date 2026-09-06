package d2raster

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func FuzzFlattenArcIsBounded(f *testing.F) {
	f.Add(0.0, 0.0, 10.0, 7.0, 12.0, 4.0, 0.3, false, true)
	f.Add(-3.0, 8.0, 31.0, -19.0, -2.0, -5.0, math.Pi/2, true, false)
	f.Fuzz(func(t *testing.T, sx, sy, ex, ey, rx, ry, rotation float64, largeArc, sweep bool) {
		values := []float64{sx, sy, ex, ey, rx, ry, rotation}
		for _, value := range values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return
			}
		}
		limit := errors.New("fuzz point limit")
		count := 0
		err := flattenArc(context.Background(), d2scene.Point{X: sx, Y: sy}, d2scene.ArcTo(rx, ry, rotation, largeArc, sweep, ex, ey), d2scene.Identity(), 0.25, func(point d2scene.Point) error {
			if !finitePoint(point) {
				t.Fatalf("emitted non-finite point %#v", point)
			}
			count++
			if count > 10_000 {
				return limit
			}
			return nil
		})
		if count > 10_001 {
			t.Fatalf("emitted %d points beyond bound", count)
		}
		if errors.Is(err, limit) && count != 10_001 {
			t.Fatalf("limit error after %d points", count)
		}
	})
}
