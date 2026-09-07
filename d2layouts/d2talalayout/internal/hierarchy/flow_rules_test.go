package hierarchy

import (
	"context"
	"errors"
	"testing"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

func TestBuildFlowRules(t *testing.T) {
	workflow := [][2]int{{0, 1}, {1, 2}, {2, 3}, {2, 4}, {3, 5}, {4, 5}, {5, 6}, {6, 7}, {1, 7}, {3, 7}, {4, 7}}
	chain := [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {6, 7}}
	tests := []struct {
		name   string
		nodes  int
		edges  [][2]int
		mutate func(*layoutgraph.Graph)
		force  bool
		want   bool
	}{
		{name: "branched_workflow", nodes: 8, edges: workflow, want: true},
		{name: "left_arrow_workflow", nodes: 8, edges: workflow, mutate: reverseAuthoredEndpoints, want: true},
		{name: "multiple_entry_workflow", nodes: 9, edges: appendPairs(workflow, [2]int{8, 2}), want: false},
		{name: "multiple_exit_workflow", nodes: 9, edges: appendPairs(workflow, [2]int{5, 8}), want: false},
		{name: "feedback_workflow", nodes: 8, edges: appendPairs(workflow, [2]int{4, 2}), want: false},
		{name: "plain_chain", nodes: 8, edges: chain, want: false},
		{name: "chain_with_parallel_arrows_and_loops", nodes: 8, edges: appendPairs(chain, [2]int{2, 3}, [2]int{0, 0}, [2]int{7, 7}), want: false},
		{name: "forced_chain", nodes: 8, edges: chain, force: true, want: true},
		{name: "fanin", nodes: 3, edges: [][2]int{{0, 2}, {1, 2}}, want: false},
		{name: "fanout", nodes: 3, edges: [][2]int{{0, 1}, {0, 2}}, want: false},
		{name: "left_arrow_fanin", nodes: 3, edges: [][2]int{{0, 2}, {1, 2}}, mutate: reverseAuthoredEndpoints, want: false},
		{name: "fan_with_parallel_arrows_and_loops", nodes: 3, edges: [][2]int{{0, 2}, {1, 2}, {0, 2}, {0, 0}, {2, 2}}, want: false},
		{name: "pair_with_parallel_arrows", nodes: 2, edges: [][2]int{{0, 1}, {0, 1}, {0, 1}}, want: false},
		{name: "two_by_two_is_not_a_fan", nodes: 4, edges: [][2]int{{0, 2}, {0, 3}, {1, 2}, {1, 3}}, want: false},
		{name: "undirected_fan", nodes: 3, edges: [][2]int{{0, 2}, {1, 2}}, mutate: func(g *layoutgraph.Graph) { g.Edges[0].TargetArrowhead = layoutgraph.NoArrowhead }, want: false},
		{name: "bidirectional_fan", nodes: 3, edges: [][2]int{{0, 2}, {1, 2}}, mutate: func(g *layoutgraph.Graph) { g.Edges[0].SourceArrowhead = layoutgraph.TriangleArrowhead }, want: false},
		{name: "one_many_one_retained", nodes: 4, edges: [][2]int{{0, 1}, {0, 2}, {1, 3}, {2, 3}}, want: false},
		{name: "wide_fan_retained", nodes: 10, edges: [][2]int{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 5}, {0, 6}, {0, 7}, {0, 8}, {0, 9}}, want: false},
		{name: "dense_fan_retained", nodes: 3, edges: [][2]int{{0, 2}, {0, 2}, {0, 2}, {1, 2}, {1, 2}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := flowRuleGraph(tt.nodes, tt.edges)
			if tt.mutate != nil {
				tt.mutate(g)
			}
			h, err := build(t.Context(), g, tt.force, Candidates(g), nil)
			if err != nil {
				t.Fatal(err)
			}
			if (h != nil) != tt.want {
				t.Fatalf("hierarchy accepted = %v, want %v", h != nil, tt.want)
			}
		})
	}
}

func TestBuildFlowRulesCancellation(t *testing.T) {
	g := flowRuleGraph(3, [][2]int{{0, 2}, {1, 2}})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	h, err := build(ctx, g, false, Candidates(g), nil)
	if h != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled build = %v, %v", h, err)
	}
}

func TestAutomaticWorkflowBounds(t *testing.T) {
	tests := []struct {
		name        string
		nodes       int
		descendants int
		parallel    int
		loops       int
		want        bool
	}{
		{name: "128_nodes", nodes: 128, want: true},
		{name: "129_nodes", nodes: 129, want: false},
		{name: "128_with_descendants", nodes: 126, descendants: 2, want: true},
		{name: "129_with_descendants", nodes: 127, descendants: 2, want: false},
		{name: "256_structural_edges", nodes: 128, parallel: 128, want: true},
		{name: "257_structural_edges", nodes: 128, parallel: 129, want: false},
		{name: "loops_do_not_use_structural_edge_limit", nodes: 128, parallel: 128, loops: 3, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A diamond followed by a chain has one source, one sink and
			// one parallel rank regardless of its total length.
			edges := [][2]int{{0, 1}, {0, 2}, {1, 3}, {2, 3}}
			for i := 3; i < tt.nodes-1; i++ {
				edges = append(edges, [2]int{i, i + 1})
			}
			original := append([][2]int(nil), edges...)
			for i := range tt.parallel {
				edges = append(edges, original[i%len(original)])
			}
			for range tt.loops {
				edges = append(edges, [2]int{0, 0})
			}
			g := flowRuleGraph(tt.nodes, edges)
			if tt.descendants != 0 {
				children := make([]*layoutgraph.Node, tt.descendants)
				for i := range children {
					children[i] = layoutgraph.NewNode(layoutgraph.EntityID(tt.nodes+len(edges)+i+1), 30, 30)
				}
				setContainerChildrenForTest(g, g.Nodes[3], children)
			}
			h := layoutgraph.NewHierarchy()
			levels := make(map[*layoutgraph.Node]int, tt.nodes)
			for i, n := range g.Nodes {
				level := max(0, i-1)
				if i == 1 {
					level = 1
				}
				levels[n] = level
			}
			h.ReplaceLevels(levels)
			h.LevelCount = tt.nodes - 1
			if got := isValid(h, g, nil, 1, 1); got != tt.want {
				t.Fatalf("workflow accepted = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAutomaticWorkflowBoundsPreserveOrdinaryHierarchy(t *testing.T) {
	const levels, columns = 12, 12
	var edges [][2]int
	for level := 0; level < levels-1; level++ {
		for column := range columns {
			edges = append(edges, [2]int{level*columns + column, (level+1)*columns + column})
		}
	}
	for column := 0; column < columns-1; column++ {
		edges = append(edges, [2]int{column, column + 1})
	}
	g := flowRuleGraph(levels*columns, edges)
	h := layoutgraph.NewHierarchy()
	nodeLevels := make(map[*layoutgraph.Node]int, len(g.Nodes))
	for i, n := range g.Nodes {
		nodeLevels[n] = i / columns
	}
	h.ReplaceLevels(nodeLevels)
	h.LevelCount = levels
	if !isValid(h, g, nil, 1, columns) {
		t.Fatal("ordinary compact hierarchy above workflow size limit was rejected")
	}
}

func TestBuildAutomaticWorkflowMinimumExtent(t *testing.T) {
	// This graph previously fit through the general placement path, but
	// automatic hierarchy placement creates 80 ranks of 300px plus 79
	// minimum 90px gaps: 31,110px, beyond the 30,000px engine limit.
	edges := [][2]int{{0, 1}, {0, 2}, {1, 3}, {2, 3}}
	for i := 3; i < 80; i++ {
		edges = append(edges, [2]int{i, i + 1})
	}
	tests := []struct {
		name      string
		direction geo.Orientation
		width     float64
		height    float64
		force     bool
		want      bool
	}{
		{name: "tall_vertical", direction: geo.Bottom, width: 80, height: 300, want: false},
		{name: "wide_horizontal", direction: geo.Right, width: 300, height: 80, want: false},
		{name: "short_wide_vertical", direction: geo.Bottom, width: 300, height: 80, want: true},
		{name: "narrow_tall_horizontal", direction: geo.Right, width: 80, height: 300, want: true},
		{name: "authored_hierarchy_retains_control", direction: geo.Bottom, width: 80, height: 300, force: true, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := flowRuleGraph(81, edges)
			g.Directions[nil] = tt.direction
			for _, n := range g.Nodes {
				n.Width, n.Height = tt.width, tt.height
			}
			h, err := build(t.Context(), g, tt.force, Candidates(g), nil)
			if err != nil {
				t.Fatal(err)
			}
			if (h != nil) != tt.want {
				t.Fatalf("hierarchy accepted = %v, want %v", h != nil, tt.want)
			}
		})
	}
}

func flowRuleGraph(count int, edges [][2]int) *layoutgraph.Graph {
	g := layoutgraph.NewGraph()
	for i := range count {
		g.AddNode(layoutgraph.NewNode(layoutgraph.EntityID(i+1), 100, 60))
	}
	for i, endpoints := range edges {
		e := g.Connect(g.Nodes[endpoints[0]], g.Nodes[endpoints[1]])
		e.ID = layoutgraph.EntityID(count + i + 1)
		e.SourceArrowhead = layoutgraph.NoArrowhead
		e.TargetArrowhead = layoutgraph.TriangleArrowhead
	}
	return g
}

func appendPairs(base [][2]int, pairs ...[2]int) [][2]int {
	return append(append([][2]int(nil), base...), pairs...)
}

func reverseAuthoredEndpoints(g *layoutgraph.Graph) {
	for _, e := range g.Edges {
		e.From, e.To = e.To, e.From
		e.SourceArrowhead = layoutgraph.TriangleArrowhead
		e.TargetArrowhead = layoutgraph.NoArrowhead
	}
}
