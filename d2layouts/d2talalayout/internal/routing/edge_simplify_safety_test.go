package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestSimplifyChecksBothReplacementLegs(t *testing.T) {
	for _, transpose := range []bool{false, true} {
		for _, obstruction := range []string{"node", "edge", "target approach", "clear"} {
			t.Run(obstruction+map[bool]string{false: "/horizontal", true: "/vertical"}[transpose], func(t *testing.T) {
				g := layoutgraph.NewGraph()
				from := g.AddNode(layoutgraph.NewNode(1, 40, 40))
				from.TopLeft = geo.NewPoint(0, 0)
				to := g.AddNode(layoutgraph.NewNode(2, 40, 40))
				to.TopLeft = geo.NewPoint(240, 50)
				e := g.Connect(from, to)
				e.Points = []*geo.Point{geo.NewPoint(40, 20), geo.NewPoint(80, 20), geo.NewPoint(80, 120), geo.NewPoint(200, 120), geo.NewPoint(200, 70), geo.NewPoint(240, 70)}
				switch obstruction {
				case "node":
					n := g.AddNode(layoutgraph.NewNode(3, 20, 20))
					n.TopLeft = geo.NewPoint(190, 35)
				case "edge":
					a := g.AddNode(layoutgraph.NewNode(3, 10, 10))
					a.TopLeft = geo.NewPoint(140, 40)
					b := g.AddNode(layoutgraph.NewNode(4, 10, 10))
					b.TopLeft = geo.NewPoint(210, 40)
					other := g.Connect(a, b)
					other.Points = []*geo.Point{geo.NewPoint(150, 45), geo.NewPoint(210, 45)}
				case "target approach":
					to.TopLeft = geo.NewPoint(180, 30)
					e.Points = e.Points[:5]
				}
				if transpose {
					for _, n := range g.Nodes {
						n.TopLeft.X, n.TopLeft.Y = n.TopLeft.Y, n.TopLeft.X
						n.Width, n.Height = n.Height, n.Width
					}
					for _, edge := range g.Edges {
						for _, p := range edge.Points {
							p.X, p.Y = p.Y, p.X
						}
					}
				}
				before := captureExactRouteTest(e)
				if err := SimplifyEdgeRoutes(context.Background(), g); err != nil {
					t.Fatal(err)
				}
				if obstruction == "clear" {
					if len(e.Points) != 4 {
						t.Fatalf("clear route retained unnecessary bends: %v", e.Points)
					}
				} else {
					before.assertRestored(t)
				}
			})
		}
	}
}

func TestSimplifyChecksObstacleContainingNewBend(t *testing.T) {
	g, e := simplificationMutationGraph()
	obstacle := g.AddNode(layoutgraph.NewNode(3, 8, 8))
	obstacle.TopLeft = geo.NewPoint(16, -4)
	before := captureExactRouteTest(e)
	if err := SimplifyEdgeRoutes(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	before.assertRestored(t)
}
