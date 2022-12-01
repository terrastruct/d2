package d2sequence

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/go2"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

func Layout2(ctx context.Context, g *d2graph.Graph, layout func(ctx context.Context, g *d2graph.Graph) error) error {
	// new graph objects without sequence diagram objects and their replacement (rectangle node)
	newObjects := make([]*d2graph.Object, 0, len(g.Objects))
	edgesToRemove := make(map[*d2graph.Edge]struct{})

	sequenceDiagrams := make(map[*d2graph.Object]*sequenceDiagram)

	objChildrenArray := make(map[*d2graph.Object][]*d2graph.Object)

	queue := make([]*d2graph.Object, 1, len(g.Objects))
	queue[0] = g.Root
	for len(queue) > 0 {
		obj := queue[0]
		queue = queue[1:]

		newObjects = append(newObjects, obj)
		if obj.Attributes.Shape.Value == d2target.ShapeSequenceDiagram {
			// TODO: should update obj.References too?

			// clean current children and keep a backup to restore them later
			obj.Children = make(map[string]*d2graph.Object)
			objChildrenArray[obj] = obj.ChildrenArray
			obj.ChildrenArray = nil
			// creates a mock rectangle so that layout considers this size
			sdMock := obj.EnsureChild([]string{"sequence_diagram"})
			sdMock.Attributes.Shape.Value = d2target.ShapeRectangle
			sdMock.Attributes.Label.Value = ""
			newObjects = append(newObjects, sdMock)

			var messages []*d2graph.Edge
			for _, edge := range g.Edges {
				if strings.HasPrefix(edge.Src.AbsID(), obj.AbsID()) && strings.HasPrefix(edge.Dst.AbsID(), obj.AbsID()) {
					edgesToRemove[edge] = struct{}{}
					messages = append(messages, edge)
				}
			}

			sd := newSequenceDiagram(objChildrenArray[obj], messages)
			sd.layout()
			sdMock.Box = geo.NewBox(nil, sd.getWidth(), sd.getHeight())
			sequenceDiagrams[obj] = sd
		} else {
			queue = append(queue, obj.ChildrenArray...)
		}
	}

	newEdges := make([]*d2graph.Edge, 0, len(g.Edges)-len(edgesToRemove))
	for _, edge := range g.Edges {
		if _, exists := edgesToRemove[edge]; !exists {
			newEdges = append(newEdges, edge)
		}
	}

	g.Objects = newObjects
	g.Edges = newEdges

	if err := layout(ctx, g); err != nil {
		return err
	}

	// restores objects & edges
	for edge := range edgesToRemove {
		g.Edges = append(g.Edges, edge)
	}
	for obj, children := range objChildrenArray {
		sdMock := obj.ChildrenArray[0]
		sequenceDiagrams[obj].shift(sdMock.TopLeft)
		obj.Children = make(map[string]*d2graph.Object)
		for _, child := range children {
			g.Objects = append(g.Objects, child)
			obj.Children[child.ID] = child
		}
		obj.ChildrenArray = children

		for _, edge := range sequenceDiagrams[obj].lifelines {
			g.Edges = append(g.Edges, edge)
		}
	}

	return nil
}

func Layout(ctx context.Context, g *d2graph.Graph) (err error) {
	sd := newSequenceDiagram(nil, nil)
	sd.layout()
	return nil
}

type sequenceDiagram struct {
	messages  []*d2graph.Edge
	lifelines []*d2graph.Edge
	actors    []*d2graph.Object
	spans     []*d2graph.Object

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

func newSequenceDiagram(actors []*d2graph.Object, messages []*d2graph.Edge) *sequenceDiagram {
	sd := &sequenceDiagram{
		messages:       messages,
		actors:         actors,
		spans:          nil,
		lifelines:      nil,
		objectRank:     make(map[*d2graph.Object]int),
		minMessageRank: make(map[*d2graph.Object]int),
		maxMessageRank: make(map[*d2graph.Object]int),
		messageYStep:   MIN_MESSAGE_DISTANCE,
		actorXStep:     MIN_ACTOR_DISTANCE,
		maxActorHeight: 0.,
	}

	for rank, actor := range actors {
		sd.objectRank[actor] = rank
		sd.maxActorHeight = math.Max(sd.maxActorHeight, actor.Height)

		queue := make([]*d2graph.Object, len(actor.ChildrenArray))
		copy(queue, actor.ChildrenArray)
		for len(queue) > 0 {
			span := queue[0]
			queue = queue[1:]

			// spans are always rectangles and have no labels
			span.Attributes.Label = d2graph.Scalar{Value: ""}
			span.Attributes.Shape = d2graph.Scalar{Value: shape.SQUARE_TYPE}
			sd.spans = append(sd.spans, span)
			sd.objectRank[span] = rank

			queue = append(queue, span.ChildrenArray...)
		}
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

	return sd
}

func (sd *sequenceDiagram) setMinMaxMessageRank(actor *d2graph.Object, rank int) {
	if minRank, exists := sd.minMessageRank[actor]; exists {
		sd.minMessageRank[actor] = go2.IntMin(minRank, rank)
	} else {
		sd.minMessageRank[actor] = rank
	}

	sd.maxMessageRank[actor] = go2.IntMax(sd.maxMessageRank[actor], rank)
}

func (sd *sequenceDiagram) layout() {
	sd.placeActors()
	sd.placeSpans()
	sd.routeMessages()
	sd.addLifelineEdges()
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
		sd.lifelines = append(sd.lifelines, &d2graph.Edge{
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
		span.ZIndex = 1
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
	// TODO: map to avoid looping around every time?
	for _, actor := range sd.actors {
		if actor == obj {
			return true
		}
	}
	return false
}

func (sd *sequenceDiagram) getWidth() float64 {
	// the layout is always placed starting at 0, so the width is just the last actor
	lastActor := sd.actors[len(sd.actors)-1]
	return lastActor.TopLeft.X + lastActor.Width
}

func (sd *sequenceDiagram) getHeight() float64 {
	// the layout is always placed starting at 0, so the height is just the last message
	return sd.getMessageY(len(sd.messages))
}

func (sd *sequenceDiagram) shift(tl *geo.Point) {
	allObjects := append([]*d2graph.Object{}, sd.actors...)
	allObjects = append(allObjects, sd.spans...)
	for _, obj := range allObjects {
		obj.TopLeft.X += tl.X
		obj.TopLeft.Y += tl.Y
	}

	allEdges := append([]*d2graph.Edge{}, sd.messages...)
	allEdges = append(allEdges, sd.lifelines...)
	for _, edge := range allEdges {
		for _, p := range edge.Route {
			p.X += tl.X
			p.Y += tl.Y
		}
	}
}
