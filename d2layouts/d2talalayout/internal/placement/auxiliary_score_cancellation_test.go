package placement

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func TestMoveNodeToBestAuxiliaryScoreCancellationLocation(t *testing.T) {
	tests := []struct {
		name  string
		table bool
	}{
		{name: "column crossing", table: true},
		{name: "symmetry", table: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			node := layoutgraph.NewNode(1, 10, 10)
			node.TopLeft = geo.NewPoint(0, 0)
			if tt.table {
				node.SetShape(shape.TABLE_TYPE)
				node.SetNumColumns(1)
			}
			g.AddNewNodeToContainer(nil, node)
			child := layoutgraph.NewNode(2, 1, 1)
			child.TopLeft = geo.NewPoint(0.1, 0.1)
			g.AddNewNodeToContainer(node, child)
			g.ComputeCellSize()

			// NodeEdgeLength's preflight and final checks succeed. Column scoring is
			// always polled next; non-tables then continue into symmetry scoring.
			remaining := 5
			if tt.table {
				remaining = 4
			}
			ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: remaining}
			guard, err := limits.NewOptimizationWorkGuard(ctx, "CompactionMoves", limits.MaxOptimizationWorkUnits)
			if err != nil {
				t.Fatal(err)
			}
			moved, err := moveNodeToBest(
				ctx, g, node,
				[]*geo.Point{geo.NewPoint(50, 50)},
				nil,
				true,
				guard,
			)
			requireCanceledAt(t, err, "EdgeLength")
			if moved {
				t.Fatal("move reported success after cancellation")
			}
		})
	}
}
