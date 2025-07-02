package d2sequence

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
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

	// the distance from actor[i] center to actor[i+1] center
	// every neighbor actors need different distances depending on the message labels between them
	actorXStep []float64

	yStep          float64
	maxActorHeight float64

	verticalIndices map[string]int
}

func getObjEarliestLineNum(o *d2graph.Object) int {
	min := int(math.MaxInt32)
	for _, ref := range o.References {
		if ref.MapKey == nil {
			continue
		}
		if ref.Key.HasGlob() {
			continue
		}
		min = go2.IntMin(min, ref.MapKey.Range.Start.Line)
	}
	return min
}

func getEdgeEarliestLineNum(e *d2graph.Edge) int {
	min := int(math.MaxInt32)
	for _, ref := range e.References {
		if ref.MapKey == nil {
			continue
		}
		if ref.Edge.Src.HasGlob() || ref.Edge.Dst.HasGlob() {
			continue
		}
		min = go2.IntMin(min, ref.MapKey.Range.Start.Line)
	}
	return min
}

func newSequenceDiagram(objects []*d2graph.Object, messages []*d2graph.Edge) (*sequenceDiagram, error) {
	var actors []*d2graph.Object
	var groups []*d2graph.Object

	slices.SortFunc(objects, func(a, b *d2graph.Object) int {
		return cmp.Compare(getObjEarliestLineNum(a), getObjEarliestLineNum(b))
	})
	slices.SortFunc(messages, func(a, b *d2graph.Edge) int {
		return cmp.Compare(getEdgeEarliestLineNum(a), getEdgeEarliestLineNum(b))
	})

	for _, obj := range objects {
		if obj.IsSequenceDiagramGroup() {
			queue := []*d2graph.Object{obj}
			// Groups may have more nested groups
			for len(queue) > 0 {
				curr := queue[0]
				curr.LabelPosition = go2.Pointer(label.InsideTopLeft.String())
				groups = append(groups, curr)
				queue = queue[1:]
				queue = append(queue, curr.ChildrenArray...)
			}
		} else {
			actors = append(actors, obj)
		}
	}

	if len(actors) == 0 {
		return nil, errors.New("no actors declared in sequence diagram")
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
		actorXStep:      make([]float64, len(actors)-1),
		yStep:           MIN_MESSAGE_DISTANCE,
		maxActorHeight:  0.,
		verticalIndices: make(map[string]int),
	}

	for rank, actor := range actors {
		sd.root = actor.Parent
		sd.objectRank[actor] = rank

		if actor.Width < MIN_ACTOR_WIDTH {
			dslShape := strings.ToLower(actor.Shape.Value)
			switch dslShape {
			case d2target.ShapePerson, d2target.ShapeOval, d2target.ShapeSquare, d2target.ShapeCircle:
				// scale shape up to min width uniformly
				actor.Height *= MIN_ACTOR_WIDTH / actor.Width
			}
			actor.Width = MIN_ACTOR_WIDTH
		}
		sd.maxActorHeight = math.Max(sd.maxActorHeight, actor.Height)

		queue := make([]*d2graph.Object, len(actor.ChildrenArray))
		copy(queue, actor.ChildrenArray)
		maxNoteWidth := 0.
		for len(queue) > 0 {
			child := queue[0]
			queue = queue[1:]

			// spans are children of actors that have edges
			// edge groups are children of actors with no edges and children edges
			if child.IsSequenceDiagramNote() {
				sd.verticalIndices[child.AbsID()] = getObjEarliestLineNum(child)
				child.Shape = d2graph.Scalar{Value: shape.PAGE_TYPE}
				sd.notes = append(sd.notes, child)
				sd.objectRank[child] = rank
				child.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
				maxNoteWidth = math.Max(maxNoteWidth, child.Width)
			} else {
				// spans have no labels
				// TODO why not? Spans should be able to
				child.Label = d2graph.Scalar{Value: ""}
				child.Shape = d2graph.Scalar{Value: shape.SQUARE_TYPE}
				sd.spans = append(sd.spans, child)
				sd.objectRank[child] = rank
			}

			queue = append(queue, child.ChildrenArray...)
		}

		if rank != len(actors)-1 {
			actorHW := actor.Width / 2.
			nextActorHW := actors[rank+1].Width / 2.
			sd.actorXStep[rank] = math.Max(actorHW+nextActorHW+HORIZONTAL_PAD, MIN_ACTOR_DISTANCE)
			sd.actorXStep[rank] = math.Max(maxNoteWidth/2.+HORIZONTAL_PAD, sd.actorXStep[rank])
			if rank > 0 {
				sd.actorXStep[rank-1] = math.Max(maxNoteWidth/2.+HORIZONTAL_PAD, sd.actorXStep[rank-1])
			}
		}
	}

	for _, message := range sd.messages {
		sd.verticalIndices[message.AbsID()] = getEdgeEarliestLineNum(message)

		// ensures that long labels, spanning over multiple actors, don't make for large gaps between actors
		// by distributing the label length across the actors rank difference
		rankDiff := math.Abs(float64(sd.objectRank[message.Src]) - float64(sd.objectRank[message.Dst]))
		if rankDiff != 0 {
			distributedLabelWidth := float64(message.LabelDimensions.Width) / rankDiff
			for rank := go2.IntMin(sd.objectRank[message.Src], sd.objectRank[message.Dst]); rank <= go2.IntMax(sd.objectRank[message.Src], sd.objectRank[message.Dst])-1; rank++ {
				sd.actorXStep[rank] = math.Max(sd.actorXStep[rank], distributedLabelWidth+LABEL_HORIZONTAL_PAD)
			}
		} else {
			// self edge
			nextRank := sd.objectRank[message.Src]
			if nextRank < len(sd.actorXStep) {
				labelAdjust := float64(message.LabelDimensions.Width) + label.PADDING*4
				sd.actorXStep[nextRank] = math.Max(sd.actorXStep[nextRank], labelAdjust)
			}
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

	sd.yStep += VERTICAL_PAD
	sd.maxActorHeight += VERTICAL_PAD
	if sd.root.HasLabel() {
		sd.maxActorHeight += float64(sd.root.LabelDimensions.Height)
	}

	return sd, nil
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
	sort.SliceStable(sd.groups, func(i, j int) bool {
		return sd.groups[i].Level() > sd.groups[j].Level()
	})
	for _, group := range sd.groups {
		group.ZIndex = GROUP_Z_INDEX
		sd.placeGroup(group)
	}
	for _, group := range sd.groups {
		sd.adjustGroupLabel(group)
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
				labelHeight := float64(m.LabelDimensions.Height) / 2.
				edgePad := math.Max(labelHeight+GROUP_CONTAINER_PADDING, MIN_MESSAGE_DISTANCE/2.)
				minX = math.Min(minX, p.X-HORIZONTAL_PAD)
				minY = math.Min(minY, p.Y-edgePad)
				maxX = math.Max(maxX, p.X+HORIZONTAL_PAD)
				maxY = math.Max(maxY, p.Y+edgePad)
			}
		}
	}
	// Groups should horizontally encompass all notes of the actor
	for _, n := range sd.notes {
		inGroup := false
		for _, ref := range n.References {
			if ref.Key.HasGlob() {
				continue
			}
			curr := ref.ScopeObj
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
			minX = math.Min(minX, n.TopLeft.X-HORIZONTAL_PAD)
			minY = math.Min(minY, n.TopLeft.Y-MIN_MESSAGE_DISTANCE/2.)
			maxX = math.Max(maxX, n.TopLeft.X+n.Width+HORIZONTAL_PAD)
			maxY = math.Max(maxY, n.TopLeft.Y+n.Height+MIN_MESSAGE_DISTANCE/2.)
		}
	}

	for _, ch := range group.ChildrenArray {
		for _, g := range sd.groups {
			if ch == g {
				minX = math.Min(minX, ch.TopLeft.X-GROUP_CONTAINER_PADDING)
				minY = math.Min(minY, ch.TopLeft.Y-GROUP_CONTAINER_PADDING)
				maxX = math.Max(maxX, ch.TopLeft.X+ch.Width+GROUP_CONTAINER_PADDING)
				maxY = math.Max(maxY, ch.TopLeft.Y+ch.Height+GROUP_CONTAINER_PADDING)
				break
			}
		}
	}

	group.Box = geo.NewBox(
		geo.NewPoint(
			minX,
			minY,
		),
		maxX-minX,
		maxY-minY,
	)
}

func (sd *sequenceDiagram) adjustGroupLabel(group *d2graph.Object) {
	if !group.HasLabel() {
		return
	}

	heightAdd := (group.LabelDimensions.Height + EDGE_GROUP_LABEL_PADDING/2.)
	if heightAdd < GROUP_CONTAINER_PADDING {
		return
	}

	group.Height += float64(heightAdd)

	// Extend stuff within this group
	for _, g := range sd.groups {
		if g.TopLeft.Y < group.TopLeft.Y && g.TopLeft.Y+g.Height > group.TopLeft.Y {
			g.Height += float64(heightAdd)
		}
	}
	for _, s := range sd.spans {
		if s.TopLeft.Y < group.TopLeft.Y && s.TopLeft.Y+s.Height > group.TopLeft.Y {
			s.Height += float64(heightAdd)
		}
	}

	// Move stuff down
	for _, m := range sd.messages {
		if go2.Min(m.Route[0].Y, m.Route[len(m.Route)-1].Y) > group.TopLeft.Y {
			for _, p := range m.Route {
				p.Y += float64(heightAdd)
			}
		}
	}
	for _, s := range sd.spans {
		if s.TopLeft.Y > group.TopLeft.Y {
			s.TopLeft.Y += float64(heightAdd)
		}
	}
	for _, g := range sd.groups {
		if g.TopLeft.Y > group.TopLeft.Y {
			g.TopLeft.Y += float64(heightAdd)
		}
	}
	for _, n := range sd.notes {
		if n.TopLeft.Y > group.TopLeft.Y {
			n.TopLeft.Y += float64(heightAdd)
		}
	}
}

// placeActors places actors bottom aligned, side by side with centers spaced by sd.actorXStep
func (sd *sequenceDiagram) placeActors() {
	centerX := sd.actors[0].Width / 2.
	for rank, actor := range sd.actors {
		var yOffset float64
		if actor.HasOutsideBottomLabel() {
			if actor.IconPosition == nil {
				actor.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
			}
			yOffset = sd.maxActorHeight - actor.Height
			if actor.HasLabel() {
				yOffset -= float64(actor.LabelDimensions.Height)
			}
		} else {
			if actor.Icon != nil && actor.Shape.Value != d2target.ShapeImage {
				if actor.LabelPosition == nil {
					actor.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
				}
				if actor.IconPosition == nil {
					actor.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
				}
			} else {
				if actor.IconPosition == nil {
					actor.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
				}
			}
			yOffset = sd.maxActorHeight - actor.Height
		}
		halfWidth := actor.Width / 2.
		actor.TopLeft = geo.NewPoint(math.Round(centerX-halfWidth), yOffset)
		if rank != len(sd.actors)-1 {
			centerX += sd.actorXStep[rank]
		}
	}
}

// addLifelineEdges adds a new edge for each actor in the graph that represents the its lifeline
// . ┌──────────────┐
// . │     actor    │
// . └──────┬───────┘
// .        │
// .        │ lifeline
// .        │
// .        │
func (sd *sequenceDiagram) addLifelineEdges() {
	endY := 0.
	if len(sd.messages) > 0 {
		lastRoute := sd.messages[len(sd.messages)-1].Route
		for _, p := range lastRoute {
			endY = math.Max(endY, p.Y)
		}
	}
	for _, note := range sd.notes {
		endY = math.Max(endY, note.TopLeft.Y+note.Height)
	}
	for _, actor := range sd.actors {
		endY = math.Max(endY, actor.TopLeft.Y+actor.Height)
	}
	endY += sd.yStep

	for _, actor := range sd.actors {
		actorBottom := actor.Center()
		actorBottom.Y = actor.TopLeft.Y + actor.Height
		if *actor.LabelPosition == label.OutsideBottomCenter.String() && actor.HasLabel() {
			actorBottom.Y += float64(actor.LabelDimensions.Height) + LIFELINE_LABEL_PAD
		}
		actorLifelineEnd := actor.Center()
		actorLifelineEnd.Y = endY
		style := d2graph.Style{
			StrokeDash:  &d2graph.Scalar{Value: fmt.Sprintf("%d", LIFELINE_STROKE_DASH)},
			StrokeWidth: &d2graph.Scalar{Value: fmt.Sprintf("%d", LIFELINE_STROKE_WIDTH)},
		}
		if actor.Style.StrokeDash != nil {
			style.StrokeDash = &d2graph.Scalar{Value: actor.Style.StrokeDash.Value}
		}
		if actor.Style.Stroke != nil {
			style.Stroke = &d2graph.Scalar{Value: actor.Style.Stroke.Value}
		}

		sd.lifelines = append(sd.lifelines, &d2graph.Edge{
			Attributes: d2graph.Attributes{Style: style},
			Src:        actor,
			SrcArrow:   false,
			Dst: &d2graph.Object{
				ID: actor.ID + fmt.Sprintf("-lifeline-end-%d", go2.StringToIntHash(actor.ID+"-lifeline-end")),
			},
			DstArrow: false,
			Route:    []*geo.Point{actorBottom, actorLifelineEnd},
			ZIndex:   LIFELINE_Z_INDEX,
		})
	}
}

func IsLifelineEnd(obj *d2graph.Object) bool {
	// lifeline ends only have ID and no graph parent or box set
	if obj.Graph != nil || obj.Parent != nil || obj.Box != nil {
		return false
	}
	if !strings.Contains(obj.ID, "-lifeline-end-") {
		return false
	}
	parts := strings.Split(obj.ID, "-lifeline-end-")
	if len(parts) > 1 {
		hash := parts[len(parts)-1]
		actorID := strings.Join(parts[:len(parts)-1], "-lifeline-end-")
		if strconv.Itoa(go2.StringToIntHash(actorID+"-lifeline-end")) == hash {
			return true
		}
	}
	return false
}

func (sd *sequenceDiagram) placeNotes() {
	rankToX := make(map[int]float64)
	for _, actor := range sd.actors {
		rankToX[sd.objectRank[actor]] = actor.Center().X
	}

	for _, note := range sd.notes {
		verticalIndex := sd.verticalIndices[note.AbsID()]
		y := sd.maxActorHeight + sd.yStep

		for _, msg := range sd.messages {
			if sd.verticalIndices[msg.AbsID()] < verticalIndex {
				if msg.Src == msg.Dst {
					// For self-messages, account for the full vertical space they occupy
					y += sd.yStep + math.Max(float64(msg.LabelDimensions.Height), MIN_MESSAGE_DISTANCE)*1.5
				} else {
					y += sd.yStep + float64(msg.LabelDimensions.Height)
				}
			}
		}
		for _, otherNote := range sd.notes {
			if sd.verticalIndices[otherNote.AbsID()] < verticalIndex {
				y += otherNote.Height + sd.yStep
			}
		}

		x := rankToX[sd.objectRank[note]] - (note.Width / 2.)
		note.Box.TopLeft = geo.NewPoint(x, y)
		note.ZIndex = NOTE_Z_INDEX
	}
}

// placeSpans places spans over the object lifeline
// . ┌──────────┐
// . │  actor   │
// . └────┬─────┘
// .    ┌─┴──┐
// .    │    │
// .    |span|
// .    │    │
// .    └─┬──┘
// .      │
// .   lifeline
// .      │
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
			if firstMessage.Src == firstMessage.Dst || span == firstMessage.Src {
				minMessageY = firstMessage.Route[0].Y
			} else {
				minMessageY = firstMessage.Route[len(firstMessage.Route)-1].Y
			}
		}
		maxMessageY := math.Inf(-1)
		if lastMessage, exists := sd.lastMessage[span]; exists {
			if lastMessage.Src == lastMessage.Dst || span == lastMessage.Dst {
				maxMessageY = lastMessage.Route[len(lastMessage.Route)-1].Y
			} else {
				maxMessageY = lastMessage.Route[0].Y
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
		// -1 because the actors count as 1 level
		width := SPAN_BASE_WIDTH + (float64(span.Level()-sd.root.Level()-2) * SPAN_DEPTH_GROWTH_FACTOR)
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

		var startX, endX float64
		if startCenter := getCenter(message.Src); startCenter != nil {
			startX = startCenter.X
		} else {
			return fmt.Errorf("could not find center of %s. Is it declared as an actor?", message.Src.ID)
		}
		if endCenter := getCenter(message.Dst); endCenter != nil {
			endX = endCenter.X
		} else {
			return fmt.Errorf("could not find center of %s. Is it declared as an actor?", message.Dst.ID)
		}
		isToDescendant := strings.HasPrefix(message.Dst.AbsID(), message.Src.AbsID()+".")
		isFromDescendant := strings.HasPrefix(message.Src.AbsID(), message.Dst.AbsID()+".")
		isSelfMessage := message.Src == message.Dst

		currSrc := message.Src
		for !currSrc.Parent.IsSequenceDiagram() {
			currSrc = currSrc.Parent
		}
		currDst := message.Dst
		for !currDst.Parent.IsSequenceDiagram() {
			currDst = currDst.Parent
		}
		isToSibling := currSrc == currDst

		if isSelfMessage || isToDescendant || isFromDescendant || isToSibling {
			midX := startX + math.Max(SELF_MESSAGE_HORIZONTAL_TRAVEL, float64(message.LabelDimensions.Width)/2.+label.PADDING*2)
			startY := messageOffset + noteOffset
			endY := startY + math.Max(float64(message.LabelDimensions.Height), MIN_MESSAGE_DISTANCE)*1.5
			message.Route = []*geo.Point{
				geo.NewPoint(startX, startY),
				geo.NewPoint(midX, startY),
				geo.NewPoint(midX, endY),
				geo.NewPoint(endX, endY),
			}
			messageOffset = endY + sd.yStep - noteOffset
		} else {
			startY := messageOffset + noteOffset + float64(message.LabelDimensions.Height/2.)
			message.Route = []*geo.Point{
				geo.NewPoint(startX, startY),
				geo.NewPoint(endX, startY),
			}
			messageOffset = startY + float64(message.LabelDimensions.Height/2.) + sd.yStep - noteOffset
		}

		if message.Label.Value != "" {
			message.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}
	return nil
}

func getCenter(obj *d2graph.Object) *geo.Point {
	if obj == nil {
		return nil
	} else if obj.Box != nil && obj.Box.TopLeft != nil {
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
	rightmost := lastActor.TopLeft.X + lastActor.Width

	for _, m := range sd.messages {
		for _, p := range m.Route {
			rightmost = math.Max(rightmost, p.X)
		}
		// Self referential messages may have labels that extend further
		if m.Src == m.Dst {
			rightmost = math.Max(rightmost, m.Route[1].X+float64(m.LabelDimensions.Width)/2.)
		}
	}

	return rightmost
}

func (sd *sequenceDiagram) getHeight() float64 {
	return sd.lifelines[0].Route[1].Y
}

func (sd *sequenceDiagram) shift(tl *geo.Point) {
	allObjects := append([]*d2graph.Object{}, sd.actors...)
	allObjects = append(allObjects, sd.spans...)
	allObjects = append(allObjects, sd.groups...)
	allObjects = append(allObjects, sd.notes...)
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
