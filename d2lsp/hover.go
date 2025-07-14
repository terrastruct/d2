// Hover implements LSP hover documentation for D2 language constructs
package d2lsp

import (
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/d2target"
)

// HoverInfo represents hover documentation information
type HoverInfo struct {
	Contents    string // Markdown formatted documentation
	Range       *d2ast.Range
	Language    string // Language identifier for syntax highlighting
}

// GetHoverInfo returns hover documentation for the element at the given position
func GetHoverInfo(text string, line, column int) (*HoverInfo, error) {
	ast, err := d2parser.Parse("", strings.NewReader(text), nil)
	if err != nil {
		// Try to parse partial content for better error recovery
		partialText := getTextUntilPosition(text, line, column)
		ast, _ = d2parser.Parse("", strings.NewReader(partialText), nil)
	}

	if ast == nil {
		return nil, nil
	}

	return getHoverAtPosition(text, ast, line, column), nil
}

// getHoverAtPosition finds hover information at the specified position
func getHoverAtPosition(text string, m *d2ast.Map, line, column int) *HoverInfo {
	if m == nil {
		return nil
	}

	pos := d2ast.Position{Line: line, Column: column}

	// Check all nodes in the map
	for _, n := range m.Nodes {
		if n.MapKey == nil {
			continue
		}

		mk := n.MapKey
		
		// Check if position is within this node's range
		if !isPositionInRange(pos, mk.Range) {
			continue
		}

		// Check nested maps first
		if mk.Value.Map != nil && isPositionInRange(pos, mk.Value.Map.Range) {
			if nested := getHoverAtPosition(text, mk.Value.Map, line, column); nested != nil {
				return nested
			}
		}

		// Get hover info for this key
		return getHoverForKey(mk, pos)
	}

	return nil
}

// isPositionInRange checks if position is within the given range
func isPositionInRange(pos d2ast.Position, r d2ast.Range) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Column < r.Start.Column {
		return false
	}
	if pos.Line == r.End.Line && pos.Column > r.End.Column {
		return false
	}
	return true
}

// getHoverForKey returns hover information for a specific key
func getHoverForKey(mk *d2ast.Key, pos d2ast.Position) *HoverInfo {
	if len(mk.Edges) > 0 {
		return getEdgeHover(mk, pos)
	}

	key := mk.Key
	if key == nil || len(key.Path) == 0 {
		return nil
	}

	// Find which part of the key path the cursor is on
	for i, pathElement := range key.Path {
		elementRange := pathElement.Unbox().GetRange()
		if isPositionInRange(pos, elementRange) {
			keyName := pathElement.Unbox().ScalarString()
			keyPath := getKeyPathString(key.Path[:i+1])
			
			return getKeywordHover(keyName, keyPath, elementRange)
		}
	}

	return nil
}

// getEdgeHover returns hover information for edges
func getEdgeHover(mk *d2ast.Key, pos d2ast.Position) *HoverInfo {
	if len(mk.Edges) == 0 {
		return nil
	}

	edge := mk.Edges[0]
	
	// Check if hovering over edge arrow
	if edge.Dst != nil {
		dstRange := edge.Dst.GetRange()
		if isPositionInRange(pos, dstRange) {
			return &HoverInfo{
				Contents: "**Edge Connection**\n\nDefines a connection between two objects in the diagram.\n\n" +
					"**Syntax**: `source -> target` or `source <-> target` for bidirectional\n\n" +
					"**Properties**: Can have labels, styling, and arrowhead customizations.",
				Range:    &dstRange,
				Language: "d2",
			}
		}
	}

	// Check if hovering over source or destination
	if edge.Src != nil {
		srcRange := edge.Src.GetRange()
		if isPositionInRange(pos, srcRange) {
			return &HoverInfo{
				Contents: "**Edge Source**\n\nThe source object of this connection.",
				Range:    &srcRange,
				Language: "d2",
			}
		}
	}

	return nil
}

// getKeywordHover returns hover documentation for keywords
func getKeywordHover(keyName, fullPath string, r d2ast.Range) *HoverInfo {
	// Check for reserved keywords
	if hover := getReservedKeywordHover(keyName, fullPath); hover != nil {
		hover.Range = &r
		return hover
	}

	// Check for style keywords
	if hover := getStyleKeywordHover(keyName, fullPath); hover != nil {
		hover.Range = &r
		return hover
	}

	// Check for shape values
	if hover := getShapeHover(keyName); hover != nil {
		hover.Range = &r
		return hover
	}

	// Check for special values
	if hover := getSpecialValueHover(keyName, fullPath); hover != nil {
		hover.Range = &r
		return hover
	}

	// Default object hover
	return &HoverInfo{
		Contents: fmt.Sprintf("**Object**: `%s`\n\nD2 diagram object. Can contain:\n- Properties (shape, style, etc.)\n- Nested objects\n- Connections to other objects", keyName),
		Range:    &r,
		Language: "d2",
	}
}

// getReservedKeywordHover returns hover for reserved keywords
func getReservedKeywordHover(keyName, fullPath string) *HoverInfo {
	switch keyName {
	case "label":
		return &HoverInfo{
			Contents: "**label** - Object Label\n\n" +
				"Sets the display text for an object.\n\n" +
				"**Usage**:\n```d2\nobject.label: \"My Label\"\n```\n\n" +
				"**Properties**:\n- `near`: Position relative to object\n- Supports markdown formatting",
			Language: "d2",
		}
	case "shape":
		return &HoverInfo{
			Contents: "**shape** - Object Shape\n\n" +
				"Defines the visual shape of an object.\n\n" +
				"**Usage**:\n```d2\nobject.shape: rectangle\n```\n\n" +
				"**Available shapes**: rectangle, circle, oval, diamond, parallelogram, hexagon, cylinder, queue, package, step, callout, stored_data, person, diamond, oval, cloud, text, code, class, sql_table, image, sequence_diagram",
			Language: "d2",
		}
	case "style":
		return &HoverInfo{
			Contents: "**style** - Visual Styling\n\n" +
				"Container for all visual styling properties.\n\n" +
				"**Usage**:\n```d2\nobject.style: {\n  fill: blue\n  stroke: red\n  opacity: 0.8\n}\n```\n\n" +
				"**Properties**: fill, stroke, opacity, font-size, bold, italic, shadow, and more",
			Language: "d2",
		}
	case "icon":
		return &HoverInfo{
			Contents: "**icon** - Object Icon\n\n" +
				"Adds an icon to an object.\n\n" +
				"**Usage**:\n```d2\nobject.icon: https://icons.terrastruct.com/tech/golang.svg\n```\n\n" +
				"**Properties**:\n- `near`: Position relative to object\n- Supports URLs and local paths",
			Language: "d2",
		}
	case "tooltip":
		return &HoverInfo{
			Contents: "**tooltip** - Hover Information\n\n" +
				"Displays additional information when hovering over an object.\n\n" +
				"**Usage**:\n```d2\nobject.tooltip: |\n  # Additional Info\n  This appears on hover\n|\n```\n\n" +
				"Supports markdown formatting for rich tooltips.",
			Language: "d2",
		}
	case "constraint":
		return &HoverInfo{
			Contents: "**constraint** - Layout Constraints\n\n" +
				"Controls object positioning and layout behavior.\n\n" +
				"**Usage**:\n```d2\nobject.constraint: near\n```\n\n" +
				"**Values**: near, constant",
			Language: "d2",
		}
	case "near":
		return &HoverInfo{
			Contents: "**near** - Positioning\n\n" +
				"Controls the position of labels, icons, or objects relative to their parent.\n\n" +
				"**Usage**:\n```d2\nlabel.near: top-center\nobject.near: other_object\n```\n\n" +
				"**Positions**: top-left, top-center, top-right, center-left, center-right, bottom-left, bottom-center, bottom-right, or any object ID",
			Language: "d2",
		}
	case "direction":
		return &HoverInfo{
			Contents: "**direction** - Layout Direction\n\n" +
				"Controls the layout direction for the diagram or container.\n\n" +
				"**Usage**:\n```d2\ndirection: right\n```\n\n" +
				"**Values**: up, down, left, right",
			Language: "d2",
		}
	case "width", "height":
		return &HoverInfo{
			Contents: fmt.Sprintf("**%s** - Object Dimensions\n\n"+
				"Sets the %s of an object in pixels.\n\n"+
				"**Usage**:\n```d2\nobject.%s: 200\n```\n\n"+
				"Value should be a positive number representing pixels.", keyName, keyName, keyName),
			Language: "d2",
		}
	case "top", "left":
		return &HoverInfo{
			Contents: fmt.Sprintf("**%s** - Absolute Positioning\n\n"+
				"Sets the absolute %s position of an object in pixels.\n\n"+
				"**Usage**:\n```d2\nobject.%s: 100\n```\n\n"+
				"Used for precise positioning within containers.", keyName, keyName, keyName),
			Language: "d2",
		}
	case "link":
		return &HoverInfo{
			Contents: "**link** - External Link\n\n" +
				"Makes an object clickable with an external URL.\n\n" +
				"**Usage**:\n```d2\nobject.link: https://example.com\n```\n\n" +
				"Clicking the object will open the URL in a new tab.",
			Language: "d2",
		}
	case "class":
		return &HoverInfo{
			Contents: "**class** - CSS Class Reference\n\n" +
				"References a class defined in the classes section.\n\n" +
				"**Usage**:\n```d2\nclasses: {\n  important: { style.fill: red }\n}\nobject.class: important\n```\n\n" +
				"Applies the styling from the referenced class.",
			Language: "d2",
		}
	case "classes":
		return &HoverInfo{
			Contents: "**classes** - Style Classes\n\n" +
				"Defines reusable style classes.\n\n" +
				"**Usage**:\n```d2\nclasses: {\n  error: {\n    style.fill: red\n    style.font-color: white\n  }\n}\n```\n\n" +
				"Classes can be referenced using the `class` property.",
			Language: "d2",
		}
	case "vars":
		return &HoverInfo{
			Contents: "**vars** - Variables\n\n" +
				"Defines reusable variables for the diagram.\n\n" +
				"**Usage**:\n```d2\nvars: {\n  primary-color: blue\n}\nobject.style.fill: ${primary-color}\n```\n\n" +
				"Variables can be referenced using `${variable-name}` syntax.",
			Language: "d2",
		}
	case "source-arrowhead", "target-arrowhead":
		return &HoverInfo{
			Contents: fmt.Sprintf("**%s** - Arrow Customization\n\n"+
				"Customizes the appearance of the %s arrowhead.\n\n"+
				"**Usage**:\n```d2\nedge.%s: {\n  shape: diamond\n  style.filled: true\n}\n```\n\n"+
				"**Properties**: shape, label, style", keyName, strings.Split(keyName, "-")[0], keyName),
			Language: "d2",
		}
	case "layers":
		return &HoverInfo{
			Contents: "**layers** - Diagram Layers\n\n" +
				"Creates multiple layers of the same diagram structure.\n\n" +
				"**Usage**:\n```d2\nlayers: {\n  base: { a -> b }\n  detailed: { a -> b -> c }\n}\n```\n\n" +
				"Each layer shows a different view or state of the diagram.",
			Language: "d2",
		}
	case "scenarios":
		return &HoverInfo{
			Contents: "**scenarios** - Diagram Scenarios\n\n" +
				"Creates different scenarios within a layer.\n\n" +
				"**Usage**:\n```d2\nscenarios: {\n  happy: { success -> result }\n  error: { failure -> retry }\n}\n```\n\n" +
				"Shows different execution paths or states.",
			Language: "d2",
		}
	case "steps":
		return &HoverInfo{
			Contents: "**steps** - Sequence Steps\n\n" +
				"Creates sequential steps within a scenario.\n\n" +
				"**Usage**:\n```d2\nsteps: {\n  1: { start -> process }\n  2: { process -> end }\n}\n```\n\n" +
				"Shows progression through time or sequence.",
			Language: "d2",
		}
	}
	return nil
}

// getStyleKeywordHover returns hover for style-specific keywords
func getStyleKeywordHover(keyName, fullPath string) *HoverInfo {
	// Check if this is a style property
	if !strings.Contains(fullPath, "style.") && !isStyleContext(fullPath) {
		return nil
	}

	switch keyName {
	case "fill":
		return &HoverInfo{
			Contents: "**fill** - Fill Color\n\n" +
				"Sets the background/fill color of an object.\n\n" +
				"**Usage**:\n```d2\nobject.style.fill: blue\nobject.style.fill: \"#FF0000\"\n```\n\n" +
				"**Values**: Color names (red, blue, green, etc.) or hex codes (#RRGGBB)",
			Language: "d2",
		}
	case "stroke":
		return &HoverInfo{
			Contents: "**stroke** - Border Color\n\n" +
				"Sets the border/outline color of an object.\n\n" +
				"**Usage**:\n```d2\nobject.style.stroke: red\nobject.style.stroke: \"#00FF00\"\n```\n\n" +
				"**Values**: Color names or hex codes",
			Language: "d2",
		}
	case "opacity":
		return &HoverInfo{
			Contents: "**opacity** - Transparency\n\n" +
				"Sets the transparency level of an object.\n\n" +
				"**Usage**:\n```d2\nobject.style.opacity: 0.5\n```\n\n" +
				"**Range**: 0.0 (completely transparent) to 1.0 (completely opaque)",
			Language: "d2",
		}
	case "stroke-width":
		return &HoverInfo{
			Contents: "**stroke-width** - Border Thickness\n\n" +
				"Sets the thickness of the object's border.\n\n" +
				"**Usage**:\n```d2\nobject.style.stroke-width: 3\n```\n\n" +
				"**Range**: 0 to 15 pixels",
			Language: "d2",
		}
	case "stroke-dash":
		return &HoverInfo{
			Contents: "**stroke-dash** - Dashed Border\n\n" +
				"Creates a dashed border pattern.\n\n" +
				"**Usage**:\n```d2\nobject.style.stroke-dash: 5\n```\n\n" +
				"**Range**: 0 to 10 (dash length)",
			Language: "d2",
		}
	case "border-radius":
		return &HoverInfo{
			Contents: "**border-radius** - Rounded Corners\n\n" +
				"Sets the roundness of object corners.\n\n" +
				"**Usage**:\n```d2\nobject.style.border-radius: 8\n```\n\n" +
				"**Range**: 0 (sharp corners) and up (more rounded)",
			Language: "d2",
		}
	case "font-size":
		return &HoverInfo{
			Contents: "**font-size** - Text Size\n\n" +
				"Sets the size of text within the object.\n\n" +
				"**Usage**:\n```d2\nobject.style.font-size: 16\n```\n\n" +
				"**Range**: 8 to 100 pixels",
			Language: "d2",
		}
	case "font-color":
		return &HoverInfo{
			Contents: "**font-color** - Text Color\n\n" +
				"Sets the color of text within the object.\n\n" +
				"**Usage**:\n```d2\nobject.style.font-color: white\nobject.style.font-color: \"#FFFFFF\"\n```\n\n" +
				"**Values**: Color names or hex codes",
			Language: "d2",
		}
	case "bold":
		return &HoverInfo{
			Contents: "**bold** - Bold Text\n\n" +
				"Makes text bold when set to true.\n\n" +
				"**Usage**:\n```d2\nobject.style.bold: true\n```\n\n" +
				"**Values**: true or false",
			Language: "d2",
		}
	case "italic":
		return &HoverInfo{
			Contents: "**italic** - Italic Text\n\n" +
				"Makes text italic when set to true.\n\n" +
				"**Usage**:\n```d2\nobject.style.italic: true\n```\n\n" +
				"**Values**: true or false",
			Language: "d2",
		}
	case "underline":
		return &HoverInfo{
			Contents: "**underline** - Underlined Text\n\n" +
				"Underlines text when set to true.\n\n" +
				"**Usage**:\n```d2\nobject.style.underline: true\n```\n\n" +
				"**Values**: true or false",
			Language: "d2",
		}
	case "shadow":
		return &HoverInfo{
			Contents: "**shadow** - Drop Shadow\n\n" +
				"Adds a drop shadow effect to the object.\n\n" +
				"**Usage**:\n```d2\nobject.style.shadow: true\n```\n\n" +
				"**Values**: true or false",
			Language: "d2",
		}
	case "multiple":
		return &HoverInfo{
			Contents: "**multiple** - Multiple Objects Effect\n\n" +
				"Creates a visual effect showing multiple stacked objects.\n\n" +
				"**Usage**:\n```d2\nobject.style.multiple: true\n```\n\n" +
				"**Values**: true or false",
			Language: "d2",
		}
	case "3d":
		return &HoverInfo{
			Contents: "**3d** - 3D Effect\n\n" +
				"Adds a 3D visual effect to square objects.\n\n" +
				"**Usage**:\n```d2\nobject.style.3d: true\n```\n\n" +
				"**Values**: true or false (only works with square shapes)",
			Language: "d2",
		}
	case "animated":
		return &HoverInfo{
			Contents: "**animated** - Edge Animation\n\n" +
				"Animates the edge with a flowing effect.\n\n" +
				"**Usage**:\n```d2\nedge.style.animated: true\n```\n\n" +
				"**Values**: true or false (only for edges)",
			Language: "d2",
		}
	case "filled":
		return &HoverInfo{
			Contents: "**filled** - Filled Arrowhead\n\n" +
				"Makes arrowheads filled instead of outlined.\n\n" +
				"**Usage**:\n```d2\nedge.style.filled: true\n```\n\n" +
				"**Values**: true or false (for edges and arrowheads)",
			Language: "d2",
		}
	case "double-border":
		return &HoverInfo{
			Contents: "**double-border** - Double Border\n\n" +
				"Creates a double border effect around the object.\n\n" +
				"**Usage**:\n```d2\nobject.style.double-border: true\n```\n\n" +
				"**Values**: true or false",
			Language: "d2",
		}
	case "fill-pattern":
		return &HoverInfo{
			Contents: "**fill-pattern** - Fill Pattern\n\n" +
				"Applies a pattern to the object's fill.\n\n" +
				"**Usage**:\n```d2\nobject.style.fill-pattern: dots\n```\n\n" +
				"**Values**: dots, lines, grain",
			Language: "d2",
		}
	case "text-transform":
		return &HoverInfo{
			Contents: "**text-transform** - Text Case\n\n" +
				"Transforms the case of text within the object.\n\n" +
				"**Usage**:\n```d2\nobject.style.text-transform: uppercase\n```\n\n" +
				"**Values**: none, uppercase, lowercase, capitalize",
			Language: "d2",
		}
	case "font":
		return &HoverInfo{
			Contents: "**font** - Font Family\n\n" +
				"Sets the font family for text.\n\n" +
				"**Usage**:\n```d2\nobject.style.font: mono\n```\n\n" +
				"**Values**: Default D2 fonts or system font names",
			Language: "d2",
		}
	}
	return nil
}

// getShapeHover returns hover information for shape values
func getShapeHover(shapeName string) *HoverInfo {
	for _, shape := range d2target.Shapes {
		if shape == shapeName {
			descriptions := map[string]string{
				"rectangle":        "Standard rectangular shape, good for most objects",
				"square":          "Square shape, can use 3D effects",
				"circle":          "Circular shape, good for processes or states",
				"oval":            "Oval/ellipse shape, softer alternative to rectangle",
				"diamond":         "Diamond shape, commonly used for decisions",
				"parallelogram":   "Parallelogram shape, often used for data/input",
				"hexagon":         "Hexagonal shape, used for preparation steps",
				"cylinder":        "Cylinder shape, typically for databases",
				"queue":           "Queue shape, for message queues or buffers",
				"package":         "Package shape, for components or modules",
				"step":            "Step shape, for process steps",
				"callout":         "Callout shape, for annotations or comments",
				"stored_data":     "Stored data shape, for data storage",
				"person":          "Person/actor shape, for users or actors",
				"cloud":           "Cloud shape, for cloud services",
				"text":            "Text-only shape, no border",
				"code":            "Code block shape, for code examples",
				"class":           "UML class shape, for class diagrams",
				"sql_table":       "SQL table shape, for database schemas",
				"image":           "Image shape, for embedding images",
				"sequence_diagram": "Sequence diagram shape, for sequence flows",
			}

			description := descriptions[shapeName]
			if description == "" {
				description = "Shape for diagram objects"
			}

			return &HoverInfo{
				Contents: fmt.Sprintf("**Shape**: `%s`\n\n%s\n\n**Usage**:\n```d2\nobject.shape: %s\n```", shapeName, description, shapeName),
				Language: "d2",
			}
		}
	}
	return nil
}

// getSpecialValueHover returns hover for special values like true/false, colors, etc.
func getSpecialValueHover(value, fullPath string) *HoverInfo {
	switch value {
	case "true", "false":
		if isBooleanContext(fullPath) {
			return &HoverInfo{
				Contents: fmt.Sprintf("**Boolean Value**: `%s`\n\nBoolean values control on/off states for various properties.", value),
				Language: "d2",
			}
		}
	case "up", "down", "left", "right":
		if strings.Contains(fullPath, "direction") {
			return &HoverInfo{
				Contents: fmt.Sprintf("**Direction**: `%s`\n\nControls the layout direction of the diagram or container.", value),
				Language: "d2",
			}
		}
	case "top-left", "top-center", "top-right", "center-left", "center-right", "bottom-left", "bottom-center", "bottom-right":
		if strings.Contains(fullPath, "near") {
			return &HoverInfo{
				Contents: fmt.Sprintf("**Position**: `%s`\n\nPositions labels, icons, or objects relative to their parent.", value),
				Language: "d2",
			}
		}
	case "dots", "lines", "grain":
		if strings.Contains(fullPath, "fill-pattern") {
			return &HoverInfo{
				Contents: fmt.Sprintf("**Fill Pattern**: `%s`\n\nApplies a visual pattern to the object's background.", value),
				Language: "d2",
			}
		}
	case "none", "uppercase", "lowercase", "capitalize":
		if strings.Contains(fullPath, "text-transform") {
			return &HoverInfo{
				Contents: fmt.Sprintf("**Text Transform**: `%s`\n\nControls the capitalization of text within the object.", value),
				Language: "d2",
			}
		}
	}
	
	// Check for common colors
	if isColorValue(value) {
		return &HoverInfo{
			Contents: fmt.Sprintf("**Color**: `%s`\n\nColor value for styling objects. Can be a color name or hex code.", value),
			Language: "d2",
		}
	}

	return nil
}

// Helper functions
func getKeyPathString(path []*d2ast.StringBox) string {
	var parts []string
	for _, node := range path {
		parts = append(parts, node.Unbox().ScalarString())
	}
	return strings.Join(parts, ".")
}

func isStyleContext(fullPath string) bool {
	styleKeywords := []string{"fill", "stroke", "opacity", "stroke-width", "stroke-dash", "border-radius",
		"font-size", "font-color", "bold", "italic", "underline", "shadow", "multiple", "3d",
		"animated", "filled", "double-border", "fill-pattern", "text-transform", "font"}
	
	for _, keyword := range styleKeywords {
		if strings.HasSuffix(fullPath, keyword) {
			return true
		}
	}
	return false
}

func isBooleanContext(fullPath string) bool {
	booleanProps := []string{"bold", "italic", "underline", "shadow", "multiple", "3d", "animated", "filled", "double-border"}
	for _, prop := range booleanProps {
		if strings.Contains(fullPath, prop) {
			return true
		}
	}
	return false
}

func isColorValue(value string) bool {
	// Common color names
	colors := []string{"red", "green", "blue", "yellow", "orange", "purple", "pink", "cyan", "magenta",
		"black", "white", "gray", "grey", "brown", "lime", "navy", "olive", "teal", "silver", "maroon"}
	
	for _, color := range colors {
		if value == color {
			return true
		}
	}
	
	// Check for hex color pattern
	if len(value) == 7 && value[0] == '#' {
		for i := 1; i < 7; i++ {
			c := value[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
		return true
	}
	
	return false
}
