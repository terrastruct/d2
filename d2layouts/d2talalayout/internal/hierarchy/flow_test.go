package hierarchy

import (
	"math"
	"slices"
	"testing"
)

func TestFourPassMedianIsCenteredAndReflectionInvariant(t *testing.T) {
	for _, tc := range []struct {
		values []float64
		want   float64
	}{
		{nil, 0}, {[]float64{7}, 7}, {[]float64{4, -2}, 1}, {[]float64{5, 1, 3}, 3},
		{[]float64{200, -100, 100, 0}, 50}, {[]float64{math.MaxFloat64, math.MaxFloat64}, math.MaxFloat64},
	} {
		before := slices.Clone(tc.values)
		if got := median(tc.values); got != tc.want {
			t.Fatalf("median(%v)=%g, want %g", tc.values, got, tc.want)
		}
		reflected := make([]float64, len(tc.values))
		for i, x := range tc.values {
			reflected[i] = -x
		}
		if got := median(reflected); got != -tc.want {
			t.Fatalf("reflected median=%g, want %g", got, -tc.want)
		}
		if !slices.Equal(tc.values, before) {
			t.Fatal("median mutated input")
		}
	}
}
