package d2oracle_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
	"oss.terrastruct.com/util-go/go2"
	"oss.terrastruct.com/util-go/mapfs"
	"oss.terrastruct.com/util-go/xjson"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/d2target"
)

// TODO: make assertions less specific
// TODO: move n objects and n edges assertions as fields on test instead of as callback

func TestCreate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		boardPath []string
		name      string
		text      string
		key       string

		expKey     string
		expErr     string
		exp        string
		assertions func(t *testing.T, g *d2graph.Graph)
	}{
		{
			name: "base",
			text: ``,
			key:  `square`,

			expKey: `square`,
			exp: `square
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "square" {
					t.Fatalf("expected g.Objects[0].ID to be square: %#v", g.Objects[0])
				}
				if g.Objects[0].Label.MapKey.Value.Unbox() != nil {
					t.Fatalf("expected g.Objects[0].Label.Node.Value.Unbox() == nil: %#v", g.Objects[0].Label.MapKey.Value)
				}
				if d2format.Format(g.Objects[0].Label.MapKey.Key) != "square" {
					t.Fatalf("expected g.Objects[0].Label.Node.Key to be square: %#v", g.Objects[0].Label.MapKey.Key)
				}
			},
		},
		{
			name: "gen_key_suffix",
			text: `"x "
`,
			key: `"x "`,

			expKey: `x  2`,
			exp: `"x "
x  2
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("unexpected objects length: %#v", g.Objects)
				}
				if g.Objects[1].ID != `x  2` {
					t.Fatalf("bad object ID: %#v", g.Objects[1])
				}
			},
		},
		{
			name: "nested",
			text: ``,
			key:  `b.c.square`,

			expKey: `b.c.square`,
			exp: `b.c.square
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("unexpected objects length: %#v", g.Objects)
				}
				if g.Objects[2].AbsID() != "b.c.square" {
					t.Fatalf("bad absolute ID: %#v", g.Objects[2].AbsID())
				}
				if d2format.Format(g.Objects[2].Label.MapKey.Key) != "b.c.square" {
					t.Fatalf("bad mapkey: %#v", g.Objects[2].Label.MapKey.Key)
				}
				if g.Objects[2].Label.MapKey.Value.Unbox() != nil {
					t.Fatalf("expected nil mapkey value: %#v", g.Objects[2].Label.MapKey.Value)
				}
			},
		},
		{
			name: "gen_key",
			text: `square`,
			key:  `square`,

			expKey: `square 2`,
			exp: `square
square 2
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[1].ID != "square 2" {
					t.Fatalf("expected g.Objects[1].ID to be square 2: %#v", g.Objects[1])
				}
				if g.Objects[1].Label.MapKey.Value.Unbox() != nil {
					t.Fatalf("expected g.Objects[1].Label.Node.Value.Unbox() == nil: %#v", g.Objects[1].Label.MapKey.Value)
				}
				if d2format.Format(g.Objects[1].Label.MapKey.Key) != "square 2" {
					t.Fatalf("expected g.Objects[1].Label.Node.Key to be square 2: %#v", g.Objects[1].Label.MapKey.Key)
				}
			},
		},
		{
			name: "gen_key_nested",
			text: `x.y.z.square`,
			key:  `x.y.z.square`,

			expKey: `x.y.z.square 2`,
			exp: `x.y.z.square
x.y.z.square 2
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("unexpected objects length: %#v", g.Objects)
				}
				if g.Objects[4].ID != "square 2" {
					t.Fatalf("unexpected object id: %#v", g.Objects[4])
				}
			},
		},
		{
			name: "scope",
			text: `x.y.z: {
}`,
			key: `x.y.z.square`,

			expKey: `x.y.z.square`,
			exp: `x.y.z: {
  square
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
				if g.Objects[3].ID != "square" {
					t.Fatalf("expected g.Objects[3].ID to be square: %#v", g.Objects[3])
				}
				if g.Objects[3].Label.MapKey.Value.Unbox() != nil {
					t.Fatalf("expected g.Objects[3].Label.Node.Value.Unbox() == nil: %#v", g.Objects[3].Label.MapKey.Value)
				}
				if d2format.Format(g.Objects[3].Label.MapKey.Key) != "square" {
					t.Fatalf("expected g.Objects[3].Label.Node.Key to be square: %#v", g.Objects[3].Label.MapKey.Key)
				}
			},
		},
		{
			name: "gen_key_scope",
			text: `x.y.z: {
  square
}`,
			key: `x.y.z.square`,

			expKey: `x.y.z.square 2`,
			exp: `x.y.z: {
  square
  square 2
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
				if g.Objects[4].ID != "square 2" {
					t.Fatalf("expected g.Objects[4].ID to be square 2: %#v", g.Objects[4])
				}
				if g.Objects[4].Label.MapKey.Value.Unbox() != nil {
					t.Fatalf("expected g.Objects[4].Label.Node.Value.Unbox() == nil: %#v", g.Objects[4].Label.MapKey.Value)
				}
				if d2format.Format(g.Objects[4].Label.MapKey.Key) != "square 2" {
					t.Fatalf("expected g.Objects[4].Label.Node.Key to be square 2: %#v", g.Objects[4].Label.MapKey.Key)
				}
			},
		},
		{
			name: "gen_key_n",
			text: `x.y.z: {
  square
  square 2
  square 3
  square 4
  square 5
  square 6
  square 7
  square 8
  square 9
  square 10
}`,
			key: `x.y.z.square`,

			expKey: `x.y.z.square 11`,
			exp: `x.y.z: {
  square
  square 2
  square 3
  square 4
  square 5
  square 6
  square 7
  square 8
  square 9
  square 10
  square 11
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 14 {
					t.Fatalf("expected 14 objects: %#v", g.Objects)
				}
				if g.Objects[13].ID != "square 11" {
					t.Fatalf("expected g.Objects[13].ID to be square 11: %#v", g.Objects[13])
				}
				if d2format.Format(g.Objects[13].Label.MapKey.Key) != "square 11" {
					t.Fatalf("expected g.Objects[13].Label.Node.Key to be square 11: %#v", g.Objects[13].Label.MapKey.Key)
				}
			},
		},
		{
			name: "edge",
			text: ``,
			key:  `x -> y`,

			expKey: `(x -> y)[0]`,
			exp: `x -> y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "x" {
					t.Fatalf("expected g.Edges[0].Src.ID == x: %#v", g.Edges[0].Src.ID)
				}
				if g.Edges[0].Dst.ID != "y" {
					t.Fatalf("expected g.Edges[0].Dst.ID == y: %#v", g.Edges[0].Dst.ID)
				}
			},
		},
		{
			name: "edge_nested",
			text: ``,
			key:  `container.(x -> y)`,

			expKey: `container.(x -> y)[0]`,
			exp: `container.(x -> y)
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("unexpected objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("unexpected edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_scope",
			text: `container: {
}`,
			key: `container.(x -> y)`,

			expKey: `container.(x -> y)[0]`,
			exp: `container: {
  x -> y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "edge_scope_flat",
			text: `container: {
}`,
			key: `container.x -> container.y`,

			expKey: `container.(x -> y)[0]`,
			exp: `container: {
  x -> y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "edge_scope_nested",
			text: `x.y`,
			key:  `x.y.z -> x.y.q`,

			expKey: `x.y.(z -> q)[0]`,
			exp: `x.y: {
  z -> q
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("unexpected objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("unexpected edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_unique",
			text: `x -> y
hello.(x -> y)
hello.(x -> y)
`,
			key: `hello.(x -> y)`,

			expKey: `hello.(x -> y)[2]`,
			exp: `x -> y
hello.(x -> y)
hello.(x -> y)
hello.(x -> y)
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 4 {
					t.Fatalf("expected 4 edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "container",
			text: `b`,
			key:  `b.q`,

			expKey: `b.q`,
			exp: `b: {
  q
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "container_edge",
			text: `b`,
			key:  `b.x -> b.y`,

			expKey: `b.(x -> y)[0]`,
			exp: `b: {
  x -> y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "container_edge_label",
			text: `b: zoom`,
			key:  `b.x -> b.y`,

			expKey: `b.(x -> y)[0]`,
			exp: `b: zoom {
  x -> y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "make_scope_multiline",

			text: `rawr: {shape: circle}
`,
			key: `rawr.orange`,

			expKey: `rawr.orange`,
			exp: `rawr: {
  shape: circle
  orange
}
`,
		},
		{
			name: "make_scope_multiline_spacing_1",

			text: `before
rawr: {shape: circle}
after
`,
			key: `rawr.orange`,

			expKey: `rawr.orange`,
			exp: `before
rawr: {
  shape: circle
  orange
}
after
`,
		},
		{
			name: "make_scope_multiline_spacing_2",

			text: `before

rawr: {shape: circle}

after
`,
			key: `rawr.orange`,

			expKey: `rawr.orange`,
			exp: `before

rawr: {
  shape: circle
  orange
}

after
`,
		},
		{
			name: "layers-basic",

			text: `a

layers: {
  x: {
    a
  }
}
`,
			key:       `b`,
			boardPath: []string{"x"},

			expKey: `b`,
			exp: `a

layers: {
  x: {
    a
    b
  }
}
`,
		},
		{
			name: "add_layer/1",
			text: `b`,
			key:  `layers.c`,

			expKey: `layers.c`,
			exp: `b

layers: {
  c
}
`,
		},
		{
			name: "add_layer/2",
			text: `b
layers: {
  c: {
    x
  }
}`,
			key: `layers.b`,

			expKey: `layers.b`,
			exp: `b

layers: {
  c: {
    x
  }
  b
}
`,
		},
		{
			name: "add_layer/3",
			text: `b

layers: {
	c: {
    d
  }
}
`,
			key: `layers.c`,

			boardPath: []string{"c"},
			expKey:    `layers.c`,
			exp: `b

layers: {
  c: {
    d

    layers: {
      c
    }
  }
}
`,
		},
		{
			name: "add_layer/4",
			text: `b

layers: {
	c
}
`,
			key: `d`,

			boardPath: []string{"c"},
			expKey:    `d`,
			exp: `b

layers: {
  c: {
    d
  }
}
`,
		},
		{
			name: "layers-edge",

			text: `a

layers: {
  x: {
    a
  }
}
`,
			key:       `a -> b`,
			boardPath: []string{"x"},

			expKey: `(a -> b)[0]`,
			exp: `a

layers: {
  x: {
    a
    a -> b
  }
}
`,
		},
		{
			name: "layers-edge-duplicate",

			text: `a -> b

layers: {
  x: {
    a -> b
  }
}
`,
			key:       `a -> b`,
			boardPath: []string{"x"},

			expKey: `(a -> b)[1]`,
			exp: `a -> b

layers: {
  x: {
    a -> b
    a -> b
  }
}
`,
		},
		{
			name: "scenarios-basic",

			text: `a
b

scenarios: {
  x: {
    a
  }
}
`,
			key:       `c`,
			boardPath: []string{"x"},

			expKey: `c`,
			exp: `a
b

scenarios: {
  x: {
    a
    c
  }
}
`,
		},
		{
			name: "scenarios-edge",

			text: `a
b

scenarios: {
  x: {
    a
  }
}
`,
			key:       `a -> b`,
			boardPath: []string{"x"},

			expKey: `(a -> b)[0]`,
			exp: `a
b

scenarios: {
  x: {
    a
    a -> b
  }
}
`,
		},
		{
			name: "scenarios-edge-inherited",

			text: `a -> b

scenarios: {
  x: {
    a
  }
}
`,
			key:       `a -> b`,
			boardPath: []string{"x"},

			expKey: `(a -> b)[1]`,
			exp: `a -> b

scenarios: {
  x: {
    a
    a -> b
  }
}
`,
		},
		{
			name: "steps-basic",

			text: `a
d

steps: {
  x: {
    b
  }
}
`,
			key:       `c`,
			boardPath: []string{"x"},

			expKey: `c`,
			exp: `a
d

steps: {
  x: {
    b
    c
  }
}
`,
		},
		{
			name: "steps-edge",

			text: `a
d

steps: {
  x: {
    b
  }
}
`,
			key:       `d -> b`,
			boardPath: []string{"x"},

			expKey: `(d -> b)[0]`,
			exp: `a
d

steps: {
  x: {
    b
    d -> b
  }
}
`,
		},
		{
			name: "steps-conflict",

			text: `a
d

steps: {
  x: {
    b
  }
}
`,
			key:       `d`,
			boardPath: []string{"x"},

			expKey: `d 2`,
			exp: `a
d

steps: {
  x: {
    b
    d 2
  }
}
`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var newKey string
			et := editTest{
				text: tc.text,
				testFunc: func(g *d2graph.Graph) (*d2graph.Graph, error) {
					var err error
					g, newKey, err = d2oracle.Create(g, tc.boardPath, tc.key)
					return g, err
				},

				exp:    tc.exp,
				expErr: tc.expErr,
				assertions: func(t *testing.T, g *d2graph.Graph) {
					if newKey != tc.expKey {
						t.Fatalf("expected %q but got %q", tc.expKey, newKey)
					}
					if tc.assertions != nil {
						tc.assertions(t, g)
					}
				},
			}
			et.run(t)
		})
	}
}

func TestSet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		boardPath []string
		name      string
		text      string
		fsTexts   map[string]string
		key       string
		tag       *string
		value     *string

		expErr     string
		exp        string
		assertions func(t *testing.T, g *d2graph.Graph)
	}{
		{
			name: "base",
			text: ``,
			key:  `square`,

			exp: `square
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "square" {
					t.Fatalf("expected g.Objects[0].ID to be square: %#v", g.Objects[0])
				}
				if g.Objects[0].Label.MapKey.Value.Unbox() != nil {
					t.Fatalf("expected g.Objects[0].Label.Node.Value.Unbox() == nil: %#v", g.Objects[0].Label.MapKey.Value)
				}
				if d2format.Format(g.Objects[0].Label.MapKey.Key) != "square" {
					t.Fatalf("expected g.Objects[0].Label.Node.Key to be square: %#v", g.Objects[0].Label.MapKey.Key)
				}
			},
		},
		{
			name:  "edge",
			text:  `x -> y: one`,
			key:   `(x -> y)[0]`,
			value: go2.Pointer(`two`),

			exp: `x -> y: two
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "x" {
					t.Fatalf("expected g.Edges[0].Src.ID == x: %#v", g.Edges[0].Src.ID)
				}
				if g.Edges[0].Dst.ID != "y" {
					t.Fatalf("expected g.Edges[0].Dst.ID == y: %#v", g.Edges[0].Dst.ID)
				}
				if g.Edges[0].Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Label.Value == two: %#v", g.Edges[0].Label.Value)
				}
			},
		},
		{
			name:  "shape",
			text:  `square`,
			key:   `square.shape`,
			value: go2.Pointer(`square`),

			exp: `square: {shape: square}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "square" {
					t.Fatalf("expected g.Objects[0].ID to be square: %#v", g.Objects[0])
				}
				if g.Objects[0].Shape.Value != d2target.ShapeSquare {
					t.Fatalf("expected g.Objects[0].Shape.Value == square: %#v", g.Objects[0].Shape.Value)
				}
			},
		},
		{
			name:  "replace_shape",
			text:  `square.shape: square`,
			key:   `square.shape`,
			value: go2.Pointer(`circle`),

			exp: `square.shape: circle
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "square" {
					t.Fatalf("expected g.Objects[0].ID to be square: %#v", g.Objects[0])
				}
				if g.Objects[0].Shape.Value != d2target.ShapeCircle {
					t.Fatalf("expected g.Objects[0].Shape.Value == circle: %#v", g.Objects[0].Shape.Value)
				}
			},
		},
		{
			name: "new_style",
			text: `square
`,
			key:   `square.style.opacity`,
			value: go2.Pointer(`0.2`),
			exp: `square: {style.opacity: 0.2}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.AST.Nodes) != 1 {
					t.Fatal(g.AST)
				}
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 object but got %#v", len(g.Objects))
				}
				f, err := strconv.ParseFloat(g.Objects[0].Style.Opacity.Value, 64)
				if err != nil || f != 0.2 {
					t.Fatalf("expected g.Objects[0].Map.Nodes[0].MapKey.Value.Number.Value.Float64() == 0.2: %#v", f)
				}
			},
		},
		{
			name: "inline_style",
			text: `square: {style.opacity: 0.2}
`,
			key:   `square.style.fill`,
			value: go2.Pointer(`red`),
			exp: `square: {
  style.opacity: 0.2
  style.fill: red
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.AST.Nodes) != 1 {
					t.Fatal(g.AST)
				}
			},
		},
		{
			name: "expanded_map_style",
			text: `square: {
	style: {
    opacity: 0.1
  }
}
`,
			key:   `square.style.opacity`,
			value: go2.Pointer(`0.2`),
			exp: `square: {
  style: {
    opacity: 0.2
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.AST.Nodes) != 1 {
					t.Fatal(g.AST)
				}
				if len(g.AST.Nodes[0].MapKey.Value.Map.Nodes) != 1 {
					t.Fatalf("expected 1 node within square but got %v", len(g.AST.Nodes[0].MapKey.Value.Map.Nodes))
				}
				f, err := strconv.ParseFloat(g.Objects[0].Style.Opacity.Value, 64)
				if err != nil || f != 0.2 {
					t.Fatal(err, f)
				}
			},
		},
		{
			name: "replace_style",
			text: `square.style.opacity: 0.1
`,
			key:   `square.style.opacity`,
			value: go2.Pointer(`0.2`),
			exp: `square.style.opacity: 0.2
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.AST.Nodes) != 1 {
					t.Fatal(g.AST)
				}
				f, err := strconv.ParseFloat(g.Objects[0].Style.Opacity.Value, 64)
				if err != nil || f != 0.2 {
					t.Fatal(err, f)
				}
			},
		},
		{
			name: "replace_style_edgecase",
			text: `square.style.fill: orange
`,
			key:   `square.style.opacity`,
			value: go2.Pointer(`0.2`),
			exp: `square.style.fill: orange
square.style.opacity: 0.2
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.AST.Nodes) != 2 {
					t.Fatal(g.AST)
				}
				f, err := strconv.ParseFloat(g.Objects[0].Style.Opacity.Value, 64)
				if err != nil || f != 0.2 {
					t.Fatal(err, f)
				}
			},
		},
		{
			name: "set_position",
			text: `square
`,
			key:   `square.top`,
			value: go2.Pointer(`200`),
			exp: `square: {top: 200}
`,
		},
		{
			name: "replace_position",
			text: `square: {
  width: 100
  top: 32
	left: 44
}
`,
			key:   `square.top`,
			value: go2.Pointer(`200`),
			exp: `square: {
  width: 100
  top: 200
  left: 44
}
`,
		},
		{
			name: "set_dimensions",
			text: `square
`,
			key:   `square.width`,
			value: go2.Pointer(`200`),
			exp: `square: {width: 200}
`,
		},
		{
			name: "replace_dimensions",
			text: `square: {
  width: 100
}
`,
			key:   `square.width`,
			value: go2.Pointer(`200`),
			exp: `square: {
  width: 200
}
`,
		},
		{
			name: "set_tooltip",
			text: `square
`,
			key:   `square.tooltip`,
			value: go2.Pointer(`y`),
			exp: `square: {tooltip: y}
`,
		},
		{
			name: "replace_tooltip",
			text: `square: {
  tooltip: x
}
`,
			key:   `square.tooltip`,
			value: go2.Pointer(`y`),
			exp: `square: {
  tooltip: y
}
`,
		},
		{
			name: "replace_link",
			text: `square: {
  link: https://google.com
}
`,
			key:   `square.link`,
			value: go2.Pointer(`https://apple.com`),
			exp: `square: {
  link: https://apple.com
}
`,
		},
		{
			name: "replace_arrowhead",
			text: `x -> y: {
  target-arrowhead.shape: diamond
}
`,
			key:   `(x -> y)[0].target-arrowhead.shape`,
			value: go2.Pointer(`circle`),
			exp: `x -> y: {
  target-arrowhead.shape: circle
}
`,
		},
		{
			name: "replace_arrowhead_map",
			text: `x -> y: {
  target-arrowhead: {
    shape: diamond
  }
}
`,
			key:   `(x -> y)[0].target-arrowhead.shape`,
			value: go2.Pointer(`circle`),
			exp: `x -> y: {
  target-arrowhead: {
    shape: circle
  }
}
`,
		},
		{
			name: "replace_edge_style_map",
			text: `x -> y: {
  style: {
    stroke-dash: 3
  }
}
`,
			key:   `(x -> y)[0].style.stroke-dash`,
			value: go2.Pointer(`4`),
			exp: `x -> y: {
  style: {
    stroke-dash: 4
  }
}
`,
		},
		{
			name: "replace_edge_style",
			text: `x -> y: {
  style.stroke-width: 1
  style.stroke-dash: 4
}
`,
			key:   `(x -> y)[0].style.stroke-dash`,
			value: go2.Pointer(`3`),
			exp: `x -> y: {
  style.stroke-width: 1
  style.stroke-dash: 3
}
`,
		},
		{
			name:  "set_fill_pattern",
			text:  `square`,
			key:   `square.style.fill-pattern`,
			value: go2.Pointer(`grain`),
			exp: `square: {style.fill-pattern: grain}
`,
		},
		{
			name: "replace_fill_pattern",
			text: `square: {
  style.fill-pattern: lines
}
`,
			key:   `square.style.fill-pattern`,
			value: go2.Pointer(`grain`),
			exp: `square: {
  style.fill-pattern: grain
}
`,
		},
		{
			name: "classes-style",
			text: `classes: {
  a: {
    style.fill: red
  }
}
b.class: a
`,
			key:   `b.style.fill`,
			value: go2.Pointer(`green`),
			exp: `classes: {
  a: {
    style.fill: red
  }
}
b.class: a
b.style.fill: green
`,
		},
		{
			name: "dupe-classes-style",
			text: `classes: {
  a: {
    style.fill: red
  }
}
b.class: a
b.style.fill: red
`,
			key:   `b.style.fill`,
			value: go2.Pointer(`green`),
			exp: `classes: {
  a: {
    style.fill: red
  }
}
b.class: a
b.style.fill: green
`,
		},
		{
			name: "unapplied-classes-style",
			text: `classes: {
  a: {
    style.fill: red
  }
}
b.style.fill: red
`,
			key:   `b.style.fill`,
			value: go2.Pointer(`green`),
			exp: `classes: {
  a: {
    style.fill: red
  }
}
b.style.fill: green
`,
		},
		{
			name: "unapplied-classes-style-2",
			text: `classes: {
  a: {
    style.fill: red
  }
}
b
`,
			key:   `b.style.fill`,
			value: go2.Pointer(`green`),
			exp: `classes: {
  a: {
    style.fill: red
  }
}
b: {style.fill: green}
`,
		},
		{
			name: "class-with-label",
			text: `classes: {
  user: {
    label: ""
  }
}

a.class: user
`,
			key:   `a.style.opacity`,
			value: go2.Pointer(`0.5`),
			exp: `classes: {
  user: {
    label: ""
  }
}

a.class: user
a.style.opacity: 0.5
`,
		},
		{
			name: "edge-class-with-label",
			text: `classes: {
  user: {
    label: ""
  }
}

a -> b: {
  class: user
}
`,
			key:   `(a -> b)[0].style.opacity`,
			value: go2.Pointer(`0.5`),
			exp: `classes: {
  user: {
    label: ""
  }
}

a -> b: {
  class: user
  style.opacity: 0.5
}
`,
		},
		{
			name: "var-with-label",
			text: `vars: {
  user: ""
}

a: ${user}
`,
			key:   `a.style.opacity`,
			value: go2.Pointer(`0.5`),
			exp: `vars: {
  user: ""
}

a: ${user} {style.opacity: 0.5}
`,
		},
		{
			name: "glob-with-label",
			text: `*.label: ""
a
`,
			key:   `a.style.opacity`,
			value: go2.Pointer(`0.5`),
			exp: `*.label: ""
a
a.style.opacity: 0.5
`,
		},
		{
			name: "label_unset",
			text: `square: "Always try to do things in chronological order; it's less confusing that way."
`,
			key:   `square.label`,
			value: nil,

			exp: `square
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "square" {
					t.Fatalf("expected g.Objects[0].ID to be square: %#v", g.Objects[0])
				}
				if g.Objects[0].Shape.Value == d2target.ShapeSquare {
					t.Fatalf("expected g.Objects[0].Shape.Value == square: %#v", g.Objects[0].Shape.Value)
				}
			},
		},
		{
			name:  "label",
			text:  `square`,
			key:   `square.label`,
			value: go2.Pointer(`Always try to do things in chronological order; it's less confusing that way.`),

			exp: `square: "Always try to do things in chronological order; it's less confusing that way."
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "square" {
					t.Fatalf("expected g.Objects[0].ID to be square: %#v", g.Objects[0])
				}
				if g.Objects[0].Shape.Value == d2target.ShapeSquare {
					t.Fatalf("expected g.Objects[0].Shape.Value == square: %#v", g.Objects[0].Shape.Value)
				}
			},
		},
		{
			name:  "label_replace",
			text:  `square: I am deeply CONCERNED and I want something GOOD for BREAKFAST!`,
			key:   `square`,
			value: go2.Pointer(`Always try to do things in chronological order; it's less confusing that way.`),

			exp: `square: "Always try to do things in chronological order; it's less confusing that way."
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.AST.Nodes) != 1 {
					t.Fatal(g.AST)
				}
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].ID != "square" {
					t.Fatal(g.Objects[0])
				}
				if g.Objects[0].Label.Value == "I am deeply CONCERNED and I want something GOOD for BREAKFAST!" {
					t.Fatal(g.Objects[0].Label.Value)
				}
			},
		},
		{
			name:  "map_key_missing",
			text:  `a -> b`,
			key:   `a`,
			value: go2.Pointer(`Never offend people with style when you can offend them with substance.`),

			exp: `a -> b
a: Never offend people with style when you can offend them with substance.
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		{
			name: "nested_alex",
			text: `this: {
  label: do
  test -> here: asdf
}`,
			key: `this.here`,
			value: go2.Pointer(`How much of their influence on you is a result of your influence on them?
A conference is a gathering of important people who singly can do nothing`),

			exp: `this: {
  label: do
  test -> here: asdf
  here: "How much of their influence on you is a result of your influence on them?\nA conference is a gathering of important people who singly can do nothing"
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		{
			name: "label_primary",
			text: `oreo: {
 q -> z
}`,
			key:   `oreo`,
			value: go2.Pointer(`QOTD: "It's been Monday all week today."`),

			exp: `oreo: 'QOTD: "It''s been Monday all week today."' {
  q -> z
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_index_nested",
			text: `oreo: {
 q -> z
}`,
			key:   `(oreo.q -> oreo.z)[0]`,
			value: go2.Pointer(`QOTD`),

			exp: `oreo: {
  q -> z: QOTD
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_index_case",
			text: `Square: {
  Square -> Square 2
}
z: {
  x -> y
}
`,
			key:   `Square.(Square -> Square 2)[0]`,
			value: go2.Pointer(`two`),

			exp: `Square: {
  Square -> Square 2: two
}
z: {
  x -> y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 6 {
					t.Fatalf("expected 6 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 2 {
					t.Fatalf("expected 2 edges: %#v", g.Edges)
				}
				if g.Edges[0].Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Label.Value == two: %#v", g.Edges[0].Label.Value)
				}
			},
		},
		{
			name: "icon",
			text: `meow
			`,
			key:   `meow.icon`,
			value: go2.Pointer(`https://icons.terrastruct.com/essentials/087-menu.svg`),

			exp: `meow: {icon: https://icons.terrastruct.com/essentials/087-menu.svg}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].Icon.String() != "https://icons.terrastruct.com/essentials/087-menu.svg" {
					t.Fatal(g.Objects[0].Icon.String())
				}
			},
		},
		{
			name: "edge_chain",
			text: `oreo: {
  q -> z -> p: wsup
}`,
			key: `(oreo.q -> oreo.z)[0]`,
			value: go2.Pointer(`QOTD:
  "It's been Monday all week today."`),

			exp: `oreo: {
  q -> z -> p: wsup
  (q -> z)[0]: "QOTD:\n  \"It's been Monday all week today.\""
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 2 {
					t.Fatalf("expected 2 edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_nested_label_set",
			text: `oreo: {
  q -> z: wsup
}`,
			key:   `(oreo.q -> oreo.z)[0].label`,
			value: go2.Pointer(`yo`),

			exp: `oreo: {
  q -> z: yo
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "q" {
					t.Fatal(g.Edges[0].Src.ID)
				}
			},
		},
		{
			name: "shape_nested_style_set",
			text: `x
`,
			key:   `x.style.opacity`,
			value: go2.Pointer(`0.4`),

			exp: `x: {style.opacity: 0.4}
`,
		},
		{
			name: "edge_nested_style_set",
			text: `oreo: {
  q -> z: wsup
}
`,
			key:   `(oreo.q -> oreo.z)[0].style.opacity`,
			value: go2.Pointer(`0.4`),

			exp: `oreo: {
  q -> z: wsup {style.opacity: 0.4}
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, 3, len(g.Objects))
				assert.JSON(t, 1, len(g.Edges))
				assert.JSON(t, "q", g.Edges[0].Src.ID)
				assert.JSON(t, "0.4", g.Edges[0].Style.Opacity.Value)
			},
		},
		{
			name: "edge_chain_append_style",
			text: `x -> y -> z
`,
			key:   `(x -> y)[0].style.animated`,
			value: go2.Pointer(`true`),

			exp: `x -> y -> z
(x -> y)[0].style.animated: true
`,
		},
		{
			name: "edge_chain_existing_style",
			text: `x -> y -> z
(y -> z)[0].style.opacity: 0.4
`,
			key:   `(y -> z)[0].style.animated`,
			value: go2.Pointer(`true`),

			exp: `x -> y -> z
(y -> z)[0].style.opacity: 0.4
(y -> z)[0].style.animated: true
`,
		},
		{
			name: "edge_key_and_key",
			text: `a
a.b -> a.c
`,
			key:   `a.(b -> c)[0].style.animated`,
			value: go2.Pointer(`true`),

			exp: `a
a.b -> a.c: {style.animated: true}
`,
		},
		{
			name: "edge_label",
			text: `a -> b: "yo"
`,
			key:   `(a -> b)[0].style.animated`,
			value: go2.Pointer(`true`),

			exp: `a -> b: "yo" {style.animated: true}
`,
		},
		{
			name: "edge_append_style",
			text: `x -> y
`,
			key:   `(x -> y)[0].style.animated`,
			value: go2.Pointer(`true`),

			exp: `x -> y: {style.animated: true}
`,
		},
		{
			name: "edge_set_arrowhead",
			text: `x -> y
`,
			key:   `(x -> y)[0].target-arrowhead.shape`,
			value: go2.Pointer(`diamond`),

			exp: `x -> y: {target-arrowhead.shape: diamond}
`,
		},
		{
			name: "edge-arrowhead-filled/1",
			text: `x -> y
`,
			key:   `(x -> y)[0].target-arrowhead.style.filled`,
			value: go2.Pointer(`true`),

			exp: `x -> y: {target-arrowhead.style.filled: true}
`,
		},
		{
			name: "edge-arrowhead-filled/2",
			text: `x -> y: {
  target-arrowhead: * {
    shape: diamond
  }
}
`,
			key:   `(x -> y)[0].target-arrowhead.style.filled`,
			value: go2.Pointer(`true`),

			exp: `x -> y: {
  target-arrowhead: * {
    shape: diamond
    style.filled: true
  }
}
`,
		},
		{
			name: "edge-arrowhead-filled/3",
			text: `x -> y: {
	target-arrowhead.shape: diamond
}
`,
			key:   `(x -> y)[0].target-arrowhead.style.filled`,
			value: go2.Pointer(`true`),

			exp: `x -> y: {
  target-arrowhead.shape: diamond
  target-arrowhead.style.filled: true
}
`,
		},
		{
			name: "edge-arrowhead-filled/4",
			text: `x -> y: {
	target-arrowhead.shape: diamond
  target-arrowhead.style.filled: true
}
`,
			key:   `(x -> y)[0].target-arrowhead.style.filled`,
			value: go2.Pointer(`false`),

			exp: `x -> y: {
  target-arrowhead.shape: diamond
  target-arrowhead.style.filled: false
}
`,
		},
		{
			name: "edge-arrowhead-filled/5",
			text: `x -> y: {
	target-arrowhead.shape: diamond
	target-arrowhead.style: {
    filled: false
  }
}
`,
			key:   `(x -> y)[0].target-arrowhead.style.filled`,
			value: go2.Pointer(`true`),

			exp: `x -> y: {
  target-arrowhead.shape: diamond
  target-arrowhead.style: {
    filled: true
  }
}
`,
		},
		{
			name: "edge_replace_arrowhead",
			text: `x -> y: {target-arrowhead.shape: circle}
`,
			key:   `(x -> y)[0].target-arrowhead.shape`,
			value: go2.Pointer(`diamond`),

			exp: `x -> y: {target-arrowhead.shape: diamond}
`,
		},
		{
			name: "edge_replace_arrowhead_indexed",
			text: `x -> y
(x -> y)[0].target-arrowhead.shape: circle
`,
			key:   `(x -> y)[0].target-arrowhead.shape`,
			value: go2.Pointer(`diamond`),

			exp: `x -> y
(x -> y)[0].target-arrowhead.shape: diamond
`,
		},
		{
			name: "edge_merge_arrowhead",
			text: `x -> y: {
	target-arrowhead: {
		label: 1
  }
}
`,
			key:   `(x -> y)[0].target-arrowhead.shape`,
			value: go2.Pointer(`diamond`),

			exp: `x -> y: {
  target-arrowhead: {
    label: 1
    shape: diamond
  }
}
`,
		},
		{
			name: "edge_merge_style",
			text: `x -> y: {
	style: {
    opacity: 0.4
  }
}
`,
			key:   `(x -> y)[0].style.animated`,
			value: go2.Pointer(`true`),

			exp: `x -> y: {
  style: {
    opacity: 0.4
    animated: true
  }
}
`,
		},
		{
			name: "edge_flat_merge_arrowhead",
			text: `x -> y -> z
(x -> y)[0].target-arrowhead.shape: diamond
`,
			key:   `(x -> y)[0].target-arrowhead.shape`,
			value: go2.Pointer(`circle`),

			exp: `x -> y -> z
(x -> y)[0].target-arrowhead.shape: circle
`,
		},
		{
			name: "edge_index_merge_style",
			text: `x -> y -> z
(x -> y)[0].style.opacity: 0.4
`,
			key:   `(x -> y)[0].style.opacity`,
			value: go2.Pointer(`0.5`),

			exp: `x -> y -> z
(x -> y)[0].style.opacity: 0.5
`,
		},
		{
			name: "edge_chain_nested_set",
			text: `oreo: {
  q -> z -> p: wsup
}`,
			key:   `(oreo.q -> oreo.z)[0].style.opacity`,
			value: go2.Pointer(`0.4`),

			exp: `oreo: {
  q -> z -> p: wsup
  (q -> z)[0].style.opacity: 0.4
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 2 {
					t.Fatalf("expected 2 edges: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "q" {
					t.Fatal(g.Edges[0].Src.ID)
				}
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatal(g.Edges[0].Style.Opacity.Value)
				}
			},
		},
		{
			name: "block_string_oneline",

			text:  ``,
			key:   `x`,
			tag:   go2.Pointer("md"),
			value: go2.Pointer(`|||what's up|||`),

			exp: `x: ||||md |||what's up||| ||||
`,
		},
		{
			name: "block_string_multiline",

			text: ``,
			key:  `x`,
			tag:  go2.Pointer("md"),
			value: go2.Pointer(`# header
He has not acquired a fortune; the fortune has acquired him.
He has not acquired a fortune; the fortune has acquired him.`),

			exp: `x: |md
  # header
  He has not acquired a fortune; the fortune has acquired him.
  He has not acquired a fortune; the fortune has acquired him.
|
`,
		},
		// TODO: pass
		/*
			{
				name: "oneline_constraint",

				text: `My Table: {
					shape: sql_table
					column: int
				}
				`,
				key:   `My Table.column.constraint`,
				value: utils.Pointer("PK"),

				exp: `My Table: {
					shape: sql_table
					column: int {constraint: PK}
				}
				`,
			},
		*/
		// TODO: pass
		/*
					{
						name: "oneline_style",

						text: `foo: bar
			`,
						key:   `foo.style_fill`,
						value: utils.Pointer("red"),

						exp: `foo: bar {style_fill: red}
			`,
					},
		*/

		{
			name: "errors/bad_tag",

			text: `x.icon: hello
`,
			key: "x.icon",
			tag: go2.Pointer("one two"),
			value: go2.Pointer(`three
four
five
six
`),

			expErr: `failed to set "x.icon" to "one two" "\"three\\nfour\\nfive\\nsix\\n\"": spaces are not allowed in blockstring tags`,
		},
		{
			name: "layers-usable-ref-style",

			text: `a

layers: {
  x: {
    a
  }
}
`,
			key:       `a.style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a

layers: {
  x: {
    a: {style.opacity: 0.2}
  }
}
`,
		},
		{
			name: "layers-unusable-ref-style",

			text: `a

layers: {
  x: {
    b
  }
}
`,
			key:       `a.style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a

layers: {
  x: {
    b
    a.style.opacity: 0.2
  }
}
`,
		},
		{
			name: "scenarios-usable-ref-style",

			text: `a: outer

scenarios: {
  x: {
		a: inner
  }
}
`,
			key:       `a.style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a: outer

scenarios: {
  x: {
    a: inner {style.opacity: 0.2}
  }
}
`,
		},
		{
			name: "scenarios-multiple",

			text: `a

scenarios: {
  x: {
		b
    a.style.fill: red
  }
}
`,
			key:       `a.style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a

scenarios: {
  x: {
    b
    a.style.fill: red
    a.style.opacity: 0.2
  }
}
`,
		},
		{
			name: "scenarios-nested-usable-ref-style",

			text: `a: {
  b: outer
}

scenarios: {
  x: {
    a: {
      b: inner
    }
  }
}
`,
			key:       `a.b.style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a: {
  b: outer
}

scenarios: {
  x: {
    a: {
      b: inner {style.opacity: 0.2}
    }
  }
}
`,
		},
		{
			name: "scenarios-unusable-ref-style",

			text: `a

scenarios: {
  x: {
    b
  }
}
`,
			key:       `a.style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a

scenarios: {
  x: {
    b
    a.style.opacity: 0.2
  }
}
`,
		},
		{
			name: "scenarios-label-primary",

			text: `a: {
  style.opacity: 0.2
}

scenarios: {
  x: {
		a: {
      style.opacity: 0.3
    }
  }
}
`,
			key:       `a`,
			value:     go2.Pointer(`b`),
			boardPath: []string{"x"},

			exp: `a: {
  style.opacity: 0.2
}

scenarios: {
  x: {
    a: b {
      style.opacity: 0.3
    }
  }
}
`,
		},
		{
			name: "scenarios-label-primary-missing",

			text: `a: {
  style.opacity: 0.2
}

scenarios: {
  x: {
		b
  }
}
`,
			key:       `a`,
			value:     go2.Pointer(`b`),
			boardPath: []string{"x"},

			exp: `a: {
  style.opacity: 0.2
}

scenarios: {
  x: {
    b
    a: b
  }
}
`,
		},
		{
			name: "scenarios-edge-set",

			text: `a -> b

scenarios: {
  x: {
		c
  }
}
`,
			key:       `(a -> b)[0].style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a -> b

scenarios: {
  x: {
    c
    (a -> b)[0].style.opacity: 0.2
  }
}
`,
		},
		{
			name: "scenarios-existing-edge-set",

			text: `a -> b

scenarios: {
  x: {
    a -> b
		c
  }
}
`,
			key:       `(a -> b)[1].style.opacity`,
			value:     go2.Pointer(`0.2`),
			boardPath: []string{"x"},

			exp: `a -> b

scenarios: {
  x: {
    a -> b: {style.opacity: 0.2}
    c
  }
}
`,
		},
		{
			name: "scenarios-arrowhead",

			text: `a -> b: {
  target-arrowhead.shape: triangle
}
x -> y

scenarios: {
  x: {
    (a -> b)[0]: {
       target-arrowhead.shape: circle
    }
		c -> d
  }
}
`,
			key:       `(a -> b)[0].target-arrowhead.shape`,
			value:     go2.Pointer(`diamond`),
			boardPath: []string{"x"},

			exp: `a -> b: {
  target-arrowhead.shape: triangle
}
x -> y

scenarios: {
  x: {
    (a -> b)[0]: {
      target-arrowhead.shape: diamond
    }
    c -> d
  }
}
`,
		},
		{
			name: "import/1",

			text: `x: {
  ...@meow.x
  y
}
`,
			fsTexts: map[string]string{
				"meow.d2": `x: {
  style.fill: blue
}
`,
			},
			key:   `x.style.stroke`,
			value: go2.Pointer(`red`),
			exp: `x: {
  ...@meow.x
  y
  style.stroke: red
}
`,
		},
		{
			name: "import/2",

			text: `x: {
  ...@meow.x
  y
}
`,
			fsTexts: map[string]string{
				"meow.d2": `x: {
  style.fill: blue
}
`,
			},
			key:   `x.style.fill`,
			value: go2.Pointer(`red`),
			exp: `x: {
  ...@meow.x
  y
  style.fill: red
}
`,
		},
		{
			name: "import/3",

			text: `x: {
  ...@meow.x
  y
	style.fill: red
}
`,
			fsTexts: map[string]string{
				"meow.d2": `x: {
  style.fill: blue
}
`,
			},
			key:   `x.style.fill`,
			value: go2.Pointer(`yellow`),
			exp: `x: {
  ...@meow.x
  y
  style.fill: yellow
}
`,
		},
		{
			name: "import/4",

			text: `...@yo
a`,
			fsTexts: map[string]string{
				"yo.d2": `b`,
			},
			key:   `b.style.fill`,
			value: go2.Pointer(`red`),
			exp: `...@yo
a
b.style.fill: red
`,
		},
		{
			name: "import/5",

			text: `a
x: {
  ...@yo
}`,
			fsTexts: map[string]string{
				"yo.d2": `b`,
			},
			key:   `x.b.style.fill`,
			value: go2.Pointer(`red`),
			exp: `a
x: {
  ...@yo
  b.style.fill: red
}
`,
		},
		{
			name: "import/6",

			text: `a
x: @yo`,
			fsTexts: map[string]string{
				"yo.d2": `b`,
			},
			key:   `x.b.style.fill`,
			value: go2.Pointer(`red`),
			exp: `a
x: @yo
x.b.style.fill: red
`,
		},
		{
			name: "import/7",

			text: `...@yo
b.style.fill: red`,
			fsTexts: map[string]string{
				"yo.d2": `b`,
			},
			key:   `b.style.opacity`,
			value: go2.Pointer("0.5"),
			exp: `...@yo
b.style.fill: red
b.style.opacity: 0.5
`,
		},
		{
			name: "import/8",

			text: `a

layers: {
  x: @yo
}`,
			boardPath: []string{"x"},
			fsTexts: map[string]string{
				"yo.d2": `b`,
			},
			key:   `b.style.fill`,
			value: go2.Pointer(`red`),
			exp: `a

layers: {
  x: {
    ...@yo
    b.style.fill: red
  }
}
`,
		},
		{
			name: "import/9",

			text: `...@yo
`,
			fsTexts: map[string]string{
				"yo.d2": `a -> b`,
			},
			key:   `(a -> b)[0].style.stroke`,
			value: go2.Pointer(`red`),
			exp: `...@yo
(a -> b)[0].style.stroke: red
`,
		},
		{
			name: "label-near/1",

			text: `x
`,
			key:   `x.label.near`,
			value: go2.Pointer(`bottom-right`),
			exp: `x: {label.near: bottom-right}
`,
		},
		{
			name: "label-near/2",

			text: `x.label.near: bottom-left
`,
			key:   `x.label.near`,
			value: go2.Pointer(`bottom-right`),
			exp: `x.label.near: bottom-right
`,
		},
		{
			name: "label-near/3",

			text: `x: {
  label.near: bottom-left
}
`,
			key:   `x.label.near`,
			value: go2.Pointer(`bottom-right`),
			exp: `x: {
  label.near: bottom-right
}
`,
		},
		{
			name: "label-near/4",

			text: `x: {
  label: hi {
    near: bottom-left
  }
}
`,
			key:   `x.label.near`,
			value: go2.Pointer(`bottom-right`),
			exp: `x: {
  label: hi {
    near: bottom-right
  }
}
`,
		},
		{
			name: "label-near/5",

			text: `x: hi {
	label: {
    near: bottom-left
	}
}
`,
			key:   `x.label.near`,
			value: go2.Pointer(`bottom-right`),
			exp: `x: hi {
  label: {
    near: bottom-right
  }
}
`,
		},
		{
			name: "glob-field/1",

			text: `*.style.fill: red
a
b
`,
			key:   `a.style.fill`,
			value: go2.Pointer(`blue`),
			exp: `*.style.fill: red
a: {style.fill: blue}
b
`,
		},
		{
			name: "glob-field/2",

			text: `(* -> *)[*].style.stroke: red
a -> b
a -> b
`,
			key:   `(a -> b)[0].style.stroke`,
			value: go2.Pointer(`blue`),
			exp: `(* -> *)[*].style.stroke: red
a -> b: {style.stroke: blue}
a -> b
`,
		},
		{
			name: "glob-field/3",

			text: `(* -> *)[*].style.stroke: red
a -> b: {style.stroke: blue}
a -> b
`,
			key:   `(a -> b)[0].style.stroke`,
			value: go2.Pointer(`green`),
			exp: `(* -> *)[*].style.stroke: red
a -> b: {style.stroke: green}
a -> b
`,
		},
		{
			name: "nested-edge-chained/1",

			text: `a: {
  b: {
    c
  }
}

x -> a.b -> a.b.c
`,
			key:   `(a.b -> a.b.c)[0].style.stroke`,
			value: go2.Pointer(`green`),
			exp: `a: {
  b: {
    c
  }
}

x -> a.b -> a.b.c
(a.b -> a.b.c)[0].style.stroke: green
`,
		},
		{
			name: "nested-edge-chained/2",

			text: `z: {
  a: {
    b: {
      c
    }
  }
  x -> a.b -> a.b.c
}
`,
			key:   `(z.a.b -> z.a.b.c)[0].style.stroke`,
			value: go2.Pointer(`green`),
			exp: `z: {
  a: {
    b: {
      c
    }
  }
  x -> a.b -> a.b.c
  (a.b -> a.b.c)[0].style.stroke: green
}
`,
		},
		{
			name: "edge-comment",

			text: `x -> y: {
  # hi
  style.stroke: blue
}
`,
			key:   `(x -> y)[0].style.stroke`,
			value: go2.Pointer(`green`),
			exp: `x -> y: {
  # hi
  style.stroke: green
}
`,
		},
		{
			name: "scenario-child",

			text: `a -> b

scenarios: {
  x: {
    hi
  }
}
`,
			key:       `(a -> b)[0].style.stroke-width`,
			value:     go2.Pointer(`3`),
			boardPath: []string{"x"},
			exp: `a -> b

scenarios: {
  x: {
    hi
    (a -> b)[0].style.stroke-width: 3
  }
}
`,
		},
		{
			name: "scenario-grandchild",

			text: `a -> b

scenarios: {
	x: {
		scenarios: {
			c: {
				(a -> b)[0].style.bold: true
			}
		}
	}
}
		`,
			key:       `(a -> b)[0].style.stroke-width`,
			value:     go2.Pointer(`3`),
			boardPath: []string{"x", "c"},
			exp: `a -> b

scenarios: {
  x: {
    scenarios: {
      c: {
        (a -> b)[0].style.bold: true
        (a -> b)[0].style.stroke-width: 3
      }
    }
  }
}
`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			et := editTest{
				text:    tc.text,
				fsTexts: tc.fsTexts,
				testFunc: func(g *d2graph.Graph) (*d2graph.Graph, error) {
					return d2oracle.Set(g, tc.boardPath, tc.key, tc.tag, tc.value)
				},

				exp:        tc.exp,
				expErr:     tc.expErr,
				assertions: tc.assertions,
			}
			et.run(t)
		})
	}
}

func TestReconnectEdge(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		boardPath []string
		text      string
		edgeKey   string
		newSrc    string
		newDst    string

		expErr     string
		exp        string
		assertions func(t *testing.T, g *d2graph.Graph)
	}{
		{
			name: "basic",
			text: `a
b
c
a -> b
`,
			edgeKey: `(a -> b)[0]`,
			newDst:  "c",
			exp: `a
b
c
a -> c
`,
		},
		{
			name: "src",
			text: `a
b
c
a -> b
`,
			edgeKey: `(a -> b)[0]`,
			newSrc:  "c",
			exp: `a
b
c
c -> b
`,
		},
		{
			name: "both",
			text: `a
b
c
a -> b
`,
			edgeKey: `(a -> b)[0]`,
			newSrc:  "b",
			newDst:  "a",
			exp: `a
b
c
b -> a
`,
		},
		{
			name: "contained",
			text: `a.x -> a.y
a.z`,
			edgeKey: `a.(x -> y)[0]`,
			newDst:  "a.z",
			exp: `a.x -> a.z
a.y
a.z
`,
		},
		{
			name: "scope_outer",
			text: `a: {
  x -> y
}
b`,
			edgeKey: `(a.x -> a.y)[0]`,
			newDst:  "b",
			exp: `a: {
  x -> _.b
  y
}
b
`,
		},
		{
			name: "scope_inner",
			text: `a: {
  x -> y
	z: {
    b
  }
}`,
			edgeKey: `(a.x -> a.y)[0]`,
			newDst:  "a.z.b",
			exp: `a: {
  x -> z.b
  y

  z: {
    b
  }
}
`,
		},
		{
			name: "loop",
			text: `a -> a
b`,
			edgeKey: `(a -> a)[0]`,
			newDst:  "b",
			exp: `a -> b
b
`,
		},
		{
			name: "preserve_old_obj",
			text: `a -> b
(a -> b)[0].style.stroke: red
c`,
			edgeKey: `(a -> b)[0]`,
			newSrc:  "a",
			newDst:  "c",
			exp: `a -> c
b
(a -> c)[0].style.stroke: red
c
`,
		},
		{
			name: "middle_chain",
			text: `a -> b -> c
x`,
			edgeKey: `(a -> b)[0]`,
			newDst:  "x",
			exp: `b -> c
a -> x
x
`,
		},
		{
			name: "middle_chain_src",
			text: `a -> b -> c
x`,
			edgeKey: `(b -> c)[0]`,
			newSrc:  "x",
			exp: `a -> b
x -> c
x
`,
		},
		{
			name: "middle_chain_both",
			text: `a -> b -> c -> d
x`,
			edgeKey: `(b -> c)[0]`,
			newSrc:  "x",
			newDst:  "x",
			exp: `a -> b
c -> d
x -> x
x
`,
		},
		{
			name: "middle_chain_first",
			text: `a -> b -> c -> d
x`,
			edgeKey: `(a -> b)[0]`,
			newSrc:  "x",
			exp: `a
x -> b -> c -> d
x
`,
		},
		{
			name: "middle_chain_last",
			text: `a -> b -> c -> d
x`,
			edgeKey: `(c -> d)[0]`,
			newDst:  "x",
			exp: `a -> b -> c -> x
d
x
`,
		},
		// These _3 and _4 match the delta tests
		{
			name: "in_chain_3",

			text: `a -> b -> a -> c
`,
			edgeKey: "(a -> b)[0]",
			newDst:  "c",

			exp: `b -> a -> c
a -> c
`,
		},
		{
			name: "in_chain_4",

			text: `a -> c -> a -> c
b
`,
			edgeKey: "(a -> c)[0]",
			newDst:  "b",

			exp: `c -> a -> c
a -> b
b
`,
		},
		{
			name: "indexed_ref",
			text: `a -> b
x
(a -> b)[0].style.stroke: red
`,
			edgeKey: `(a -> b)[0]`,
			newDst:  "x",
			exp: `a -> x
b
x
(a -> x)[0].style.stroke: red
`,
		},
		{
			name: "reverse",
			text: `a -> b
`,
			edgeKey: `(a -> b)[0]`,
			newSrc:  "b",
			newDst:  "a",
			exp: `b -> a
`,
		},
		{
			name: "second_index",
			text: `a -> b: {
  style.stroke: blue
}
a -> b: {
  style.stroke: red
}
x
`,
			edgeKey: `(a -> b)[1]`,
			newDst:  "x",
			exp: `a -> b: {
  style.stroke: blue
}
a -> x: {
  style.stroke: red
}
x
`,
		},
		{
			name: "nonexistant_edge",
			text: `a -> b
`,
			edgeKey: `(b -> a)[0]`,
			newDst:  "a",
			expErr:  "edge not found",
		},
		{
			name: "nonexistant_obj",
			text: `a -> b
`,
			edgeKey: `(a -> b)[0]`,
			newDst:  "x",
			expErr:  "newDst not found",
		},
		{
			name: "layers-basic",
			text: `a

layers: {
  x: {
    b
    c
    a -> b
  }
}
`,
			boardPath: []string{"x"},
			edgeKey:   `(a -> b)[0]`,
			newDst:    "c",
			exp: `a

layers: {
  x: {
    b
    c
    a -> c
  }
}
`,
		},
		{
			name: "scenarios-basic",
			text: `a

scenarios: {
  x: {
    b
    c
    a -> b
  }
}
`,
			boardPath: []string{"x"},
			edgeKey:   `(a -> b)[0]`,
			newDst:    "c",
			exp: `a

scenarios: {
  x: {
    b
    c
    a -> c
  }
}
`,
		},
		{
			name: "scenarios-outer-scope",
			text: `a

scenarios: {
  x: {
    d -> b
  }
}
`,
			boardPath: []string{"x"},
			edgeKey:   `(d -> b)[0]`,
			newDst:    "a",
			exp: `a

scenarios: {
  x: {
    d -> a
    b
  }
}
`,
		},
		{
			name: "scenarios-chain",
			text: `a -> b -> c

scenarios: {
  x: {
    d
  }
}
`,
			boardPath: []string{"x"},
			edgeKey:   `(a -> b)[0]`,
			newDst:    "d",
			expErr:    `operation would modify AST outside of given scope`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			et := editTest{
				text: tc.text,
				testFunc: func(g *d2graph.Graph) (*d2graph.Graph, error) {
					var newSrc *string
					var newDst *string
					if tc.newSrc != "" {
						newSrc = &tc.newSrc
					}
					if tc.newDst != "" {
						newDst = &tc.newDst
					}
					return d2oracle.ReconnectEdge(g, tc.boardPath, tc.edgeKey, newSrc, newDst)
				},

				exp:        tc.exp,
				expErr:     tc.expErr,
				assertions: tc.assertions,
			}
			et.run(t)
		})
	}
}

func TestRename(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		boardPath []string

		text    string
		fsTexts map[string]string
		key     string
		newName string

		expErr     string
		exp        string
		assertions func(t *testing.T, g *d2graph.Graph)
	}{
		{
			name: "flat",

			text: `nerve-gift-earther
`,
			key:     `nerve-gift-earther`,
			newName: `---`,

			exp: `"---"
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected one object: %#v", g.Objects)
				}
				if g.Objects[0].ID != `"---"` {
					t.Fatalf("unexpected object id: %q", g.Objects[0].ID)
				}
			},
		},
		{
			name: "generated",

			text: `Square
`,
			key:     `Square`,
			newName: `Square`,

			exp: `Square
`,
		},
		{
			name: "generated-conflict",

			text: `Square
Square 2
`,
			key:     `Square 2`,
			newName: `Square`,

			exp: `Square
Square 2
`,
		},
		{
			name: "near",

			text: `x: {
  near: y
}
y
`,
			key:     `y`,
			newName: `z`,

			exp: `x: {
  near: z
}
z
`,
		},
		{
			name: "conflict",

			text: `lalal
la
`,
			key:     `lalal`,
			newName: `la`,

			exp: `la 2
la
`,
		},
		{
			name: "conflict 2",

			text: `1.2.3: {
  4
  5
}
`,
			key:     "1.2.3.4",
			newName: "5",

			exp: `1.2.3: {
  5 2
  5
}
`,
		},
		{
			name: "conflict_with_dots",

			text: `"a.b"
y
`,
			key:     "y",
			newName: "a.b",

			exp: `"a.b"
"a.b 2"
`,
		},
		{
			name: "conflict_with_numbers",

			text: `1
Square
`,
			key:     `Square`,
			newName: `1`,

			exp: `1
1 2
`,
		},
		{
			name: "nested",

			text: `x.y.z.q.nerve-gift-earther
x.y.z.q: {
  nerve-gift-earther
}
`,
			key:     `x.y.z.q.nerve-gift-earther`,
			newName: `nerve-gift-jingler`,

			exp: `x.y.z.q.nerve-gift-jingler
x.y.z.q: {
  nerve-gift-jingler
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected five objects: %#v", g.Objects)
				}
				if g.Objects[4].AbsID() != "x.y.z.q.nerve-gift-jingler" {
					t.Fatalf("unexpected object absolute id: %q", g.Objects[4].AbsID())
				}
			},
		},
		{
			name: "edges",

			text: `q.z -> p.k -> q.z -> l.a -> q.z
q: {
  q -> + -> z
  z: label
}
`,
			key:     `q.z`,
			newName: `%%%`,

			exp: `q.%%% -> p.k -> q.%%% -> l.a -> q.%%%
q: {
  q -> + -> %%%
  %%%: label
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 8 {
					t.Fatalf("expected eight objects: %#v", g.Objects)
				}
				if g.Objects[1].AbsID() != "q.%%%" {
					t.Fatalf("unexpected object absolute ID: %q", g.Objects[1].AbsID())
				}
			},
		},
		{
			name: "container",

			text: `ok.q.z -> p.k -> ok.q.z -> l.a -> ok.q.z
ok.q: {
  q -> + -> z
  z: label
}
ok: {
  q: {
    i
  }
}
(ok.q.z -> p.k)[0]: "furbling, v.:"
more.(ok.q.z -> p.k): "furbling, v.:"
`,
			key:     `ok.q`,
			newName: `<gosling>`,

			exp: `ok."<gosling>".z -> p.k -> ok."<gosling>".z -> l.a -> ok."<gosling>".z
ok."<gosling>": {
  q -> + -> z
  z: label
}
ok: {
  "<gosling>": {
    i
  }
}
(ok."<gosling>".z -> p.k)[0]: "furbling, v.:"
more.(ok.q.z -> p.k): "furbling, v.:"
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 16 {
					t.Fatalf("expected 16 objects: %#v", g.Objects)
				}
				if g.Objects[2].AbsID() != `ok."<gosling>".z` {
					t.Fatalf("unexpected object absolute ID: %q", g.Objects[1].AbsID())
				}
			},
		},
		{
			name: "complex_edge_1",

			text: `a.b.(x -> y).style.animated
`,
			key:     "a.b",
			newName: "ooo",

			exp: `a.ooo.(x -> y).style.animated
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		{
			name: "complex_edge_2",

			text: `a.b.(x -> y).style.animated
`,
			key:     "a.b.x",
			newName: "papa",

			exp: `a.b.(papa -> y).style.animated
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		/* TODO: handle edge keys
				{
					name: "complex_edge_3",

					text: `a.b.(x -> y).q.z
		`,
					key:     "a.b.(x -> y)[0].q",
					newName: "zoink",

					exp: `a.b.(x -> y).zoink.z
		`,
					assertions: func(t *testing.T, g *d2graph.Graph) {
						if len(g.Objects) != 4 {
							t.Fatalf("expected 4 objects: %#v", g.Objects)
						}
						if len(g.Edges) != 1 {
							t.Fatalf("expected 1 edge: %#v", g.Edges)
						}
					},
				},
		*/
		{
			name: "arrows",

			text: `x -> y
`,
			key:     "(x -> y)[0]",
			newName: "(x <- y)[0]",

			exp: `x <- y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if !g.Edges[0].SrcArrow || g.Edges[0].DstArrow {
					t.Fatalf("expected src arrow and no dst arrow: %#v", g.Edges[0])
				}
			},
		},
		{
			name: "arrows_complex",

			text: `a.b.(x -- y).style.animated
`,
			key:     "a.b.(x -- y)[0]",
			newName: "(x <-> y)[0]",

			exp: `a.b.(x <-> y).style.animated
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if !g.Edges[0].SrcArrow || !g.Edges[0].DstArrow {
					t.Fatalf("expected src arrow and dst arrow: %#v", g.Edges[0])
				}
			},
		},
		{
			name: "arrows_chain",

			text: `x -> y -> z -> q
`,
			key:     "(x -> y)[0]",
			newName: "(x <-> y)[0]",

			exp: `x <-> y -> z -> q
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 3 {
					t.Fatalf("expected 3 edges: %#v", g.Edges)
				}
				if !g.Edges[0].SrcArrow || !g.Edges[0].DstArrow {
					t.Fatalf("expected src arrow and dst arrow: %#v", g.Edges[0])
				}
			},
		},
		{
			name: "arrows_trim_common",

			text: `x.(x -> y -> z -> q)
`,
			key:     "(x.x -> x.y)[0]",
			newName: "(x.x <-> x.y)[0]",

			exp: `x.(x <-> y -> z -> q)
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 3 {
					t.Fatalf("expected 3 edges: %#v", g.Edges)
				}
				if !g.Edges[0].SrcArrow || !g.Edges[0].DstArrow {
					t.Fatalf("expected src arrow and dst arrow: %#v", g.Edges[0])
				}
			},
		},
		{
			name: "arrows_trim_common_2",

			text: `x.x -> x.y -> x.z -> x.q)
`,
			key:     "(x.x -> x.y)[0]",
			newName: "(x.x <-> x.y)[0]",

			exp: `x.x <-> x.y -> x.z -> x.q)
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 3 {
					t.Fatalf("expected 3 edges: %#v", g.Edges)
				}
				if !g.Edges[0].SrcArrow || !g.Edges[0].DstArrow {
					t.Fatalf("expected src arrow and dst arrow: %#v", g.Edges[0])
				}
			},
		},

		{
			name: "errors/empty_key",

			text: ``,
			key:  "",

			expErr: `failed to rename "" to "": empty map key: ""`,
		},
		{
			name: "errors/nonexistent",

			text:    ``,
			key:     "1.2.3.4",
			newName: "bic",

			expErr: `failed to rename "1.2.3.4" to "bic": key does not exist`,
		},

		{
			name: "errors/reserved_keys",

			text: `x.icon: hello
`,
			key:     "x.icon",
			newName: "near",
			expErr:  `failed to rename "x.icon" to "near": cannot rename to reserved keyword: "near"`,
		},
		{
			name: "layers-basic",

			text: `x

layers: {
  y: {
    a
  }
}
`,
			boardPath: []string{"y"},
			key:       "a",
			newName:   "b",

			exp: `x

layers: {
  y: {
    b
  }
}
`,
		},
		{
			name: "scenarios-basic",

			text: `x

scenarios: {
  y: {
    a
  }
}
`,
			boardPath: []string{"y"},
			key:       "a",
			newName:   "b",

			exp: `x

scenarios: {
  y: {
    b
  }
}
`,
		},
		{
			name: "scenarios-conflict",

			text: `x

scenarios: {
  y: {
    a
  }
}
`,
			boardPath: []string{"y"},
			key:       "a",
			newName:   "x",

			exp: `x

scenarios: {
  y: {
    x 2
  }
}
`,
		},
		{
			name: "scenarios-scope-err",

			text: `x

scenarios: {
  y: {
    a
  }
}
`,
			boardPath: []string{"y"},
			key:       "x",
			newName:   "b",

			expErr: `failed to rename "x" to "b": operation would modify AST outside of given scope`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			et := editTest{
				text:    tc.text,
				fsTexts: tc.fsTexts,
				testFunc: func(g *d2graph.Graph) (*d2graph.Graph, error) {
					objectsBefore := len(g.Objects)
					var err error
					g, _, err = d2oracle.Rename(g, tc.boardPath, tc.key, tc.newName)
					if err == nil {
						objectsAfter := len(g.Objects)
						if objectsBefore != objectsAfter {
							t.Log(d2format.Format(g.AST))
							return nil, fmt.Errorf("rename cannot destroy or create objects: found %d objects before and %d objects after", objectsBefore, objectsAfter)
						}
					}

					return g, err
				},

				exp:        tc.exp,
				expErr:     tc.expErr,
				assertions: tc.assertions,
			}
			et.run(t)
		})
	}
}

func TestMove(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		skip      bool
		name      string
		boardPath []string

		text               string
		fsTexts            map[string]string
		key                string
		newKey             string
		includeDescendants bool

		expErr     string
		exp        string
		assertions func(t *testing.T, g *d2graph.Graph)
	}{
		{
			name: "basic",

			text: `a
`,
			key:    `a`,
			newKey: `b`,

			exp: `b
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 1)
				assert.JSON(t, g.Objects[0].ID, "b")
			},
		},
		{
			name: "basic_nested",

			text: `a: {
  b
}
`,
			key:    `a.b`,
			newKey: `a.c`,

			exp: `a: {
  c
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 2)
				assert.JSON(t, g.Objects[1].ID, "c")
			},
		},
		{
			name: "duplicate",

			text: `a: {
  b: {
    shape: cylinder
  }
}

a: {
  b: {
    shape: cylinder
  }
}
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a

a
b: {
  shape: cylinder
}
`,
		},
		{
			name: "duplicate_generated",

			text: `x
x 2
x 3: {
  x 3
  x 4
}
x 4
y
`,
			key:    `x 3`,
			newKey: `y.x 3`,

			exp: `x
x 2

x 3
x 5

x 4
y: {
  x 3
}
`,
		},
		{
			name: "rename_2",

			text: `a: {
  b 2
  y 2
}
b 2
x
`,
			key:    `a`,
			newKey: `x.a`,

			exp: `b
y 2

b 2
x: {
  a
}
`,
		},
		{
			name: "parentheses",

			text: `x -> y (z)
z: ""
`,
			key:    `"y (z)"`,
			newKey: `z.y (z)`,

			exp: `x -> z.y (z)
z: ""
`,
		},
		{
			name: "middle_container_generated_conflict",

			text: `a.Square.Text 3 -> a.Square.Text 2

a.Square -> a.Text

a: {
  Text
  Square: {
    Text 2
    Text 3
  }
  Square

  Text 2
}
`,
			key:    `a.Square`,
			newKey: `Square`,

			exp: `a.Text 3 -> a.Text 4

Square -> a.Text

a: {
  Text

  Text 4
  Text 3

  Text 2
}
Square
`,
		},
		{
			name: "into_container_existing_map",

			text: `a: {
  b
}
c
`,
			key:    `c`,
			newKey: `a.c`,

			exp: `a: {
  b
  c
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 3)
				assert.JSON(t, "a", g.Objects[0].ID)
				assert.JSON(t, 2, len(g.Objects[0].Children))
			},
		},
		{
			name: "into_container_with_flat_keys",

			text: `a
c: {
  style.opacity: 0.4
  style.fill: "#FFFFFF"
  style.stroke: "#FFFFFF"
}
`,
			key:    `c`,
			newKey: `a.c`,

			exp: `a: {
  c: {
    style.opacity: 0.4
    style.fill: "#FFFFFF"
    style.stroke: "#FFFFFF"
  }
}
`,
		},
		{
			name: "into_container_nonexisting_map",

			text: `a
c
`,
			key:    `c`,
			newKey: `a.c`,

			exp: `a: {
  c
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 2)
				assert.JSON(t, "a", g.Objects[0].ID)
				assert.JSON(t, 1, len(g.Objects[0].Children))
			},
		},
		{
			name: "basic_out_of_container",

			text: `a: {
  b
}
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a
b
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 2)
				assert.JSON(t, "a", g.Objects[0].ID)
				assert.JSON(t, 0, len(g.Objects[0].Children))
			},
		},
		{
			name: "out_of_newline_container",

			text: `"a\n": {
  b
}
`,
			key:    `"a\n".b`,
			newKey: `b`,

			exp: `"a\n"
b
`,
		},
		{
			name: "partial_slice",

			text: `a: {
  b
}
a.b
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a
b
`,
		},
		{
			name: "partial_edge_slice",

			text: `a: {
  b
}
a.b -> c
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a
b -> c
b
`,
		},
		{
			name: "full_edge_slice",

			text: `a: {
	b: {
    c
  }
  b.c -> d
}
a.b.c -> a.d
`,
			key:    `a.b.c`,
			newKey: `c`,

			exp: `a: {
  b
  _.c -> d
}
c -> a.d
c
`,
		},
		{
			name: "full_slice",

			text: `a: {
	b: {
    c
  }
  b.c
}
a.b.c
`,
			key:    `a.b.c`,
			newKey: `c`,

			exp: `a: {
  b
}
c
`,
		},
		{
			name: "slice_style",

			text: `a: {
  b
}
a.b.icon: https://icons.terrastruct.com/essentials/142-target.svg
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a
a
b
b.icon: https://icons.terrastruct.com/essentials/142-target.svg
`,
		},
		{
			name: "between_containers",

			text: `a: {
  b
}
c
`,
			key:    `a.b`,
			newKey: `c.b`,

			exp: `a
c: {
  b
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 3)
				assert.JSON(t, "a", g.Objects[0].ID)
				assert.JSON(t, 0, len(g.Objects[0].Children))
				assert.JSON(t, "c", g.Objects[1].ID)
				assert.JSON(t, 1, len(g.Objects[1].Children))
			},
		},
		{
			name: "hoist_container_children",

			text: `a: {
  b
  c
}
d
`,
			key:    `a`,
			newKey: `d.a`,

			exp: `b
c

d: {
  a
}
`,
		},
		{
			name: "middle_container",

			text: `x: {
  y: {
    z
  }
}
`,
			key:    `x.y`,
			newKey: `y`,

			exp: `x: {
  z
}
y
`,
		},
		{
			// a.b does not move from its scope, just extends path
			name: "extend_stationary_path",

			text: `a.b
a: {
	b
	c
}
`,
			key:    `a.b`,
			newKey: `a.c.b`,

			exp: `a.c.b
a: {
  c: {
    b
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 3)
			},
		},
		{
			name: "extend_map",

			text: `a.b: {
  e
}
a: {
	b
	c
}
`,
			key:    `a.b`,
			newKey: `a.c.b`,

			exp: `a: {
  e
}
a: {
  c: {
    b
  }
}
`,
		},
		{
			name: "into_container_with_flat_style",

			text: `x.style.border-radius: 5
y
`,
			key:    `y`,
			newKey: `x.y`,

			exp: `x: {
  style.border-radius: 5
  y
}
`,
		},
		{
			name: "flat_between_containers",

			text: `a.b
c
`,
			key:    `a.b`,
			newKey: `c.b`,

			exp: `a
c: {
  b
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 3)
			},
		},
		{
			name: "underscore-connection",

			text: `a: {
  b

  _.c.d -> b
}

c: {
  d
}
`,
			key:    `a.b`,
			newKey: `c.b`,

			exp: `a: {
  _.c.d -> _.c.b
}

c: {
  d
  b
}
`,
		},

		{
			name: "nested-underscore-move-out",
			text: `guitar: {
	books: {
		_._.pipe
  }
}
`,
			key:    `pipe`,
			newKey: `guitar.pipe`,

			exp: `guitar: {
  books
  pipe
}
`,
		},
		{
			name: "flat_middle_container",

			text: `a.b.c
d
`,
			key:    `a.b`,
			newKey: `d.b`,

			exp: `a.c
d: {
  b
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 4)
			},
		},
		{
			name: "flat_merge",

			text: `a.b
c.d: meow
`,
			key:    `a.b`,
			newKey: `c.b`,

			exp: `a
c: {
  d: meow
  b
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.JSON(t, len(g.Objects), 4)
			},
		},
		{
			name: "flat_reparent_with_value",
			text: `a.b: "yo"
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a
b: "yo"
`,
		},
		{
			name: "flat_reparent_with_map_value",
			text: `a.b: {
  shape: hexagon
}
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a
b: {
  shape: hexagon
}
`,
		},
		{
			name: "flat_reparent_with_mixed_map_value",
			text: `a.b: {
  # this is reserved
  shape: hexagon
  # this is not
  c
}
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a: {
  # this is not
  c
}
b: {
  # this is reserved
  shape: hexagon
}
`,
		},
		{
			name: "flat_style",

			text: `a.style.opacity: 0.4
a.style.fill: black
b
`,
			key:    `a`,
			newKey: `b.a`,

			exp: `b: {
  a.style.opacity: 0.4
  a.style.fill: black
}
`,
		},
		{
			name: "flat_nested_merge",

			text: `a.b.c.d.e
p.q.b.m.o
`,
			key:    `a.b.c`,
			newKey: `p.q.z`,

			exp: `a.b.d.e
p.q: {
  b.m.o
  z
}
`,
		},
		{
			// We open up only the most nested
			name: "flat_nested_merge_multiple_refs",

			text: `a: {
  b.c.d
  e.f
  e.g
}
a.b.c
a.b.c.q
`,
			key:    `a.e`,
			newKey: `a.b.c.e`,

			exp: `a: {
  b.c: {
    d
    e
  }
  f
  g
}
a.b.c
a.b.c.q
`,
		},
		{
			// TODO
			skip: true,
			// Choose to move to a reference that is less nested but has an existing map
			name: "less_nested_map",

			text: `a: {
  b: {
    c
  }
}
a.b.c: {
  d
}
e
`,
			key:    `e`,
			newKey: `a.b.c.e`,

			exp: `a: {
  b: {
    c
  }
}
a.b.c: {
  d
  e
}
`,
		},
		{
			name: "invalid-near",

			text: `x: {
  near: y
}
y
`,
			key:    `y`,
			newKey: `x.y`,

			exp: `x: {
  near: y
  y
}
`,
			expErr: `failed to move: "y" to "x.y": failed to recompile:
x: {
  near: x.y
  y
}

d2/testdata/d2oracle/TestMove/invalid-near.d2:2:9: near keys cannot be set to an descendant`,
		},
		{
			name: "near",

			text: `x: {
  near: y
}
a
y
`,
			key:    `y`,
			newKey: `a.y`,

			exp: `x: {
  near: a.y
}
a: {
  y
}
`,
		},
		{
			name: "flat_near",

			text: `x.near: y
a
y
`,
			key:    `y`,
			newKey: `a.y`,

			exp: `x.near: a.y
a: {
  y
}
`,
		},
		{
			name: "container_near",

			text: `x: {
  y: {
    near: x.a.b.z
  }
  a.b.z
}
y
`,
			key:    `x.a.b`,
			newKey: `y.a`,

			exp: `x: {
  y: {
    near: x.a.z
  }
  a.z
}
y: {
  a
}
`,
		},
		{
			name: "nhooyr_one",

			text: `a: {
  b.c
}
d
`,
			key:    `a.b`,
			newKey: `d.q`,

			exp: `a: {
  c
}
d: {
  q
}
`,
		},
		{
			name: "nhooyr_two",

			text: `a: {
  b.c -> meow
}
d: {
  x
}
`,
			key:    `a.b`,
			newKey: `d.b`,

			exp: `a: {
  c -> meow
}
d: {
  x
  b
}
`,
		},
		{
			name: "unique_name",

			text: `a: {
  b
}
a.b
c: {
  b
}
`,
			key:    `c.b`,
			newKey: `a.b`,

			exp: `a: {
  b
  b 2
}
a.b
c
`,
		},
		{
			name: "unique_name_with_references",

			text: `a: {
  b
}
d -> c.b
c: {
  b
}
`,
			key:    `c.b`,
			newKey: `a.b`,

			exp: `a: {
  b
  b 2
}
d -> a.b 2
c
`,
		},
		{
			name: "map_transplant",

			text: `a: {
  b
  style: {
    opacity: 0.4
  }
  c
  label: "yo"
}
d
`,
			key:    `a`,
			newKey: `d.a`,

			exp: `b

c

d: {
  a: {
    style: {
      opacity: 0.4
    }

    label: "yo"
  }
}
`,
		},
		{
			name: "map_with_label",

			text: `a: "yo" {
  c
}
d
`,
			key:    `a`,
			newKey: `d.a`,

			exp: `c

d: {
  a: "yo"
}
`,
		},
		{
			name: "underscore_merge",

			text: `a: {
	_.b: "yo"
}
b: "what"
c
`,
			key:    `b`,
			newKey: `c.b`,

			exp: `a

c: {
  b: "yo"
  b: "what"
}
`,
		},
		{
			name: "underscore_children",

			text: `a: {
  _.b
}
b
`,
			key:    `b`,
			newKey: `c`,

			exp: `a: {
  _.c
}
c
`,
		},
		{
			name: "underscore_transplant",

			text: `a: {
  b: {
    _.c
  }
}
`,
			key:    `a.c`,
			newKey: `c`,

			exp: `a: {
  b
}
c
`,
		},
		{
			name: "underscore_split",

			text: `a: {
  b: {
    _.c.f
  }
}
`,
			key:    `a.c`,
			newKey: `c`,

			exp: `a: {
  b: {
    _.f
  }
}
c
`,
		},
		{
			name: "underscore_edge_container_1",

			text: `a: {
  _.b -> c
}
`,
			key:    `b`,
			newKey: `a.b`,

			exp: `a: {
  b -> c
}
`,
		},
		{
			name: "underscore_edge_container_2",

			text: `a: {
  _.b -> c
}
`,
			key:    `b`,
			newKey: `a.c.b`,

			exp: `a: {
  c.b -> c
}
`,
		},
		{
			name: "underscore_edge_container_3",

			text: `a: {
  _.b -> c
}
`,
			key:    `b`,
			newKey: `d`,

			exp: `a: {
  _.d -> c
}
`,
		},
		{
			name: "underscore_edge_container_4",

			text: `a: {
  _.b -> c
}
`,
			key:    `b`,
			newKey: `a.f`,

			exp: `a: {
  f -> c
}
`,
		},
		{
			name: "underscore_edge_container_5",

			text: `a: {
  _.b -> _.c
}
`,
			key:    `b`,
			newKey: `c.b`,

			exp: `a: {
  _.c.b -> _.c
}
`,
		},
		{
			name: "underscore_edge_container_6",

			text: `x: {
  _.y.a -> _.y.b
}
`,
			key:                `y`,
			newKey:             `x.y`,
			includeDescendants: true,

			exp: `x: {
  y.a -> y.b
}
`,
		},
		{
			name: "underscore_edge_container_7",

			text: `x: {
  _.y.a -> _.y.b
}
`,
			key:                `x`,
			newKey:             `y.x`,
			includeDescendants: false,

			exp: `x: {
  y.a -> y.b
}
`,
		},
		{
			name: "underscore_edge_split",

			text: `a: {
  b: {
    _.c.f -> yo
  }
}
`,
			key:    `a.c`,
			newKey: `c`,

			exp: `a: {
  b: {
    _.f -> yo
  }
}
c
`,
		},
		{
			name: "underscore_split_out",

			text: `a: {
  b: {
    _.c.f
  }
  c: {
    e
  }
}
`,
			key:    `a.c.f`,
			newKey: `a.c.e.f`,

			exp: `a: {
  b: {
    _.c
  }
  c: {
    e: {
      f
    }
  }
}
`,
		},
		{
			name: "underscore_edge_children",

			text: `a: {
  _.b -> c
}
b
`,
			key:    `b`,
			newKey: `c`,

			exp: `a: {
  _.c -> c
}
c
`,
		},
		{
			name: "move_container_children",

			text: `b: {
  p
  q
}
a
d
`,
			key:    `b`,
			newKey: `d.b`,

			exp: `p
q

a
d: {
  b
}
`,
		},
		{
			name: "move_container_conflict_children",

			text: `x: {
  a
  b
}
a
d
`,
			key:    `x`,
			newKey: `d.x`,

			exp: `a 2
b

a
d: {
  x
}
`,
		},
		{
			name: "edge_conflict",

			text: `x.y.a -> x.y.b
y
`,
			key:    `x`,
			newKey: `y.x`,

			exp: `y 2.a -> y 2.b
y: {
  x
}
`,
		},
		{
			name: "edge_basic",

			text: `a -> b
`,
			key:    `a`,
			newKey: `c`,

			exp: `c -> b
`,
		},
		{
			name: "edge_nested_basic",

			text: `a: {
  b -> c
}
`,
			key:    `a.b`,
			newKey: `a.d`,

			exp: `a: {
  d -> c
}
`,
		},
		{
			name: "edge_into_container",

			text: `a: {
  d
}
b -> c
`,
			key:    `b`,
			newKey: `a.b`,

			exp: `a: {
  d
}
a.b -> c
`,
		},
		{
			name: "edge_out_of_container",

			text: `a: {
  b -> c
}
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a: {
  _.b -> c
}
`,
		},
		{
			name: "connected_nested",

			text: `x -> y.z
`,
			key:    `y.z`,
			newKey: `z`,

			exp: `x -> z
y
`,
		},
		{
			name: "chain_connected_nested",

			text: `y.z -> x -> y.z
`,
			key:    `y.z`,
			newKey: `z`,

			exp: `z -> x -> z
y
`,
		},
		{
			name: "chain_connected_nested_no_extra_create",

			text: `y.b -> x -> y.z
`,
			key:    `y.z`,
			newKey: `z`,

			exp: `y.b -> x -> z
`,
		},
		{
			name: "edge_across_containers",

			text: `a: {
  b -> c
}
d
`,
			key:    `a.b`,
			newKey: `d.b`,

			exp: `a: {
  _.d.b -> c
}
d
`,
		},
		{
			name: "move_out_of_edge",

			text: `a.b.c -> d.e.f
`,
			key:    `a.b`,
			newKey: `q`,

			exp: `a.c -> d.e.f
q
`,
		},
		{
			name: "move_out_of_nested_edge",

			text: `a.b.c -> d.e.f
`,
			key:    `a.b`,
			newKey: `d.e.q`,

			exp: `a.c -> d.e.f
d.e: {
  q
}
`,
		},
		{
			name: "append_multiple_styles",

			text: `a: {
  style: {
    opacity: 0.4
  }
}
a: {
  style: {
    fill: "red"
  }
}
d
`,
			key:    `a`,
			newKey: `d.a`,

			exp: `d: {
  a: {
    style: {
      opacity: 0.4
    }
  }
  a: {
    style: {
      fill: "red"
    }
  }
}
`,
		},
		{
			name: "move_into_key_with_value",

			text: `a: meow
b
`,
			key:    `b`,
			newKey: `a.b`,

			exp: `a: meow {
  b
}
`,
		},
		{
			name: "gnarly_1",

			text: `a.b.c -> d.e.f
b: meow {
	p: "eyy"
  q
  p.p -> q.q
}
b.p.x -> d
`,
			key:    `b`,
			newKey: `d.b`,

			exp: `a.b.c -> d.e.f
d: {
  b: meow
}
p: "eyy"
q
p.p -> q.q

p.x -> d
`,
		},
		{
			name: "reuse_map",

			text: `a: {
  b: {
    hey
  }
  b.yo
}
k
`,
			key:    `k`,
			newKey: `a.b.k`,

			exp: `a: {
  b: {
    hey
    k
  }
  b.yo
}
`,
		},
		{
			// TODO the heuristic for splitting open new maps should be only if the key has no existing maps and it also has either zero or one children. if it has two children or more then we should not be opening a map and just append the key at the most nested map.
			//       first loop over explicit references from first to last.
			//
			// explicit ref means its the leaf disregarding reserved fields.
			// implicit ref means there is a shape declared after the target element.
			//
			// then loop over the implicit references and only if there is no explicit ref do you need to add the implicit ref to the scope but only if appended == false (which would be set when looping through explicit refs).
			skip: true,
			name: "merge_nested_flat",

			text: `a: {
  b.c
  b.d
  b.e.g
}
k
`,
			key:    `k`,
			newKey: `a.b.k`,

			exp: `a: {
  b.c
  b.d
  b.e.g
  b.k
}
`,
		},
		{
			name: "merge_nested_maps",

			text: `a: {
  b.c
  b.d
  b.e.g
  b.d: {
    o
  }
}
k
`,
			key:    `k`,
			newKey: `a.b.k`,

			exp: `a: {
  b.c
  b.d
  b.e.g
  b: {
    d: {
      o
    }
    k
  }
}
`,
		},
		{
			name: "merge_reserved",

			text: `a: {
  b.c
	b.label: "yo"
	b.label: "hi"
  b.e.g
}
k
`,
			key:    `k`,
			newKey: `a.b.k`,

			exp: `a: {
  b.c
  b.label: "yo"
  b.label: "hi"
  b: {
    e.g
    k
  }
}
`,
		},
		{
			name: "multiple_nesting_levels",

			text: `a: {
	b: {
    c
    c.g
  }
  b.c.d
  x
}
a.b.c.f
`,
			key:    `a.x`,
			newKey: `a.b.c.x`,

			exp: `a: {
  b: {
    c
    c: {
      g
      x
    }
  }
  b.c.d
}
a.b.c.f
`,
		},
		{
			name: "edge_chain_basic",

			text: `a -> b -> c
`,
			key:    `a`,
			newKey: `d`,

			exp: `d -> b -> c
`,
		},
		{
			name: "edge_chain_into_container",

			text: `a -> b -> c
d
`,
			key:    `a`,
			newKey: `d.a`,

			exp: `d.a -> b -> c
d
`,
		},
		{
			name: "edge_chain_out_container",

			text: `a: {
  b -> c -> d
}
`,
			key:    `a.c`,
			newKey: `c`,

			exp: `a: {
  b -> _.c -> d
}
`,
		},
		{
			name: "edge_chain_circular",

			text: `a: {
  b -> c -> b
}
`,
			key:    `a.b`,
			newKey: `b`,

			exp: `a: {
  _.b -> c -> _.b
}
`,
		},
		{
			name: "container_multiple_refs_with_underscore",

			text: `a
b: {
  _.a
}
`,
			key:    `a`,
			newKey: `b.a`,

			exp: `b: {
  a
}
`,
		},
		{
			name: "container_conflicts_generated",
			text: `Square 2: "" {
  Square: ""
}
Square: ""
Square 3
`,
			key:    `Square 2`,
			newKey: `Square 3.Square 2`,

			exp: `Square 2: ""

Square: ""
Square 3: {
  Square 2: ""
}
`,
		},
		{
			name: "include_descendants_flat_1",
			text: `x.y
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x.y
}
`,
		},
		{
			name: "include_descendants_flat_2",
			text: `a.x.y
a.z
`,
			key:                `a.x`,
			newKey:             `a.z.x`,
			includeDescendants: true,

			exp: `a
a.z: {
  x.y
}
`,
		},
		{
			name: "include_descendants_flat_3",
			text: `a.x.y
a.z
`,
			key:                `a.x`,
			newKey:             `x`,
			includeDescendants: true,

			exp: `a
a.z
x.y
`,
		},
		{
			name: "include_descendants_flat_4",
			text: `a.x.y
a.z
`,
			key:                `a.x.y`,
			newKey:             `y`,
			includeDescendants: true,

			exp: `a.x
a.z
y
`,
		},
		{
			name: "include_descendants_map_1",
			text: `x: {
  y
}
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x: {
    y
  }
}
`,
		},
		{
			name: "include_descendants_map_2",
			text: `x: {
	y: {
    c
  }
  y.b
}
x.y.b
z
`,
			key:                `x.y`,
			newKey:             `a`,
			includeDescendants: true,

			exp: `x
x
z
a: {
  c
}
a.b
`,
		},
		{
			name: "include_descendants_grandchild",
			text: `x: {
  y.a
  y: {
    b
  }
}
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x: {
    y.a
    y: {
      b
    }
  }
}
`,
		},
		{
			name: "include_descendants_sql",
			text: `x: {
  shape: sql_table
	a: b
}
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x: {
    shape: sql_table
    a: b
  }
}
`,
		},
		{
			name: "include_descendants_edge_child",
			text: `x: {
  a -> b
}
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x: {
    a -> b
  }
}
`,
		},
		{
			name: "include_descendants_edge_ref_1",
			text: `x
z
x.a -> x.b
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x
}
z.x.a -> z.x.b
`,
		},
		{
			name: "include_descendants_edge_ref_2",
			text: `x -> y.z
`,
			key:                `y.z`,
			newKey:             `z`,
			includeDescendants: true,

			exp: `x -> z
y
`,
		},
		{
			name: "include_descendants_edge_ref_3",
			text: `x -> y.z.a
`,
			key:                `y.z`,
			newKey:             `z`,
			includeDescendants: true,

			exp: `x -> z.a
y
`,
		},
		{
			name: "include_descendants_edge_ref_4",
			text: `x -> y.z.a
b
`,
			key:                `y.z`,
			newKey:             `b.z`,
			includeDescendants: true,

			exp: `x -> b.z.a
b
y
`,
		},
		{
			name: "include_descendants_edge_ref_5",
			text: `foo: {
  x -> y.z.a
  b
}
`,
			key:                `foo.y.z`,
			newKey:             `foo.b.z`,
			includeDescendants: true,

			exp: `foo: {
  x -> b.z.a
  b
  y
}
`,
		},
		{
			name: "include_descendants_edge_ref_6",
			text: `x -> y
z
`,
			key:                `y`,
			newKey:             `z.y`,
			includeDescendants: true,

			exp: `x -> z.y
z
`,
		},
		{
			name: "include_descendants_edge_ref_7",
			text: `d.t -> d.np.s
`,
			key:                `d.np.s`,
			newKey:             `d.s`,
			includeDescendants: true,

			exp: `d.t -> d.s
d.np
`,
		},
		{
			name: "include_descendants_nested_1",
			text: `y.z
b
`,
			key:                `y.z`,
			newKey:             `b.z`,
			includeDescendants: true,

			exp: `y
b: {
  z
}
`,
		},
		{
			name: "include_descendants_nested_2",
			text: `y.z
y.b
`,
			key:                `y.z`,
			newKey:             `y.b.z`,
			includeDescendants: true,

			exp: `y
y.b: {
  z
}
`,
		},
		{
			name: "include_descendants_underscore",
			text: `github.code -> local.dev

github: {
  _.local.dev -> _.aws.workflows
  _.aws: {
    workflows
  }
}
`,
			key:                `aws.workflows`,
			newKey:             `github.workflows`,
			includeDescendants: true,

			exp: `github.code -> local.dev

github: {
  _.local.dev -> workflows
  _.aws
  workflows
}
`,
		},
		{
			name: "include_descendants_underscore_2",
			text: `a: {
  b: {
    _.c
  }
}
`,
			key:                `a.b`,
			newKey:             `b`,
			includeDescendants: true,

			exp: `a
b: {
  _.a.c
}
`,
		},
		{
			name: "include_descendants_underscore_3",
			text: `a: {
  b: {
    _.c -> d
		_.c -> _.d
  }
}
`,
			key:                `a.b`,
			newKey:             `b`,
			includeDescendants: true,

			exp: `a
b: {
  _.a.c -> d
  _.a.c -> _.a.d
}
`,
		},
		{
			name: "include_descendants_edge_ref_underscore",
			text: `x
z
x.a -> x.b
b: {
  _.x.a -> _.x.b
}
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x
}
z.x.a -> z.x.b
b: {
  _.z.x.a -> _.z.x.b
}
`,
		},
		{
			name: "include_descendants_near",
			text: `x.y
z
a.near: x.y
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x.y
}
a.near: z.x.y
`,
		},
		{
			name: "include_descendants_conflict",
			text: `x.y
z.x
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x
  x 2.y
}
`,
		},
		{
			name: "include_descendants_non_conflict",
			text: `x.y
z.x
y
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `z: {
  x
  x 2.y
}
y
`,
		},
		{
			name: "nested_reserved_2",
			text: `A.B.C.shape: circle
`,
			key:    `A.B.C`,
			newKey: `C`,

			exp: `A.B
C.shape: circle
`,
		},
		{
			name: "nested_reserved_3",
			text: `A.B.C.shape: circle
A.B: {
  C
  D
}
`,
			key:    `A.B.C`,
			newKey: `A.B.D.C`,

			exp: `A.B
A.B: {
  D: {
    C.shape: circle
    C
  }
}
`,
		},
		{
			name: "include_descendants_nested_reserved_2",
			text: `A.B.C.shape: circle
`,
			key:                `A.B.C`,
			newKey:             `C`,
			includeDescendants: true,

			exp: `A.B
C.shape: circle
`,
		},
		{
			name: "include_descendants_nested_reserved_3",
			text: `A.B.C.shape: circle
`,
			key:                `A.B`,
			newKey:             `C`,
			includeDescendants: true,

			exp: `A
C.C.shape: circle
`,
		},
		{
			name: "include_descendants_move_out",
			text: `a.b: {
  c: {
    d
  }
}
`,
			key:                `a.b`,
			newKey:             `b`,
			includeDescendants: true,

			exp: `a
b: {
  c: {
    d
  }
}
`,
		},
		{
			name: "include_descendants_underscore_regression",
			text: `x: {
  _.a
}
a
`,
			key:                `a`,
			newKey:             `x.a`,
			includeDescendants: true,

			exp: `x: {
  a
}
`,
		},
		{
			name: "include_descendants_underscore_regression_2",
			text: `x: {
  _.a.b
}
`,
			key:                `a`,
			newKey:             `x.a`,
			includeDescendants: true,

			exp: `x: {
  a.b
}
`,
		},
		{
			name: "layers-basic",

			text: `a

layers: {
  x: {
    b
    c
  }
}
`,
			key:       `c`,
			newKey:    `b.c`,
			boardPath: []string{"x"},

			exp: `a

layers: {
  x: {
    b: {
      c
    }
  }
}
`,
		},
		{
			name: "scenarios-out-of-scope",

			text: `a

scenarios: {
  x: {
    b
    c
  }
}
`,
			key:       `a`,
			newKey:    `b.a`,
			boardPath: []string{"x"},

			expErr: `failed to move: "a" to "b.a": operation would modify AST outside of given scope`,
		},
	}

	for _, tc := range testCases {
		if tc.skip {
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			et := editTest{
				text:    tc.text,
				fsTexts: tc.fsTexts,
				testFunc: func(g *d2graph.Graph) (*d2graph.Graph, error) {
					objectsBefore := len(g.Objects)
					var err error
					g, err = d2oracle.Move(g, tc.boardPath, tc.key, tc.newKey, tc.includeDescendants)
					if err == nil {
						objectsAfter := len(g.Objects)
						if objectsBefore != objectsAfter {
							t.Log(d2format.Format(g.AST))
							return nil, fmt.Errorf("move cannot destroy or create objects: found %d objects before and %d objects after", objectsBefore, objectsAfter)
						}
					}
					return g, err
				},

				exp:        tc.exp,
				expErr:     tc.expErr,
				assertions: tc.assertions,
			}
			et.run(t)
		})
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		boardPath []string

		text    string
		fsTexts map[string]string
		key     string

		expErr     string
		exp        string
		assertions func(t *testing.T, g *d2graph.Graph)
	}{
		{
			name: "flat",

			text: `nerve-gift-earther
`,
			key: `nerve-gift-earther`,

			exp: ``,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 0 {
					t.Fatalf("expected zero objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "edge_identical_child",

			text: `x.x.y.z -> x.y.b
`,
			key: `x`,

			exp: `x.y.z -> y.b
`,
		},
		{
			name: "duplicate_generated",

			text: `x
x 2
x 3: {
  x 3
  x 4
}
x 4
y
`,
			key: `x 3`,
			exp: `x
x 2

x 3
x 5

x 4
y
`,
		},
		{
			name: "table_refs",

			text: `a: {
  shape: sql_table
  b
}
c: {
  shape: sql_table
  d
}

a.b
a.b -> c.d
`,
			key: `a`,

			exp: `c: {
  shape: sql_table
  d
}
c.d
`,
		},
		{
			name: "class_refs",

			text: `a: {
  shape: class
	b: int
}

a.b
`,
			key: `a`,

			exp: ``,
		},
		{
			name: "edge_both_identical_childs",

			text: `x.x.y.z -> x.x.b
`,
			key: `x`,

			exp: `x.y.z -> x.b
`,
		},
		{
			name: "edge_conflict",

			text: `x.y.a -> x.y.b
y
`,
			key: `x`,

			exp: `y 2.a -> y 2.b
y
`,
		},
		{
			name: "underscore_remove",

			text: `x: {
  _.y
  _.a -> _.b
  _.c -> d
}
`,
			key: `x`,

			exp: `y
a -> b
c -> d
`,
		},
		{
			name: "underscore_no_conflict",

			text: `x: {
	y: {
    _._.z
  }
  z
}
`,
			key: `x.y`,

			exp: `x: {
  _.z

  z
}
`,
		},
		{
			name: "nested_underscore_update",

			text: `guitar: {
	books: {
    _._.pipe
  }
}
`,
			key: `guitar`,

			exp: `books: {
  _.pipe
}
`,
		},
		{
			name: "only-underscore",

			text: `guitar: {
	books: {
    _._.pipe
  }
}
`,
			key: `pipe`,

			exp: `guitar: {
  books
}
`,
		},
		{
			name: "only-underscore-nested",

			text: `guitar: {
	books: {
		_._.pipe: {
      a
    }
  }
}
`,
			key: `pipe`,

			exp: `guitar: {
  books
}
a
`,
		},
		{
			name: "node_in_edge",

			text: `x -> y -> z -> q -> p
z.ok: {
  what's up
}
`,
			key: `z`,

			exp: `x -> y
q -> p
ok: {
  what's up
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 6 {
					t.Fatalf("expected 6 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 2 {
					t.Fatalf("expected two edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "node_in_edge_last",

			text: `x -> y -> z -> q -> a.b.p
a.b.p: {
  what's up
}
`,
			key: `a.b.p`,

			exp: `x -> y -> z -> q
a.b: {
  what's up
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 7 {
					t.Fatalf("expected 7 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 3 {
					t.Fatalf("expected three edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "children",

			text: `p: {
  what's up
  x -> y
}
`,
			key: `p`,

			exp: `what's up
x -> y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		{
			name: "hoist_children",

			text: `a: {
  b: {
    c
  }
}
`,
			key: `a.b`,

			exp: `a: {
  c
}
`,
		},
		{
			name: "hoist_edge_children",

			text: `a: {
  b
  c -> d
}
`,
			key: `a`,

			exp: `b
c -> d
`,
		},
		{
			name: "children_conflicts",

			text: `p: {
  x
}
x
`,
			key: `p`,

			exp: `x 2

x
`,
		},
		{
			name: "edge_map_style",

			text: `x -> y: { style.stroke: red }
`,
			key: `(x -> y)[0].style.stroke`,

			exp: `x -> y
`,
		},
		{
			// Just checks that removing an object removes the arrowhead field too
			name: "breakup_arrowhead",

			text: `x -> y: {
  target-arrowhead.shape: diamond
}
(x -> y)[0].source-arrowhead: {
  shape: diamond
}
`,
			key: `x`,

			exp: `y
`,
		},
		{
			name: "arrowhead",

			text: `x -> y: {
  target-arrowhead.shape: diamond
}
`,
			key: `(x -> y)[0].target-arrowhead`,

			exp: `x -> y
`,
		},
		{
			name: "arrowhead_shape",

			text: `x -> y: {
  target-arrowhead.shape: diamond
}
`,
			key: `(x -> y)[0].target-arrowhead.shape`,

			exp: `x -> y
`,
		},
		{
			name: "arrowhead_label",

			text: `x -> y: {
  target-arrowhead.shape: diamond
  target-arrowhead.label: 1
}
`,
			key: `(x -> y)[0].target-arrowhead.label`,

			exp: `x -> y: {
  target-arrowhead.shape: diamond
}
`,
		},
		{
			name: "arrowhead_map",

			text: `x -> y: {
	target-arrowhead: {
    shape: diamond
  }
}
`,
			key: `(x -> y)[0].target-arrowhead.shape`,

			exp: `x -> y
`,
		},
		{
			name: "edge-only-style",

			text: `x -> y: {
  style.stroke: red
}
`,
			key: `(x -> y)[0].style.stroke`,

			exp: `x -> y
`,
		},
		{
			name: "edge_key_style",

			text: `x -> y
(x -> y)[0].style.stroke: red
`,
			key: `(x -> y)[0].style.stroke`,

			exp: `x -> y
`,
		},
		{
			name: "nested_edge_key_style",

			text: `a: {
  x -> y
}
a.(x -> y)[0].style.stroke: red
`,
			key: `a.(x -> y)[0].style.stroke`,

			exp: `a: {
  x -> y
}
`,
		},
		{
			name: "multiple_flat_style",

			text: `x.style.opacity: 0.4
x.style.fill: red
`,
			key: `x.style.fill`,

			exp: `x.style.opacity: 0.4
`,
		},
		{
			name: "edge_flat_style",

			text: `A -> B
A.style.stroke-dash: 5
`,
			key: `A`,

			exp: `B
`,
		},
		{
			name: "flat_reserved",

			text: `A -> B
A.style.stroke-dash: 5
`,
			key: `A.style.stroke-dash`,

			exp: `A -> B
`,
		},
		{
			name: "singular_flat_style",

			text: `x.style.fill: red
`,
			key: `x.style.fill`,

			exp: `x
`,
		},
		{
			name: "nested_flat_style",

			text: `x: {
	style.fill: red
}
`,
			key: `x.style.fill`,

			exp: `x
`,
		},
		{
			name: "multiple_map_styles",

			text: `x: {
  style: {
    opacity: 0.4
    fill: red
  }
}
`,
			key: `x.style.fill`,

			exp: `x: {
  style: {
    opacity: 0.4
  }
}
`,
		},
		{
			name: "singular_map_style",

			text: `x: {
  style: {
    fill: red
  }
}
`,
			key: `x.style.fill`,

			exp: `x
`,
		},
		{
			name: "delete_near",

			text: `x: {
	near: y
}
y
`,
			key: `x.near`,

			exp: `x
y
`,
		},
		{
			name: "delete_container_of_near",

			text: `direction: down
first input -> start game -> game loop

game loop: {
  direction: down
  input -> increase bird top velocity

  move bird -> move pipes -> render

  render -> no collision -> wait 16 milliseconds -> move bird
  render -> collision detected -> game over
  no collision.near: game loop.collision detected
}
`,
			key: `game loop`,

			exp: `direction: down
first input -> start game

input -> increase bird top velocity

move bird -> move pipes -> render

render -> no collision -> wait 16 milliseconds -> move bird
render -> collision detected -> game over
no collision.near: collision detected
`,
		},
		{
			name: "delete_tooltip",

			text: `x: {
	tooltip: yeah
}
`,
			key: `x.tooltip`,

			exp: `x
`,
		},
		{
			name: "delete_link",

			text: `x.link: https://google.com
`,
			key: `x.link`,

			exp: `x
`,
		},
		{
			name: "delete_icon",

			text: `y.x: {
  link: https://google.com
	icon: https://google.com/memes.jpeg
}
`,
			key: `y.x.icon`,

			exp: `y.x: {
  link: https://google.com
}
`,
		},
		{
			name: "delete_redundant_flat_near",

			text: `x

y
`,
			key: `x.near`,

			exp: `x

y
`,
		},
		{
			name: "delete_needed_flat_near",

			text: `x.near: y
y
`,
			key: `x.near`,

			exp: `x
y
`,
		},
		{
			name: "children_no_self_conflict",

			text: `x: {
  x
}
`,
			key: `x`,

			exp: `x
`,
		},
		{
			name: "near",

			text: `x: {
  near: y
}
y
`,
			key: `y`,

			exp: `x
`,
		},
		{
			name: "container_near",

			text: `x: {
  y: {
    near: x.z
  }
  z
	a: {
	  near: x.z
  }
}
`,
			key: `x`,

			exp: `y: {
  near: z
}
z
a: {
  near: z
}
`,
		},
		{
			name: "multi_near",

			text: `Starfish: {
  API
  Bluefish: {
    near: Starfish.API
  }
	Yo: {
    near: Blah
  }
}
Blah
`,
			key: `Starfish`,

			exp: `API
Bluefish: {
  near: API
}
Yo: {
  near: Blah
}

Blah
`,
		},
		{
			name: "children_nested_conflicts",

			text: `p: {
	x: {
    y
  }
}
x
`,
			key: `p`,

			exp: `x 2: {
  y
}

x
`,
		},
		{
			name: "children_referenced_conflicts",

			text: `p: {
	x
}
x

p.x: "hi"
`,
			key: `p`,

			exp: `x 2

x

x 2: "hi"
`,
		},
		{
			name: "children_flat_conflicts",

			text: `p.x
x

p.x: "hi"
`,
			key: `p`,

			exp: `x 2
x

x 2: "hi"
`,
		},
		{
			name: "children_edges_flat_conflicts",

			text: `p.x -> p.y -> p.z
x
z

p.x: "hi"
p.z: "ey"
`,
			key: `p`,

			exp: `x 2 -> y -> z 2
x
z

x 2: "hi"
z 2: "ey"
`,
		},
		{
			name: "children_nested_referenced_conflicts",

			text: `p: {
	x.y
}
x

p.x: "hi"
p.x.y: "hey"
`,
			key: `p`,

			exp: `x 2.y

x

x 2: "hi"
x 2.y: "hey"
`,
		},
		{
			name: "children_edge_conflicts",

			text: `p: {
	x -> y
}
x

p.x: "hi"
`,
			key: `p`,

			exp: `x 2 -> y

x

x 2: "hi"
`,
		},
		{
			name: "children_multiple_conflicts",

			text: `p: {
	x -> y
	x
	y
}
x
y

p.x: "hi"
`,
			key: `p`,

			exp: `x 2 -> y 2
x 2
y 2

x
y

x 2: "hi"
`,
		},
		{
			name: "multi_path_map_conflict",

			text: `x.y: {
  z
}
x: {
  z
}
`,
			key: `x.y`,

			exp: `x: {
  z 2
}
x: {
  z
}
`,
		},
		{
			name: "multi_path_map_no_conflict",

			text: `x.y: {
  z
}
x: {
  z
}
`,
			key: `x`,

			exp: `y: {
  z
}

z
`,
		},
		{
			name: "children_scope",

			text: `x.q: {
  p: {
    what's up
    x -> y
  }
}
`,
			key: `x.q.p`,

			exp: `x.q: {
  what's up
  x -> y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
			},
		},
		{
			name: "children_order",

			text: `c: {
  before
  y: {
    congo
  }
  after
}
`,
			key: `c.y`,

			exp: `c: {
  before

  congo

  after
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "edge_first",

			text: `l.p.d: {x -> p -> y -> z}
`,
			key: `l.p.d.(x -> p)[0]`,

			exp: `l.p.d: {x; p -> y -> z}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 7 {
					t.Fatalf("expected 7 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 2 {
					t.Fatalf("unexpected edges: %#v", g.Objects)
				}
			},
		},
		{
			name: "multiple_flat_middle_container",

			text: `a.b.c
a.b.d
`,
			key: `a.b`,

			exp: `a.c
a.d
`,
		},
		{
			name: "edge_middle",

			text: `l.p.d: {x -> y -> z -> q -> p}
`,
			key: `l.p.d.(z -> q)[0]`,

			exp: `l.p.d: {x -> y -> z; q -> p}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 8 {
					t.Fatalf("expected 8 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 3 {
					t.Fatalf("expected three edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_last",

			text: `l.p.d: {x -> y -> z -> q -> p}
`,
			key: `l.p.d.(q -> p)[0]`,

			exp: `l.p.d: {x -> y -> z -> q; p}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 8 {
					t.Fatalf("expected 8 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 3 {
					t.Fatalf("expected three edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "key_with_edges",

			text: `hello.meow -> hello.bark
`,
			key: `hello.(meow -> bark)[0]`,

			exp: `hello.meow
hello.bark
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected three objects: %#v", g.Objects)
				}
				if len(g.Edges) != 0 {
					t.Fatalf("expected zero edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "key_with_edges_2",

			text: `hello.meow -> hello.bark
`,
			key: `hello.meow`,

			exp: `hello.bark
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "key_with_edges_3",

			text: `hello.(meow -> bark)
`,
			key: `hello.meow`,

			exp: `hello.bark
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "key_with_edges_4",

			text: `hello.(meow -> bark)
`,
			key: `(hello.meow -> hello.bark)[0]`,

			exp: `hello.meow
hello.bark
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected three objects: %#v", g.Objects)
				}
				if len(g.Edges) != 0 {
					t.Fatalf("expected zero edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "nested",

			text: `a.b.c.d
`,
			key: `a.b`,

			exp: `a.c.d
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "nested_2",

			text: `a.b.c.d
`,
			key: `a.b.c.d`,

			exp: `a.b.c
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_1",

			text: `x -> p -> y -> z
`,
			key: `p`,

			exp: `x
y -> z
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_2",

			text: `p -> y -> z
`,
			key: `y`,

			exp: `p
z
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_3",

			text: `y -> p -> y -> z
`,
			key: `y`,

			exp: `p
z
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_4",

			text: `y -> p
`,
			key: `p`,

			exp: `y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 object: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_5",

			text: `x: {
  a -> b -> c
  q -> p
}
`,
			key: `x.a`,

			exp: `x: {
  b -> c
  q -> p
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_6",

			text: `x: {
  lol
}
x.p.q.z
`,
			key: `x.p.q.z`,

			exp: `x: {
  lol
}
x.p.q
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 4 {
					t.Fatalf("expected 4 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_7",

			text: `x: {
  lol
}
x.p.q.more
x.p.q.z
`,
			key: `x.p.q.z`,

			exp: `x: {
  lol
}
x.p.q.more
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "order_8",

			text: `x -> y
bark
y -> x
zebra
x -> q
kang
`,
			key: `x`,

			exp: `bark
y

zebra
q

kang
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 5 {
					t.Fatalf("expected 5 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "empty_map",

			text: `c: {
  y: {
    congo
  }
}
`,
			key: `c.y.congo`,

			exp: `c: {
  y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
			},
		},
		{
			name: "edge_common",

			text: `x.a -> x.y
`,
			key: "x",

			exp: `a -> y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("unexpected edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_common_2",

			text: `x.(a -> y)
`,
			key: "x",

			exp: `a -> y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("unexpected edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_common_3",

			text: `x.(a -> y)
`,
			key: "(x.a -> x.y)[0]",

			exp: `x.a
x.y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 0 {
					t.Fatalf("unexpected edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_common_4",

			text: `x.a -> x.y
`,
			key: "x.(a -> y)[0]",

			exp: `x.a
x.y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 0 {
					t.Fatalf("unexpected edges: %#v", g.Edges)
				}
			},
		},
		{
			name: "edge_decrement",

			text: `a -> b
a -> b
a -> b
a -> b
a -> b
(a -> b)[0]: zero
(a -> b)[1]: one
(a -> b)[2]: two
(a -> b)[3]: three
(a -> b)[4]: four
`,
			key: `(a -> b)[2]`,

			exp: `a -> b
a -> b

a -> b
a -> b
(a -> b)[0]: zero
(a -> b)[1]: one

(a -> b)[2]: three
(a -> b)[3]: four
`,
		},
		{
			name: "shape_class",
			text: `D2 Parser: {
  shape: class

  # Default visibility is + so no need to specify.
  +reader: io.RuneReader
  readerPos: d2ast.Position

  # Private field.
  -lookahead: "[]rune"

  # Protected field.
  # We have to escape the # to prevent the line from being parsed as a comment.
  \#lookaheadPos: d2ast.Position

  +peek(): (r rune, eof bool)
  rewind()
  commit()

  \#peekn(n int): (s string, eof bool)
}

"github.com/terrastruct/d2parser.git" -> D2 Parser
`,
			key: `D2 Parser`,

			exp: `"github.com/terrastruct/d2parser.git"
`,
		},
		// TODO: delete disks.id as it's redundant
		{
			name: "shape_sql_table",

			text: `cloud: {
  disks: {
    shape: sql_table
    id: int {constraint: primary_key}
  }
  blocks: {
    shape: sql_table
    id: int {constraint: primary_key}
    disk: int {constraint: foreign_key}
    blob: blob
  }
  blocks.disk -> disks.id

  AWS S3 Vancouver -> disks
}
`,
			key: "cloud.blocks",

			exp: `cloud: {
  disks: {
    shape: sql_table
    id: int {constraint: primary_key}
  }
  disks.id

  AWS S3 Vancouver -> disks
}
`,
		},
		{
			name: "nested_reserved",

			text: `x.y.z: {
  label: Sweet April showers do spring May flowers.
  icon: bingo
	near: x.y.jingle
  shape: parallelogram
  style: {
    stroke: red
  }
}
x.y.jingle
`,
			key: "x.y.z",

			exp: `x.y
x.y.jingle
`,
		},
		{
			name: "only_delete_obj_reserved",

			text: `A: {style.stroke: "#000e3d"}
B
A -> B: {style.stroke: "#2b50c2"}
`,
			key: `A.style.stroke`,
			exp: `A
B
A -> B: {style.stroke: "#2b50c2"}
`,
		},
		{
			name: "only_delete_edge_reserved",

			text: `A: {style.stroke: "#000e3d"}
B
A -> B: {style.stroke: "#2b50c2"}
`,
			key: `(A->B)[0].style.stroke`,
			exp: `A: {style.stroke: "#000e3d"}
B
A -> B
`,
		},
		{
			name: "width",

			text: `x: {
  width: 200
}
`,
			key: `x.width`,

			exp: `x
`,
		},
		{
			name: "left",

			text: `x: {
  left: 200
}
`,
			key: `x.left`,

			exp: `x
`,
		},
		{
			name: "conflicts_generated",
			text: `Text 4
Square: {
  Text 4: {
    Text 2
  }
  Text
}
`,
			key: `Square`,

			exp: `Text 4

Text 2: {
  Text 2
}
Text
`,
		},
		{
			name: "conflicts_generated_continued",
			text: `Text 4

Text: {
  Text 2
}
Text 2
`,
			key: `Text`,

			exp: `Text 4

Text

Text 2
`,
		},
		{
			name: "conflicts_generated_3",
			text: `x: {
  Square 2
  Square 3
}

Square 2
Square
`,
			key: `x`,

			exp: `Square 4
Square 3

Square 2
Square
`,
		},
		{
			name: "drop_value",
			text: `a.b.c: "c label"
`,
			key: `a.b.c`,

			exp: `a.b
`,
		},
		{
			name: "drop_value_with_primary",
			text: `a.b: hello {
  shape: circle
}
`,
			key: `a.b`,

			exp: `a
`,
		},
		{
			name: "save_map",
			text: `a.b: {
  shape: circle
}
`,
			key: `a`,

			exp: `b: {
  shape: circle
}
`,
		},
		{
			name: "save_map_with_primary",
			text: `a.b: hello {
  shape: circle
}
`,
			key: `a`,

			exp: `b: hello {
  shape: circle
}
`,
		},
		{
			name: "chaos_1",

			text: `cm: {shape: cylinder}
cm <-> cm: {source-arrowhead.shape: cf-one-required}
mt: z
cdpdxz

bymdyk: hdzuj {shape: class}

bymdyk <-> bymdyk
cm

cm <-> bymdyk: {
  source-arrowhead.shape: cf-many-required
  target-arrowhead.shape: arrow
}
bymdyk <-> cdpdxz

bymdyk -> cm: nk {
  target-arrowhead.shape: diamond
  target-arrowhead.label: 1
}
`,
			key: `bymdyk`,

			exp: `cm: {shape: cylinder}
cm <-> cm: {source-arrowhead.shape: cf-one-required}
mt: z
cdpdxz

cm
`,
		},
		{
			name: "layers-basic",

			text: `a

layers: {
  x: {
    b
    c
  }
}
`,
			key:       `c`,
			boardPath: []string{"x"},

			exp: `a

layers: {
  x: {
    b
  }
}
`,
		},
		{
			name: "scenarios-basic",

			text: `a

scenarios: {
  x: {
    b
    c
  }
}
`,
			key:       `c`,
			boardPath: []string{"x"},

			exp: `a

scenarios: {
  x: {
    b
  }
}
`,
		},
		{
			name: "scenarios-inherited",

			text: `a

scenarios: {
  x: {
    b
    c
  }
}
`,
			key:       `a`,
			boardPath: []string{"x"},

			exp: `a

scenarios: {
  x: {
    b
    c
    a: null
  }
}
`,
		},
		{
			name: "scenarios-edge-inherited",

			text: `a -> b

scenarios: {
  x: {
    b
    c
  }
}
`,
			key:       `(a -> b)[0]`,
			boardPath: []string{"x"},

			exp: `a -> b

scenarios: {
  x: {
    b
    c
    (a -> b)[0]: null
  }
}
`,
		},
		{
			name: "import/1",

			text: `...@meow
y
`,
			fsTexts: map[string]string{
				"meow.d2": `x: {
  a
}
`,
			},
			key: `x`,
			exp: `...@meow
y
x: null
`,
		},
		{
			name: "import/2",

			text: `...@meow

scenarios: {
  y: {
    c
  }
}
`,
			fsTexts: map[string]string{
				"meow.d2": `x: {
  a
}
`,
			},
			boardPath: []string{"y"},
			key:       `x`,
			exp: `...@meow

scenarios: {
  y: {
    c
    x: null
  }
}
`,
		},
		{
			name: "import/3",

			text: `...@meow
`,
			fsTexts: map[string]string{
				"meow.d2": `a -> b
`,
			},
			key: `(a -> b)[0]`,
			exp: `...@meow
(a -> b)[0]: null
`,
		},
		{
			name: "import/4",

			text: `...@meow
`,
			fsTexts: map[string]string{
				"meow.d2": `a.link: https://google.com
`,
			},
			key: `a.link`,
			exp: `...@meow
a.link: null
`,
		},
		{
			name: "import/5",

			text: `...@meow
`,
			fsTexts: map[string]string{
				"meow.d2": `a -> b: {
	target-arrowhead: 1
}
`,
			},
			key: `(a -> b)[0].target-arrowhead`,
			exp: `...@meow
(a -> b)[0].target-arrowhead: null
`,
		},
		{
			name: "import/6",

			text: `...@meow
`,
			fsTexts: map[string]string{
				"meow.d2": `a.style.fill: red
`,
			},
			key: `a.style.fill`,
			exp: `...@meow
a.style.fill: null
`,
		},
		{
			name: "import/7",

			text: `...@meow
a.label.near: center-center
`,
			fsTexts: map[string]string{
				"meow.d2": `a
`,
			},
			key: `a.label.near`,
			exp: `...@meow
`,
		},
		{
			name: "import/8",

			text: `...@meow
(a -> b)[0].style.stroke: red
`,
			fsTexts: map[string]string{
				"meow.d2": `a -> b
`,
			},
			key: `(a -> b)[0].style.stroke`,
			exp: `...@meow
`,
		},
		{
			name: "label-near/1",

			text: `yes: {label.near: center-center}
`,
			key: `yes.label.near`,
			exp: `yes
`,
		},
		{
			name: "label-near/2",

			text: `yes.label.near: center-center
`,
			key: `yes.label.near`,
			exp: `yes
`,
		},
		{
			name: "connection-glob",

			text: `* -> *
a
b
`,
			key: `(a -> b)[0]`,
			exp: `* -> *
a
b
(a -> b)[0]: null
`,
		},
		{
			name: "glob-child/1",

			text: `*.b
a
`,
			key: `a.b`,
			exp: `*.b
a
a.b: null
`,
		},
		{
			name: "delete-imported-layer-obj",

			text: `layers: {
  x: {
    ...@meow
  }
}
`,
			fsTexts: map[string]string{
				"meow.d2": `a
`,
			},
			boardPath: []string{"x"},
			key:       `a`,
			exp: `layers: {
  x: {
    ...@meow
    a: null
  }
}
`,
		},
		{
			name: "delete-not-layer-obj",

			text: `b.style.fill: red
layers: {
  x: {
		a
  }
}
`,
			key: `b.style.fill`,
			exp: `b

layers: {
  x: {
    a
  }
}
`,
		},
		{
			name: "delete-layer-obj",

			text: `layers: {
  x: {
		a
  }
}
`,
			boardPath: []string{"x"},
			key:       `a`,
			exp: `layers: {
  x
}
`,
		},
		{
			name: "delete-layer-style",

			text: `layers: {
  x: {
		a.style.fill: red
  }
}
`,
			boardPath: []string{"x"},
			key:       `a.style.fill`,
			exp: `layers: {
  x: {
    a
  }
}
`,
		},
		{
			name: "edge-out-layer",

			text: `x: {
	a -> b
}
`,
			key: `x.(a -> b)[0].style.stroke`,
			exp: `x: {
  a -> b
}
`,
		},
		{
			name: "edge-in-layer",

			text: `layers: {
  test: {
    x: {
			a -> b
    }
  }
}
`,
			boardPath: []string{"test"},
			key:       `x.(a -> b)[0].style.stroke`,
			exp: `layers: {
  test: {
    x: {
      a -> b
    }
  }
}
`,
		},
		{
			name: "label-near-in-layer",

			text: `layers: {
  x: {
    y: {
      label.near: center-center
    }
    a
  }
}
`,
			boardPath: []string{"x"},
			key:       `y`,
			exp: `layers: {
  x: {
    a
  }
}
`,
		},
		{
			name: "update-near-in-layer",

			text: `layers: {
  x: {
    y: {
      near: a
    }
    a
  }
}
`,
			boardPath: []string{"x"},
			key:       `y`,
			exp: `layers: {
  x: {
    a
  }
}
`,
		},
		{
			name: "edge-with-glob",

			text: `x -> y
y

(* -> *)[*].style.opacity: 0.8
`,
			key: `(x -> y)[0]`,
			exp: `x
y

(* -> *)[*].style.opacity: 0.8
`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			et := editTest{
				text:    tc.text,
				fsTexts: tc.fsTexts,
				testFunc: func(g *d2graph.Graph) (*d2graph.Graph, error) {
					return d2oracle.Delete(g, tc.boardPath, tc.key)
				},

				exp:        tc.exp,
				expErr:     tc.expErr,
				assertions: tc.assertions,
			}
			et.run(t)
		})
	}
}

type editTest struct {
	text     string
	fsTexts  map[string]string
	testFunc func(*d2graph.Graph) (*d2graph.Graph, error)

	exp        string
	expErr     string
	assertions func(*testing.T, *d2graph.Graph)
}

func (tc editTest) run(t *testing.T) {
	var tfs *mapfs.FS
	d2Path := fmt.Sprintf("d2/testdata/d2oracle/%v.d2", t.Name())
	if tc.fsTexts != nil {
		tc.fsTexts["index.d2"] = tc.text
		d2Path = "index.d2"
		var err error
		tfs, err = mapfs.New(tc.fsTexts)
		assert.Success(t, err)
		t.Cleanup(func() {
			assert.Success(t, tfs.Close())
		})
	}

	g, _, err := d2compiler.Compile(d2Path, strings.NewReader(tc.text), &d2compiler.CompileOptions{
		FS: tfs,
	})
	assert.Success(t, err)

	g, err = tc.testFunc(g)
	if tc.expErr != "" {
		if err == nil {
			t.Fatalf("expected error with: %q", tc.expErr)
		}
		ds, err := diff.Strings(tc.expErr, err.Error())
		if err != nil {
			t.Fatal(err)
		}
		if ds != "" {
			t.Fatalf("unexpected error: %s", ds)
		}
	} else if err != nil {
		t.Fatal(err)
	}

	if tc.expErr == "" {
		if tc.assertions != nil {
			t.Run("assertions", func(t *testing.T) {
				tc.assertions(t, g)
			})
		}

		newText := d2format.Format(g.AST)
		ds, err := diff.Strings(tc.exp, newText)
		if err != nil {
			t.Fatal(err)
		}
		if ds != "" {
			t.Fatalf("tc.exp != newText:\n%s", ds)
		}
	}

	got := struct {
		Graph *d2graph.Graph `json:"graph"`
		Err   string         `json:"err"`
	}{
		Graph: g,
		Err:   fmt.Sprintf("%#v", err),
	}

	err = diff.TestdataJSON(filepath.Join("..", "testdata", "d2oracle", t.Name()), got)
	assert.Success(t, err)
}

func TestReconnectEdgeIDDeltas(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		boardPath []string
		text      string
		edge      string
		newSrc    string
		newDst    string

		exp    string
		expErr string
	}{
		{
			name: "basic",

			text: `a -> b
x
`,
			edge:   "(a -> b)[0]",
			newDst: "x",

			exp: `{
  "(a -> b)[0]": "(a -> x)[0]"
}`,
		},
		{
			name: "both",
			text: `a
b
c
a -> b
`,
			edge:   `(a -> b)[0]`,
			newSrc: "b",
			newDst: "a",
			exp: `{
  "(a -> b)[0]": "(b -> a)[0]"
}`,
		},
		{
			name: "contained",

			text: `a.x -> a.y
a.z
`,
			edge:   "a.(x -> y)[0]",
			newDst: "a.z",

			exp: `{
  "a.(x -> y)[0]": "a.(x -> z)[0]"
}`,
		},
		{
			name: "second_index",

			text: `a -> b
a -> b
c
`,
			edge:   "(a -> b)[1]",
			newDst: "c",

			exp: `{
  "(a -> b)[1]": "(a -> c)[0]"
}`,
		},
		{
			name: "old_sibling_decrement",

			text: `a -> b
a -> b
c
`,
			edge:   "(a -> b)[0]",
			newDst: "c",

			exp: `{
  "(a -> b)[0]": "(a -> c)[0]",
  "(a -> b)[1]": "(a -> b)[0]"
}`,
		},
		{
			name: "new_sibling_increment",

			text: `a -> b
c -> b
a -> b
`,
			edge:   "(c -> b)[0]",
			newSrc: "a",

			exp: `{
  "(a -> b)[1]": "(a -> b)[2]",
  "(c -> b)[0]": "(a -> b)[1]"
}`,
		},
		{
			name: "increment_and_decrement",

			text: `a -> b
c -> b
c -> b
a -> b
`,
			edge:   "(c -> b)[0]",
			newSrc: "a",

			exp: `{
  "(a -> b)[1]": "(a -> b)[2]",
  "(c -> b)[0]": "(a -> b)[1]",
  "(c -> b)[1]": "(c -> b)[0]"
}`,
		},
		{
			name: "in_chain",

			text: `a -> b -> a -> b
c
`,
			edge:   "(a -> b)[0]",
			newDst: "c",

			exp: `{
  "(a -> b)[0]": "(a -> c)[0]",
  "(a -> b)[1]": "(a -> b)[0]"
}`,
		},
		{
			name: "in_chain_2",

			text: `a -> b -> a -> b
c
`,
			edge:   "(a -> b)[1]",
			newDst: "c",

			exp: `{
  "(a -> b)[1]": "(a -> c)[0]"
}`,
		},
		{
			name: "in_chain_3",

			text: `a -> b -> a -> c
`,
			edge:   "(a -> b)[0]",
			newDst: "c",

			exp: `{
  "(a -> b)[0]": "(a -> c)[1]"
}`,
		},
		{
			name: "in_chain_4",

			text: `a -> c -> a -> c
b
`,
			edge:   "(a -> c)[0]",
			newDst: "b",

			exp: `{
  "(a -> c)[0]": "(a -> b)[0]",
  "(a -> c)[1]": "(a -> c)[0]"
}`,
		},
		{
			name: "scenarios-outer-scope",
			text: `a

scenarios: {
  x: {
    d -> b
  }
}
`,
			boardPath: []string{"x"},
			edge:      `(d -> b)[0]`,
			newDst:    "a",
			exp: `{
  "(d -> b)[0]": "(d -> a)[0]"
}`,
		},
		{
			name: "scenarios-second",
			text: `g
a -> b
d

scenarios: {
  x: {
    d -> b
  }
}
`,
			boardPath: []string{"x"},
			edge:      `(d -> b)[0]`,
			newSrc:    "a",
			exp: `{
  "(d -> b)[0]": "(a -> b)[1]"
}`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d2Path := fmt.Sprintf("d2/testdata/d2oracle/%v.d2", t.Name())
			g, _, err := d2compiler.Compile(d2Path, strings.NewReader(tc.text), nil)
			if err != nil {
				t.Fatal(err)
			}

			var newSrc *string
			var newDst *string
			if tc.newSrc != "" {
				newSrc = &tc.newSrc
			}
			if tc.newDst != "" {
				newDst = &tc.newDst
			}

			deltas, err := d2oracle.ReconnectEdgeIDDeltas(g, tc.boardPath, tc.edge, newSrc, newDst)
			if tc.expErr != "" {
				if err == nil {
					t.Fatalf("expected error with: %q", tc.expErr)
				}
				ds, err := diff.Strings(tc.expErr, err.Error())
				if err != nil {
					t.Fatal(err)
				}
				if ds != "" {
					t.Fatalf("unexpected error: %s", ds)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if hasRepeatedValue(deltas) {
				t.Fatalf("deltas set more than one value equal to another: %s", string(xjson.Marshal(deltas)))
			}

			ds, err := diff.Strings(tc.exp, string(xjson.Marshal(deltas)))
			if err != nil {
				t.Fatal(err)
			}
			if ds != "" {
				t.Fatalf("unexpected deltas: %s", ds)
			}
		})
	}
}

func TestMoveIDDeltas(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		text               string
		key                string
		newKey             string
		includeDescendants bool

		exp    string
		expErr string
	}{
		{
			name: "rename",

			text: `x
`,
			key:    "x",
			newKey: "y",

			exp: `{
  "x": "y"
}`,
		},
		{
			name: "rename_identical",

			text: `Square
`,
			key:    "Square",
			newKey: "Square",

			exp: `{}`,
		},
		{
			name: "children_no_self_conflict",

			text: `x: {
  x
}
y
`,
			key:    `x`,
			newKey: `y.x`,

			exp: `{
  "x": "y.x",
  "x.x": "x"
}`,
		},
		{
			name: "into_container",

			text: `x
y
x -> z
`,
			key:    "x",
			newKey: "y.x",

			exp: `{
  "(x -> z)[0]": "(y.x -> z)[0]",
  "x": "y.x"
}`,
		},
		{
			name: "out_container",

			text: `x: {
  y
}
x.y -> z
`,
			key:    "x.y",
			newKey: "y",

			exp: `{
  "(x.y -> z)[0]": "(y -> z)[0]",
  "x.y": "y"
}`,
		},
		{
			name: "container_with_edge",

			text: `x {
  a
  b
  a -> b
}
y
`,
			key:    "x",
			newKey: "y.x",

			exp: `{
  "x": "y.x",
  "x.(a -> b)[0]": "(a -> b)[0]",
  "x.a": "a",
  "x.b": "b"
}`,
		},
		{
			name: "out_conflict",

			text: `x: {
  y
}
y
x.y -> z
`,
			key:    "x.y",
			newKey: "y",

			exp: `{
  "(x.y -> z)[0]": "(y 2 -> z)[0]",
  "x.y": "y 2"
}`,
		},
		{
			name: "into_conflict",

			text: `x: {
  y
}
y
x.y -> z
`,
			key:    "y",
			newKey: "x.y",

			exp: `{
  "y": "x.y 2"
}`,
		},
		{
			name: "move_container",

			text: `x: {
  a
  b
}
y
x.a -> x.b
x.a -> x.b
`,
			key:    "x",
			newKey: "y.x",

			exp: `{
  "x": "y.x",
  "x.(a -> b)[0]": "(a -> b)[0]",
  "x.(a -> b)[1]": "(a -> b)[1]",
  "x.a": "a",
  "x.b": "b"
}`,
		},
		{
			name: "conflicts",

			text: `x: {
  a
  b
}
a
y
x.a -> x.b
`,
			key:    "x",
			newKey: "y.x",

			exp: `{
  "x": "y.x",
  "x.(a -> b)[0]": "(a 2 -> b)[0]",
  "x.a": "a 2",
  "x.b": "b"
}`,
		},
		{
			name: "container_conflicts_generated",
			text: `Square 2: "" {
  Square: ""
}
Square: ""
Square 3
`,
			key:    `Square 2`,
			newKey: `Square 3.Square 2`,

			exp: `{
  "Square 2": "Square 3.Square 2",
  "Square 2.Square": "Square 2"
}`,
		},
		{
			name: "duplicate_generated",

			text: `x
x 2
x 3: {
  x 3
  x 4
}
x 4
y
`,
			key:    `x 3`,
			newKey: `y.x 3`,

			exp: `{
  "x 3": "y.x 3",
  "x 3.x 3": "x 3",
  "x 3.x 4": "x 5"
}`,
		},
		{
			name: "include_descendants_flat",

			text: `x.y
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `{
  "x": "z.x",
  "x.y": "z.x.y"
}`,
		},
		{
			name: "include_descendants_map",

			text: `x: {
  y
}
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `{
  "x": "z.x",
  "x.y": "z.x.y"
}`,
		},
		{
			name: "include_descendants_conflict",

			text: `x.y
z.x
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `{
  "x": "z.x 2",
  "x.y": "z.x 2.y"
}`,
		},
		{
			name: "include_descendants_non_conflict",

			text: `x.y
z.x
y
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `{
  "x": "z.x 2",
  "x.y": "z.x 2.y"
}`,
		},
		{
			name: "include_descendants_edge_ref",
			text: `x -> y.z
`,
			key:                `y.z`,
			newKey:             `z`,
			includeDescendants: true,

			exp: `{
  "(x -> y.z)[0]": "(x -> z)[0]",
  "y.z": "z"
}`,
		},
		{
			name: "include_descendants_edge_ref_2",
			text: `x -> y.z
`,
			key:                `y.z`,
			newKey:             `z`,
			includeDescendants: true,

			exp: `{
  "(x -> y.z)[0]": "(x -> z)[0]",
  "y.z": "z"
}`,
		},
		{
			name: "include_descendants_edge_ref_3",
			text: `x -> y.z.a
`,
			key:                `y.z`,
			newKey:             `z`,
			includeDescendants: true,

			exp: `{
  "(x -> y.z.a)[0]": "(x -> z.a)[0]",
  "y.z": "z",
  "y.z.a": "z.a"
}`,
		},
		{
			name: "include_descendants_edge_ref_4",
			text: `x -> y.z.a
b
`,
			key:                `y.z`,
			newKey:             `b.z`,
			includeDescendants: true,

			exp: `{
  "(x -> y.z.a)[0]": "(x -> b.z.a)[0]",
  "y.z": "b.z",
  "y.z.a": "b.z.a"
}`,
		},
		{
			name: "include_descendants_underscore_2",
			text: `a: {
  b: {
    _.c
  }
}
`,
			key:                `a.b`,
			newKey:             `b`,
			includeDescendants: true,

			exp: `{
  "a.b": "b"
}`,
		},
		{
			name: "include_descendants_underscore_3",
			text: `a: {
  b: {
    _.c -> d
		_.c -> _.d
  }
}
`,
			key:                `a.b`,
			newKey:             `b`,
			includeDescendants: true,

			exp: `{
  "a.(c -> b.d)[0]": "(a.c -> b.d)[0]",
  "a.b": "b",
  "a.b.d": "b.d"
}`,
		},
		{
			name: "include_descendants_edge_ref_underscore",
			text: `x
z
x.a -> x.b
b: {
  _.x.a -> _.x.b
}
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `{
  "x": "z.x",
  "x.(a -> b)[0]": "z.x.(a -> b)[0]",
  "x.(a -> b)[1]": "z.x.(a -> b)[1]",
  "x.a": "z.x.a",
  "x.b": "z.x.b"
}`,
		},
		{
			name: "include_descendants_sql_table",

			text: `x: {
  shape: sql_table
  a: b
}
z
`,
			key:                `x`,
			newKey:             `z.x`,
			includeDescendants: true,

			exp: `{
  "x": "z.x"
}`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d2Path := fmt.Sprintf("d2/testdata/d2oracle/%v.d2", t.Name())
			g, _, err := d2compiler.Compile(d2Path, strings.NewReader(tc.text), nil)
			if err != nil {
				t.Fatal(err)
			}

			deltas, err := d2oracle.MoveIDDeltas(g, tc.key, tc.newKey, tc.includeDescendants)
			if tc.expErr != "" {
				if err == nil {
					t.Fatalf("expected error with: %q", tc.expErr)
				}
				ds, err := diff.Strings(tc.expErr, err.Error())
				if err != nil {
					t.Fatal(err)
				}
				if ds != "" {
					t.Fatalf("unexpected error: %s", ds)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if hasRepeatedValue(deltas) {
				t.Fatalf("deltas set more than one value equal to another: %s", string(xjson.Marshal(deltas)))
			}

			ds, err := diff.Strings(tc.exp, string(xjson.Marshal(deltas)))
			if err != nil {
				t.Fatal(err)
			}
			if ds != "" {
				t.Fatalf("unexpected deltas: %s", ds)
			}
		})
	}
}

func TestDeleteIDDeltas(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		boardPath []string
		text      string
		key       string

		exp    string
		expErr string
	}{
		{
			name: "delete_node",

			text: `x.y.p -> x.y.q
x.y.z.w.e.p.l
x.y.z.1.2.3.4
x.y.3.4.5.6
x.y.3.4.6.7
x.y.3.4.6.7 -> x.y.3.4.5.6
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
`,
			key: "x.y",

			exp: `{
  "x.y.(p -> q)[0]": "x.(p -> q)[0]",
  "x.y.3": "x.3",
  "x.y.3.4": "x.3.4",
  "x.y.3.4.(6.7 -> 5.6)[0]": "x.3.4.(6.7 -> 5.6)[0]",
  "x.y.3.4.5": "x.3.4.5",
  "x.y.3.4.5.6": "x.3.4.5.6",
  "x.y.3.4.6": "x.3.4.6",
  "x.y.3.4.6.7": "x.3.4.6.7",
  "x.y.p": "x.p",
  "x.y.q": "x.q",
  "x.y.z": "x.z",
  "x.y.z.(w.e.p.l -> 1.2.3.4)[0]": "x.z.(w.e.p.l -> 1.2.3.4)[0]",
  "x.y.z.1": "x.z.1",
  "x.y.z.1.2": "x.z.1.2",
  "x.y.z.1.2.3": "x.z.1.2.3",
  "x.y.z.1.2.3.4": "x.z.1.2.3.4",
  "x.y.z.w": "x.z.w",
  "x.y.z.w.e": "x.z.w.e",
  "x.y.z.w.e.p": "x.z.w.e.p",
  "x.y.z.w.e.p.l": "x.z.w.e.p.l"
}`,
		},
		{
			name: "children_no_self_conflict",

			text: `x: {
  x
}
`,
			key: `x`,

			exp: `{
  "x.x": "x"
}`,
		},
		{
			name: "duplicate_generated",

			text: `x
x 2
x 3: {
  x 3
  x 4
}
x 4
y
`,
			key: `x 3`,
			exp: `{
  "x 3.x 3": "x 3",
  "x 3.x 4": "x 5"
}`,
		},
		{
			name: "nested-height",

			text: `x: {
  a -> b
  height: 200
}
`,
			key: `x.height`,

			exp: `null`,
		},
		{
			name: "edge-style",

			text: `x <-> y: {
  target-arrowhead: circle
  source-arrowhead: diamond
}
`,
			key: `(x <-> y)[0].target-arrowhead`,

			exp: `null`,
		},
		{
			name: "only-reserved",
			text: `guitar: {
	books: {
		_._.pipe: {
      a
    }
  }
}
`,
			key: `pipe`,

			exp: `{
  "pipe.a": "a"
}`,
		},
		{
			name: "delete_container_with_conflicts",

			text: `x {
  a
  b
}
a
b
c
x.a -> c
`,
			key: "x",

			exp: `{
  "(x.a -> c)[0]": "(a 2 -> c)[0]",
  "x.a": "a 2",
  "x.b": "b 2"
}`,
		},
		{
			name: "multiword",

			text: `Starfish: {
  API
}
Starfish.API
`,
			key: "Starfish",

			exp: `{
  "Starfish.API": "API"
}`,
		},
		{
			name: "delete_container_with_edge",

			text: `x {
  a
  b
  a -> b
}
`,
			key: "x",

			exp: `{
  "x.(a -> b)[0]": "(a -> b)[0]",
  "x.a": "a",
  "x.b": "b"
}`,
		},
		{
			name: "delete_edge_field",

			text: `a -> b
a -> b
`,
			key: "(a -> b)[0].style.opacity",

			exp: "null",
		},
		{
			name: "delete_edge",

			text: `x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[0]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[1]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[2]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[3]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[4]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[5]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[6]: meow
`,
			key: "(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[1]",

			exp: `{
  "x.y.z.(w.e.p.l -> 1.2.3.4)[2]": "x.y.z.(w.e.p.l -> 1.2.3.4)[1]",
  "x.y.z.(w.e.p.l -> 1.2.3.4)[3]": "x.y.z.(w.e.p.l -> 1.2.3.4)[2]",
  "x.y.z.(w.e.p.l -> 1.2.3.4)[4]": "x.y.z.(w.e.p.l -> 1.2.3.4)[3]",
  "x.y.z.(w.e.p.l -> 1.2.3.4)[5]": "x.y.z.(w.e.p.l -> 1.2.3.4)[4]",
  "x.y.z.(w.e.p.l -> 1.2.3.4)[6]": "x.y.z.(w.e.p.l -> 1.2.3.4)[5]"
}`,
		},
		{
			name: "delete_generated_id_conflicts",

			text: `Text 2: {
	Text
	Text 3
}
Text
`,
			key: "Text 2",

			exp: `{
  "Text 2.Text": "Text 2",
  "Text 2.Text 3": "Text 3"
}`,
		},
		{
			name: "delete_generated_id_conflicts_2",

			text: `Text 4
Square: {
  Text 4: {
    Text 2
  }
  Text
}
`,
			key: "Square",

			exp: `{
  "Square.Text": "Text",
  "Square.Text 4": "Text 2",
  "Square.Text 4.Text 2": "Text 2.Text 2"
}`,
		},
		{
			name: "delete_generated_id_conflicts_2_continued",

			text: `Text 4

Text: {
  Text 2
}
Text 2
`,
			key: "Text",

			exp: `{
  "Text.Text 2": "Text"
}`,
		},
		{
			name: "conflicts_generated_3",
			text: `x: {
  Square 2
  Square 3
}

Square 2
Square
`,
			key: `x`,

			exp: `{
  "x.Square 2": "Square 4",
  "x.Square 3": "Square 3"
}`,
		},
		{
			name: "scenarios-basic",
			text: `x

scenarios: {
  y: {
    a
  }
}
`,
			boardPath: []string{"y"},
			key:       `a`,

			exp: `{}`,
		},
		{
			name: "scenarios-parent",
			text: `x

scenarios: {
  y: {
    a.x
  }
}
`,
			boardPath: []string{"y"},
			key:       `a`,

			exp: `{
  "a.x": "x 2"
}`,
		},
		{
			name: "layers-parent",
			text: `x

layers: {
  y: {
    a.x
  }
}
`,
			boardPath: []string{"y"},
			key:       `a`,

			exp: `{
  "a.x": "x"
}`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d2Path := fmt.Sprintf("d2/testdata/d2oracle/%v.d2", t.Name())
			g, _, err := d2compiler.Compile(d2Path, strings.NewReader(tc.text), nil)
			if err != nil {
				t.Fatal(err)
			}

			deltas, err := d2oracle.DeleteIDDeltas(g, tc.boardPath, tc.key)
			if tc.expErr != "" {
				if err == nil {
					t.Fatalf("expected error with: %q", tc.expErr)
				}
				ds, err := diff.Strings(tc.expErr, err.Error())
				if err != nil {
					t.Fatal(err)
				}
				if ds != "" {
					t.Fatalf("unexpected error: %s", ds)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if hasRepeatedValue(deltas) {
				t.Fatalf("deltas set more than one value equal to another: %s", string(xjson.Marshal(deltas)))
			}

			ds, err := diff.Strings(tc.exp, string(xjson.Marshal(deltas)))
			if err != nil {
				t.Fatal(err)
			}
			if ds != "" {
				t.Fatalf("unexpected deltas: %s", ds)
			}
		})
	}
}

func hasRepeatedValue(m map[string]string) bool {
	seen := make(map[string]struct{}, len(m))
	for _, v := range m {
		if _, ok := seen[v]; ok {
			return true
		}
		seen[v] = struct{}{}
	}
	return false
}

func TestRenameIDDeltas(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		boardPath []string
		text      string
		key       string
		newName   string

		exp    string
		expErr string
	}{
		{
			name: "rename_node",

			text: `x.y.p -> x.y.q
x.y.z.w.e.p.l
x.y.z.1.2.3.4
x.y.3.4.5.6
x.y.3.4.6.7
x.y.3.4.6.7 -> x.y.3.4.5.6
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
`,
			key:     "x.y",
			newName: "papa",

			exp: `{
  "x.y": "x.papa",
  "x.y.(p -> q)[0]": "x.papa.(p -> q)[0]",
  "x.y.3": "x.papa.3",
  "x.y.3.4": "x.papa.3.4",
  "x.y.3.4.(6.7 -> 5.6)[0]": "x.papa.3.4.(6.7 -> 5.6)[0]",
  "x.y.3.4.5": "x.papa.3.4.5",
  "x.y.3.4.5.6": "x.papa.3.4.5.6",
  "x.y.3.4.6": "x.papa.3.4.6",
  "x.y.3.4.6.7": "x.papa.3.4.6.7",
  "x.y.p": "x.papa.p",
  "x.y.q": "x.papa.q",
  "x.y.z": "x.papa.z",
  "x.y.z.(w.e.p.l -> 1.2.3.4)[0]": "x.papa.z.(w.e.p.l -> 1.2.3.4)[0]",
  "x.y.z.1": "x.papa.z.1",
  "x.y.z.1.2": "x.papa.z.1.2",
  "x.y.z.1.2.3": "x.papa.z.1.2.3",
  "x.y.z.1.2.3.4": "x.papa.z.1.2.3.4",
  "x.y.z.w": "x.papa.z.w",
  "x.y.z.w.e": "x.papa.z.w.e",
  "x.y.z.w.e.p": "x.papa.z.w.e.p",
  "x.y.z.w.e.p.l": "x.papa.z.w.e.p.l"
}`,
		},
		{
			name: "rename_conflict",

			text: `x
y
`,
			key:     "x",
			newName: "y",

			exp: `{
  "x": "y 2"
}`,
		},
		{
			name: "generated-conflict",

			text: `Square
Square 2
`,
			key:     `Square 2`,
			newName: `Square`,

			exp: `{}`,
		},
		{
			name: "rename_conflict_with_dots",

			text: `"a.b"
y
`,
			key:     "y",
			newName: "a.b",

			exp: `{
  "y": "\"a.b 2\""
}`,
		},
		{
			name: "rename_conflict_with_numbers",

			text: `1
Square
`,
			key:     `Square`,
			newName: `1`,

			exp: `{
  "Square": "1 2"
}`,
		},
		{
			name: "rename_identical",

			text: `Square
`,
			key:     "Square",
			newName: "Square",

			exp: `{}`,
		},
		{
			name: "rename_edge",

			text: `x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
x.y.z.w.e.p.l -> x.y.z.1.2.3.4
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[0]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[1]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[2]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[3]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[4]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[5]: meow
(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[6]: meow
`,
			key:     "(x.y.z.w.e.p.l -> x.y.z.1.2.3.4)[1]",
			newName: "(x.y.z.w.e.p.l <-> x.y.z.1.2.3.4)[1]",

			exp: `{
  "x.y.z.(w.e.p.l -> 1.2.3.4)[1]": "x.y.z.(w.e.p.l <-> 1.2.3.4)[1]"
}`,
		},
		{
			name: "layers-basic",

			text: `x

layers: {
  y: {
    a
  }
}
`,
			boardPath: []string{"y"},
			key:       "a",
			newName:   "b",

			exp: `{
  "a": "b"
}`,
		},
		{
			name: "scenarios-conflict",

			text: `x

scenarios: {
  y: {
    a
  }
}
`,
			boardPath: []string{"y"},
			key:       "a",
			newName:   "x",

			exp: `{
  "a": "x 2"
}`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d2Path := fmt.Sprintf("d2/testdata/d2oracle/%v.d2", t.Name())
			g, _, err := d2compiler.Compile(d2Path, strings.NewReader(tc.text), nil)
			if err != nil {
				t.Fatal(err)
			}

			deltas, err := d2oracle.RenameIDDeltas(g, tc.boardPath, tc.key, tc.newName)
			if tc.expErr != "" {
				if err == nil {
					t.Fatalf("expected error with: %q", tc.expErr)
				}
				ds, err := diff.Strings(tc.expErr, err.Error())
				if err != nil {
					t.Fatal(err)
				}
				if ds != "" {
					t.Fatalf("unexpected error: %s", ds)
				}
			} else if err != nil {
				t.Fatal(err)
			}

			if hasRepeatedValue(deltas) {
				t.Fatalf("deltas set more than one value equal to another: %s", string(xjson.Marshal(deltas)))
			}

			ds, err := diff.Strings(tc.exp, string(xjson.Marshal(deltas)))
			if err != nil {
				t.Fatal(err)
			}
			if ds != "" {
				t.Fatalf("unexpected deltas: %s", ds)
			}
		})
	}
}
