package d2compiler

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2ir"
	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2target"
)

// Compare the entire graph, including ASTs, references, parents, legends and
// nested boards, against the previous copying implementation below. Graph
// serialization intentionally omits some of those fields.
func TestCompileBoardCopyOracle(t *testing.T) {
	cases := []struct {
		name, source string
		invalid      bool
	}{
		{"empty", "", false},
		{"ordinary", `a: {b; c}; a.b -> a.c: connection`, false},
		{"classes", `classes: {red: {style.fill: red}; linked: {link: https://example.com}}
a: {class: red; b.class: red}
a.b -> c: {class: linked}`, false},
		{"variables", `vars: {text: hello; nodes: {x; y; x -> y}}
a: ${text}
group: ${nodes}`, false},
		{"legend", `vars: {d2-legend: {a: {style.fill: red}; a -> b: relation}}
x -> y`, false},
		{"quoted_board_names", `"layers": {x}; "steps": {y}; "scenarios": {z}`, false},
		{"empty_boards", `layers: {empty}; scenarios: {}; steps: {}`, false},
		{"folded_board_names", `ſteps: {one: {a}}; b`, false},
		{"nested_classes", `classes: {box: {style.fill: red}}
a.class: box
layers: {one: {classes: {box: {style.fill: blue}}; b.class: box; layers: {deep: {c.class: box}}}}
scenarios: {two: {a.style.fill: green}}
steps: {three: {a -> z}; four: {z -> zz}}`, false},
		{"folders", `layers: {one: {layers: {two: {a}}}}`, false},
		{"board_links", `a.link: layers.one
layers: {one: {b.link: root}}`, false},
		{"glob_class", `classes: {container: {style.fill: transparent}}
**: {&label: cont*; class: container}
cont_target`, false},
		{"class_cycle", `classes: {x: {class: x}}; a.class: x`, true},
		{"invalid_shape", `a: {shape: image; b}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldGraph, oldConfig, oldErr, oldAST := compileCopyOracleInput(t, tc.source, true)
			graph, config, err, ast := compileCopyOracleInput(t, tc.source, false)
			if oldErr != err {
				t.Fatalf("errors differ: original %q, current %q", oldErr, err)
			}
			if (err != "") != tc.invalid {
				t.Fatalf("unexpected compile result: %q", err)
			}
			if !reflect.DeepEqual(oldGraph, graph) {
				t.Fatal("full graph differs from copying implementation")
			}
			if !reflect.DeepEqual(oldConfig, config) {
				t.Fatal("configuration differs from copying implementation")
			}
			if !reflect.DeepEqual(oldAST, ast) {
				t.Fatal("source AST differs from copying implementation")
			}
		})
	}
}

func compileCopyOracleInput(t *testing.T, source string, original bool) (*d2graph.Graph, *d2target.Config, string, *d2ast.Map) {
	t.Helper()
	ast, ir := parseCopyOracleInput(t, source)
	var g *d2graph.Graph
	var err error
	if original {
		g, err = compileIRCopyOracle(ast, ir)
	} else {
		g, err = compileIR(ast, ir)
	}
	if err != nil {
		return nil, nil, err.Error(), ast
	}
	g.SortObjectsByAST()
	g.SortEdgesByAST()
	config, err := compileConfig(ir)
	if err != nil {
		return nil, nil, err.Error(), ast
	}
	return g, config, "", ast
}

func parseCopyOracleInput(t *testing.T, source string) (*d2ast.Map, *d2ir.Map) {
	t.Helper()
	ast, err := d2parser.Parse("copy-oracle.d2", strings.NewReader(source), nil)
	if err != nil {
		t.Fatal(err)
	}
	ir, _, err := d2ir.Compile(ast, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ast, ir
}

func TestCompileRootIROwnership(t *testing.T) {
	const source = `vars: {d2-config: {theme-id: 100; pad: 42}}
classes: {red: {style.fill: red}}
a: {class: red; b}
a.b -> c: relation
"layers": {x}
`
	ast, ir := parseCopyOracleInput(t, source)
	beforeIR, beforeAST := copyOracleJSON(t, ir), copyOracleJSON(t, ast)
	beforeNodes, beforeParents := copyOracleNodes(ir)
	g, err := compileIR(ast, ir)
	if err != nil {
		t.Fatal(err)
	}
	// Graph attributes must remain independently owned even though the IR itself
	// is read directly. AST references keep their existing shared ownership.
	for _, obj := range g.Objects {
		obj.Label.Value = "changed"
		if obj.Style.Fill != nil {
			obj.Style.Fill.Value = "blue"
		}
	}
	if string(beforeIR) != string(copyOracleJSON(t, ir)) || string(beforeAST) != string(copyOracleJSON(t, ast)) {
		t.Fatal("graph compilation or attribute edits mutated input IR/AST")
	}
	nodes, parents := copyOracleNodes(ir)
	if len(nodes) != len(beforeNodes) {
		t.Fatal("graph compilation changed the input node count")
	}
	for i := range nodes {
		if nodes[i] != beforeNodes[i] || parents[i] != beforeParents[i] {
			t.Fatal("graph compilation changed input node identity, order, or parent links")
		}
	}
	repeated, err := compileIR(ast, ir)
	if err != nil {
		t.Fatal(err)
	}
	expected, _, expectedErr, _ := compileCopyOracleInput(t, source, true)
	if expectedErr != "" {
		t.Fatal(expectedErr)
	}
	repeated.SortObjectsByAST()
	repeated.SortEdgesByAST()
	if !reflect.DeepEqual(expected, repeated) {
		t.Fatal("recompiling the same IR differs from an independent copying compile")
	}
}

func TestCompileBoardCopyWithCustomReservedKeywords(t *testing.T) {
	// Do not run in parallel: callers may customize this exported keyword map.
	for _, name := range []string{"layers", "scenarios", "steps"} {
		t.Run(name, func(t *testing.T) {
			value, existed := d2ast.ReservedKeywords[name]
			delete(d2ast.ReservedKeywords, name)
			t.Cleanup(func() {
				if existed {
					d2ast.ReservedKeywords[name] = value
				} else {
					delete(d2ast.ReservedKeywords, name)
				}
			})

			// Without the reserved keyword, GetField also matches the quoted
			// declaration. CopyBase would move it after tail in the source IR.
			source := fmt.Sprintf(`%q: {one: {a}}; tail`, name)
			ast, ir := parseCopyOracleInput(t, source)
			before := copyOracleJSON(t, ir)
			g, err := compileIR(ast, ir)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(copyOracleJSON(t, ir)) {
				t.Fatal("custom keyword lookup allowed folder comparison to mutate source IR")
			}
			g.SortObjectsByAST()
			g.SortEdgesByAST()
			want, _, wantErr, _ := compileCopyOracleInput(t, source, true)
			if wantErr != "" || !reflect.DeepEqual(g, want) {
				t.Fatalf("custom keyword graph differs from copying implementation: %s", wantErr)
			}
		})
	}
}

func copyOracleJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func copyOracleNodes(n d2ir.Node) (nodes, parents []d2ir.Node) {
	var visit func(d2ir.Node)
	visit = func(n d2ir.Node) {
		nodes = append(nodes, n)
		parents = append(parents, n.Parent())
		switch n := n.(type) {
		case *d2ir.Map:
			for _, f := range n.Fields {
				visit(f)
			}
			for _, e := range n.Edges {
				visit(e)
			}
		case *d2ir.Field:
			if n.Primary_ != nil {
				visit(n.Primary_)
			}
			if n.Composite != nil {
				visit(n.Composite)
			}
		case *d2ir.Edge:
			if n.Primary_ != nil {
				visit(n.Primary_)
			}
			if n.Map_ != nil {
				visit(n.Map_)
			}
		case *d2ir.Array:
			for _, v := range n.Values {
				visit(v)
			}
		}
	}
	visit(n)
	return nodes, parents
}

// Frozen board-conversion orchestration from the pre-optimization source.
func compileIRCopyOracle(ast *d2ast.Map, m *d2ir.Map) (*d2graph.Graph, error) {
	c := &compiler{
		err: &d2parser.ParseError{},
	}

	g := d2graph.NewGraph()
	g.AST = ast
	g.BaseAST = ast
	c.compileBoardCopyOracle(g, m)
	if len(c.err.Errors) > 0 {
		return nil, c.err
	}
	c.validateBoardLinks(g)
	if len(c.err.Errors) > 0 {
		return nil, c.err
	}
	return g, nil
}

func (c *compiler) compileBoardCopyOracle(g *d2graph.Graph, ir *d2ir.Map) *d2graph.Graph {
	ir = ir.Copy(nil).(*d2ir.Map)
	c.compileMap(g.Root, ir)
	c.setDefaultShapes(g)
	if len(c.err.Errors) == 0 {
		c.validateKeys(g.Root, ir)
	}
	c.validateLabels(g)
	c.validateNear(g)
	c.validateEdges(g)
	c.validatePositionsCompatibility(g)

	c.compileLegend(g, ir)

	c.compileBoardsFieldCopyOracle(g, ir, "layers")
	c.compileBoardsFieldCopyOracle(g, ir, "scenarios")
	c.compileBoardsFieldCopyOracle(g, ir, "steps")
	if d2ir.ParentMap(ir).CopyBase(nil).Equal(ir.CopyBase(nil)) {
		if len(g.Layers) > 0 || len(g.Scenarios) > 0 || len(g.Steps) > 0 {
			g.IsFolderOnly = true
		}
	}
	if len(g.Objects) == 0 {
		g.IsFolderOnly = true
	}
	return g
}

func (c *compiler) compileBoardsFieldCopyOracle(g *d2graph.Graph, ir *d2ir.Map, fieldName string) {
	boards := ir.GetField(d2ast.FlatUnquotedString(fieldName))
	if boards.Map() == nil {
		return
	}
	for _, f := range boards.Map().Fields {
		m := f.Map()
		if f.Map() == nil {
			m = &d2ir.Map{}
		}
		if g.GetBoard(f.Name.ScalarString()) != nil {
			c.errorf(f.References[0].AST(), "board name %v already used by another board", f.Name.ScalarString())
			continue
		}
		g2 := d2graph.NewGraph()
		g2.Parent = g
		g2.AST = m.AST().(*d2ast.Map)
		if g.BaseAST != nil {
			g2.BaseAST = findFieldAST(g.BaseAST, f)
		}
		c.compileBoardCopyOracle(g2, m)
		if f.Primary() != nil {
			c.compileLabel(&g2.Root.Attributes, f)
		}
		g2.Name = f.Name.ScalarString()
		switch fieldName {
		case "layers":
			g.Layers = append(g.Layers, g2)
		case "scenarios":
			g.Scenarios = append(g.Scenarios, g2)
		case "steps":
			g.Steps = append(g.Steps, g2)
		}
	}
}
