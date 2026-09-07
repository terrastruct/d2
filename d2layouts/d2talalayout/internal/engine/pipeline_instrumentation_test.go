package engine

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

var benchmarkPipelineSink *pipeline

func newInstrumentationTestPipeline(stages []pipelineStage) *pipeline {
	graph := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 1, 1)
	node.TopLeft = geo.NewPoint(0, 0)
	graph.AddNode(node)
	return &pipeline{graph: graph, stages: stages}
}

func TestDefaultPipelineStagePlanIsSharedAndOverridesAreIsolated(t *testing.T) {
	first := newPipeline(layoutgraph.NewGraph(), 1, false)
	second := newPipeline(layoutgraph.NewGraph(), 1, false)
	firstPlan := first.stagePlan()
	secondPlan := second.stagePlan()
	if len(firstPlan) == 0 || len(firstPlan) != len(secondPlan) {
		t.Fatalf("default stage counts = %d and %d", len(firstPlan), len(secondPlan))
	}
	if &firstPlan[0] != &secondPlan[0] {
		t.Fatal("default pipelines rebuilt private stage descriptors")
	}
	if first.stageDurations != nil || second.stageDurations != nil {
		t.Fatal("default pipeline enabled stage timing")
	}
	wantNames := []string{
		"Prescale",
		"PreprocessSequences",
		"Preprocess",
		"PreprocessTrees",
		"PreprocessHierarchies",
		"PreprocessClusters",
		"PreprocessHubs",
		"NodePlacement",
		"SwapStuff",
		"Transpose",
		"AlignAxes",
		"GapNormalization",
		"AlignAxes",
		"OptimizeClusters",
		"AlignAxes",
		"BalanceSymmetry",
		"Equidistance",
		"AlignAxes",
		"BinPack",
		"CleanupStuff",
		"Rescale",
		"EdgeRouting",
		"Crosshatch",
		"Dejitter",
		"EdgeRouting",
		"SimplifyEdgeRoutes",
		"SwapEdgePorts",
		"StraightEdgesFallback",
		"BalanceEdgeSegments",
		"FixClusterEdgeBranching",
		"TraceEdgesToShapeBorder",
		"ReorderDuplicates",
		"BinPack",
		"PlaceLabels",
		"NudgeEdgeChannels",
		"ShortcutEdgeRoutes",
		"Normalize",
	}
	gotNames := make([]string, len(firstPlan))
	for index, stage := range firstPlan {
		gotNames[index] = stage.name
	}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("default stage order = %v, want %v", gotNames, wantNames)
	}

	first.stages = append([]pipelineStage(nil), firstPlan...)
	first.stages[0].name = "test override"
	if second.stagePlan()[0].name != "Prescale" {
		t.Fatal("private stage override mutated the shared default plan")
	}
	emptyOverride := &pipeline{stages: []pipelineStage{}}
	if plan := emptyOverride.stagePlan(); plan == nil || len(plan) != 0 {
		t.Fatalf("non-nil empty stage override resolved to %#v", plan)
	}
	if err := emptyOverride.runAllStages(t.Context()); err != nil {
		t.Fatalf("non-nil empty stage override ran default stages: %v", err)
	}
}

func TestPipelineStageTimingIsOptInAndPostCancellation(t *testing.T) {
	stage := pipelineStage{
		name: "timed",
		run: func(*pipeline, context.Context) error {
			time.Sleep(time.Millisecond)
			return nil
		},
	}
	untimed := newInstrumentationTestPipeline([]pipelineStage{stage})
	if err := untimed.runAllStages(t.Context()); err != nil {
		t.Fatal(err)
	}
	if untimed.stageDurations != nil {
		t.Fatal("ordinary pipeline recorded stage timing")
	}

	timed := newInstrumentationTestPipeline([]pipelineStage{stage})
	timed.enableStageTiming()
	if err := timed.runAllStages(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(timed.stageDurations) != 1 || timed.stageDurations[0] <= 0 {
		t.Fatalf("timed stage durations = %v", timed.stageDurations)
	}

	ctx, cancel := context.WithCancel(t.Context())
	canceled := newInstrumentationTestPipeline([]pipelineStage{{
		name: "cancel",
		run: func(*pipeline, context.Context) error {
			cancel()
			return nil
		},
	}})
	canceled.enableStageTiming()
	if err := canceled.runAllStages(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled pipeline error = %v, want context.Canceled", err)
	}
	if canceled.stageDurations[0] != 0 {
		t.Fatalf("canceled stage duration = %v, want no post-check timing", canceled.stageDurations[0])
	}
}

func TestPipelineRouteSnapshotObserverIsOptIn(t *testing.T) {
	pipeline := newPipeline(layoutgraph.NewGraph(), 1, false)
	ctx := t.Context()
	if observer := pipeline.routeObserver(ctx); observer != pipeline {
		t.Fatalf("disabled snapshot observer = %#v, want allocation-free pipeline observer", observer)
	}
	var observer any
	if allocations := testing.AllocsPerRun(1000, func() {
		observer = pipeline.routeObserver(ctx)
	}); allocations != 0 {
		t.Fatalf("disabled snapshot-observer allocations = %v, want 0", allocations)
	}
	if observer != pipeline {
		t.Fatalf("disabled snapshot observer after allocation probe = %#v, want pipeline", observer)
	}
	pipeline.storeSnapshots = true
	if _, ok := pipeline.routeObserver(ctx).(*routingSnapshotObserver); !ok {
		t.Fatal("enabled snapshot instrumentation did not install its observer")
	}
}

func BenchmarkNewPipeline(b *testing.B) {
	graph := layoutgraph.NewGraph()
	b.ReportAllocs()
	for b.Loop() {
		benchmarkPipelineSink = newPipeline(graph, 1, false)
	}
}

func BenchmarkRunPipelineStageOverhead(b *testing.B) {
	stages := make([]pipelineStage, len(newPipeline(layoutgraph.NewGraph(), 1, false).stagePlan()))
	for index := range stages {
		stages[index] = pipelineStage{
			name: "noop",
			run:  func(*pipeline, context.Context) error { return nil },
		}
	}
	pipeline := newInstrumentationTestPipeline(stages)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := pipeline.runAllStages(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
