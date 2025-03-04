package d2compiler_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
	"oss.terrastruct.com/util-go/mapfs"

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
		// For tests that use imports, define `index.d2` as text and other files here
		files map[string]string

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

				if g.Objects[0].Shape.Value != d2target.ShapeCircle {
					t.Fatalf("expected g.Objects[0].Shape.Value to be circle: %#v", g.Objects[0].Shape.Value)
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

				if g.Objects[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("expected g.Objects[0].Style.Opacity.Value to be 0.4: %#v", g.Objects[0].Style.Opacity.Value)
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
				if g.Objects[0].Shape.Value != d2target.ShapeHexagon {
					t.Fatalf("expected g.Objects[0].Shape.Value to be hexagon: %#v", g.Objects[0].Shape.Value)
				}
				if g.Objects[0].WidthAttr.Value != "200" {
					t.Fatalf("expected g.Objects[0].Width.Value to be 200: %#v", g.Objects[0].WidthAttr.Value)
				}
				if g.Objects[0].HeightAttr.Value != "230" {
					t.Fatalf("expected g.Objects[0].Height.Value to be 230: %#v", g.Objects[0].HeightAttr.Value)
				}
			},
		},
		{
			name: "positions",
			text: `hey: {
	top: 200
	left: 230
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "200", g.Objects[0].Top.Value)
			},
		},
		{
			name: "reserved_missing_values",
			text: `foobar: {
  width
  bottom
  left
  right
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/reserved_missing_values.d2:2:3: reserved field "width" must have a value
d2/testdata/d2compiler/TestCompile/reserved_missing_values.d2:4:3: reserved field "left" must have a value`,
		},
		{
			name: "positions_negative",
			text: `hey: {
	top: 200
	left: -200
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/positions_negative.d2:3:8: left must be a non-negative integer: "-200"`,
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
				if g.Objects[0].Shape.Value != d2target.ShapeCircle {
					t.Fatalf("expected Attributes.Shape.Value to be circle: %#v", g.Objects[0].Shape.Value)
				}
				if g.Objects[0].WidthAttr != nil {
					t.Fatalf("expected Attributes.Width to be nil: %#v", g.Objects[0].WidthAttr)
				}
				if g.Objects[0].HeightAttr == nil {
					t.Fatalf("Attributes.Height is nil")
				}
			},
		},
		{
			name: "dimensions_on_containers",
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
				if g.Objects[0].Icon == nil {
					t.Fatal("Attribute icon is nil")
				}
			},
		},
		{
			name: "fill-pattern",
			text: `x: {
	style: {
    fill-pattern: dots
  }
}
`,
		},
		{
			name: "invalid-fill-pattern",
			text: `x: {
	style: {
    fill-pattern: ddots
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/invalid-fill-pattern.d2:3:19: expected "fill-pattern" to be one of: none, dots, lines, grain, paper`,
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
			name: "image_children_Steps",

			text: `x: {
  icon: https://icons.terrastruct.com/aws/_Group%20Icons/EC2-instance-container_light-bg.svg
  shape: image
  Steps
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/image_children_Steps.d2:4:3: steps is only allowed at a board root`,
		},
		{
			name: "name-with-dot-underscore",
			text: `A: {
  _.C
}

"D.E": {
  _.C
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 3, len(g.Objects))
			},
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
				if g.Objects[0].Style.StrokeWidth.Value != "0" {
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
			name: "underscore_connection",
			text: `a: {
  _.c.d -> _.c.b
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 4, len(g.Objects))
				tassert.Equal(t, 1, len(g.Edges))
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
				if g.Objects[1].Label.Value != "But it's real.  And if it's real it can be affected ...  we may not be able" {
					t.Fatalf("expected g.Objects[1].Label.Value to be last value: %#v", g.Objects[1].Label.Value)
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
				if g.Objects[0].Label.Value != "All we are given is possibilities -- to make ourselves one thing or another." {
					t.Fatalf("expected g.Objects[0].Label.Value to be last value: %#v", g.Objects[0].Label.Value)
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
			name: "md_block_string_err",

			text: `test: |md
  # What about pipes

  Will escaping \| work?
|
`,
			expErr: `d2/testdata/d2compiler/TestCompile/md_block_string_err.d2:4:19: unexpected text after md block string. See https://d2lang.com/tour/text#advanced-block-strings.
d2/testdata/d2compiler/TestCompile/md_block_string_err.d2:5:1: block string must be terminated with |`,
		},
		{
			name:   "no_empty_block_string",
			text:   `Text: |md |`,
			expErr: `d2/testdata/d2compiler/TestCompile/no_empty_block_string.d2:1:1: block string cannot be empty`,
		},
		{
			name:   "no_white_spaces_only_block_string",
			text:   `Text: |md      |`,
			expErr: `d2/testdata/d2compiler/TestCompile/no_white_spaces_only_block_string.d2:1:1: block string cannot be empty`,
		},
		{
			name: "no_new_lines_only_block_string",
			text: `Text: |md


|`,
			expErr: `d2/testdata/d2compiler/TestCompile/no_new_lines_only_block_string.d2:1:1: block string cannot be empty`,
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
				if g.Edges[0].Label.Value != "Can you imagine how life could be improved if we could do away with" {
					t.Fatalf("unexpected g.Edges[0].Label: %#v", g.Edges[0].Label)
				}
				if g.Edges[1].Label.Value != "Well, it's garish, ugly, and derelicts have used it for a toilet." {
					t.Fatalf("unexpected g.Edges[1].Label: %#v", g.Edges[1].Label)
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
				if g.Edges[0].Label.Value != "Well, it's garish, ugly, and derelicts have used it for a toilet." {
					t.Fatalf("unexpected g.Edges[0].Label: %#v", g.Edges[0].Label)
				}
			},
		},
		{
			name: "legend",

			text: `
			vars: {
  d2-legend: {
    User: "A person who interacts with the system" {
      shape: person
      style: {
        fill: "#f5f5f5"
      }
    }

    Database: "Stores application data" {
      shape: cylinder
      style.fill: "#b5d3ff"
    }

    HiddenShape: "This should not appear in the legend" {
      style.opacity: 0
    }

    User -> Database: "Reads data" {
      style.stroke: "blue"
    }

    Database -> User: "Returns results" {
      style.stroke-dash: 5
    }
  }
}

user: User
db: Database
user -> db: Uses
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if g.Legend == nil {
					t.Fatal("Expected Legend to be non-nil")
					return
				}

				// 2. Verify the correct objects are in the legend
				if len(g.Legend.Objects) != 2 {
					t.Errorf("Expected 2 objects in legend, got %d", len(g.Legend.Objects))
				}

				// Check for User object
				hasUser := false
				hasDatabase := false
				for _, obj := range g.Legend.Objects {
					if obj.ID == "User" {
						hasUser = true
						if obj.Shape.Value != "person" {
							t.Errorf("User shape incorrect, expected 'person', got: %s", obj.Shape.Value)
						}
					} else if obj.ID == "Database" {
						hasDatabase = true
						if obj.Shape.Value != "cylinder" {
							t.Errorf("Database shape incorrect, expected 'cylinder', got: %s", obj.Shape.Value)
						}
					} else if obj.ID == "HiddenShape" {
						t.Errorf("HiddenShape should not be in legend due to opacity: 0")
					}
				}

				if !hasUser {
					t.Errorf("User object missing from legend")
				}
				if !hasDatabase {
					t.Errorf("Database object missing from legend")
				}

				// 3. Verify the correct edges are in the legend
				if len(g.Legend.Edges) != 2 {
					t.Errorf("Expected 2 edges in legend, got %d", len(g.Legend.Edges))
				}

				// Check for expected edges
				hasReadsEdge := false
				hasReturnsEdge := false
				for _, edge := range g.Legend.Edges {
					if edge.Label.Value == "Reads data" {
						hasReadsEdge = true
						// Check edge properties
						if edge.Style.Stroke == nil {
							t.Errorf("Reads edge stroke is nil")
						} else if edge.Style.Stroke.Value != "blue" {
							t.Errorf("Reads edge stroke incorrect, expected 'blue', got: %s", edge.Style.Stroke.Value)
						}
					} else if edge.Label.Value == "Returns results" {
						hasReturnsEdge = true
						// Check edge properties
						if edge.Style.StrokeDash == nil {
							t.Errorf("Returns edge stroke-dash is nil")
						} else if edge.Style.StrokeDash.Value != "5" {
							t.Errorf("Returns edge stroke-dash incorrect, expected '5', got: %s", edge.Style.StrokeDash.Value)
						}
					} else if edge.Label.Value == "Hidden connection" {
						t.Errorf("Hidden connection should not be in legend due to opacity: 0")
					}
				}

				if !hasReadsEdge {
					t.Errorf("'Reads data' edge missing from legend")
				}
				if !hasReturnsEdge {
					t.Errorf("'Returns results' edge missing from legend")
				}

				// 4. Verify the regular diagram content is still there
				userObj, hasUserObj := g.Root.HasChild([]string{"user"})
				if !hasUserObj {
					t.Errorf("Main diagram missing 'user' object")
				} else if userObj.Label.Value != "User" {
					t.Errorf("User label incorrect, expected 'User', got: %s", userObj.Label.Value)
				}

				dbObj, hasDBObj := g.Root.HasChild([]string{"db"})
				if !hasDBObj {
					t.Errorf("Main diagram missing 'db' object")
				} else if dbObj.Label.Value != "Database" {
					t.Errorf("DB label incorrect, expected 'Database', got: %s", dbObj.Label.Value)
				}

				// Check the main edge
				if len(g.Edges) == 0 {
					t.Errorf("No edges found in main diagram")
				} else {
					mainEdge := g.Edges[0]
					if mainEdge.Label.Value != "Uses" {
						t.Errorf("Main edge label incorrect, expected 'Uses', got: %s", mainEdge.Label.Value)
					}
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

				if g.Edges[0].Label.Value != "The kids will love our inflatable slides" {
					t.Fatalf("unexpected g.Edges[0].Label: %#v", g.Edges[0].Label.Value)
				}
				if g.Edges[1].Label.Value != "The kids will love our inflatable slides" {
					t.Fatalf("unexpected g.Edges[1].Label: %#v", g.Edges[1].Label.Value)
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
				if g.Edges[0].Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Label to be two: %#v", g.Edges[0].Label)
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
				if g.Edges[0].Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Label to be two: %#v", g.Edges[0].Label)
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
				if g.Edges[0].Label.Value != "two" {
					t.Fatalf("expected g.Edges[0].Label to be two: %#v", g.Edges[0].Label)
				}
			},
		},
		{
			name: "markdown_ampersand",
			text: `memo: |md
  <a href="https://www.google.com/search?q=d2&newwindow=1&amp;bar">d2</a>
|
`,
		},
		{
			name: "unsemantic_markdown",

			text: `test:|
foobar
<p>
|
`,
			expErr: `d2/testdata/d2compiler/TestCompile/unsemantic_markdown.d2:1:1: malformed Markdown: element <p> closed by </div>`,
		},
		{
			name: "unsemantic_markdown_2",

			text: `test:|
foo<br>
bar
|
`,
			expErr: `d2/testdata/d2compiler/TestCompile/unsemantic_markdown_2.d2:1:1: malformed Markdown: element <br> closed by </p>`,
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
				if g.Edges[0].Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Label.Value != "asdf" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				assert.String(t, "", g.Edges[0].Shape.Value)
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
			name: "edge_arrowhead_primary",

			text: `x -> y: {
  source-arrowhead: Reisner's Rule of Conceptual Inertia
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "Reisner's Rule of Conceptual Inertia", g.Edges[0].SrcArrowhead.Label.Value)
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
				assert.String(t, "", g.Edges[0].Shape.Value)
				assert.String(t, "", g.Edges[0].Label.Value)
				assert.JSON(t, nil, g.Edges[0].Style.Filled)
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
				assert.String(t, "", g.Edges[0].Shape.Value)
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
				assert.String(t, "", g.Edges[0].Shape.Value)
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
				assert.String(t, "", g.Edges[0].Label.Value)
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
				assert.String(t, "", g.Edges[0].Shape.Value)
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
				assert.String(t, "", g.Edges[0].Shape.Value)
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
				if g.Edges[0].Style.Animated.Value != "true" {
					t.Fatalf("Edges[0].Style.Animated.Value: %#v", g.Edges[0].Style.Animated.Value)
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
		},
		{
			name: "edge_invalid_style",

			text: `x -> y: {
  opacity: 0.5
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/edge_invalid_style.d2:2:3: opacity must be style.opacity`,
		},
		{
			name: "obj_invalid_style",

			text: `x: {
  opacity: 0.5
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/obj_invalid_style.d2:2:3: opacity must be style.opacity`,
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
				if g.Edges[0].Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
				}
				if g.Edges[1].Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[1].Label.Value)
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
				if g.Edges[0].Label.Value != "Space: the final frontier.  These are the voyages of the starship Enterprise." {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
				}
				if g.Edges[0].Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
				}
				if g.Edges[0].Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
				}
				if g.Edges[0].Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
				}
				if g.Edges[0].Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
				}
				if g.Edges[0].Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
				}
				if g.Edges[0].Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Edges[0].Style.Opacity.Value != "0.4" {
					t.Fatalf("unexpected g.Edges[0].Style.Opacity.Value: %#v", g.Edges[0].Style.Opacity.Value)
				}
				if g.Edges[0].Label.Value != "" {
					t.Fatalf("unexpected g.Edges[0].Label.Value : %#v", g.Edges[0].Label.Value)
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
				if g.Objects[0].Link.Value != "https://google.com" {
					t.Fatal(g.Objects[0].Link.Value)
				}
			},
		},
		{
			name: "non_url_link",

			text: `x: {
  link: vscode://file//Users/pmoura/logtalk/examples/searching/hill_climbing1.lgt:35:0
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].Link.Value != "vscode://file//Users/pmoura/logtalk/examples/searching/hill_climbing1.lgt:35:0" {
					t.Fatal(g.Objects[0].Link.Value)
				}
			},
		},
		{
			name: "glob-connection-steps",

			text: `*.style.stroke: black

layers: {
  ok: @ok
}
`,
			files: map[string]string{
				"ok.d2": `
steps: {
  1: {
    step1
  }
}
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.Equal(t, 0, len(g.Steps))
				assert.Equal(t, 1, len(g.Layers))
				assert.Equal(t, 1, len(g.Layers[0].Steps))
			},
		},
		{
			name: "import_url_link",

			text: `...@test
`,
			files: map[string]string{
				"test.d2": `elem: elem {
  link: https://google.com
}`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].Link.Value != "https://google.com" {
					t.Fatal(g.Objects[0].Link.Value)
				}
			},
		},
		{
			name: "nested-scope-1",

			text: `...@second
`,
			files: map[string]string{
				"second.d2": `second: {
  ...@third
}`,
				"third.d2": `third: {
  elem
}`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.Equal(t, 3, len(g.Objects))
			},
		},
		{
			name: "nested-scope-2",

			text: `...@second
a.style.fill: null
`,
			files: map[string]string{
				"second.d2": `a.style.fill: red`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.Equal(t, 1, len(g.Objects))
			},
		},
		{
			name: "url_tooltip",
			text: `x: {tooltip: https://google.com}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}

				if g.Objects[0].Tooltip.Value != "https://google.com" {
					t.Fatal(g.Objects[0].Tooltip.Value)
				}
			},
		},
		{
			name:   "no_url_link_and_url_tooltip_concurrently",
			text:   `x: {link: https://not-google.com; tooltip: https://google.com}`,
			expErr: `d2/testdata/d2compiler/TestCompile/no_url_link_and_url_tooltip_concurrently.d2:1:44: Tooltip cannot be set to URL when link is also set (for security)`,
		},
		{
			name:   "url_link_non_url_tooltip_ok",
			text:   `x: {link: https://not-google.com; tooltip: note: url.ParseRequestURI might see this as a URL}`,
			expErr: ``,
		},
		{
			name: "url_link_and_not_url_tooltip_concurrently",
			text: `x: {link: https://google.com; tooltip: hello world}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Objects) != 1 {
					t.Fatal(g.Objects)
				}
				if g.Objects[0].Link.Value != "https://google.com" {
					t.Fatal(g.Objects[0].Link.Value)
				}

				if g.Objects[0].Tooltip.Value != "hello world" {
					t.Fatal(g.Objects[0].Tooltip.Value)
				}
			},
		},
		{
			name:   "no_url_link_and_path_url_label_concurrently",
			text:   `x -> y: https://google.com {link: https://not-google.com }`,
			expErr: `d2/testdata/d2compiler/TestCompile/no_url_link_and_path_url_label_concurrently.d2:1:35: Label cannot be set to URL when link is also set (for security)`,
		},
		{
			name: "url_link_and_path_url_label_concurrently",
			text: `x -> y: hello world {link: https://google.com}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				if len(g.Edges) != 1 {
					t.Fatal(len(g.Edges))
				}
				if g.Edges[0].Link.Value != "https://google.com" {
					t.Fatal(g.Edges[0].Link.Value)
				}

				if g.Edges[0].Label.Value != "hello world" {
					t.Fatal(g.Edges[0].Label.Value)
				}
			},
		},
		{
			name: "nil_scope_obj_regression",

			text: `a
b: {
  _.a
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "a", g.Objects[0].ID)
				for _, ref := range g.Objects[0].References {
					tassert.NotNil(t, ref.ScopeObj)
				}
			},
		},
		{
			name: "near_constant",

			text: `x.near: top-center
`,
		},
		{
			name: "near-invalid",

			text: `mongodb: MongoDB {
  perspective: perspective (View) {
    password
  }

  explanation: |md
    perspective.model.js
  | {
    near: mongodb
  }
}

a: {
  near: a.b
  b
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near-invalid.d2:9:11: near keys cannot be set to an ancestor
d2/testdata/d2compiler/TestCompile/near-invalid.d2:14:9: near keys cannot be set to an descendant`,
		},
		{
			name: "near_bad_constant",

			text: `x.near: txop-center
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near_bad_constant.d2:1:9: near key "txop-center" must be the absolute path to a shape or one of the following constants: top-left, top-center, top-right, center-left, center-right, bottom-left, bottom-center, bottom-right`,
		},
		{
			name: "near_special",

			text: `x.near: z.x
z: {
  grid-rows: 1
  x
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near_special.d2:1:9: near keys cannot be set to descendants of special objects, like grid cells`,
		},
		{
			name: "near_bad_connected",

			text: `
				x: {
					near: top-center
				}
				x -> y
			`,
			expErr: ``,
		},
		{
			name: "near_descendant_connect_to_outside",
			text: `
				x: {
					near: top-left
					y
				}
				x.y -> z
			`,
			expErr: "",
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
				if g.Objects[0].NearKey == nil {
					t.Fatal("missing near key")
				}
				if g.Objects[0].Icon.Path != "orange" {
					t.Fatal(g.Objects[0].Icon)
				}
				if g.Objects[0].Style.Opacity.Value != "0.5" {
					t.Fatal(g.Objects[0].Style.Opacity)
				}
				if g.Objects[0].Style.Stroke.Value != "red" {
					t.Fatal(g.Objects[0].Style.Stroke)
				}
				if g.Objects[0].Style.Fill.Value != "green" {
					t.Fatal(g.Objects[0].Style.Fill)
				}
			},
		},
		{
			name: "reserved_quoted/1",
			text: `x: {
  "label": hello
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.Equal(t, 2, len(g.Objects))
				assert.Equal(t, "x", g.Objects[0].Label.Value)
			},
		},
		{
			name: "reserved_quoted/2",
			text: `my_table: {
  shape: sql_table
  width: 200
  height: 200
  "shape": string
  "icon": string
  "width": int
  "height": int
}
		`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.Equal(t, 4, len(g.Objects[0].SQLTable.Columns))
				assert.Equal(t, `shape`, g.Objects[0].SQLTable.Columns[0].Name.Label)
			},
		},
		{
			name: "reserved_quoted/3",
			text: `*."shape"
x
		`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.Equal(t, 2, len(g.Objects))
				assert.Equal(t, `x.shape`, g.Objects[0].AbsID())
			},
		},
		{
			name: "reserved_quoted/4",
			text: `x."style"."fill"`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.Equal(t, 3, len(g.Objects))
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
			expErr: `d2/testdata/d2compiler/TestCompile/edge_in_column.d2:3:7: sql_table columns cannot have children
d2/testdata/d2compiler/TestCompile/edge_in_column.d2:3:12: sql_table columns cannot have children`,
		},
		{
			name: "no-nested-columns-sql",

			text: `x: {
  shape: sql_table
  a -- b.b
}`,
			expErr: `d2/testdata/d2compiler/TestCompile/no-nested-columns-sql.d2:3:10: sql_table columns cannot have children`,
		},
		{
			name: "no-nested-columns-sql-2",

			text: `x: {
  shape: sql_table
  a
}
x.a.b`,
			expErr: `d2/testdata/d2compiler/TestCompile/no-nested-columns-sql-2.d2:5:5: sql_table columns cannot have children`,
		},
		{
			name: "no-nested-columns-class",

			text: `x: {
  shape: class
  a.a
}`,
			expErr: `d2/testdata/d2compiler/TestCompile/no-nested-columns-class.d2:3:5: class fields cannot have children`,
		},
		{
			name: "improper-class-ref",

			text:   `myobj.class.style.stroke-dash: 3`,
			expErr: `d2/testdata/d2compiler/TestCompile/improper-class-ref.d2:1:7: "class" must be the last part of the key`,
		},
		{
			name: "tail-style",

			text:   `myobj.style: 3`,
			expErr: `d2/testdata/d2compiler/TestCompile/tail-style.d2:1:7: "style" expected to be set to a map of key-values, or contain an additional keyword like "style.opacity: 0.4"`,
		},
		{
			name: "tail-style-map",

			text:   `myobj.style: {}`,
			expErr: `d2/testdata/d2compiler/TestCompile/tail-style-map.d2:1:7: "style" expected to be set to a map of key-values, or contain an additional keyword like "style.opacity: 0.4"`,
		},
		{
			name: "bad-style-nesting",

			text:   `myobj.style.style.stroke-dash: 3`,
			expErr: `d2/testdata/d2compiler/TestCompile/bad-style-nesting.d2:1:13: invalid style keyword: "style"`,
		},
		{
			name: "edge_to_style",

			text: `x: {style.opacity: 0.4}
y -> x.style
`,
			expErr: `d2/testdata/d2compiler/TestCompile/edge_to_style.d2:2:8: reserved keywords are prohibited in edges`,
		},
		{
			name: "keyword-container",

			text: `a.near.b
`,
			expErr: `d2/testdata/d2compiler/TestCompile/keyword-container.d2:1:3: "near" must be the last part of the key`,
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
b`, g.Objects[0].Label.Value)
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
				assert.String(t, "b\rb", g.Objects[0].Label.Value)
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
				if g.Objects[0].Style.Opacity.Value != "0.4" {
					t.Fatal(g.Objects[0].Style.Opacity.Value)
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
				if g.Objects[0].Style.Opacity.Value != "0.4" {
					t.Fatal(g.Objects[0].Style.Opacity.Value)
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
				if g.Objects[0].Style.Opacity.Value != "0.4" {
					t.Fatal(g.Objects[0].Style.Opacity.Value)
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
				tassert.Equal(t, "true", g.Edges[0].Style.Animated.Value)
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
			expErr: `d2/testdata/d2compiler/TestCompile/3d_oval.d2:2:1: key "3d" can only be applied to squares, rectangles, and hexagons`,
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
				assert.String(t, "sequence_diagram", g.Objects[0].Shape.Value)
			},
		},
		{
			name: "near_sequence",

			text: `x: {
  shape: sequence_diagram
  a
}
b.near: x.a
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near_sequence.d2:5:9: near keys cannot be set to an object within sequence diagrams`,
		},
		{
			name: "sequence-timestamp",

			text: `shape: sequence_diagram
a
b

"04:20,11:20": {
  "loop through each table": {
    a."start_time = datetime.datetime.now"
    a -> b
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 1, len(g.Edges))
				tassert.Equal(t, 5, len(g.Objects))
				tassert.Equal(t, "a", g.Objects[0].ID)
				tassert.Equal(t, "b", g.Objects[1].ID)
				tassert.Equal(t, `"04:20,11:20"`, g.Objects[2].ID)
				tassert.Equal(t, `loop through each table`, g.Objects[3].ID)
				tassert.Equal(t, 1, len(g.Objects[0].ChildrenArray))
				tassert.Equal(t, 0, len(g.Objects[1].ChildrenArray))
				tassert.Equal(t, 1, len(g.Objects[2].ChildrenArray))
				tassert.True(t, g.Edges[0].ContainedBy(g.Objects[3]))
			},
		},
		{
			name: "root_sequence",

			text: `shape: sequence_diagram
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "sequence_diagram", g.Root.Shape.Value)
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
			expErr: ``,
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
				assert.String(t, "right", g.Root.Direction.Value)
			},
		},
		{
			name: "default_direction",

			text: `x`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "", g.Objects[0].Direction.Value)
			},
		},
		{
			name: "set_direction",

			text: `x: {
  direction: left
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				assert.String(t, "left", g.Objects[0].Direction.Value)
			},
		},
		{
			name: "constraint_label",

			text: `foo {
  label: bar
  constraint: BIZ
}`,
			expErr: `d2/testdata/d2compiler/TestCompile/constraint_label.d2:3:3: "constraint" keyword can only be used in "sql_table" shapes`,
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
			name: "sql-constraints",
			text: `x: {
  shape: sql_table
  a: int {constraint: primary_key}
  b: int {constraint: [primary_key; foreign_key]}
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				table := g.Objects[0].SQLTable
				tassert.Equal(t, []string{"primary_key"}, table.Columns[0].Constraint)
				tassert.Equal(t, []string{"primary_key", "foreign_key"}, table.Columns[1].Constraint)
			},
		},
		{
			name: "sql-null-constraint",
			text: `x: {
  shape: sql_table
  a: int {constraint: null}
  b: int {constraint: [null]}
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				table := g.Objects[0].SQLTable
				tassert.Nil(t, table.Columns[0].Constraint)
				tassert.Equal(t, []string{"null"}, table.Columns[1].Constraint)
			},
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
		{
			name: "link-board-ok",
			text: `x.link: layers.x
layers: {
	x: {
	  y
	}
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.x", g.Objects[0].Link.Value)
			},
		},
		{
			name: "link-board-mixed",
			text: `question: How does the cat go?
question.link: layers.cat

layers: {
  cat: {
    the cat -> meeeowwww: goes
  }
}

scenarios: {
  green: {
    question.style.fill: green
  }
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.cat", g.Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.cat", g.Scenarios[0].Objects[0].Link.Value)
			},
		},
		{
			name: "no-self-link",
			text: `
x.link: scenarios.a

layers: {
  g: {
    s.link: _.layers.g
  }
}

scenarios: {
  a: {
    b
  }
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Scenarios[0].Objects[0].Link)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Layers[0].Objects[0].Link)
			},
		},
		{
			name: "link-board-not-found-1",
			text: `x.link: layers.x
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Link)
			},
		},
		{
			name: "link-board-not-found-2",
			text: `layers: {
    one: {
        ping: {
            link: two
        }
    }
    two: {
        pong: {
            link: one
        }
    }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Layers[0].Objects[0].Link)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Layers[1].Objects[0].Link)
			},
		},
		{
			name: "link-board-not-board",
			text: `zzz
x.link: layers.x.y
layers: {
  x: {
    y
  }
}`,

			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Link)
			},
		},
		{
			name: "link-board-nested",
			text: `x.link: layers.x.layers.x
layers: {
  x: {
    layers: {
      x: {
        hello
      }
    }
  }
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.x.layers.x", g.Objects[0].Link.Value)
			},
		},
		{
			name: "link-board-key-nested",
			text: `x: {
  y.link: layers.x
}
layers: {
  x: {
    yo
  }
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.x", g.Objects[1].Link.Value)
			},
		},
		{
			name: "link-board-underscore",
			text: `x
layers: {
	x: {
	  yo
    layers: {
      x: {
        hello.link: _._.layers.x
        hey.link: _
      }
    }
  }
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.NotNil(t, g.Layers[0].Layers[0].Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.x", g.Layers[0].Layers[0].Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.x", g.Layers[0].Layers[0].Objects[1].Link.Value)
			},
		},
		{
			name: "link-file-underscore",
			text: `...@x`,
			files: map[string]string{
				"x.d2": `x

layers: {
  a: { c }
  b: { d.link: _.layers.a }
	e: {
    l

		layers: {
			j: {
			  k.link: _
			  n.link: _._
			  m.link: _._.layers.a
			}
		}
  }
}
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.a", g.Layers[1].Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.e", g.Layers[2].Layers[0].Objects[0].Link.Value)
				tassert.Equal(t, "root", g.Layers[2].Layers[0].Objects[1].Link.Value)
				tassert.Equal(t, "root.layers.a", g.Layers[2].Layers[0].Objects[2].Link.Value)
			},
		},
		{
			name: "link-beyond-import-root",
			text: `...@x`,
			files: map[string]string{
				"x.d2": `x.link: _.layers.z
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Link)
			},
		},
		{
			name: "link-board-underscore-not-found",
			text: `x
layers: {
  x: {
    yo
    layers: {
      x: {
        hello.link: _._._
      }
    }
  }
}`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Layers[0].Layers[0].Objects[0].Link)
			},
		},
		{
			name: "import-icon-near",
			text: `y: @y
`,
			files: map[string]string{
				"y.d2": `syslog
*.icon.near: center-left
`,
			},
		},
		{
			name: "border-radius-negative",
			text: `x
x: {
  style.border-radius: -1
}`,
			expErr: `d2/testdata/d2compiler/TestCompile/border-radius-negative.d2:3:24: expected "border-radius" to be a number greater or equal to 0`,
		},
		{
			name: "text-transform",
			text: `direction: right
x -> y: hi {
  style: {
    text-transform: capitalize
  }
}
x.style.text-transform: uppercase
y.style.text-transform: lowercase`,
		},
		{
			name: "near_near_const",
			text: `
title: Title {
	near: top-center
}

obj {
	near: title
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/near_near_const.d2:7:8: near keys cannot be set to an object with a constant near key`,
		},
		{
			name: "label-near-parent",
			text: `hey: sushi {
	label.near: outside-top-left
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "sushi", g.Objects[0].Attributes.Label.Value)
				tassert.Equal(t, "outside-top-left", g.Objects[0].Attributes.LabelPosition.Value)
			},
		},
		{
			name: "label-near-composite-separate",
			text: `hey: {
	label: sushi
	label.near: outside-top-left
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "sushi", g.Objects[0].Attributes.Label.Value)
				tassert.Equal(t, "outside-top-left", g.Objects[0].Attributes.LabelPosition.Value)
			},
		},
		{
			name: "label-near-composite-together",
			text: `hey: {
  label: sushi {
		near: outside-top-left
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "sushi", g.Objects[0].Attributes.Label.Value)
				tassert.Equal(t, "outside-top-left", g.Objects[0].Attributes.LabelPosition.Value)
			},
		},
		{
			name: "icon-near-composite-together",
			text: `hey: {
	icon: https://asdf.com {
		near: outside-top-left
  }
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "asdf.com", g.Objects[0].Attributes.Icon.Host)
				tassert.Equal(t, "outside-top-left", g.Objects[0].Attributes.IconPosition.Value)
			},
		},
		{
			name: "label-near-invalid-edge",
			text: `hey: {
  label: sushi {
		near: outside-top-left
		a -> b
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/label-near-invalid-edge.d2:2:3: unexpected edges in map`,
		},
		{
			name: "label-near-invalid-field",
			text: `hey: {
  label: sushi {
		near: outside-top-left
		a
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/label-near-invalid-field.d2:4:3: unexpected field a`,
		},
		{
			name: "grid",
			text: `hey: {
	grid-rows: 200
	grid-columns: 230
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "200", g.Objects[0].GridRows.Value)
			},
		},
		{
			name: "grid_negative",
			text: `hey: {
	grid-rows: 200
	grid-columns: -200
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/grid_negative.d2:3:16: grid-columns must be a positive integer: "-200"`,
		},
		{
			name: "grid_gap_negative",
			text: `hey: {
	horizontal-gap: -200
	vertical-gap: -30
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/grid_gap_negative.d2:2:18: horizontal-gap must be a non-negative integer: "-200"
d2/testdata/d2compiler/TestCompile/grid_gap_negative.d2:3:16: vertical-gap must be a non-negative integer: "-30"`,
		},
		{
			name: "grid_edge",
			text: `hey: {
	grid-rows: 1
	a -> b: ok
}
c -> hey.b
hey.a -> c
hey -> hey.a

hey -> c: ok
`,
			expErr: `d2/testdata/d2compiler/TestCompile/grid_edge.d2:7:1: edge from grid diagram "hey" cannot enter itself`,
		},
		{
			name: "grid_deeper_edge",
			text: `hey: {
	grid-rows: 1
	a -> b: ok
	b: {
		c -> d: ok now
		c.e -> c.f.g: ok
		c.e -> d.h: ok
		c -> d.h: ok
	}
	a: {
		grid-columns: 1
		e -> f: also ok now
		e: {
			g -> h: ok
			g -> h.h: ok
		}
		e -> f.i: ok now
		e.g -> f.i: ok now
	}
	a -> b.c: ok now
	a.e -> b.c: ok now
	a -> a.e: not ok
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/grid_deeper_edge.d2:22:2: edge from grid diagram "hey.a" cannot enter itself`,
		},
		{
			name: "parent_graph_edge_to_descendant",
			text: `tl: {
	near: top-left
	a.b
}
grid: {
	grid-rows: 1
	cell.c.d
}
seq: {
	shape: sequence_diagram
	e.f
}
tl -> tl.a: no
tl -> tl.a.b: no
grid-> grid.cell: no
grid-> grid.cell.c: no
grid.cell -> grid.cell.c: no
grid.cell -> grid.cell.c.d: no
seq -> seq.e: no
seq -> seq.e.f: no
`,
			expErr: `d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:13:1: edge from constant near "tl" cannot enter itself
d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:14:1: edge from constant near "tl" cannot enter itself
d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:17:1: edge from grid cell "grid.cell" cannot enter itself
d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:18:1: edge from grid cell "grid.cell" cannot enter itself
d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:15:1: edge from grid diagram "grid" cannot enter itself
d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:16:1: edge from grid diagram "grid" cannot enter itself
d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:19:1: edge from sequence diagram "seq" cannot enter itself
d2/testdata/d2compiler/TestCompile/parent_graph_edge_to_descendant.d2:20:1: edge from sequence diagram "seq" cannot enter itself`,
		},
		{
			name: "grid_nested",
			text: `hey: {
	grid-rows: 200
	grid-columns: 200

	a
	b
	c
	d.valid descendant
	e: {
		grid-rows: 1
		grid-columns: 2

		a
		b
	}
}
`,
			expErr: ``,
		},
		{
			name: "classes",
			text: `classes: {
  dragon_ball: {
    label: ""
    shape: circle
    style.fill: orange
  }
  path: {
    label: "then"
    style.stroke-width: 4
  }
}
nostar: { class: dragon_ball }
1star: "*" { class: dragon_ball; style.fill: red }
2star: { label: "**"; class: dragon_ball }

nostar -> 1star: { class: path }
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 3, len(g.Objects))
				tassert.Equal(t, "dragon_ball", g.Objects[0].Classes[0])
				tassert.Equal(t, "", g.Objects[0].Label.Value)
				// Class field overrides primary
				tassert.Equal(t, "", g.Objects[1].Label.Value)
				tassert.Equal(t, "**", g.Objects[2].Label.Value)
				tassert.Equal(t, "orange", g.Objects[0].Style.Fill.Value)
				tassert.Equal(t, "red", g.Objects[1].Style.Fill.Value)

				tassert.Equal(t, "4", g.Edges[0].Style.StrokeWidth.Value)
				tassert.Equal(t, "then", g.Edges[0].Label.Value)
			},
		},
		{
			name: "array-classes",
			text: `classes: {
  dragon_ball: {
    label: ""
    shape: circle
    style.fill: orange
  }
  path: {
    label: "then"
    style.stroke-width: 4
  }
	path2: {
    style.stroke-width: 2
	}
}
nostar: { class: [dragon_ball; path] }
1star: { class: [path; dragon_ball] }

nostar -> 1star: { class: [path; path2] }
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "then", g.Objects[0].Label.Value)
				tassert.Equal(t, "", g.Objects[1].Label.Value)
				tassert.Equal(t, "circle", g.Objects[0].Shape.Value)
				tassert.Equal(t, "circle", g.Objects[1].Shape.Value)
				tassert.Equal(t, "2", g.Edges[0].Style.StrokeWidth.Value)
			},
		},
		{
			name: "comma-array-class",

			text: `classes: {
  dragon_ball: {
    label: ""
    shape: circle
    style.fill: orange
  }
  path: {
    label: "then"
    style.stroke-width: 4
  }
}
nostar: { class: [dragon_ball, path] }`,
			expErr: `d2/testdata/d2compiler/TestCompile/comma-array-class.d2:12:11: class "dragon_ball, path" not found. Did you mean to use ";" to separate array items?`,
		},
		{
			name: "reordered-classes",
			text: `classes: {
  x: {
    shape: circle
  }
}
a.class: x
classes.x.shape: diamond
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 1, len(g.Objects))
				tassert.Equal(t, "diamond", g.Objects[0].Shape.Value)
			},
		},
		{
			name: "nested-array-classes",
			text: `classes: {
  one target: {
		target-arrowhead.label: 1
  }
	association: {
		target-arrowhead.shape: arrow
	}
}

a -> b: { class: [one target; association] }
a -> b: { class: [association; one target] }
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				// They have the same, regardless of order of class application
				// since the classes modify attributes exclusive of each other
				tassert.Equal(t, "1", g.Edges[0].DstArrowhead.Label.Value)
				tassert.Equal(t, "1", g.Edges[1].DstArrowhead.Label.Value)
				tassert.Equal(t, "arrow", g.Edges[0].DstArrowhead.Shape.Value)
				tassert.Equal(t, "arrow", g.Edges[1].DstArrowhead.Shape.Value)
			},
		},
		{
			name: "var_in_glob",
			text: `vars: {
  v: {
    ok
  }
}

x1 -> x2

x*: {
  ...${v}
}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, 4, len(g.Objects))
				tassert.Equal(t, "x1", g.Objects[0].AbsID())
				tassert.Equal(t, "x1.ok", g.Objects[1].AbsID())
				tassert.Equal(t, "x2", g.Objects[2].AbsID())
				tassert.Equal(t, "x2.ok", g.Objects[3].AbsID())
			},
		},
		{
			name: "var_in_markdown",
			text: `vars: {
  v: ok
}

x: |md
  m${v}y

	` + "`hey ${v}`" + `

	regular markdown

	` + "```" + `
	bye ${v}
	` + "```" + `
|
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.True(t, strings.Contains(g.Objects[0].Attributes.Label.Value, "moky"))
				tassert.False(t, strings.Contains(g.Objects[0].Attributes.Label.Value, "m${v}y"))
				// Code spans untouched
				tassert.True(t, strings.Contains(g.Objects[0].Attributes.Label.Value, "hey ${v}"))
				// Code blocks untouched
				tassert.True(t, strings.Contains(g.Objects[0].Attributes.Label.Value, "bye ${v}"))
			},
		},
		{
			name: "var_in_vars",
			text: `vars: {
    Apple: {
        shape:circle
        label:Granny Smith
    }
    Cherry: {
        shape:circle
        label:Rainier Cherry
    }
    SummerFruit: {
        xx: ${Apple}
        cc: ${Cherry}
        xx -> cc
    }
}

x: ${Apple}
c: ${Cherry}
sf: ${SummerFruit}
`,
		},
		{
			name: "spread_var_order",
			text: `vars: {
  before_elem: {
    "before_elem"
  }
  after_elem: {
    "after_elem"
  }
}

...${before_elem}
elem
...${after_elem}
`,
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "before_elem", g.Objects[0].AbsID())
				tassert.Equal(t, "elem", g.Objects[1].AbsID())
				tassert.Equal(t, "after_elem", g.Objects[2].AbsID())
			},
		},
		{
			name: "class-shape-class",
			text: `classes: {
  classClass: {
    shape: class
  }
}

object: {
  class: classClass
  length(): int
}
`,
		},
		{
			name: "no-class-primary",
			text: `x.class
`,
			expErr: `d2/testdata/d2compiler/TestCompile/no-class-primary.d2:1:3: reserved field "class" must have a value`,
		},
		{
			name: "no-class-inside-classes",
			text: `classes: {
  x: {
    class: y
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/no-class-inside-classes.d2:3:5: "class" cannot appear within "classes"`,
		},
		{
			// This is okay
			name: "missing-class",
			text: `x.class: yo
`,
		},
		{
			name: "classes-unreserved",
			text: `classes: {
  mango: {
    seed
  }
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/classes-unreserved.d2:3:5: seed is an invalid class field, must be reserved keyword`,
		},
		{
			name: "classes-internal-edge",
			text: `classes: {
  mango: {
		width: 100
  }
  jango: {
    height: 100
  }
  mango -> jango
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/classes-internal-edge.d2:8:3: classes cannot contain an edge`,
		},
		{
			name: "reserved-composite",
			text: `shape: sequence_diagram {
  alice -> bob: What does it mean\nto be well-adjusted?
  bob -> alice: The ability to play bridge or\ngolf as if they were games.
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/reserved-composite.d2:1:1: reserved field shape does not accept composite`,
		},
		{
			name: "text_no_label",
			text: `a: "ok" {
	shape: text
}
b: " \n " {
	shape: text
}
c: "" {
	shape: text
}
d: "" {
	shape: circle
}
e: " \n "
f: |md  |
g: |md

|
`,
			expErr: `d2/testdata/d2compiler/TestCompile/text_no_label.d2:14:1: block string cannot be empty
d2/testdata/d2compiler/TestCompile/text_no_label.d2:15:1: block string cannot be empty
d2/testdata/d2compiler/TestCompile/text_no_label.d2:4:1: shape text must have a non-empty label
d2/testdata/d2compiler/TestCompile/text_no_label.d2:7:1: shape text must have a non-empty label`,
		},
		{
			name: "var-not-color",
			text: `vars: {
  d2-config: {
    theme-overrides: {
      B1: potato
			potato: B1
    }
  }
}
a
`,
			expErr: `d2/testdata/d2compiler/TestCompile/var-not-color.d2:4:7: expected "B1" to be a valid named color ("orange") or a hex code ("#f0ff3a")
d2/testdata/d2compiler/TestCompile/var-not-color.d2:5:4: "potato" is not a valid theme code`,
		},
		{
			name: "no_arrowheads_in_shape",

			text: `x.target-arrowhead.shape: cf-one
y.source-arrowhead.shape: cf-one
`,
			expErr: `d2/testdata/d2compiler/TestCompile/no_arrowheads_in_shape.d2:1:3: "target-arrowhead" can only be used on connections
d2/testdata/d2compiler/TestCompile/no_arrowheads_in_shape.d2:2:3: "source-arrowhead" can only be used on connections`,
		},
		{
			name: "shape-hierarchy",
			text: `x: {
  shape: hierarchy
  a -> b
}
`,
		},
		{
			name: "fixed-pos-shape-hierarchy",
			text: `x: {
  shape: hierarchy
  a -> b
	a.top: 20
	a.left: 20
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/fixed-pos-shape-hierarchy.d2:4:2: position keywords cannot be used with shape "hierarchy"
d2/testdata/d2compiler/TestCompile/fixed-pos-shape-hierarchy.d2:5:2: position keywords cannot be used with shape "hierarchy"`,
		},
		{
			name: "vars-in-imports",
			text: `dev: {
  vars: {
    env: Dev
  }
  ...@template.d2
}

qa: {
  vars: {
    env: Qa
  }
  ...@template.d2
}
`,
			files: map[string]string{
				"template.d2": `env: {
  label: ${env} Environment
  vm: {
    label: My Virtual machine!
  }
}`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "dev.env", g.Objects[1].AbsID())
				tassert.Equal(t, "Dev Environment", g.Objects[1].Label.Value)
				tassert.Equal(t, "qa.env", g.Objects[2].AbsID())
				tassert.Equal(t, "Qa Environment", g.Objects[2].Label.Value)
			},
		},
		{
			name: "spread-import-link",
			text: `k

layers: {
  x: {...@x}
}`,
			files: map[string]string{
				"x.d2": `a.link: layers.b
layers: {
  b: {
    d
  }
}`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.x.layers.b", g.Layers[0].Objects[0].Link.Value)
			},
		},
		{
			name: "import-link-underscore-1",
			text: `k

layers: {
  x: {...@x}
}`,
			files: map[string]string{
				"x.d2": `a
layers: {
  b: {
    d.link: _
		s.link: _.layers.k

    layers: {
      c: {
        c.link: _
				z.link: _._
				f.link: _._.layers.b
      }
    }
  }
  k: {
    k
  }
}`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.x", g.Layers[0].Layers[0].Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.x.layers.b", g.Layers[0].Layers[0].Layers[0].Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.x", g.Layers[0].Layers[0].Layers[0].Objects[1].Link.Value)
				tassert.Equal(t, "root.layers.x.layers.b", g.Layers[0].Layers[0].Layers[0].Objects[2].Link.Value)
				tassert.Equal(t, "root.layers.x.layers.k", g.Layers[0].Layers[0].Objects[1].Link.Value)
			},
		},
		{
			name: "import-link-underscore-2",
			text: `k

layers: {
  x: @x
}`,
			files: map[string]string{
				"x.d2": `a
layers: {
  b: {
    d.link: _
		s.link: _.layers.k

    layers: {
      c: {
        c.link: _
				z.link: _._
				f.link: _._.layers.b
      }
    }
  }
  k: {
    k
  }
}`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.x", g.Layers[0].Layers[0].Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.x.layers.b", g.Layers[0].Layers[0].Layers[0].Objects[0].Link.Value)
				tassert.Equal(t, "root.layers.x", g.Layers[0].Layers[0].Layers[0].Objects[1].Link.Value)
				tassert.Equal(t, "root.layers.x.layers.b", g.Layers[0].Layers[0].Layers[0].Objects[2].Link.Value)
				tassert.Equal(t, "root.layers.x.layers.k", g.Layers[0].Layers[0].Objects[1].Link.Value)
			},
		},
		{
			name: "import-link-underscore-3",
			text: `k

layers: {
  x: @x
	b: {
    b
  }
}`,
			files: map[string]string{
				"x.d2": `a
layers: {
  y: @y
}`,
				"y.d2": `o.link: _._.layers.b
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.b", g.Layers[0].Layers[0].Objects[0].Link.Value)
			},
		},
		{
			name: "invalid-link-1",
			text: `k

layers: {
  x: {...@x}
}`,
			files: map[string]string{
				"x.d2": `k
layers: {
  y.link: @n
}`,
				"n.d2": `n`,
			},
			expErr: `d2/testdata/d2compiler/TestCompile/x.d2:3:5: a board itself cannot be linked; only objects within a board can be linked`,
		},
		{
			name: "invalid-link-2",
			text: `k

layers: {
  x: @x
}`,
			files: map[string]string{
				"x.d2": `k
layers: {
  y.link: @n
}`,
				"n.d2": `n`,
			},
			expErr: `d2/testdata/d2compiler/TestCompile/x.d2:3:5: a board itself cannot be linked; only objects within a board can be linked`,
		},
		{
			name: "import-nested-layers",
			text: `k

layers: {
  x: {...@x}
}`,
			files: map[string]string{
				"x.d2": `a
layers: {
  b: {
		d

		layers: {
		  c: {
			  c
			}
		}
  }
}`,
			},
		},
		{
			name: "multiple-import-nested-layers",
			text: `k

layers: {
  x: {...@y/x}
}`,
			files: map[string]string{
				"y/x.d2": `a.c.link: layers.b

layers: {
  b: {...@n}
}`,
				"y/n.d2": "p",
			},
		},
		{
			name: "import-link-layer-1",
			text: `k

layers: {
  x: {...@y}
  z: { hi }
}`,
			files: map[string]string{
				"y.d2": `a.link: _.layers.z
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.z", g.Layers[0].Objects[0].Link.Value)
			},
		},
		{
			name: "import-link-layer-2",
			text: `...@y

layers: {
  z: { hi }
}`,
			files: map[string]string{
				"y.d2": `a.link: layers.z
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.z", g.Objects[0].Link.Value)
			},
		},
		{
			name: "import-link-layer-3",
			text: `k

layers: {
  x: {...@y}
  z: { hi }
}`,
			files: map[string]string{
				"y.d2": `a
layers: {
  lol: {
    asdf.link: _._.layers.z
  }
}
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.z", g.Layers[0].Layers[0].Objects[0].Link.Value)
			},
		},
		{
			name: "import-link-layer-4",
			text: `k

layers: {
  x: @y
  z: { hi }
}`,
			files: map[string]string{
				"y.d2": `a
layers: {
  lol: {
    asdf.link: _.layers.z
  }
	z: { fjf }
}
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "root.layers.x.layers.z", g.Layers[0].Layers[0].Objects[0].Link.Value)
			},
		},
		{
			name: "sql-table-header-newline",
			text: `x: {
  shape: sql_table
  label: hello\nworld
}

y: "hello\nworld" {
  shape: sql_table
	hi: there
}
`,
			expErr: `d2/testdata/d2compiler/TestCompile/sql-table-header-newline.d2:3:3: shape sql_table cannot have newlines in label
d2/testdata/d2compiler/TestCompile/sql-table-header-newline.d2:6:1: shape sql_table cannot have newlines in label`,
		},
		{
			name: "sequence-diagram-icons",
			text: `shape: sequence_diagram
svc_1: {
  icon: https://icons.terrastruct.com/dev%2Fdocker.svg
  shape: image
}

a: A
b: B

svc_1.t1 -> a: do with A
svc_1."think about A"
svc_1.t2 -> b: do with B
`,
		},
		{
			name: "layer-import-nested-layer",
			text: `layers: {
	ok: {...@meow}
}
`,
			files: map[string]string{
				"meow.d2": `layers: {
  1: {
    asdf
  }
}
`,
			},
			assertions: func(t *testing.T, g *d2graph.Graph) {
				tassert.Equal(t, "d2/testdata/d2compiler/TestCompile/layer-import-nested-layer.d2", g.Layers[0].AST.Range.Path)
				tassert.Equal(t, "d2/testdata/d2compiler/TestCompile/meow.d2", g.Layers[0].Layers[0].AST.Range.Path)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := &d2compiler.CompileOptions{}
			if tc.files != nil {
				tc.files["index.d2"] = tc.text
				renamed := make(map[string]string)
				for file, content := range tc.files {
					renamed[fmt.Sprintf("d2/testdata/d2compiler/TestCompile/%v", file)] = content
				}
				fs, err := mapfs.New(renamed)
				assert.Success(t, err)
				t.Cleanup(func() {
					err = fs.Close()
					assert.Success(t, err)
				})
				opts.FS = fs
			}
			d2Path := fmt.Sprintf("d2/testdata/d2compiler/%v.d2", t.Name())
			g, _, err := d2compiler.Compile(d2Path, strings.NewReader(tc.text), opts)
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
	t.Run("nulls", testNulls)
	t.Run("vars", testVars)
	t.Run("globs", testGlobs)
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
				g, _ := assertCompile(t, `base

layers: {
  one: {
    santa
  }
  two: {
    clause
  }
}
`, "")
				assert.Equal(t, 2, len(g.Layers))
				assert.Equal(t, "one", g.Layers[0].Name)
				assert.Equal(t, "two", g.Layers[1].Name)
			},
		},
		{
			name: "recursive",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `base

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
			name: "isFolderOnly",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
layers: {
  one: {
    santa
  }
  two: {
    clause
		scenarios: {
			seinfeld: {
			}
			missoula: {
				steps: {
					missus: one two three
				}
			}
		}
  }
}
`, "")
				assert.True(t, g.IsFolderOnly)
				assert.Equal(t, 2, len(g.Layers))
				assert.Equal(t, "one", g.Layers[0].Name)
				assert.Equal(t, "two", g.Layers[1].Name)
				assert.Equal(t, 2, len(g.Layers[1].Scenarios))
				assert.False(t, g.Layers[1].Scenarios[0].IsFolderOnly)
				assert.False(t, g.Layers[1].Scenarios[1].IsFolderOnly)
			},
		},
		{
			name: "isFolderOnly-shapes",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
direction: right

steps: {
  1: {
    RJ
  }
}
`, "")
				assert.True(t, g.IsFolderOnly)
			},
		},
		{
			name: "board-label-primary",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `hi
layers: {
  1: one {
    RJ
  }
  2: {
    label: two
    RJ
  }
}
`, "")
				assert.Equal(t, "one", g.Layers[0].Root.Label.Value)
				assert.Equal(t, "two", g.Layers[1].Root.Label.Value)
			},
		},
		{
			name: "no-inherit-label",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
label: hi

steps: {
  1: {
    RJ
  }
}
`, "")
				assert.True(t, g.Root.Label.MapKey != nil)
				assert.True(t, g.Steps[0].Root.Label.MapKey == nil)
			},
		},
		{
			name: "scenarios_edge_index",
			run: func(t *testing.T) {
				assertCompile(t, `a -> x

scenarios: {
  1: {
    (a -> x)[0].style.opacity: 0.1
  }
}
`, "")
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
		{
			name: "style-nested-boards",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `**.style.stroke: black

scenarios: {
  a: {
    x
  }
  b: {
    x
  }
}
steps: {
  c: {
    x
  }
  d: {
    x
  }
}
layers: {
  e: {
    x
  }
}
`, ``)
				assert.Equal(t, "black", g.Scenarios[0].Objects[0].Style.Stroke.Value)
				assert.Equal(t, "black", g.Scenarios[1].Objects[0].Style.Stroke.Value)
				assert.Equal(t, "black", g.Steps[0].Objects[0].Style.Stroke.Value)
				assert.Equal(t, "black", g.Steps[1].Objects[0].Style.Stroke.Value)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Layers[0].Objects[0].Style.Stroke)
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

func testNulls(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "shape",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a
a: null
`, "")
					assert.Equal(t, 0, len(g.Objects))
				},
			},
			{
				name: "basic-edge",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a -> b
(a -> b)[0]: null
`, "")
					assert.Equal(t, 2, len(g.Objects))
					assert.Equal(t, 0, len(g.Edges))
				},
			},
			{
				name: "nested-edge",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a.b.c -> a.d.e
a.b.c -> a.d.e

a.(b.c -> d.e)[0]: null
(a.b.c -> a.d.e)[1]: null
`, "")
					assert.Equal(t, 5, len(g.Objects))
					assert.Equal(t, 0, len(g.Edges))
				},
			},
			{
				name: "attribute",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a.style.opacity: 0.2
a.style.opacity: null
			`, "")
					assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Attributes.Style.Opacity)
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

	t.Run("reappear", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "shape",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a
a: null
a
`, "")
					assert.Equal(t, 1, len(g.Objects))
				},
			},
			{
				name: "edge",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a -> b
(a -> b)[0]: null
a -> b
`, "")
					assert.Equal(t, 2, len(g.Objects))
					assert.Equal(t, 1, len(g.Edges))
				},
			},
			{
				name: "attribute-reset",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a.style.opacity: 0.2
a: null
a
`, "")
					assert.Equal(t, 1, len(g.Objects))
					assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Attributes.Style.Opacity)
				},
			},
			{
				name: "edge-reset",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a -> b
a: null
a
`, "")
					assert.Equal(t, 2, len(g.Objects))
					assert.Equal(t, 0, len(g.Edges))
				},
			},
			{
				name: "children-reset",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a.b.c
a.b: null
a.b
`, "")
					assert.Equal(t, 2, len(g.Objects))
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

	t.Run("implicit", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "delete-connection",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
x -> y
y: null
`, "")
					assert.Equal(t, 1, len(g.Objects))
					assert.Equal(t, 0, len(g.Edges))
				},
			},
			{
				name: "delete-nested-connection",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
a -> b.c
b.c: null
`, "")
					assert.Equal(t, 2, len(g.Objects))
					assert.Equal(t, 0, len(g.Edges))
				},
			},
			{
				name: "delete-multiple-connections",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
x -> y
z -> y
y -> a
y: null
`, "")
					assert.Equal(t, 3, len(g.Objects))
					assert.Equal(t, 0, len(g.Edges))
				},
			},
			{
				name: "no-delete-connection",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
y: null
x -> y
`, "")
					assert.Equal(t, 2, len(g.Objects))
					assert.Equal(t, 1, len(g.Edges))
				},
			},
			{
				name: "delete-children",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
x.y.z
a.b.c

x: null
a.b: null
`, "")
					assert.Equal(t, 1, len(g.Objects))
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

	t.Run("multiboard", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "scenario",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
x

scenarios: {
  a: {
    x: null
  }
}
`, "")
					assert.Equal(t, 0, len(g.Scenarios[0].Objects))
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

func testVars(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "shape-label",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}
hi: ${x}
`, "")
					assert.Equal(t, 1, len(g.Objects))
					assert.Equal(t, "im a var", g.Objects[0].Label.Value)
				},
			},
			{
				name: "style",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  primary-color: red
}
hi: {
  style.fill: ${primary-color}
}
`, "")
					assert.Equal(t, 1, len(g.Objects))
					assert.Equal(t, "red", g.Objects[0].Style.Fill.Value)
				},
			},
			{
				name: "number",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	columns: 2
}
hi: {
	grid-columns: ${columns}
	x
}
`, "")
					assert.Equal(t, "2", g.Objects[0].GridColumns.Value)
				},
			},
			{
				name: "nested",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	colors: {
    primary: {
      button: red
    }
  }
}
hi: {
  style.fill: ${colors.primary.button}
}
`, "")
					assert.Equal(t, "red", g.Objects[0].Style.Fill.Value)
				},
			},
			{
				name: "combined",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}
hi: 1 ${x} 2
`, "")
					assert.Equal(t, "1 im a var 2", g.Objects[0].Label.Value)
				},
			},
			{
				name: "double-quoted",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}
hi: "1 ${x} 2"
`, "")
					assert.Equal(t, "1 im a var 2", g.Objects[0].Label.Value)
				},
			},
			{
				name: "double-border",
				run: func(t *testing.T) {
					assertCompile(t, `
a.shape: Circle
a.style.double-border: true
`, "")
				},
			},
			{
				name: "invalid-double-border",
				run: func(t *testing.T) {
					assertCompile(t, `
a.shape: hexagon
a.style.double-border: true
`, `d2/testdata/d2compiler/TestCompile2/vars/basic/invalid-double-border.d2:3:1: key "double-border" can only be applied to squares, rectangles, circles, ovals`)
				},
			},
			{
				name: "single-quoted",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}
hi: '1 ${x} 2'
`, "")
					assert.Equal(t, "1 ${x} 2", g.Objects[0].Label.Value)
				},
			},
			{
				name: "edge-label",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}
a -> b: ${x}
`, "")
					assert.Equal(t, 1, len(g.Edges))
					assert.Equal(t, "im a var", g.Edges[0].Label.Value)
				},
			},
			{
				name: "edge-map",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}
a -> b: {
  target-arrowhead.label: ${x}
}
`, "")
					assert.Equal(t, 1, len(g.Edges))
					assert.Equal(t, "im a var", g.Edges[0].DstArrowhead.Label.Value)
				},
			},
			{
				name: "quoted-var",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  primaryColors: {
    button: {
      active: "#4baae5"
    }
  }
}

button: {
  style: {
    border-radius: 5
    fill: ${primaryColors.button.active}
  }
}
`, "")
					assert.Equal(t, `#4baae5`, g.Objects[0].Style.Fill.Value)
				},
			},
			{
				name: "quoted-var-quoted-sub",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: "hi"
}

y: "hey ${x}"
`, "")
					assert.Equal(t, `hey hi`, g.Objects[0].Label.Value)
				},
			},
			{
				name: "parent-scope",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im root var
}
a: {
  vars: {
    b: im nested var
  }
  hi: ${x}
}
`, "")
					assert.Equal(t, "im root var", g.Objects[1].Label.Value)
				},
			},
			{
				name: "map",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  cool-style: {
		fill: red
  }
  arrows: {
    target-arrowhead.label: yay
  }
}
hi.style: ${cool-style}
a -> b: ${arrows}
`, "")
					assert.Equal(t, "red", g.Objects[0].Style.Fill.Value)
					assert.Equal(t, "yay", g.Edges[0].DstArrowhead.Label.Value)
				},
			},
			{
				name: "primary-and-composite",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	x: all {
		a: b
  }
}
z: ${x}
`, "")
					assert.Equal(t, "z", g.Objects[0].ID)
					assert.Equal(t, "all", g.Objects[0].Label.Value)
					assert.Equal(t, 1, len(g.Objects[0].Children))
				},
			},
			{
				name: "spread",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	x: all {
		a: b
    b: c
  }
}
z: {
  ...${x}
  c
}
`, "")
					assert.Equal(t, "z", g.Objects[0].ID)
					assert.Equal(t, 4, len(g.Objects))
					assert.Equal(t, 3, len(g.Objects[0].Children))
				},
			},
			{
				name: "array",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	base-constraints: [UNQ; NOT NULL]
}
a: {
  shape: sql_table
	b: int {constraint: ${base-constraints}}
}
`, "")
					assert.Equal(t, "a", g.Objects[0].ID)
					assert.Equal(t, 2, len(g.Objects[0].SQLTable.Columns[0].Constraint))
				},
			},
			{
				name: "comment-array",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
  list: [
    "a";
    "b";
    "c";
    "d"
    # e
  ]
}

a
`, "")
				},
			},
			{
				name: "spread-array",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	base-constraints: [UNQ; NOT NULL]
}
a: {
  shape: sql_table
	b: int {constraint: [PK; ...${base-constraints}]}
}
`, "")
					assert.Equal(t, "a", g.Objects[0].ID)
					assert.Equal(t, 3, len(g.Objects[0].SQLTable.Columns[0].Constraint))
				},
			},
			{
				name: "sub-array",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	x: all
}
z.class: [a; ${x}]
`, "")
					assert.Equal(t, "z", g.Objects[0].ID)
					assert.Equal(t, "all", g.Objects[0].Attributes.Classes[1])
				},
			},
			{
				name: "multi-part-array",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	x: all
}
z.class: [a; ${x}together]
`, "")
					assert.Equal(t, "z", g.Objects[0].ID)
					assert.Equal(t, "alltogether", g.Objects[0].Attributes.Classes[1])
				},
			},
			{
				name: "double-quote-primary",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	x: always {
    a: b
  }
}
z: "${x} be my maybe"
`, "")
					assert.Equal(t, "z", g.Objects[0].ID)
					assert.Equal(t, "always be my maybe", g.Objects[0].Label.Value)
				},
			},
			{
				name: "spread-nested",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  disclaimer: {
    I am not a lawyer
  }
}
custom-disclaimer: DRAFT DISCLAIMER {
  ...${disclaimer}
}
`, "")
					assert.Equal(t, 2, len(g.Objects))
				},
			},
			{
				name: "spread-edge",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  connections: {
    x -> a
  }
}
hi: {
  ...${connections}
}
`, "")
					assert.Equal(t, 3, len(g.Objects))
					assert.Equal(t, 1, len(g.Edges))
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

	t.Run("override", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "label",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}
hi: ${x}
hi: not a var
`, "")
					assert.Equal(t, 1, len(g.Objects))
					assert.Equal(t, "not a var", g.Objects[0].Label.Value)
				},
			},
			{
				name: "map",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im root var
}
a: {
  vars: {
    x: im nested var
  }
  hi: ${x}
}
`, "")
					assert.Equal(t, "im nested var", g.Objects[1].Label.Value)
				},
			},
			{
				name: "var-in-var",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
	surname: Smith
}
a: {
  vars: {
		trade1: Black${surname}
		trade2: Metal${surname}
  }
  hi: ${trade1}
}
`, "")
					assert.Equal(t, "BlackSmith", g.Objects[1].Label.Value)
				},
			},
			{
				name: "recursive-var",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: a
}
hi: {
  vars: {
    x: ${x}-b
		b: ${x}
  }
  yo: ${x}
  hey: ${b}
}
`, "")
					assert.Equal(t, "a-b", g.Objects[1].Label.Value)
					assert.Equal(t, "a-b", g.Objects[2].Label.Value)
				},
			},
			{
				name: "null",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
	surname: Smith
}
a: {
  vars: {
		surname: null
  }
  hi: John ${surname}
}
`, `d2/testdata/d2compiler/TestCompile2/vars/override/null.d2:9:3: could not resolve variable "surname"`)
				},
			},
			{
				name: "nested-null",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
	surnames: {
    john: smith
  }
}
a: {
  vars: {
		surnames: {
      john: null
    }
  }
  hi: John ${surname}
}
`, `d2/testdata/d2compiler/TestCompile2/vars/override/nested-null.d2:13:3: could not resolve variable "surname"`)
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

	t.Run("boards", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "layer",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}

layers: {
  l: {
    hi: ${x}
  }
}
`, "")
					assert.Equal(t, 1, len(g.Layers[0].Objects))
					assert.Equal(t, "im a var", g.Layers[0].Objects[0].Label.Value)
				},
			},
			{
				name: "layer-2",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: root var x
  y: root var y
}

layers: {
  l: {
    vars: {
      x: layer var x
    }
    hi: ${x}
    hello: ${y}
  }
}
`, "")
					assert.Equal(t, "hi", g.Layers[0].Objects[0].ID)
					assert.Equal(t, "layer var x", g.Layers[0].Objects[0].Label.Value)
					assert.Equal(t, "hello", g.Layers[0].Objects[1].ID)
					assert.Equal(t, "root var y", g.Layers[0].Objects[1].Label.Value)
				},
			},
			{
				name: "scenario",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im a var
}

scenarios: {
  l: {
    hi: ${x}
  }
}
`, "")
					assert.Equal(t, 1, len(g.Scenarios[0].Objects))
					assert.Equal(t, "im a var", g.Scenarios[0].Objects[0].Label.Value)
				},
			},
			{
				name: "overlay",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im x var
}

scenarios: {
  l: {
    vars: {
      y: im y var
    }
    x: ${x}
    y: ${y}
  }
}
layers: {
  l2: {
    vars: {
      y: im y var
    }
    x: ${x}
    y: ${y}
  }
}
`, "")
					assert.Equal(t, 2, len(g.Scenarios[0].Objects))
					assert.Equal(t, "im x var", g.Scenarios[0].Objects[0].Label.Value)
					assert.Equal(t, "im y var", g.Scenarios[0].Objects[1].Label.Value)
					assert.Equal(t, 2, len(g.Layers[0].Objects))
					assert.Equal(t, "im x var", g.Layers[0].Objects[0].Label.Value)
					assert.Equal(t, "im y var", g.Layers[0].Objects[1].Label.Value)
				},
			},
			{
				name: "replace",
				run: func(t *testing.T) {
					g, _ := assertCompile(t, `
vars: {
  x: im x var
}

scenarios: {
  l: {
    vars: {
      x: im replaced x var
    }
    x: ${x}
  }
}
`, "")
					assert.Equal(t, 1, len(g.Scenarios[0].Objects))
					assert.Equal(t, "im replaced x var", g.Scenarios[0].Objects[0].Label.Value)
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

	t.Run("config", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "basic",
				run: func(t *testing.T) {
					_, config := assertCompile(t, `
vars: {
	d2-config: {
    sketch: true
  }
}

x -> y
`, "")
					assert.Equal(t, true, *config.Sketch)
				},
			},
			{
				name: "invalid",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
	d2-config: {
    sketch: lol
  }
}

x -> y
`, `d2/testdata/d2compiler/TestCompile2/vars/config/invalid.d2:4:5: expected a boolean for "sketch", got "lol"`)
				},
			},
			{
				name: "not-root",
				run: func(t *testing.T) {
					assertCompile(t, `
x: {
  vars: {
  	d2-config: {
      sketch: false
    }
  }
}
`, `d2/testdata/d2compiler/TestCompile2/vars/config/not-root.d2:4:4: "d2-config" can only appear at root vars`)
				},
			},
			{
				name: "data",
				run: func(t *testing.T) {
					_, config := assertCompile(t, `
vars: {
	d2-config: {
		data: {
      cat: hat
      later: [1;5;2]
    }
  }
}
`, ``)
					assert.Equal(t, 2, len(config.Data))
					assert.Equal(t, "hat", config.Data["cat"])
					assert.Equal(t, "1", (config.Data["later"]).([]interface{})[0])
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

	t.Run("errors", func(t *testing.T) {
		t.Parallel()

		tca := []struct {
			name string
			skip bool
			run  func(t *testing.T)
		}{
			{
				name: "missing",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
  x: hey
}
hi: ${z}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/missing.d2:5:1: could not resolve variable "z"`)
				},
			},
			{
				name: "multi-part-map",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
  x: {
    a: b
  }
}
hi: 1 ${x}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/multi-part-map.d2:7:1: cannot substitute composite variable "x" as part of a string`)
				},
			},
			{
				name: "quoted-map",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
  x: {
    a: b
  }
}
hi: "${x}"
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/quoted-map.d2:7:1: cannot substitute map variable "x" in quotes`)
				},
			},
			{
				name: "nested-missing",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
  x: {
    y: hey
  }
}
hi: ${x.z}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/nested-missing.d2:7:1: could not resolve variable "x.z"`)
				},
			},
			{
				name: "out-of-scope",
				run: func(t *testing.T) {
					assertCompile(t, `
a: {
  vars: {
    x: hey
  }
}
hi: ${x}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/out-of-scope.d2:7:1: could not resolve variable "x"`)
				},
			},
			{
				name: "recursive-var",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
  x: ${x}
}
hi: ${x}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/recursive-var.d2:3:3: could not resolve variable "x"`)
				},
			},

			{
				name: "spread-non-map",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
	x: all
}
z: {
  ...${x}
  c
}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/spread-non-map.d2:6:3: cannot spread non-composite`)
				},
			},
			{
				name: "missing-array",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
	x: b
}
z: {
  class: [...${a}]
}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/missing-array.d2:6:3: could not resolve variable "a"`)
				},
			},
			{
				name: "spread-non-array",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
	x: {
    a: b
  }
}
z: {
  class: [...${x}]
}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/spread-non-array.d2:8:11: cannot spread non-array into array`)
				},
			},
			{
				name: "spread-non-solo",
				// NOTE: this doesn't get parsed correctly and so the error message isn't exactly right, but the important thing is that it errors
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
	x: {
    a: b
  }
}
z: {
	d: ...${x}
  c
}
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/spread-non-solo.d2:8:2: cannot substitute composite variable "x" as part of a string`)
				},
			},
			{
				name: "spread-mid-string",
				run: func(t *testing.T) {
					assertCompile(t, `
vars: {
  test: hello
}

mybox: {
  label: prefix${test}suffix
}
`, "")
				},
			},
			{
				name: "undeclared-var-usage",
				run: func(t *testing.T) {
					assertCompile(t, `
x: { ...${v} }
`, `d2/testdata/d2compiler/TestCompile2/vars/errors/undeclared-var-usage.d2:2:4: could not resolve variable "v"`)
				},
			},
			{
				name: "split-var-usage",
				run: func(t *testing.T) {
					assertCompile(t, `
x1

vars: {
  v: {
    style.fill: green
  }
}

x1: { ...${v} }
`, ``)
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

func testGlobs(t *testing.T) {
	t.Parallel()

	tca := []struct {
		name string
		skip bool
		run  func(t *testing.T)
	}{
		{
			name: "alixander-lazy-globs-review/1",
			run: func(t *testing.T) {
				assertCompile(t, `
***.style.fill: yellow
**.shape: circle
*.style.multiple: true

x: {
  y
}

layers: {
  next: {
    a
  }
}
`, "")
			},
		},
		{
			name: "alixander-lazy-globs-review/2",
			run: func(t *testing.T) {
				assertCompile(t, `
**.style.fill: yellow

scenarios: {
  b: {
    a -> b
  }
}
`, "")
			},
		},
		{
			name: "alixander-lazy-globs-review/3",
			run: func(t *testing.T) {
				assertCompile(t, `
***: {
  c: d
}

***: {
  style.fill: red
}

table: {
  shape: sql_table
  a: b
}

class: {
  shape: class
  a: b
}
`, "")
			},
		},
		{
			name: "double-glob-err-val",
			run: func(t *testing.T) {
				assertCompile(t, `
**: {
  label: hi
  label.near: center
}

x: {
  a -> b
}
`, `d2/testdata/d2compiler/TestCompile2/globs/double-glob-err-val.d2:4:3: invalid "near" field`)
			},
		},
		{
			name: "double-glob-override-err-val",
			run: func(t *testing.T) {
				assertCompile(t, `
(** -> **)[*]: {
	label.near: top-center
}
(** -> **)[*]: {
	label.near: invalid
}

x: {
  a -> b
}
`, `d2/testdata/d2compiler/TestCompile2/globs/double-glob-override-err-val.d2:6:2: invalid "near" field`)
			},
		},
		{
			name: "creating-node-bug",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*.*a -> *.*b

container_1: {
	a
}

container_2: {
	b
}
`, ``)
				assert.Equal(t, 4, len(g.Objects))
			},
		},
		{
			name: "override-edge/1",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
(* -> *)[*].style.stroke: red
(* -> *)[*].style.stroke: green
a -> b
`, ``)
				assert.Equal(t, "green", g.Edges[0].Attributes.Style.Stroke.Value)
			},
		},
		{
			name: "override-edge/2",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
(* -> *)[*].style.stroke: red
a -> b: {style.stroke: green}
a -> b
`, ``)
				assert.Equal(t, "green", g.Edges[0].Attributes.Style.Stroke.Value)
				assert.Equal(t, "red", g.Edges[1].Attributes.Style.Stroke.Value)
			},
		},
		{
			name: "exists-filter",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*: {
  &link: *
	style.underline: true
}

x
y.link: https://google.com
`, ``)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Attributes.Style.Underline)
				assert.Equal(t, "true", g.Objects[1].Attributes.Style.Underline.Value)
			},
		},
		{
			name: "leaf-filter-1",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
**: {
  &leaf: false
  style.fill: red
}
**: {
  &leaf: true
  style.stroke: yellow
}
a.b.c
`, ``)
				assert.Equal(t, "a", g.Objects[0].ID)
				assert.Equal(t, "red", g.Objects[0].Attributes.Style.Fill.Value)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Attributes.Style.Stroke)
				assert.Equal(t, "b", g.Objects[1].ID)
				assert.Equal(t, "red", g.Objects[1].Attributes.Style.Fill.Value)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[1].Attributes.Style.Stroke)
				assert.Equal(t, "c", g.Objects[2].ID)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[2].Attributes.Style.Fill)
				assert.Equal(t, "yellow", g.Objects[2].Attributes.Style.Stroke.Value)
			},
		},
		{
			name: "leaf-filter-2",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
**: {
  &leaf: true
  style.stroke: yellow
}
a: {
  b -> c
}
d: {
  e
}
`, ``)
				assert.Equal(t, "a", g.Objects[0].ID)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Attributes.Style.Stroke)
				assert.Equal(t, "b", g.Objects[1].ID)
				assert.Equal(t, "yellow", g.Objects[1].Attributes.Style.Stroke.Value)
				assert.Equal(t, "c", g.Objects[2].ID)
				assert.Equal(t, "yellow", g.Objects[2].Attributes.Style.Stroke.Value)
				assert.Equal(t, "d", g.Objects[3].ID)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[3].Attributes.Style.Stroke)
				assert.Equal(t, "e", g.Objects[4].ID)
				assert.Equal(t, "yellow", g.Objects[4].Attributes.Style.Stroke.Value)
			},
		},
		{
			name: "connected-filter",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*: {
  &connected: true
  style.fill: red
}
a -> b
c
`, ``)
				assert.Equal(t, "a", g.Objects[0].ID)
				assert.Equal(t, "red", g.Objects[0].Attributes.Style.Fill.Value)
				assert.Equal(t, "b", g.Objects[1].ID)
				assert.Equal(t, "red", g.Objects[1].Attributes.Style.Fill.Value)
				assert.Equal(t, "c", g.Objects[2].ID)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[2].Attributes.Style.Fill)
			},
		},
		{
			name: "glob-filter",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*: {
  &link: *google*
	style.underline: true
}

x
y.link: https://google.com
z.link: https://yahoo.com
`, ``)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[0].Attributes.Style.Underline)
				assert.Equal(t, "true", g.Objects[1].Attributes.Style.Underline.Value)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[2].Attributes.Style.Underline)
			},
		},
		{
			name: "reapply-scenario",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*.b*.shape: circle
x: {
  b
}

scenarios: {
  k: {
    x: {
      b
    }
  }
}
`, ``)
				assert.Equal(t, "circle", g.Objects[1].Attributes.Shape.Value)
				assert.Equal(t, "circle", g.Scenarios[0].Objects[1].Attributes.Shape.Value)
			},
		},
		{
			name: "second-scenario",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*.b*.shape: circle

scenarios: {
  k: {
    x: {
      b
    }
  }
  z: {
    x: {
      b
    }
  }
}
`, ``)
				assert.Equal(t, "circle", g.Scenarios[0].Objects[1].Attributes.Shape.Value)
				assert.Equal(t, "circle", g.Scenarios[1].Objects[1].Attributes.Shape.Value)
			},
		},
		{
			name: "default-glob-filter/1",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*: {
	&shape: rectangle
  style.fill: red
}
*: {
  &style.opacity: 1
  style.stroke: blue
}
a
b.shape: circle
c.shape: rectangle
`, ``)
				assert.Equal(t, "red", g.Objects[0].Style.Fill.Value)
				assert.Equal(t, "blue", g.Objects[0].Style.Stroke.Value)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[1].Style.Fill)
				assert.Equal(t, "red", g.Objects[2].Style.Fill.Value)
			},
		},
		{
			name: "default-glob-filter/2",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*: {
	&shape: rectangle
  style.opacity: 0.2
}
a
b -> c
`, ``)
				assert.Equal(t, "0.2", g.Objects[0].Style.Opacity.Value)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[0].Style.Opacity)
			},
		},
		{
			name: "default-glob-filter/3",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*: {
	&icon: ""
  style.opacity: 0.2
}
a
b.icon: https://google.com/cat.jpg
`, ``)
				assert.Equal(t, "0.2", g.Objects[0].Style.Opacity.Value)
				assert.Equal(t, (*d2graph.Scalar)(nil), g.Objects[1].Style.Opacity)
			},
		},
		{
			name: "default-glob-filter/4",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
*: {
	&opacity: 1
  style.stroke: red
}
(* -> *)[*]: {
	&opacity: 1
  style.stroke: red
}
a
b -> c
`, ``)
				assert.Equal(t, "red", g.Objects[0].Style.Stroke.Value)
				assert.Equal(t, "red", g.Edges[0].Style.Stroke.Value)
			},
		},
		{
			name: "merge-glob-values",
			run: func(t *testing.T) {
				assertCompile(t, `
"a"
*.style.stroke-width: 2
*.style.font-size: 14
a.width: 339
`, ``)
			},
		},
		{
			name: "mixed-edge-quoting",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
"a"."b"."c"."z1" -> "a"."b"."c"."z2"
`, ``)
				assert.Equal(t, 5, len(g.Objects))
			},
		},
		{
			name: "suspension-lazy",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
a -> b
c
**: suspend
(** -> **)[*]: suspend
d
`, ``)
				assert.Equal(t, 1, len(g.Objects))
			},
		},
		{
			name: "suspension-quotes",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
a -> b
c
**: suspend
(** -> **)[*]: suspend
d: "suspend"
d -> d: "suspend"
`, ``)
				assert.Equal(t, 1, len(g.Objects))
				assert.Equal(t, 1, len(g.Edges))
			},
		},
		{
			name: "edge-glob-ampersand-filter/1",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
  (* -> *)[*]: {
    &src: a
    style.stroke-dash: 3
  }
  (* -> *)[*]: {
    &dst: c
    style.stroke: blue
  }
  (* -> *)[*]: {
    &src: b
    &dst: c
    style.fill: red
  }
  a -> b
  b -> c
  a -> c
  `, ``)
				tassert.Equal(t, 3, len(g.Edges))

				tassert.Equal(t, "a", g.Edges[0].Src.ID)
				tassert.Equal(t, "b", g.Edges[0].Dst.ID)
				tassert.Equal(t, "3", g.Edges[0].Style.StrokeDash.Value)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[0].Style.Stroke)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[0].Style.Fill)

				tassert.Equal(t, "b", g.Edges[1].Src.ID)
				tassert.Equal(t, "c", g.Edges[1].Dst.ID)
				tassert.Equal(t, "blue", g.Edges[1].Style.Stroke.Value)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[1].Style.StrokeDash)
				tassert.Equal(t, "red", g.Edges[1].Style.Fill.Value)

				tassert.Equal(t, "a", g.Edges[2].Src.ID)
				tassert.Equal(t, "c", g.Edges[2].Dst.ID)
				tassert.Equal(t, "3", g.Edges[2].Style.StrokeDash.Value)
				tassert.Equal(t, "blue", g.Edges[2].Style.Stroke.Value)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[2].Style.Fill)
			},
		},
		{
			name: "edge-glob-ampersand-filter/2",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
a: {
		shape: circle
		style: {
				fill: blue
				opacity: 0.8
		}
}
b: {
		shape: rectangle
		style: {
				fill: red
				opacity: 0.5
		}
}
c: {
		shape: diamond
		style.fill: green
		style.opacity: 0.8
}

(* -> *)[*]: {
		&src.style.fill: blue
		style.stroke-dash: 3
}
(* -> *)[*]: {
		&dst.style.opacity: 0.8
		style.stroke: cyan
}
(* -> *)[*]: {
		&src.shape: rectangle
		&dst.style.fill: green
		style.stroke-width: 5
}

a -> b
b -> c
a -> c
        `, ``)

				tassert.Equal(t, 3, len(g.Edges))

				tassert.Equal(t, "a", g.Edges[0].Src.ID)
				tassert.Equal(t, "b", g.Edges[0].Dst.ID)
				tassert.Equal(t, "3", g.Edges[0].Style.StrokeDash.Value)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[0].Style.Stroke)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[0].Style.StrokeWidth)

				tassert.Equal(t, "b", g.Edges[1].Src.ID)
				tassert.Equal(t, "c", g.Edges[1].Dst.ID)
				tassert.Equal(t, "cyan", g.Edges[1].Style.Stroke.Value)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[1].Style.StrokeDash)
				tassert.Equal(t, "5", g.Edges[1].Style.StrokeWidth.Value)

				tassert.Equal(t, "a", g.Edges[2].Src.ID)
				tassert.Equal(t, "c", g.Edges[2].Dst.ID)
				tassert.Equal(t, "3", g.Edges[2].Style.StrokeDash.Value)
				tassert.Equal(t, "cyan", g.Edges[2].Style.Stroke.Value)
				tassert.Equal(t, (*d2graph.Scalar)(nil), g.Edges[2].Style.StrokeWidth)
			},
		},
		{
			name: "md-shape",
			run: func(t *testing.T) {
				g, _ := assertCompile(t, `
a.shape: circle
a: |md #hi |

b.shape: circle
b.label: |md #hi |

c: |md #hi |
c.shape: circle

d.label: |md #hi |
d.shape: circle

e: {
  shape: circle
  label: |md #hi |
}
        `, ``)
				tassert.Equal(t, 5, len(g.Objects))
				for _, obj := range g.Objects {
					tassert.Equal(t, "circle", obj.Shape.Value, "Object "+obj.ID+" should have circle shape")
					tassert.Equal(t, "markdown", obj.Language, "Object "+obj.ID+" should have md language")
				}
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
}

func assertCompile(t *testing.T, text string, expErr string) (*d2graph.Graph, *d2target.Config) {
	d2Path := fmt.Sprintf("d2/testdata/d2compiler/%v.d2", t.Name())
	g, config, err := d2compiler.Compile(d2Path, strings.NewReader(text), nil)
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
	return g, config
}
