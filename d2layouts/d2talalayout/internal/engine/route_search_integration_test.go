//go:build !race

package engine

import (
	"path/filepath"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/routing"
)

func TestRouteSearchCorpusTelemetry(t *testing.T) {
	const measuredCheckedInCorpusPeak uint64 = 5_157_522
	const measuredRepresentativeCorpusPeak uint64 = 51_125_738
	if routing.MaxSearchWorkUnits != 120_000_000 {
		t.Fatalf("default route-search work limit = %d, want calibrated 120000000", routing.MaxSearchWorkUnits)
	}
	if routing.MaxSearchWorkUnits < measuredRepresentativeCorpusPeak*2 || routing.MaxSearchWorkUnits > measuredRepresentativeCorpusPeak*4 {
		t.Fatalf(
			"default route-search limit %d does not leave bounded headroom over representative corpus peak %d",
			routing.MaxSearchWorkUnits,
			measuredRepresentativeCorpusPeak,
		)
	}

	names, err := testGraphNames()
	if err != nil {
		t.Fatal(err)
	}
	telemetry := &routing.SearchTelemetry{}
	ctx := routing.WithSearchTelemetry(withTestLogger(t.Context(), t), telemetry)
	for _, name := range names {
		graph, err := readGraph(ctx, filepath.Join(layoutTestDir, name, "graph.input.json"))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := layoutWithSnapshots(ctx, graph, talaSeed, false); err != nil {
			t.Fatalf("layout %s: %v", name, err)
		}
	}

	samples := telemetry.WorkSamples()
	if len(samples) == 0 {
		t.Fatal("checked-in layout corpus exercised no route flavors")
	}
	var peak uint64
	for _, work := range samples {
		if work > peak {
			peak = work
		}
	}
	t.Logf("route search corpus: cases=%d flavor samples=%d peak=%d", len(names), len(samples), peak)
	if peak > measuredCheckedInCorpusPeak {
		t.Fatalf("route-search corpus peak %d exceeds calibrated peak %d", peak, measuredCheckedInCorpusPeak)
	}
}
