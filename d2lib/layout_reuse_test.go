package d2lib

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	d2log "github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/textmeasure"
)

func TestCompileReusesStyleOnlyScenarioAndStepGeometry(t *testing.T) {
	input := `direction: right
a: Alpha
b: Beta
c: Gamma
a -> b: one
b -> c: two

scenarios: {
  recolor: {
    a.style.fill: red
    (a -> b)[0].style.stroke: blue
  }
  faded: {
    b.style.opacity: 0.4
  }
}

steps: {
  first: {
    c.style.fill: green
  }
  second: {
    a.style.stroke-width: 5
  }
}
`

	optimized, optimizedCalls := compileForLayoutReuseTest(t, input, true)
	baseline, baselineCalls := compileForLayoutReuseTest(t, input, false)
	if optimizedCalls != 1 {
		t.Fatalf("optimized layout calls = %d, want 1", optimizedCalls)
	}
	if baselineCalls != 5 {
		t.Fatalf("baseline layout calls = %d, want 5", baselineCalls)
	}
	assertExactDiagramAndSVG(t, optimized, baseline)

	baseGeometry := diagramGeometryOf(optimized)
	for _, scenario := range optimized.Scenarios {
		if got := diagramGeometryOf(scenario); !reflect.DeepEqual(got, baseGeometry) {
			t.Fatalf("scenario %q geometry differs from base:\ngot  %#v\nwant %#v", scenario.Name, got, baseGeometry)
		}
	}
	for _, step := range optimized.Steps {
		if got := diagramGeometryOf(step); !reflect.DeepEqual(got, baseGeometry) {
			t.Fatalf("step %q geometry differs from base:\ngot  %#v\nwant %#v", step.Name, got, baseGeometry)
		}
	}
	if got := shapeByID(t, optimized.Scenarios[0], "a").Fill; got != "red" {
		t.Fatalf("scenario style output was not preserved: fill = %q, want red", got)
	}
	if got := shapeByID(t, optimized.Steps[1], "a").StrokeWidth; got != 5 {
		t.Fatalf("step style output was not preserved: stroke width = %d, want 5", got)
	}
}

func TestCompileLayoutReuseRejectsGeometryChangesAndLayers(t *testing.T) {
	input := `a -> b

layers: {
  independent: {
    a -> b
  }
}

scenarios: {
  wider: {
    a.width: 240
  }
}

steps: {
  step-wider: {
    a.width: 240
  }
  recolor: {
    a.style.fill: red
  }
}
`

	optimized, optimizedCalls := compileForLayoutReuseTest(t, input, true)
	baseline, baselineCalls := compileForLayoutReuseTest(t, input, false)
	// Root, layer, geometry-changing scenario, and first step lay out. The
	// style-only cumulative second step reuses the first step.
	if optimizedCalls != 4 {
		t.Fatalf("optimized layout calls = %d, want 4", optimizedCalls)
	}
	if baselineCalls != 5 {
		t.Fatalf("baseline layout calls = %d, want 5", baselineCalls)
	}
	assertExactDiagramAndSVG(t, optimized, baseline)

	if reflect.DeepEqual(diagramGeometryOf(optimized), diagramGeometryOf(optimized.Scenarios[0])) {
		t.Fatal("geometry-changing scenario unexpectedly matched base geometry")
	}
	if got, want := diagramGeometryOf(optimized.Steps[1]), diagramGeometryOf(optimized.Steps[0]); !reflect.DeepEqual(got, want) {
		t.Fatalf("style-only cumulative step did not reuse prior-step geometry:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestLayoutInputSignatureIgnoresOnlyGeometryNeutralStyles(t *testing.T) {
	styleOnly := `a -> b
scenarios: {
  changed: {
    a.style.fill: red
    a.style.opacity: 0.5
    a.style.shadow: true
    (a -> b)[0].style.stroke: blue
  }
}`
	_, calls := compileForLayoutReuseTest(t, styleOnly, true)
	if calls != 1 {
		t.Fatalf("style-only layout calls = %d, want 1", calls)
	}

	modifier := `a -> b
scenarios: {
  changed: {
    a.style.3d: true
  }
}`
	_, calls = compileForLayoutReuseTest(t, modifier, true)
	if calls != 2 {
		t.Fatalf("geometry-modifier layout calls = %d, want 2", calls)
	}

	_, calls = compileForLayoutReuseTestEngine(t, styleOnly, true, "custom")
	if calls != 2 {
		t.Fatalf("custom-layout calls = %d, want 2", calls)
	}
}

func TestCompileLayoutReuseRequiresExplicitStableResolver(t *testing.T) {
	input := `a
scenarios: {
  recolor: { a.style.fill: red }
}`

	engine := "dagre"
	layoutCalls := 0
	opts := &CompileOptions{
		Layout: &engine,
		Ruler:  layoutReuseTestRuler,
		// Deliberately use the built-in engine name with a resolver whose
		// behavior changes between boards. LayoutReuse defaults to false, so
		// arbitrary public resolvers retain their historical per-board calls.
		LayoutResolver: func(string) (d2graph.LayoutGraph, error) {
			return func(_ context.Context, g *d2graph.Graph) error {
				layoutCalls++
				for i, obj := range g.Objects {
					obj.TopLeft = geo.NewPoint(float64(layoutCalls*100+i), 0)
				}
				return nil
			}, nil
		},
	}
	ctx := d2log.WithDefault(context.Background())
	diagram, _, err := Compile(ctx, input, opts, &d2svg.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if layoutCalls != 2 {
		t.Fatalf("layout calls = %d, want 2", layoutCalls)
	}
	if got, want := shapeByID(t, diagram, "a").Pos.X, 100; got != want {
		t.Fatalf("base x = %d, want %d", got, want)
	}
	if got, want := shapeByID(t, diagram.Scenarios[0], "a").Pos.X, 200; got != want {
		t.Fatalf("scenario x = %d, want %d", got, want)
	}
}

func TestCompileLayoutReusePreservesPerBoardCancellation(t *testing.T) {
	input := `a -> b
scenarios: {
  recolor: { a.style.fill: red }
}`

	run := func(reuse bool) (int, error) {
		ctx, cancel := context.WithCancel(d2log.WithDefault(context.Background()))
		calls := 0
		engine := "elk"
		opts := &CompileOptions{
			Layout:      &engine,
			LayoutReuse: reuse,
			Ruler:       layoutReuseTestRuler,
			LayoutResolver: func(string) (d2graph.LayoutGraph, error) {
				return func(ctx context.Context, g *d2graph.Graph) error {
					calls++
					err := d2elklayout.DefaultLayout(ctx, g)
					if calls == 1 && err == nil {
						cancel()
					}
					return err
				}, nil
			},
		}
		_, _, err := Compile(ctx, input, opts, &d2svg.RenderOpts{})
		return calls, err
	}

	baselineCalls, baselineErr := run(false)
	reuseCalls, reuseErr := run(true)
	if baselineCalls != 2 || reuseCalls != 2 {
		t.Fatalf("layout calls without/with reuse = %d/%d, want 2/2", baselineCalls, reuseCalls)
	}
	if !errors.Is(baselineErr, context.Canceled) || !errors.Is(reuseErr, context.Canceled) {
		t.Fatalf("errors without/with reuse = %v/%v, want context cancellation", baselineErr, reuseErr)
	}
	if baselineErr.Error() != reuseErr.Error() {
		t.Fatalf("error with reuse = %q, want %q", reuseErr, baselineErr)
	}
}

func BenchmarkCompileStyleOnlyBoards(b *testing.B) {
	input := benchmarkStyleOnlyBoards(32, 8, 8)
	for _, tc := range []struct {
		name  string
		reuse bool
	}{
		{name: "reuse", reuse: true},
		{name: "full-layout", reuse: false},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			ctx := d2log.WithDefault(context.Background())
			for i := 0; i < b.N; i++ {
				calls := 0
				opts := dagreCompileOptions(&calls)
				renderOpts := &d2svg.RenderOpts{}
				opts.LayoutReuse = tc.reuse
				if _, _, err := compileInput(ctx, input, opts, renderOpts); err != nil {
					b.Fatal(err)
				}
				wantCalls := 17
				if tc.reuse {
					wantCalls = 1
				}
				if calls != wantCalls {
					b.Fatalf("layout calls = %d, want %d", calls, wantCalls)
				}
			}
		})
	}
}

func compileForLayoutReuseTest(t *testing.T, input string, allowLayoutReuse bool) (*d2target.Diagram, int) {
	return compileForLayoutReuseTestEngine(t, input, allowLayoutReuse, "dagre")
}

func compileForLayoutReuseTestEngine(t *testing.T, input string, allowLayoutReuse bool, engine string) (*d2target.Diagram, int) {
	t.Helper()
	calls := 0
	opts := dagreCompileOptions(&calls)
	opts.Layout = &engine
	opts.LayoutReuse = allowLayoutReuse
	ctx := d2log.WithDefault(context.Background())
	diagram, _, err := compileInput(ctx, input, opts, &d2svg.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	return diagram, calls
}

func dagreCompileOptions(calls *int) *CompileOptions {
	engine := "dagre"
	return &CompileOptions{
		Layout: &engine,
		Ruler:  layoutReuseTestRuler,
		LayoutResolver: func(string) (d2graph.LayoutGraph, error) {
			return func(ctx context.Context, g *d2graph.Graph) error {
				*calls++
				return d2dagrelayout.Layout(ctx, g, nil)
			}, nil
		},
	}
}

var layoutReuseTestRuler = func() *textmeasure.Ruler {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		panic(err)
	}
	return ruler
}()

func assertExactDiagramAndSVG(t *testing.T, got, want *d2target.Diagram) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatal("diagram output with layout reuse differs from full-layout output")
	}
	gotSVG, err := d2svg.RenderMultiboard(got, &d2svg.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	wantSVG, err := d2svg.RenderMultiboard(want, &d2svg.RenderOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotSVG) != len(wantSVG) {
		t.Fatalf("rendered board count = %d, want %d", len(gotSVG), len(wantSVG))
	}
	for i := range gotSVG {
		if !bytes.Equal(gotSVG[i], wantSVG[i]) {
			t.Fatalf("rendered SVG board %d differs from full-layout output", i)
		}
	}
}

type diagramGeometry struct {
	shapes      []shapeGeometry
	connections []connectionGeometry
}

type shapeGeometry struct {
	id                          string
	pos                         d2target.Point
	width, height               int
	labelPosition, iconPosition string
}

type connectionGeometry struct {
	id              string
	route           []geo.Point
	isCurve         bool
	labelPosition   string
	labelPercentage float64
}

func diagramGeometryOf(diagram *d2target.Diagram) diagramGeometry {
	result := diagramGeometry{
		shapes:      make([]shapeGeometry, len(diagram.Shapes)),
		connections: make([]connectionGeometry, len(diagram.Connections)),
	}
	for i, shape := range diagram.Shapes {
		result.shapes[i] = shapeGeometry{
			id: shape.ID, pos: shape.Pos, width: shape.Width, height: shape.Height,
			labelPosition: shape.LabelPosition, iconPosition: shape.IconPosition,
		}
	}
	for i, connection := range diagram.Connections {
		route := make([]geo.Point, len(connection.Route))
		for j, point := range connection.Route {
			route[j] = *point
		}
		result.connections[i] = connectionGeometry{
			id: connection.ID, route: route, isCurve: connection.IsCurve,
			labelPosition: connection.LabelPosition, labelPercentage: connection.LabelPercentage,
		}
	}
	return result
}

func shapeByID(t *testing.T, diagram *d2target.Diagram, id string) d2target.Shape {
	t.Helper()
	for _, shape := range diagram.Shapes {
		if shape.ID == id {
			return shape
		}
	}
	t.Fatalf("shape %q not found", id)
	return d2target.Shape{}
}

func benchmarkStyleOnlyBoards(nodes, scenarios, steps int) string {
	var b strings.Builder
	b.WriteString("direction: right\n")
	for i := 0; i < nodes; i++ {
		fmt.Fprintf(&b, "n%d: node %d\n", i, i)
		if i > 0 {
			fmt.Fprintf(&b, "n%d -> n%d\n", i-1, i)
		}
	}
	b.WriteString("scenarios: {\n")
	for i := 0; i < scenarios; i++ {
		fmt.Fprintf(&b, "s%d: { n%d.style.fill: red }\n", i, i%nodes)
	}
	b.WriteString("}\nsteps: {\n")
	for i := 0; i < steps; i++ {
		fmt.Fprintf(&b, "t%d: { n%d.style.opacity: 0.%d }\n", i, i%nodes, i+1)
	}
	b.WriteString("}\n")
	return b.String()
}
