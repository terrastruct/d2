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
	groups    []*d2graph.Object
	spans     []*d2graph.Object
	notes     []*d2graph.Object

	// can be either actors or spans
	// rank: left to right position of actors/spans (spans have the same rank as their parents)
	objectRank map[*d2graph.Object]int

	// keep track of the first and last message of a given actor/span
	firstMessage map[*d2graph.Object]*d2graph.Edge
	lastMessage  map[*d2graph.Object]*d2graph.Edge

	yStep          float64
	maxActorHeight float64

	verticalIndices map[string]int
}

func getObjEarliestLineNum(o *d2graph.Object) int {
	min := int(math.MaxInt64)
	for _, ref := range o.References {
		if ref.MapKey == nil {
			continue
		}
		min = go2.IntMin(min, ref.MapKey.Range.Start.Line)
	}
	return min
}

func getEdgeEarliestLineNum(e *d2graph.Edge) int {
	min := int(math.MaxInt64)
	for _, ref := range e.References {
		if ref.MapKey == nil {
			continue
		}
		min = go2.IntMin(min, ref.MapKey.Range.Start.Line)
	}
	return min
}

func newSequenceDiagram(objects []*d2graph.Object, messages []*d2graph.Edge) *sequenceDiagram {
	var actors []*d2graph.Object
	var groups []*d2graph.Object

	for _, obj := range objects {
		if obj.IsSequenceDiagramGroup() {
			queue := []*d2graph.Object{obj}
			// Groups may have more nested groups
			for len(queue) > 0 {
				curr := queue[0]
				groups = append(groups, curr)
				queue = queue[1:]
				for _, c := range curr.ChildrenArray {
					queue = append(queue, c)
				}
			}
		} else {
			actors = append(actors, obj)
		}
	}

	sd := &sequenceDiagram{
		messages:        messages,
		actors:          actors,
		groups:          groups,
		spans:           nil,
		notes:           nil,
		lifelines:       nil,
		objectRank:      make(map[*d2graph.Object]int),
		firstMessage:    make(map[*d2graph.Object]*d2graph.Edge),
		lastMessage:     make(map[*d2graph.Object]*d2graph.Edge),
		yStep:           MIN_MESSAGE_DISTANCE,
		maxActorHeight:  0.,
		verticalIndices: make(map[string]int),
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
			child := queue[0]
			queue = queue[1:]

			// spans are children of actors that have edges
			// edge groups are children of actors with no edges and children edges
			if child.IsSequenceDiagramNote() {
				sd.verticalIndices[child.AbsID()] = getObjEarliestLineNum(child)
				// TODO change to page type when it doesn't look deformed
				child.Attributes.Shape = d2graph.Scalar{Value: shape.SQUARE_TYPE}
				sd.notes = append(sd.notes, child)
				sd.objectRank[child] = rank
				child.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
			} else {
				// spans have no labels
				// TODO why not? Spans should be able to
				child.Attributes.Label = d2graph.Scalar{Value: ""}
				child.Attributes.Shape = d2graph.Scalar{Value: shape.SQUARE_TYPE}
				sd.spans = append(sd.spans, child)
				sd.objectRank[child] = rank
			}

			queue = append(queue, child.ChildrenArray...)
		}
	}

	for _, message := range sd.messages {
		sd.verticalIndices[message.AbsID()] = getEdgeEarliestLineNum(message)
		sd.yStep = math.Max(sd.yStep, float64(message.LabelDimensions.Height))

		sd.lastMessage[message.Src] = message
		if _, exists := sd.firstMessage[message.Src]; !exists {
			sd.firstMessage[message.Src] = message
		}
		sd.lastMessage[message.Dst] = message
		if _, exists := sd.firstMessage[message.Dst]; !exists {
			sd.firstMessage[message.Dst] = message
		}

	}

	sd.yStep += VERTICAL_PAD
	sd.maxActorHeight += VERTICAL_PAD
	if sd.root.LabelHeight != nil {
		sd.maxActorHeight += float64(*sd.root.LabelHeight)
	}

	return sd
}

func (sd *sequenceDiagram) layout() error {
	sd.placeActors()
	sd.placeNotes()
	if err := sd.routeMessages(); err != nil {
		return err
	}
	sd.placeSpans()
	sd.adjustRouteEndpoints()
	sd.placeGroups()
	sd.addLifelineEdges()
	return nil
}

func (sd *sequenceDiagram) placeGroups() {
	for _, group := range sd.groups {
		group.ZIndex = GROUP_Z_INDEX
		sd.placeGroup(group)
	}
}

func (sd *sequenceDiagram) placeGroup(group *d2graph.Object) {
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)

	for _, m := range sd.messages {
		if m.ContainedBy(group) {
			for _, p := range m.Route {
				minX = math.Min(minX, p.X)
				minY = math.Min(minY, p.Y)
				maxX = math.Max(maxX, p.X)
				maxY = math.Max(maxY, p.Y)
			}
		}
	}
	// Groups should horizontally encompass all notes of the actor
	for _, n := range sd.notes {
		inGroup := false
		for _, ref := range n.References {
			curr := ref.UnresolvedScopeObj
			for curr != nil {
				if curr == group {
					inGroup = true
					break
				}
				curr = curr.Parent
			}
			if inGroup {
				break
			}
		}
		if inGroup {
			minY = math.Min(minY, n.TopLeft.Y)
			maxY = math.Max(maxY, n.TopLeft.Y+n.Height)
			minX = math.Min(minX, n.TopLeft.X)
			maxX = math.Max(maxX, n.TopLeft.X+n.Width)
		}
	}

	group.Box = geo.NewBox(
		geo.NewPoint(
			minX-HORIZONTAL_PAD,
			minY-(MIN_MESSAGE_DISTANCE/2.),
		),
		maxX-minX+HORIZONTAL_PAD*2,
		maxY-minY+MIN_MESSAGE_DISTANCE,
	)
}

// placeActors places actors bottom aligned, side by side
func (sd *sequenceDiagram) placeActors() {
	x := 0.
	for _, actor := range sd.actors {
		shape := actor.Attributes.Shape.Value
		var yOffset float64
		if shape == d2target.ShapeImage || shape == d2target.ShapePerson {
			actor.LabelPosition = go2.Pointer(string(label.OutsideBottomCenter))
			yOffset = sd.maxActorHeight - actor.Height
			if actor.LabelHeight != nil {
				yOffset -= float64(*actor.LabelHeight)
			}
		} else {
			actor.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
			yOffset = sd.maxActorHeight - actor.Height
		}
		actor.TopLeft = geo.NewPoint(x, yOffset)
		x += actor.Width + MIN_ACTOR_DISTANCE
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
	endY := 0.
	for _, p := range lastRoute {
		endY = math.Max(endY, p.Y)
	}
	for _, note := range sd.notes {
		endY = math.Max(endY, note.TopLeft.Y+note.Height)
	}
	endY += sd.yStep

	for _, actor := range sd.actors {
		actorBottom := actor.Center()
		actorBottom.Y = actor.TopLeft.Y + actor.Height
		if *actor.LabelPosition == string(label.OutsideBottomCenter) && actor.LabelHeight != nil {
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
			ZIndex:   LIFELINE_Z_INDEX,
		})
	}
}

func (sd *sequenceDiagram) placeNotes() {
	rankToX := make(map[int]float64)
	for _, actor := range sd.actors {
		rankToX[sd.objectRank[actor]] = actor.Center().X
	}

	for i, note := range sd.notes {
		verticalIndex := sd.verticalIndices[note.AbsID()]
		y := sd.maxActorHeight + sd.yStep

		for _, msg := range sd.messages {
			if sd.verticalIndices[msg.AbsID()] < verticalIndex {
				y += sd.yStep
			}
		}
		for _, otherNote := range sd.notes[:i] {
			y += otherNote.Height + sd.yStep
		}

		x := rankToX[sd.objectRank[note]] - (note.Width / 2.)
		note.Box.TopLeft = geo.NewPoint(x, y)
		note.ZIndex = NOTE_Z_INDEX
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
		span.ZIndex = SPAN_Z_INDEX
	}
}

// routeMessages routes horizontal edges (messages) from Src to Dst lifeline (actor/span center)
// in another step, routes are adjusted to spans borders when necessary
func (sd *sequenceDiagram) routeMessages() error {
	messageOffset := sd.maxActorHeight + sd.yStep
	for _, message := range sd.messages {
		message.ZIndex = MESSAGE_Z_INDEX
		noteOffset := 0.
		for _, note := range sd.notes {
			if sd.verticalIndices[note.AbsID()] < sd.verticalIndices[message.AbsID()] {
				noteOffset += note.Height + sd.yStep
			}
		}
		startY := messageOffset + noteOffset

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
		messageOffset += sd.yStep

		if message.Attributes.Label.Value != "" {
			message.LabelPosition = go2.Pointer(string(label.InsideMiddleCenter))
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
