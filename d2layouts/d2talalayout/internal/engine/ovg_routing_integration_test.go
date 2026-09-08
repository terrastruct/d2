package engine

import (
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/lib/env"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/util-go/diff"
)

var ovgTestDir = filepath.Join("..", "testdata", "ovg")

func TestBuildOVGFromGraph(t *testing.T) {
	if env.SkipGraphDiffTests() {
		t.SkipNow()
	}
	t.Parallel()

	testCasesDirs, err := os.ReadDir(ovgTestDir)
	if err != nil {
		t.Fatal("Could not load test cases")
	}

	testCaseFilter := ""
	for _, file := range testCasesDirs {
		if testCaseFilter != "" && !strings.Contains(file.Name(), testCaseFilter) {
			continue
		}
		t.Run(file.Name(), compareOVGFromGraph)
	}
}

func compareOVGFromGraph(t *testing.T) {
	t.Parallel()

	testDir := filepath.Join(ovgTestDir, path.Base(t.Name()))
	ctx := log.Leveled(withTestLogger(t.Context(), t), slog.LevelError)
	graph, err := readGraph(ctx, filepath.Join(testDir, "graph.input.json"))
	if err != nil {
		t.Fatal(err)
	}

	pipeline, err := layoutWithSnapshots(ctx, graph, talaSeed, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(pipeline.snapshots) == 0 {
		t.Fatal("cannot compare OVGs because there are no snapshots")
	}

	wantGoldens := make([]string, 0, len(pipeline.snapshots))
	for index, snapshot := range pipeline.snapshots {
		goldenBase := filepath.Join(testDir, fmt.Sprintf("ovg_%d", index))
		wantGoldens = append(wantGoldens, goldenBase+".exp.json")
		if err := diff.TestdataJSON(goldenBase, snapshot.ovg); err != nil {
			t.Error(err)
		}
	}
	gotGoldens, err := filepath.Glob(filepath.Join(testDir, "ovg_*.exp.json"))
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(wantGoldens)
	if !slices.Equal(gotGoldens, wantGoldens) {
		t.Fatalf("OVG goldens = %v; want %v", gotGoldens, wantGoldens)
	}
}
