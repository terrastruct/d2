package d2sequence

import (
	"context"
	"fmt"
	"math"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/go2"
	"oss.terrastruct.com/d2/lib/label"
)

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	pad := 50. // 2 * 25
	edgeYStep := 100.
	actorXStep := 200.
	maxActorHeight := 0.

	var actorsInOrder []*d2graph.Object
	seen := make(map[*d2graph.Object]struct{})
	for _, edge := range g.Edges {
		if _, exists := seen[edge.Src]; !exists {
			seen[edge.Src] = struct{}{}
			actorsInOrder = append(actorsInOrder, edge.Src)
		}
		if _, exists := seen[edge.Dst]; !exists {
			seen[edge.Dst] = struct{}{}
			actorsInOrder = append(actorsInOrder, edge.Dst)
		}

		edgeYStep = math.Max(edgeYStep, float64(edge.LabelDimensions.Height)+pad)
		actorXStep = math.Max(actorXStep, float64(edge.LabelDimensions.Width)+pad)
		maxActorHeight = math.Max(maxActorHeight, edge.Src.Height+pad)
		maxActorHeight = math.Max(maxActorHeight, edge.Dst.Height+pad)
	}

	placeActors(actorsInOrder, maxActorHeight, actorXStep)
	// edges are placed in the order users define them
	routeEdges(g.Edges, maxActorHeight, edgeYStep)
	addLifelineEdges(g, actorsInOrder, edgeYStep)

	return nil
}

// placeActors places actors bottom aligned, side by side
func placeActors(actorsInOrder []*d2graph.Object, maxHeight, xStep float64) {
	x := 0.
	for _, actors := range actorsInOrder {
		yOffset := maxHeight - actors.Height
		actors.TopLeft = geo.NewPoint(x, yOffset)
		x += actors.Width + xStep
		actors.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
	}
}

// routeEdges routes horizontal edges from Src to Dst
func routeEdges(edgesInOrder []*d2graph.Edge, startY, yStep float64) {
	edgeY := startY + yStep // in case the first edge has a tall label
	for _, edge := range edgesInOrder {
		start := edge.Src.Center()
		start.Y = edgeY
		end := edge.Dst.Center()
		end.Y = edgeY
		edge.Route = []*geo.Point{start, end}
		edgeY += yStep

		if edge.Attributes.Label.Value != "" {
			// TODO: consider label right-to-left
			edge.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
		}
	}
}

// addLifelineEdges adds a new edge for each actor in the graph that represents the
// edge below the actor showing its lifespan
// ┌──────────────┐
// │     actor    │
// └──────┬───────┘
//        │
//        │ lifeline
//        │
//        │
func addLifelineEdges(g *d2graph.Graph, actorsInOrder []*d2graph.Object, yStep float64) {
	endY := g.Edges[len(g.Edges)-1].Route[0].Y + yStep
	for _, actor := range actorsInOrder {
		actorBottom := actor.Center()
		actorBottom.Y = actor.TopLeft.Y + actor.Height
		actorLifelineEnd := actor.Center()
		actorLifelineEnd.Y = endY
		g.Edges = append(g.Edges, &d2graph.Edge{
			Attributes: d2graph.Attributes{
				Style: d2graph.Style{
					StrokeDash: &d2graph.Scalar{
						Value: "10",
					},
					Stroke:      actor.Attributes.Style.Stroke,
					StrokeWidth: actor.Attributes.Style.StrokeWidth,
				},
			},
			Src:      actor,
			SrcArrow: false,
			Dst: &d2graph.Object{
				ID: actor.ID + fmt.Sprintf("-lifeline-end-%d", go2.StringToIntHash(actor.ID+"-lifeline-end")),
			},
			DstArrow: false,
			Route:    []*geo.Point{actorBottom, actorLifelineEnd},
		})
	}
}
