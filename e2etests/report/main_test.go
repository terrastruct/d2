package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fixture(t *testing.T, root, path, content string) string {
	t.Helper()
	path = filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverSnapshotsVariantsAndDelta(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "stable/example/dagre/sketch.exp.svg", "same")
	fixture(t, root, "stable/example/dagre/sketch.got.svg", "same")
	fixture(t, root, "stable/example/dagre/isometric.exp.svg", "old")
	fixture(t, root, "stable/example/dagre/isometric.got.svg", "new")
	fixture(t, root, "stable/example/dagre/isometric.layers.0.steps.1.exp.svg", "child")
	fixture(t, root, "stable/example/dagre/isometric.layers.1.got.svg", "new board")
	fixture(t, root, "regression/example/elk/sketch.exp.svg", "other set")
	fixture(t, root, "stable/example/elk/alternate.exp.svg", "legacy svg")
	fixture(t, root, "stable/example/dagre/board.exp.json", "{}")
	fixture(t, root, "stable/example/dagre/raster.exp.png", "not isometric")
	fixture(t, root, "stable/example/dagre/isometric..exp.png", "empty suffix")
	fixture(t, root, "stable/example/dagre/isometric..exp.svg", "empty suffix")
	cases := []struct {
		name  string
		opts  discoveryOptions
		names []string
	}{
		{"all", discoveryOptions{Variant: "all"}, []string{
			"regression/example/elk/sketch.svg", "stable/example/dagre/isometric.layers.0.steps.1.svg",
			"stable/example/dagre/isometric.layers.1.svg", "stable/example/dagre/isometric.svg",
			"stable/example/dagre/sketch.svg", "stable/example/elk/alternate.svg",
		}},
		{"isometric", discoveryOptions{Variant: "isometric"}, []string{
			"stable/example/dagre/isometric.layers.0.steps.1.svg", "stable/example/dagre/isometric.layers.1.svg", "stable/example/dagre/isometric.svg",
		}},
		{"sketch", discoveryOptions{Variant: "sketch"}, []string{
			"regression/example/elk/sketch.svg", "stable/example/dagre/sketch.svg", "stable/example/elk/alternate.svg",
		}},
		{"delta", discoveryOptions{Variant: "all", Delta: true}, []string{
			"stable/example/dagre/isometric.layers.1.svg", "stable/example/dagre/isometric.svg",
		}},
		{"set and case", discoveryOptions{Variant: "all", TestSet: "^regression$", TestCase: "^example$"}, []string{"regression/example/elk/sketch.svg"}},
		{"no match", discoveryOptions{Variant: "all", TestCase: "absent"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := discoverTests(root, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			ids := map[string]bool{}
			for _, item := range got {
				names = append(names, item.Name)
				if ids[item.ID] || item.ID == "" {
					t.Errorf("nonunique ID %q", item.ID)
				}
				ids[item.ID] = true
				if strings.Contains(item.Name, "layers.1.svg") && (!item.MissingExpected || item.ExpImage != "" || item.GotLabel != "Got") {
					t.Errorf("missing baseline not represented: %+v", item)
				}
				if item.Name == "stable/example/dagre/isometric.svg" && (item.ExpImage == "" || item.GotLabel != "Got") {
					t.Errorf("change not paired: %+v", item)
				}
				if item.Name == "stable/example/dagre/sketch.svg" && (item.ExpImage != "" || item.GotLabel != "Expected") {
					t.Errorf("equal got not ignored: %+v", item)
				}
			}
			if !reflect.DeepEqual(names, tc.names) {
				t.Fatalf("got %v; want %v", names, tc.names)
			}
		})
	}
}

func TestDiscoverIgnoresPNGVisualSnapshots(t *testing.T) {
	root := t.TempDir()
	for _, ext := range []string{"svg", "png"} {
		fixture(t, root, "stable/example/elk/isometric.exp."+ext, "same")
		fixture(t, root, "stable/example/elk/isometric.got."+ext, "same")
		fixture(t, root, "stable/example/elk/isometric.layers.0.exp."+ext, "before")
		fixture(t, root, "stable/example/elk/isometric.layers.0.got."+ext, "after")
		fixture(t, root, "stable/example/elk/isometric.steps.0.got."+ext, "new board")
	}
	fixture(t, root, "stable/legacy-only/dagre/isometric.exp.png", "legacy PNG")
	fixture(t, root, "stable/legacy-only/dagre/isometric.layers.0.got.png", "legacy PNG delta")
	fixture(t, root, "stable/example/elk/sketch.got.png", "PNG sketch")
	fixture(t, root, "stable/example/elk/sketch.exp.svg", "ordinary")
	for _, delta := range []bool{false, true} {
		items, err := discoverTests(root, discoveryOptions{Variant: "isometric", Delta: delta})
		if err != nil {
			t.Fatal(err)
		}
		want := 3
		if delta {
			want = 2
		}
		if len(items) != want {
			t.Fatalf("delta=%v: got %d snapshots, want %d", delta, len(items), want)
		}
		for _, item := range items {
			if item.Variant != "isometric" || strings.Contains(item.Name, "sketch") || filepath.Ext(item.GotImage) != ".svg" {
				t.Fatalf("misclassified snapshot: %+v", item)
			}
		}
	}
	items, err := discoverTests(root, discoveryOptions{Variant: "sketch"})
	if err != nil || len(items) != 1 || items[0].Name != "stable/example/elk/sketch.svg" {
		t.Fatalf("SVG isometric snapshots leaked into sketch report: %+v %v", items, err)
	}
	items, err = discoverTests(root, discoveryOptions{Variant: "all"})
	if err != nil || len(items) != 4 {
		t.Fatalf("legacy PNGs leaked into the default report: %+v %v", items, err)
	}
	reportPath := filepath.Join(t.TempDir(), "report.html")
	if err := writeReport(reportPath, items); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), ".png") || strings.Contains(string(output), "legacy-only") {
		t.Fatal("SVG report contains a PNG visual link or legacy PNG-only case")
	}
}

func TestDiscoverSnapshotErrors(t *testing.T) {
	for _, opts := range []discoveryOptions{{Variant: "png"}, {Variant: "all", TestSet: "["}, {Variant: "all", TestCase: "["}} {
		if _, err := discoverTests(t.TempDir(), opts); err == nil {
			t.Errorf("accepted %+v", opts)
		}
	}
	if _, err := discoverTests(filepath.Join(t.TempDir(), "absent"), discoveryOptions{Variant: "all"}); err == nil {
		t.Fatal("ignored missing snapshot tree")
	}
}

func TestReportRelativeLinksAndEscaping(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "testdata")
	fixture(t, data, "stable/space # & <case>/elk/isometric.got.svg", "new")
	items, err := discoverTests(data, discoveryOptions{Variant: "all", Delta: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items", len(items))
	}
	original := items[0].GotImage
	path := filepath.Join(root, "review", "nested", "report.html")
	if err := writeReport(path, items); err != nil {
		t.Fatal(err)
	}
	if items[0].GotImage != original {
		t.Fatal("writeReport mutated discovery results")
	}
	output, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(output)
	for _, want := range []string{"Missing expected snapshot", "space # &amp; &lt;case&gt;", "../../testdata/stable/space%20%23%20&amp;%20%3Ccase%3E/elk/isometric.got.svg", `id="test-1"`, `alt="Got `} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
	if strings.Contains(html, root) || strings.Contains(html, "ZgotmplZ") {
		t.Fatal("absolute or unsafe link in report")
	}
	if err := writeReport(path, nil); err != nil {
		t.Fatal(err)
	}
	output, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "No snapshots match") || strings.Contains(string(output), "Missing expected") {
		t.Fatal("empty report left stale results")
	}
}

func TestDiscoverChangedSVG(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "stable/diagram/dagre/sketch.exp.svg", "old SVG")
	fixture(t, root, "stable/diagram/dagre/sketch.got.svg", "new SVG")
	items, err := discoverTests(root, discoveryOptions{Variant: "sketch", Delta: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ExpImage == "" || items[0].GotLabel != "Got" || items[0].MissingExpected {
		t.Fatalf("changed SVG should have old/new pair: %+v", items)
	}
}

func TestDiscoverLayoutFreeAndNestedCases(t *testing.T) {
	root := t.TempDir()
	fixture(t, root, "asciitxtar/focused/sketch.exp.svg", "ascii svg")
	fixture(t, root, "asciitxtar/other/sketch.exp.svg", "other")
	fixture(t, root, "asciitxtar/nested/focused/sketch.exp.svg", "nested ascii")
	fixture(t, root, "asciitxtar/nested/elk/sketch.exp.svg", "case named elk")
	fixture(t, root, "stable/group/focused/dagre/sketch.exp.svg", "nested svg")
	fixture(t, root, "stable/group/focused/dagre/isometric.exp.svg", "nested dagre")
	fixture(t, root, "stable/group/focused/elk/isometric.exp.svg", "nested elk")
	fixture(t, root, "stable/group/other/elk/isometric.exp.svg", "other nested case")
	for _, tc := range []struct {
		set, testcase, variant string
		names                  []string
	}{
		{"^asciitxtar$", "^focused$", "all", []string{"asciitxtar/focused/sketch.svg"}},
		{"^asciitxtar$", "^nested/focused$", "sketch", []string{"asciitxtar/nested/focused/sketch.svg"}},
		{"^asciitxtar$", "^nested/elk$", "sketch", []string{"asciitxtar/nested/elk/sketch.svg"}},
		{"^asciitxtar$", "", "isometric", nil},
		{"^stable$", "^group/focused$", "isometric", []string{"stable/group/focused/dagre/isometric.svg", "stable/group/focused/elk/isometric.svg"}},
		{"^stable$", "^group/focused$", "sketch", []string{"stable/group/focused/dagre/sketch.svg"}},
	} {
		t.Run(tc.set+tc.testcase+tc.variant, func(t *testing.T) {
			items, err := discoverTests(root, discoveryOptions{Variant: tc.variant, TestSet: tc.set, TestCase: tc.testcase})
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, item := range items {
				names = append(names, item.Name)
			}
			if !reflect.DeepEqual(names, tc.names) {
				t.Fatalf("got %v; want %v", names, tc.names)
			}
		})
	}
}
