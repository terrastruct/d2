// To accept an intentionally changed layout fixture, run:
//
//	TESTDATA_ACCEPT=1 go test ./d2layouts/d2talalayout/internal/engine -run '^TestGraphs/<case>$'
//
// Review every generated JSON change before committing it.
package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"log/slog"

	"github.com/d2lang/util-go/diff"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/env"

	"github.com/d2lang/d2/lib/log"
)

var layoutTestDir = filepath.Join("..", "testdata", "layout")

const talaSeed = 123

type pipelineTestContext struct {
	seed     int64
	name     string
	graph    *layoutgraph.Graph
	pipeline *pipeline
}

func newPipelineTestContext() *pipelineTestContext {
	return &pipelineTestContext{
		seed: talaSeed,
	}
}

func TestDeterministicNodeOrder(t *testing.T) {
	ctx := withTestLogger(t.Context(), t)
	c := newPipelineTestContext()
	c.name = "clusters"

	var nodeOrder []layoutgraph.EntityID
	iterations := 5
	for i := 0; i < iterations; i++ {
		err := c.setup(ctx)
		if err != nil {
			t.Fatal(err)
		}

		err = c.pipeline.runAllStages(ctx)
		if err != nil {
			t.Fatal(err)
		}

		currentOrder := make([]layoutgraph.EntityID, 0, len(c.graph.Nodes))
		for _, n := range c.graph.Nodes {
			currentOrder = append(currentOrder, n.ID)
		}
		if nodeOrder != nil && !slices.Equal(nodeOrder, currentOrder) {
			t.Fatalf("pipeline node order changed on iteration %d: got %v, want %v", i, currentOrder, nodeOrder)
		}
		nodeOrder = currentOrder
	}
}

func TestDeterministicTreePlacement(t *testing.T) {
	ctx := withTestLogger(t.Context(), t)
	c := newPipelineTestContext()
	c.name = "deterministic_tree_placement"

	var baseline any
	for i := 0; i < 15; i++ {
		log.Debug(ctx, "deterministic check", slog.Any("iteration", i))
		err := c.setup(ctx)
		if err != nil {
			t.Fatal(err)
		}

		err = c.pipeline.runAllStages(ctx)
		if err != nil {
			t.Fatal(err)
		}
		serialized, err := graphjson.Serialize(ctx, c.graph)
		if err != nil {
			t.Fatal(err)
		}
		if baseline != nil && !reflect.DeepEqual(baseline, serialized) {
			t.Fatalf("tree placement changed on iteration %d", i)
		}
		baseline = serialized
	}
}

func TestGraphs(t *testing.T) {
	if env.SkipGraphDiffTests() {
		t.SkipNow()
	}
	t.Parallel()

	names, err := testGraphNames()
	if err != nil {
		t.Fatal(err)
	} else if len(names) == 0 {
		t.Fatal("Could not find test graphs")
	}

	for _, name := range names {
		t.Run(name, pipelineTest)
	}
}

func testGraphNames() ([]string, error) {
	files, err := os.ReadDir(layoutTestDir)
	if err != nil {
		return nil, err
	}

	names := []string{}
	for _, f := range files {
		if f.IsDir() {
			var testFilesExist bool
			if _, err := os.Stat(filepath.Join(layoutTestDir, f.Name(), "graph.input.json")); err == nil {
				testFilesExist = true
			}
			if _, err := os.Stat(filepath.Join(layoutTestDir, f.Name(), "graph.exp.json")); err == nil {
				testFilesExist = true
			}
			if testFilesExist {
				// we want to run the test if any of the files exists..
				// just skip if both files are missing, which means with
				// high probability that the directory is not a valid test case
				names = append(names, f.Name())
			}
		}
	}
	return names, nil
}

func pipelineTest(t *testing.T) {
	t.Parallel()

	ctx := withTestLogger(t.Context(), t)
	c := newPipelineTestContext()
	c.name = filepath.Base(t.Name())

	err := c.setup(ctx)
	if err != nil {
		t.Fatal(err)
	}

	err = c.runTest(ctx)
	if err != nil {
		t.Fatal(err)
	}
}

func (c *pipelineTestContext) setup(ctx context.Context) error {
	var err error
	c.graph, err = readGraph(ctx, filepath.Join(layoutTestDir, c.name, "graph.input.json"))
	if err != nil {
		return err
	}

	c.pipeline = newPipeline(c.graph, c.seed, false)
	return nil
}

func (c *pipelineTestContext) runTest(ctx context.Context) error {
	err := c.pipeline.runAllStages(ctx)
	if err != nil {
		return err
	}

	return compareGraphGolden(ctx, filepath.Join(layoutTestDir, c.name, "graph"), c.graph)
}

func compareGraphGolden(ctx context.Context, path string, graph *layoutgraph.Graph) error {
	serialized, err := graphjson.Serialize(ctx, graph)
	if err != nil {
		return err
	}
	// The historical Graph.MarshalJSON fixture used encoding/json, whose HTML
	// escaping is part of the existing golden files. Preserve those bytes while
	// keeping serialization owned by graphjson rather than by the graph model.
	serializedJSON, err := json.Marshal(serialized)
	if err != nil {
		return err
	}
	return diff.TestdataJSON(path, json.RawMessage(serializedJSON))
}
