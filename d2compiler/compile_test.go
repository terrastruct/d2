package d2compiler_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/diff"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
)

func TestCompile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string

		expErr     string
		assertions func(t *testing.T, g *d2graph.Graph)
	}{
		{
			name: "basic_shape",

			text: `
x: {
  shape: circle
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}

				if g.Objects[0].Attributes.Shape.Value != d2target.ShapeCircle {
					t.Fatalf("expected g.Objects[0].Attributes.Shape.Value to be circle: %#v", g.Objects[0].Attributes.Shape.Value)
				}

			},
		},
		{
			name: "basic_style",

			text: `
x: {
	style.opacity: 0.4
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}

				if g.Objects[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("expected g.Objects[0].Attributes.Style.Opacity.Value to be 0.4: %#v", g.Objects[0].Attributes.Style.Opacity.Value)
				}

			},
		},
		{
			name: "image_style",

			text: `hey: "" {
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
  shape: image
  style.stroke: "#0D32B2"
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
			},
		},

		{
			name: "dimensions_on_nonimage",

			text: `hey: "" {
  shape: hexagon
	width: 200
	height: 230
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/dimensions_on_nonimage.d2:3:2: width is only applicable to image shapes.
d2/testdata/d2compiler/TestCompile/dimensions_on_nonimage.d2:4:2: height is only applicable to image shapes.
`,
		},
		{
			name: "basic_icon",

			text: `hey: "" {
  icon: https://icons.terrastruct.com/essentials/004-picture.svg
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if g.Objects[0].Attributes.Icon == nil {
					t.Fatal("Attribute icon is nil")
				}
			},
		},
		{
			name: "shape_unquoted_hex",

			text: `x: {
	style: {
    fill: #ffffff
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/shape_unquoted_hex.d2:3:10: missing value after colon
`,
		},
		{
			name: "edge_unquoted_hex",

			text: `x -> y: {
	style: {
    fill: #ffffff
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/edge_unquoted_hex.d2:3:10: missing value after colon
`,
		},
		{
			name: "blank_underscore",

			text: `x: {
  y
  _
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/blank_underscore.d2:3:3: invalid use of parent "_"
`,
		},
		{
			name: "image_non_style",

			text: `x: {
  shape: image
  icon: https://icons.terrastruct.com/aws/_Group%20Icons/EC2-instance-container_light-bg.svg
  name: y
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/image_non_style.d2:4:3: image shapes cannot have children.
`,
		},
		{
			name: "stroke-width",

			text: `hey {
  style.stroke-width: 0
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].Attributes.Style.StrokeWidth.Value != "0" {
					t.Fatalf("unexpected")
				}
			},
		},
		{
			name: "illegal-stroke-width",

			text: `hey {
  style.stroke-width: -1
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/illegal-stroke-width.d2:2:23: expected "stroke-width" to be a number between 0 and 15
`,
		},
		{
			name: "underscore_parent_create",

			text: `
x: {
	_.y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Root.ChildrenArray) != 2 {
					t.Fatalf("expected 2 objects at the root: %#v", len(g.Root.ChildrenArray))
				}

			},
		},
		{
			name: "underscore_parent_not_root",

			text: `
x: {
  y: {
    _.z
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Root.ChildrenArray) != 1 {
					t.Fatalf("expected 1 object at the root: %#v", len(g.Root.ChildrenArray))
				}
				if len(g.Objects[0].ChildrenArray) != 2 {
					t.Fatalf("expected 2 objects within x: %v", len(g.Objects[0].ChildrenArray))
				}

			},
		},
		{
			name: "underscore_parent_preference_1",

			text: `
x: {
	_.y: "All we are given is possibilities -- to make ourselves one thing or another."
}
y: "But it's real.  And if it's real it can be affected ...  we may not be able"
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Root.ChildrenArray) != 2 {
					t.Fatalf("expected 2 objects at the root: %#v", len(g.Root.ChildrenArray))
				}
				if g.Objects[1].Attributes.Label.Value != "But it's real.  And if it's real it can be affected ...  we may not be able" {
					t.Fatalf("expected g.Objects[1].Label.Value to be last value: %#v", g.Objects[1].Attributes.Label.Value)
				}
			},
		},
		{
			name: "underscore_parent_preference_2",

			text: `
y: "But it's real.  And if it's real it can be affected ...  we may not be able"
x: {
	_.y: "All we are given is possibilities -- to make ourselves one thing or another."
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "y" {
					t.Fatalf("expected g.Objects[0].ID to be y: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "x" {
					t.Fatalf("expected g.Objects[1].ID to be x: %#v", g.Objects[1])
				}

				if len(g.Root.ChildrenArray) != 2 {
					t.Fatalf("expected 2 objects at the root: %#v", len(g.Root.ChildrenArray))
				}
				if g.Objects[0].Attributes.Label.Value != "All we are given is possibilities -- to make ourselves one thing or another." {
					t.Fatalf("expected g.Objects[0].Label.Value to be last value: %#v", g.Objects[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "underscore_parent_squared",

			text: `
x: {
  y: {
    _._.z
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", len(g.Objects))
				}

				if len(g.Root.ChildrenArray) != 2 {
					t.Fatalf("expected 2 objects at the root: %#v", len(g.Root.ChildrenArray))
				}
			},
		},
		{
			name: "underscore_parent_root",

			text: `
_.x
`,
			expErr: `d2/testdata/d2compiler/TestCompile/underscore_parent_root.d2:2:1: parent "_" cannot be used in the root scope
`,
		},
		{
			name: "underscore_parent_middle_path",

			text: `
x: {
  y._.z
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/underscore_parent_middle_path.d2:3:3: parent "_" can only be used in the beginning of paths, e.g. "_.x"
`,
		},
		{
			name: "underscore_parent_sandwich_path",

			text: `
x: {
  _.z._
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/underscore_parent_sandwich_path.d2:3:3: parent "_" can only be used in the beginning of paths, e.g. "_.x"
`,
		},
		{
			name: "underscore_edge",

			text: `
x: {
  _.y -> _.x
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "y" {
					t.Fatalf("expected g.Edges[0].Src.ID to be y: %#v", g.Edges[0])
				}
				if g.Edges[0].Dst.ID != "x" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be x: %#v", g.Edges[0])
				}
				if g.Edges[0].SrcArrow {
					t.Fatalf("expected g.Edges[0].SrcArrow to be false: %#v", g.Edges[0])
				}
				if !g.Edges[0].DstArrow {
					t.Fatalf("expected g.Edges[0].DstArrow to be true: %#v", g.Edges[0])
				}
			},
		},
		{
			name: "underscore_edge_chain",

			text: `
x: {
  _.y -> _.x -> _.z
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}
				if g.Objects[2].ID != "z" {
					t.Fatalf("expected g.Objects[2].ID to be z: %#v", g.Objects[2])
				}

				if len(g.Edges) != 2 {
					t.Fatalf("expected 2 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "y" {
					t.Fatalf("expected g.Edges[0].Src.ID to be y: %#v", g.Edges[0])
				}
				if g.Edges[0].Dst.ID != "x" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be x: %#v", g.Edges[0])
				}
				if g.Edges[1].Src.ID != "x" {
					t.Fatalf("expected g.Edges[1].Src.ID to be x: %#v", g.Edges[1])
				}
				if g.Edges[1].Dst.ID != "z" {
					t.Fatalf("expected g.Edges[1].Dst.ID to be z: %#v", g.Edges[1])
				}
			},
		},
		{
			name: "underscore_edge_existing",

			text: `
a -> b: "Can you imagine how life could be improved if we could do away with"
x: {
	_.a -> _.b: "Well, it's garish, ugly, and derelicts have used it for a toilet."
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 2 {
					t.Fatalf("expected 2 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "a" {
					t.Fatalf("expected g.Edges[0].Src.ID to be a: %#v", g.Edges[0])
				}
				if g.Edges[0].Dst.ID != "b" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be b: %#v", g.Edges[0])
				}
				if g.Edges[1].Src.ID != "a" {
					t.Fatalf("expected g.Edges[1].Src.ID to be a: %#v", g.Edges[1])
				}
				if g.Edges[1].Dst.ID != "b" {
					t.Fatalf("expected g.Edges[1].Dst.ID to be b: %#v", g.Edges[1])
				}
				if g.Edges[0].Attributes.Label.Value != "Can you imagine how life could be improved if we could do away with" {
					t.Fatalf("unexpected g.Edges[0].Label: %#v", g.Edges[0].Attributes.Label)
				}
				if g.Edges[1].Attributes.Label.Value != "Well, it's garish, ugly, and derelicts have used it for a toilet." {
					t.Fatalf("unexpected g.Edges[1].Label: %#v", g.Edges[1].Attributes.Label)
				}
			},
		},
		{
			name: "underscore_edge_index",

			text: `
a -> b: "Can you imagine how life could be improved if we could do away with"
x: {
	(_.a -> _.b)[0]: "Well, it's garish, ugly, and derelicts have used it for a toilet."
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "a" {
					t.Fatalf("expected g.Edges[0].Src.ID to be a: %#v", g.Edges[0])
				}
				if g.Edges[0].Dst.ID != "b" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be b: %#v", g.Edges[0])
				}
				if g.Edges[0].Attributes.Label.Value != "Well, it's garish, ugly, and derelicts have used it for a toilet." {
					t.Fatalf("unexpected g.Edges[0].Label: %#v", g.Edges[0].Attributes.Label)
				}
			},
		},
		{
			name: "underscore_edge_nested",

			text: `
x: {
	y: {
		_._.z -> _.y
	}
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.AbsID() != "z" {
					t.Fatalf("expected g.Edges[0].Src.AbsID() to be z: %#v", g.Edges[0].Src.AbsID())
				}
				if g.Edges[0].Dst.AbsID() != "x.y" {
					t.Fatalf("expected g.Edges[0].Dst.AbsID() to be x.y: %#v", g.Edges[0].Dst.AbsID())
				}
			},
		},
		{
			name: "edge",

			text: `
x -> y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "x" {
					t.Fatalf("expected g.Edges[0].Src.ID to be x: %#v", g.Edges[0])
				}
				if g.Edges[0].Dst.ID != "y" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be y: %#v", g.Edges[0])
				}
				if g.Edges[0].SrcArrow {
					t.Fatalf("expected g.Edges[0].SrcArrow to be false: %#v", g.Edges[0])
				}
				if !g.Edges[0].DstArrow {
					t.Fatalf("expected g.Edges[0].DstArrow to be true: %#v", g.Edges[0])
				}
			},
		},
		{
			name: "edge_chain",

			text: `
x -> y -> z: "The kids will love our inflatable slides"
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}
				if g.Objects[2].ID != "z" {
					t.Fatalf("expected g.Objects[2].ID to be z: %#v", g.Objects[2])
				}

				if len(g.Edges) != 2 {
					t.Fatalf("expected 2 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "x" {
					t.Fatalf("expected g.Edges[0].Src.ID to be x: %#v", g.Edges[0])
				}
				if g.Edges[0].Dst.ID != "y" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be y: %#v", g.Edges[0])
				}
				if g.Edges[1].Src.ID != "y" {
					t.Fatalf("expected g.Edges[1].Src.ID to be x: %#v", g.Edges[1])
				}
				if g.Edges[1].Dst.ID != "z" {
					t.Fatalf("expected g.Edges[1].Dst.ID to be y: %#v", g.Edges[1])
				}

				if g.Edges[0].Attributes.Label.Value != "The kids will love our inflatable slides" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label: %#v", g.Edges[0].Attributes.Label.Value)
				}
				if g.Edges[1].Attributes.Label.Value != "The kids will love our inflatable slides" {
					t.Fatalf("unexpected g.Edges[1].Attributes.Label: %#v", g.Edges[1].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_index",

			text: `
x -> y: one
(x -> y)[0]: two
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "x" {
					t.Fatalf("expected g.Edges[0].Src.ID to be x: %#v", g.Edges[0].Src)
				}
				if g.Edges[0].Dst.ID != "y" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be y: %#v", g.Edges[0].Dst)
				}
				if g.Edges[0].SrcArrow {
					t.Fatalf("expected g.Edges[0].SrcArrow to be false: %#v", g.Edges[0].SrcArrow)
				}
				if !g.Edges[0].DstArrow {
					t.Fatalf("expected g.Edges[0].DstArrow to be true: %#v", g.Edges[0].DstArrow)
				}
				if g.Edges[0].Attributes.Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Attributes.Label to be two: %#v", g.Edges[0].Attributes.Label)
				}
			},
		},
		{
			name: "edge_index_nested",

			text: `
b: {
	x -> y: one
	(x -> y)[0]: two
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "b" {
					t.Fatalf("expected g.Objects[0].ID to be b: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "x" {
					t.Fatalf("expected g.Objects[1].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[2].ID != "y" {
					t.Fatalf("expected g.Objects[2].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.AbsID() != "b.x" {
					t.Fatalf("expected g.Edges[0].Src.AbsoluteID() to be x: %#v", g.Edges[0].Src)
				}
				if g.Edges[0].Dst.AbsID() != "b.y" {
					t.Fatalf("expected g.Edges[0].Dst.AbsoluteID() to be y: %#v", g.Edges[0].Dst)
				}
				if g.Edges[0].SrcArrow {
					t.Fatalf("expected g.Edges[0].SrcArrow to be false: %#v", g.Edges[0].SrcArrow)
				}
				if !g.Edges[0].DstArrow {
					t.Fatalf("expected g.Edges[0].DstArrow to be true: %#v", g.Edges[0].DstArrow)
				}
				if g.Edges[0].Attributes.Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Attributes.Label to be two: %#v", g.Edges[0].Attributes.Label)
				}
			},
		},
		{
			name: "edge_index_nested_cross_scope",

			text: `
b: {
	x -> y: one
}
b.(x -> y)[0]: two
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "b" {
					t.Fatalf("expected g.Objects[0].ID to be b: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "x" {
					t.Fatalf("expected g.Objects[1].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[2].ID != "y" {
					t.Fatalf("expected g.Objects[2].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.AbsID() != "b.x" {
					t.Fatalf("expected g.Edges[0].Src.AbsoluteID() to be x: %#v", g.Edges[0].Src)
				}
				if g.Edges[0].Dst.AbsID() != "b.y" {
					t.Fatalf("expected g.Edges[0].Dst.AbsoluteID() to be y: %#v", g.Edges[0].Dst)
				}
				if g.Edges[0].SrcArrow {
					t.Fatalf("expected g.Edges[0].SrcArrow to be false: %#v", g.Edges[0].SrcArrow)
				}
				if !g.Edges[0].DstArrow {
					t.Fatalf("expected g.Edges[0].DstArrow to be true: %#v", g.Edges[0].DstArrow)
				}
				if g.Edges[0].Attributes.Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Attributes.Label to be two: %#v", g.Edges[0].Attributes.Label)
				}
			},
		},
		{
			name: "edge_map",

			text: `
x -> y: {
  label: "Space: the final frontier.  These are the voyages of the starship Enterprise."
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "x" {
					t.Fatalf("expected g.Objects[0].ID to be x: %#v", g.Objects[0])
				}
				if g.Objects[1].ID != "y" {
					t.Fatalf("expected g.Objects[1].ID to be y: %#v", g.Objects[1])
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Src.ID != "x" {
					t.Fatalf("expected g.Edges[0].Src.ID to be x: %#v", g.Edges[0])
				}
				if g.Edges[0].Dst.ID != "y" {
					t.Fatalf("expected g.Edges[0].Dst.ID to be y: %#v", g.Edges[0])
				}
				if g.Edges[0].Attributes.Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_label_map",

			text: `hey y9 -> qwer: asdf {style.opacity: 0.5}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Label.Value != "asdf" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_map_arrowhead",

			text: `x -> y: {
  source-arrowhead: {
    shape: diamond
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				diff.AssertStringEq(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.Empty(t, g.Edges[0].Attributes.Shape.Value)
				// Make sure the DSL didn't change. this is a regression test where it did
				exp := `x -> y: {
  source-arrowhead: {
    shape: diamond
  }
}
`
				newText := d2format.Format(g.AST)
				ds, err := diff.Strings(exp, newText)
				if err != nil {
					t.Fatal(err)
				}
				if ds != "" {
					t.Fatalf("exp != newText:\n%s", ds)
				}
			},
		},
		{
			name: "edge_arrowhead_fields",

			text: `x -> y: {
  source-arrowhead: Reisner's Rule of Conceptual Inertia {
    shape: diamond
  }
  target-arrowhead: QOTD
  target-arrowhead: {
    style.filled: true
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				diff.AssertStringEq(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				diff.AssertStringEq(t, "Reisner's Rule of Conceptual Inertia", g.Edges[0].SrcArrowhead.Label.Value)
				diff.AssertStringEq(t, "QOTD", g.Edges[0].DstArrowhead.Label.Value)
				diff.AssertStringEq(t, "true", g.Edges[0].DstArrowhead.Style.Filled.Value)
				assert.Empty(t, g.Edges[0].Attributes.Shape.Value)
				assert.Empty(t, g.Edges[0].Attributes.Label.Value)
				assert.Nil(t, g.Edges[0].Attributes.Style.Filled)
			},
		},
		{
			name: "edge_flat_arrowhead",

			text: `x -> y
(x -> y)[0].source-arrowhead.shape: diamond
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				diff.AssertStringEq(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.Empty(t, g.Edges[0].Attributes.Shape.Value)
			},
		},
		{
			// tests setting to an arrowhead-only shape
			name: "edge_non_shape_arrowhead",

			text: `x -> y: { source-arrowhead.shape: triangle }
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				diff.AssertStringEq(t, "triangle", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.Empty(t, g.Edges[0].Attributes.Shape.Value)
			},
		},
		{
			name: "object_arrowhead_shape",

			text: `x: {shape: triangle}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/object_arrowhead_shape.d2:1:5: invalid shape, can only set "triangle" for arrowheads
`,
		},
		{
			name: "edge_flat_label_arrowhead",

			text: `x -> y: {
  # comment
  source-arrowhead.label: yo
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				diff.AssertStringEq(t, "yo", g.Edges[0].SrcArrowhead.Label.Value)
				assert.Empty(t, g.Edges[0].Attributes.Label.Value)
			},
		},
		{
			name: "edge_semiflat_arrowhead",

			text: `x -> y
(x -> y)[0].source-arrowhead: {
  shape: diamond
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				diff.AssertStringEq(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.Empty(t, g.Edges[0].Attributes.Shape.Value)
			},
		},
		{
			name: "edge_mixed_arrowhead",

			text: `x -> y: {
  target-arrowhead.shape: diamond
}
(x -> y)[0].source-arrowhead: {
  shape: diamond
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}
				diff.AssertStringEq(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				diff.AssertStringEq(t, "diamond", g.Edges[0].DstArrowhead.Shape.Value)
				assert.Empty(t, g.Edges[0].Attributes.Shape.Value)
			},
		},
		{
			name: "edge_exclusive_style",

			text: `
x -> y: {
	style.animated: true
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Animated.Value != "true" {
					t.Fatalf("Edges[0].Attributes.Style.Animated.Value: %#v", g.Edges[0].Attributes.Style.Animated.Value)
				}
			},
		},
		{
			name: "nested_edge",

			text: `sequence -> quest: {
  space -> stars
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/nested_edge.d2:1:1: edges cannot be nested within another edge
`,
		},
		{
			name: "shape_edge_style",

			text: `
x: {
	style.animated: true
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/shape_edge_style.d2:3:2: key "animated" can only be applied to edges
`,
		},
		{
			name: "edge_chain_map",

			text: `
x -> y -> z: {
  label: "Space: the final frontier.  These are the voyages of the starship Enterprise."
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 2 {
					t.Fatalf("expected 2 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
				if g.Edges[1].Attributes.Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[1].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_index_map",

			text: `
x -> y
(x -> y)[0]: {
  label: "Space: the final frontier.  These are the voyages of the starship Enterprise."
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_map_nested",

			text: `
x -> y: {
  style: {
    opacity: 0.4
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
			},
		},
		{
			name: "edge_map_nested_flat",

			text: `
x -> y: {
	style.opacity: 0.4
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
				if g.Edges[0].Attributes.Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_map_group_flat",

			text: `
x -> y
(x -> y)[0].style.opacity: 0.4
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
				if g.Edges[0].Attributes.Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_map_group_semiflat",

			text: `x -> y
(x -> y)[0].style: {
  opacity: 0.4
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatalf("expected 2 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
				if g.Edges[0].Attributes.Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_key_group_flat_nested",

			text: `
x: {
  a -> b
}
x.(a -> b)[0].style.opacity: 0.4
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
				if g.Edges[0].Attributes.Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_key_group_flat_nested_underscore",

			text: `
a -> b
x: {
	(_.a -> _.b)[0].style.opacity: 0.4
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
				if g.Edges[0].Attributes.Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_key_group_map_nested_underscore",

			text: `
a -> b
x: {
	(_.a -> _.b)[0]: {
		style: {
			opacity: 0.4
		}
	}
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
				if g.Edges[0].Attributes.Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_key_group_map_flat_nested_underscore",

			text: `
a -> b
x: {
	(_.a -> _.b)[0]: {
		style.opacity: 0.4
	}
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 3 {
					t.Fatalf("expected 3 objects: %#v", g.Objects)
				}

				if len(g.Edges) != 1 {
					t.Fatalf("expected 1 edge: %#v", g.Edges)
				}
				if g.Edges[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Style.Opacity.Value: %#v", g.Edges[0].Attributes.Style.Opacity.Value)
				}
				if g.Edges[0].Attributes.Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Attributes.Label.Value : %#v", g.Edges[0].Attributes.Label.Value)
				}
			},
		},
		{
			name: "edge_map_non_reserved",

			text: `
x -> y: {
  z
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/edge_map_non_reserved.d2:2:1: edge map keys must be reserved keywords
`,
		},
		{
			name: "url_link",

			text: `x: {
  link: https://google.com
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].Attributes.Link != "https://google.com" {
					t.Fatal(g.Objects[0].Attributes.Link)
				}
			},
		},
		{
			name: "path_link",

			text: `x: {
  link: Overview.Untitled board 7.zzzzz
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].Attributes.Link != "Overview.Untitled board 7.zzzzz" {
					t.Fatal(g.Objects[0].Attributes.Link)
				}
			},
		},
		{
			name: "reserved_icon_near_style",

			text: `x: {
  icon: orange
  style.opacity: 0.5
  style.stroke: red
	style.fill: green
}
x.near: y
y
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].Attributes.NearKey == nil {
					t.Fatal("missing near key")
				}
				if g.Objects[0].Attributes.Icon.Path != "orange" {
					t.Fatal(g.Objects[0].Attributes.Icon)
				}
				if g.Objects[0].Attributes.Style.Opacity.Value != "0.5" {
					t.Fatal(g.Objects[0].Attributes.Style.Opacity)
				}
				if g.Objects[0].Attributes.Style.Stroke.Value != "red" {
					t.Fatal(g.Objects[0].Attributes.Style.Stroke)
				}
				if g.Objects[0].Attributes.Style.Fill.Value != "green" {
					t.Fatal(g.Objects[0].Attributes.Style.Fill)
				}
			},
		},
		{
			name: "errors/reserved_icon_style",

			text: `x: {
  near: y
  icon: "::????:::%%orange"
  style.opacity: -1
  style.opacity: 232
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/errors/reserved_icon_style.d2:3:9: bad icon url "::????:::%%orange": parse "::????:::%%orange": missing protocol scheme
d2/testdata/d2compiler/TestCompile/errors/reserved_icon_style.d2:4:18: expected "opacity" to be a number between 0.0 and 1.0
d2/testdata/d2compiler/TestCompile/errors/reserved_icon_style.d2:5:18: expected "opacity" to be a number between 0.0 and 1.0
d2/testdata/d2compiler/TestCompile/errors/reserved_icon_style.d2:1:1: near key "y" does not exist. It must be the absolute path to a shape.
`,
		},
		{
			name: "errors/missing_shape_icon",

			text: `x.shape: image`,
			expErr: `d2/testdata/d2compiler/TestCompile/errors/missing_shape_icon.d2:1:1: image shape must include an "icon" field
`,
		},
		{
			name: "edge_in_column",

			text: `x: {
  shape: sql_table
  x: {p -> q}
}`,
		},
		{
			name: "edge_to_style",

			text: `x: {style.opacity: 0.4}
y -> x.style
`,
			expErr: `d2/testdata/d2compiler/TestCompile/edge_to_style.d2:2:1: cannot connect to reserved keyword
`,
		},
		{
			name: "escaped_id",

			text: `b\nb`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				diff.AssertStringEq(t, `b
b`, g.Objects[0].Attributes.Label.Value)
			},
		},
		{
			name: "class_style",

			text: `IUserProperties: {
  shape: "class"
  firstName?: "string"
  style.opacity: 0.4
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if len(g.Objects[0].Class.Fields) != 1 {
					t.Fatal(len(g.Objects[0].Class.Fields))
				}
				if len(g.Objects[0].Class.Methods) != 0 {
					t.Fatal(len(g.Objects[0].Class.Methods))
				}
				if g.Objects[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatal(g.Objects[0].Attributes.Style.Opacity.Value)
				}
			},
		},
		{
			name: "table_style",

			text: `IUserProperties: {
  shape: sql_table
  GetType(): string
  style.opacity: 0.4
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if len(g.Objects[0].SQLTable.Columns) != 1 {
					t.Fatal(len(g.Objects[0].SQLTable.Columns))
				}
				if g.Objects[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatal(g.Objects[0].Attributes.Style.Opacity.Value)
				}
			},
		},
		{
			name: "table_style_map",

			text: `IUserProperties: {
  shape: sql_table
  GetType(): string
  style: {
    opacity: 0.4
    color: blue
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if len(g.Objects[0].SQLTable.Columns) != 1 {
					t.Fatal(len(g.Objects[0].SQLTable.Columns))
				}
				if g.Objects[0].Attributes.Style.Opacity.Value != "0.4" {
					t.Fatal(g.Objects[0].Attributes.Style.Opacity.Value)
				}
			},
		},
		{
			name: "class_paren",

			text: `_shape_: "shape" {
  shape: class

	field here
  GetType(): string
  Is(): bool
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				diff.AssertStringEq(t, `field here`, g.Objects[0].Class.Fields[0].Name)
				diff.AssertStringEq(t, `GetType()`, g.Objects[0].Class.Methods[0].Name)
				diff.AssertStringEq(t, `Is()`, g.Objects[0].Class.Methods[1].Name)
			},
		},
		{
			name: "sql_paren",

			text: `_shape_: "shape" {
  shape: sql_table

  GetType(): string
  Is(): bool
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				diff.AssertStringEq(t, `GetType()`, g.Objects[0].SQLTable.Columns[0].Name)
				diff.AssertStringEq(t, `Is()`, g.Objects[0].SQLTable.Columns[1].Name)
			},
		},
		{
			name: "nested_sql",

			text: `outer: {
  table: {
    shape: sql_table

    GetType(): string
    Is(): bool
  }
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 2 {
					t.Fatal(g.Objects)
				}
				if _, has := g.Objects[0].HasChild([]string{"table"}); !has {
					t.Fatal(g.Objects)
				}
				if len(g.Objects[0].ChildrenArray) != 1 {
					t.Fatal(g.Objects)
				}
				diff.AssertStringEq(t, `GetType()`, g.Objects[1].SQLTable.Columns[0].Name)
				diff.AssertStringEq(t, `Is()`, g.Objects[1].SQLTable.Columns[1].Name)
			},
		},
		{
			name: "3d_oval",

			text: `SVP1.style.shape: oval
SVP1.style.3d: true`,
			expErr: `d2/testdata/d2compiler/TestCompile/3d_oval.d2:2:1: key "3d" can only be applied to squares and rectangles
`,
		}, {
			name: "edge_column_index",
			text: `src: {
	shape: sql_table
	id: int
	dst_id: int
}

dst: {
	shape: sql_table
	id: int
	name: string
}

dst.id <-> src.dst_id
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				srcIndex := g.Edges[0].SrcTableColumnIndex
				if srcIndex == nil || *srcIndex != 0 {
					t.Fatalf("expected SrcTableColumnIndex to be 0, got %v", srcIndex)
				}
				dstIndex := g.Edges[0].DstTableColumnIndex
				if dstIndex == nil || *dstIndex != 1 {
					t.Fatalf("expected DstTableColumnIndex to be 1, got %v", dstIndex)
				}
			},
		},
		{
			name: "basic_sequence",

			text: `x: {
  shape: sequence_diagram
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				diff.AssertStringEq(t, "sequence_diagram", g.Objects[0].Attributes.Shape.Value)
			},
		},
		{
			name: "root_sequence",

			text: `shape: sequence_diagram
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				diff.AssertStringEq(t, "sequence_diagram", g.Root.Attributes.Shape.Value)
			},
		},
		{
			name: "default_orientation",

			text: `x`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				diff.AssertStringEq(t, "vertical", g.Objects[0].Attributes.Orientation.Value)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			d2Path := fmt.Sprintf("d2/testdata/d2compiler/%v.d2", t.Name())
			g, err := d2compiler.Compile(d2Path, strings.NewReader(tc.text), nil)
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

			if tc.expErr == "" && tc.assertions != nil {
				t.Run("assertions", func(t *testing.T) {
					tc.assertions(t, g)
				})
			}

			got := struct {
				Graph *d2graph.Graph `json:"graph"`
				Err   error          `json:"err"`
			}{
				Graph: g,
				Err:   err,
			}

			err = diff.Testdata(filepath.Join("..", "testdata", "d2compiler", t.Name()), got)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
