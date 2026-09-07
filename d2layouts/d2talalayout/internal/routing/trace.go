package routing

import (
	"context"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

// TraceEdgesToShapeBorder trims every route endpoint to the rendered shape.
func TraceEdgesToShapeBorder(ctx context.Context, graph *layoutgraph.Graph) error {
	return traceEdgesToShapeBorderWithWorkLimit(ctx, graph, maxRouteStageWorkUnits)
}

func traceEdgesToShapeBorderWithWorkLimit(ctx context.Context, graph *layoutgraph.Graph, workLimit uint64) error {
	return runAtomicRouteStage(ctx, "TraceEdgesToShapeBorder", graph, nil, workLimit, func(guard *routeWorkGuard) error {
		for _, edge := range graph.Edges {
			if err := traceToShapeBorderGuarded(edge, guard); err != nil {
				return err
			}
		}
		return nil
	})
}

func traceToShapeBorder(edge *layoutgraph.Edge) {
	if edge == nil || edge.From == nil || edge.To == nil || len(edge.Points) < 2 {
		return
	}
	fromNode, toNode := edge.From, edge.To
	var originalFromTopLeft, originalToTopLeft *geo.Point
	var originalFromPointer, originalToPointer *geo.Point
	defer func() {
		// Restore in reverse mutation order in case both endpoints share a node.
		if originalToTopLeft != nil {
			*originalToPointer = *originalToTopLeft
			toNode.TopLeft = originalToPointer
		}
		if originalFromTopLeft != nil {
			*originalFromPointer = *originalFromTopLeft
			fromNode.TopLeft = originalFromPointer
		}
	}()

	// If an edge passes through a 3D/multiple modifier, use the offset box for
	// tracing to the rendered border.
	if dx, dy := edge.From.ModifierElementAdjustments(); dx != 0 || dy != 0 {
		start := edge.Points[0]
		if start.X > edge.From.TopLeft.X+dx &&
			start.Y < edge.From.TopLeft.Y+edge.From.Height-dy {
			originalFromPointer = edge.From.TopLeft
			originalFromTopLeft = edge.From.TopLeft.Copy()
			orientation := edge.From.PortOrientation(start)

			// If connected to the top or right side, move the segment back
			// before tracing to the border.
			next := edge.Points[1]
			if orientation == geo.Right || start.X == originalFromTopLeft.X+edge.From.Width {
				edge.From.TopLeft.X += dx
				topRight := edge.From.TopLeft.AddVector(geo.Vector{edge.From.Width, 0})
				bottomRight := topRight.AddVector(geo.Vector{0, edge.From.Height})
				newStart := geo.IntersectionPoint(start, next, topRight, bottomRight)
				if newStart != nil {
					start.X = newStart.X
					start.Y = newStart.Y
				}
			} else if orientation == geo.Top || start.Y == originalFromTopLeft.Y {
				edge.From.TopLeft.Y -= dy
				topRight := edge.From.TopLeft.AddVector(geo.Vector{edge.From.Width, 0})
				newStart := geo.IntersectionPoint(start, next, edge.From.TopLeft, topRight)
				if newStart != nil {
					start.X = newStart.X
					start.Y = newStart.Y
				}
			}
		}
	}
	if dx, dy := edge.To.ModifierElementAdjustments(); dx != 0 || dy != 0 {
		end := edge.Points[len(edge.Points)-1]
		if end.X > edge.To.TopLeft.X+dx &&
			end.Y < edge.To.TopLeft.Y+edge.To.Height-dy {
			originalToPointer = edge.To.TopLeft
			originalToTopLeft = edge.To.TopLeft.Copy()
			orientation := edge.To.PortOrientation(end)

			previous := edge.Points[len(edge.Points)-2]
			if orientation == geo.Right || end.X == originalToTopLeft.X+edge.To.Width {
				edge.To.TopLeft.X += dx
				topRight := edge.To.TopLeft.AddVector(geo.Vector{edge.To.Width, 0})
				bottomRight := topRight.AddVector(geo.Vector{0, edge.To.Height})
				newEnd := geo.IntersectionPoint(end, previous, topRight, bottomRight)
				if newEnd != nil {
					end.X = newEnd.X
					end.Y = newEnd.Y
				}
			} else if orientation == geo.Top || end.Y == originalToTopLeft.Y {
				edge.To.TopLeft.Y -= dy
				topRight := edge.To.TopLeft.AddVector(geo.Vector{edge.To.Width, 0})
				newEnd := geo.IntersectionPoint(end, previous, edge.To.TopLeft, topRight)
				if newEnd != nil {
					end.X = newEnd.X
					end.Y = newEnd.Y
				}
			}
		}
	}

	borderPoint := shape.TraceToShapeBorder(edge.From.Shape, edge.Points[0], edge.Points[1])
	if borderPoint != nil {
		edge.Points[0] = borderPoint
	}
	borderPoint = shape.TraceToShapeBorder(edge.To.Shape, edge.Points[len(edge.Points)-1], edge.Points[len(edge.Points)-2])
	if borderPoint != nil {
		edge.Points[len(edge.Points)-1] = borderPoint
	}
}
