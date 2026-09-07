package d2target

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"testing"
)

func TestHashJSONWithoutIconsMatchesNormalization(t *testing.T) {
	labels := []string{
		"", `quoted "icon":{"Scheme":"fake"}`, `escaped \\"icon":{`,
		"<script>\u2028emoji: 🌐\x00", string([]byte{0xff, '\\', '"'}),
	}
	icons := []*url.URL{nil, {}, {Scheme: "https", Host: "example.com", Path: `/"icon":{`, RawQuery: "a=<b>&c=d"}}
	for _, label := range labels {
		for _, icon := range icons {
			shape := Shape{Icon: icon, Text: Text{Label: label}, Class: Class{Fields: []ClassField{{Name: label}}}}
			connection := Connection{Icon: icon, Text: Text{Label: label}, SrcLabel: &Text{Label: label}}
			for _, value := range []any{shape, []Shape{shape, {}}, []Connection{connection, {}}, []Shape(nil), []Connection{}} {
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				want, err := stabilizeHashURLs(raw)
				if err != nil {
					t.Fatal(err)
				}
				got, err := marshalHashJSON(value)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("hash JSON differs for %T, label %q, icon %v", value, label, icon)
				}
			}
		}
	}
	if _, err := marshalHashJSON(Shape{Opacity: math.NaN()}); err == nil {
		t.Fatal("hash JSON accepted non-finite geometry")
	}
}

func TestCorpusOrderAndAppendixNumbering(t *testing.T) {
	diagram := Diagram{
		Shapes: []Shape{
			{Text: Text{Label: "shape"}, Tooltip: "tip", Link: "link", PrettyLink: "pretty"},
			{Type: ShapeClass, Text: Text{Label: "class"}, Class: Class{
				Fields:  []ClassField{{Name: "field", Type: "type", Visibility: "private"}},
				Methods: []ClassMethod{{Name: "method", Return: "return", Visibility: "protected"}},
			}},
			{Type: ShapeSQLTable, Text: Text{Label: "table"}, SQLTable: SQLTable{Columns: []SQLColumn{
				{Name: Text{Label: "column"}, Type: Text{Label: "integer"}, Constraint: []string{"primary_key", "unique"}},
			}}},
			{Text: Text{Label: "unicode🌐"}, Tooltip: "last"},
		},
		Connections: []Connection{
			{Text: Text{Label: "edge"}, SrcLabel: &Text{Label: "source"}, DstLabel: &Text{Label: "destination"}},
		},
		Legend: &Legend{Shapes: []Shape{{Text: Text{Label: "legend-shape"}}}, Connections: []Connection{{Text: Text{Label: "legend-edge"}}}},
	}
	// SQL constraints intentionally appear twice, matching the corpus used by
	// existing font subsets, and appendix numbering counts only links/tooltips.
	const want = "shapetip1link2prettyclassfieldtype-methodreturn#tablecolumnintegerPK, UNQPK, UNQunicode🌐last3edgesourcedestinationLegendlegend-shapelegend-edge"
	if got := diagram.GetCorpus(); got != want {
		t.Fatalf("corpus = %q, want %q", got, want)
	}
	diagram.Legend.Label = "named"
	if got := diagram.GetCorpus(); got != strings.Replace(want, "Legend", "named", 1) {
		t.Fatalf("named legend corpus = %q", got)
	}
}

func TestNestedCorpusTraversal(t *testing.T) {
	board := func(label string) *Diagram {
		return &Diagram{Shapes: []Shape{{Text: Text{Label: label}, Tooltip: "tip"}}}
	}
	diagram := board("root")
	diagram.Layers = []*Diagram{board("layer")}
	diagram.Layers[0].Scenarios = []*Diagram{board("nested")}
	diagram.Scenarios = []*Diagram{board("scenario")}
	diagram.Steps = []*Diagram{board("step")}
	const want = "roottip1layertip1nestedtip1scenariotip1steptip1"
	if got := diagram.GetNestedCorpus(); got != want {
		t.Fatalf("nested corpus = %q, want %q", got, want)
	}
	if got := (Diagram{}).GetNestedCorpus(); got != "" {
		t.Fatalf("empty corpus = %q", got)
	}
}

func BenchmarkDiagramCorpus(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			diagram := Diagram{Shapes: make([]Shape, count)}
			for i := range diagram.Shapes {
				diagram.Shapes[i].Label = fmt.Sprintf("service_%04d: process request", i)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = diagram.GetCorpus()
			}
		})
	}
}

func BenchmarkDiagramHashJSON(b *testing.B) {
	for _, icons := range []bool{false, true} {
		b.Run(fmt.Sprintf("icons=%t", icons), func(b *testing.B) {
			shapes := make([]Shape, 1000)
			for i := range shapes {
				shapes[i].Label = fmt.Sprintf("service_%04d: process request", i)
			}
			if icons {
				shapes[0].Icon = &url.URL{Scheme: "https", Host: "example.com", Path: "/icon.svg"}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := marshalHashJSON(shapes); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
