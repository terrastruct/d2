package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"

	"github.com/d2lang/d2/lib/log"
)

func BenchmarkAutolayout(b *testing.B) {
	benchmarkAll := os.Getenv("BENCHMARK_ALL_STAGES") != ""
	stagesToBenchmark := map[string]struct{}{
		"NodePlacement":         {},
		"AlignAxes":             {},
		"GapNormalization":      {},
		"Equidistance":          {},
		"BalanceSymmetry":       {},
		"EdgeRouting":           {},
		"PreprocessHierarchies": {},
		"SwapStuff":             {},
		"BinPack":               {},
	}

	for _, gp := range loadBenchmarkGraphs(b, os.Getenv("BENCHMARK_GRAPH")) {
		// Capture the loop variable for the benchmark closure.
		graphPath := gp
		b.Run(filepath.Base(graphPath), func(b *testing.B) {
			pipelineMetrics := make(map[string]int64)
			for b.Loop() {
				b.StopTimer() // we don't want to benchmark setup
				ctx := b.Context()
				ctx = withTestLogger(ctx, b)
				ctx = log.Leveled(ctx, slog.LevelError)
				g, err := readGraph(ctx, graphPath)
				if err != nil {
					b.Fatalf("Could not load graph %v. Error %v", graphPath, err)
				}

				b.StartTimer()
				pipeline, err := layoutWithStageTimings(ctx, g, 123)
				b.StopTimer() // we don't want to benchmark the code below

				if err != nil {
					b.Fatalf("Autolayout returned an error during benchmark. Error: %v", err)
				}

				for i, stage := range pipeline.stagePlan() {
					if _, has := stagesToBenchmark[stage.name]; has || benchmarkAll {
						pipelineMetrics[fmt.Sprintf("%02d_%s", i, stage.name)] += pipeline.stageDurations[i].Milliseconds()
					}
				}
				// B.Loop requires the timer to be running at the next loop check. The
				// next iteration stops it again before graph setup.
				b.StartTimer()
			}
			for name, total := range pipelineMetrics {
				b.ReportMetric(float64(total)/float64(b.N), fmt.Sprintf("%s_ms/op", name))
			}
		})
	}
}

func TestBenchmarkFixturesParse(t *testing.T) {
	for _, graphPath := range loadBenchmarkGraphs(t, "") {
		if _, err := readGraph(t.Context(), graphPath); err != nil {
			t.Fatalf("read benchmark fixture %s: %v", graphPath, err)
		}
	}
}

func loadBenchmarkGraphs(tb testing.TB, graphNamePrefix string) []string {
	tb.Helper()

	benchmarkDir := filepath.Join("..", "testdata", "benchmark")
	files, err := os.ReadDir(benchmarkDir)
	if err != nil {
		tb.Fatal(err)
	}

	paths := make([]string, 0, len(files))
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		if graphNamePrefix == "" || strings.HasPrefix(f.Name(), graphNamePrefix) {
			paths = append(paths, filepath.Join(benchmarkDir, f.Name()))
		}
	}
	if len(paths) == 0 {
		if graphNamePrefix == "" {
			tb.Fatalf("no benchmark fixtures found in %s", benchmarkDir)
		}
		tb.Fatalf("no benchmark fixture starts with %q", graphNamePrefix)
	}
	return paths
}
