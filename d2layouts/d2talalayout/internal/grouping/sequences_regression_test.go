package grouping

import (
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"

	"github.com/stretchr/testify/assert"
)

func TestBuildSequence(t *testing.T) {
	g := layoutgraph.NewGraph()

	n1 := layoutgraph.NewNode(1, 30, 30)
	n2 := layoutgraph.NewNode(2, 30, 30)
	n3 := layoutgraph.NewNode(3, 30, 30)
	g.AddNode(n1)
	g.AddNode(n2)
	g.AddNode(n3)
	g.Connect(n1, n2)
	g.Connect(n2, n3)
	seq := buildSequence([]*layoutgraph.Node{n1, n2, n3}, g, nil, 123)

	// vessel not placed if nodes were not placed
	assert.Nil(t, seq.Vessel.TopLeft)
}

func TestBuildSequenceUsesPositionedSteps(t *testing.T) {
	g := layoutgraph.NewGraph()

	n1 := layoutgraph.NewNode(1, 30, 30)
	n2 := layoutgraph.NewNode(2, 30, 30)
	n3 := layoutgraph.NewNode(3, 30, 30)
	g.AddNode(n1)
	g.AddNode(n2)
	g.AddNode(n3)
	g.Connect(n1, n2)
	g.Connect(n2, n3)
	n1.TopLeft = geo.NewPoint(10, 10)
	n2.TopLeft = geo.NewPoint(50, 5)
	n3.TopLeft = geo.NewPoint(30, 35)

	seq := buildSequence([]*layoutgraph.Node{n1, n2, n3}, g, nil, 123)

	// vessel placed if nodes were placed
	assert.Equal(t, 10.0, seq.Vessel.TopLeft.X)
	assert.Equal(t, 5.0, seq.Vessel.TopLeft.Y)
}

func TestSyncSequencesInGraph(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 100, 100)
	b := layoutgraph.NewNode(2, 100, 100)
	c := layoutgraph.NewNode(3, 100, 100)
	d := layoutgraph.NewNode(4, 100, 100)

	abVessel := layoutgraph.NewNode(5, 200-shape.STEP_WEDGE_WIDTH, 100)
	cdVessel := layoutgraph.NewNode(6, 200-shape.STEP_WEDGE_WIDTH, 100)

	a.TopLeft = geo.NewPoint(1000, 1000)
	b.TopLeft = geo.NewPoint(1100-shape.STEP_WEDGE_WIDTH, 1000)

	c.TopLeft = geo.NewPoint(1500, 1000)
	d.TopLeft = geo.NewPoint(1600-shape.STEP_WEDGE_WIDTH, 1000)

	for _, n := range []*layoutgraph.Node{a, b, c, d, abVessel, cdVessel} {
		n.Graph = graph
	}
	graph.Sequences = map[*layoutgraph.Node]*layoutgraph.Sequence{
		abVessel: {Vessel: abVessel, Nodes: []*layoutgraph.Node{a, b}},
		cdVessel: {Vessel: cdVessel, Nodes: []*layoutgraph.Node{c, d}},
	}
	graph.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		nil: {abVessel, c, d},
	}
	for _, n := range graph.Containers[nil] {
		graph.AddNodeUnchecked(n)
	}

	abVessel.TopLeft = geo.NewPoint(2000, 1000)
	cdVessel.TopLeft = geo.NewPoint(2000, 1000)

	graph.SyncSequences()

	// a and b should move to vessel
	assert.Equal(t, a.TopLeft, geo.NewPoint(2000, 1000))
	assert.Equal(t, b.TopLeft, geo.NewPoint(2100-shape.STEP_WEDGE_WIDTH, 1000))

	// c and d should not move to vessel
	assert.Equal(t, c.TopLeft, geo.NewPoint(1500, 1000))
	assert.Equal(t, d.TopLeft, geo.NewPoint(1600-shape.STEP_WEDGE_WIDTH, 1000))
}
