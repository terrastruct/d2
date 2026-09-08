package engine

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/hierarchy"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func compoundAdmissionBlocks() (*layoutgraph.Graph, []*layoutgraph.Node) {
	g := layoutgraph.NewGraph()
	g.Directions[nil] = geo.Bottom
	var roots []*layoutgraph.Node
	for i, p := range [][2]float64{{0, 0}, {50, 160}, {180, 320}} {
		n := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 100, 80)
		n.TopLeft = geo.NewPoint(p[0], p[1])
		g.AddNewNodeToContainer(nil, n)
		roots = append(roots, n)
	}
	return g, roots
}

func TestCompoundAdmissionUsesWholeBlocksAndIgnoresInternalEdges(t *testing.T) {
	g, roots := compoundAdmissionBlocks()
	nested := layoutgraph.NewNode(8, 30, 30)
	nested.TopLeft = geo.NewPoint(0, 0)
	g.AddNewNodeToContainer(roots[0], nested)
	from := layoutgraph.NewNode(4, 10, 10)
	from.TopLeft = geo.NewPoint(5, 10)
	g.AddNewNodeToContainer(nested, from)
	to := layoutgraph.NewNode(5, 10, 10)
	to.TopLeft = geo.NewPoint(125, 180)
	g.AddNewNodeToContainer(roots[1], to)
	// The descendant endpoints have disjoint spans, but their outer blocks
	// overlap across the flow. This is the distinction from an endpoint floor.
	g.Connect(from, to)
	if got := compoundCrossAxisDetours(g); got != 0 {
		t.Fatalf("overlapping outer blocks: got %d, want 0", got)
	}
	inner := layoutgraph.NewNode(6, 10, 10)
	inner.TopLeft = geo.NewPoint(80, 55)
	g.AddNewNodeToContainer(roots[0], inner)
	g.Connect(from, inner)
	g.Connect(inner, inner)
	if got := compoundCrossAxisDetours(g); got != 0 {
		t.Fatalf("internal and loop routes changed outer admission: got %d", got)
	}
	g.Connect(to, roots[2])
	g.Connect(to, roots[2])
	if got := compoundCrossAxisDetours(g); got != 2 {
		t.Fatalf("parallel external connections: got %d, want 2", got)
	}
}

func TestCompoundAdmissionDirectionReversalAndRigidTranslation(t *testing.T) {
	for _, direction := range []geo.Orientation{geo.NONE, geo.Bottom, geo.Top, geo.Left, geo.Right} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("direction-%v/reversed-%t", direction, reverse), func(t *testing.T) {
				g, roots := compoundAdmissionBlocks()
				g.Directions[nil] = direction
				if direction.IsHorizontal() {
					for _, n := range g.Nodes {
						n.TopLeft.X, n.TopLeft.Y = n.TopLeft.Y, n.TopLeft.X
						n.Width, n.Height = n.Height, n.Width
					}
				}
				for _, to := range roots[1:] {
					if reverse {
						g.Connect(to, roots[0])
					} else {
						g.Connect(roots[0], to)
					}
				}
				before, err := layoutgraph.Clone(t.Context(), g)
				if err != nil {
					t.Fatal(err)
				}
				if got := compoundCrossAxisDetours(g); got != 1 {
					t.Fatalf("got %d, want 1", got)
				}
				requireGraphsSerializeEqual(t.Context(), t, before, g)
				for _, n := range g.Nodes {
					n.Translate(4096.25, -8192.5)
				}
				if got := compoundCrossAxisDetours(g); got != 1 {
					t.Fatalf("rigid translation changed admission: got %d, want 1", got)
				}
			})
		}
	}
}

func TestCompoundAdmissionTouchingAndDegenerateSpans(t *testing.T) {
	for _, test := range []struct {
		name                string
		firstWidth, secondX float64
		secondWidth         float64
		want                int
	}{
		{"touching", 100, 100, 20, 0},
		{"separated", 100, 101, 20, 1},
		{"contained point", 100, 50, 0, 0},
		{"coincident points", 0, 0, 0, 0},
		{"separate points", 0, 1, 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			g, roots := compoundAdmissionBlocks()
			roots[0].Width = test.firstWidth
			roots[1].TopLeft.X, roots[1].Width = test.secondX, test.secondWidth
			g.Connect(roots[0], roots[1])
			if got := compoundCrossAxisDetours(g); got != test.want {
				t.Fatalf("got %d, want %d", got, test.want)
			}
		})
	}
}

func TestCompoundAdmissionRejectsWiderLanesWithoutMutatingIncumbent(t *testing.T) {
	g := layoutgraph.NewGraph()
	g.Directions[nil] = geo.Bottom
	var roots []*layoutgraph.Node
	for i := 0; i < 5; i++ {
		n := layoutgraph.NewNode(layoutgraph.EntityID(i+1), 120, 150)
		n.TopLeft = geo.NewPoint(0, float64(i)*300)
		g.AddNewNodeToContainer(nil, n)
		roots = append(roots, n)
	}
	first, second := layoutgraph.NewNode(6, 40, 30), layoutgraph.NewNode(7, 40, 30)
	first.TopLeft, second.TopLeft = geo.NewPoint(40, 20), geo.NewPoint(40, 80)
	g.AddNewNodeToContainer(roots[0], first)
	g.AddNewNodeToContainer(roots[0], second)
	g.Connect(first, second)
	for _, branch := range roots[1:4] {
		g.Connect(second, branch)
		g.Connect(branch, roots[4])
	}
	for _, e := range g.Edges {
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
		e.Points = []*geo.Point{e.From.Center(), e.To.Center()}
	}
	before, err := layoutgraph.Clone(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := layoutgraph.Clone(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := hierarchy.PlaceCompound(t.Context(), proposal, rand.New(rand.NewSource(0)))
	if err != nil || !changed {
		t.Fatalf("fixture did not produce an applicable placement: changed=%v err=%v", changed, err)
	}
	if compoundCrossAxisDetours(proposal) <= compoundCrossAxisDetours(g) {
		t.Fatal("fixture did not increase the number of disjoint outer lanes")
	}
	selected, err := CompoundCandidate(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	if selected != g {
		t.Fatal("a rejected admission must retain the exact incumbent graph")
	}
	requireGraphsSerializeEqual(t.Context(), t, before, g)
}
