package d2sequence

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

type sequenceDiagram struct {
	root      *d2graph.Object
	messages  []*d2graph.Edge
	lifelines []*d2graph.Edge
	actors    []*d2graph.Object
	spans     []*d2graph.Object

	// can be either actors or spans
	// rank: left to right position of actors/spans (spans have the same rank as their parents)
	objectRank map[*d2graph.Object]int

	// keep track of the first and last message of a given actor/span
	firstMessage map[*d2graph.Object]*d2graph.Edge
	lastMessage  map[*d2graph.Object]*d2graph.Edge

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
		firstMessage:   make(map[*d2graph.Object]*d2graph.Edge),
		lastMessage:    make(map[*d2graph.Object]*d2graph.Edge),
		messageYStep:   MIN_MESSAGE_DISTANCE,
		actorXStep:     MIN_ACTOR_DISTANCE,
		maxActorHeight: 0.,
	}

	for rank, actor := range actors {
		sd.root = actor.Parent
		sd.objectRank[actor] = rank

		if actor.Width < MIN_ACTOR_WIDTH {
			aspectRatio := actor.Height / actor.Width
			actor.Width = MIN_ACTOR_WIDTH
			actor.Height = math.Round(aspectRatio * actor.Width)
		}
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

	for _, message := range sd.messages {
		sd.messageYStep = math.Max(sd.messageYStep, float64(message.LabelDimensions.Height))

		// ensures that long labels, spanning over multiple actors, don't make for large gaps between actors
		// by distributing the label length across the actors rank difference
		rankDiff := math.Abs(float64(sd.objectRank[message.Src]) - float64(sd.objectRank[message.Dst]))
		if rankDiff != 0 {
			// rankDiff = 0 for self edges
			distributedLabelWidth := float64(message.LabelDimensions.Width) / rankDiff
			sd.actorXStep = math.Max(sd.actorXStep, distributedLabelWidth+HORIZONTAL_PAD)

		}
		sd.lastMessage[message.Src] = message
		if _, exists := sd.firstMessage[message.Src]; !exists {
			sd.firstMessage[message.Src] = message
		}
		sd.lastMessage[message.Dst] = message
		if _, exists := sd.firstMessage[message.Dst]; !exists {
			sd.firstMessage[message.Dst] = message
		}

	}

	sd.messageYStep += VERTICAL_PAD
	sd.maxActorHeight += VERTICAL_PAD
	if sd.root.LabelHeight != nil {
		sd.maxActorHeight += float64(*sd.root.LabelHeight)
	}

	return sd
}

func (sd *sequenceDiagram) layout() error {
	sd.placeActors()
	if err := sd.routeMessages(); err != nil {
		return err
	}
	sd.placeSpans()
	sd.adjustRouteEndpoints()
	sd.addLifelineEdges()
	return nil
}

// placeActors places actors bottom aligned, side by side
func (sd *sequenceDiagram) placeActors() {
	x := 0.
	for _, actor := range sd.actors {
		shape := actor.Attributes.Shape.Value
		var yOffset float64
		if shape == d2target.ShapeImage || shape == d2target.ShapePerson {
			actor.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
			yOffset = sd.maxActorHeight - actor.Height - float64(*actor.LabelHeight)
		} else {
			actor.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
			yOffset = sd.maxActorHeight - actor.Height
		}
		actor.TopLeft = geo.NewPoint(x, yOffset)
		x += actor.Width + sd.actorXStep
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
	lastRoute := sd.messages[len(sd.messages)-1].Route
	endY := lastRoute[len(lastRoute)-1].Y + MIN_MESSAGE_DISTANCE
	for _, actor := range sd.actors {
		actorBottom := actor.Center()
		actorBottom.Y = actor.TopLeft.Y + actor.Height
		if *actor.LabelPosition == string(label.OutsideBottomCenter) {
			actorBottom.Y += float64(*actor.LabelHeight) + LIFELINE_LABEL_PAD
		}
		actorLifelineEnd := actor.Center()
		actorLifelineEnd.Y = endY
		sd.lifelines = append(sd.lifelines, &d2graph.Edge{
			Attributes: d2graph.Attributes{
				Style: d2graph.Style{
					StrokeDash:  &d2graph.Scalar{Value: fmt.Sprintf("%d", LIFELINE_STROKE_DASH)},
					StrokeWidth: &d2graph.Scalar{Value: fmt.Sprintf("%d", LIFELINE_STROKE_WIDTH)},
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
	// the order is important because the only way a child span exists is if there's a message to it
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
		if firstMessage, exists := sd.firstMessage[span]; exists {
			// needs to check Src/Dst because of self-edges or edges to/from descendants
			if span == firstMessage.Src {
				minMessageY = firstMessage.Route[0].Y
			} else {
				minMessageY = firstMessage.Route[len(firstMessage.Route)-1].Y
			}
		}
		maxMessageY := math.Inf(-1)
		if lastMessage, exists := sd.lastMessage[span]; exists {
			if span == lastMessage.Src {
				maxMessageY = lastMessage.Route[0].Y
			} else {
				maxMessageY = lastMessage.Route[len(lastMessage.Route)-1].Y
			}
		}

		// if it is the same as the child top left, add some padding
		minY := math.Min(minMessageY, minChildY)
		if minY == minChildY || minY == minMessageY {
			minY -= SPAN_MESSAGE_PAD
		}
		maxY := math.Max(maxMessageY, maxChildY)
		if maxY == maxChildY || maxY == maxMessageY {
			maxY += SPAN_MESSAGE_PAD
		}

		height := math.Max(maxY-minY, MIN_SPAN_HEIGHT)
		// -2 because the actors count as level 1 making the first level span getting 2*SPAN_DEPTH_GROW_FACTOR
		width := SPAN_BASE_WIDTH + (float64(span.Level()-2) * SPAN_DEPTH_GROWTH_FACTOR)
		x := rankToX[sd.objectRank[span]] - (width / 2.)
		span.Box = geo.NewBox(geo.NewPoint(x, minY), width, height)
		span.ZIndex = 1
	}
}

// routeMessages routes horizontal edges (messages) from Src to Dst lifeline (actor/span center)
// in another step, routes are adjusted to spans borders when necessary
func (sd *sequenceDiagram) routeMessages() error {
	startY := sd.maxActorHeight + sd.messageYStep
	for _, message := range sd.messages {
		message.ZIndex = 2
		var startX, endX float64
		if startCenter := getCenter(message.Src); startCenter != nil {
			startX = startCenter.X
		} else {
			return fmt.Errorf("could not find center of %s", message.Src.AbsID())
		}
		if endCenter := getCenter(message.Dst); endCenter != nil {
			endX = endCenter.X
		} else {
			return fmt.Errorf("could not find center of %s", message.Dst.AbsID())
		}
		isLeftToRight := startX < endX
		isToDescendant := strings.HasPrefix(message.Dst.AbsID(), message.Src.AbsID())
		isFromDescendant := strings.HasPrefix(message.Src.AbsID(), message.Dst.AbsID())
		isSelfMessage := message.Src == message.Dst

		if isSelfMessage || isToDescendant || isFromDescendant {
			midX := startX + MIN_MESSAGE_DISTANCE
			endY := startY + MIN_MESSAGE_DISTANCE
			message.Route = []*geo.Point{
				geo.NewPoint(startX, startY),
				geo.NewPoint(midX, startY),
				geo.NewPoint(midX, endY),
				geo.NewPoint(endX, endY),
			}
		} else {
			message.Route = []*geo.Point{
				geo.NewPoint(startX, startY),
				geo.NewPoint(endX, startY),
			}
		}
		startY += sd.messageYStep

		if message.Attributes.Label.Value != "" {
			if isSelfMessage || isFromDescendant || isToDescendant {
				message.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
			} else if isLeftToRight {
				message.LabelPosition = go2.Pointer(string(label.OutsideTopCenter))
			} else {
				// the label will be placed above the message because the orientation is based on the edge normal vector
				message.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
			}
		}
	}
	return nil
}

func getCenter(obj *d2graph.Object) *geo.Point {
	if obj == nil {
		return nil
	} else if obj.TopLeft != nil {
		return obj.Center()
	}
	return getCenter(obj.Parent)
}

// adjustRouteEndpoints adjust the first and last points of message routes when they are spans
// routeMessages() will route to the actor lifelife as a reference point and this function
// adjust to span width when necessary
func (sd *sequenceDiagram) adjustRouteEndpoints() {
	for _, message := range sd.messages {
		route := message.Route
		if !sd.isActor(message.Src) {
			if sd.objectRank[message.Src] <= sd.objectRank[message.Dst] {
				route[0].X += message.Src.Width / 2.
			} else {
				route[0].X -= message.Src.Width / 2.
			}
		}
		if !sd.isActor(message.Dst) {
			if sd.objectRank[message.Src] < sd.objectRank[message.Dst] {
				route[len(route)-1].X -= message.Dst.Width / 2.
			} else {
				route[len(route)-1].X += message.Dst.Width / 2.
			}
		}
	}
}

func (sd *sequenceDiagram) isActor(obj *d2graph.Object) bool {
	return obj.Parent == sd.root
}

func (sd *sequenceDiagram) getWidth() float64 {
	// the layout is always placed starting at 0, so the width is just the last actor
	lastActor := sd.actors[len(sd.actors)-1]
	return lastActor.TopLeft.X + lastActor.Width
}

func (sd *sequenceDiagram) getHeight() float64 {
	return sd.lifelines[0].Route[1].Y
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
