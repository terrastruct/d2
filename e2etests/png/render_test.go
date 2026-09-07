package png_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2target"
)

// The main E2E suite checks compilation, layout and SVG for the entire corpus.
// This separate bundle checks native PNG pixels for representative compiled
// boards. Assets and font fallback have dedicated renderer and CLI tests.
func TestPNG(t *testing.T) {
	for _, fixture := range []string{
		"stable/all_shapes/elk",
		"patterns/all_shapes/dagre",
		"stable/class_and_sqlTable_border_radius/elk",
		"stable/sql_table_row_connections/elk",
		"stable/sequence_diagram_real/elk",
		"stable/nested_diagram_types/elk",
		"stable/transparent_3d/elk",
		"regression/opacity-on-label/elk",
		"themes/dark_terrastruct_flagship/dagre",
		"themes/terminal/elk",
		"stable/complex-layers/elk",
	} {
		t.Run(fixture, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "testdata", fixture, "board.exp.json"))
			if err != nil {
				t.Fatal(err)
			}
			var diagram d2target.Diagram
			if err := json.Unmarshal(data, &diagram); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join("testdata", fixture)
			accept := os.Getenv("TESTDATA_ACCEPT") != "" || os.Getenv("TA") != ""
			used := make(map[string]bool)
			// Render sequentially to bound supersampled color/depth/shadow buffers.
			var walk func(*d2target.Diagram, string)
			walk = func(board *d2target.Diagram, name string) {
				if !board.IsFolderOnly {
					used[name+".exp.png"] = true
					t.Run(name, func(t *testing.T) {
						data, err := d2isometricimg.Render(context.Background(), board, &d2isometricimg.Options{
							Format: d2isometricimg.PNG,
							Width:  1200, Height: 1200, FitContent: true,
						})
						if err != nil {
							t.Fatal(err)
						}
						if err := pngSnapshot(filepath.Join(dir, name), data, accept); err != nil {
							t.Error(err)
						}
					})
				}
				for i, children := range [][]*d2target.Diagram{board.Layers, board.Scenarios, board.Steps} {
					kind := []string{"layers", "scenarios", "steps"}[i]
					for j, child := range children {
						walk(child, fmt.Sprintf("%s.%s.%d", name, kind, j))
					}
				}
			}
			walk(&diagram, "isometric")
			paths, err := filepath.Glob(filepath.Join(dir, "isometric*.exp.png"))
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range paths {
				if used[filepath.Base(path)] {
					continue
				}
				if !accept {
					t.Errorf("stale PNG board snapshot %s (rerun with TESTDATA_ACCEPT=1 or TA=1 to remove)", path)
					continue
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(strings.TrimSuffix(path, ".exp.png") + ".got.png"); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
		})
	}
}
