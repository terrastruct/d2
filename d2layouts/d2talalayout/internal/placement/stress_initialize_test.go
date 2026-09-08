package placement

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func stressTestGraph(n int, connected bool) *layoutgraph.Graph {
	g := layoutgraph.NewGraph()
	for i := 0; i < n; i++ {
		g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i), 100, 50))
	}
	if connected {
		for i := 1; i < n; i++ {
			g.Connect(g.Nodes[i-1], g.Nodes[i])
		}
	}
	return g
}

func TestGraphDistanceInitialization(t *testing.T) {
	g := stressTestGraph(12, true)
	applied, err := initializeByGraphDistance(context.Background(), g)
	if err != nil || !applied {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	seen := map[geo.Point]bool{}
	prior := make([]geo.Point, len(g.Nodes))
	for i, n := range g.Nodes {
		p := *n.TopLeft
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.Mod(p.X, 1) != 0 || math.Mod(p.Y, 1) != 0 || seen[p] {
			t.Fatalf("invalid/occupied grid position %v", p)
		}
		seen[p] = true
		prior[i] = p
		if n.Width != 100 || n.Height != 50 {
			t.Fatal("initialization resized a node")
		}
	}
	// A long path must not collapse its distant endpoints onto neighboring cells.
	distance := func(a, b *layoutgraph.Node) float64 {
		return math.Hypot(a.TopLeft.X-b.TopLeft.X, a.TopLeft.Y-b.TopLeft.Y)
	}
	if distance(g.Nodes[0], g.Nodes[11]) < 2*distance(g.Nodes[0], g.Nodes[1]) {
		t.Fatal("global path distance lost")
	}
	applied, err = initializeByGraphDistance(context.Background(), g)
	if err != nil || !applied {
		t.Fatal(err)
	}
	for i, n := range g.Nodes {
		if *n.TopLeft != prior[i] {
			t.Fatal("non-deterministic initialization")
		}
	}
}

func TestGraphDistanceFallbackAndCancellation(t *testing.T) {
	for _, kind := range []string{"fixed", "disconnected", "large", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			n := 8
			if kind == "large" {
				n = 65
			}
			g := stressTestGraph(n, kind != "disconnected")
			for i, node := range g.Nodes {
				node.TopLeft = geo.NewPoint(float64(i), 17)
			}
			if kind == "fixed" {
				g.Nodes[2].FixedTopLeft = geo.NewPoint(20, 30)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "cancelled" {
				cancel()
			}
			points := make([]*geo.Point, n)
			for i, node := range g.Nodes {
				points[i] = node.TopLeft
			}
			applied, err := initializeByGraphDistance(ctx, g)
			if applied {
				t.Fatal("expected fallback")
			}
			if (err != nil) != (kind == "cancelled") {
				t.Fatalf("unexpected error %v", err)
			}
			for i, node := range g.Nodes {
				if node.TopLeft != points[i] || *node.TopLeft != (geo.Point{X: float64(i), Y: 17}) {
					t.Fatal("fallback changed geometry")
				}
			}
		})
	}
}
