package d2sequence

import (
	"context"
	"fmt"
	"math"
	"sort"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/go2"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	sd := &sequenceDiagram{
		graph:          g,
		objectRank:     make(map[*d2graph.Object]int),
		objectLevel:    make(map[*d2graph.Object]int),
		minEdgeRank:    make(map[*d2graph.Object]int),
		maxEdgeRank:    make(map[*d2graph.Object]int),
		edgeYStep:      MIN_EDGE_DISTANCE,
		actorXStep:     MIN_ACTOR_DISTANCE,
		maxActorHeight: 0.,
	}

	sd.init()
	sd.placeActors()
	sd.placeSpans()
	sd.routeEdges()
	sd.addLifelineEdges()

	return nil
}

type sequenceDiagram struct {
	graph *d2graph.Graph

	edges  []*d2graph.Edge
	actors []*d2graph.Object
	spans  []*d2graph.Object

	// can be either actors or spans
	// rank: left to right position of actors/spans (spans have the same rank as their parents)
	objectRank map[*d2graph.Object]int
	// similar to d2graph.Object.Level() just don't make the recursive calls
	objectLevel map[*d2graph.Object]int

	// keep track of the first and last edge of a given actor
	// the edge rank is the order in which it appears from top to bottom
	minEdgeRank map[*d2graph.Object]int
	maxEdgeRank map[*d2graph.Object]int

	edgeYStep      float64
	actorXStep     float64
	maxActorHeight float64
}

func (sd *sequenceDiagram) init() {
	sd.edges = make([]*d2graph.Edge, len(sd.graph.Edges))
	copy(sd.edges, sd.graph.Edges)

	queue := make([]*d2graph.Object, len(sd.graph.Root.ChildrenArray))
	copy(queue, sd.graph.Root.ChildrenArray)
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]

		if sd.isActor(obj) {
			sd.actors = append(sd.actors, obj)
			sd.objectRank[obj] = len(sd.actors)
			sd.objectLevel[obj] = 0
			sd.maxActorHeight = math.Max(sd.maxActorHeight, obj.Height)
		} else {
			// spans are always rectangles and have no labels
			obj.Attributes.Label = d2graph.Scalar{Value: ""}
			obj.Attributes.Shape = d2graph.Scalar{Value: shape.SQUARE_TYPE}
			sd.spans = append(sd.spans, obj)
			sd.objectRank[obj] = sd.objectRank[obj.Parent]
			sd.objectLevel[obj] = sd.objectLevel[obj.Parent] + 1
		}

		queue = append(queue, obj.ChildrenArray...)
	}

	for rank, edge := range sd.edges {
		sd.edgeYStep = math.Max(sd.edgeYStep, float64(edge.LabelDimensions.Height))

		sd.setMinMaxEdgeRank(edge.Src, rank)
		sd.setMinMaxEdgeRank(edge.Dst, rank)

		// ensures that long labels, spanning over multiple actors, don't make for large gaps between actors
		// by distributing the label length across the actors rank difference
		rankDiff := math.Abs(float64(sd.objectRank[edge.Src]) - float64(sd.objectRank[edge.Dst]))
		distributedLabelWidth := float64(edge.LabelDimensions.Width) / rankDiff
		sd.actorXStep = math.Max(sd.actorXStep, distributedLabelWidth+HORIZONTAL_PAD)
	}

	sd.maxActorHeight += VERTICAL_PAD
	sd.edgeYStep += VERTICAL_PAD
}

func (sd *sequenceDiagram) setMinMaxEdgeRank(actor *d2graph.Object, rank int) {
	if minRank, exists := sd.minEdgeRank[actor]; exists {
		sd.minEdgeRank[actor] = go2.IntMin(minRank, rank)
	} else {
		sd.minEdgeRank[actor] = rank
	}

	sd.maxEdgeRank[actor] = go2.IntMax(sd.maxEdgeRank[actor], rank)
}

// placeActors places actors bottom aligned, side by side
func (sd *sequenceDiagram) placeActors() {
	x := 0.
	for _, actors := range sd.actors {
		yOffset := sd.maxActorHeight - actors.Height
		actors.TopLeft = geo.NewPoint(x, yOffset)
		x += actors.Width + sd.actorXStep
		actors.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
	}
}

// addLifelineEdges adds a new edge for each actor in the graph that represents the its lifeline
// ┌──────────────┐
// │     actor    │
// └──────┬───────┘
//        │
//        │ lifeline
//        │
//        │
func (sd *sequenceDiagram) addLifelineEdges() {
	endY := sd.getEdgeY(len(sd.edges))
	for _, actor := range sd.actors {
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

// placeSpans places spans over the object lifeline
// ┌──────────┐
// │  actor   │
// └────┬─────┘
//    ┌─┴──┐
//    │    │
//    |span|
//    │    │
//    └─┬──┘
//      │
//   lifeline
//      │
func (sd *sequenceDiagram) placeSpans() {
	// quickly find the span center X
	rankToX := make(map[int]float64)
	for _, actor := range sd.actors {
		rankToX[sd.objectRank[actor]] = actor.Center().X
	}

	// places spans from most to least nested
	// the order is important because the only way a child span exists is if there'e an edge to it
	// however, the parent span might not have an edge to it and then its position is based on the child position
	// or, there can be edge to it, but it comes after the child one meaning the top left position is still based on the child
	// and not on its own edge
	spanFromMostNested := make([]*d2graph.Object, len(sd.spans))
	copy(spanFromMostNested, sd.spans)
	sort.SliceStable(spanFromMostNested, func(i, j int) bool {
		return sd.objectLevel[spanFromMostNested[i]] > sd.objectLevel[spanFromMostNested[j]]
	})
	for _, span := range spanFromMostNested {
		// finds the position based on children
		minChildY := math.Inf(1)
		maxChildY := math.Inf(-1)
		for _, child := range span.ChildrenArray {
			minChildY = math.Min(minChildY, child.TopLeft.Y)
			maxChildY = math.Max(maxChildY, child.TopLeft.Y+child.Height)
		}

		// finds the position if there are edges to this span
		minEdgeY := math.Inf(1)
		if minRank, exists := sd.minEdgeRank[span]; exists {
			minEdgeY = sd.getEdgeY(minRank)
		}
		maxEdgeY := math.Inf(-1)
		if maxRank, exists := sd.maxEdgeRank[span]; exists {
			maxEdgeY = sd.getEdgeY(maxRank)
		}

		// if it is the same as the child top left, add some padding
		minY := math.Min(minEdgeY, minChildY)
		if minY == minChildY {
			minY -= SPAN_DEPTH_GROW_FACTOR
		} else {
			minY -= SPAN_EDGE_PAD
		}
		maxY := math.Max(maxEdgeY, maxChildY)
		if maxY == maxChildY {
			maxY += SPAN_DEPTH_GROW_FACTOR
		} else {
			maxY += SPAN_EDGE_PAD
		}

		height := math.Max(maxY-minY, MIN_SPAN_HEIGHT)
		width := SPAN_WIDTH + (float64(sd.objectLevel[span]-1) * SPAN_DEPTH_GROW_FACTOR)
		x := rankToX[sd.objectRank[span]] - (width / 2.)
		span.Box = geo.NewBox(geo.NewPoint(x, minY), width, height)
	}
}

// routeEdges routes horizontal edges from Src to Dst
func (sd *sequenceDiagram) routeEdges() {
	for rank, edge := range sd.edges {
		isLeftToRight := edge.Src.TopLeft.X < edge.Dst.TopLeft.X

		// finds the proper anchor point based on the edge direction
		var startX, endX float64
		if sd.isActor(edge.Src) {
			startX = edge.Src.Center().X
		} else if isLeftToRight {
			startX = edge.Src.TopLeft.X + edge.Src.Width
		} else {
			startX = edge.Src.TopLeft.X
		}

		if sd.isActor(edge.Dst) {
			endX = edge.Dst.Center().X
		} else if isLeftToRight {
			endX = edge.Dst.TopLeft.X
		} else {
			endX = edge.Dst.TopLeft.X + edge.Dst.Width
		}

		if isLeftToRight {
			startX += SPAN_EDGE_PAD
			endX -= SPAN_EDGE_PAD
		} else {
			startX -= SPAN_EDGE_PAD
			endX += SPAN_EDGE_PAD
		}

		edgeY := sd.getEdgeY(rank)
		edge.Route = []*geo.Point{
			geo.NewPoint(startX, edgeY),
			geo.NewPoint(endX, edgeY),
		}

		if edge.Attributes.Label.Value != "" {
			if isLeftToRight {
				edge.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
			} else {
				// the label will be placed above the edge because the orientation is based on the edge normal vector
				edge.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
			}
		}
	}
}

func (sd *sequenceDiagram) getEdgeY(rank int) float64 {
	// +1 so that the first edge has the top padding for its label
	return ((float64(rank) + 1.) * sd.edgeYStep) + sd.maxActorHeight
}

func (sd *sequenceDiagram) isActor(obj *d2graph.Object) bool {
	return obj.Parent == sd.graph.Root
}
