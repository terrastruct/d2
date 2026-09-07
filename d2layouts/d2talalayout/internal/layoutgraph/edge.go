package layoutgraph

import (
	"fmt"
	"math"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labelgeom"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

const edgeStrokeWidth = 3.0

type Edge struct {
	D2ID   *string
	ID     EntityID
	From   *Node
	To     *Node
	Points []*geo.Point

	SourceArrowhead      Arrowhead
	TargetArrowhead      Arrowhead
	SourceArrowheadLabel *Label
	TargetArrowheadLabel *Label

	MinWidth        int
	MinHeight       int
	Label           *Label
	LabelPercentage float64
	IsCurve         bool

	// edges between SQL Table shapes
	FromTableColumnIndex *int
	ToTableColumnIndex   *int

	IsInvisible bool
	Style       EdgeStyle

	// hierarchyRankWeight is transient metadata used only by the temporary
	// simple DAG passed to hierarchy ranking. Ordinary layout edges keep the
	// zero-value unset state and therefore use unit weight.
	hierarchyRankWeight    int
	hierarchyRankWeightSet bool
}

func (e1 *Edge) EquivalentStyles(e2 *Edge) bool {
	return e1.stylesMatchOneWay(e2) && e2.stylesMatchOneWay(e1)
}

func (e1 *Edge) stylesMatchOneWay(e2 *Edge) bool {
	if e1.Style.Opacity != nil {
		if !(e2.Style.Opacity != nil && e2.Style.Opacity.Value == e1.Style.Opacity.Value) {
			return false
		}
	}
	if e1.Style.Stroke != nil {
		if !(e2.Style.Stroke != nil && e2.Style.Stroke.Value == e1.Style.Stroke.Value) {
			return false
		}
	}
	if e1.Style.StrokeWidth != nil {
		if !(e2.Style.StrokeWidth != nil && e2.Style.StrokeWidth.Value == e1.Style.StrokeWidth.Value) {
			return false
		}
	}
	if e1.Style.StrokeDash != nil {
		if !(e2.Style.StrokeDash != nil && e2.Style.StrokeDash.Value == e1.Style.StrokeDash.Value) {
			return false
		}
	}
	if e1.Style.Animated != nil {
		if !(e2.Style.Animated != nil && e2.Style.Animated.Value == e1.Style.Animated.Value) {
			return false
		}
	}

	return true
}

func NewEdge(from, to *Node) *Edge {
	return &Edge{
		From:   from,
		To:     to,
		Points: []*geo.Point{},
	}
}

func (e *Edge) isTreeEdge() bool {
	if _, is := e.From.Graph.NodeToTree[e.From]; is {
		return true
	}
	if _, is := e.To.Graph.NodeToTree[e.To]; is {
		return true
	}
	return false
}

func (e *Edge) SourcePort() *geo.Point {
	if len(e.Points) == 0 {
		return nil
	}
	return e.Points[0]
}

func (e *Edge) TargetPort() *geo.Point {
	if len(e.Points) == 0 {
		return nil
	}
	return e.Points[len(e.Points)-1]
}

func (edge *Edge) entityID() EntityID {
	if edge == nil {
		return 0
	}
	return edge.ID
}

func (edge *Edge) isLoop() bool {
	return edge.From == edge.To
}

func (edge *Edge) HasSourceArrow() bool {
	return edge.SourceArrowhead != "" &&
		edge.SourceArrowhead != NoArrowhead
}

func (edge *Edge) HasTargetArrow() bool {
	return edge.TargetArrowhead != "" &&
		edge.TargetArrowhead != NoArrowhead
}

func (edge *Edge) isDirected() bool {
	return edge.HasSourceArrow() != edge.HasTargetArrow()
}

func (edge *Edge) isBidirectional() bool {
	return edge.HasSourceArrow() && edge.HasTargetArrow()
}

func (edge *Edge) isUndirected() bool {
	return !edge.HasSourceArrow() && !edge.HasTargetArrow()
}

// directedEndpoints returns the semantic source and target of a directed edge.
// Edge.From and Edge.To preserve declaration order, so a source-arrow-only edge
// points from To to From.
func (edge *Edge) directedEndpoints() (from, to *Node, ok bool) {
	if edge == nil || !edge.isDirected() {
		return nil, nil, false
	}
	if edge.HasSourceArrow() {
		return edge.To, edge.From, true
	}
	return edge.From, edge.To, true
}

func (edge *Edge) ownArrowheadsMatch() bool {
	return edge.SourceArrowhead == edge.TargetArrowhead
}

func (edge *Edge) hasMatchingArrowheads(otherEdge *Edge) bool {
	if edge.SourceArrowhead == otherEdge.SourceArrowhead && edge.TargetArrowhead == otherEdge.TargetArrowhead {
		return true
	}
	if edge.SourceArrowhead == otherEdge.TargetArrowhead && edge.TargetArrowhead == otherEdge.SourceArrowhead {
		return true
	}
	return false
}

func (edge *Edge) hasArrowTo(node *Node) bool {
	return (node == edge.From && edge.HasSourceArrow()) || (node == edge.To && edge.HasTargetArrow())
}

func (edge *Edge) arrowheadTo(node *Node) Arrowhead {
	if node == edge.From {
		return edge.SourceArrowhead
	} else {
		return edge.TargetArrowhead
	}
}

func (e *Edge) DebugID() string {
	arrow := "-"
	if e.HasSourceArrow() {
		arrow = "<" + arrow
	}
	if e.HasTargetArrow() {
		arrow += ">"
	}
	if !e.HasSourceArrow() && !e.HasTargetArrow() {
		arrow += "-"
	}
	from := e.From.DebugID()
	if e.FromTableColumnIndex != nil {
		from = fmt.Sprintf("%s[%d]", from, *e.FromTableColumnIndex)
	}
	to := e.To.DebugID()
	if e.ToTableColumnIndex != nil {
		to = fmt.Sprintf("%s[%d]", to, *e.ToTableColumnIndex)
	}
	label := ""
	if e.Label != nil {
		label = ": " + e.Label.Text
	}
	return fmt.Sprintf("%s %s %s%s", from, arrow, to, label)
}

func (e *Edge) isBetweenTableColumns() bool {
	return e.FromTableColumnIndex != nil && e.ToTableColumnIndex != nil
}

func (e *Edge) hasTableColumn() bool {
	return e.FromTableColumnIndex != nil || e.ToTableColumnIndex != nil
}

func (e *Edge) isClusterEdge() bool {
	return e.From.Cluster != nil || e.To.Cluster != nil
}

func (e *Edge) euclideanDistance() float64 {
	return e.From.distance(e.To, true)
}

func (e *Edge) segmentCount() int {
	// One point for source and one for target centers = 2 for a straight line
	return len(e.Points) - 1
}

// reconnect takes care of reassigning an edge
// 1. Update the other endpoint's adjacent nodes to replace old one with new
// 2. Update the new endpoint's adjacent nodes to include the other endpoint
// 3. Remove the other endpoint from the old endpoint's adjacent nodes
// 4. Reassign the edge pointers
func (e *Edge) reconnect(newEndpoint *Node, isTo bool) {
	if isTo {
		e.To.removeEdge(e)
		newEndpoint.addEdge(e)

		e.To = newEndpoint
	} else {
		e.From.removeEdge(e)
		newEndpoint.addEdge(e)

		e.From = newEndpoint
	}
}

// isAxisAligned checks if the connected ends are axis aligned
// Test is to draw a line through the center and see if it overlaps
func (e *Edge) isAxisAligned() bool {
	if e.From == nil || e.To == nil {
		return false
	}

	if e.hasTableColumn() {
		ports := e.facingTablePorts(nil, nil)
		if ports.hasFrom && ports.hasTo {
			if geo.PrecisionCompare(ports.from.Y, ports.to.Y, AxisAlignmentTolerance) == 0 {
				return true
			}
		} else if ports.hasFrom {
			if geo.PrecisionCompare(ports.from.Y, e.To.TopLeft.Y+e.To.Height/2, AxisAlignmentTolerance) == 0 {
				return true
			}
		} else if ports.hasTo {
			if geo.PrecisionCompare(ports.to.Y, e.From.TopLeft.Y+e.From.Height/2, AxisAlignmentTolerance) == 0 {
				return true
			}
		} else if geo.PrecisionCompare(e.From.TopLeft.X, e.To.TopLeft.X, AxisAlignmentTolerance) == 0 {
			// if the tables are stacked, consider them aligned one of the sides is aligned
			return true
		} else if geo.PrecisionCompare(e.From.TopLeft.X+e.From.Width, e.To.TopLeft.X+e.To.Width, AxisAlignmentTolerance) == 0 {
			// if the tables are stacked, consider them aligned one of the sides is aligned
			return true
		}
		return false
	}

	// Horizontal alignment
	if geo.PrecisionCompare(e.From.TopLeft.Y+e.From.Height/2, e.To.TopLeft.Y+e.To.Height/2, AxisAlignmentTolerance) == 0 {
		return true
	}

	// Vertical alignment
	if geo.PrecisionCompare(e.From.TopLeft.X+e.From.Width/2, e.To.TopLeft.X+e.To.Width/2, AxisAlignmentTolerance) == 0 {
		return true
	}

	return false
}

func (edge *Edge) isDuplicateOf(otherEdge *Edge) bool {
	if otherEdge.To == edge.To && otherEdge.From == edge.From {
		return true
	}
	if otherEdge.From == edge.To && otherEdge.To == edge.From {
		return true
	}

	return false
}

func (edge *Edge) hasDuplicateIn(edges []*Edge) bool {
	for _, otherEdge := range edges {
		if otherEdge.isDuplicateOf(edge) {
			return true
		}
	}
	return false
}

type Edges []*Edge

// return map to lookup the edges connected to each port of the node
func (edges Edges) portEdges(node *Node) map[geo.Point][]*Edge {
	portEdges := make(map[geo.Point][]*Edge)
	for _, edge := range edges {
		var port *geo.Point
		switch {
		case edge.From == node:
			port = edge.SourcePort()
		case edge.To == node:
			port = edge.TargetPort()
		default:
			continue
		}

		if port == nil {
			continue
		}

		if _, exists := portEdges[*port]; !exists {
			portEdges[*port] = make([]*Edge, 0)
		}
		portEdges[*port] = append(portEdges[*port], edge)
	}
	return portEdges
}

func (edge *Edge) Length() float64 {
	return geo.Route(edge.Points).Length()
}

// LabelTopLeft returns the top-left point for a label of the given size and
// position on edge.
func (edge *Edge) LabelTopLeft(labelPosition label.Position, width, height float64) *geo.Point {
	point, _ := labelPosition.GetPointOnRoute(
		edge.Points,
		edgeStrokeWidth,
		edge.LabelPercentage,
		width,
		height,
	)
	return point
}

func (e *Edge) bounds() (*geo.Point, *geo.Point) {
	tl, br := e.boundingBoxValues()
	return &tl, &br
}

func (e *Edge) boundingBoxValues() (geo.Point, geo.Point) {
	tl := geo.Point{X: math.Inf(1), Y: math.Inf(1)}
	br := geo.Point{X: math.Inf(-1), Y: math.Inf(-1)}

	for _, p := range e.Points {
		tl.X = math.Min(tl.X, p.X)
		tl.Y = math.Min(tl.Y, p.Y)
		br.X = math.Max(br.X, p.X)
		br.Y = math.Max(br.Y, p.Y)
	}

	if e.Label != nil && len(e.Points) != 0 && e.Label.Position != label.Unset {
		labelTL := e.LabelTopLeft(e.Label.Position, e.Label.Width, e.Label.Height)
		tl.X = math.Min(tl.X, labelTL.X)
		tl.Y = math.Min(tl.Y, labelTL.Y)
		br.X = math.Max(br.X, labelTL.X+e.Label.Width)
		br.Y = math.Max(br.Y, labelTL.Y+e.Label.Height)
	}
	if len(e.Points) > 0 {
		if label := e.SourceArrowheadLabel; label != nil {
			labelTL := labelgeom.ArrowheadTopLeft(
				e.Points,
				false,
				string(e.SourceArrowhead),
				string(e.TargetArrowhead),
				label.Width,
				label.Height,
			)
			tl.X = math.Min(tl.X, labelTL.X)
			tl.Y = math.Min(tl.Y, labelTL.Y)
			br.X = math.Max(br.X, labelTL.X+label.Width)
			br.Y = math.Max(br.Y, labelTL.Y+label.Height)
		}
		if label := e.TargetArrowheadLabel; label != nil {
			labelTL := labelgeom.ArrowheadTopLeft(
				e.Points,
				true,
				string(e.SourceArrowhead),
				string(e.TargetArrowhead),
				label.Width,
				label.Height,
			)
			tl.X = math.Min(tl.X, labelTL.X)
			tl.Y = math.Min(tl.Y, labelTL.Y)
			br.X = math.Max(br.X, labelTL.X+label.Width)
			br.Y = math.Max(br.Y, labelTL.Y+label.Height)
		}
	}

	tl.X = math.Round(tl.X)
	tl.Y = math.Round(tl.Y)
	br.X = math.Round(br.X)
	br.Y = math.Round(br.Y)
	return tl, br
}

func (e *Edge) isTargetedTo(n *Node) bool {
	return e.hasArrowTo(n)
}

type EdgeSegment struct {
	geo.Segment
	edge *Edge
}

// For each axes:
// 1. For every edge segment of the axis, get its range of movement
// ---- The range of movement: the lower and upper bounds of where it can move
// ---- The considerations for bounds are node borders and locked in segments
// 2. Get the smallest range of movement, and group it with other edges that overlap and share the same range.
// 3. Evenly distribute the edges within its range of motion. They are now locked in.
// NOTE: Moving them may change the relative order of the edges.
// 4. Repeat from 1 until all edges locked in

// These two share the same range of movement (the left and right ends of the top node)
// .    ┌────────────────┐
// .    │                │
// .    │                │
// .    └──┬─────────────┘
// .       │   ▲
// .       │   │
// .       │   │
// .       │   │
// .       │   │
// .       │   │
// .       │   │
// .       │   │
// .       ▼   │
// . ┌─────────┴──────────────┐
// . │                        │
// . │                        │
// . └────────────────────────┘
//
// These two vertical segments do *not* share range of movement, since the bottom one has a higher floor.
// . ┌───────┐
// . │       │
// . │       │◄─────────────────┐
// . │       │                  │
// . │       │                  │
// . └───────┘                  │
// .                            │        ┌────────────────────────┐
// .                            │        │                        │
// .                            │        │                        │
// .                            │        │                        │
// .                            │        │                        │
// .  ┌──────┐                  │        │                        │
// .  │      │                  │        │                        │
// .  │      │◄─────────────────┼──┐     │                        │
// .  │      │                  │  │     │                        │
// .  └──────┘                  │  │     │                        │
// .                            │  │     │                        │
// .                            │  │     │                        │
// .  ┌──────┐                  │  │     │                        │
// .  │      ├──────────────────┘  │     │                        │
// .  │      │                     │     │                        │
// .  └──────┘     ┌──────┐        │     │                        │
// .               │      │        │     │                        │
// .               │      │        │     │                        │
// .               │      │        │     │                        │
// .               └──────┘        │     │                        │
// .                               │     │                        │
// .                               │     │                        │
// .                               │     └────────────────────────┘
// .  ┌───────┐                    │
// .  │       │                    │
// .  │       ├────────────────────┘
// .  │       │
// .  └───────┘
type facingTablePortValues struct {
	from        geo.Point
	to          geo.Point
	hasFrom     bool
	hasTo       bool
	orientation geo.Orientation
}

// facingTablePorts returns the ports related to the connected tables
// if they are properly aligned, without allocating points. If one table is on
// top of the other, there's no way to choose the proper direction and then
// there are no ports to return.
func (e *Edge) facingTablePorts(abductionFrom, abductionTo *Node) facingTablePortValues {
	if !e.hasTableColumn() {
		return facingTablePortValues{orientation: geo.NONE}
	}
	from := e.From
	if abductionFrom != nil {
		from = abductionFrom
	}
	to := e.To
	if abductionTo != nil {
		to = abductionTo
	}
	orientation := from.orientation(to)
	ports := facingTablePortValues{orientation: orientation}
	switch orientation {
	case geo.TopLeft, geo.BottomLeft, geo.Left:
		if e.FromTableColumnIndex != nil {
			ports.from, ports.hasFrom = from.tableColumnPortValue(geo.Right, *e.FromTableColumnIndex)
		}
		if e.ToTableColumnIndex != nil {
			ports.to, ports.hasTo = to.tableColumnPortValue(geo.Left, *e.ToTableColumnIndex)
		}
		return ports
	case geo.Right, geo.TopRight, geo.BottomRight:
		if e.FromTableColumnIndex != nil {
			ports.from, ports.hasFrom = from.tableColumnPortValue(geo.Left, *e.FromTableColumnIndex)
		}
		if e.ToTableColumnIndex != nil {
			ports.to, ports.hasTo = to.tableColumnPortValue(geo.Right, *e.ToTableColumnIndex)
		}
		return ports
	}
	return facingTablePortValues{orientation: geo.NONE}
}

func (e *Edge) HasLargeArrowheadLabel() bool {
	return (e.SourceArrowheadLabel != nil && len(e.SourceArrowheadLabel.Text) > 3) ||
		(e.TargetArrowheadLabel != nil && len(e.TargetArrowheadLabel.Text) > 3)
}

func (e *Edge) isStraight() bool {
	p := e.Points[0]
	q := e.Points[1]
	for i := 2; i < len(e.Points); i++ {
		if orientation(p, q, e.Points[i]) != 0 {
			return false
		}
		p = q
		q = e.Points[i]
	}
	return true
}

func (e *Edge) hasOverlappingEnd() bool {
	start := e.Points[0]
	end := e.Points[len(e.Points)-1]

	for _, fromEdge := range e.From.Edges {
		if fromEdge == e {
			continue
		}

		if nonNilEquals(fromEdge.Points[0], start) || nonNilEquals(fromEdge.Points[len(fromEdge.Points)-1], start) {
			return true
		}
	}
	for _, toEdge := range e.To.Edges {
		if toEdge == e {
			continue
		}

		if nonNilEquals(toEdge.Points[0], end) || nonNilEquals(toEdge.Points[len(toEdge.Points)-1], end) {
			return true
		}
	}
	return false
}

/*
The edge to the left may have routed first and been more optimal
The edge to the right then routed, but could not route optimally due to obstruction
So we balance by going to the less optimal spot, since branching from same point is still more aesthetically pleasing
// ┌──────────┐                ┌───────────┐
// │          │                │           │
// │          │                │           │
// └──────────┘                └───────────┘
//      ▲                            ▲
//      │                            │
//      │                            │
//      │              ┌─────────────┘
//      │              │
//      │              │
//      │              │
//      │              │
//      └──────────────┤
//                     │
//                     │
//                     │
//              ┌──────┴──────┐
//              │             │
//              │             │
//              │             │
//              │             │
//              └─────────────┘
*/
