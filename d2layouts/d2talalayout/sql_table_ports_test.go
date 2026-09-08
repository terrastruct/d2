package d2talalayout

import (
	"context"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
)

func TestSQLTableRowPortsWithPlainEndpoint(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, *d2graph.Graph, *d2graph.Edge) error
	}{
		{
			name: "Layout",
			run: func(ctx context.Context, graph *d2graph.Graph, _ *d2graph.Edge) error {
				return Layout(ctx, graph, &Options{Seeds: []int64{1}, MaxConcurrency: 1})
			},
		},
		{
			name: "RouteEdges",
			run: func(ctx context.Context, graph *d2graph.Graph, edge *d2graph.Edge) error {
				return RouteEdges(ctx, graph, []*d2graph.Edge{edge})
			},
		},
	}
	connections := []struct {
		name          string
		tableIsSource bool
		reverseArrow  bool
	}{
		{name: "table.row -> plain", tableIsSource: true},
		{name: "plain <- table.row", reverseArrow: true},
		{name: "plain -> table.row"},
		{name: "table.row <- plain", tableIsSource: true, reverseArrow: true},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			for _, connection := range connections {
				t.Run(connection.name, func(t *testing.T) {
					graph, edge := newD2TransactionGraph(false)
					table := graph.Objects[0]
					table.Shape.Value = d2target.ShapeSQLTable
					table.SQLTable = &d2target.SQLTable{Columns: make([]d2target.SQLColumn, 3)}
					table.Height = 160
					for _, object := range graph.Objects {
						object.Top = &d2graph.Scalar{Value: strconv.Itoa(int(object.TopLeft.Y))}
						object.Left = &d2graph.Scalar{Value: strconv.Itoa(int(object.TopLeft.X))}
					}
					if connection.tableIsSource {
						edge.SrcTableColumnIndex = new(2)
					} else {
						edge.Src, edge.Dst = edge.Dst, edge.Src
						slices.Reverse(edge.Route)
						edge.DstTableColumnIndex = new(2)
					}
					edge.SrcArrow = connection.reverseArrow
					edge.DstArrow = !connection.reverseArrow

					if err := operation.run(t.Context(), graph, edge); err != nil {
						t.Fatal(err)
					}
					if len(edge.Route) < 2 {
						t.Fatalf("edge has %d route points", len(edge.Route))
					}
					endpoint := edge.Route[0]
					if !connection.tableIsSource {
						endpoint = edge.Route[len(edge.Route)-1]
					}
					// The table has a header and three equally sized rows. The
					// requested third row is centered 3.5 row heights below its top.
					want := geo.Point{
						X: table.TopLeft.X + table.Width,
						Y: table.TopLeft.Y + math.Round(table.Height/4*3.5),
					}
					if *endpoint != want {
						t.Errorf("table endpoint = %v, want third-row port %v", *endpoint, want)
					}
				})
			}
		})
	}
}
