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
	sd := &sequenceDiagram{
		graph:          g,
		edgeYStep:      MIN_EDGE_DISTANCE,
		actorXStep:     MIN_ACTOR_DISTANCE,
		maxActorHeight: 0.,
	}

	actorRank := make(map[*d2graph.Object]int)
	for rank, actor := range g.Objects {
		actorRank[actor] = rank
	}
	for _, edge := range g.Edges {
		sd.edgeYStep = math.Max(sd.edgeYStep, float64(edge.LabelDimensions.Height)+HORIZONTAL_PAD)
		sd.maxActorHeight = math.Max(sd.maxActorHeight, edge.Src.Height+HORIZONTAL_PAD)
		sd.maxActorHeight = math.Max(sd.maxActorHeight, edge.Dst.Height+HORIZONTAL_PAD)
		// ensures that long labels, spanning over multiple actors, don't make for large gaps between actors
		// by distributing the label length across the actors rank difference
		rankDiff := math.Abs(float64(actorRank[edge.Src]) - float64(actorRank[edge.Dst]))
		distributedLabelWidth := float64(edge.LabelDimensions.Width) / rankDiff
		sd.actorXStep = math.Max(sd.actorXStep, distributedLabelWidth+HORIZONTAL_PAD)
	}

	sd.placeActors()
	sd.routeEdges()
	sd.addLifelineEdges()

	return nil
}

type sequenceDiagram struct {
	graph *d2graph.Graph

	edgeYStep      float64
	actorXStep     float64
	maxActorHeight float64
}

// placeActors places actors bottom aligned, side by side
func (sd *sequenceDiagram) placeActors() {
	x := 0.
	for _, actors := range sd.graph.Objects {
		yOffset := sd.maxActorHeight - actors.Height
		actors.TopLeft = geo.NewPoint(x, yOffset)
		x += actors.Width + sd.actorXStep
		actors.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
	}
}

// routeEdges routes horizontal edges from Src to Dst
func (sd *sequenceDiagram) routeEdges() {
	edgeY := sd.maxActorHeight + sd.edgeYStep // in case the first edge has a tall label
	for _, edge := range sd.graph.Edges {
		start := edge.Src.Center()
		start.Y = edgeY
		end := edge.Dst.Center()
		end.Y = edgeY
		edge.Route = []*geo.Point{start, end}
		edgeY += sd.edgeYStep

		if edge.Attributes.Label.Value != "" {
			isLeftToRight := edge.Src.TopLeft.X < edge.Dst.TopLeft.X
			if isLeftToRight {
				edge.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
			} else {
				edge.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
			}
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
func (sd *sequenceDiagram) addLifelineEdges() {
	endY := sd.graph.Edges[len(sd.graph.Edges)-1].Route[0].Y + sd.edgeYStep
	for _, actor := range sd.graph.Objects {
		actorBottom := actor.Center()
		actorBottom.Y = actor.TopLeft.Y + actor.Height
		actorLifelineEnd := actor.Center()
		actorLifelineEnd.Y = endY
		sd.graph.Edges = append(sd.graph.Edges, &d2graph.Edge{
			Attributes: d2graph.Attributes{
				Style: d2graph.Style{
					StrokeDash:  &d2graph.Scalar{Value: "10"},
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
