package e2etests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/util-go/diff"
)

// Bound aggregate geometry and vector-surface memory independently of go test's
// parallelism.
var isometricRenderSlots = make(chan struct{}, 2)

// These source fixtures require artwork or glyphs outside D2's bundled
// resources. Keep checking their explicit errors without vendoring resources,
// fetching URLs, or depending on host fonts. Other renderer tests cover
// supplied assets and fallback fonts. A new error must be reviewed; a fixture
// that becomes self-contained must regain its SVG snapshot.
var isometricResourceErrors = map[string]string{
	"real_world/spyre_encoder/isometric":                  "has no available font for U+21D2; configure a font or fallback resolver covering this character",
	"regression/ampersand-escape/isometric":               "has no available font for U+2208; configure a font or fallback resolver covering this character",
	"regression/dagre_disconnected_edge/isometric":        "isometric surface icon requires an explicit asset resolver for external sources",
	"regression/elk_img_empty_label_panic/isometric":      "isometric surface icon requires an explicit asset resolver for external sources",
	"regression/glob_dimensions/isometric":                "has no available font for U+2B24; configure a font or fallback resolver covering this character",
	"regression/grid_image_label_position/isometric":      "isometric surface icon requires an explicit asset resolver for external sources",
	"regression/query_param_escape/isometric":             "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/centered_horizontal_connections/isometric":    "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/cycle-order/isometric":                        "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/dagger_grid/isometric":                        "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/dagre_spacing/isometric":                      "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/dagre_spacing_right/isometric":                "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/grid_icon/isometric":                          "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/grid_outside_labels/isometric":                "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/icon-containers/isometric":                    "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/icon-label/isometric":                         "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/icon_positions/isometric":                     "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/images/isometric":                             "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/investigate/isometric":                        "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/label-near/isometric":                         "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/label_positions/isometric":                    "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/multiple_person_label/isometric":              "has no available font for U+3042; configure a font or fallback resolver covering this character",
	"stable/overlapping_child_label/isometric":            "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/overlapping_image_container_labels/isometric": "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/sequence_diagram_all_shapes/isometric":        "isometric surface icon requires an explicit asset resolver for external sources",
	"stable/simple_grid_edges/isometric":                  "has no available font for U+2B24; configure a font or fallback resolver covering this character",
	"stable/teleport_grid/isometric":                      "isometric surface icon requires an explicit asset resolver for external sources",
	"themes/3d-sides/isometric":                           "isometric surface icon requires an explicit asset resolver for external sources",
	"themes/origami/isometric":                            "has no available font for U+901A; configure a font or fallback resolver covering this character",
	"todo/container_icon_label/isometric":                 "isometric surface icon requires an explicit asset resolver for external sources",
	"txtar/connection-icons/isometric":                    "isometric surface icon requires an explicit asset resolver for external sources",
	"txtar/icon-style/isometric":                          "isometric surface icon requires an explicit asset resolver for external sources",
	"txtar/legend-leak/isometric":                         "has no available font for U+51E1; configure a font or fallback resolver covering this character",
	"txtar/legend/isometric":                              "has no available font for U+51E1; configure a font or fallback resolver covering this character",
	"txtar/sequence-icon-label/isometric":                 "isometric surface icon requires an explicit asset resolver for external sources",
	"txtar/sql-icon/isometric":                            "isometric surface icon requires an explicit asset resolver for external sources",
	"txtar/sql-table-reserved/isometric":                  "isometric surface icon requires an explicit asset resolver for external sources",
	"txtar/unicode/isometric":                             "has no available font for U+25A1; configure a font or fallback resolver covering this character",
	"unicode/chinese/isometric":                           "has no available font for U+5E8A; configure a font or fallback resolver covering this character",
	"unicode/japanese-basic/isometric":                    "has no available font for U+3042; configure a font or fallback resolver covering this character",
	"unicode/japanese-full/isometric":                     "has no available font for U+3042; configure a font or fallback resolver covering this character",
	"unicode/japanese-mixed/isometric":                    "has no available font for U+30C8; configure a font or fallback resolver covering this character",
	"unicode/korean/isometric":                            "has no available font for U+ACE0; configure a font or fallback resolver covering this character",
	"unicode/mixed-language-2/isometric":                  "has no available font for U+6211; configure a font or fallback resolver covering this character",
	"unicode/mixed-language/isometric":                    "has no available font for U+6709; configure a font or fallback resolver covering this character",
	"unicode/with-style/isometric":                        "has no available font for U+304A; configure a font or fallback resolver covering this character",
}

type isometricBoard struct {
	name    string
	diagram *d2target.Diagram
}

func isometricBoards(diagram *d2target.Diagram) []isometricBoard {
	var boards []isometricBoard
	var walk func(*d2target.Diagram, string)
	walk = func(board *d2target.Diagram, name string) {
		if !board.IsFolderOnly {
			boards = append(boards, isometricBoard{name, board})
		}
		for i, children := range [][]*d2target.Diagram{board.Layers, board.Scenarios, board.Steps} {
			kind := []string{"layers", "scenarios", "steps"}[i]
			for j, child := range children {
				// Source order avoids unsafe or ambiguous filenames from authored
				// board names. The corresponding names remain in board.exp.json.
				walk(child, fmt.Sprintf("%s.%s.%d", name, kind, j))
			}
		}
	}
	walk(diagram, "isometric")
	return boards
}

func renderIsometric(t *testing.T, ctx context.Context, diagram *d2target.Diagram, dir string, opts *d2svg.RenderOpts) {
	t.Helper()
	if os.Getenv("SKIP_ISOMETRIC_CHECK") != "" || os.Getenv("SKIP_SVG_CHECK") != "" {
		return
	}
	isometricRenderSlots <- struct{}{}
	defer func() { <-isometricRenderSlots }()
	accept := os.Getenv("TESTDATA_ACCEPT") != "" || os.Getenv("TA") != ""
	boards := isometricBoards(diagram)
	var rendered []isometricBoard
	failed := false
	for _, board := range boards {
		options := d2isometricimg.Options{
			Render: d2isometric.RenderOpts{
				ThemeID: opts.ThemeID, ThemeOverrides: opts.ThemeOverrides,
			},
			Format: d2isometricimg.SVG,
			Width:  1200, Height: 1200, FitContent: true,
		}
		data, err := d2isometricimg.Render(ctx, board.diagram, &options)
		resourceKey := strings.TrimPrefix(t.Name(), "TestE2E/") + "/" + board.name
		if want := isometricResourceErrors[resourceKey]; want != "" {
			if err == nil {
				t.Errorf("%s no longer needs external resources; remove its resource-error expectation and add its SVG snapshot", resourceKey)
				failed = true
			} else if !strings.Contains(err.Error(), want) {
				t.Errorf("render %s SVG: got %v, want resource error containing %q", board.name, err, want)
				failed = true
			} else {
				t.Logf("%s: checked resource error instead of SVG snapshot: %v", board.name, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("render %s SVG: %v", board.name, err)
			failed = true
			continue
		}
		rendered = append(rendered, board)
		if err := diff.Testdata(filepath.Join(dir, board.name), ".svg", data); err != nil {
			t.Error(err)
		}
	}
	if failed {
		return
	}
	if err := staleIsometricSnapshots(dir, rendered, accept); err != nil {
		t.Error(err)
	}
}

// Removing a source board or switching it to a resource-error check must also
// remove its golden, so the visual report only presents current coverage.
func staleIsometricSnapshots(dir string, boards []isometricBoard, accept bool) error {
	paths, err := filepath.Glob(filepath.Join(dir, "isometric*.exp.svg"))
	if err != nil {
		return err
	}
	used := make(map[string]bool, len(boards))
	for _, board := range boards {
		used[board.name+".exp.svg"] = true
	}
	for _, path := range paths {
		name := filepath.Base(path)
		if !strings.HasPrefix(name, "isometric.") || used[name] {
			continue
		}
		if !accept {
			return fmt.Errorf("stale isometric board snapshot %s (rerun with TESTDATA_ACCEPT=1 or TA=1 to remove)", path)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := os.Remove(strings.TrimSuffix(path, ".exp.svg") + ".got.svg"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func TestIsometricBoards(t *testing.T) {
	leaf := func(name string) *d2target.Diagram { return &d2target.Diagram{Name: name} }
	a, b, c, d := leaf("../a"), leaf("../a"), leaf("same"), leaf("same")
	b.Steps = []*d2target.Diagram{d}
	root := &d2target.Diagram{IsFolderOnly: true, Layers: []*d2target.Diagram{a, b}, Scenarios: []*d2target.Diagram{c}}
	boards := isometricBoards(root)
	wantNames := []string{"isometric.layers.0", "isometric.layers.1", "isometric.layers.1.steps.0", "isometric.scenarios.0"}
	wantBoards := []*d2target.Diagram{a, b, d, c}
	if len(boards) != len(wantNames) {
		t.Fatalf("got %d boards, want %d", len(boards), len(wantNames))
	}
	for i, board := range boards {
		if board.name != wantNames[i] || board.diagram != wantBoards[i] {
			t.Fatalf("board %d: got %s, want %s", i, board.name, wantNames[i])
		}
	}
	dir := t.TempDir()
	stale := filepath.Join(dir, "isometric.steps.0.exp.svg")
	got := filepath.Join(dir, "isometric.steps.0.got.svg")
	for _, path := range []string{stale, got} {
		if err := os.WriteFile(path, []byte("old board"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := staleIsometricSnapshots(dir, boards, false); err == nil {
		t.Fatal("removed board was silently retained")
	}
	if err := staleIsometricSnapshots(dir, boards, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{stale, got} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("accept did not remove stale board snapshot %s", path)
		}
	}
}

// Exercise the actual E2E writer: accept and recheck SVG without creating PNG
// companions. Export-specific PNG snapshots live in the separate png package.
func TestIsometricSVGSnapshot(t *testing.T) {
	t.Setenv("SKIP_ISOMETRIC_CHECK", "")
	t.Setenv("SKIP_SVG_CHECK", "")
	t.Setenv("TA", "")
	t.Setenv("TESTDATA_ACCEPT", "1")
	data, err := os.ReadFile("testdata/stable/class/elk/board.exp.json")
	if err != nil {
		t.Fatal(err)
	}
	var diagram d2target.Diagram
	if err := json.Unmarshal(data, &diagram); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	renderIsometric(t, context.Background(), &diagram, dir, &d2svg.RenderOpts{})
	if info, err := os.Stat(filepath.Join(dir, "isometric.exp.svg")); err != nil || info.Size() == 0 {
		t.Fatalf("missing SVG snapshot: %v", err)
	}
	t.Setenv("TESTDATA_ACCEPT", "")
	renderIsometric(t, context.Background(), &diagram, dir, &d2svg.RenderOpts{})
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != "isometric.exp.svg" {
		t.Fatalf("SVG-only E2E writer left extra output: %v", files)
	}
	for _, flag := range []string{"SKIP_ISOMETRIC_CHECK", "SKIP_SVG_CHECK"} {
		t.Run(flag, func(t *testing.T) {
			t.Setenv(flag, "1")
			dir := t.TempDir()
			renderIsometric(t, context.Background(), &diagram, dir, &d2svg.RenderOpts{})
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatalf("skip flag left output: %v, %v", files, err)
			}
		})
	}
}
