package routing

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/quality"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func TestBalanceFixedPortsMovesOnlyInteriorCorridor(t *testing.T) {
	for _, kind := range []string{shape.TABLE_TYPE, shape.DIAMOND_TYPE} {
		t.Run(kind, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			from := g.AddNode(layoutgraph.NewNode(1, 100, 100))
			from.TopLeft = geo.NewPoint(0, 0)
			from.SetShape(kind)
			to := g.AddNode(layoutgraph.NewNode(2, 100, 100))
			to.TopLeft = geo.NewPoint(300, 0)
			to.SetShape(kind)
			e := g.Connect(from, to)
			if kind == shape.TABLE_TYPE {
				from.SetNumColumns(1)
				to.SetNumColumns(1)
				column := 0
				e.FromTableColumnIndex = &column
				e.ToTableColumnIndex = &column
			}
			e.Points = []*geo.Point{geo.NewPoint(100, 50), geo.NewPoint(130, 50), geo.NewPoint(130, 200), geo.NewPoint(220, 200), geo.NewPoint(220, 300), geo.NewPoint(270, 300), geo.NewPoint(270, 50), geo.NewPoint(300, 50)}
			original := make([]geo.Point, len(e.Points))
			for i, p := range e.Points {
				original[i] = *p
			}
			before, err := quality.Inspect(context.Background(), g)
			if err != nil {
				t.Fatal(err)
			}
			if err := BalanceEdgeSegments(context.Background(), g); err != nil {
				t.Fatal(err)
			}
			if *e.Points[0] != original[0] || *e.Points[1] != original[1] || *e.Points[len(e.Points)-2] != original[len(original)-2] || *e.Points[len(e.Points)-1] != original[len(original)-1] {
				t.Fatal("fixed approach moved")
			}
			moved := len(e.Points) != len(original)
			for i, p := range e.Points {
				if i >= len(original) || *p != original[i] {
					moved = true
				}
			}
			if !moved {
				t.Fatal("interior corridor remained frozen")
			}
			after, err := quality.Inspect(context.Background(), g)
			if err != nil {
				t.Fatal(err)
			}
			if after.RouteObstructions > before.RouteObstructions || after.Crossings > before.Crossings {
				t.Fatalf("balancing introduced conflict: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestBalanceFixedPortsPreservesCurves(t *testing.T) {
	for _, kind := range []string{shape.TABLE_TYPE, shape.DIAMOND_TYPE} {
		t.Run(kind, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			from := g.AddNode(layoutgraph.NewNode(1, 100, 100))
			from.TopLeft = geo.NewPoint(0, 0)
			from.SetShape(kind)
			to := g.AddNode(layoutgraph.NewNode(2, 100, 100))
			to.TopLeft = geo.NewPoint(300, 0)
			to.SetShape(kind)
			e := g.Connect(from, to)
			if kind == shape.TABLE_TYPE {
				from.SetNumColumns(1)
				to.SetNumColumns(1)
				column := 0
				e.FromTableColumnIndex = &column
				e.ToTableColumnIndex = &column
			}
			e.IsCurve = true
			// Two cubic segments can have an orthogonal control polygon. Moving
			// its interior bends still changes the rendered curves, which the
			// polyline obstruction and crossing checks cannot validate.
			e.Points = []*geo.Point{
				geo.NewPoint(100, 50), geo.NewPoint(130, 50), geo.NewPoint(130, 200),
				geo.NewPoint(220, 200), geo.NewPoint(220, 300), geo.NewPoint(300, 300),
				geo.NewPoint(300, 50),
			}
			original := make([]geo.Point, len(e.Points))
			for i, p := range e.Points {
				original[i] = *p
			}
			if err := BalanceEdgeSegments(context.Background(), g); err != nil {
				t.Fatal(err)
			}
			if !e.IsCurve || len(e.Points) != len(original) {
				t.Fatal("curve representation changed")
			}
			for i, p := range e.Points {
				if *p != original[i] {
					t.Fatalf("curve point %d moved: got %v, want %v", i, *p, original[i])
				}
			}
		})
	}
}
