package routing

import (
	"slices"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestIdealTurnAxesOwnsRouteTurnPolicy(t *testing.T) {
	source := layoutgraph.NewNode(1, 20, 20)
	source.TopLeft = geo.NewPoint(100, 100)
	target := layoutgraph.NewNode(2, 20, 20)
	target.TopLeft = geo.NewPoint(0, 0)

	want := []idealTurnAxis{{isX: true, val: 60}, {isX: false, val: 60}}
	if got := idealTurnAxes(source, target); !slices.Equal(got, want) {
		t.Fatalf("ideal axes = %#v, want %#v", got, want)
	}

	target.TopLeft = geo.NewPoint(105, 105)
	if got := idealTurnAxes(source, target); len(got) != 0 {
		t.Fatalf("overlapping-node ideal axes = %#v, want none", got)
	}
}
