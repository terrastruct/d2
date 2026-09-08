package layoutgraph

import (
	"testing"

	"github.com/d2lang/d2/lib/shape"
)

func TestSequenceAdvance(t *testing.T) {
	for _, test := range []struct {
		name  string
		width float64
		want  float64
	}{
		{name: "negative", width: -1, want: 0},
		{name: "zero", width: 0, want: 0},
		{name: "narrow", width: 10, want: 5},
		{name: "wedge width", width: shape.STEP_WEDGE_WIDTH, want: shape.STEP_WEDGE_WIDTH / 2},
		{name: "wide", width: 100, want: 100 - shape.STEP_WEDGE_WIDTH},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := SequenceAdvance(test.width); got != test.want {
				t.Fatalf("SequenceAdvance(%v) = %v; want %v", test.width, got, test.want)
			}
		})
	}
}
