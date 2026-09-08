package d2talalayout

import (
	"context"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/graphjson"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/shape"
)

func TestScoreCompare(t *testing.T) {
	tests := []struct {
		name  string
		left  layoutScore
		right layoutScore
		want  int
	}{
		{name: "lower penalty", left: layoutScore{penalty: 1, area: 100}, right: layoutScore{penalty: 2, area: 1}, want: -1},
		{name: "higher penalty", left: layoutScore{penalty: 2, area: 1}, right: layoutScore{penalty: 1, area: 100}, want: 1},
		{name: "smaller area", left: layoutScore{penalty: 1, area: 10}, right: layoutScore{penalty: 1, area: 20}, want: -1},
		{name: "larger area", left: layoutScore{penalty: 1, area: 20}, right: layoutScore{penalty: 1, area: 10}, want: 1},
		{name: "fractional area", left: layoutScore{penalty: 1, area: 10.25}, right: layoutScore{penalty: 1, area: 10.5}, want: -1},
		{name: "large area", left: layoutScore{penalty: 1, area: 1e18}, right: layoutScore{penalty: 1, area: 1e18 + 256}, want: -1},
		{name: "equivalent", left: layoutScore{penalty: 1, area: 10}, right: layoutScore{penalty: 1, area: 10}, want: 0},
		{name: "finite before NaN", left: layoutScore{penalty: 1}, right: layoutScore{penalty: math.NaN()}, want: -1},
		{name: "NaN after finite", left: layoutScore{penalty: math.NaN()}, right: layoutScore{penalty: 1}, want: 1},
		{name: "invalid penalties equivalent", left: layoutScore{penalty: math.Inf(1)}, right: layoutScore{penalty: math.NaN()}, want: 0},
		{name: "valid area before NaN", left: layoutScore{penalty: 1, area: 10}, right: layoutScore{penalty: 1, area: math.NaN()}, want: -1},
		{name: "NaN area after valid", left: layoutScore{penalty: 1, area: math.NaN()}, right: layoutScore{penalty: 1, area: 10}, want: 1},
		{name: "valid area before infinity", left: layoutScore{penalty: 1, area: 10}, right: layoutScore{penalty: 1, area: math.Inf(1)}, want: -1},
		{name: "valid area before negative", left: layoutScore{penalty: 1, area: 10}, right: layoutScore{penalty: 1, area: -1}, want: -1},
		{name: "invalid areas equivalent", left: layoutScore{penalty: 1, area: math.Inf(-1)}, right: layoutScore{penalty: 1, area: math.NaN()}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.left.compare(tt.right); got != tt.want {
				t.Fatalf("Compare() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestEvaluateSeedRejectsInvalidCompletedGraphs(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*layoutgraph.Graph)
		want   string
	}{
		{
			name: "non-finite node position",
			mutate: func(graph *layoutgraph.Graph) {
				graph.Nodes[0].TopLeft.X = math.NaN()
			},
			want: "node x",
		},
		{
			name: "non-positive node dimension",
			mutate: func(graph *layoutgraph.Graph) {
				graph.Nodes[0].Width = 0
			},
			want: "node width",
		},
		{
			name: "incomplete edge route",
			mutate: func(graph *layoutgraph.Graph) {
				graph.Edges[0].Points = nil
			},
			want: "incomplete route",
		},
		{
			name: "non-finite route point",
			mutate: func(graph *layoutgraph.Graph) {
				graph.Edges[0].Points[0].Y = math.Inf(1)
			},
			want: "route y",
		},
		{
			name: "invalid label percentage",
			mutate: func(graph *layoutgraph.Graph) {
				graph.Edges[0].LabelPercentage = math.NaN()
			},
			want: "label percentage",
		},
		{
			name: "foreign endpoint",
			mutate: func(graph *layoutgraph.Graph) {
				graph.Edges[0].From = nil
			},
			want: "source outside",
		},
		{
			name: "duplicate node ID",
			mutate: func(graph *layoutgraph.Graph) {
				graph.Nodes[1].ID = graph.Nodes[0].ID
			},
			want: "node ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt, err := layoutgraph.Clone(t.Context(), completed)
			if err != nil {
				t.Fatal(err)
			}
			tt.mutate(attempt)
			if _, err := evaluateSeedResult(t.Context(), input, attempt); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("EvaluateSeed() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestEvaluateSeedCancellationPrecedesValidation(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	completed.Edges[0].Points = nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := evaluateSeedResult(ctx, input, completed); !errors.Is(err, context.Canceled) {
		t.Fatalf("EvaluateSeed() error = %v, want context.Canceled", err)
	}
}

func TestEvaluateSeedRejectsDerivedEvaluationWork(t *testing.T) {
	graph := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 20, 20)
	b := layoutgraph.NewNode(2, 20, 20)
	a.TopLeft = geo.NewPoint(0, 0)
	b.TopLeft = geo.NewPoint(100, 100)
	graph.AddNewNodeToContainer(nil, a)
	graph.AddNewNodeToContainer(nil, b)

	// 7,100 one-segment edges fit comfortably inside the record and route-point
	// limits, but pairwise crossing evaluation would otherwise perform more than
	// 50 million derived checks. Result evaluation must enforce the engine
	// budget before entering the quadratic scoring kernel.
	for i := 0; i < 7_100; i++ {
		edge := graph.Connect(a, b)
		edge.ID = layoutgraph.EntityID(i + 1)
		edge.Points = []*geo.Point{
			geo.NewPoint(0, float64(i)),
			geo.NewPoint(100, float64(i+1)),
		}
	}
	input := seedInput{graph: graph}
	_, err := evaluateSeedResult(t.Context(), input, graph)
	if err == nil || !strings.Contains(err.Error(), "TALA Evaluate work exceeds limit 50000000") {
		t.Fatalf("EvaluateSeed() error = %v, want derived evaluation work limit", err)
	}
}

func TestEvaluateSeedRejectsChangedTopology(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	completed.Edges[0].From = completed.Edges[0].To
	if _, err := evaluateSeedResult(t.Context(), input, completed); err == nil || !strings.Contains(err.Error(), "endpoints changed") {
		t.Fatalf("EvaluateSeed() error = %v, want endpoint-topology rejection", err)
	}
}

func TestEvaluateSeedRejectsChangedImmutableGeometryMetadata(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	input.graph.IsRootHierarchy = true
	input.graph.Directions[nil] = geo.Right
	for _, node := range input.graph.Nodes {
		node.SetShape("Table")
		node.SetNumColumns(2)
		node.Label = &layoutgraph.Label{Text: "node", Position: label.InsideTopLeft, Width: 30, Height: 12}
		node.Icon = &layoutgraph.Icon{Position: label.OutsideTopLeft}
	}
	input.graph.Nodes[0].FixedTopLeft = geo.NewPoint(40, -20)
	input.graph.Nodes[0].DesiredWidth = new(140.)
	input.graph.Nodes[0].DesiredHeight = new(90.)
	input.graph.Nodes[0].Width = *input.graph.Nodes[0].DesiredWidth
	input.graph.Nodes[0].Height = *input.graph.Nodes[0].DesiredHeight
	input.graph.Nodes[0].ForceHierarchy = true
	edge := input.graph.Edges[0]
	edge.Label = &layoutgraph.Label{Text: "edge", Width: 28, Height: 10}
	edge.SourceArrowhead = layoutgraph.Arrowhead(d2target.DiamondArrowhead)
	edge.SourceArrowheadLabel = &layoutgraph.Label{Text: "source", Width: 20, Height: 10}
	edge.TargetArrowheadLabel = &layoutgraph.Label{Text: "target", Width: 20, Height: 10}
	edge.MinWidth = 64
	edge.MinHeight = 32
	edge.FromTableColumnIndex = new(0)
	edge.ToTableColumnIndex = new(1)
	style := reflect.ValueOf(&edge.Style).Elem()
	styleScalarType := reflect.TypeFor[*layoutgraph.StyleScalar]()
	for fieldIndex := range style.NumField() {
		if style.Field(fieldIndex).Type() != styleScalarType {
			t.Fatalf("EdgeStyle.%s has type %v, want %v", style.Type().Field(fieldIndex).Name, style.Field(fieldIndex).Type(), styleScalarType)
		}
		style.Field(fieldIndex).Set(reflect.ValueOf(&layoutgraph.StyleScalar{Value: "0"}))
	}

	completed, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evaluateSeedResult(t.Context(), input, completed); err != nil {
		t.Fatalf("uncorrupted result rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*layoutgraph.Graph)
		want   string
	}{
		{name: "root hierarchy", mutate: func(graph *layoutgraph.Graph) { graph.IsRootHierarchy = false }, want: "root hierarchy metadata"},
		{name: "graph direction", mutate: func(graph *layoutgraph.Graph) { graph.Directions[nil] = geo.Left }, want: "graph direction metadata"},
		{name: "node shape", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].SetShape("Circle") }, want: "shape changed"},
		{name: "node container", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].Container = graph.Nodes[1] }, want: "container ownership changed"},
		{name: "node table columns", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].SetNumColumns(3) }, want: "table column count"},
		{name: "node 3D outline", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].Is3D = true }, want: "3D outline"},
		{name: "node multiple outline", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].IsMultiple = true }, want: "multiple outline"},
		{name: "node visibility", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].IsInvisible = true }, want: "visibility metadata"},
		{name: "node fixed position metadata", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].FixedTopLeft.X++ }, want: "fixed position metadata"},
		{name: "node desired width metadata", mutate: func(graph *layoutgraph.Graph) { *graph.Nodes[0].DesiredWidth++ }, want: "desired-size metadata"},
		{name: "node desired height metadata", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].DesiredHeight = nil }, want: "desired-size metadata"},
		{name: "node forced hierarchy", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].ForceHierarchy = false }, want: "forced-hierarchy metadata"},
		{name: "node label text", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].Label.Text = "replacement" }, want: "label identity"},
		{name: "node label presence", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].Label = nil }, want: "label identity"},
		{name: "node fixed label position", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].Label.Position = label.InsideBottomRight }, want: "fixed label position"},
		{name: "node icon presence", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].Icon = nil }, want: "icon presence"},
		{name: "node fixed icon position", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].Icon.Position = label.OutsideBottomRight }, want: "fixed icon position"},
		{name: "node font size presence", mutate: func(graph *layoutgraph.Graph) { graph.Nodes[0].FontSize = nil }, want: "font size is outside"},
		{name: "node font size domain", mutate: func(graph *layoutgraph.Graph) { *graph.Nodes[0].FontSize = 17 }, want: "font size is outside"},
		{name: "edge arrowhead", mutate: func(graph *layoutgraph.Graph) {
			graph.Edges[0].SourceArrowhead = layoutgraph.Arrowhead(d2target.TriangleArrowhead)
		}, want: "arrowhead metadata"},
		{name: "source arrowhead label", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].SourceArrowheadLabel.Width++ }, want: "source arrowhead label"},
		{name: "target arrowhead label", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].TargetArrowheadLabel.Text = "replacement" }, want: "target arrowhead label"},
		{name: "edge minimum geometry", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].MinWidth++ }, want: "minimum geometry"},
		{name: "edge source table column", mutate: func(graph *layoutgraph.Graph) { *graph.Edges[0].FromTableColumnIndex = 1 }, want: "table column attachment"},
		{name: "edge target table column", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].ToTableColumnIndex = nil }, want: "table column attachment"},
		{name: "edge visibility", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].IsInvisible = true }, want: "visibility metadata"},
		{name: "edge style", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].Style.Stroke = &layoutgraph.StyleScalar{Value: "red"} }, want: "style metadata"},
		{name: "edge label text", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].Label.Text = "replacement" }, want: "label identity"},
		{name: "edge label width", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].Label.Width++ }, want: "label identity or dimensions"},
		{name: "edge label height", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].Label.Height++ }, want: "label identity or dimensions"},
		{name: "edge label presence", mutate: func(graph *layoutgraph.Graph) { graph.Edges[0].Label = nil }, want: "label identity"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attempt, err := layoutgraph.Clone(t.Context(), completed)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(attempt)
			_, err = evaluateSeedResult(t.Context(), input, attempt)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("EvaluateSeed() error = %v, want substring %q", err, test.want)
			}
		})
	}
	t.Run("every edge style field", func(t *testing.T) {
		styleType := reflect.TypeFor[layoutgraph.EdgeStyle]()
		styleScalarType := reflect.TypeFor[*layoutgraph.StyleScalar]()
		for fieldIndex := range styleType.NumField() {
			field := styleType.Field(fieldIndex)
			t.Run(field.Name, func(t *testing.T) {
				if field.Type != styleScalarType {
					t.Fatalf("EdgeStyle.%s has type %v, want %v", field.Name, field.Type, styleScalarType)
				}
				attempt, err := layoutgraph.Clone(t.Context(), completed)
				if err != nil {
					t.Fatal(err)
				}
				style := reflect.ValueOf(&attempt.Edges[0].Style).Elem()
				value := "corrupt"
				if current, _ := style.Field(fieldIndex).Interface().(*layoutgraph.StyleScalar); current != nil && current.Value == value {
					value = "different"
				}
				style.Field(fieldIndex).Set(reflect.ValueOf(&layoutgraph.StyleScalar{Value: value}))

				_, err = evaluateSeedResult(t.Context(), input, attempt)
				if err == nil || !strings.Contains(err.Error(), "style metadata") {
					t.Fatalf("EvaluateSeed() error = %v, want complete style-metadata rejection", err)
				}
			})
		}
	})
	t.Run("font size output domain", func(t *testing.T) {
		attempt, err := layoutgraph.Clone(t.Context(), completed)
		if err != nil {
			t.Fatal(err)
		}
		*attempt.Nodes[0].FontSize = d2fonts.FONT_SIZE_L
		if _, err := evaluateSeedResult(t.Context(), input, attempt); err != nil {
			t.Fatalf("valid layout font size rejected: %v", err)
		}
	})
}

func TestValidateLayoutResultMetadataDoesNotValidateOutputConstraints(t *testing.T) {
	input := layoutgraph.NewGraph()
	node := layoutgraph.NewNode(1, 140, 90)
	node.TopLeft = geo.NewPoint(40, -20)
	node.FixedTopLeft = geo.NewPoint(40, -20)
	node.DesiredWidth = new(140.)
	node.DesiredHeight = new(90.)
	input.AddNewNodeToContainer(nil, node)

	completed, err := layoutgraph.Clone(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	completed.Nodes[0].TopLeft = geo.NewPoint(0, 0)
	completed.Nodes[0].Width = 100
	completed.Nodes[0].Height = 80

	// Result validation establishes that immutable input metadata survived
	// the attempt. Whether a layout honors those constraints is a separate
	// validity policy.
	if err := validateLayoutResultMetadata(t.Context(), input, completed, nil); err != nil {
		t.Fatalf("unchanged constraint metadata rejected because of output geometry: %v", err)
	}
}

func TestEvaluateSeedAcceptsUnmodifiedEngineConstraintOutputs(t *testing.T) {
	// This input reproduces a constraint-satisfaction bug on this PR's master
	// base: singleton layout overwrites a fixed position. Result validation
	// accepts either the old or fixed output as long as immutable constraint
	// metadata itself is unchanged.
	t.Run("fixed singleton", func(t *testing.T) {
		inputGraph := layoutgraph.NewGraph()
		node := layoutgraph.NewNode(1, 40, 40)
		node.TopLeft = geo.NewPoint(137, 241)
		node.FixedTopLeft = geo.NewPoint(137, 241)
		inputGraph.AddNewNodeToContainer(nil, node)
		input := seedInput{graph: inputGraph}

		completed, err := runSeed(t.Context(), input, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := evaluateSeedResult(t.Context(), input, completed); err != nil {
			t.Fatalf("unmodified fixed-singleton result rejected: %v", err)
		}
	})

}

func TestEvaluateSeedAllowsLayoutOwnedLabelAndIconPositions(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range input.graph.Nodes {
		node.Label = &layoutgraph.Label{Text: "node", Width: 30, Height: 12}
		node.Icon = &layoutgraph.Icon{}
	}
	input.graph.Edges[0].Label = &layoutgraph.Label{Text: "edge", Width: 28, Height: 10}
	completed, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Nodes[0].Label.Position == label.Unset || completed.Nodes[0].Icon.Position == label.Unset {
		t.Fatalf("layout left label or icon position unset: label=%v icon=%v", completed.Nodes[0].Label.Position, completed.Nodes[0].Icon.Position)
	}
	if completed.Edges[0].Label.Position == label.Unset {
		t.Fatal("layout left edge label position unset")
	}
	if _, err := evaluateSeedResult(t.Context(), input, completed); err != nil {
		t.Fatalf("layout-owned label or icon position rejected: %v", err)
	}
}

func TestValidateLayoutResultTopologyAllowsOnlyConsumedSequenceEdges(t *testing.T) {
	input := layoutgraph.NewGraph()
	a := layoutgraph.NewNode(1, 20, 10)
	b := layoutgraph.NewNode(2, 20, 10)
	c := layoutgraph.NewNode(3, 20, 10)
	for index, node := range []*layoutgraph.Node{a, b, c} {
		node.TopLeft = geo.NewPoint(float64(index*40), 0)
		input.AddNewNodeToContainer(nil, node)
	}
	a.SetShape(shape.STEP_TYPE)
	b.SetShape(shape.STEP_TYPE)
	sequenceEdge := input.Connect(a, b)
	sequenceEdge.ID = 10
	ordinaryEdge := input.Connect(b, c)
	ordinaryEdge.ID = 20

	completed, err := layoutgraph.Clone(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	findEdge := func(graph *layoutgraph.Graph, id layoutgraph.EntityID) *layoutgraph.Edge {
		t.Helper()
		for _, edge := range graph.Edges {
			if edge.ID == id {
				return edge
			}
		}
		t.Fatalf("edge %d not found", id)
		return nil
	}
	completed.Disconnect(findEdge(completed, sequenceEdge.ID))

	sequenceEdges, err := validateLayoutResultTopology(t.Context(), input, completed)
	if err != nil {
		t.Fatalf("sequence-defining edge removal rejected: %v", err)
	}
	if _, ok := sequenceEdges[sequenceEdge.ID]; !ok || len(sequenceEdges) != 1 {
		t.Fatalf("sequence edges = %v, want only %d", sequenceEdges, sequenceEdge.ID)
	}
	if err := validateLayoutResultMetadata(t.Context(), input, completed, sequenceEdges); err != nil {
		t.Fatalf("metadata rejected consumed sequence edge: %v", err)
	}
	if err := validateTopology(t.Context(), input, completed, nil); err == nil || !strings.Contains(err.Error(), "missing edge") {
		t.Fatalf("strict topology error = %v, want missing-edge rejection", err)
	}

	t.Run("ordinary edge remains required", func(t *testing.T) {
		attempt, err := layoutgraph.Clone(t.Context(), completed)
		if err != nil {
			t.Fatal(err)
		}
		attempt.Disconnect(findEdge(attempt, ordinaryEdge.ID))
		if _, err := validateLayoutResultTopology(t.Context(), input, attempt); err == nil || !strings.Contains(err.Error(), "missing edge") {
			t.Fatalf("topology error = %v, want missing ordinary-edge rejection", err)
		}
	})

	t.Run("unexpected edge remains forbidden", func(t *testing.T) {
		attempt, err := layoutgraph.Clone(t.Context(), completed)
		if err != nil {
			t.Fatal(err)
		}
		extra := attempt.Connect(attempt.Nodes[0], attempt.Nodes[2])
		extra.ID = 30
		if _, err := validateLayoutResultTopology(t.Context(), input, attempt); err == nil || !strings.Contains(err.Error(), "unexpected edge") {
			t.Fatalf("topology error = %v, want unexpected-edge rejection", err)
		}
	})

	t.Run("duplicate result edge IDs remain forbidden", func(t *testing.T) {
		attempt, err := layoutgraph.Clone(t.Context(), completed)
		if err != nil {
			t.Fatal(err)
		}
		duplicate := attempt.Connect(attempt.Nodes[1], attempt.Nodes[2])
		duplicate.ID = ordinaryEdge.ID
		if _, err := validateLayoutResultTopology(t.Context(), input, attempt); err == nil || !strings.Contains(err.Error(), "duplicate edge ID") {
			t.Fatalf("topology error = %v, want duplicate-edge rejection", err)
		}
	})
}

func TestSeedMetadataValidationRejectsCanceledContext(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := validateLayoutResultMetadata(ctx, input.graph, attempt, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("metadata validation error = %v, want context.Canceled", err)
	}
}

func TestSeedResultLifecycle(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	objects := append([]*d2graph.Object(nil), d2Graph.Objects...)
	edges := append([]*d2graph.Edge(nil), d2Graph.Edges...)

	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluateSeedResult(t.Context(), input, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if result.score.area <= 0 {
		t.Fatalf("evaluated area = %v, want positive", result.score.area)
	}
	if err := applySeedResult(t.Context(), d2Graph, result); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d2Graph.Objects, objects) || !slices.Equal(d2Graph.Edges, edges) {
		t.Fatal("applying a seed replaced D2 object or edge identities")
	}
}

func TestApplySeedResultCancellationLeavesD2GraphUnchanged(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluateSeedResult(t.Context(), input, attempt)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotD2Graph(t, d2Graph)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := applySeedResult(ctx, d2Graph, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplySeedResult() error = %v, want context.Canceled", err)
	}
	before.assertUnchanged(t, d2Graph)
}

func TestCloneSeedInputIsIndependent(t *testing.T) {
	d2Graph, _ := newD2TransactionGraph(false)
	input, err := newSeedInput(t.Context(), d2Graph)
	if err != nil {
		t.Fatal(err)
	}
	cloneGraph, err := layoutgraph.Clone(t.Context(), input.graph)
	if err != nil {
		t.Fatal(err)
	}
	cloneGraph.Nodes[0].Width++
	if input.graph.Nodes[0].Width == cloneGraph.Nodes[0].Width {
		t.Fatal("mutating a cloned seed input changed its source")
	}
}

func TestRunSeedLeavesInputUnchanged(t *testing.T) {
	for _, test := range []struct {
		name     string
		canceled bool
	}{
		{name: "completed"},
		{name: "canceled", canceled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			d2Graph, _ := newD2TransactionGraph(false)
			input, err := newSeedInput(t.Context(), d2Graph)
			if err != nil {
				t.Fatal(err)
			}
			before, err := graphjson.Marshal(t.Context(), input.graph)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.canceled {
				cancel()
			}
			result, err := runSeed(ctx, input, 1)
			if test.canceled {
				if result != nil || !errors.Is(err, context.Canceled) {
					t.Fatalf("RunSeed() = (%v, %v), want (nil, context.Canceled)", result, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			after, err := graphjson.Marshal(t.Context(), input.graph)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(before, after) {
				t.Fatal("RunSeed mutated its input")
			}
		})
	}
}
