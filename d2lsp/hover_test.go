package d2lsp_test

import (
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2lsp"
	"oss.terrastruct.com/util-go/assert"
)

func TestGetHoverInfo(t *testing.T) {
	t.Run("basic_object", func(t *testing.T) {
		script := `myObject: {
  shape: rectangle
}`
		hover, err := d2lsp.GetHoverInfo(script, 0, 2)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**Object**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "myObject"))
	})

	t.Run("shape_keyword", func(t *testing.T) {
		script := `object.shape: rectangle`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**shape**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "visual shape"))
	})

	t.Run("shape_value", func(t *testing.T) {
		script := `object.shape: rectangle`
		hover, err := d2lsp.GetHoverInfo(script, 0, 15)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**Shape**: `rectangle`"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Standard rectangular shape"))
	})

	t.Run("style_keyword", func(t *testing.T) {
		script := `object.style.fill: blue`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**fill**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "background/fill color"))
	})

	t.Run("style_container", func(t *testing.T) {
		script := `object.style: {
  fill: blue
}`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**style**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Visual Styling"))
	})

	t.Run("boolean_value", func(t *testing.T) {
		script := `object.style.bold: true`
		hover, err := d2lsp.GetHoverInfo(script, 0, 19)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**Boolean Value**"))
	})

	t.Run("color_value", func(t *testing.T) {
		script := `object.style.fill: blue`
		hover, err := d2lsp.GetHoverInfo(script, 0, 19)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**Color**: `blue`"))
	})

	t.Run("hex_color", func(t *testing.T) {
		script := `object.style.fill: "#FF0000"`
		hover, err := d2lsp.GetHoverInfo(script, 0, 20)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**Color**: `#FF0000`"))
	})

	t.Run("label_keyword", func(t *testing.T) {
		script := `object.label: "My Label"`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**label**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "display text"))
	})

	t.Run("icon_keyword", func(t *testing.T) {
		script := `object.icon: "https://example.com/icon.svg"`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**icon**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Adds an icon"))
	})

	t.Run("tooltip_keyword", func(t *testing.T) {
		script := `object.tooltip: "Info"`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**tooltip**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "additional information"))
	})

	t.Run("direction_keyword", func(t *testing.T) {
		script := `direction: right`
		hover, err := d2lsp.GetHoverInfo(script, 0, 4)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**direction**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "layout direction"))
	})

	t.Run("direction_value", func(t *testing.T) {
		script := `direction: right`
		hover, err := d2lsp.GetHoverInfo(script, 0, 11)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**Direction**: `right`"))
	})

	t.Run("near_keyword", func(t *testing.T) {
		script := `label.near: top-center`
		hover, err := d2lsp.GetHoverInfo(script, 0, 6)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**near**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Positioning"))
	})

	t.Run("near_value", func(t *testing.T) {
		script := `label.near: top-center`
		hover, err := d2lsp.GetHoverInfo(script, 0, 12)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**Position**: `top-center`"))
	})

	t.Run("width_height", func(t *testing.T) {
		script := `object.width: 200`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**width**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "pixels"))
	})

	t.Run("arrowhead", func(t *testing.T) {
		script := `edge.source-arrowhead.shape: diamond`
		hover, err := d2lsp.GetHoverInfo(script, 0, 5)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**source-arrowhead**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Arrow Customization"))
	})

	t.Run("classes_keyword", func(t *testing.T) {
		script := `classes: {
  error: { style.fill: red }
}`
		hover, err := d2lsp.GetHoverInfo(script, 0, 4)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**classes**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "reusable style classes"))
	})

	t.Run("class_keyword", func(t *testing.T) {
		script := `object.class: error`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**class**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "CSS Class Reference"))
	})

	t.Run("vars_keyword", func(t *testing.T) {
		script := `vars: {
  color: blue
}`
		hover, err := d2lsp.GetHoverInfo(script, 0, 2)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**vars**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Variables"))
	})

	t.Run("layers_keyword", func(t *testing.T) {
		script := `layers: {
  base: { a -> b }
}`
		hover, err := d2lsp.GetHoverInfo(script, 0, 3)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**layers**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Diagram Layers"))
	})

	t.Run("scenarios_keyword", func(t *testing.T) {
		script := `scenarios: {
  happy: { success -> result }
}`
		hover, err := d2lsp.GetHoverInfo(script, 0, 4)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**scenarios**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Diagram Scenarios"))
	})

	t.Run("steps_keyword", func(t *testing.T) {
		script := `steps: {
  1: { start -> process }
}`
		hover, err := d2lsp.GetHoverInfo(script, 0, 2)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**steps**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Sequence Steps"))
	})

	t.Run("link_keyword", func(t *testing.T) {
		script := `object.link: "https://example.com"`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**link**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "External Link"))
	})

	t.Run("constraint_keyword", func(t *testing.T) {
		script := `object.constraint: near`
		hover, err := d2lsp.GetHoverInfo(script, 0, 7)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**constraint**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Layout Constraints"))
	})
}

func TestGetHoverInfoStyleProperties(t *testing.T) {
	t.Run("opacity", func(t *testing.T) {
		script := `object.style.opacity: 0.5`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**opacity**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "transparency"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "0.0"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "1.0"))
	})

	t.Run("stroke_width", func(t *testing.T) {
		script := `object.style.stroke-width: 2`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**stroke-width**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "thickness"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "0 to 15"))
	})

	t.Run("font_size", func(t *testing.T) {
		script := `object.style.font-size: 16`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**font-size**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "8 to 100"))
	})

	t.Run("bold", func(t *testing.T) {
		script := `object.style.bold: true`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**bold**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Bold Text"))
	})

	t.Run("shadow", func(t *testing.T) {
		script := `object.style.shadow: true`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**shadow**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "drop shadow"))
	})

	t.Run("3d", func(t *testing.T) {
		script := `object.style.3d: true`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**3d**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "3D Effect"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "square"))
	})

	t.Run("animated", func(t *testing.T) {
		script := `edge.style.animated: true`
		hover, err := d2lsp.GetHoverInfo(script, 0, 12)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**animated**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Edge Animation"))
	})

	t.Run("filled", func(t *testing.T) {
		script := `edge.style.filled: true`
		hover, err := d2lsp.GetHoverInfo(script, 0, 11)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**filled**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "Filled Arrowhead"))
	})

	t.Run("fill_pattern", func(t *testing.T) {
		script := `object.style.fill-pattern: dots`
		hover, err := d2lsp.GetHoverInfo(script, 0, 13)
		assert.Success(t, err)
		assert.NotEqual(t, nil, hover)
		assert.Equal(t, true, strings.Contains(hover.Contents, "**fill-pattern**"))
		assert.Equal(t, true, strings.Contains(hover.Contents, "pattern"))
	})
}

func TestGetHoverInfoEdges(t *testing.T) {
	t.Run("basic_edge", func(t *testing.T) {
		script := `a -> b`
		_, err := d2lsp.GetHoverInfo(script, 0, 3)
		assert.Success(t, err)
		// For edges, we might not get hover on the arrow itself depending on position
		// This tests the basic parsing doesn't crash
	})

	t.Run("edge_with_label", func(t *testing.T) {
		script := `a -> b: "Connection"`
		_, err := d2lsp.GetHoverInfo(script, 0, 3)
		assert.Success(t, err)
		// Basic parsing test
	})
}

func TestGetHoverInfoShapes(t *testing.T) {
	shapes := []string{
		"rectangle", "square", "circle", "oval", "diamond", 
		"parallelogram", "hexagon", "cylinder", "queue", "package",
		"step", "callout", "stored_data", "person", "cloud", 
		"text", "code", "class", "sql_table",
	}

	for _, shape := range shapes {
		t.Run("shape_"+shape, func(t *testing.T) {
			script := `object.shape: ` + shape
			hover, err := d2lsp.GetHoverInfo(script, 0, 15)
			assert.Success(t, err)
			if hover != nil {
				assert.Equal(t, true, strings.Contains(hover.Contents, "**Shape**: `"+shape+"`"))
			}
		})
	}
}

func TestGetHoverInfoFillPatterns(t *testing.T) {
	patterns := []string{"dots", "lines", "grain"}

	for _, pattern := range patterns {
		t.Run("pattern_"+pattern, func(t *testing.T) {
			script := `object.style.fill-pattern: ` + pattern
			hover, err := d2lsp.GetHoverInfo(script, 0, 28)
			assert.Success(t, err)
			if hover != nil {
				assert.Equal(t, true, strings.Contains(hover.Contents, "**Fill Pattern**: `"+pattern+"`"))
			}
		})
	}
}

func TestGetHoverInfoTextTransforms(t *testing.T) {
	transforms := []string{"none", "uppercase", "lowercase", "capitalize"}

	for _, transform := range transforms {
		t.Run("transform_"+transform, func(t *testing.T) {
			script := `object.style.text-transform: ` + transform
			hover, err := d2lsp.GetHoverInfo(script, 0, 30)
			assert.Success(t, err)
			if hover != nil {
				assert.Equal(t, true, strings.Contains(hover.Contents, "**Text Transform**: `"+transform+"`"))
			}
		})
	}
}

func TestGetHoverInfoNoHover(t *testing.T) {
	t.Run("empty_file", func(t *testing.T) {
		script := ""
		hover, err := d2lsp.GetHoverInfo(script, 0, 0)
		assert.Success(t, err)
		assert.Equal(t, nil, hover)
	})

	t.Run("position_outside_content", func(t *testing.T) {
		script := `object: value`
		hover, err := d2lsp.GetHoverInfo(script, 0, 100)
		assert.Success(t, err)
		assert.Equal(t, nil, hover)
	})

	t.Run("position_in_whitespace", func(t *testing.T) {
		script := `object: {
		
  shape: rectangle
}`
		_, err := d2lsp.GetHoverInfo(script, 1, 2)
		assert.Success(t, err)
		// Should not crash, might or might not return hover
	})
}

func TestGetHoverInfoComplexStructures(t *testing.T) {
	t.Run("nested_objects", func(t *testing.T) {
		script := `container: {
  inner: {
    shape: rectangle
    style: {
      fill: blue
      opacity: 0.8
    }
  }
}`
		// Test hovering on nested shape
		hover, err := d2lsp.GetHoverInfo(script, 2, 4)
		assert.Success(t, err)
		if hover != nil {
			assert.Equal(t, true, strings.Contains(hover.Contents, "shape"))
		}

		// Test hovering on nested style property
		hover, err = d2lsp.GetHoverInfo(script, 4, 6)
		assert.Success(t, err)
		if hover != nil {
			assert.Equal(t, true, strings.Contains(hover.Contents, "fill"))
		}
	})

	t.Run("classes_and_styles", func(t *testing.T) {
		script := `classes: {
  error: {
    style.fill: red
    style.font-color: white
  }
}

object: {
  class: error
  shape: rectangle
}`
		// Test hovering on class definition
		hover, err := d2lsp.GetHoverInfo(script, 0, 0)
		assert.Success(t, err)
		if hover != nil {
			assert.Equal(t, true, strings.Contains(hover.Contents, "classes"))
		}

		// Test hovering on class usage
		hover, err = d2lsp.GetHoverInfo(script, 8, 2)
		assert.Success(t, err)
		if hover != nil {
			assert.Equal(t, true, strings.Contains(hover.Contents, "class"))
		}
	})
}
