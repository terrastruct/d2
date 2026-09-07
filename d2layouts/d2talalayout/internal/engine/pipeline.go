package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/hierarchy"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/labeling"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/packing"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placement"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/proximity"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/routing"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/trees"
)

const maxGraphSize = limits.MaxGraphSize

// LayoutOptions configures one deterministic layout attempt.
type LayoutOptions struct {
	Seed int64
}

type pipelineStage struct {
	name string
	run  func(*pipeline, context.Context) error
}

type routingSnapshot struct {
	ovg   *routing.OVG
	graph *layoutgraph.Graph
}

func newRoutingSnapshot(ctx context.Context, ovg *routing.OVG, g *layoutgraph.Graph) (*routingSnapshot, error) {
	// Clone the OVG and graph at the same point in the routing pipeline so tests
	// and developer tooling can render a consistent snapshot.
	ovgCopy := routing.NewOVG(nil)
	ovgJSON, err := json.Marshal(ovg)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(ovgJSON, ovgCopy); err != nil {
		return nil, err
	}
	graphCopy, err := layoutgraph.Clone(ctx, g)
	if err != nil {
		return nil, err
	}
	return &routingSnapshot{
		ovg:   ovgCopy,
		graph: graphCopy,
	}, nil
}

type pipeline struct {
	graph                    *layoutgraph.Graph
	seed                     int64
	random                   *rand.Rand
	hierarchyPlacementRandom *rand.Rand
	stages                   []pipelineStage
	storeSnapshots           bool
	snapshots                []*routingSnapshot
	stageDurations           []time.Duration
	forceReroute             bool
	edgeRoutingComplete      bool
	alignAxesNeeded          bool
}

// defaultPipelineStages is immutable after initialization. Tests that wrap or
// replace stages must first install a private stages override on their pipeline.
var defaultPipelineStages = [...]pipelineStage{
	{name: "Prescale", run: (*pipeline).prescaleStage},
	{name: "PreprocessSequences", run: (*pipeline).preprocessSequenceStage},
	{name: "Preprocess", run: (*pipeline).preprocessStage},
	{name: "PreprocessTrees", run: (*pipeline).preprocessTreesStage},
	{name: "PreprocessHierarchies", run: (*pipeline).preprocessHierarchies},
	{name: "PreprocessClusters", run: (*pipeline).preprocessClusters},
	{name: "PreprocessHubs", run: (*pipeline).preprocessHubs},
	{name: "NodePlacement", run: (*pipeline).nodePlacementStage},
	{name: "SwapStuff", run: (*pipeline).swapNodesStage},
	{name: "Transpose", run: (*pipeline).transposeStage},
	{name: "AlignAxes", run: (*pipeline).alignAxes},
	{name: "GapNormalization", run: (*pipeline).gapNormalizationStage},
	{name: "AlignAxes", run: (*pipeline).alignAxes},
	{name: "OptimizeClusters", run: (*pipeline).optimizeClustersStage},
	{name: "AlignAxes", run: (*pipeline).alignAxes},
	{name: "BalanceSymmetry", run: (*pipeline).balanceSymmetryStage},
	{name: "Equidistance", run: (*pipeline).equidistanceStage},
	{name: "AlignAxes", run: (*pipeline).alignAxes},
	{name: "BinPack", run: (*pipeline).binPack},
	{name: "CleanupStuff", run: (*pipeline).cleanupGroupsStage},
	{name: "Rescale", run: (*pipeline).rescaleStage},
	{name: "EdgeRouting", run: (*pipeline).edgeRoutingStage},
	{name: "Crosshatch", run: (*pipeline).crosshatchStage},
	{name: "Dejitter", run: (*pipeline).dejitterStage},
	{name: "EdgeRouting", run: (*pipeline).edgeRoutingStage},
	{name: "SimplifyEdgeRoutes", run: (*pipeline).simplifyEdgeRoutes},
	{name: "SwapEdgePorts", run: (*pipeline).swapEdgePorts},
	{name: "StraightEdgesFallback", run: (*pipeline).straightEdgesFallback},
	{name: "BalanceEdgeSegments", run: (*pipeline).balanceEdgeSegments},
	{name: "FixClusterEdgeBranching", run: (*pipeline).fixClusterEdgeBranching},
	{name: "TraceEdgesToShapeBorder", run: (*pipeline).traceEdgesToShapeBorder},
	{name: "ReorderDuplicates", run: (*pipeline).reorderDuplicates},
	// Second binpack doesn't try to avoid edges.
	{name: "BinPack", run: (*pipeline).binPack},
	{name: "PlaceLabels", run: (*pipeline).placeLabels},
	{name: "NudgeEdgeChannels", run: (*pipeline).nudgeEdgeChannels},
	{name: "ShortcutEdgeRoutes", run: (*pipeline).shortcutEdgeRoutes},
	{name: "Normalize", run: (*pipeline).normalizeStage},
}

func (p *pipeline) stagePlan() []pipelineStage {
	if p.stages != nil {
		return p.stages
	}
	return defaultPipelineStages[:]
}

func (p *pipeline) enableStageTiming() {
	p.stageDurations = make([]time.Duration, len(p.stagePlan()))
}

func (p *pipeline) simplifyEdgeRoutes(ctx context.Context) error {
	return routing.SimplifyEdgeRoutes(ctx, p.graph)
}

func (p *pipeline) shortcutEdgeRoutes(ctx context.Context) error {
	return routing.ShortcutEdgeRoutes(ctx, p.graph)
}

// newPipeline initializes one deterministic layout pipeline.
func newPipeline(graph *layoutgraph.Graph, randSeed int64, storeSnapshots bool) *pipeline {
	return &pipeline{
		graph:                    graph,
		seed:                     randSeed,
		hierarchyPlacementRandom: rand.New(rand.NewSource(randSeed)),
		random:                   rand.New(rand.NewSource(randSeed)),
		storeSnapshots:           storeSnapshots,
		alignAxesNeeded:          true,
	}
}

// prescaleStage takes trivial time, so it does not poll the context itself.
func (p *pipeline) prescaleStage(ctx context.Context) error {
	placement.Prescale(p.graph)
	return nil
}

func (p *pipeline) preprocessSequenceStage(ctx context.Context) error {
	return grouping.AddSequences(ctx, p.graph, p.random)
}

func (p *pipeline) preprocessHubs(ctx context.Context) error {
	return proximity.AddHubs(ctx, p.graph)
}

func (p *pipeline) preprocessClusters(ctx context.Context) error {
	return grouping.AddClusters(ctx, p.graph, p.seed, p.random)
}

func (p *pipeline) preprocessHierarchies(ctx context.Context) error {
	if err := hierarchy.Assign(ctx, p.graph, nil, hierarchy.Candidates(p.graph)); err != nil {
		return err
	}

	if err := hierarchy.Place(ctx, p.graph, nil, p.hierarchyPlacementRandom); err != nil {
		return err
	}

	hierarchy.RemoveIsolatedMemberships(p.graph)

	return nil
}

func (p *pipeline) preprocessStage(ctx context.Context) error {
	placement.Prepare(p.graph)
	return nil
}

func (p *pipeline) preprocessTreesStage(ctx context.Context) error {
	return trees.Preprocess(ctx, p.graph)
}

func (p *pipeline) gapNormalizationStage(ctx context.Context) error {
	changed, err := placement.NormalizeGaps(ctx, p.graph)
	p.alignAxesNeeded = changed
	return err
}

func (p *pipeline) nodePlacementStage(ctx context.Context) error {
	return placement.Place(ctx, p.graph, p.seed)
}

func (p *pipeline) optimizeClustersStage(ctx context.Context) error {
	p.graph.ComputeCellSize()
	var err error
	p.alignAxesNeeded, err = placement.OptimizeClusters(ctx, p.graph)
	return err
}

func (p *pipeline) balanceSymmetryStage(ctx context.Context) error {
	return placement.BalanceSymmetry(ctx, p.graph)
}

func (p *pipeline) binPack(ctx context.Context) error {
	return packing.Pack(ctx, p.graph, nil)
}

func (p *pipeline) swapNodesStage(ctx context.Context) error {
	return placement.Swap(ctx, p.graph)
}

func (p *pipeline) transposeStage(ctx context.Context) error {
	return placement.TransposeAll(ctx, p.graph)
}

func (p *pipeline) cleanupGroupsStage(ctx context.Context) error {
	grouping.Cleanup(p.graph)
	return nil
}

func (p *pipeline) rescaleStage(ctx context.Context) error {
	p.graph.ComputeCellSize()
	placement.Pad(p.graph)
	return nil
}

func (p *pipeline) edgeRoutingStage(ctx context.Context) error {
	return p.runGraphRouting(ctx, nil)
}

type routingSnapshotObserver struct {
	pipeline *pipeline
	ctx      context.Context
	started  bool
}

func (observer *routingSnapshotObserver) SubgraphRouted(ovg *routing.OVG) error {
	if !observer.started {
		observer.pipeline.snapshots = nil
		observer.started = true
	}
	snapshot, err := newRoutingSnapshot(observer.ctx, ovg, observer.pipeline.graph)
	if err != nil {
		return fmt.Errorf("copy edge-routing snapshot: %w", err)
	}
	observer.pipeline.snapshots = append(observer.pipeline.snapshots, snapshot)
	return nil
}

func (p *pipeline) RoutingCompleted() {
	p.edgeRoutingComplete = true
}

func (p *pipeline) SubgraphRouted(_ *routing.OVG) error {
	p.snapshots = nil
	return nil
}

func (p *pipeline) routeObserver(ctx context.Context) routing.SubgraphRouteObserver {
	if !p.storeSnapshots {
		return p
	}
	return &routingSnapshotObserver{pipeline: p, ctx: ctx}
}

func (p *pipeline) runGraphRouting(ctx context.Context, workLimit *uint64) (err error) {
	if p == nil || p.graph == nil {
		return fmt.Errorf("TALA EdgeRouting requires a graph")
	}
	originalSnapshots := p.snapshots
	originalRoutingComplete := p.edgeRoutingComplete
	succeeded := false
	defer func() {
		if recovered := recover(); recovered != nil {
			p.snapshots = originalSnapshots
			p.edgeRoutingComplete = originalRoutingComplete
			panic(recovered)
		}
		if !succeeded {
			p.snapshots = originalSnapshots
			p.edgeRoutingComplete = originalRoutingComplete
		}
	}()

	options := routing.GraphRouteOptions{
		ForceReroute:              p.forceReroute,
		RoutesPreviouslyCompleted: p.edgeRoutingComplete,
		Observer:                  p.routeObserver(ctx),
		CompletionObserver:        p,
	}
	var routingComplete bool
	if workLimit == nil {
		routingComplete, err = routing.RouteGraph(ctx, p.graph, options)
	} else {
		routingComplete, err = routing.RouteGraphWithWorkLimit(ctx, p.graph, options, *workLimit)
	}
	if err != nil {
		return err
	}
	p.edgeRoutingComplete = routingComplete
	succeeded = true
	return nil
}

// straightEdgesFallback replaces expensive orthogonal routes with suitable
// straight routes after the normal routing attempts have completed.
func (p *pipeline) straightEdgesFallback(ctx context.Context) error {
	return routing.StraightEdgesFallback(ctx, p.graph)
}

func (p *pipeline) traceEdgesToShapeBorder(ctx context.Context) error {
	return routing.TraceEdgesToShapeBorder(ctx, p.graph)
}

func (p *pipeline) balanceEdgeSegments(ctx context.Context) error {
	return routing.BalanceEdgeSegments(ctx, p.graph)
}

func (p *pipeline) nudgeEdgeChannels(ctx context.Context) error {
	return routing.NudgeEdgeChannels(ctx, p.graph)
}

func (p *pipeline) fixClusterEdgeBranching(ctx context.Context) error {
	return routing.FixClusterEdgeBranching(ctx, p.graph)
}

func (p *pipeline) dejitterStage(ctx context.Context) error {
	var err error
	p.forceReroute, err = placement.Dejitter(ctx, p.graph)
	if err != nil {
		return err
	}

	// TODO account for nearby nodes and cascading nested-container expansion
	// before expanding containers at this stage.

	return nil
}

func (p *pipeline) equidistanceStage(ctx context.Context) error {
	p.graph.ComputeCellSize()
	var err error
	p.alignAxesNeeded, err = placement.Equidistance(ctx, p.graph)
	return err
}

func (p *pipeline) alignAxes(ctx context.Context) error {
	if !p.alignAxesNeeded {
		return nil
	}
	p.alignAxesNeeded = false
	return placement.Align(ctx, p.graph)
}

func (p *pipeline) normalizeStage(ctx context.Context) error {
	placement.Normalize(p.graph)
	return nil
}

func (p *pipeline) reorderDuplicates(ctx context.Context) error {
	return routing.ReorderDuplicates(ctx, p.graph)
}

func (p *pipeline) placeLabels(ctx context.Context) error {
	return labeling.Place(ctx, p.graph)
}

func (p *pipeline) swapEdgePorts(ctx context.Context) error {
	return routing.SwapAllEdgePorts(ctx, p.graph)
}

func (p *pipeline) crosshatchStage(ctx context.Context) error {
	return routing.Crosshatch(ctx, p.graph)
}

func (p *pipeline) resetFullLayoutRouteState() {
	for _, edge := range p.graph.Edges {
		if edge == nil {
			continue
		}
		edge.Points = nil
		if edge.Label == nil || !edge.Label.PositionFixed() {
			edge.LabelPercentage = 0
			if edge.Label != nil && edge.Label.Position.IsUnlocked() {
				edge.Label.Position = label.Unset
			}
		}
		edge.IsCurve = false
	}
	p.graph.ResetPlacementCosts()
}

// runAllStages executes the layout stages in order against the pipeline graph.
func (p *pipeline) runAllStages(ctx context.Context) error {
	ctx, _, err := layoutgraph.EnsureTransactionWorkGuard(ctx, "AutolayoutTransactions")
	if err != nil {
		return err
	}
	stages := p.stagePlan()
	timed := p.stageDurations != nil
	if timed && len(p.stageDurations) != len(stages) {
		p.stageDurations = make([]time.Duration, len(stages))
	}
	for stageIndex := range stages {
		stage := &stages[stageIndex]
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w", stage.name, err)
		}
		if stageIndex == 0 {
			grouping.ResetClusters(p.graph)
			// A full layout owns geometry, routing, and route-derived presentation.
			// An incoming route cannot remain authoritative once node placement, obstacle
			// sizing, and padding may change its coordinate space. Invalidate its
			// auto-placed label and cost state too, while retaining explicit locked
			// label positions. None of that stale state should influence the first,
			// pre-routing BinPack or the new route search.
			p.resetFullLayoutRouteState()
		}
		var start time.Time
		if timed {
			start = time.Now()
		}
		if err := stage.run(p, ctx); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w", stage.name, err)
		}
		if timed {
			p.stageDurations[stageIndex] = time.Since(start)
		}

		// Only check overflow if nodes have been placed
		tl, br := p.graph.BoundingBox()
		if tl != nil && br != nil {
			width := br.X - tl.X
			height := br.Y - tl.Y
			if (width > maxGraphSize) || (height > maxGraphSize) {
				return invariant.Errorf("Dimensions w:%v, h:%v reached after stage %v", width, height, stage.name)
			}
		}
	}
	return nil
}

// Layout returns one complete deterministic layout in a new graph. Input is
// validated and remains unchanged, so callers may safely reuse it for other
// layout attempts.
func Layout(ctx context.Context, graph *layoutgraph.Graph, options LayoutOptions) (*layoutgraph.Graph, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA Autolayout requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Autolayout: %w", err)
	}
	if graph == nil {
		return nil, fmt.Errorf("TALA Autolayout requires a graph")
	}
	workspace, err := layoutgraph.Clone(ctx, graph)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, fmt.Errorf("Autolayout: %w", contextErr)
		}
		return nil, err
	}
	if _, err := runLayout(ctx, workspace, options, pipelineInstrumentation{}, false); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Autolayout: %w", err)
	}
	return workspace, nil
}

type pipelineInstrumentation struct {
	storeSnapshots        bool
	measureStageDurations bool
}

func runLayout(ctx context.Context, graph *layoutgraph.Graph, options LayoutOptions, instrumentation pipelineInstrumentation, validate bool) (*pipeline, error) {
	if ctx == nil {
		return nil, fmt.Errorf("TALA Autolayout requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Autolayout: %w", err)
	}
	if graph == nil {
		return nil, fmt.Errorf("TALA Autolayout requires a graph")
	}
	if validate {
		if err := layoutgraph.Validate(ctx, "Autolayout", graph); err != nil {
			return nil, err
		}
	}
	if len(graph.Nodes) == 0 {
		return nil, nil
	}
	workCtx := ctx
	cancelWork := func() {}
	if ctx.Done() == nil {
		workCtx, cancelWork = context.WithCancel(ctx)
	}
	defer cancelWork()
	pipeline := newPipeline(graph, options.Seed, instrumentation.storeSnapshots)
	if instrumentation.measureStageDurations {
		pipeline.enableStageTiming()
	}
	return pipeline, pipeline.runAllStages(workCtx)
}
