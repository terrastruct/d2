package placementcost

import (
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestFlowContinuityNodeEdgeLengthIntegration(t *testing.T) {
	for _, stage := range []struct {
		name         string
		includeSizes bool
	}{
		{name: "sized", includeSizes: true},
		{name: "sizeless", includeSizes: false},
	} {
		t.Run(stage.name, func(t *testing.T) {
			points := []geo.Point{{X: 0, Y: -160}, {X: 0, Y: 160}}
			directed, directedScratch := flowFixture(points, []bool{true, false})
			control, controlScratch := flowFixture(points, []bool{true, false})
			for _, edge := range control.Edges {
				edge.TargetArrowhead = layoutgraph.NoArrowhead
			}
			// With direction penalties disabled and no labels or abductions,
			// arrowheads change only continuity. Subtracting the undirected
			// control isolates the hook in the complete edge-length scorer.
			options := EdgeLengthOptions{IncludeNodeSizes: stage.includeSizes}
			directedScorer := NewNodeEdgeLengthScorer(directed, options)
			defer directedScorer.Close()
			controlScorer := NewNodeEdgeLengthScorer(control, options)
			defer controlScorer.Close()

			// Placement fixes this scale across a candidate sweep. Initialize
			// both caches before moving geometry to preserve the same scale.
			turnCost := directed.Graph.TurnCost()
			if turnCost <= 0 || control.Graph.TurnCost() != turnCost {
				t.Fatal("fixture requires equal, positive turn costs")
			}
			for _, candidate := range []struct {
				name     string
				outgoing geo.Point
				turns    float64
			}{
				{name: "straight", outgoing: geo.Point{X: 0, Y: 160}, turns: 0},
				{name: "elbow", outgoing: geo.Point{X: 160, Y: 0}, turns: 1},
				{name: "hairpin", outgoing: geo.Point{X: 0, Y: -320}, turns: 2},
				{name: "straight_again", outgoing: geo.Point{X: 0, Y: 160}, turns: 0},
			} {
				t.Run(candidate.name, func(t *testing.T) {
					for _, scratch := range []*edgeScratch{directedScratch, controlScratch} {
						outgoing := scratch.aRepl[1]
						outgoing.TopLeft.X = candidate.outgoing.X - outgoing.Width/2
						outgoing.TopLeft.Y = candidate.outgoing.Y - outgoing.Height/2
					}
					score := func(node *layoutgraph.Node, scorer *NodeEdgeLengthScorer) float64 {
						t.Helper()
						direct, err := NodeEdgeLength(t.Context(), node, options)
						if err != nil {
							t.Fatal(err)
						}
						prepared, err := scorer.Score(t.Context())
						if err != nil {
							t.Fatal(err)
						}
						if math.Float64bits(prepared) != math.Float64bits(direct) {
							t.Fatalf("prepared=%v direct=%v", prepared, direct)
						}
						return direct
					}
					got := score(directed, &directedScorer) - score(control, &controlScorer)
					want := 0.0
					if stage.includeSizes {
						want = candidate.turns * turnCost
					}
					if math.Abs(got-want) > 1e-9 {
						t.Fatalf("continuity contribution=%v want=%v", got, want)
					}
				})
			}
		})
	}
}
