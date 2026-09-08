package d2talalayout

import (
	"context"
	"fmt"
	"math"
	"slices"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

type resultEdgeTopology struct {
	from layoutgraph.EntityID
	to   layoutgraph.EntityID
}

// validateLayoutResultTopology binds a completed workspace to its immutable
// input while allowing sequence-defining edges to be consumed by the layoutgraph.
// All other input records must still be present with the same identity and
// endpoints, and the result may not introduce extra records.
func validateLayoutResultTopology(ctx context.Context, expected, actual *layoutgraph.Graph) (map[layoutgraph.EntityID]struct{}, error) {
	sequenceEdges, err := grouping.SequenceDefiningEdges(ctx, expected)
	if err != nil {
		return nil, err
	}
	if err := validateTopology(ctx, expected, actual, sequenceEdges); err != nil {
		return nil, err
	}
	return sequenceEdges, nil
}

func validateTopology(ctx context.Context, expected, actual *layoutgraph.Graph, allowedMissingEdges map[layoutgraph.EntityID]struct{}) error {
	if expected == nil || actual == nil {
		return fmt.Errorf("cannot compare nil graph topology")
	}
	if len(expected.Nodes) != len(actual.Nodes) {
		return fmt.Errorf("node count changed from %d to %d", len(expected.Nodes), len(actual.Nodes))
	}
	expectedNodes := make(map[layoutgraph.EntityID]struct{}, len(expected.Nodes))
	for _, node := range expected.Nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("input contains a nil node")
		}
		if _, exists := expectedNodes[node.ID]; exists {
			return fmt.Errorf("input contains duplicate node ID %d", node.ID)
		}
		expectedNodes[node.ID] = struct{}{}
	}
	for _, node := range actual.Nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := expectedNodes[node.ID]; !ok {
			return fmt.Errorf("result contains unexpected node ID %d", node.ID)
		}
	}

	expectedEdges := make(map[layoutgraph.EntityID]resultEdgeTopology, len(expected.Edges))
	for _, edge := range expected.Edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if edge == nil || edge.From == nil || edge.To == nil {
			return fmt.Errorf("input contains an edge with a nil endpoint")
		}
		if _, exists := expectedEdges[edge.ID]; exists {
			return fmt.Errorf("input contains duplicate edge ID %d", edge.ID)
		}
		expectedEdges[edge.ID] = resultEdgeTopology{from: edge.From.ID, to: edge.To.ID}
	}
	actualEdges := make(map[layoutgraph.EntityID]struct{}, len(actual.Edges))
	for _, edge := range actual.Edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if edge == nil || edge.From == nil || edge.To == nil {
			return fmt.Errorf("result contains an edge with a nil endpoint")
		}
		if _, duplicate := actualEdges[edge.ID]; duplicate {
			return fmt.Errorf("result contains duplicate edge ID %d", edge.ID)
		}
		actualEdges[edge.ID] = struct{}{}
		want, ok := expectedEdges[edge.ID]
		if !ok {
			return fmt.Errorf("result contains unexpected edge ID %d", edge.ID)
		}
		if edge.From.ID != want.from || edge.To.ID != want.to {
			return fmt.Errorf(
				"edge %d endpoints changed from %d->%d to %d->%d",
				edge.ID,
				want.from,
				want.to,
				edge.From.ID,
				edge.To.ID,
			)
		}
	}
	for id := range expectedEdges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, present := actualEdges[id]; present {
			continue
		}
		if _, allowed := allowedMissingEdges[id]; allowed {
			continue
		}
		return fmt.Errorf("result is missing edge ID %d", id)
	}
	return nil
}

// validateLayoutResultMetadata checks the immutable geometry semantics that a
// completed layout attempt must preserve. Attempt-owned
// boxes, routes, font-size values, mutable node-label geometry, edge-label
// positions, Near relationships, and derived layout structures deliberately
// remain outside this comparison. Near relationships can be both inferred and
// removed by hierarchy/cluster processing. Fixed-position and desired-size values are
// bound as metadata, but whether the result satisfies those constraints is an
// output-validity concern, not an immutable-metadata concern.
func validateLayoutResultMetadata(
	ctx context.Context,
	expected, actual *layoutgraph.Graph,
	allowedMissingEdges map[layoutgraph.EntityID]struct{},
) error {
	if expected == nil || actual == nil {
		return fmt.Errorf("cannot compare nil graph metadata")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if expected.IsRootHierarchy != actual.IsRootHierarchy {
		return fmt.Errorf("root hierarchy metadata changed")
	}
	if equal, err := sameDirectionMetadata(ctx, expected, actual); err != nil {
		return err
	} else if !equal {
		return fmt.Errorf("graph direction metadata changed")
	}

	expectedNodes := make(map[layoutgraph.EntityID]*layoutgraph.Node, len(expected.Nodes))
	for _, node := range expected.Nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("input contains a nil node")
		}
		expectedNodes[node.ID] = node
	}
	for _, node := range actual.Nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("result contains a nil node")
		}
		input := expectedNodes[node.ID]
		if input == nil {
			return fmt.Errorf("result contains unexpected node ID %d", node.ID)
		}
		if input.ShapeType() != node.ShapeType() {
			return fmt.Errorf("node %d shape changed from %q to %q", node.ID, input.ShapeType(), node.ShapeType())
		}
		if (input.Container == nil) != (node.Container == nil) ||
			input.Container != nil && input.Container.ID != node.Container.ID {
			return fmt.Errorf("node %d container ownership changed", node.ID)
		}
		if input.NumColumns() != node.NumColumns() {
			return fmt.Errorf("node %d table column count changed from %d to %d", node.ID, input.NumColumns(), node.NumColumns())
		}
		if input.Is3D != node.Is3D {
			return fmt.Errorf("node %d 3D outline metadata changed", node.ID)
		}
		if input.IsMultiple != node.IsMultiple {
			return fmt.Errorf("node %d multiple outline metadata changed", node.ID)
		}
		if input.IsInvisible != node.IsInvisible {
			return fmt.Errorf("node %d visibility metadata changed", node.ID)
		}
		if !sameOptionalValue(input.FixedTopLeft, node.FixedTopLeft) {
			return fmt.Errorf("node %d fixed position metadata changed", node.ID)
		}
		if !sameOptionalValue(input.DesiredWidth, node.DesiredWidth) ||
			!sameOptionalValue(input.DesiredHeight, node.DesiredHeight) {
			return fmt.Errorf("node %d desired-size metadata changed", node.ID)
		}
		if input.ForceHierarchy != node.ForceHierarchy {
			return fmt.Errorf("node %d forced-hierarchy metadata changed", node.ID)
		}
		if !sameLabelIdentity(input.Label, node.Label) {
			return fmt.Errorf("node %d label identity changed", node.ID)
		}
		if input.Label != nil && input.Label.Position != label.Unset && input.Label.Position != node.Label.Position {
			return fmt.Errorf("node %d fixed label position changed", node.ID)
		}
		if (input.Icon == nil) != (node.Icon == nil) {
			return fmt.Errorf("node %d icon presence changed", node.ID)
		}
		if input.Icon != nil && input.Icon.Position != label.Unset && input.Icon.Position != node.Icon.Position {
			return fmt.Errorf("node %d fixed icon position changed", node.ID)
		}
		if !isValidResultFontSize(input.FontSize, node.FontSize) {
			return fmt.Errorf("node %d font size is outside the layout output domain", node.ID)
		}
	}

	expectedEdges := make(map[layoutgraph.EntityID]*layoutgraph.Edge, len(expected.Edges))
	for _, edge := range expected.Edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if edge == nil {
			return fmt.Errorf("input contains a nil edge")
		}
		expectedEdges[edge.ID] = edge
	}
	actualEdges := make(map[layoutgraph.EntityID]struct{}, len(actual.Edges))
	for _, edge := range actual.Edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if edge == nil {
			return fmt.Errorf("result contains a nil edge")
		}
		actualEdges[edge.ID] = struct{}{}
		input := expectedEdges[edge.ID]
		if input == nil {
			return fmt.Errorf("result contains unexpected edge ID %d", edge.ID)
		}
		if input.SourceArrowhead != edge.SourceArrowhead || input.TargetArrowhead != edge.TargetArrowhead {
			return fmt.Errorf("edge %d arrowhead metadata changed", edge.ID)
		}
		if !sameImmutableLabel(input.SourceArrowheadLabel, edge.SourceArrowheadLabel) {
			return fmt.Errorf("edge %d source arrowhead label metadata changed", edge.ID)
		}
		if !sameImmutableLabel(input.TargetArrowheadLabel, edge.TargetArrowheadLabel) {
			return fmt.Errorf("edge %d target arrowhead label metadata changed", edge.ID)
		}
		if input.MinWidth != edge.MinWidth || input.MinHeight != edge.MinHeight {
			return fmt.Errorf("edge %d minimum geometry changed", edge.ID)
		}
		if !sameOptionalValue(input.FromTableColumnIndex, edge.FromTableColumnIndex) ||
			!sameOptionalValue(input.ToTableColumnIndex, edge.ToTableColumnIndex) {
			return fmt.Errorf("edge %d table column attachment metadata changed", edge.ID)
		}
		if input.IsInvisible != edge.IsInvisible {
			return fmt.Errorf("edge %d visibility metadata changed", edge.ID)
		}
		if !sameImmutableEdgeStyle(input.Style, edge.Style) {
			return fmt.Errorf("edge %d style metadata changed", edge.ID)
		}
		if !sameEdgeLabelMetadata(input.Label, edge.Label) {
			return fmt.Errorf("edge %d label identity or dimensions changed", edge.ID)
		}
	}
	for id := range expectedEdges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, present := actualEdges[id]; present {
			continue
		}
		if _, allowed := allowedMissingEdges[id]; allowed {
			continue
		}
		return fmt.Errorf("result is missing edge ID %d", id)
	}
	return nil
}

func sameLabelIdentity(expected, actual *layoutgraph.Label) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	return expected.Text == actual.Text
}

func sameEdgeLabelMetadata(expected, actual *layoutgraph.Label) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	return expected.Text == actual.Text &&
		expected.Width == actual.Width &&
		expected.Height == actual.Height
}

func sameImmutableLabel(expected, actual *layoutgraph.Label) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	return expected.Text == actual.Text &&
		expected.Position == actual.Position &&
		expected.Width == actual.Width &&
		expected.Height == actual.Height
}

// EquivalentStyles is intentionally a routing-specific comparison and omits
// renderer fields. Layout validation has a stronger contract: every attempt
// must return each carried input style unchanged, including fields such as
// BorderRadius that other layout stages may consume.
func sameImmutableEdgeStyle(expected, actual layoutgraph.EdgeStyle) bool {
	expectedValues := [...]*layoutgraph.StyleScalar{
		expected.Opacity,
		expected.Stroke,
		expected.Fill,
		expected.FillPattern,
		expected.StrokeWidth,
		expected.StrokeDash,
		expected.BorderRadius,
		expected.Shadow,
		expected.ThreeDee,
		expected.Multiple,
		expected.Font,
		expected.FontSize,
		expected.FontColor,
		expected.Animated,
		expected.Bold,
		expected.Italic,
		expected.Underline,
		expected.Filled,
		expected.DoubleBorder,
		expected.TextTransform,
	}
	actualValues := [...]*layoutgraph.StyleScalar{
		actual.Opacity,
		actual.Stroke,
		actual.Fill,
		actual.FillPattern,
		actual.StrokeWidth,
		actual.StrokeDash,
		actual.BorderRadius,
		actual.Shadow,
		actual.ThreeDee,
		actual.Multiple,
		actual.Font,
		actual.FontSize,
		actual.FontColor,
		actual.Animated,
		actual.Bold,
		actual.Italic,
		actual.Underline,
		actual.Filled,
		actual.DoubleBorder,
		actual.TextTransform,
	}
	for index := range expectedValues {
		if !sameOptionalValue(expectedValues[index], actualValues[index]) {
			return false
		}
	}
	return true
}

func sameOptionalValue[T comparable](expected, actual *T) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	return *expected == *actual
}

func isValidResultFontSize(expected, actual *int) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}
	return *actual == *expected || slices.Contains(d2fonts.FontSizes, *actual)
}

func sameDirectionMetadata(ctx context.Context, expected, actual *layoutgraph.Graph) (bool, error) {
	type directionKey struct {
		id   layoutgraph.EntityID
		root bool
	}
	canonical := func(graph *layoutgraph.Graph) (map[directionKey]geo.Orientation, error) {
		directions := make(map[directionKey]geo.Orientation, len(graph.Directions))
		for node, direction := range graph.Directions {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			key := directionKey{root: node == nil}
			if node != nil {
				key.id = node.ID
			}
			if _, duplicate := directions[key]; duplicate {
				return nil, fmt.Errorf("graph direction metadata repeats a canonical key")
			}
			directions[key] = direction
		}
		return directions, nil
	}
	expectedDirections, err := canonical(expected)
	if err != nil {
		return false, err
	}
	actualDirections, err := canonical(actual)
	if err != nil {
		return false, err
	}
	if len(expectedDirections) != len(actualDirections) {
		return false, nil
	}
	for key, direction := range expectedDirections {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if actualDirection, ok := actualDirections[key]; !ok || actualDirection != direction {
			return false, nil
		}
	}
	return true, nil
}

func validateCompletedGraph(ctx context.Context, graph *layoutgraph.Graph) error {
	if graph == nil {
		return fmt.Errorf("graph is nil")
	}
	nodes := make(map[*layoutgraph.Node]struct{}, len(graph.Nodes))
	nodeIDs := make(map[layoutgraph.EntityID]struct{}, len(graph.Nodes))
	for index, node := range graph.Nodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("node %d is nil", index)
		}
		if _, exists := nodes[node]; exists {
			return fmt.Errorf("node %d is repeated", node.ID)
		}
		nodes[node] = struct{}{}
		if _, exists := nodeIDs[node.ID]; exists {
			return fmt.Errorf("node ID %d is duplicated", node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
		if err := validateResultNode(node); err != nil {
			return err
		}
	}

	edges := make(map[*layoutgraph.Edge]struct{}, len(graph.Edges))
	edgeIDs := make(map[layoutgraph.EntityID]struct{}, len(graph.Edges))
	for index, edge := range graph.Edges {
		if err := ctx.Err(); err != nil {
			return err
		}
		if edge == nil {
			return fmt.Errorf("edge %d is nil", index)
		}
		if _, exists := edges[edge]; exists {
			return fmt.Errorf("edge %d is repeated", edge.ID)
		}
		edges[edge] = struct{}{}
		if _, exists := edgeIDs[edge.ID]; exists {
			return fmt.Errorf("edge ID %d is duplicated", edge.ID)
		}
		edgeIDs[edge.ID] = struct{}{}
		if _, exists := nodes[edge.From]; !exists {
			return fmt.Errorf("edge %d has a source outside the result graph", edge.ID)
		}
		if _, exists := nodes[edge.To]; !exists {
			return fmt.Errorf("edge %d has a destination outside the result graph", edge.ID)
		}
		if err := validateResultEdge(ctx, edge); err != nil {
			return err
		}
	}
	return nil
}

func validateResultNode(node *layoutgraph.Node) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	if node.TopLeft == nil {
		return fmt.Errorf("node %d has no position", node.ID)
	}
	if err := validateResultCoordinate("node x", node.TopLeft.X); err != nil {
		return fmt.Errorf("node %d: %w", node.ID, err)
	}
	if err := validateResultCoordinate("node y", node.TopLeft.Y); err != nil {
		return fmt.Errorf("node %d: %w", node.ID, err)
	}
	if err := validateResultDimension("node width", node.Width, false); err != nil {
		return fmt.Errorf("node %d: %w", node.ID, err)
	}
	if err := validateResultDimension("node height", node.Height, false); err != nil {
		return fmt.Errorf("node %d: %w", node.ID, err)
	}
	if !isFiniteResultNumber(node.TopLeft.X+node.Width) || !isFiniteResultNumber(node.TopLeft.Y+node.Height) {
		return fmt.Errorf("node %d bounds overflow", node.ID)
	}
	if node.Label != nil {
		if err := validateResultDimension("node label width", node.Label.Width, true); err != nil {
			return fmt.Errorf("node %d: %w", node.ID, err)
		}
		if err := validateResultDimension("node label height", node.Label.Height, true); err != nil {
			return fmt.Errorf("node %d: %w", node.ID, err)
		}
	}
	return nil
}

func validateResultEdge(ctx context.Context, edge *layoutgraph.Edge) error {
	if edge == nil {
		return fmt.Errorf("edge is nil")
	}
	if len(edge.Points) < 2 {
		return fmt.Errorf("edge %d has an incomplete route", edge.ID)
	}
	for index, point := range edge.Points {
		if index%256 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if point == nil {
			return fmt.Errorf("edge %d route point %d is nil", edge.ID, index)
		}
		if err := validateResultCoordinate("route x", point.X); err != nil {
			return fmt.Errorf("edge %d point %d: %w", edge.ID, index, err)
		}
		if err := validateResultCoordinate("route y", point.Y); err != nil {
			return fmt.Errorf("edge %d point %d: %w", edge.ID, index, err)
		}
	}
	if !isFiniteResultNumber(edge.LabelPercentage) || edge.LabelPercentage < 0 || edge.LabelPercentage > 1 {
		return fmt.Errorf("edge %d has invalid label percentage %v", edge.ID, edge.LabelPercentage)
	}
	labels := []struct {
		name  string
		label *layoutgraph.Label
	}{
		{name: "label", label: edge.Label},
		{name: "source arrowhead label", label: edge.SourceArrowheadLabel},
		{name: "target arrowhead label", label: edge.TargetArrowheadLabel},
	}
	for _, item := range labels {
		if item.label == nil {
			continue
		}
		if err := validateResultDimension(item.name+" width", item.label.Width, true); err != nil {
			return fmt.Errorf("edge %d: %w", edge.ID, err)
		}
		if err := validateResultDimension(item.name+" height", item.label.Height, true); err != nil {
			return fmt.Errorf("edge %d: %w", edge.ID, err)
		}
	}
	return nil
}

func validateResultCoordinate(name string, value float64) error {
	if !isFiniteResultNumber(value) || math.Abs(value) > maxResultCoordinate {
		return fmt.Errorf("%s is outside the finite supported range: %v", name, value)
	}
	return nil
}

func validateResultDimension(name string, value float64, zeroAllowed bool) error {
	if !isFiniteResultNumber(value) || value < 0 || (!zeroAllowed && value == 0) || value > maxResultCoordinate {
		return fmt.Errorf("%s is outside the supported range: %v", name, value)
	}
	return nil
}

func isFiniteResultNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
