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

func Layout2(ctx context.Context, g *d2graph.Graph, layoutFn func(ctx context.Context, g *d2graph.Graph) error) error {
	return nil
}

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	sd := &sequenceDiagram{
		graph:          g,
		objectRank:     make(map[*d2graph.Object]int),
		minMessageRank: make(map[*d2graph.Object]int),
		maxMessageRank: make(map[*d2graph.Object]int),
		messageYStep:   MIN_MESSAGE_DISTANCE,
		actorXStep:     MIN_ACTOR_DISTANCE,
		maxActorHeight: 0.,
	}

	sd.init()
	sd.placeActors()
	sd.placeSpans()
	sd.routeMessages()
	sd.addLifelineEdges()

	return nil
}

type sequenceDiagram struct {
	graph *d2graph.Graph

	messages []*d2graph.Edge
	actors   []*d2graph.Object
	spans    []*d2graph.Object

	// can be either actors or spans
	// rank: left to right position of actors/spans (spans have the same rank as their parents)
	objectRank map[*d2graph.Object]int

	// keep track of the first and last message of a given actor/span
	// the message rank is the order in which it appears from top to bottom
	minMessageRank map[*d2graph.Object]int
	maxMessageRank map[*d2graph.Object]int

	messageYStep   float64
	actorXStep     float64
	maxActorHeight float64
}

func (sd *sequenceDiagram) init() {
	sd.messages = make([]*d2graph.Edge, len(sd.graph.Edges))
	copy(sd.messages, sd.graph.Edges)

	queue := make([]*d2graph.Object, len(sd.graph.Root.ChildrenArray))
	copy(queue, sd.graph.Root.ChildrenArray)
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]

		if sd.isActor(obj) {
			sd.actors = append(sd.actors, obj)
			sd.objectRank[obj] = len(sd.actors)
			sd.maxActorHeight = math.Max(sd.maxActorHeight, obj.Height)
		} else {
			// spans are always rectangles and have no labels
			obj.Attributes.Label = d2graph.Scalar{Value: ""}
			obj.Attributes.Shape = d2graph.Scalar{Value: shape.SQUARE_TYPE}
			sd.spans = append(sd.spans, obj)
			sd.objectRank[obj] = sd.objectRank[obj.Parent]
		}

		queue = append(queue, obj.ChildrenArray...)
	}

	for rank, message := range sd.messages {
		sd.messageYStep = math.Max(sd.messageYStep, float64(message.LabelDimensions.Height))

		sd.setMinMaxMessageRank(message.Src, rank)
		sd.setMinMaxMessageRank(message.Dst, rank)

		// ensures that long labels, spanning over multiple actors, don't make for large gaps between actors
		// by distributing the label length across the actors rank difference
		rankDiff := math.Abs(float64(sd.objectRank[message.Src]) - float64(sd.objectRank[message.Dst]))
		distributedLabelWidth := float64(message.LabelDimensions.Width) / rankDiff
		sd.actorXStep = math.Max(sd.actorXStep, distributedLabelWidth+HORIZONTAL_PAD)
	}

	sd.maxActorHeight += VERTICAL_PAD
	sd.messageYStep += VERTICAL_PAD
}

func (sd *sequenceDiagram) setMinMaxMessageRank(actor *d2graph.Object, rank int) {
	if minRank, exists := sd.minMessageRank[actor]; exists {
		sd.minMessageRank[actor] = go2.IntMin(minRank, rank)
	} else {
		sd.minMessageRank[actor] = rank
	}

	sd.maxMessageRank[actor] = go2.IntMax(sd.maxMessageRank[actor], rank)
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
	endY := sd.getMessageY(len(sd.messages))
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
	// the order is important because the only way a child span exists is if there'e an message to it
	// however, the parent span might not have a message to it and then its position is based on the child position
	// or, there can be a message to it, but it comes after the child one meaning the top left position is still based on the child
	// and not on its own message
	spanFromMostNested := make([]*d2graph.Object, len(sd.spans))
	copy(spanFromMostNested, sd.spans)
	sort.SliceStable(spanFromMostNested, func(i, j int) bool {
		return spanFromMostNested[i].Level() > spanFromMostNested[j].Level()
	})
	for _, span := range spanFromMostNested {
		// finds the position based on children
		minChildY := math.Inf(1)
		maxChildY := math.Inf(-1)
		for _, child := range span.ChildrenArray {
			minChildY = math.Min(minChildY, child.TopLeft.Y)
			maxChildY = math.Max(maxChildY, child.TopLeft.Y+child.Height)
		}

		// finds the position if there are messages to this span
		minMessageY := math.Inf(1)
		if minRank, exists := sd.minMessageRank[span]; exists {
			minMessageY = sd.getMessageY(minRank)
		}
		maxMessageY := math.Inf(-1)
		if maxRank, exists := sd.maxMessageRank[span]; exists {
			maxMessageY = sd.getMessageY(maxRank)
		}

		// if it is the same as the child top left, add some padding
		minY := math.Min(minMessageY, minChildY)
		if minY == minChildY {
			minY -= SPAN_DEPTH_GROW_FACTOR
		} else {
			minY -= SPAN_MESSAGE_PAD
		}
		maxY := math.Max(maxMessageY, maxChildY)
		if maxY == maxChildY {
			maxY += SPAN_DEPTH_GROW_FACTOR
		} else {
			maxY += SPAN_MESSAGE_PAD
		}

		height := math.Max(maxY-minY, MIN_SPAN_HEIGHT)
		// -2 because the actors count as level 1 making the first level span getting 2*SPAN_DEPTH_GROW_FACTOR
		width := SPAN_WIDTH + (float64(span.Level()-2) * SPAN_DEPTH_GROW_FACTOR)
		x := rankToX[sd.objectRank[span]] - (width / 2.)
		span.Box = geo.NewBox(geo.NewPoint(x, minY), width, height)
	}
}

// routeMessages routes horizontal edges (messages) from Src to Dst
func (sd *sequenceDiagram) routeMessages() {
	for rank, message := range sd.messages {
		isLeftToRight := message.Src.TopLeft.X < message.Dst.TopLeft.X

		// finds the proper anchor point based on the message direction
		var startX, endX float64
		if sd.isActor(message.Src) {
			startX = message.Src.Center().X
		} else if isLeftToRight {
			startX = message.Src.TopLeft.X + message.Src.Width
		} else {
			startX = message.Src.TopLeft.X
		}

		if sd.isActor(message.Dst) {
			endX = message.Dst.Center().X
		} else if isLeftToRight {
			endX = message.Dst.TopLeft.X
		} else {
			endX = message.Dst.TopLeft.X + message.Dst.Width
		}

		if isLeftToRight {
			startX += SPAN_MESSAGE_PAD
			endX -= SPAN_MESSAGE_PAD
		} else {
			startX -= SPAN_MESSAGE_PAD
			endX += SPAN_MESSAGE_PAD
		}

		messageY := sd.getMessageY(rank)
		message.Route = []*geo.Point{
			geo.NewPoint(startX, messageY),
			geo.NewPoint(endX, messageY),
		}

		if message.Attributes.Label.Value != "" {
			if isLeftToRight {
				message.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
			} else {
				// the label will be placed above the message because the orientation is based on the edge normal vector
				message.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
			}
		}
	}
}

func (sd *sequenceDiagram) getMessageY(rank int) float64 {
	// +1 so that the first message has the top padding for its label
	return ((float64(rank) + 1.) * sd.messageYStep) + sd.maxActorHeight
}

func (sd *sequenceDiagram) isActor(obj *d2graph.Object) bool {
	return obj.Parent == sd.graph.Root
}
