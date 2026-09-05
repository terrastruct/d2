package svg

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

func TestFormatFloat(t *testing.T) {
	for _, tc := range []struct {
		value float64
		want  string
	}{
		{0, "0"}, {math.Copysign(0, -1), "-0"}, {42, "42"},
		{3.14, "3.14"}, {-0.0000001, "-0"}, {0.0000001, "0"},
		{0.000001, "0.000001"}, {1.2345678, "1.234568"},
		{999.9999999, "1000"}, {math.Inf(1), "+Inf"}, {math.NaN(), "NaN"},
	} {
		if got := FormatFloat(tc.value); got != tc.want {
			t.Errorf("FormatFloat(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}

// Use the old fmt implementation as an independent oracle, including extremes
// and values very near its six-place rounding boundary. Compare IEEE bits so
// signed zero cannot silently change either.
func TestFormatFloatPreservesRoundedValue(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 100000; i++ {
		value := math.Float64frombits(rng.Uint64())
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		before := fmt.Sprintf("%f", value)
		after := FormatFloat(value)
		oldValue, oldErr := strconv.ParseFloat(before, 64)
		newValue, newErr := strconv.ParseFloat(after, 64)
		if (oldErr == nil) != (newErr == nil) || math.Float64bits(oldValue) != math.Float64bits(newValue) {
			t.Fatalf("%v: %q and %q parse differently", value, before, after)
		}
		if len(after) > len(before) || strings.Contains(after, ".") && strings.HasSuffix(after, "0") {
			t.Fatalf("noncompact output %q from %q", after, before)
		}
	}
}
