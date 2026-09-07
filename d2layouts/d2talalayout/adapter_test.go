package d2talalayout

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"math"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2sequence"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

func TestLayoutRejectsNilContext(t *testing.T) {
	graph, _ := newD2TransactionGraph(false)
	before := snapshotD2Graph(t, graph)
	//lint:ignore SA1012 Verify rejection of an invalid public API argument.
	if err := Layout(nil, graph, nil); err == nil {
		t.Fatal("Layout accepted a nil context")
	}
	before.assertUnchanged(t, graph)
}

func TestRouteEdgesRejectsNilContext(t *testing.T) {
	graph, _ := newD2TransactionGraph(false)
	before := snapshotD2Graph(t, graph)
	//lint:ignore SA1012 Verify rejection of an invalid public API argument.
	if err := RouteEdges(nil, graph, graph.Edges); err == nil {
		t.Fatal("RouteEdges accepted a nil context")
	}
	before.assertUnchanged(t, graph)
}

func TestDefaultOptionsReturnsFreshCopy(t *testing.T) {
	first := DefaultOptions()
	second := DefaultOptions()

	first.Seeds[0] = 99
	if second.Seeds[0] != 1 {
		t.Fatalf("mutating one default changed another: %v", second.Seeds)
	}
	if first.MaxConcurrency < 1 || first.MaxConcurrency > defaultMaxSeedConcurrency {
		t.Fatalf("default concurrency = %d", first.MaxConcurrency)
	}
}

func TestAllocateD2EntityIDsPreservesUnsignedSingletonHash(t *testing.T) {
	const want layoutgraph.EntityID = 3_826_002_220
	allocated, err := allocateD2EntityIDs(t.Context(), "object", []d2EntityIdentity[string]{
		{entity: "object-a", absID: "a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := allocated["object-a"]
	if got != want {
		t.Fatalf("allocated ID for %q = %d, want unsigned FNV-1a value %d", "a", got, want)
	}
	if got <= 1<<31-1 {
		t.Fatalf("test hash %d does not exercise the high uint32 bit", got)
	}
	if got <= 0 || got == -1 {
		t.Fatalf("high-bit real ID %d overlaps the negative synthetic-ID domain", got)
	}
}

func TestAllocateD2EntityIDsResolvesCollisionsDeterministically(t *testing.T) {
	if d2FNV32("costarring") != d2FNV32("liquid") {
		t.Fatal("test strings no longer collide")
	}
	first := []d2EntityIdentity[string]{
		{entity: "first", absID: "costarring"},
		{entity: "second", absID: "liquid"},
		{entity: "singleton", absID: "a"},
	}
	second := []d2EntityIdentity[string]{first[2], first[1], first[0]}
	firstAllocation, err := allocateD2EntityIDs(t.Context(), "object", first)
	if err != nil {
		t.Fatal(err)
	}
	secondAllocation, err := allocateD2EntityIDs(t.Context(), "object", second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstAllocation, secondAllocation) {
		t.Fatalf("permuting input changed allocation: first=%v second=%v", firstAllocation, secondAllocation)
	}
	if firstAllocation["first"] != firstD2SpillEntityID || firstAllocation["second"] != firstD2SpillEntityID+1 {
		t.Fatalf("collision spill IDs = (%d, %d), want (%d, %d)",
			firstAllocation["first"],
			firstAllocation["second"],
			firstD2SpillEntityID,
			firstD2SpillEntityID+1,
		)
	}
	if got, want := firstAllocation["singleton"], layoutgraph.EntityID(d2FNV32("a")); got != want {
		t.Fatalf("singleton ID = %d, want preserved hash %d", got, want)
	}
}

func TestAllocateD2EntityIDsSpillsReservedZeroHash(t *testing.T) {
	zeroHashID := string([]byte{0xcc, 0x24, 0x31, 0xc4})
	if got := d2FNV32(zeroHashID); got != 0 {
		t.Fatalf("zero-hash fixture hashes to %d", got)
	}
	allocated, err := allocateD2EntityIDs(t.Context(), "object", []d2EntityIdentity[string]{
		{entity: "zero", absID: zeroHashID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := allocated["zero"]; got != firstD2SpillEntityID {
		t.Fatalf("zero-hash ID = %d, want spill ID %d", got, firstD2SpillEntityID)
	}
}

func TestAllocateD2EntityIDsRejectsDuplicateIdentity(t *testing.T) {
	_, err := allocateD2EntityIDs(t.Context(), "edge", []d2EntityIdentity[int]{
		{entity: 1, absID: "duplicate"},
		{entity: 2, absID: "duplicate"},
	})
	if err == nil || !strings.Contains(err.Error(), `edge ID "duplicate" is repeated`) {
		t.Fatalf("duplicate identity error = %v", err)
	}
}

func BenchmarkAmbiguousIdentitySort(b *testing.B) {
	const size = 1024
	base := make([]hashedD2EntityIdentity[int], size)
	for i := range base {
		base[i] = hashedD2EntityIdentity[int]{
			d2EntityIdentity: d2EntityIdentity[int]{entity: i, absID: strconv.Itoa(size - i)},
			hash:             uint32(i / 4),
		}
	}

	b.Run("reflect", func(b *testing.B) {
		work := make([]hashedD2EntityIdentity[int], len(base))
		b.ReportAllocs()
		for b.Loop() {
			copy(work, base)
			sort.Slice(work, func(i, j int) bool {
				if work[i].hash != work[j].hash {
					return work[i].hash < work[j].hash
				}
				return work[i].absID < work[j].absID
			})
		}
	})

	b.Run("typed", func(b *testing.B) {
		work := make([]hashedD2EntityIdentity[int], len(base))
		b.ReportAllocs()
		for b.Loop() {
			copy(work, base)
			slices.SortFunc(work, func(a, b hashedD2EntityIdentity[int]) int {
				if order := cmp.Compare(a.hash, b.hash); order != 0 {
					return order
				}
				return cmp.Compare(a.absID, b.absID)
			})
		}
	})
}

func TestLayoutPlanPrecedence(t *testing.T) {
	opts := &Options{Seeds: []int64{4, 5}, MaxConcurrency: 1}

	fromData, concurrency, err := layoutPlan(&d2graph.Graph{Data: map[string]any{
		"tala-seeds": []any{9, 9, 8},
	}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fromData, []int64{9, 8}) || concurrency != 1 {
		t.Fatalf("data plan = (%v, %d)", fromData, concurrency)
	}

	fromOpts, _, err := layoutPlan(&d2graph.Graph{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fromOpts, []int64{4, 5}) {
		t.Fatalf("option seeds = %v", fromOpts)
	}

	fromDefaults, concurrency, err := layoutPlan(&d2graph.Graph{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fromDefaults, []int64{1, 2, 3}) {
		t.Fatalf("default seeds = %v", fromDefaults)
	}
	if concurrency < 1 || concurrency > len(fromDefaults) {
		t.Fatalf("default concurrency = %d", concurrency)
	}

	fromZeroValue, zeroConcurrency, err := layoutPlan(&d2graph.Graph{}, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fromZeroValue, fromDefaults) || zeroConcurrency != concurrency {
		t.Fatalf(
			"zero-value plan = (%v, %d), want defaults (%v, %d)",
			fromZeroValue,
			zeroConcurrency,
			fromDefaults,
			concurrency,
		)
	}

	if _, _, err := layoutPlan(&d2graph.Graph{}, &Options{Seeds: []int64{}}); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("explicit empty seeds error = %v", err)
	}

	fromZeroWithData, _, err := layoutPlan(&d2graph.Graph{Data: map[string]any{
		"tala-seeds": []any{8, 9},
	}}, &Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(fromZeroWithData, []int64{8, 9}) {
		t.Fatalf("graph data did not override zero-value defaults: %v", fromZeroWithData)
	}
}

func TestNormalizeSeeds(t *testing.T) {
	got, err := normalizeSeeds([]int64{3, 1, 3, 2, 1})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []int64{3, 1, 2}) {
		t.Fatalf("deduplication did not preserve first-seen order: %v", got)
	}

	if _, err := normalizeSeeds(nil); err == nil || !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("expected clear empty-seed error, got %v", err)
	}

	max := make([]int64, maxSeeds)
	for i := range max {
		max[i] = int64(i)
	}
	if _, err := normalizeSeeds(max); err != nil {
		t.Fatalf("maxSeeds should be accepted: %v", err)
	}

	over := append(slices.Clone(max), int64(maxSeeds))
	if _, err := normalizeSeeds(over); err == nil || !strings.Contains(err.Error(), "at most 16") {
		t.Fatalf("expected maxSeeds error, got %v", err)
	}

	duplicates := append(slices.Clone(max), max...)
	if got, err := normalizeSeeds(duplicates); err != nil || len(got) != maxSeeds {
		t.Fatalf("duplicates should not count against maxSeeds: seeds=%v err=%v", got, err)
	}

	tooManyEntries := make([]int64, maxSeedEntries+1)
	if _, err := normalizeSeeds(tooManyEntries); err == nil || !strings.Contains(err.Error(), "at most 64 seed entries") {
		t.Fatalf("expected seed-entry bound, got %v", err)
	}
}

func TestLayoutPlanRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want string
	}{
		{name: "not a list", raw: "1", want: "must be a list"},
		{name: "malformed member", raw: []any{1, "nope"}, want: "index 1"},
		{name: "overflow", raw: []string{"9223372036854775808"}, want: "signed 64-bit"},
		{name: "empty", raw: []any{}, want: "at least one"},
		{name: "too many entries", raw: make([]int, maxSeedEntries+1), want: "at most 64 seed entries"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := layoutPlan(&d2graph.Graph{Data: map[string]any{
				"tala-seeds": tt.raw,
			}}, &Options{Seeds: []int64{7}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got %v", tt.want, err)
			}
		})
	}

	for _, concurrency := range []int{-1, maxSeeds + 1} {
		_, _, err := layoutPlan(&d2graph.Graph{}, &Options{Seeds: []int64{1}, MaxConcurrency: concurrency})
		if err == nil || !strings.Contains(err.Error(), "MaxConcurrency") {
			t.Fatalf("MaxConcurrency=%d error = %v", concurrency, err)
		}
	}
	_, concurrency, err := layoutPlan(&d2graph.Graph{}, &Options{Seeds: []int64{1, 2}, MaxConcurrency: maxSeeds})
	if err != nil || concurrency != 2 {
		t.Fatalf("clamped concurrency = %d, err = %v", concurrency, err)
	}
}

func TestCoordinateLocalSeedsRunsAllWithBoundedConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		seeds := []int64{1, 2, 3, 4, 5, 6}
		var active atomic.Int32
		var maxActive atomic.Int32
		var completed atomic.Int32

		best, err := coordinateLocalSeeds(t.Context(), seeds, 2, func(index int, seed int64) localSeedAttempt {
			current := active.Add(1)
			for {
				maximum := maxActive.Load()
				if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
					break
				}
			}
			defer active.Add(-1)
			time.Sleep(time.Duration(len(seeds)-index) * time.Millisecond)
			completed.Add(1)
			return localSeedAttempt{
				index:  index,
				seed:   seed,
				result: seedResult{score: layoutScore{penalty: float64(seed)}},
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if completed.Load() != int32(len(seeds)) {
			t.Fatalf("completed attempts = %d", completed.Load())
		}
		if maximum := maxActive.Load(); maximum != 2 {
			t.Fatalf("maximum concurrency = %d, want 2", maximum)
		}
		if best.score.penalty != 1 {
			t.Fatalf("best score = %+v", best.score)
		}
	})
}

func TestCoordinateLocalSeedsSelectionIsIndependentOfCompletionOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		graphs := []*layoutgraph.Graph{layoutgraph.NewGraph(), layoutgraph.NewGraph(), layoutgraph.NewGraph()}
		best, err := coordinateLocalSeeds(t.Context(), []int64{1, 2, 3}, 3, func(index int, seed int64) localSeedAttempt {
			time.Sleep(time.Duration(3-index) * time.Millisecond)
			return localSeedAttempt{
				index: index,
				seed:  seed,
				result: seedResult{
					graph: graphs[index],
					score: layoutScore{penalty: 1, area: 1},
				},
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if best.graph != graphs[2] {
			t.Fatal("equal results did not favor the later configured seed")
		}
	})
}

func TestCoordinateLocalSeedsToleratesPartialFailure(t *testing.T) {
	best, err := coordinateLocalSeeds(t.Context(), []int64{1, 2}, 2, func(index int, seed int64) localSeedAttempt {
		attempt := localSeedAttempt{index: index, seed: seed}
		if seed == 1 {
			attempt.err = errors.New("first failed")
		} else {
			attempt.result = seedResult{score: layoutScore{penalty: 2}}
		}
		return attempt
	})
	if err != nil || best.score.penalty != 2 {
		t.Fatalf("partial failure result = (%+v, %v)", best.score, err)
	}
}

func TestCoordinateLocalSeedsReportsAllFailures(t *testing.T) {
	_, err := coordinateLocalSeeds(t.Context(), []int64{1, 2}, 2, func(index int, seed int64) localSeedAttempt {
		return localSeedAttempt{index: index, seed: seed, err: errors.New("failed")}
	})
	if err == nil || !strings.Contains(err.Error(), "seed 1") || !strings.Contains(err.Error(), "seed 2") {
		t.Fatalf("all-failure error = %v", err)
	}
}

func TestCoordinateLocalSeedsCancellationIsStrictAndJoinsWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	started := make(chan struct{}, 2)
	exited := make(chan struct{})
	type callResult struct {
		result seedResult
		err    error
	}
	resultCh := make(chan callResult, 1)
	go func() {
		selected, err := coordinateLocalSeeds(ctx, []int64{1, 2}, 2, func(index int, seed int64) localSeedAttempt {
			started <- struct{}{}
			if seed == 1 {
				return localSeedAttempt{index: index, seed: seed, result: seedResult{score: layoutScore{penalty: 1}}}
			}
			<-ctx.Done()
			close(exited)
			return localSeedAttempt{index: index, seed: seed, err: ctx.Err()}
		})
		resultCh <- callResult{result: selected, err: err}
	}()
	<-started
	<-started
	cancel()
	got := <-resultCh
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("coordinator error = %v, want context.Canceled", got.err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("coordinator returned before its worker exited")
	}
}

func TestLayoutIsDeterministicAcrossConcurrency(t *testing.T) {
	sequential, _ := newD2TransactionGraph(false)
	concurrent, _ := newD2TransactionGraph(false)
	seeds := []int64{1, 2, 3}
	if err := Layout(t.Context(), sequential, &Options{Seeds: seeds, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	if err := Layout(t.Context(), concurrent, &Options{Seeds: seeds, MaxConcurrency: len(seeds)}); err != nil {
		t.Fatal(err)
	}
	sequentialSnapshot := snapshotD2Graph(t, sequential)
	concurrentSnapshot := snapshotD2Graph(t, concurrent)
	if !bytes.Equal(sequentialSnapshot.json, concurrentSnapshot.json) ||
		!bytes.Equal(sequentialSnapshot.serialized, concurrentSnapshot.serialized) {
		t.Fatal("layout changed with seed concurrency")
	}
}

func TestLayoutPreCanceledErrorPrecedesSeedResolution(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	graph := d2graph.NewGraph()
	graph.Data = map[string]any{
		"tala-seeds": []any{"invalid"},
	}

	if err := Layout(ctx, graph, nil); err != context.Canceled {
		t.Fatalf("Layout error = %v, want context.Canceled", err)
	}
}

func TestRecoverAsError(t *testing.T) {
	formatted := false
	payload := panicPayload{
		formatted: &formatted,
		text:      "SECRET diagram data\ngoroutine 1 [running]:\n/private/path.go:10",
	}
	call := func() (err error) {
		defer recoverAsError("test", &err)
		panic(payload)
	}

	err := call()
	if err == nil || err.Error() != "TALA test failed due to an internal invariant" {
		t.Fatalf("expected sanitized invariant error, got %v", err)
	}
	if formatted {
		t.Fatal("recovered panic payload was formatted")
	}
	if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "goroutine") || strings.Contains(err.Error(), "/private/") {
		t.Fatalf("panic payload leaked through public error: %v", err)
	}
}

type panicPayload struct {
	formatted *bool
	text      string
}

func (p panicPayload) String() string {
	*p.formatted = true
	return p.text
}

func TestPublicEntrypointsRejectInvalidGraphs(t *testing.T) {
	if err := Layout(t.Context(), nil, nil); err == nil || !strings.Contains(err.Error(), "root object") {
		t.Fatalf("Layout should reject a nil graph, got %v", err)
	}
	if err := RouteEdges(t.Context(), nil, nil); err == nil || !strings.Contains(err.Error(), "root object") {
		t.Fatalf("RouteEdges should reject a nil graph, got %v", err)
	}
}

func TestCollectD2ObjectVisitsPreservesDFSOrder(t *testing.T) {
	g := d2graph.NewGraph()
	a := appendD2Object(g.Root, "a")
	appendD2Object(a, "aa")
	appendD2Object(a, "ab")
	appendD2Object(g.Root, "b")

	visits, _, err := collectD2ObjectVisits(t.Context(), g.Root)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(visits))
	for i, visit := range visits {
		got[i] = visit.object.ID
	}
	if !slices.Equal(got, []string{"a", "aa", "ab", "b"}) {
		t.Fatalf("DFS order = %v", got)
	}
}

func TestTranslateGraphRejectsMalformedObjectTrees(t *testing.T) {
	tests := []struct {
		name  string
		graph func() *d2graph.Graph
		want  string
	}{
		{
			name: "nil child",
			graph: func() *d2graph.Graph {
				g := d2graph.NewGraph()
				g.Root.ChildrenArray = append(g.Root.ChildrenArray, nil)
				return g
			},
			want: "nil object",
		},
		{
			name: "shared child",
			graph: func() *d2graph.Graph {
				g := d2graph.NewGraph()
				left := appendD2Object(g.Root, "left")
				right := appendD2Object(g.Root, "right")
				shared := appendD2Object(left, "shared")
				right.ChildrenArray = append(right.ChildrenArray, shared)
				return g
			},
			want: "referenced more than once",
		},
		{
			name: "cycle",
			graph: func() *d2graph.Graph {
				g := d2graph.NewGraph()
				parent := appendD2Object(g.Root, "parent")
				child := appendD2Object(parent, "child")
				child.ChildrenArray = append(child.ChildrenArray, parent)
				return g
			},
			want: "ChildrenArray cycle",
		},
		{
			name: "mismatched parent",
			graph: func() *d2graph.Graph {
				g := d2graph.NewGraph()
				child := appendD2Object(g.Root, "child")
				child.Parent = nil
				return g
			},
			want: "does not match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := translateGraph(t.Context(), tt.graph(), layoutgraph.NewGraph(), false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("translation error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTranslateGraphBoundsObjectsAndDepth(t *testing.T) {
	t.Run("object count", func(t *testing.T) {
		g := d2graph.NewGraph()
		for i := 0; i <= maxInputNodes; i++ {
			appendD2Object(g.Root, "node")
		}
		_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
		if err == nil || !strings.Contains(err.Error(), "object count exceeds") {
			t.Fatalf("translation error = %v", err)
		}
	})

	t.Run("depth", func(t *testing.T) {
		g := d2graph.NewGraph()
		parent := g.Root
		for i := 0; i <= maxInputTreeDepth; i++ {
			parent = appendD2Object(parent, "nested")
		}
		_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
		if err == nil || !strings.Contains(err.Error(), "nesting depth exceeds") {
			t.Fatalf("translation error = %v", err)
		}
	})
}

func TestTranslateGraphRejectsInvalidObjectGeometry(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*d2graph.Object)
		want   string
	}{
		{
			name: "missing box",
			mutate: func(object *d2graph.Object) {
				object.Box = nil
			},
			want: "has no box",
		},
		{
			name: "zero width",
			mutate: func(object *d2graph.Object) {
				object.Width = 0
			},
			want: "width is outside",
		},
		{
			name: "non-finite height",
			mutate: func(object *d2graph.Object) {
				object.Height = math.Inf(1)
			},
			want: "height is outside",
		},
		{
			name: "non-finite position",
			mutate: func(object *d2graph.Object) {
				object.TopLeft.X = math.NaN()
			},
			want: "top-left x is outside",
		},
		{
			name: "negative label dimensions",
			mutate: func(object *d2graph.Object) {
				object.LabelDimensions.Width = -1
			},
			want: "label dimensions must be nonnegative",
		},
		{
			name: "invalid opacity",
			mutate: func(object *d2graph.Object) {
				object.Style.Opacity = &d2graph.Scalar{Value: "opaque"}
			},
			want: "invalid opacity",
		},
		{
			name: "invalid font size",
			mutate: func(object *d2graph.Object) {
				object.Style.FontSize = &d2graph.Scalar{Value: "huge"}
			},
			want: "invalid font-size",
		},
		{
			name: "invalid label position",
			mutate: func(object *d2graph.Object) {
				position := label.Unset.String()
				object.LabelPosition = &position
			},
			want: "invalid label position",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newD2TransactionGraph(false)
			tt.mutate(g.Objects[0])
			_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("translation error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTranslateGraphBoundsAndValidatesEdges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*d2graph.Graph)
		want   string
	}{
		{
			name: "edge count",
			mutate: func(g *d2graph.Graph) {
				g.Edges = make([]*d2graph.Edge, maxInputEdges+1)
			},
			want: "edge count exceeds",
		},
		{
			name: "repeated edge",
			mutate: func(g *d2graph.Graph) {
				g.Edges = append(g.Edges, g.Edges[0])
			},
			want: "is repeated",
		},
		{
			name: "foreign source",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].Src = &d2graph.Object{ID: "foreign"}
			},
			want: "source outside",
		},
		{
			name: "foreign destination",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].Dst = &d2graph.Object{ID: "foreign"}
			},
			want: "destination outside",
		},
		{
			name: "route point count",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].Route = make([]*geo.Point, maxInputRoutePoints+1)
			},
			want: "route point count exceeds",
		},
		{
			name: "nil route point",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].Route[0] = nil
			},
			want: "nil route point",
		},
		{
			name: "non-finite route point",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].Route[0].X = math.Inf(1)
			},
			want: "finite supported range",
		},
		{
			name: "negative edge label dimensions",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].LabelDimensions.Height = -1
			},
			want: "label dimensions must be nonnegative",
		},
		{
			name: "negative arrowhead label dimensions",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].SrcArrowhead = &d2graph.Attributes{
					LabelDimensions: d2target.TextDimensions{Width: -1},
				}
			},
			want: "source arrowhead label dimensions must be nonnegative",
		},
		{
			name: "single route point",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].Route = g.Edges[0].Route[:1]
			},
			want: "either zero or at least two",
		},
		{
			name: "degenerate route segment",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].Route[1] = g.Edges[0].Route[0].Copy()
			},
			want: "degenerate route segment",
		},
		{
			name: "invalid edge label position",
			mutate: func(g *d2graph.Graph) {
				position := label.OutsideLeftMiddle.String()
				g.Edges[0].LabelPosition = &position
			},
			want: "invalid label position",
		},
		{
			name: "unknown arrowhead",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].SrcArrowhead = &d2graph.Attributes{}
				g.Edges[0].SrcArrowhead.Shape.Value = "mystery"
			},
			want: "unsupported source arrowhead",
		},
		{
			name: "invalid arrowhead filled boolean",
			mutate: func(g *d2graph.Graph) {
				g.Edges[0].SrcArrowhead = &d2graph.Attributes{}
				g.Edges[0].SrcArrowhead.Style.Filled = &d2graph.Scalar{Value: "maybe"}
			},
			want: "invalid filled boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newD2TransactionGraph(false)
			tt.mutate(g)
			_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), true)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("translation error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTranslateGraphRequiresCoherentD2TopologyViews(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*d2graph.Graph)
		want   string
	}{
		{
			name: "child missing from map",
			mutate: func(g *d2graph.Graph) {
				delete(g.Root.Children, "a")
			},
			want: "inconsistent Children and ChildrenArray sets",
		},
		{
			name: "child map points elsewhere",
			mutate: func(g *d2graph.Graph) {
				g.Root.Children["a"] = g.Objects[1]
			},
			want: "inconsistent Children and ChildrenArray sets",
		},
		{
			name: "object missing from flat list",
			mutate: func(g *d2graph.Graph) {
				g.Objects = g.Objects[1:]
			},
			want: "different sets",
		},
		{
			name: "object repeated in flat list",
			mutate: func(g *d2graph.Graph) {
				g.Objects[1] = g.Objects[0]
			},
			want: "repeats object",
		},
		{
			name: "object belongs to another graph",
			mutate: func(g *d2graph.Graph) {
				g.Objects[0].Graph = d2graph.NewGraph()
			},
			want: "belongs to a different graph",
		},
		{
			name: "object has nil graph pointer",
			mutate: func(g *d2graph.Graph) {
				g.Objects[0].Graph = nil
			},
			want: "belongs to a different graph",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newD2TransactionGraph(false)
			tt.mutate(g)
			_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("translation error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTranslateGraphUsesObjectBeforeNearConstant(t *testing.T) {
	g := d2graph.NewGraph()
	nearTarget := appendD2Object(g.Root, "top-left")
	nearSource := appendD2Object(g.Root, "source")
	nearSource.NearKey = d2ast.MakeKeyPath([]string{"top-left"})
	talaGraph := layoutgraph.NewGraph()

	bindings, err := translateGraph(t.Context(), g, talaGraph, false)
	if err != nil {
		t.Fatal(err)
	}
	var sourceNode, targetNode *layoutgraph.Node
	for _, node := range talaGraph.Nodes {
		switch node.ID {
		case bindings.objectIDs[nearSource]:
			sourceNode = node
		case bindings.objectIDs[nearTarget]:
			targetNode = node
		}
	}
	if sourceNode == nil || targetNode == nil {
		t.Fatal("translated near nodes were not found")
	}
	if _, found := sourceNode.Nears[targetNode]; !found {
		t.Fatal("object named like a near constant was not resolved as an object")
	}
}

func TestTranslateGraphUsesObjectBeforeReservedNearPart(t *testing.T) {
	g := d2graph.NewGraph()
	nearTarget := appendD2Object(g.Root, "label")
	nearSource := appendD2Object(g.Root, "source")
	nearSource.NearKey = d2ast.MakeKeyPath([]string{"label"})
	talaGraph := layoutgraph.NewGraph()

	bindings, err := translateGraph(t.Context(), g, talaGraph, false)
	if err != nil {
		t.Fatal(err)
	}
	var sourceNode, targetNode *layoutgraph.Node
	for _, node := range talaGraph.Nodes {
		switch node.ID {
		case bindings.objectIDs[nearSource]:
			sourceNode = node
		case bindings.objectIDs[nearTarget]:
			targetNode = node
		}
	}
	if sourceNode == nil || targetNode == nil {
		t.Fatal("translated near nodes were not found")
	}
	if _, found := sourceNode.Nears[targetNode]; !found {
		t.Fatal("object named like a reserved keyword was not resolved as an object")
	}
}

func TestFindD2ObjectRejectsNilMappedChild(t *testing.T) {
	root := d2graph.NewGraph().Root
	root.Children["child"] = nil
	_, _, err := findD2Object(t.Context(), root, []string{"child"})
	if err == nil || !strings.Contains(err.Error(), "nil child") {
		t.Fatalf("nil child error = %v", err)
	}
}

func TestTranslateGraphRejectsInvalidIntegerAttributes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*d2graph.Object)
		want   string
	}{
		{
			name: "malformed top",
			mutate: func(object *d2graph.Object) {
				object.Top = &d2graph.Scalar{Value: "nope"}
				object.Left = &d2graph.Scalar{Value: "1"}
			},
			want: "invalid integer top attribute",
		},
		{
			name: "zero desired width",
			mutate: func(object *d2graph.Object) {
				object.WidthAttr = &d2graph.Scalar{Value: "0"}
			},
			want: "width attribute must be between 1",
		},
		{
			name: "overflow desired height",
			mutate: func(object *d2graph.Object) {
				object.HeightAttr = &d2graph.Scalar{Value: "1000000001"}
			},
			want: "height attribute must be between",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newD2TransactionGraph(false)
			tt.mutate(g.Objects[0])
			_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("translation error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTranslateGraphRejectsIndependentTopAndLeftConstraints(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*d2graph.Object)
	}{
		{name: "top", mutate: func(object *d2graph.Object) { object.Top = &d2graph.Scalar{Value: "1"} }},
		{name: "left", mutate: func(object *d2graph.Object) { object.Left = &d2graph.Scalar{Value: "1"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := d2graph.NewGraph()
			object := appendD2Object(g.Root, "constrained")
			test.mutate(object)
			_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
			if err == nil || !strings.Contains(err.Error(), "must set top and left together") {
				t.Fatalf("independent %s constraint error = %v", test.name, err)
			}
		})
	}
}

func TestLayoutPreservesFixedSingletonPosition(t *testing.T) {
	graph := d2graph.NewGraph()
	object := appendD2Object(graph.Root, "fixed")
	object.Box = geo.NewBox(geo.NewPoint(0, 0), 40, 40)
	object.Top = &d2graph.Scalar{Value: "241"}
	object.Left = &d2graph.Scalar{Value: "137"}

	if err := Layout(t.Context(), graph, &Options{Seeds: []int64{1}, MaxConcurrency: 1}); err != nil {
		t.Fatal(err)
	}
	want := geo.Point{X: 137, Y: 241}
	if object.TopLeft == nil || *object.TopLeft != want {
		t.Fatalf("fixed singleton TopLeft = %v, want %v", object.TopLeft, want)
	}
}

func TestTranslateGraphBoundsSQLColumnsAndIndices(t *testing.T) {
	t.Run("aggregate columns", func(t *testing.T) {
		g, _ := newD2TransactionGraph(false)
		g.Objects[0].Shape.Value = d2target.ShapeSQLTable
		g.Objects[0].SQLTable = &d2target.SQLTable{
			Columns: make([]d2target.SQLColumn, maxInputNodes+1),
		}
		_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
		if err == nil || !strings.Contains(err.Error(), "SQL table column count exceeds") {
			t.Fatalf("translation error = %v", err)
		}
	})

	t.Run("table payload on non-table shape", func(t *testing.T) {
		g, _ := newD2TransactionGraph(false)
		g.Objects[0].SQLTable = &d2target.SQLTable{Columns: make([]d2target.SQLColumn, 1)}
		_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
		if err == nil || !strings.Contains(err.Error(), "is not an SQL table") {
			t.Fatalf("translation error = %v", err)
		}
	})

	t.Run("out of range index", func(t *testing.T) {
		g, edge := newD2TransactionGraph(false)
		g.Objects[0].Shape.Value = d2target.ShapeSQLTable
		g.Objects[0].SQLTable = &d2target.SQLTable{Columns: make([]d2target.SQLColumn, 1)}
		edge.SrcTableColumnIndex = new(1)
		_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
		if err == nil || !strings.Contains(err.Error(), "outside table with 1 columns") {
			t.Fatalf("translation error = %v", err)
		}
	})

	t.Run("index on non-table", func(t *testing.T) {
		g, edge := newD2TransactionGraph(false)
		edge.SrcTableColumnIndex = new(0)
		_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), false)
		if err == nil || !strings.Contains(err.Error(), "non-table object") {
			t.Fatalf("translation error = %v", err)
		}
	})
}

func TestTranslateGraphAcceptsOnlyGeneratedLifelineForm(t *testing.T) {
	g, _ := newD2TransactionGraph(true)
	if _, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), true); err != nil {
		t.Fatalf("generated lifeline was rejected: %v", err)
	}

	lifeline := g.Edges[len(g.Edges)-1]
	lifeline.Dst.ID = "other" + lifeline.Dst.ID[len("a"):]
	_, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), true)
	if err == nil || !strings.Contains(err.Error(), "destination outside") {
		t.Fatalf("mismatched lifeline was accepted: %v", err)
	}

	g, _ = newD2TransactionGraph(true)
	lifeline = g.Edges[len(g.Edges)-1]
	lifeline.Dst.ID = d2sequence.LifelineEndID("b")
	if !d2sequence.IsLifelineEnd(lifeline.Dst) {
		t.Fatal("test endpoint is not a canonical lifeline ID")
	}
	_, err = translateGraph(t.Context(), g, layoutgraph.NewGraph(), true)
	if err == nil || !strings.Contains(err.Error(), "destination outside") {
		t.Fatalf("another actor's canonical lifeline was accepted: %v", err)
	}
}

func TestRouteEdgesBoundsRequestedSubsetBeforeTranslation(t *testing.T) {
	g, edge := newD2TransactionGraph(false)
	if err := RouteEdges(t.Context(), g, []*d2graph.Edge{edge, edge}); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("duplicate requested edge error = %v", err)
	}
	requested := make([]*d2graph.Edge, maxInputEdges+1)
	if err := RouteEdges(t.Context(), g, requested); err == nil || !strings.Contains(err.Error(), "requested D2 edge count exceeds") {
		t.Fatalf("requested edge count error = %v", err)
	}
	if err := RouteEdges(t.Context(), g, []*d2graph.Edge{{}}); err == nil || !strings.Contains(err.Error(), "is not in the graph") {
		t.Fatalf("foreign requested edge error = %v", err)
	}
}

func TestRouteEdgesRequiresPositionedObjects(t *testing.T) {
	g, edge := newD2TransactionGraph(false)
	g.Objects[0].TopLeft = nil
	err := RouteEdges(t.Context(), g, []*d2graph.Edge{edge})
	if err == nil || !strings.Contains(err.Error(), "has no top-left position") {
		t.Fatalf("missing position error = %v", err)
	}
}

func TestRouteEdgesPreservesUnselectedRouteGeometry(t *testing.T) {
	g, unselected := newD2TransactionGraph(false)
	unselected.Label.Value = "existing label"
	unselected.LabelDimensions = d2target.TextDimensions{Width: 80, Height: 20}
	position := label.OutsideTopCenter.String()
	percentage := 0.5
	unselected.LabelPosition = &position
	unselected.LabelPercentage = &percentage
	originalLabelPosition := unselected.LabelPosition
	originalLabelPercentage := unselected.LabelPercentage
	unselected.IsCurve = true
	unselected.Route = []*geo.Point{
		geo.NewPoint(110, 50),
		geo.NewPoint(180, 10),
		geo.NewPoint(250, 50),
	}
	originalRoute := append([]*geo.Point(nil), unselected.Route...)
	originalValues := make([]*geo.Point, len(unselected.Route))
	for index, point := range unselected.Route {
		originalValues[index] = point.Copy()
	}

	selected := &d2graph.Edge{
		Index:    1,
		Src:      unselected.Src,
		Dst:      unselected.Dst,
		DstArrow: true,
	}
	g.Edges = append(g.Edges, selected)

	if err := RouteEdges(t.Context(), g, []*d2graph.Edge{selected}); err != nil {
		t.Fatal(err)
	}
	if !unselected.IsCurve {
		t.Fatal("rerouting a selected edge cleared the unselected edge's curve flag")
	}
	if unselected.LabelPosition != originalLabelPosition || unselected.LabelPercentage != originalLabelPercentage ||
		*unselected.LabelPosition != position || *unselected.LabelPercentage != percentage {
		t.Fatal("rerouting a selected edge changed unselected label placement")
	}
	if len(unselected.Route) != len(originalRoute) {
		t.Fatalf("unselected route has %d points, want %d", len(unselected.Route), len(originalRoute))
	}
	for index := range originalRoute {
		if unselected.Route[index] != originalRoute[index] || !unselected.Route[index].Equals(originalValues[index]) {
			t.Fatalf("unselected route point %d changed from %p %v to %p %v", index, originalRoute[index], originalValues[index], unselected.Route[index], unselected.Route[index])
		}
	}
}

func TestRouteEdgesPreservesUnselectedLifelineDestination(t *testing.T) {
	g, selected := newD2TransactionGraph(true)
	unselected := g.Edges[len(g.Edges)-1]
	originalDestination := unselected.Dst

	if err := RouteEdges(t.Context(), g, []*d2graph.Edge{selected}); err != nil {
		t.Fatal(err)
	}
	if unselected.Dst != originalDestination {
		t.Fatal("rerouting a selected edge rewrote an unselected lifeline destination")
	}
}

func TestRouteEdgesPreservesSelectedSQLTableRowPorts(t *testing.T) {
	g, selected := newD2TransactionGraph(false)
	for _, object := range g.Objects {
		object.Shape.Value = d2target.ShapeSQLTable
		object.SQLTable = &d2target.SQLTable{Columns: make([]d2target.SQLColumn, 3)}
	}
	sourceColumn, targetColumn := 0, 2
	selected.SrcTableColumnIndex = &sourceColumn
	selected.DstTableColumnIndex = &targetColumn

	if err := RouteEdges(t.Context(), g, []*d2graph.Edge{selected}); err != nil {
		t.Fatal(err)
	}
	if len(selected.Route) < 2 {
		t.Fatalf("selected table edge has %d route points", len(selected.Route))
	}
	rowHeight := g.Objects[0].Height / 4
	wantSource := geo.Point{
		X: g.Objects[0].TopLeft.X + g.Objects[0].Width,
		Y: g.Objects[0].TopLeft.Y + math.Round(rowHeight*1.5),
	}
	wantTarget := geo.Point{
		X: g.Objects[1].TopLeft.X,
		Y: g.Objects[1].TopLeft.Y + math.Round(rowHeight*3.5),
	}
	if got := *selected.Route[0]; got != wantSource {
		t.Fatalf("source table port = %v, want row port %v", got, wantSource)
	}
	if got := *selected.Route[len(selected.Route)-1]; got != wantTarget {
		t.Fatalf("target table port = %v, want row port %v", got, wantTarget)
	}
}

func TestCollectD2ObjectVisitsChecksContextDuringTraversal(t *testing.T) {
	g := d2graph.NewGraph()
	appendD2Object(g.Root, "a")
	appendD2Object(g.Root, "b")
	base, cancel := context.WithCancel(t.Context())
	ctx := &cancelAfterContext{Context: base, cancel: cancel, after: 2}

	_, _, err := collectD2ObjectVisits(ctx, g.Root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("traversal error = %v, want context.Canceled", err)
	}
}

type cancelAfterContext struct {
	context.Context
	cancel context.CancelFunc
	after  int
	checks int
}

func (ctx *cancelAfterContext) Err() error {
	ctx.checks++
	if ctx.checks == ctx.after {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func appendD2Object(parent *d2graph.Object, id string) *d2graph.Object {
	child := &d2graph.Object{
		Graph:    parent.Graph,
		Parent:   parent,
		ID:       id,
		IDVal:    id,
		Box:      geo.NewBox(geo.NewPoint(0, 0), 10, 10),
		Children: make(map[string]*d2graph.Object),
	}
	parent.Children[strings.ToLower(id)] = child
	parent.ChildrenArray = append(parent.ChildrenArray, child)
	if parent.Graph != nil {
		parent.Graph.Objects = append(parent.Graph.Objects, child)
	}
	return child
}
