package engine

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func canceledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

type cancelAfterErrChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelAfterErrChecks) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
}

func requireCanceledAt(t *testing.T, err error, location string) {
	t.Helper()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(%v, context.Canceled) = false", err)
	}
	if !strings.Contains(err.Error(), location) {
		t.Fatalf("cancellation error = %v, want operation %q", err, location)
	}
}

func TestAutolayoutCanceledBeforeWork(t *testing.T) {
	_, err := layoutWithSnapshots(canceledContext(), layoutgraph.NewGraph(), 1, false)
	requireCanceledAt(t, err, "Autolayout")
}

func TestLayoutCancellationBeforeCommit(t *testing.T) {
	graph := layoutgraph.NewGraph()
	before, err := graphjson.Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterErrChecks{Context: context.Background(), remaining: 2}
	result, err := Layout(ctx, graph, LayoutOptions{Seed: 1})
	requireCanceledAt(t, err, "Autolayout")
	if result != nil {
		t.Fatal("Layout returned a partial result after cancellation")
	}
	after, err := graphjson.Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("Layout changed the graph after cancellation")
	}
}

func TestLayoutCancellationAfterRouteInvalidationKeepsInput(t *testing.T) {
	graph := layoutgraph.NewGraph()
	from := layoutgraph.NewNode(1, 40, 40)
	from.TopLeft = geo.NewPoint(0, 0)
	from.FixedTopLeft = from.TopLeft.Copy()
	to := layoutgraph.NewNode(2, 40, 40)
	to.TopLeft = geo.NewPoint(200, 0)
	to.FixedTopLeft = to.TopLeft.Copy()
	graph.AddNewNodeToContainer(nil, from)
	graph.AddNewNodeToContainer(nil, to)
	graph.Connect(from, to).Points = []*geo.Point{
		geo.NewPoint(40, 20),
		geo.NewPoint(200, 20),
	}

	before, err := graphjson.Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &cancelAfterRunAllStagesRouteInvalidation{Context: context.Background(), done: make(chan struct{})}
	result, err := Layout(ctx, graph, LayoutOptions{Seed: 1})
	requireCanceledAt(t, err, "PreprocessSequences")
	if result != nil {
		t.Fatal("Layout returned a partial result after route invalidation")
	}
	if ctx.runAllStagesChecks != 3 {
		t.Fatalf("runAllStages context checks = %d, want cancellation after the post-Prescale check", ctx.runAllStagesChecks)
	}
	after, err := graphjson.Serialize(t.Context(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("Layout mutated its input after cancellation following route invalidation")
	}
}

type cancelAfterRunAllStagesRouteInvalidation struct {
	context.Context
	runAllStagesChecks int
	done               <-chan struct{}
}

func (ctx *cancelAfterRunAllStagesRouteInvalidation) Done() <-chan struct{} { return ctx.done }

func (ctx *cancelAfterRunAllStagesRouteInvalidation) Err() error {
	callers := make([]uintptr, 16)
	count := runtime.Callers(2, callers)
	frames := runtime.CallersFrames(callers[:count])
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".(*pipeline).runAllStages") {
			ctx.runAllStagesChecks++
			if ctx.runAllStagesChecks >= 3 {
				return context.Canceled
			}
			break
		}
		if !more {
			break
		}
	}
	return ctx.Context.Err()
}

func TestOptimizationStagesCanceledBeforeWork(t *testing.T) {
	tests := []struct {
		name string
		run  func(*pipeline, context.Context) error
	}{
		{name: "gap normalization", run: (*pipeline).gapNormalizationStage},
		{name: "optimize clusters", run: (*pipeline).optimizeClustersStage},
		{name: "balance symmetry", run: (*pipeline).balanceSymmetryStage},
		{name: "swap", run: (*pipeline).swapNodesStage},
		{name: "transpose", run: (*pipeline).transposeStage},
		{name: "dejitter", run: (*pipeline).dejitterStage},
		{name: "equidistance", run: (*pipeline).equidistanceStage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run(&pipeline{graph: layoutgraph.NewGraph()}, canceledContext())
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("stage error = %v, want context.Canceled", err)
			}
		})
	}
}
