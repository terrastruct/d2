package d2elklayout_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/log"
)

type expectedPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type expectedObject struct {
	ID     string  `json:"id"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type expectedEdge struct {
	ID    string          `json:"id"`
	Route []expectedPoint `json:"route"`
}

type expectedLayout struct {
	Algorithm string           `json:"algorithm"`
	Objects   []expectedObject `json:"objects"`
	Edges     []expectedEdge   `json:"edges"`
}

// This fixture freezes D2's post-layout geometry after elk-go has passed its
// separate differential gates against the official ELK.js 0.12.0 asset. It is
// a D2 regression fixture, not an independently generated ELK.js oracle.
func TestD2ExposedAlgorithmsMatchFrozenElkGo012Profile(t *testing.T) {
	const fixturePath = "testdata/elk_go_0_12_d2_algorithms.json"
	expectedData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var expectedLayouts []expectedLayout
	if err := json.Unmarshal(expectedData, &expectedLayouts); err != nil {
		t.Fatal(err)
	}
	expectedByAlgorithm := make(map[string]expectedLayout, len(expectedLayouts))
	for _, expected := range expectedLayouts {
		expectedByAlgorithm[expected.Algorithm] = expected
	}

	testCases := []struct {
		algorithm     string
		input         string
		deterministic bool
	}{
		// These algorithms consume existing or coincident positions and do not
		// require an edge to establish their input model.
		{algorithm: "fixed", input: "a\nb\nc\n", deterministic: true},
		{algorithm: "box", input: "a\nb\nc\n", deterministic: true},
		{algorithm: "random", input: "a\nb\nc\n"},
		{algorithm: "sporeOverlap", input: "a\nb\nc\n"},
		{algorithm: "sporeCompaction", input: "a\nb\nc\n"},
		{algorithm: "rectpacking", input: "a\nb\nc\n", deterministic: true},

		// Exercise routed-edge readback for algorithms that lay out connected
		// graphs. The directed fixture is also a tree, as required by radial.
		{algorithm: "layered", input: "a -> b\na -> c\nc -> d\n", deterministic: true},
		{algorithm: "stress", input: "a -> b\na -> c\nc -> d\n", deterministic: true},
		{algorithm: "mrtree", input: "a -> b\na -> c\nc -> d\n", deterministic: true},
		{algorithm: "radial", input: "a -> b\na -> c\nc -> d\n", deterministic: true},
		{algorithm: "force", input: "a -> b\na -> c\nc -> d\n", deterministic: true},
	}

	accept := os.Getenv("TESTDATA_ACCEPT") == "1"
	actualLayouts := make([]expectedLayout, 0, len(expectedLayouts))
	for _, tc := range testCases {
		t.Run(tc.algorithm, func(t *testing.T) {
			g, _, err := d2compiler.Compile("", strings.NewReader(tc.input), nil)
			if err != nil {
				t.Fatalf("compile fixture: %v", err)
			}

			// Supplying concrete, deliberately non-uniform boxes keeps this test
			// independent of text measurement and covers shape geometry readback.
			for i, obj := range g.Objects {
				obj.Box = geo.NewBox(
					geo.NewPoint(float64(17+i*113), float64(29+(i%2)*71)),
					float64(60+i*7),
					float64(40+i*5),
				)
			}

			opts := d2elklayout.DefaultOpts
			opts.Algorithm = tc.algorithm
			ctx := log.With(context.Background(), testlog.New(t))
			if err := d2elklayout.Layout(ctx, g, &opts); err != nil {
				t.Fatalf("layout with %q: %v", tc.algorithm, err)
			}

			for _, obj := range g.Objects {
				if obj.Box == nil || obj.TopLeft == nil {
					t.Fatalf("object %q has no layout box", obj.AbsID())
				}
				assertFinite(t, fmt.Sprintf("object %q x", obj.AbsID()), obj.TopLeft.X)
				assertFinite(t, fmt.Sprintf("object %q y", obj.AbsID()), obj.TopLeft.Y)
				assertFinite(t, fmt.Sprintf("object %q width", obj.AbsID()), obj.Width)
				assertFinite(t, fmt.Sprintf("object %q height", obj.AbsID()), obj.Height)
				if obj.Width <= 0 || obj.Height <= 0 {
					t.Fatalf("object %q has non-positive dimensions %gx%g", obj.AbsID(), obj.Width, obj.Height)
				}
			}

			for _, edge := range g.Edges {
				if len(edge.Route) < 2 {
					t.Fatalf("edge %q has no routed geometry", edge.AbsID())
				}
				for i, point := range edge.Route {
					if point == nil {
						t.Fatalf("edge %q route point %d is nil", edge.AbsID(), i)
					}
					assertFinite(t, fmt.Sprintf("edge %q route point %d x", edge.AbsID(), i), point.X)
					assertFinite(t, fmt.Sprintf("edge %q route point %d y", edge.AbsID(), i), point.Y)
				}
			}

			if !tc.deterministic {
				// Random and both SPOrE providers deliberately perturb coincident
				// input with Math.random in elkjs. Their exact coordinates were
				// never stable, so the finite-geometry checks above are the useful
				// compatibility contract for those three algorithms.
				return
			}

			actual := expectedLayout{Algorithm: tc.algorithm}
			for _, obj := range g.Objects {
				actual.Objects = append(actual.Objects, expectedObject{
					ID:     obj.AbsID(),
					X:      obj.TopLeft.X,
					Y:      obj.TopLeft.Y,
					Width:  obj.Width,
					Height: obj.Height,
				})
			}
			for _, edge := range g.Edges {
				actualEdge := expectedEdge{ID: edge.AbsID()}
				for _, point := range edge.Route {
					actualEdge.Route = append(actualEdge.Route, expectedPoint{X: point.X, Y: point.Y})
				}
				actual.Edges = append(actual.Edges, actualEdge)
			}
			actualLayouts = append(actualLayouts, actual)
			if accept {
				return
			}

			expected, ok := expectedByAlgorithm[tc.algorithm]
			if !ok {
				t.Fatalf("no frozen D2/elk-go 0.12 profile fixture for %q", tc.algorithm)
			}
			if len(g.Objects) != len(expected.Objects) {
				t.Fatalf("objects = %d, want %d", len(g.Objects), len(expected.Objects))
			}
			for i, obj := range g.Objects {
				want := expected.Objects[i]
				if obj.AbsID() != want.ID {
					t.Fatalf("object %d id = %q, want %q", i, obj.AbsID(), want.ID)
				}
				assertNear(t, fmt.Sprintf("object %q x", want.ID), obj.TopLeft.X, want.X, 1e-7)
				assertNear(t, fmt.Sprintf("object %q y", want.ID), obj.TopLeft.Y, want.Y, 1e-7)
				assertNear(t, fmt.Sprintf("object %q width", want.ID), obj.Width, want.Width, 1e-7)
				assertNear(t, fmt.Sprintf("object %q height", want.ID), obj.Height, want.Height, 1e-7)
			}
			if len(g.Edges) != len(expected.Edges) {
				t.Fatalf("edges = %d, want %d", len(g.Edges), len(expected.Edges))
			}
			for i, edge := range g.Edges {
				want := expected.Edges[i]
				if edge.AbsID() != want.ID {
					t.Fatalf("edge %d id = %q, want %q", i, edge.AbsID(), want.ID)
				}
				if len(edge.Route) != len(want.Route) {
					t.Fatalf("edge %q route length = %d, want %d", want.ID, len(edge.Route), len(want.Route))
				}
				for pointIndex, point := range edge.Route {
					assertNear(t, fmt.Sprintf("edge %q point %d x", want.ID, pointIndex), point.X, want.Route[pointIndex].X, 1e-7)
					assertNear(t, fmt.Sprintf("edge %q point %d y", want.ID, pointIndex), point.Y, want.Route[pointIndex].Y, 1e-7)
				}
			}
		})
	}
	if accept {
		var actualData bytes.Buffer
		encoder := json.NewEncoder(&actualData)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(actualLayouts); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixturePath, actualData.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDisCoIsUnsupportedInELKJS0120(t *testing.T) {
	g, _, err := d2compiler.Compile("", strings.NewReader("a\nb\nc\n"), nil)
	if err != nil {
		t.Fatalf("compile fixture: %v", err)
	}
	for i, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(float64(17+i*113), float64(29+(i%2)*71)), 60, 40)
	}
	opts := d2elklayout.DefaultOpts
	opts.Algorithm = "disco"
	err = d2elklayout.Layout(log.WithTB(context.Background(), t), g, &opts)
	if err == nil {
		t.Fatal("DisCo layout unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), `layout algorithm "disco" not found`) {
		t.Fatalf("DisCo error = %q, want an unsupported-algorithm error", err)
	}
}

func assertNear(t *testing.T, name string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.15g, want %.15g (tolerance %.3g)", name, got, want, tolerance)
	}
}

func assertFinite(t *testing.T, name string, value float64) {
	t.Helper()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		t.Fatalf("%s is not finite: %g", name, value)
	}
}
