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
)

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	sd := &sequenceDiagram{
		graph:          g,
		objectRank:     make(map[*d2graph.Object]int),
		objectDepth:    make(map[*d2graph.Object]int),
		minEdgeRank:    make(map[*d2graph.Object]int),
		maxEdgeRank:    make(map[*d2graph.Object]int),
		edgeYStep:      MIN_EDGE_DISTANCE,
		actorXStep:     MIN_ACTOR_DISTANCE,
		maxActorHeight: 0.,
	}

	sd.init()
	sd.placeActors()
	sd.placeLifespan()
	sd.routeEdges()
	sd.addLifelineEdges()

	return nil
}

type sequenceDiagram struct {
	graph *d2graph.Graph

	edges     []*d2graph.Edge
	actors    []*d2graph.Object
	lifespans []*d2graph.Object

	// can be either actors or lifespans
	objectRank  map[*d2graph.Object]int
	objectDepth map[*d2graph.Object]int

	// keep track of the first and last edge of a given actor
	// needed for lifespan
	minEdgeRank map[*d2graph.Object]int
	maxEdgeRank map[*d2graph.Object]int

	edgeYStep      float64
	actorXStep     float64
	maxActorHeight float64
}

func intMin(a, b int) int {
	return int(math.Min(float64(a), float64(b)))
}

func intMax(a, b int) int {
	return int(math.Max(float64(a), float64(b)))
}

func (sd *sequenceDiagram) init() {
	sd.edges = make([]*d2graph.Edge, len(sd.graph.Edges))
	copy(sd.edges, sd.graph.Edges)

	queue := make([]*d2graph.Object, len(sd.graph.Root.ChildrenArray))
	copy(queue, sd.graph.Root.ChildrenArray)
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]

		if obj.Parent == sd.graph.Root {
			sd.actors = append(sd.actors, obj)
			sd.objectRank[obj] = len(sd.actors)
			sd.objectDepth[obj] = 0
		} else if obj != sd.graph.Root {
			obj.Attributes.Label = d2graph.Scalar{Value: ""}
			sd.lifespans = append(sd.lifespans, obj)
			sd.objectRank[obj] = sd.objectRank[obj.Parent]
			sd.objectDepth[obj] = sd.objectDepth[obj.Parent] + 1
		}

		queue = append(queue, obj.ChildrenArray...)
	}

	for rank, edge := range sd.edges {
		if edge.Src.Parent == sd.graph.Root {
			sd.maxActorHeight = math.Max(sd.maxActorHeight, edge.Src.Height+HORIZONTAL_PAD)
		}
		if edge.Dst.Parent == sd.graph.Root {
			sd.maxActorHeight = math.Max(sd.maxActorHeight, edge.Dst.Height+HORIZONTAL_PAD)
		}
		sd.edgeYStep = math.Max(sd.edgeYStep, float64(edge.LabelDimensions.Height)+HORIZONTAL_PAD)

		sd.setMinMaxEdgeRank(edge.Src, rank)
		sd.setMinMaxEdgeRank(edge.Dst, rank)

		// ensures that long labels, spanning over multiple actors, don't make for large gaps between actors
		// by distributing the label length across the actors rank difference
		rankDiff := math.Abs(float64(sd.objectRank[edge.Src]) - float64(sd.objectRank[edge.Dst]))
		distributedLabelWidth := float64(edge.LabelDimensions.Width) / rankDiff
		sd.actorXStep = math.Max(sd.actorXStep, distributedLabelWidth+HORIZONTAL_PAD)
	}
}

func (sd *sequenceDiagram) setMinMaxEdgeRank(actor *d2graph.Object, rank int) {
	if minRank, exists := sd.minEdgeRank[actor]; exists {
		sd.minEdgeRank[actor] = intMin(minRank, rank)
	} else {
		sd.minEdgeRank[actor] = rank
	}

	sd.maxEdgeRank[actor] = intMax(sd.maxEdgeRank[actor], rank)
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

// placeLifespan places lifespan boxes over the object lifeline
// ┌──────────┐
// │  actor   │
// └────┬─────┘
//    ┌─┴──┐
//    │    │
//   lifespan
//    │    │
//    └─┬──┘
//      │
//   lifeline
//      │
func (sd *sequenceDiagram) placeLifespan() {
	rankToX := make(map[int]float64)
	for _, actor := range sd.actors {
		rankToX[sd.objectRank[actor]] = actor.Center().X
	}

	lifespanFromMostNested := make([]*d2graph.Object, len(sd.lifespans))
	copy(lifespanFromMostNested, sd.lifespans)
	sort.SliceStable(lifespanFromMostNested, func(i, j int) bool {
		return sd.objectDepth[lifespanFromMostNested[i]] > sd.objectDepth[lifespanFromMostNested[j]]
	})
	for _, lifespan := range lifespanFromMostNested {
		minChildY := math.Inf(1)
		maxChildY := math.Inf(-1)
		for _, child := range lifespan.ChildrenArray {
			minChildY = math.Min(minChildY, child.TopLeft.Y)
			maxChildY = math.Max(maxChildY, child.TopLeft.Y+child.Height)
		}

		minEdgeY := math.Inf(1)
		if minRank, exists := sd.minEdgeRank[lifespan]; exists {
			minEdgeY = sd.getEdgeY(minRank)
		}
		maxEdgeY := math.Inf(-1)
		if maxRank, exists := sd.maxEdgeRank[lifespan]; exists {
			maxEdgeY = sd.getEdgeY(maxRank)
		}

		minY := math.Min(minEdgeY, minChildY)
		if minY == minChildY {
			minY -= LIFESPAN_DEPTH_GROW_FACTOR
		}
		maxY := math.Max(maxEdgeY, maxChildY)
		if maxY == maxChildY {
			maxY += LIFESPAN_DEPTH_GROW_FACTOR
		}

		height := maxY - minY
		height = math.Max(height, DEFAULT_LIFESPAN_BOX_HEIGHT)

		width := LIFESPAN_BOX_WIDTH + (float64(sd.objectDepth[lifespan]-1) * LIFESPAN_DEPTH_GROW_FACTOR)

		x := rankToX[sd.objectRank[lifespan]] - (width / 2.)
		lifespan.Box = geo.NewBox(geo.NewPoint(x, minY), width, height)
	}
}

// routeEdges routes horizontal edges from Src to Dst
func (sd *sequenceDiagram) routeEdges() {
	for rank, edge := range sd.edges {
		isLeftToRight := edge.Src.TopLeft.X < edge.Dst.TopLeft.X

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
		edgeY := sd.getEdgeY(rank)
		edge.Route = []*geo.Point{
			geo.NewPoint(startX, edgeY),
			geo.NewPoint(endX, edgeY),
		}

		if edge.Attributes.Label.Value != "" {
			if isLeftToRight {
				edge.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
			} else {
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
