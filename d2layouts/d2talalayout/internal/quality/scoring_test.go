package quality

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/limits"
)

func TestEvaluateKeepsAreaAsSeparateTieBreaker(t *testing.T) {
	ctx := context.Background()
	makeGraph := func(width float64) *layoutgraph.Graph {
		graph := layoutgraph.NewGraph()
		node := graph.AddNode(layoutgraph.NewNode(1, width, 1))
		node.TopLeft = geo.NewPoint(0, 0)
		return graph
	}

	score99, area99, err := EvaluateWithArea(ctx, makeGraph(99))
	if err != nil {
		t.Fatal(err)
	}
	score100, area100, err := EvaluateWithArea(ctx, makeGraph(100))
	if err != nil {
		t.Fatal(err)
	}
	if score99 != score100 {
		t.Fatalf("area changed the primary score: %v versus %v", score99, score100)
	}
	if area99 != 99 || area100 != 100 || area99 >= area100 {
		t.Fatalf("unexpected area tie-breakers: %v and %v", area99, area100)
	}
}

func TestEvaluateWithAreaPreservesLargeFractionalArea(t *testing.T) {
	for _, test := range []struct {
		name          string
		width, height float64
	}{
		{name: "fractional", width: 12.5, height: 3.25},
		{name: "large", width: 1_000_000_000, height: 1_000_000_000},
	} {
		t.Run(test.name, func(t *testing.T) {
			graph := layoutgraph.NewGraph()
			node := graph.AddNode(layoutgraph.NewNode(1, test.width, test.height))
			node.TopLeft = geo.NewPoint(0.25, 0.5)

			_, area, err := EvaluateWithArea(context.Background(), graph)
			if err != nil {
				t.Fatal(err)
			}
			want := test.width * test.height
			if area != want {
				t.Fatalf("area = %v, want %v", area, want)
			}
			if test.name == "large" && area <= float64(1<<31-1) {
				t.Fatalf("large area = %v, want greater than MaxInt32", area)
			}
		})
	}
}

func TestNonSharedCrossingCountUsesFixedWidthAccumulator(t *testing.T) {
	first := &layoutgraph.Edge{Points: []*geo.Point{geo.NewPoint(0, 0), geo.NewPoint(10, 10)}}
	second := &layoutgraph.Edge{Points: []*geo.Point{geo.NewPoint(0, 10), geo.NewPoint(10, 0)}}
	guard, err := limits.NewWorkGuard(context.Background(), "Evaluate", 100)
	if err != nil {
		t.Fatal(err)
	}

	var crossings int64
	crossings, err = countNonSharedCrossings([]*layoutgraph.Edge{first, second}, guard)
	if err != nil {
		t.Fatal(err)
	}
	if crossings != 1 {
		t.Fatalf("crossing count = %d, want 1", crossings)
	}
}

func TestEvaluateCanceledBeforeWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := EvaluateWithArea(ctx, layoutgraph.NewGraph())
	requireCanceledAt(t, err, "Evaluate")
}

func TestScoreExistingLabelPlacementsCanceledBeforeWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	guard, err := limits.NewWorkGuard(ctx, "Evaluate", 2_000)
	if err != nil {
		t.Fatal(err)
	}
	// Ordinary cancellable contexts poll every 1,024 charged units. Arrange for
	// the kernel's entry charge to be the next poll.
	if err := guard.Add(1_023); err != nil {
		t.Fatal(err)
	}
	cancel()

	_, err = scoreExistingLabelPlacements(layoutgraph.NewGraph(), guard)
	requireCanceledAt(t, err, "Evaluate")
}

func TestEvaluateWorkLimitChargesEdgesWithoutCompleteRoutes(t *testing.T) {
	for _, routePoints := range []int{0, 1} {
		t.Run(fmt.Sprintf("%d route points", routePoints), func(t *testing.T) {
			graph := layoutgraph.NewGraph()
			from := graph.AddNode(layoutgraph.NewNode(1, 10, 10))
			to := graph.AddNode(layoutgraph.NewNode(2, 10, 10))
			from.TopLeft = geo.NewPoint(0, 0)
			to.TopLeft = geo.NewPoint(20, 0)
			for range 1_000 {
				edge := graph.Connect(from, to)
				if routePoints == 1 {
					edge.Points = []*geo.Point{geo.NewPoint(10, 5)}
				}
			}

			const limit = int64(1_500)
			_, _, used, err := evaluateWithAreaLimit(context.Background(), graph, limit)
			if err == nil || !strings.Contains(err.Error(), "TALA Evaluate work exceeds limit") {
				t.Fatalf("evaluateWithAreaLimit() error = %v, want evaluation work limit", err)
			}
			if used != limit+1 {
				t.Fatalf("rejection used %d units, want %d", used, limit+1)
			}
		})
	}
}

func TestEvaluateWorkLimitIsExact(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := graph.AddNode(layoutgraph.NewNode(1, 20, 20))
	b := graph.AddNode(layoutgraph.NewNode(2, 20, 20))
	c := graph.AddNode(layoutgraph.NewNode(3, 20, 20))
	d := graph.AddNode(layoutgraph.NewNode(4, 20, 20))
	for index, node := range graph.Nodes {
		node.TopLeft = geo.NewPoint(float64(index*40), 0)
	}
	first := graph.Connect(a, c)
	first.Points = []*geo.Point{geo.NewPoint(0, 0), geo.NewPoint(100, 100)}
	second := graph.Connect(b, d)
	second.Points = []*geo.Point{geo.NewPoint(0, 100), geo.NewPoint(100, 0)}

	wantScore, wantArea, required, err := evaluateWithAreaLimit(context.Background(), graph, maxEvaluationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	if required < 2 {
		t.Fatalf("evaluation used %d work units, want a nontrivial measurement", required)
	}
	gotScore, gotArea, exact, err := evaluateWithAreaLimit(context.Background(), graph, required)
	if err != nil {
		t.Fatalf("exact measured work limit rejected: %v", err)
	}
	if exact != required || gotScore != wantScore || gotArea != wantArea {
		t.Fatalf("exact-limit result = (%v, %v, %d), want (%v, %v, %d)", gotScore, gotArea, exact, wantScore, wantArea, required)
	}
	_, _, rejectedAt, err := evaluateWithAreaLimit(context.Background(), graph, required-1)
	if err == nil || !strings.Contains(err.Error(), "TALA Evaluate work exceeds limit") {
		t.Fatalf("one-unit-short error = %v, want evaluation work limit", err)
	}
	if rejectedAt != required {
		t.Fatalf("one-unit-short rejection used %d units, want %d", rejectedAt, required)
	}
}

func TestEvaluateWorkLimitCoversScoringKernels(t *testing.T) {
	graph := layoutgraph.NewGraph()
	container := layoutgraph.NewNode(1, 240, 180)
	container.TopLeft = geo.NewPoint(-40, -40)
	graph.AddNewNodeToContainer(nil, container)
	a := layoutgraph.NewNode(2, 40, 40)
	a.TopLeft = geo.NewPoint(0, 0)
	a.Label = &layoutgraph.Label{Text: "A", Width: 20, Height: 12, Position: label.OutsideTopCenter}
	b := layoutgraph.NewNode(3, 40, 40)
	b.TopLeft = geo.NewPoint(120, 80)
	b.Label = &layoutgraph.Label{Text: "B", Width: 20, Height: 12, Position: label.InsideMiddleCenter}
	graph.AddNewNodeToContainer(container, a)
	graph.AddNewNodeToContainer(container, b)

	first := graph.Connect(a, b)
	first.Points = []*geo.Point{
		geo.NewPoint(20, 20),
		geo.NewPoint(70, 20),
		geo.NewPoint(140, 100),
	}
	first.Label = &layoutgraph.Label{Text: "first", Width: 28, Height: 12, Position: label.InsideMiddleCenter}
	first.LabelPercentage = 0.5
	first.SourceArrowheadLabel = &layoutgraph.Label{Text: "source", Width: 32, Height: 12}
	second := graph.Connect(a, b)
	second.Points = []*geo.Point{
		geo.NewPoint(20, 20),
		geo.NewPoint(70, 20),
		geo.NewPoint(140, 60),
	}
	second.Label = &layoutgraph.Label{Text: "second", Width: 36, Height: 12, Position: label.InsideMiddleCenter}
	second.LabelPercentage = 0.5

	wantScore, wantArea, required, err := evaluateWithAreaLimit(context.Background(), graph, maxEvaluationWorkUnits)
	if err != nil {
		t.Fatal(err)
	}
	if required < 100 {
		t.Fatalf("label/crossing/ancestry/shared-segment evaluation used %d work units, want representative coverage", required)
	}
	gotScore, gotArea, exact, err := evaluateWithAreaLimit(context.Background(), graph, required)
	if err != nil {
		t.Fatalf("exact representative work limit rejected: %v", err)
	}
	if exact != required || gotScore != wantScore || gotArea != wantArea {
		t.Fatalf("exact representative result = (%v, %v, %d), want (%v, %v, %d)", gotScore, gotArea, exact, wantScore, wantArea, required)
	}
	_, _, rejectedAt, err := evaluateWithAreaLimit(context.Background(), graph, required-1)
	if err == nil || !strings.Contains(err.Error(), "TALA Evaluate work exceeds limit") {
		t.Fatalf("one-unit-short representative error = %v, want evaluation work limit", err)
	}
	if rejectedAt != required {
		t.Fatalf("one-unit-short representative rejection used %d units, want %d", rejectedAt, required)
	}
}

func TestEvaluatePreflightsEngineResources(t *testing.T) {
	graph := layoutgraph.NewGraph()
	graph.Nodes = make([]*layoutgraph.Node, limits.MaxEngineNodes+1)
	for i := range graph.Nodes {
		graph.Nodes[i] = layoutgraph.NewNode(layoutgraph.EntityID(i+1), 1, 1)
		graph.Nodes[i].TopLeft = geo.NewPoint(float64(i), 0)
	}
	_, _, err := EvaluateWithArea(context.Background(), graph)
	if err == nil || !strings.Contains(err.Error(), "TALA engine unique node count exceeds limit 10000") {
		t.Fatalf("EvaluateWithArea() preflight error = %v, want engine node limit", err)
	}
}

func TestEvaluateWorkLimitCoversLayoutCorpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "testdata", "layout", "*", "graph.exp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no completed layout fixtures found")
	}

	var peak int64
	var peakFile string
	for _, file := range files {
		graph, err := readGraph(t.Context(), file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		_, _, used, err := evaluateWithAreaLimit(context.Background(), graph, maxEvaluationWorkUnits)
		if err != nil {
			t.Fatalf("evaluate %s: %v", file, err)
		}
		if used > peak {
			peak = used
			peakFile = file
		}
	}
	if peak <= 0 || peak >= maxEvaluationWorkUnits {
		t.Fatalf("completed-layout evaluation peak = %d, limit = %d", peak, maxEvaluationWorkUnits)
	}
	if peak < calibratedPublicEvaluationCorpusFloor || peak > calibratedPublicEvaluationCorpusCeil {
		t.Fatalf(
			"completed-layout evaluation peak = %d (%s), want calibrated range [%d, %d]",
			peak,
			peakFile,
			calibratedPublicEvaluationCorpusFloor,
			calibratedPublicEvaluationCorpusCeil,
		)
	}
	if maxEvaluationWorkUnits < 10*peak || maxEvaluationWorkUnits > 1_000*peak {
		t.Fatalf("evaluation limit %d is not calibrated to completed-layout corpus peak %d", maxEvaluationWorkUnits, peak)
	}
}

func readGraph(ctx context.Context, filename string) (*layoutgraph.Graph, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	graph := layoutgraph.NewGraph()
	if err := graphjson.Unmarshal(ctx, data, graph); err != nil {
		return nil, err
	}
	return graph, nil
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
