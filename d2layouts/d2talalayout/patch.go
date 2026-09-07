package d2talalayout

import (
	"context"
	"fmt"
	"strconv"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

// d2Patch is a fully validated update to a D2 graph. apply performs only
// non-failing assignments, preserving the graph, object, and edge identities
// held by callers.
type d2Patch struct {
	graph        *d2graph.Graph
	objects      []d2ObjectPatch
	edges        []d2EdgePatch
	destinations []d2EdgeDestinationPatch
	replaceEdges bool
	finalEdges   []*d2graph.Edge
}

type d2ObjectPatch struct {
	object  *d2graph.Object
	box     *geo.Box
	topLeft *geo.Point
	width   float64
	height  float64

	label *d2ObjectLabelPatch
	icon  *string
	font  *d2FontSizePatch
}

type d2ObjectLabelPatch struct {
	position   *string
	dimensions d2target.TextDimensions
}

type d2FontSizePatch struct {
	existing    *d2graph.Scalar
	replacement *d2graph.Scalar
	value       string
}

type d2EdgePatch struct {
	edge *d2graph.Edge

	label *d2EdgeLabelPatch
	route []*geo.Point
	curve bool
}

type d2EdgeDestinationPatch struct {
	edge        *d2graph.Edge
	destination *d2graph.Object
}

type d2EdgeLabelPatch struct {
	position   *string
	percentage *float64
	dimensions *d2target.TextDimensions
}

func preferContextError(ctx context.Context, operation string, err error) error {
	if cause := ctx.Err(); cause != nil {
		return fmt.Errorf("tala %s stopped: %w", operation, cause)
	}
	return err
}

// applySeedResult applies one validated result through a non-failing patch.
// Cancellation before the commit leaves graph unchanged.
func applySeedResult(ctx context.Context, graph *d2graph.Graph, result seedResult) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("TALA layout canceled before apply: %w", err)
	}
	if result.graph == nil {
		return fmt.Errorf("TALA evaluated seed is empty")
	}
	if result.bindings.objectIDs == nil || result.bindings.edgeIDs == nil || result.bindings.edgeDestinations == nil {
		return fmt.Errorf("TALA evaluated seed has no D2 bindings")
	}
	patch, err := buildLayoutPatch(ctx, graph, result.graph, result.bindings, result.sequenceEdges)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("TALA layout canceled before apply: %w", err)
	}
	return commitLayoutPatch(ctx, patch)
}

// commitLayoutPatch is the layout commit point. A caller that deliberately
// salvages a completed result after a deadline must supply a live context to
// applySeedResult.
func commitLayoutPatch(ctx context.Context, patch d2Patch) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tala layout stopped before commit: %w", err)
	}
	patch.apply()
	return nil
}

// commitRoutePatch is the routing commit point. Routing does not salvage work
// after either form of context cancellation.
func commitRoutePatch(ctx context.Context, patch d2Patch) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("tala edge routing canceled before commit: %w", err)
	}
	patch.apply()
	return nil
}

func (patch d2Patch) apply() {
	for _, update := range patch.objects {
		update.box.TopLeft = update.topLeft
		update.box.Width = update.width
		update.box.Height = update.height
		if update.label != nil {
			update.object.LabelPosition = update.label.position
			update.object.LabelDimensions = update.label.dimensions
		}
		if update.icon != nil {
			update.object.IconPosition = update.icon
		}
		if update.font != nil {
			if update.font.existing != nil {
				update.font.existing.Value = update.font.value
			} else {
				update.object.Style.FontSize = update.font.replacement
			}
		}
	}
	for _, update := range patch.edges {
		if update.label != nil {
			update.edge.LabelPosition = update.label.position
			update.edge.LabelPercentage = update.label.percentage
			if update.label.dimensions != nil {
				update.edge.LabelDimensions = *update.label.dimensions
			}
		}
		update.edge.Route = update.route
		update.edge.IsCurve = update.curve
	}
	for _, update := range patch.destinations {
		update.edge.Dst = update.destination
	}
	if patch.replaceEdges {
		patch.graph.Edges = patch.finalEdges
	}
}

func buildLayoutPatch(ctx context.Context, g *d2graph.Graph, result *layoutgraph.Graph, bindings translation, sequenceEdges map[layoutgraph.EntityID]struct{}) (d2Patch, error) {
	patch := d2Patch{
		graph:        g,
		replaceEdges: true,
		objects:      make([]d2ObjectPatch, 0, len(g.Objects)),
		edges:        make([]d2EdgePatch, 0, len(g.Edges)),
	}
	patch.destinations = buildDestinationPatches(g, bindings, nil)

	nodes := make(map[layoutgraph.EntityID]*layoutgraph.Node, len(result.Nodes))
	for _, node := range result.Nodes {
		if err := ctx.Err(); err != nil {
			return d2Patch{}, err
		}
		if node == nil {
			return d2Patch{}, fmt.Errorf("tala result contains a nil node")
		}
		if node.TopLeft == nil {
			return d2Patch{}, fmt.Errorf("node %s reconnected with no position", node.DebugID())
		}
		if _, exists := nodes[node.ID]; exists {
			return d2Patch{}, fmt.Errorf("tala result contains duplicate node ID %d", node.ID)
		}
		nodes[node.ID] = node
	}
	for _, object := range g.Objects {
		if err := ctx.Err(); err != nil {
			return d2Patch{}, err
		}
		id, ok := bindings.objectIDs[object]
		if !ok {
			return d2Patch{}, fmt.Errorf("could not reassociate graph object %q", object.AbsID())
		}
		node, ok := nodes[id]
		if !ok {
			return d2Patch{}, fmt.Errorf("could not reassociate graph node %d, object ID: %s", id, object.AbsID())
		}
		objectPatch, err := buildObjectPatch(ctx, object, node)
		if err != nil {
			return d2Patch{}, err
		}
		patch.objects = append(patch.objects, objectPatch)
	}

	edges := make(map[layoutgraph.EntityID]*layoutgraph.Edge, len(result.Edges))
	for _, edge := range result.Edges {
		if err := ctx.Err(); err != nil {
			return d2Patch{}, err
		}
		if edge == nil {
			return d2Patch{}, fmt.Errorf("tala result contains a nil edge")
		}
		if _, exists := edges[edge.ID]; exists {
			return d2Patch{}, fmt.Errorf("tala result contains duplicate edge ID %d", edge.ID)
		}
		edges[edge.ID] = edge
	}
	for _, edge := range g.Edges {
		if err := ctx.Err(); err != nil {
			return d2Patch{}, err
		}
		id, ok := bindings.edgeIDs[edge]
		if !ok {
			return d2Patch{}, fmt.Errorf("could not reassociate D2 edge %q", edge.AbsID())
		}
		if _, consumed := sequenceEdges[id]; consumed {
			continue
		}
		resultEdge, ok := edges[id]
		if !ok {
			return d2Patch{}, fmt.Errorf("could not reassociate graph edge %d", id)
		}
		edgePatch, err := buildEdgePatch(ctx, edge, resultEdge, true)
		if err != nil {
			return d2Patch{}, err
		}
		patch.edges = append(patch.edges, edgePatch)
		patch.finalEdges = append(patch.finalEdges, edge)
	}

	return patch, nil
}

func buildRoutePatch(ctx context.Context, g *d2graph.Graph, bindings translation, routed []*layoutgraph.Edge) (d2Patch, error) {
	patch := d2Patch{
		graph:        g,
		edges:        make([]d2EdgePatch, 0, len(routed)),
		replaceEdges: true,
	}
	updates := make(map[layoutgraph.EntityID]*layoutgraph.Edge, len(routed))
	addUpdates := func(edges []*layoutgraph.Edge) error {
		for _, edge := range edges {
			if err := ctx.Err(); err != nil {
				return err
			}
			if edge == nil {
				return fmt.Errorf("tala route result contains a nil edge")
			}
			if _, exists := updates[edge.ID]; exists {
				return fmt.Errorf("tala route result contains duplicate edge ID %d", edge.ID)
			}
			updates[edge.ID] = edge
		}
		return nil
	}
	if err := addUpdates(routed); err != nil {
		return d2Patch{}, err
	}
	selectedIDs := make(map[layoutgraph.EntityID]struct{}, len(updates))
	for id := range updates {
		selectedIDs[id] = struct{}{}
	}
	patch.destinations = buildDestinationPatches(g, bindings, selectedIDs)

	matched := 0
	for _, edge := range g.Edges {
		if err := ctx.Err(); err != nil {
			return d2Patch{}, err
		}
		patch.finalEdges = append(patch.finalEdges, edge)
		id, ok := bindings.edgeIDs[edge]
		if !ok {
			return d2Patch{}, fmt.Errorf("could not reassociate D2 edge %q", edge.AbsID())
		}
		resultEdge, ok := updates[id]
		if !ok {
			continue
		}
		edgePatch, err := buildEdgePatch(ctx, edge, resultEdge, false)
		if err != nil {
			return d2Patch{}, err
		}
		patch.edges = append(patch.edges, edgePatch)
		matched++
	}
	if matched != len(updates) {
		return d2Patch{}, fmt.Errorf("could not reassociate %d routed edges", len(updates)-matched)
	}
	return patch, nil
}

func buildDestinationPatches(
	g *d2graph.Graph,
	bindings translation,
	selectedIDs map[layoutgraph.EntityID]struct{},
) []d2EdgeDestinationPatch {
	patches := make([]d2EdgeDestinationPatch, 0, len(bindings.edgeDestinations))
	for _, edge := range g.Edges {
		if selectedIDs != nil {
			id, bound := bindings.edgeIDs[edge]
			if !bound {
				continue
			}
			if _, selected := selectedIDs[id]; !selected {
				continue
			}
		}
		destination, ok := bindings.edgeDestinations[edge]
		if !ok {
			continue
		}
		patches = append(patches, d2EdgeDestinationPatch{
			edge:        edge,
			destination: destination,
		})
	}
	return patches
}

func buildObjectPatch(ctx context.Context, object *d2graph.Object, node *layoutgraph.Node) (d2ObjectPatch, error) {
	if err := ctx.Err(); err != nil {
		return d2ObjectPatch{}, err
	}
	if object.Box == nil {
		return d2ObjectPatch{}, fmt.Errorf("D2 object %q has no box", object.AbsID())
	}
	if node.TopLeft == nil {
		return d2ObjectPatch{}, fmt.Errorf("node %s reconnected with no position", node.DebugID())
	}
	if err := validateResultNode(node); err != nil {
		return d2ObjectPatch{}, err
	}
	patch := d2ObjectPatch{
		object:  object,
		box:     object.Box,
		topLeft: node.TopLeft.Copy(),
		width:   node.Width,
		height:  node.Height,
	}
	if node.Label != nil {
		patch.label = &d2ObjectLabelPatch{
			position: new(node.Label.Position.String()),
			dimensions: d2target.TextDimensions{
				Width:  int(node.Label.Width),
				Height: int(node.Label.Height),
			},
		}
	}
	if node.Icon != nil {
		patch.icon = new(node.Icon.Position.String())
	}
	if node.FontSize != nil {
		font := &d2FontSizePatch{
			existing: object.Style.FontSize,
			value:    strconv.Itoa(*node.FontSize),
		}
		if font.existing == nil {
			font.replacement = &d2graph.Scalar{Value: font.value}
		}
		patch.font = font
	}
	return patch, nil
}

func buildEdgePatch(ctx context.Context, edge *d2graph.Edge, result *layoutgraph.Edge, includeLabelDimensions bool) (d2EdgePatch, error) {
	if err := validateResultEdge(ctx, result); err != nil {
		return d2EdgePatch{}, err
	}
	patch := d2EdgePatch{
		edge:  edge,
		route: make([]*geo.Point, len(result.Points)),
		curve: result.IsCurve,
	}
	if result.Label != nil {
		position := result.Label.Position
		percentage := result.LabelPercentage
		if edge.SrcArrow && !edge.DstArrow {
			position = position.Mirrored()
			percentage = 1 - percentage
		}
		patch.label = &d2EdgeLabelPatch{
			position:   new(position.String()),
			percentage: new(percentage),
		}
		if includeLabelDimensions {
			patch.label.dimensions = &d2target.TextDimensions{
				Width:  int(result.Label.Width),
				Height: int(result.Label.Height),
			}
		}
	}
	for i := range result.Points {
		if i%256 == 0 {
			if err := ctx.Err(); err != nil {
				return d2EdgePatch{}, err
			}
		}
		if edge.SrcArrow && !edge.DstArrow {
			patch.route[len(result.Points)-i-1] = result.Points[i].Copy()
		} else {
			patch.route[i] = result.Points[i].Copy()
		}
	}
	return patch, nil
}
