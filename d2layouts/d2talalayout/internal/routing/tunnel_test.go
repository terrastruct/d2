package routing

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestTunnelSimple(t *testing.T) {
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 100, 100)
	b := layoutgraph.NewNode(1, 100, 100)

	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(300, 0)

	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	g.Connect(a, b)

	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 1, len(tunnels))
		for _, entry := range []*TunnelEntry{tunnels[0].EntryA, tunnels[0].EntryB} {
			metadata, ok := entry.OVGNode.portMetadataFor(entry.Node)
			assert.True(t, ok)
			assert.Zero(t, metadata.directions)
			assert.False(t, metadata.isCenterPort)
		}
	}

	b.TopLeft.Y = 300

	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 0, len(tunnels))
	}
}

func TestTunnelObscured(t *testing.T) {
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 100, 100)
	b := layoutgraph.NewNode(1, 100, 100)

	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(500, 0)

	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)
	g.Connect(a, b)

	// Fully obscured
	c := layoutgraph.NewNode(2, 100, 100)
	c.TopLeft = geo.NewPoint(300, 0)
	g.AddNodeUnchecked(c)

	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 0, len(tunnels))
	}

	// Partially obscured at end, and not enough room for a tunnel
	// ┌─────┐                ┌──────┐
	// │     │        ┌─────┐ │      │
	// │     │        │     │ │      │
	// │ a   │        │     │ │  b   │
	// │     │        │ c   │ │      │
	// │     │        │     │ │      │
	// └─────┘        └─────┘ └──────┘
	c.TopLeft = geo.NewPoint(300, 1)
	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 0, len(tunnels))
	}

	// Partially obscured at end, and enough room for a tunnel
	// ┌─────┐                ┌──────┐
	// │     │--------------> │      │
	// │     │                │      │
	// │ a   │        ┌─────┐ │  b   │
	// │     │        │ c   │ │      │
	// │     │        │     │ │      │
	// └─────┘        └─────┘ └──────┘
	c.TopLeft = geo.NewPoint(300, segmentSpacingBuffer+1)
	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 1, len(tunnels))
	}

	// Partially obscured at start, and enough room for a tunnel
	//              ┌─────┐
	// ┌─────┐      │ c   │   ┌──────┐
	// │     │      │     │   │      │
	// │     │      └─────┘   │      │
	// │ a   │                │  b   │
	// │     │--------------> │      │
	// │     │                │      │
	// └─────┘                └──────┘
	c.TopLeft = geo.NewPoint(300, -90)
	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 1, len(tunnels))
	}
}

func TestTunnelMultiple(t *testing.T) {
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	b := layoutgraph.NewNode(1, 1000, 1000)

	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(5000, 0)

	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	g.Connect(a, b)
	g.Connect(a, b)
	g.Connect(a, b)

	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 3, len(tunnels))
	}

	// Moved down to only have room for 2 tunnels
	b.TopLeft.Y = b.Height - 2*segmentSpacingBuffer

	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 2, len(tunnels))
	}

	b.TopLeft.Y = 0

	// Obscures middle just enough to allow 2 tunnels
	// ┌─────┐                ┌──────┐
	// │     ├───────────────►│      │
	// │     │                │      │
	// │     │    ┌─────┐     │      │
	// │ a   │    │ c   │     │  b   │
	// │     │    └─────┘     │      │
	// │     │                │      │
	// │     ├───────────────►│      │
	// └─────┘                └──────┘
	c := layoutgraph.NewNode(2, 1000, b.Height-segmentSpacingBuffer*2)
	c.TopLeft = geo.NewPoint(3000, segmentSpacingBuffer)
	g.AddNodeUnchecked(c)
	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 2, len(tunnels))
	}

	// Add another obstacle to obscure the top tunnel
	//            ┌───┐
	// ┌─────┐    │d  │       ┌──────┐
	// │     │    └───┘       │      │
	// │     │                │      │
	// │     │    ┌─────┐     │      │
	// │ a   │    │ c   │     │  b   │
	// │     │    └─────┘     │      │
	// │     │                │      │
	// │     ├───────────────►│      │
	// └─────┘                └──────┘
	d := layoutgraph.NewNode(3, 1000, segmentSpacingBuffer)
	d.TopLeft = geo.NewPoint(3000, 0)
	g.AddNodeUnchecked(d)
	{
		tunnels := buildTunnelsForTest(t, g)
		assert.Equal(t, 1, len(tunnels))
	}
}
