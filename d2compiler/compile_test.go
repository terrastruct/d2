package d2compiler_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"

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
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "hey" {
					t.Fatalf("expected g.Objects[0].ID to be 'hey': %#v", g.Objects[0])
				}
				if g.Objects[0].Attributes.Shape.Value != d2target.ShapeHexagon {
					t.Fatalf("expected g.Objects[0].Attributes.Shape.Value to be hexagon: %#v", g.Objects[0].Attributes.Shape.Value)
				}
				if g.Objects[0].Attributes.Width.Value != "200" {
					t.Fatalf("expected g.Objects[0].Attributes.Width.Value to be 200: %#v", g.Objects[0].Attributes.Width.Value)
				}
				if g.Objects[0].Attributes.Height.Value != "230" {
					t.Fatalf("expected g.Objects[0].Attributes.Height.Value to be 230: %#v", g.Objects[0].Attributes.Height.Value)
				}
			},
		},
		{
			name: "equal_dimensions_on_circle",

			text: `hey: "" {
	shape: circle
	width: 200
	height: 230
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/equal_dimensions_on_circle.d2:3:2: width and height must be equal for circle shapes
d2/testdata/d2compiler/TestCompile/equal_dimensions_on_circle.d2:4:2: width and height must be equal for circle shapes`,
		},
		{
			name: "single_dimension_on_circle",

			text: `hey: "" {
	shape: circle
	height: 230
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatalf("expected 1 objects: %#v", g.Objects)
				}
				if g.Objects[0].ID != "hey" {
					t.Fatalf("expected ID to be 'hey': %#v", g.Objects[0])
				}
				if g.Objects[0].Attributes.Shape.Value != d2target.ShapeCircle {
					t.Fatalf("expected Attributes.Shape.Value to be circle: %#v", g.Objects[0].Attributes.Shape.Value)
				}
				if g.Objects[0].Attributes.Width != nil {
					t.Fatalf("expected Attributes.Width to be nil: %#v", g.Objects[0].Attributes.Width)
				}
				if g.Objects[0].Attributes.Height == nil {
					t.Fatalf("Attributes.Height is nil")
				}
			},
		},
		{
			name: "no_dimensions_on_containers",

			text: `
containers: {
	circle container: {
		shape: circle
		width: 512

		diamond: {
			shape: diamond
			width: 128
			height: 64
		}
	}
	diamond container: {
		shape: diamond
		width: 512
		height: 256

		circle: {
			shape: circle
			width: 128
		}
	}
	oval container: {
		shape: oval
		width: 512
		height: 256

		hexagon: {
			shape: hexagon
			width: 128
			height: 64
		}
	}
	hexagon container: {
		shape: hexagon
		width: 512
		height: 256

		oval: {
			shape: oval
			width: 128
			height: 64
		}
	}
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/no_dimensions_on_containers.d2:5:3: width cannot be used on container: containers.circle container
d2/testdata/d2compiler/TestCompile/no_dimensions_on_containers.d2:15:3: width cannot be used on container: containers.diamond container
d2/testdata/d2compiler/TestCompile/no_dimensions_on_containers.d2:16:3: height cannot be used on container: containers.diamond container
d2/testdata/d2compiler/TestCompile/no_dimensions_on_containers.d2:25:3: width cannot be used on container: containers.oval container
d2/testdata/d2compiler/TestCompile/no_dimensions_on_containers.d2:26:3: height cannot be used on container: containers.oval container
d2/testdata/d2compiler/TestCompile/no_dimensions_on_containers.d2:36:3: width cannot be used on container: containers.hexagon container
d2/testdata/d2compiler/TestCompile/no_dimensions_on_containers.d2:37:3: height cannot be used on container: containers.hexagon container`,
		},
		{
			name: "dimension_with_style",

			text: `x: {
  width: 200
  style.multiple: true
}
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
			expErr: `d2/testdata/d2compiler/TestCompile/shape_unquoted_hex.d2:3:10: missing value after colon`,
		},
		{
			name: "edge_unquoted_hex",

			text: `x -> y: {
	style: {
    fill: #ffffff
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/edge_unquoted_hex.d2:3:10: missing value after colon`,
		},
		{
			name: "blank_underscore",

			text: `x: {
  y
  _
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/blank_underscore.d2:3:3: field key must contain more than underscores`,
		},
		{
			name: "image_non_style",

			text: `x: {
  shape: image
  icon: https://icons.terrastruct.com/aws/_Group%20Icons/EC2-instance-container_light-bg.svg
  name: y
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/image_non_style.d2:4:3: image shapes cannot have children.`,
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
			expErr: `d2/testdata/d2compiler/TestCompile/illegal-stroke-width.d2:2:23: expected "stroke-width" to be a number between 0 and 15`,
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
			name: "underscore_unresolved_obj",

			text: `
x: {
	_.y
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "y", g.Objects[1].ID)
				tassert.Equal(t, g.Objects[0].AbsID(), g.Objects[1].References[0].ScopeObj.AbsID())
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
			expErr: `d2/testdata/d2compiler/TestCompile/underscore_parent_root.d2:2:1: invalid underscore: no parent`,
		},
		{
			name: "underscore_parent_middle_path",

			text: `
x: {
  y._.z
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/underscore_parent_middle_path.d2:3:5: parent "_" can only be used in the beginning of paths, e.g. "_.x"`,
		},
		{
			name: "underscore_parent_sandwich_path",

			text: `
x: {
  _.z._
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/underscore_parent_sandwich_path.d2:3:7: parent "_" can only be used in the beginning of paths, e.g. "_.x"`,
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
				assert.String(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.String(t, "", g.Edges[0].Attributes.Shape.Value)
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
				assert.String(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.String(t, "Reisner's Rule of Conceptual Inertia", g.Edges[0].SrcArrowhead.Label.Value)
				assert.String(t, "QOTD", g.Edges[0].DstArrowhead.Label.Value)
				assert.String(t, "true", g.Edges[0].DstArrowhead.Style.Filled.Value)
				assert.String(t, "", g.Edges[0].Attributes.Shape.Value)
				assert.String(t, "", g.Edges[0].Attributes.Label.Value)
				assert.JSON(t, nil, g.Edges[0].Attributes.Style.Filled)
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
				assert.String(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.String(t, "", g.Edges[0].Attributes.Shape.Value)
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
				assert.String(t, "triangle", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.String(t, "", g.Edges[0].Attributes.Shape.Value)
			},
		},
		{
			name: "object_arrowhead_shape",

			text: `x: {shape: triangle}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/object_arrowhead_shape.d2:1:5: invalid shape, can only set "triangle" for arrowheads`,
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
				assert.String(t, "yo", g.Edges[0].SrcArrowhead.Label.Value)
				assert.String(t, "", g.Edges[0].Attributes.Label.Value)
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
				assert.String(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.String(t, "", g.Edges[0].Attributes.Shape.Value)
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
				assert.String(t, "diamond", g.Edges[0].SrcArrowhead.Shape.Value)
				assert.String(t, "diamond", g.Edges[0].DstArrowhead.Shape.Value)
				assert.String(t, "", g.Edges[0].Attributes.Shape.Value)
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
			expErr: `d2/testdata/d2compiler/TestCompile/nested_edge.d2:2:3: cannot create edge inside edge`,
		},
		{
			name: "shape_edge_style",

			text: `
x: {
	style.animated: true
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/shape_edge_style.d2:3:2: key "animated" can only be applied to edges`,
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
			expErr: `d2/testdata/d2compiler/TestCompile/edge_map_non_reserved.d2:3:3: edge map keys must be reserved keywords`,
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
			name: "near_constant",

			text: `x.near: top-center
`,
		},
		{
			name: "near_bad_constant",

			text: `x.near: txop-center
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near_bad_constant.d2:1:9: near key "txop-center" must be the absolute path to a shape or one of the following constants: top-left, top-center, top-right, center-left, center-right, bottom-left, bottom-center, bottom-right`,
		},
		{
			name: "near_bad_container",

			text: `x: {
  near: top-center
  y
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near_bad_container.d2:2:9: constant near keys cannot be set on shapes with children`,
		},
		{
			name: "near_bad_connected",

			text: `x: {
  near: top-center
}
x -> y
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near_bad_connected.d2:2:9: constant near keys cannot be set on connected shapes`,
		},
		{
			name: "nested_near_constant",

			text: `x.y.near: top-center
`,
			expErr: `d2/testdata/d2compiler/TestCompile/nested_near_constant.d2:1:11: constant near keys can only be set on root level shapes`,
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
d2/testdata/d2compiler/TestCompile/errors/reserved_icon_style.d2:5:18: expected "opacity" to be a number between 0.0 and 1.0
d2/testdata/d2compiler/TestCompile/errors/reserved_icon_style.d2:2:9: near key "y" must be the absolute path to a shape or one of the following constants: top-left, top-center, top-right, center-left, center-right, bottom-left, bottom-center, bottom-right`,
		},
		{
			name: "errors/missing_shape_icon",

			text:   `x.shape: image`,
			expErr: `d2/testdata/d2compiler/TestCompile/errors/missing_shape_icon.d2:1:1: image shape must include an "icon" field`,
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
			expErr: `d2/testdata/d2compiler/TestCompile/edge_to_style.d2:2:8: reserved keywords are prohibited in edges`,
		},
		{
			name: "escaped_id",

			text: `b\nb`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				assert.String(t, `"b\nb"`, g.Objects[0].ID)
				assert.String(t, `b
b`, g.Objects[0].Attributes.Label.Value)
			},
		},
		{
			name: "unescaped_id_cr",

			text: `b\rb`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				assert.String(t, "b\rb", g.Objects[0].ID)
				assert.String(t, "b\rb", g.Objects[0].Attributes.Label.Value)
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
    font-color: blue
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
			name: "table_connection_attr",

			text: `x: {
  shape: sql_table
  y
}
a: {
  shape: sql_table
  b
}
x.y -> a.b: {
  style.animated: true
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "true", g.Edges[0].Attributes.Style.Animated.Value)
			},
		},
		{
			name: "table_column_label",

			text: `x: {
  shape: sql_table
	w: int { label: width }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "width", g.Objects[0].SQLTable.Columns[0].Label.Label)
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
				assert.String(t, `field here`, g.Objects[0].Class.Fields[0].Name)
				assert.String(t, `GetType()`, g.Objects[0].Class.Methods[0].Name)
				assert.String(t, `Is()`, g.Objects[0].Class.Methods[1].Name)
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
				assert.String(t, `GetType()`, g.Objects[0].SQLTable.Columns[0].Name.Label)
				assert.String(t, `Is()`, g.Objects[0].SQLTable.Columns[1].Name.Label)
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
				assert.String(t, `GetType()`, g.Objects[1].SQLTable.Columns[0].Name.Label)
				assert.String(t, `Is()`, g.Objects[1].SQLTable.Columns[1].Name.Label)
			},
		},
		{
			name: "3d_oval",

			text: `SVP1.shape: oval
SVP1.style.3d: true`,
			expErr: `d2/testdata/d2compiler/TestCompile/3d_oval.d2:2:1: key "3d" can only be applied to squares and rectangles`,
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
				assert.String(t, "sequence_diagram", g.Objects[0].Attributes.Shape.Value)
			},
		},
		{
			name: "root_sequence",

			text: `shape: sequence_diagram
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "sequence_diagram", g.Root.Attributes.Shape.Value)
			},
		},
		{
			name: "leaky_sequence",

			text: `x: {
  shape: sequence_diagram
  a
}
b -> x.a
`,
			expErr: `d2/testdata/d2compiler/TestCompile/leaky_sequence.d2:5:1: connections within sequence diagrams can connect only to other objects within the same sequence diagram`,
		},
		{
			name: "sequence_scoping",

			text: `x: {
  shape: sequence_diagram
	a;b
  group: {
    a -> b
    a.t1 -> b.t1
    b.t1.t2 -> b.t1
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 7, len(g.Objects))
				tassert.Equal(t, 3, len(g.Objects[0].ChildrenArray))
			},
		},
		{
			name: "sequence_grouped_note",

			text: `shape: sequence_diagram
a;d
choo: {
  d."this note"
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 4, len(g.Objects))
				tassert.Equal(t, 3, len(g.Root.ChildrenArray))
			},
		},
		{
			name: "sequence_container",

			text: `shape: sequence_diagram
x.y.q -> j.y.p
ok: {
	x.y.q -> j.y.p
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 7, len(g.Objects))
				tassert.Equal(t, 3, len(g.Root.ChildrenArray))
			},
		},
		{
			name: "sequence_container_2",

			text: `shape: sequence_diagram
x.y.q
ok: {
	x.y.q -> j.y.p
	meow
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 8, len(g.Objects))
				tassert.Equal(t, 2, len(g.Root.ChildrenArray))
			},
		},
		{
			name: "root_direction",

			text: `direction: right`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "right", g.Root.Attributes.Direction.Value)
			},
		},
		{
			name: "default_direction",

			text: `x`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "", g.Objects[0].Attributes.Direction.Value)
			},
		},
		{
			name: "set_direction",

			text: `x: {
  direction: left
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "left", g.Objects[0].Attributes.Direction.Value)
			},
		},
		{
			name: "constraint_label",

			text: `foo {
  label: bar
  constraint: BIZ
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "bar", g.Objects[0].Attributes.Label.Value)
			},
		},
		{
			name: "invalid_direction",

			text: `x: {
  direction: diagonal
}`,
			expErr: `d2/testdata/d2compiler/TestCompile/invalid_direction.d2:2:14: direction must be one of up, down, right, left, got "diagonal"`,
		},
		{
			name: "self-referencing",

			text: `x -> x
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				_, err := diff.Strings(g.Edges[0].Dst.ID, g.Edges[0].Src.ID)
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "null",

			text: `null
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "'null'", g.Objects[0].ID)
				tassert.Equal(t, "null", g.Objects[0].IDVal)
			},
		},
		{
			name: "sql-regression",

			text: `a: {
  style: {
    fill: lemonchiffon
  }
  b: {
    shape: sql_table
    c
  }
  d
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 3, len(g.Objects))
			},
		},
		{
			name: "sql-panic",
			text: `test {
    shape: sql_table
    test_id: varchar(64) {constraint: [primary_key, foreign_key]}
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/sql-panic.d2:3:27: reserved field constraint does not accept composite`,
		},
		{
			name: "wrong_column_index",
			text: `Chinchillas: {
  shape: sql_table
  id: int {constraint: primary_key}
  whisker_len: int
  fur_color: string
  age: int
  server: int {constraint: foreign_key}
  caretaker: int {constraint: foreign_key}
}

Chinchillas_Collectibles: {
  shape: sql_table
  id: int
  collectible: id {constraint: foreign_key}
  chinchilla: id {constraint: foreign_key}
}

Chinchillas_Collectibles.chinchilla -> Chinchillas.id`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 0, *g.Edges[0].DstTableColumnIndex)
				tassert.Equal(t, 2, *g.Edges[0].SrcTableColumnIndex)
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

			err = diff.TestdataJSON(filepath.Join("..", "testdata", "d2compiler", t.Name()), got)
			assert.Success(t, err)
		})
	}
}

func TestCompile2(t *testing.T) {
	t.Parallel()

	t.Run("boards", testBoards)
	t.Run("seqdiagrams", testSeqDiagrams)
}

func testBoards(t *testing.T) {
	t.Parallel()

	tca := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "root",
			run: func(t *testing.T) {
				g := assertCompile(t, `base

layers: {
  one: {
    santa
  }
  two: {
    clause
  }
}
`, "")
				assert.JSON(t, 2, len(g.Layers))
				assert.JSON(t, "one", g.Layers[0].Name)
				assert.JSON(t, "two", g.Layers[1].Name)
			},
		},
		{
			name: "recursive",
			run: func(t *testing.T) {
				g := assertCompile(t, `base

layers: {
  one: {
    santa
  }
  two: {
    clause
		steps: {
			seinfeld: {
				reindeer
			}
			missoula: {
				montana
			}
		}
  }
}
`, "")
				assert.Equal(t, 2, len(g.Layers))
				assert.Equal(t, "one", g.Layers[0].Name)
				assert.Equal(t, "two", g.Layers[1].Name)
				assert.Equal(t, 2, len(g.Layers[1].Steps))
			},
		},
		{
			name: "errs/duplicate_board",
			run: func(t *testing.T) {
				assertCompile(t, `base

layers: {
  one: {
    santa
  }
}
steps: {
	one: {
		clause
	}
}
`, `d2/testdata/d2compiler/TestCompile2/boards/errs/duplicate_board.d2:9:2: board name one already used by another board`)
			},
		},
	}

	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func testSeqDiagrams(t *testing.T) {
	t.Parallel()

	t.Run("errs", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "sequence_diagram_edge_between_edge_groups",
				// New sequence diagram scoping implementation is disabled.
				skip: true,
				run: func(t *testing.T) {
					assertCompile(t, `
Office chatter: {
  shape: sequence_diagram
  alice: Alice
  bob: Bobby
  awkward small talk: {
    alice -> bob: uhm, hi
    bob -> alice: oh, hello
    icebreaker attempt: {
      alice -> bob: what did you have for lunch?
    }
    unfortunate outcome: {
      bob -> alice: that's personal
    }
  }
  awkward small talk.icebreaker attempt.alice -> awkward small talk.unfortunate outcome.bob
}
`, "d2/testdata/d2compiler/TestCompile2/seqdiagrams/errs/sequence_diagram_edge_between_edge_groups.d2:16:3: edges between edge groups are not allowed")
				},
			},
		}

		for _, tc := range tca {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if tc.skip {
					t.SkipNow()
				}
				tc.run(t)
			})
		}
	})
}

func assertCompile(t *testing.T, text string, expErr string) *d2graph.Graph {
	d2Path := fmt.Sprintf("d2/testdata/d2compiler/%v.d2", t.Name())
	g, err := d2compiler.Compile(d2Path, strings.NewReader(text), nil)
	if expErr != "" {
		assert.Error(t, err)
		assert.ErrorString(t, err, expErr)
	} else {
		assert.Success(t, err)
	}

	got := struct {
		Graph *d2graph.Graph `json:"graph"`
		Err   error          `json:"err"`
	}{
		Graph: g,
		Err:   err,
	}

	err = diff.TestdataJSON(filepath.Join("..", "testdata", "d2compiler", t.Name()), got)
	assert.Success(t, err)
	return g
}
