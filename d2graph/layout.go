package d2graph

import (
	"strings"

	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/d2/lib/shape"
)

const MIN_SEGMENT_LEN = 10

func (obj *Object) MoveWithDescendants(dx, dy float64) {
	obj.TopLeft.X += dx
	obj.TopLeft.Y += dy
	for _, child := range obj.ChildrenArray {
		child.MoveWithDescendants(dx, dy)
	}
}

func (obj *Object) MoveWithDescendantsTo(x, y float64) {
	dx := x - obj.TopLeft.X
	dy := y - obj.TopLeft.Y
	obj.MoveWithDescendants(dx, dy)
}

func (parent *Object) removeChild(child *Object) {
	delete(parent.Children, strings.ToLower(child.ID))
	for i := 0; i < len(parent.ChildrenArray); i++ {
		if parent.ChildrenArray[i] == child {
			parent.ChildrenArray = append(parent.ChildrenArray[:i], parent.ChildrenArray[i+1:]...)
			break
		}
	}
}

// remove obj and all descendants from graph, as a new Graph
func (g *Graph) ExtractAsNestedGraph(obj *Object) *Graph {
	descendantObjects, edges := pluckObjAndEdges(g, obj)

	tempGraph := NewGraph()
	tempGraph.Root.ChildrenArray = []*Object{obj}
	tempGraph.Root.Children[strings.ToLower(obj.ID)] = obj

	for _, descendantObj := range descendantObjects {
		descendantObj.Graph = tempGraph
	}
	tempGraph.Objects = descendantObjects
	tempGraph.Edges = edges

	obj.Parent.removeChild(obj)
	obj.Parent = tempGraph.Root

	return tempGraph
}

func pluckObjAndEdges(g *Graph, obj *Object) (descendantsObjects []*Object, edges []*Edge) {
	for i := 0; i < len(g.Edges); i++ {
		edge := g.Edges[i]
		if edge.Src == obj || edge.Dst == obj {
			edges = append(edges, edge)
			g.Edges = append(g.Edges[:i], g.Edges[i+1:]...)
			i--
		}
	}

	for i := 0; i < len(g.Objects); i++ {
		temp := g.Objects[i]
		if temp.AbsID() == obj.AbsID() {
			descendantsObjects = append(descendantsObjects, obj)
			g.Objects = append(g.Objects[:i], g.Objects[i+1:]...)
			for _, child := range obj.ChildrenArray {
				subObjects, subEdges := pluckObjAndEdges(g, child)
				descendantsObjects = append(descendantsObjects, subObjects...)
				edges = append(edges, subEdges...)
			}
			break
		}
	}

	return descendantsObjects, edges
}

func (g *Graph) InjectNestedGraph(tempGraph *Graph, parent *Object) {
	obj := tempGraph.Root.ChildrenArray[0]
	obj.MoveWithDescendantsTo(0, 0)
	obj.Parent = parent
	for _, obj := range tempGraph.Objects {
		obj.Graph = g
	}
	g.Objects = append(g.Objects, tempGraph.Objects...)
	parent.Children[strings.ToLower(obj.ID)] = obj
	parent.ChildrenArray = append(parent.ChildrenArray, obj)
	g.Edges = append(g.Edges, tempGraph.Edges...)
}

// ShiftDescendants moves Object's descendants (not including itself)
// descendants' edges are also moved by the same dx and dy (the whole route is moved if both ends are a descendant)
func (obj *Object) ShiftDescendants(dx, dy float64) {
	// also need to shift edges of descendants that are shifted
	movedEdges := make(map[*Edge]struct{})
	for _, e := range obj.Graph.Edges {
		isSrcDesc := e.Src.IsDescendantOf(obj)
		isDstDesc := e.Dst.IsDescendantOf(obj)

		if isSrcDesc && isDstDesc {
			movedEdges[e] = struct{}{}
			for _, p := range e.Route {
				p.X += dx
				p.Y += dy
			}
		}
	}

	obj.IterDescendants(func(_, curr *Object) {
		curr.TopLeft.X += dx
		curr.TopLeft.Y += dy
		for _, e := range obj.Graph.Edges {
			if _, ok := movedEdges[e]; ok {
				continue
			}
			isSrc := e.Src == curr
			isDst := e.Dst == curr

			if isSrc && isDst {
				for _, p := range e.Route {
					p.X += dx
					p.Y += dy
				}
			} else if isSrc {
				if dx == 0 {
					e.ShiftStart(dy, false)
				} else if dy == 0 {
					e.ShiftStart(dx, true)
				} else {
					e.Route[0].X += dx
					e.Route[0].Y += dy
				}
			} else if isDst {
				if dx == 0 {
					e.ShiftEnd(dy, false)
				} else if dy == 0 {
					e.ShiftEnd(dx, true)
				} else {
					e.Route[len(e.Route)-1].X += dx
					e.Route[len(e.Route)-1].Y += dy
				}
			}

			if isSrc || isDst {
				movedEdges[e] = struct{}{}
			}
		}
	})
}

// ShiftStart moves the starting point of the route by delta either horizontally or vertically
// if subsequent points are in line with the movement, they will be removed (unless it is the last point)
// start                   end
// . ├────┼────┼───┼────┼───┤   before
// . ├──dx──►
// .        ├──┼───┼────┼───┤   after
func (edge *Edge) ShiftStart(delta float64, isHorizontal bool) {
	position := func(p *geo.Point) float64 {
		if isHorizontal {
			return p.X
		}
		return p.Y
	}

	start := edge.Route[0]
	next := edge.Route[1]
	isIncreasing := position(start) < position(next)
	if isHorizontal {
		start.X += delta
	} else {
		start.Y += delta
	}

	if isIncreasing == (delta < 0) {
		// nothing more to do when moving away from the next point
		return
	}

	isAligned := func(p *geo.Point) bool {
		if isHorizontal {
			return p.Y == start.Y
		}
		return p.X == start.X
	}
	isPastStart := func(p *geo.Point) bool {
		if delta > 0 {
			return position(p) < position(start)
		} else {
			return position(p) > position(start)
		}
	}

	needsRemoval := false
	toRemove := make([]bool, len(edge.Route))
	for i := 1; i < len(edge.Route)-1; i++ {
		if !isAligned(edge.Route[i]) {
			break
		}
		if isPastStart(edge.Route[i]) {
			toRemove[i] = true
			needsRemoval = true
		}
	}
	if needsRemoval {
		edge.Route = geo.RemovePoints(edge.Route, toRemove)
	}
}

// ShiftEnd moves the ending point of the route by delta either horizontally or vertically
// if prior points are in line with the movement, they will be removed (unless it is the first point)
func (edge *Edge) ShiftEnd(delta float64, isHorizontal bool) {
	position := func(p *geo.Point) float64 {
		if isHorizontal {
			return p.X
		}
		return p.Y
	}

	end := edge.Route[len(edge.Route)-1]
	prev := edge.Route[len(edge.Route)-2]
	isIncreasing := position(prev) < position(end)
	if isHorizontal {
		end.X += delta
	} else {
		end.Y += delta
	}

	if isIncreasing == (delta > 0) {
		// nothing more to do when moving away from the next point
		return
	}

	isAligned := func(p *geo.Point) bool {
		if isHorizontal {
			return p.Y == end.Y
		}
		return p.X == end.X
	}
	isPastEnd := func(p *geo.Point) bool {
		if delta > 0 {
			return position(p) < position(end)
		} else {
			return position(p) > position(end)
		}
	}

	needsRemoval := false
	toRemove := make([]bool, len(edge.Route))
	for i := len(edge.Route) - 2; i > 0; i-- {
		if !isAligned(edge.Route[i]) {
			break
		}
		if isPastEnd(edge.Route[i]) {
			toRemove[i] = true
			needsRemoval = true
		}
	}
	if needsRemoval {
		edge.Route = geo.RemovePoints(edge.Route, toRemove)
	}
}

// GetModifierElementAdjustments returns width/height adjustments to account for shapes with 3d or multiple
func (obj *Object) GetModifierElementAdjustments() (dx, dy float64) {
	if obj.Is3D() {
		if obj.Shape.Value == d2target.ShapeHexagon {
			dy = d2target.THREE_DEE_OFFSET / 2
		} else {
			dy = d2target.THREE_DEE_OFFSET
		}
		dx = d2target.THREE_DEE_OFFSET
	} else if obj.IsMultiple() {
		dy = d2target.MULTIPLE_OFFSET
		dx = d2target.MULTIPLE_OFFSET
	}
	return dx, dy
}

func (obj *Object) ToShape() shape.Shape {
	tl := obj.TopLeft
	if tl == nil {
		tl = geo.NewPoint(0, 0)
	}
	dslShape := strings.ToLower(obj.Shape.Value)
	shapeType := d2target.DSL_SHAPE_TO_SHAPE_TYPE[dslShape]
	contentBox := geo.NewBox(tl, obj.Width, obj.Height)
	return shape.NewShape(shapeType, contentBox)
}

func (obj *Object) GetLabelTopLeft() *geo.Point {
	if obj.LabelPosition == nil {
		return nil
	}

	s := obj.ToShape()
	labelPosition := label.Position(*obj.LabelPosition)

	var box *geo.Box
	if labelPosition.IsOutside() {
		box = s.GetBox()
	} else {
		box = s.GetInnerBox()
	}

	labelTL := labelPosition.GetPointOnBox(box, label.PADDING,
		float64(obj.LabelDimensions.Width),
		float64(obj.LabelDimensions.Height),
	)
	return labelTL
}

func (edge *Edge) TraceToShape(points []*geo.Point, startIndex, endIndex int) (newStart, newEnd int) {
	srcShape := edge.Src.ToShape()
	dstShape := edge.Dst.ToShape()

	startingSegment := geo.Segment{Start: points[startIndex+1], End: points[startIndex]}
	// if an edge runs into an outside label, stop the edge at the label instead
	overlapsOutsideLabel := false
	if edge.Src.HasLabel() {
		// assumes LabelPosition, LabelWidth, LabelHeight are all set if there is a label
		labelPosition := label.Position(*edge.Src.LabelPosition)
		if labelPosition.IsOutside() {
			labelWidth := float64(edge.Src.LabelDimensions.Width)
			labelHeight := float64(edge.Src.LabelDimensions.Height)
			labelTL := labelPosition.GetPointOnBox(edge.Src.Box, label.PADDING, labelWidth, labelHeight)

			labelBox := geo.NewBox(labelTL, labelWidth, labelHeight)
			// add left/right padding to box
			labelBox.TopLeft.X -= label.PADDING
			labelBox.Width += 2 * label.PADDING
			if intersections := labelBox.Intersections(startingSegment); len(intersections) > 0 {
				overlapsOutsideLabel = true
				// move starting segment to label intersection point
				points[startIndex] = intersections[0]
				startingSegment.End = intersections[0]
				// if the segment becomes too short, just merge it with the next segment
				if startIndex+1 < len(points)-1 && startingSegment.Length() < MIN_SEGMENT_LEN {
					points[startIndex+1] = points[startIndex]
					startIndex++
				}
			}
		}
	}
	if !overlapsOutsideLabel {
		if intersections := edge.Src.Intersections(startingSegment); len(intersections) > 0 {
			// move starting segment to intersection point
			points[startIndex] = intersections[0]
			startingSegment.End = intersections[0]
			// if the segment becomes too short, just merge it with the next segment
			if startIndex+1 < len(points)-1 && startingSegment.Length() < MIN_SEGMENT_LEN {
				points[startIndex+1] = points[startIndex]
				startIndex++
			}
		}
		// trace the edge to the specific shape's border
		points[startIndex] = shape.TraceToShapeBorder(srcShape, points[startIndex], points[startIndex+1])
	}
	endingSegment := geo.Segment{Start: points[endIndex-1], End: points[endIndex]}
	overlapsOutsideLabel = false
	if edge.Dst.HasLabel() {
		// assumes LabelPosition, LabelWidth, LabelHeight are all set if there is a label
		labelPosition := label.Position(*edge.Dst.LabelPosition)
		if labelPosition.IsOutside() {
			labelWidth := float64(edge.Dst.LabelDimensions.Width)
			labelHeight := float64(edge.Dst.LabelDimensions.Height)
			labelTL := labelPosition.GetPointOnBox(edge.Dst.Box, label.PADDING, labelWidth, labelHeight)

			labelBox := geo.NewBox(labelTL, labelWidth, labelHeight)
			// add left/right padding to box
			labelBox.TopLeft.X -= label.PADDING
			labelBox.Width += 2 * label.PADDING
			if intersections := labelBox.Intersections(endingSegment); len(intersections) > 0 {
				overlapsOutsideLabel = true
				// move ending segment to label intersection point
				points[endIndex] = intersections[0]
				endingSegment.End = intersections[0]
				// if the segment becomes too short, just merge it with the previous segment
				if endIndex-1 > 0 && endingSegment.Length() < MIN_SEGMENT_LEN {
					points[endIndex-1] = points[endIndex]
					endIndex--
				}
			}
		}
	}
	if !overlapsOutsideLabel {
		if intersections := edge.Dst.Intersections(endingSegment); len(intersections) > 0 {
			// move ending segment to intersection point
			points[endIndex] = intersections[0]
			endingSegment.End = intersections[0]
			// if the segment becomes too short, just merge it with the previous segment
			if endIndex-1 > 0 && endingSegment.Length() < MIN_SEGMENT_LEN {
				points[endIndex-1] = points[endIndex]
				endIndex--
			}
		}
		points[endIndex] = shape.TraceToShapeBorder(dstShape, points[endIndex], points[endIndex-1])
	}
	return startIndex, endIndex
}
