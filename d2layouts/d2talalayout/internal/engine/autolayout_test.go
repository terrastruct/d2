//go:build !race

package engine

import (
	"path/filepath"
	"runtime/debug"
	"testing"

	"log/slog"

	"github.com/d2lang/d2/lib/log"
)

func TestAutolayoutRegression(t *testing.T) {
	t.Parallel()
	runTestCase := func(t *testing.T, filename string, seed int64) {
		t.Parallel()
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("Test case %s panic. Error %v. Stack: %v", filename, rec, string(debug.Stack()))
			}
		}()

		ctx := t.Context()
		ctx = withTestLogger(ctx, t)
		ctx = log.Leveled(ctx, slog.LevelError)
		graphPath := filepath.Join("..", "testdata", "regression", filename+".json")
		if g, err := readGraph(ctx, graphPath); err == nil {
			if _, err := layoutWithSnapshots(ctx, g, seed, false); err != nil {
				t.Fatalf("Autolayout returned an error for test case %s. Error %v", filename, err)
			} else {
				if startGraph, err := readGraph(ctx, graphPath); err != nil {
					t.Fatalf("Failed to read graph for test case %s. Error %v", filename, err)
				} else if err = checkGraphPropertiesAfterAutolayout(startGraph, g); err != nil {
					t.Fatalf("Graph comparison failed for test case %s. Error %v", filename, err)
				}
			}
		} else {
			t.Fatalf("Failed to read graph for test case %s. Error %v", filename, err)
		}
	}
	cases := []struct {
		seed     int64
		filename string
	}{
		{4397197029139916149, "container_escape_1"},
		{2314505960588799500, "container_escape_2"},
		{5902913841169428614, "container_escape_3"},
		{3171783035589032342, "container_escape_4"},
		{7778691377623283929, "container_escape_5"},
		{1636470363569335365, "container_escape_6"},
		{139599530801109251, "container_escape_7"},
		{1654996031387964605, "node_placement_shift_1"},
		{2432142976454602351, "node_placement_shift_2"},
		{4970705388244523053, "clustering_near"},
		{1475938678302850746, "cloud_port_index"},
		{123, "edge_balance_never_ending"},
		{9034163656351675339, "edge_balance_1"},
		{4429140665999086830, "edge_balance_2"},
		{8661428079105679616, "edge_balance_3"},
		{2029887799644317101, "cluster_overlap"},
		{4483618397761874234, "cluster_overlap_2"},
		{562018869481995924, "path_not_found"},
	}
	for _, c := range cases {
		t.Run(c.filename, func(t *testing.T) {
			runTestCase(t, c.filename, c.seed)
		})
	}
}
