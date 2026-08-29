package d2svgimport

import (
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

const (
	frozenMathJaxMicro           = "µ"
	frozenMathJaxUnitClass       = "MathML-Unit"
	frozenMathJaxNodeMetadata    = "data-mml-node"
	frozenMathJaxVariantMetadata = "data-variant"
)

// frozenMathJaxMicroOutline is an independently authored geometric micro sign
// at the 884-unit scale emitted by D2's frozen MathJax renderer. Its two
// vertical strokes, rounded bowl, descender, and short terminal are ordinary
// scene geometry; they are not extracted from a font.
var frozenMathJaxMicroOutline = [...]d2scene.PathCommand{
	d2scene.MoveTo(56, -360),
	d2scene.LineTo(136, -360),
	d2scene.LineTo(136, -92),
	d2scene.CubicTo(136, -35, 172, 0, 224, 0),
	d2scene.CubicTo(276, 0, 316, -35, 316, -92),
	d2scene.LineTo(316, -360),
	d2scene.LineTo(396, -360),
	d2scene.LineTo(396, -84),
	d2scene.CubicTo(396, -38, 416, -16, 456, -16),
	d2scene.LineTo(456, 58),
	d2scene.CubicTo(407, 58, 371, 39, 346, 9),
	d2scene.CubicTo(316, 53, 267, 76, 210, 76),
	d2scene.CubicTo(181, 76, 156, 68, 136, 52),
	d2scene.LineTo(136, 188),
	d2scene.LineTo(56, 188),
	d2scene.ClosePath(),
}

// frozenMathJaxTextCommands converts the one platform-font fallback present
// in D2's frozen MathJax corpus into ordinary scene path commands.
func (i *svgImporter) frozenMathJaxTextCommands(element *svgElement) ([]d2scene.PathCommand, error) {
	if err := i.validateFrozenMathJaxTextElement(element); err != nil {
		return nil, err
	}
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	if len(frozenMathJaxMicroOutline) > i.limits.MaxPathCommands {
		return nil, i.errorf("emitted path command count exceeds limit %d", i.limits.MaxPathCommands)
	}
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	return append([]d2scene.PathCommand(nil), frozenMathJaxMicroOutline[:]...), nil
}

func (i *svgImporter) validateFrozenMathJaxTextElement(element *svgElement) error {
	if element == nil || element.name != "text" {
		return i.errorf("internal MathJax text validation on a non-text element")
	}
	if len(element.children) != 0 {
		return i.errorf("frozen MathJax <text> fallback cannot contain child elements")
	}
	if string(element.text) != frozenMathJaxMicro {
		return i.errorf("frozen MathJax <text> fallback supports only one U+00B5 micro sign")
	}
	if len(element.attrOrder) != 3 || !hasOnlyAttributes(element, "transform", "font-size", "font-family") {
		return i.errorf("frozen MathJax <text> fallback has unsupported layout or presentation attributes")
	}
	if element.attrs["transform"] != "scale(1,-1)" || element.attrs["font-size"] != "884px" || element.attrs["font-family"] != "serif" {
		return i.errorf("frozen MathJax <text> fallback must use scale(1,-1), 884px, and serif")
	}
	if len(element.metadata) != 1 || element.metadata[frozenMathJaxVariantMetadata] != "normal" {
		return i.errorf("frozen MathJax <text> fallback must declare data-variant=\"normal\"")
	}
	root := element
	for root.parent != nil {
		root = root.parent
	}
	if !i.isFrozenMathJaxRoot(root) {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		return i.errorf("painted <text> is supported only inside a verified frozen MathJax root")
	}

	unit := element.parent
	if unit == nil || unit.name != "g" || len(unit.children) != 1 ||
		len(unit.classSet) != 1 || !elementHasClass(unit, frozenMathJaxUnitClass) ||
		len(unit.metadata) != 1 || unit.metadata[frozenMathJaxNodeMetadata] != "mi" ||
		!hasOnlyOptionalUnitAttributes(unit) {
		return i.errorf("frozen MathJax <text> fallback must be the sole child of an mi.MathML-Unit group")
	}
	if raw, ok := unit.attrs["transform"]; ok {
		if unit.transform.A != 1 || unit.transform.B != 0 || unit.transform.C != 0 || unit.transform.D != 1 ||
			unit.transform.E < 0 || unit.transform.F != 0 || !unit.transform.IsFinite() || !strings.HasPrefix(raw, "translate(") {
			return i.errorf("frozen MathJax unit group has unsupported placement transform")
		}
	}

	wrapper := root.children[0]
	mathNode := wrapper.children[0]
	insideMath := false
	for ancestor := unit.parent; ancestor != nil && ancestor != root; ancestor = ancestor.parent {
		if ancestor == mathNode {
			insideMath = true
			break
		}
	}
	if !insideMath {
		return i.errorf("frozen MathJax <text> fallback is outside the root math node")
	}
	return i.ctx.Err()
}

func (i *svgImporter) isFrozenMathJaxRoot(root *svgElement) bool {
	if root == nil || root != i.mathJaxRoot || !root.isRoot || root.name != "svg" || i.elementNamespace != svgNamespace ||
		len(root.attrOrder) != 4 || !hasOnlyAttributes(root, "style", "width", "height", "viewBox") ||
		len(root.metadata) != 2 || root.metadata["role"] != "img" || root.metadata["focusable"] != "false" ||
		!isFrozenMathJaxVerticalAlign(root.attrs["style"]) {
		return false
	}
	width, widthErr := parseMathJaxEXLength(root.attrs["width"])
	height, heightErr := parseMathJaxEXLength(root.attrs["height"])
	viewBox, hasViewBox, viewBoxErr := i.parseViewBoxFor(root, "root")
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 || viewBoxErr != nil || !hasViewBox || viewBox.X != 0 {
		return false
	}
	if len(root.children) != 1 {
		return false
	}
	wrapper := root.children[0]
	if wrapper.name != "g" || len(wrapper.attrOrder) != 4 ||
		!hasOnlyAttributes(wrapper, "stroke", "fill", "stroke-width", "transform") ||
		wrapper.attrs["stroke"] != "currentColor" || wrapper.attrs["fill"] != "currentColor" ||
		wrapper.attrs["stroke-width"] != "0" || wrapper.attrs["transform"] != "scale(1,-1)" ||
		len(wrapper.metadata) != 0 || wrapper.id != "" || len(wrapper.classes) != 0 || len(wrapper.children) != 1 {
		return false
	}
	mathNode := wrapper.children[0]
	return mathNode.name == "g" && len(mathNode.attrOrder) == 0 && len(mathNode.metadata) == 1 &&
		mathNode.metadata[frozenMathJaxNodeMetadata] == "math" && mathNode.id == "" && len(mathNode.classes) == 0
}

func isFrozenMathJaxVerticalAlign(style string) bool {
	trimmed := strings.TrimSpace(style)
	if !strings.HasPrefix(trimmed, "vertical-align:") {
		return false
	}
	value := strings.TrimSpace(strings.TrimPrefix(trimmed, "vertical-align:"))
	if strings.HasSuffix(value, ";") {
		value = strings.TrimSpace(strings.TrimSuffix(value, ";"))
	}
	if strings.Contains(value, ";") {
		return false
	}
	_, err := parseMathJaxEXLength(value)
	return err == nil
}

func hasOnlyAttributes(element *svgElement, names ...string) bool {
	if len(element.attrs) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := element.attrs[name]; !ok {
			return false
		}
	}
	return true
}

func hasOnlyOptionalUnitAttributes(element *svgElement) bool {
	if len(element.attrs) == 1 {
		_, class := element.attrs["class"]
		return class
	}
	if len(element.attrs) == 2 {
		_, class := element.attrs["class"]
		_, transform := element.attrs["transform"]
		return class && transform
	}
	return false
}

func (i *svgImporter) validateFrozenMathJaxTextStyle(element *svgElement, style svgStyle) error {
	_, explicitSolidFill := style.fill.(d2scene.SolidPaint)
	if !style.display || !style.visible || (!style.fillCurrent && !explicitSolidFill) || style.fillOpacity != 1 ||
		style.strokeWidth != 0 || style.fillRule != d2scene.NonZero || style.clipPathID != "" || style.opacity != 1 {
		return i.errorf("frozen MathJax <text> fallback has unsupported inherited text paint or visibility")
	}
	return nil
}
