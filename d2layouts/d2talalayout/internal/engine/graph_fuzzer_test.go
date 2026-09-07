package engine

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/log"
)

func FuzzAutolayout(f *testing.F) {
	// Keep the ordinary test run fast while covering sparse, medium, and dense
	// graphs. Go's fuzzing engine mutates these inputs and records any failure as
	// a reproducible corpus entry.
	f.Add(int64(1), uint8(10), uint8(2), uint8(50), uint8(40), uint8(10), uint8(40))
	f.Add(int64(2), uint8(15), uint8(4), uint8(60), uint8(60), uint8(25), uint8(65))
	f.Add(int64(3), uint8(20), uint8(5), uint8(40), uint8(80), uint8(45), uint8(80))

	f.Fuzz(func(
		t *testing.T,
		seed int64,
		maxNodes uint8,
		connectionIterations uint8,
		compactionPercent uint8,
		nodeSubsetPercent uint8,
		minConnectionPercent uint8,
		maxConnectionPercent uint8,
	) {
		config := normalizeFuzzerConfig(
			seed,
			maxNodes,
			connectionIterations,
			compactionPercent,
			nodeSubsetPercent,
			minConnectionPercent,
			maxConnectionPercent,
		)

		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		ctx = withTestLogger(ctx, t)
		ctx = log.Leveled(ctx, slog.LevelError)

		graph := createRandomGraph(ctx, config)
		before := roundTripGraph(ctx, t, graph)

		if _, err := layoutWithSnapshots(ctx, graph, config.seed, false); err != nil {
			t.Fatalf("autolayout: %v; config: %+v", err, config)
		}
		if err := checkStructuralGraphPropertiesAfterAutolayout(before, graph); err != nil {
			t.Fatalf("layout invariants: %v; config: %+v", err, config)
		}
		if err := checkOverlap(graph); err != nil {
			t.Fatalf("layout overlap: %v; config: %+v", err, config)
		}
		roundTripGraph(ctx, t, graph)
	})
}

func normalizeFuzzerConfig(
	seed int64,
	maxNodes uint8,
	connectionIterations uint8,
	compactionPercent uint8,
	nodeSubsetPercent uint8,
	minConnectionPercent uint8,
	maxConnectionPercent uint8,
) graphFuzzerConfig {
	minPercent := int(minConnectionPercent) % 51
	maxPercent := minPercent + int(maxConnectionPercent)%((90-minPercent)+1)
	return graphFuzzerConfig{
		seed:                          seed,
		maxNodes:                      2 + int(maxNodes)%29,
		connectionIterations:          int(connectionIterations) % 6,
		compactionFactor:              float64(10+int(compactionPercent)%81) / 100,
		nodeSubsetPercentageToConnect: float64(10+int(nodeSubsetPercent)%81) / 100,
		minConnectionProbability:      float64(minPercent) / 100,
		maxConnectionProbability:      float64(maxPercent) / 100,
	}
}

func roundTripGraph(ctx context.Context, t *testing.T, graph *layoutgraph.Graph) *layoutgraph.Graph {
	t.Helper()

	data, err := graphjson.Marshal(ctx, graph)
	if err != nil {
		t.Fatalf("serialize graph: %v", err)
	}

	got := layoutgraph.NewGraph()
	if err := graphjson.Unmarshal(ctx, data, got); err != nil {
		t.Fatalf("deserialize graph: %v", err)
	}
	requireGraphsSerializeEqual(ctx, t, graph, got)
	return got
}
