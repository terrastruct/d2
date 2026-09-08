package d2talalayout

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2sequence"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/label"
)

type d2ObjectVisit struct {
	object *d2graph.Object
	parent *d2graph.Object
	depth  int
}

// collectD2ObjectVisits validates the object tree and returns its non-root
// objects in the same depth-first pre-order used by D2. Keeping traversal
// iterative makes malformed or deeply nested caller-built graphs an ordinary
// input error instead of a process-threatening stack overflow.
func collectD2ObjectVisits(ctx context.Context, root *d2graph.Object) ([]d2ObjectVisit, map[*d2graph.Object]struct{}, error) {
	if root == nil {
		return nil, nil, fmt.Errorf("tala requires a D2 graph with a root object")
	}

	stack := []d2ObjectVisit{{object: root}}
	parents := make(map[*d2graph.Object]*d2graph.Object)
	objects := make(map[*d2graph.Object]struct{})
	visits := make([]d2ObjectVisit, 0, 64)
	scheduledObjects := 0

	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		visit := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visit.object == nil {
			return nil, nil, fmt.Errorf("D2 object tree contains a nil object at depth %d", visit.depth)
		}

		if _, seen := parents[visit.object]; seen {
			if d2ObjectIsAncestor(visit.object, visit.parent, parents) {
				return nil, nil, fmt.Errorf("D2 object %q forms a ChildrenArray cycle", visit.object.ID)
			}
			return nil, nil, fmt.Errorf("D2 object %q is referenced more than once in ChildrenArray", visit.object.ID)
		}
		if visit.object.Parent != visit.parent {
			return nil, nil, fmt.Errorf(
				"D2 object %q has a Parent that does not match its ChildrenArray parent",
				visit.object.ID,
			)
		}
		if visit.depth > maxInputTreeDepth {
			return nil, nil, fmt.Errorf("D2 object nesting depth exceeds limit %d", maxInputTreeDepth)
		}
		parents[visit.object] = visit.parent

		if visit.parent != nil {
			if len(visits) == maxInputNodes {
				return nil, nil, fmt.Errorf("D2 object count exceeds limit %d", maxInputNodes)
			}
			visits = append(visits, visit)
			objects[visit.object] = struct{}{}
		}

		children := visit.object.ChildrenArray
		if len(children) > maxInputNodes-scheduledObjects {
			return nil, nil, fmt.Errorf("D2 object count exceeds limit %d", maxInputNodes)
		}
		scheduledObjects += len(children)
		for _, c := range slices.Backward(children) {
			stack = append(stack, d2ObjectVisit{
				object: c,
				parent: visit.object,
				depth:  visit.depth + 1,
			})
		}
	}

	return visits, objects, nil
}

func d2ObjectIsAncestor(ancestor, object *d2graph.Object, parents map[*d2graph.Object]*d2graph.Object) bool {
	for object != nil {
		if object == ancestor {
			return true
		}
		object = parents[object]
	}
	return false
}

func validateD2Objects(ctx context.Context, visits []d2ObjectVisit, requirePosition bool) error {
	tableColumns := 0
	for objectIndex, visit := range visits {
		if err := ctx.Err(); err != nil {
			return err
		}
		object := visit.object
		if object.Box == nil {
			return fmt.Errorf("D2 object %d (%q) has no box", objectIndex, object.ID)
		}
		if requirePosition && object.TopLeft == nil {
			return fmt.Errorf("D2 object %q has no top-left position", object.ID)
		}
		if err := validateResultDimension("width", object.Width, false); err != nil {
			return fmt.Errorf("D2 object %q: %w", object.ID, err)
		}
		if err := validateResultDimension("height", object.Height, false); err != nil {
			return fmt.Errorf("D2 object %q: %w", object.ID, err)
		}
		if object.TopLeft != nil {
			if err := validateResultCoordinate("top-left x", object.TopLeft.X); err != nil {
				return fmt.Errorf("D2 object %q: %w", object.ID, err)
			}
			if err := validateResultCoordinate("top-left y", object.TopLeft.Y); err != nil {
				return fmt.Errorf("D2 object %q: %w", object.ID, err)
			}
			if err := validateResultCoordinate("bottom-right x", object.TopLeft.X+object.Width); err != nil {
				return fmt.Errorf("D2 object %q: %w", object.ID, err)
			}
			if err := validateResultCoordinate("bottom-right y", object.TopLeft.Y+object.Height); err != nil {
				return fmt.Errorf("D2 object %q: %w", object.ID, err)
			}
		}
		if err := validateD2TextDimensions("label", object.LabelDimensions); err != nil {
			return fmt.Errorf("D2 object %q: %w", object.ID, err)
		}
		if err := validateD2Style("D2 object "+fmt.Sprintf("%q", object.ID), object.Style); err != nil {
			return err
		}
		if object.LabelPosition != nil && !label.FromString(*object.LabelPosition).IsShapePosition() {
			return fmt.Errorf("D2 object %q has invalid label position %q", object.ID, *object.LabelPosition)
		}
		if object.IconPosition != nil && !label.FromString(*object.IconPosition).IsShapePosition() {
			return fmt.Errorf("D2 object %q has invalid icon position %q", object.ID, *object.IconPosition)
		}
		if (object.Top == nil) != (object.Left == nil) {
			return fmt.Errorf("D2 object %q must set top and left together for a locked position", object.ID)
		}
		if object.Top != nil {
			if _, err := parseD2IntegerAttribute(object.ID, "top", object.Top, -maxResultCoordinate, maxResultCoordinate); err != nil {
				return err
			}
		}
		if object.Left != nil {
			if _, err := parseD2IntegerAttribute(object.ID, "left", object.Left, -maxResultCoordinate, maxResultCoordinate); err != nil {
				return err
			}
		}
		if object.WidthAttr != nil {
			if _, err := parseD2IntegerAttribute(object.ID, "width", object.WidthAttr, 1, maxResultCoordinate); err != nil {
				return err
			}
		}
		if object.HeightAttr != nil {
			if _, err := parseD2IntegerAttribute(object.ID, "height", object.HeightAttr, 1, maxResultCoordinate); err != nil {
				return err
			}
		}

		fontSize := object.Text().FontSize
		if object.Class != nil || object.SQLTable != nil {
			fontSize -= d2target.HeaderFontAdd
		}
		if fontSize <= 0 || fontSize > maxResultCoordinate {
			return fmt.Errorf("D2 object %q has unsupported effective font size %d", object.ID, fontSize)
		}

		if object.SQLTable != nil {
			if object.Shape.Value != d2target.ShapeSQLTable {
				return fmt.Errorf("D2 object %q has SQL columns but is not an SQL table", object.ID)
			}
			if len(object.SQLTable.Columns) > maxInputNodes-tableColumns {
				return fmt.Errorf("D2 SQL table column count exceeds limit %d", maxInputNodes)
			}
			tableColumns += len(object.SQLTable.Columns)
		}
	}
	return nil
}

func validateD2Style(owner string, style d2graph.Style) error {
	if style.Opacity != nil {
		value, err := strconv.ParseFloat(style.Opacity.Value, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return fmt.Errorf("%s has invalid opacity %q", owner, style.Opacity.Value)
		}
	}
	for _, attribute := range []struct {
		name  string
		value *d2graph.Scalar
		min   int
	}{
		{name: "stroke-width", value: style.StrokeWidth, min: 0},
		{name: "font-size", value: style.FontSize, min: 1},
	} {
		if attribute.value == nil {
			continue
		}
		value, err := strconv.Atoi(attribute.value.Value)
		if err != nil || value < attribute.min || value > maxResultCoordinate {
			return fmt.Errorf("%s has invalid %s %q", owner, attribute.name, attribute.value.Value)
		}
	}
	for _, attribute := range []struct {
		name  string
		value *d2graph.Scalar
	}{
		{name: "animated", value: style.Animated},
		{name: "bold", value: style.Bold},
		{name: "double-border", value: style.DoubleBorder},
		{name: "filled", value: style.Filled},
		{name: "italic", value: style.Italic},
		{name: "multiple", value: style.Multiple},
		{name: "shadow", value: style.Shadow},
		{name: "3d", value: style.ThreeDee},
		{name: "underline", value: style.Underline},
	} {
		if attribute.value == nil {
			continue
		}
		if _, err := strconv.ParseBool(attribute.value.Value); err != nil {
			return fmt.Errorf("%s has invalid %s boolean %q", owner, attribute.name, attribute.value.Value)
		}
	}
	return nil
}

func parseD2IntegerAttribute(objectID, name string, scalar *d2graph.Scalar, minValue, maxValue int) (int, error) {
	value, err := strconv.Atoi(scalar.Value)
	if err != nil {
		return 0, fmt.Errorf("D2 object %q has invalid integer %s attribute %q", objectID, name, scalar.Value)
	}
	if value < minValue || value > maxValue {
		return 0, fmt.Errorf(
			"D2 object %q %s attribute must be between %d and %d",
			objectID,
			name,
			minValue,
			maxValue,
		)
	}
	return value, nil
}

func validateD2TextDimensions(name string, dimensions d2target.TextDimensions) error {
	if dimensions.Width < 0 || dimensions.Height < 0 {
		return fmt.Errorf("%s dimensions must be nonnegative", name)
	}
	if dimensions.Width > maxResultCoordinate || dimensions.Height > maxResultCoordinate {
		return fmt.Errorf("%s dimensions exceed supported limit %d", name, maxResultCoordinate)
	}
	return nil
}

func findD2Object(ctx context.Context, root *d2graph.Object, key []string) (*d2graph.Object, bool, error) {
	if len(key) > maxInputTreeDepth {
		return nil, false, fmt.Errorf("D2 near key depth exceeds limit %d", maxInputTreeDepth)
	}
	object := root
	for _, id := range key {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		if object == nil {
			return nil, false, fmt.Errorf("D2 near key traverses a nil object before %q", id)
		}
		child, found := object.Children[strings.ToLower(id)]
		if !found {
			return nil, false, nil
		}
		if child == nil {
			return nil, false, fmt.Errorf("D2 object %q has a nil child for near key part %q", object.ID, id)
		}
		object = child
	}
	return object, true, nil
}

// validateRequestedD2Edges bounds subset validation before allocating maps or
// translating the graph. Every requested edge must be a distinct member of g.
func validateRequestedD2Edges(g *d2graph.Graph, edges []*d2graph.Edge) error {
	if len(edges) > maxInputEdges {
		return fmt.Errorf("requested D2 edge count exceeds limit %d", maxInputEdges)
	}
	requested := make(map[*d2graph.Edge]struct{}, len(edges))
	for index, edge := range edges {
		if edge == nil {
			return fmt.Errorf("requested D2 edge %d is nil", index)
		}
		if _, duplicate := requested[edge]; duplicate {
			return fmt.Errorf("requested D2 edge %d is repeated", index)
		}
		requested[edge] = struct{}{}
	}
	if g == nil {
		return nil
	}
	if len(g.Edges) > maxInputEdges {
		return fmt.Errorf("D2 edge count exceeds limit %d", maxInputEdges)
	}
	if len(edges) > len(g.Edges) {
		return fmt.Errorf("requested D2 edge count %d exceeds graph edge count %d", len(edges), len(g.Edges))
	}
	graphEdges := make(map[*d2graph.Edge]struct{}, len(g.Edges))
	for _, edge := range g.Edges {
		graphEdges[edge] = struct{}{}
	}
	for index, edge := range edges {
		if _, exists := graphEdges[edge]; !exists {
			return fmt.Errorf("requested D2 edge %d is not in the graph", index)
		}
	}
	return nil
}

// validateD2GraphStructure keeps D2's three public topology views coherent.
// The adapter traverses ChildrenArray, resolves near keys through Children,
// and applies results through Graph.Objects; accepting different sets would
// produce a successful layout whose patch is incomplete or targets the wrong
// objects.
func validateD2GraphStructure(g *d2graph.Graph, visits []d2ObjectVisit, objects map[*d2graph.Object]struct{}) error {
	if g.Root.Graph != g {
		return fmt.Errorf("D2 root object belongs to a different graph")
	}

	allObjects := make([]*d2graph.Object, 0, len(visits)+1)
	allObjects = append(allObjects, g.Root)
	for _, visit := range visits {
		allObjects = append(allObjects, visit.object)
	}
	for _, object := range allObjects {
		if object.Graph != g {
			return fmt.Errorf("D2 object %q belongs to a different graph", object.ID)
		}
		if len(object.Children) != len(object.ChildrenArray) {
			return fmt.Errorf("D2 object %q has inconsistent Children and ChildrenArray sets", object.ID)
		}
		for _, child := range object.ChildrenArray {
			key := strings.ToLower(child.ID)
			mapped, found := object.Children[key]
			if !found || mapped != child {
				return fmt.Errorf("D2 object %q has inconsistent Children and ChildrenArray sets", object.ID)
			}
		}
	}

	if len(g.Objects) != len(visits) {
		return fmt.Errorf("D2 graph Objects and object tree contain different sets")
	}
	seen := make(map[*d2graph.Object]struct{}, len(g.Objects))
	for index, object := range g.Objects {
		if object == nil {
			return fmt.Errorf("D2 graph Objects contains nil at index %d", index)
		}
		if _, duplicate := seen[object]; duplicate {
			return fmt.Errorf("D2 graph Objects repeats object %q", object.ID)
		}
		seen[object] = struct{}{}
		if _, inTree := objects[object]; !inTree {
			return fmt.Errorf("D2 graph Objects contains object %q outside the object tree", object.ID)
		}
	}
	return nil
}

func isSupportedLifelineEdge(edge *d2graph.Edge) bool {
	if edge.SrcArrow || edge.DstArrow || edge.SrcArrowhead != nil || edge.DstArrowhead != nil {
		return false
	}
	return d2sequence.IsLifelineEnd(edge.Dst) &&
		edge.Dst.ID == d2sequence.LifelineEndID(edge.Src.ID)
}

func validateD2Edges(ctx context.Context, edges []*d2graph.Edge, objects map[*d2graph.Object]struct{}) error {
	if len(edges) > maxInputEdges {
		return fmt.Errorf("D2 edge count exceeds limit %d", maxInputEdges)
	}

	seen := make(map[*d2graph.Edge]struct{}, len(edges))
	routePoints := 0
	for edgeIndex, edge := range edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if edge == nil {
			return fmt.Errorf("D2 edge %d is nil", edgeIndex)
		}
		if _, exists := seen[edge]; exists {
			return fmt.Errorf("D2 edge %d is repeated", edgeIndex)
		}
		seen[edge] = struct{}{}
		if edge.Src == nil || edge.Dst == nil {
			return fmt.Errorf("D2 edge %d has a nil endpoint", edgeIndex)
		}
		if err := validateD2TextDimensions("label", edge.LabelDimensions); err != nil {
			return fmt.Errorf("D2 edge %d: %w", edgeIndex, err)
		}
		if err := validateD2Style(fmt.Sprintf("D2 edge %d", edgeIndex), edge.Style); err != nil {
			return err
		}
		if edge.LabelPosition != nil && !label.FromString(*edge.LabelPosition).IsEdgePosition() {
			return fmt.Errorf("D2 edge %d has invalid label position %q", edgeIndex, *edge.LabelPosition)
		}
		if p := edge.LabelPercentage; p != nil && (math.IsNaN(*p) || math.IsInf(*p, 0) || *p < 0 || *p > 1) {
			return fmt.Errorf("D2 edge %d has invalid label percentage %v: must be finite and between 0 and 1", edgeIndex, *p)
		}
		if edge.SrcArrowhead != nil {
			if err := validateD2Arrowhead(edgeIndex, "source", edge.SrcArrowhead); err != nil {
				return err
			}
			if err := validateD2TextDimensions("source arrowhead label", edge.SrcArrowhead.LabelDimensions); err != nil {
				return fmt.Errorf("D2 edge %d: %w", edgeIndex, err)
			}
		}
		if edge.DstArrowhead != nil {
			if err := validateD2Arrowhead(edgeIndex, "destination", edge.DstArrowhead); err != nil {
				return err
			}
			if err := validateD2TextDimensions("destination arrowhead label", edge.DstArrowhead.LabelDimensions); err != nil {
				return fmt.Errorf("D2 edge %d: %w", edgeIndex, err)
			}
		}
		if _, exists := objects[edge.Src]; !exists {
			return fmt.Errorf("D2 edge %d has a source outside the object tree", edgeIndex)
		}
		if _, exists := objects[edge.Dst]; !exists {
			if !isSupportedLifelineEdge(edge) {
				return fmt.Errorf("D2 edge %d has a destination outside the object tree", edgeIndex)
			}
		}
		if err := validateD2TableColumnIndex(edgeIndex, "source", edge.Src, edge.SrcTableColumnIndex); err != nil {
			return err
		}
		if err := validateD2TableColumnIndex(edgeIndex, "destination", edge.Dst, edge.DstTableColumnIndex); err != nil {
			return err
		}

		if len(edge.Route) > maxInputRoutePoints-routePoints {
			return fmt.Errorf("D2 route point count exceeds limit %d", maxInputRoutePoints)
		}
		if len(edge.Route) == 1 {
			return fmt.Errorf("D2 edge %d route requires either zero or at least two points", edgeIndex)
		}
		routePoints += len(edge.Route)
		for pointIndex, point := range edge.Route {
			if pointIndex%256 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if point == nil {
				return fmt.Errorf("D2 edge %d has a nil route point at index %d", edgeIndex, pointIndex)
			}
			if err := validateResultCoordinate("route x", point.X); err != nil {
				return fmt.Errorf("D2 edge %d route point %d: %w", edgeIndex, pointIndex, err)
			}
			if err := validateResultCoordinate("route y", point.Y); err != nil {
				return fmt.Errorf("D2 edge %d route point %d: %w", edgeIndex, pointIndex, err)
			}
			if pointIndex > 0 && point.X == edge.Route[pointIndex-1].X && point.Y == edge.Route[pointIndex-1].Y {
				return fmt.Errorf("D2 edge %d has a degenerate route segment ending at index %d", edgeIndex, pointIndex)
			}
		}
	}
	return nil
}

func validateD2Arrowhead(edgeIndex int, endpoint string, attributes *d2graph.Attributes) error {
	if attributes.Shape.Value != "" {
		if _, valid := d2target.Arrowheads[attributes.Shape.Value]; !valid {
			return fmt.Errorf("D2 edge %d has unsupported %s arrowhead %q", edgeIndex, endpoint, attributes.Shape.Value)
		}
	}
	return validateD2Style(fmt.Sprintf("D2 edge %d %s arrowhead", edgeIndex, endpoint), attributes.Style)
}

func validateD2TableColumnIndex(edgeIndex int, endpointName string, object *d2graph.Object, index *int) error {
	if index == nil {
		return nil
	}
	if object.SQLTable == nil {
		return fmt.Errorf("D2 edge %d has a %s table column index for a non-table object", edgeIndex, endpointName)
	}
	if *index < 0 || *index >= len(object.SQLTable.Columns) {
		return fmt.Errorf(
			"D2 edge %d %s table column index %d is outside table with %d columns",
			edgeIndex,
			endpointName,
			*index,
			len(object.SQLTable.Columns),
		)
	}
	return nil
}
