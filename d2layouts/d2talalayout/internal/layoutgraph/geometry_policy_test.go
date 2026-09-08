package layoutgraph

import (
	"math"
	"testing"
)

func TestCostWeightBits(t *testing.T) {
	// These values participate in layout scoring, so changing their declaration
	// must not change their float64 representation.
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{name: "turn", got: turnPenaltyMultiplier, want: math.Pow(50.0/100.0, 3)},
		{name: "center port", got: centerPortMultiplier, want: math.Pow(35.0/100.0, 3)},
		{name: "crossing", got: CrossingCostWeight, want: math.Pow(48.0/100.0, 3)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if math.Float64bits(test.got) != math.Float64bits(test.want) {
				t.Fatalf("weight bits = %016x, want %016x", math.Float64bits(test.got), math.Float64bits(test.want))
			}
		})
	}
}
