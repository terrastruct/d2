package d2svgimport

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/mazznoer/csscolorparser"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

var presentationProperties = map[string]struct{}{
	"fill": {}, "stroke": {}, "opacity": {}, "fill-opacity": {}, "stroke-opacity": {},
	"stroke-width": {}, "stroke-linecap": {}, "stroke-linejoin": {}, "stroke-miterlimit": {},
	"stroke-dasharray": {}, "stroke-dashoffset": {}, "fill-rule": {}, "clip-rule": {}, "clip-path": {}, "color": {},
	"display": {}, "visibility": {}, "transform": {}, "stop-color": {}, "stop-opacity": {},
}

type svgStyle struct {
	fill          d2scene.Paint
	stroke        d2scene.Paint
	fillCurrent   bool
	strokeCurrent bool
	color         color.NRGBA
	fillOpacity   float64
	strokeOpacity float64
	strokeWidth   float64
	lineCap       d2scene.LineCap
	lineJoin      d2scene.LineJoin
	miterLimit    float64
	dashes        []float64
	dashOffset    float64
	fillRule      d2scene.FillRule
	clipRule      d2scene.FillRule
	clipPathID    string
	opacity       float64
	display       bool
	visible       bool
}

func defaultSVGStyle() svgStyle {
	return defaultSVGStyleWithColor(color.NRGBA{A: 255})
}

func defaultSVGStyleWithColor(currentColor color.NRGBA) svgStyle {
	black := color.NRGBA{A: 255}
	return svgStyle{
		fill: d2scene.SolidPaint{Color: black}, color: currentColor,
		fillOpacity: 1, strokeOpacity: 1, strokeWidth: 1,
		lineCap: d2scene.CapButt, lineJoin: d2scene.JoinMiter, miterLimit: 4,
		fillRule: d2scene.NonZero, clipRule: d2scene.NonZero, opacity: 1, display: true, visible: true,
	}
}

// declarationsFor applies presentation attributes before an inline style, as
// required by SVG's author-style cascade. It rejects every unimplemented CSS
// property instead of silently changing the imported image.
func (i *svgImporter) declarationsFor(element *svgElement) (map[string]string, error) {
	declarations := make(map[string]string)
	for property := range presentationProperties {
		if value, ok := element.attrs[property]; ok {
			trimmed, err := trimSVGSpace(i.ctx, value)
			if err != nil {
				return nil, err
			}
			declarations[property] = trimmed
		}
	}
	for index, rule := range i.stylesheetRules {
		if index&255 == 0 {
			if err := i.ctx.Err(); err != nil {
				return nil, err
			}
		}
		if !elementHasClass(element, rule.class) {
			continue
		}
		for property, value := range rule.declarations {
			declarations[property] = value
		}
	}
	inline, ok := element.attrs["style"]
	if !ok {
		return declarations, nil
	}
	for index := 0; index < len(inline); index++ {
		if index&4095 == 0 {
			if err := i.ctx.Err(); err != nil {
				return nil, err
			}
		}
		if inline[index] == '{' || inline[index] == '}' ||
			index+1 < len(inline) && (inline[index:index+2] == "/*" || inline[index:index+2] == "*/") {
			return nil, i.errorf("element <%s> has unsupported inline CSS syntax", element.name)
		}
	}
	for start := 0; start <= len(inline); {
		if err := i.ctx.Err(); err != nil {
			return nil, err
		}
		end := start
		for end < len(inline) && inline[end] != ';' {
			end++
			if end&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return nil, err
				}
			}
		}
		raw, err := trimSVGSpace(i.ctx, inline[start:end])
		if err != nil {
			return nil, err
		}
		if end == len(inline) {
			start = len(inline) + 1
		} else {
			start = end + 1
		}
		if raw == "" {
			continue
		}
		colon := -1
		for index := 0; index < len(raw); index++ {
			if index&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return nil, err
				}
			}
			if raw[index] == ':' {
				colon = index
				break
			}
		}
		if colon <= 0 {
			return nil, i.errorf("element <%s> has malformed inline style", element.name)
		}
		property, err := trimSVGSpace(i.ctx, raw[:colon])
		if err != nil {
			return nil, err
		}
		value, err := trimSVGSpace(i.ctx, raw[colon+1:])
		if err != nil {
			return nil, err
		}
		if len(property) > 64 {
			return nil, i.errorf("element <%s> has malformed inline style", element.name)
		}
		property = strings.ToLower(property)
		if !validCSSPropertyName(property) || value == "" {
			return nil, i.errorf("element <%s> has malformed inline style", element.name)
		}
		if property == "enable-background" {
			if !element.isRoot {
				return nil, i.errorf("element <%s> has unsupported enable-background outside the root <svg>", element.name)
			}
			if err := validateEnableBackground(i.ctx, value); err != nil {
				if contextErr := i.ctx.Err(); contextErr != nil {
					return nil, contextErr
				}
				return nil, i.errorf("root <svg> has invalid enable-background: %v", err)
			}
			continue
		}
		if property == "vertical-align" {
			if !element.isRoot {
				return nil, i.errorf("element <%s> has unsupported vertical-align outside the root <svg>", element.name)
			}
			if _, err := parseMathJaxEXLength(value); err != nil {
				return nil, i.errorf("root <svg> has invalid MathJax vertical-align: %v", err)
			}
			i.mathJaxRoot = element
			continue
		}
		if _, supported := presentationProperties[property]; !supported {
			return nil, i.errorf("element <%s> has unsupported style property %q", element.name, property)
		}
		if property == "transform" {
			return nil, i.errorf("element <%s> has unsupported CSS transform; use the transform presentation attribute", element.name)
		}
		important, err := containsASCIIEqualFold(i.ctx, value, "!important")
		if err != nil {
			return nil, err
		}
		if important {
			return nil, i.errorf("element <%s> uses unsupported !important", element.name)
		}
		declarations[property] = value
	}
	return declarations, nil
}

func trimSVGSpace(ctx context.Context, value string) (string, error) {
	start := 0
	for start < len(value) && isSVGListSpace(value[start]) {
		start++
		if start&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
	}
	end := len(value)
	for end > start && isSVGListSpace(value[end-1]) {
		end--
		if (len(value)-end)&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return value[start:end], nil
}

func containsASCIIEqualFold(ctx context.Context, value, pattern string) (bool, error) {
	if len(pattern) == 0 {
		return true, nil
	}
	for start := 0; start+len(pattern) <= len(value); start++ {
		if start&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		match := true
		for index := range pattern {
			left := value[start+index]
			right := pattern[index]
			if left >= 'A' && left <= 'Z' {
				left += 'a' - 'A'
			}
			if right >= 'A' && right <= 'Z' {
				right += 'a' - 'A'
			}
			if left != right {
				match = false
				break
			}
		}
		if match {
			return true, nil
		}
	}
	return false, ctx.Err()
}

func validCSSPropertyName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && r != '-' {
			return false
		}
	}
	return true
}

func (i *svgImporter) computeStyle(parent svgStyle, element *svgElement) (svgStyle, error) {
	style := parent
	// Opacity and display do not inherit in SVG. Visibility and all of the
	// paint/stroke properties represented here do. clip-path is also reset:
	// unlike clip-rule, it is not inherited.
	style.opacity = 1
	style.display = true
	style.clipPathID = ""

	declarations := element.declarations
	if value, ok := declarations["color"]; ok {
		if equalASCIIEqualFold(value, "inherit") || equalASCIIEqualFold(value, "currentColor") {
			style.color = parent.color
		} else {
			parsed, err := parseSolidColor(value)
			if err != nil {
				return svgStyle{}, i.propertyError(element, "color", err)
			}
			style.color = parsed
		}
	}

	if value, ok := declarations["fill"]; ok {
		if equalASCIIEqualFold(value, "inherit") {
			style.fill = parent.fill
			style.fillCurrent = parent.fillCurrent
		} else {
			paint, current, err := i.parseSVGPaint(value)
			if err != nil {
				return svgStyle{}, i.propertyError(element, "fill", err)
			}
			style.fill = paint
			style.fillCurrent = current
		}
	}
	if value, ok := declarations["stroke"]; ok {
		if equalASCIIEqualFold(value, "inherit") {
			style.stroke = parent.stroke
			style.strokeCurrent = parent.strokeCurrent
		} else {
			paint, current, err := i.parseSVGPaint(value)
			if err != nil {
				return svgStyle{}, i.propertyError(element, "stroke", err)
			}
			style.stroke = paint
			style.strokeCurrent = current
		}
	}

	var err error
	if style.fillOpacity, err = i.inheritedOpacity(element, declarations, "fill-opacity", parent.fillOpacity); err != nil {
		return svgStyle{}, err
	}
	if style.strokeOpacity, err = i.inheritedOpacity(element, declarations, "stroke-opacity", parent.strokeOpacity); err != nil {
		return svgStyle{}, err
	}
	if value, ok := declarations["stroke-width"]; ok {
		if equalASCIIEqualFold(value, "inherit") {
			style.strokeWidth = parent.strokeWidth
		} else {
			style.strokeWidth, err = parseSVGLength(value, true)
			if err != nil || style.strokeWidth < 0 {
				if err == nil {
					err = fmt.Errorf("must not be negative")
				}
				return svgStyle{}, i.propertyError(element, "stroke-width", err)
			}
		}
	}
	if value, ok := declarations["stroke-linecap"]; ok {
		keyword, keywordErr := styleKeyword(value)
		if keywordErr != nil {
			return svgStyle{}, i.propertyError(element, "stroke-linecap", keywordErr)
		}
		switch keyword {
		case "inherit":
			style.lineCap = parent.lineCap
		case "butt":
			style.lineCap = d2scene.CapButt
		case "round":
			style.lineCap = d2scene.CapRound
		case "square":
			style.lineCap = d2scene.CapSquare
		default:
			return svgStyle{}, i.propertyError(element, "stroke-linecap", fmt.Errorf("unsupported value"))
		}
	}
	if value, ok := declarations["stroke-linejoin"]; ok {
		keyword, keywordErr := styleKeyword(value)
		if keywordErr != nil {
			return svgStyle{}, i.propertyError(element, "stroke-linejoin", keywordErr)
		}
		switch keyword {
		case "inherit":
			style.lineJoin = parent.lineJoin
		case "miter":
			style.lineJoin = d2scene.JoinMiter
		case "round":
			style.lineJoin = d2scene.JoinRound
		case "bevel":
			style.lineJoin = d2scene.JoinBevel
		default:
			return svgStyle{}, i.propertyError(element, "stroke-linejoin", fmt.Errorf("unsupported value"))
		}
	}
	if value, ok := declarations["stroke-miterlimit"]; ok {
		if equalASCIIEqualFold(value, "inherit") {
			style.miterLimit = parent.miterLimit
		} else {
			style.miterLimit, err = parseSVGNumber(value)
			if err != nil || style.miterLimit < 1 {
				if err == nil {
					err = fmt.Errorf("must be at least 1")
				}
				return svgStyle{}, i.propertyError(element, "stroke-miterlimit", err)
			}
		}
	}
	if value, ok := declarations["stroke-dasharray"]; ok {
		if equalASCIIEqualFold(value, "inherit") {
			style.dashes = append([]float64(nil), parent.dashes...)
		} else if equalASCIIEqualFold(value, "none") {
			style.dashes = nil
		} else {
			style.dashes, err = parseDashArray(i.ctx, value)
			if err != nil {
				if contextErr := i.ctx.Err(); contextErr != nil {
					return svgStyle{}, contextErr
				}
				return svgStyle{}, i.propertyError(element, "stroke-dasharray", err)
			}
		}
	}
	if value, ok := declarations["stroke-dashoffset"]; ok {
		if equalASCIIEqualFold(value, "inherit") {
			style.dashOffset = parent.dashOffset
		} else {
			style.dashOffset, err = parseSVGLength(value, true)
			if err != nil {
				return svgStyle{}, i.propertyError(element, "stroke-dashoffset", err)
			}
		}
	}
	if value, ok := declarations["fill-rule"]; ok {
		keyword, keywordErr := styleKeyword(value)
		if keywordErr != nil {
			return svgStyle{}, i.propertyError(element, "fill-rule", keywordErr)
		}
		switch keyword {
		case "inherit":
			style.fillRule = parent.fillRule
		case "nonzero":
			style.fillRule = d2scene.NonZero
		case "evenodd":
			style.fillRule = d2scene.EvenOdd
		default:
			return svgStyle{}, i.propertyError(element, "fill-rule", fmt.Errorf("unsupported value"))
		}
	}
	if value, ok := declarations["clip-rule"]; ok {
		keyword, keywordErr := styleKeyword(value)
		if keywordErr != nil {
			return svgStyle{}, i.propertyError(element, "clip-rule", keywordErr)
		}
		switch keyword {
		case "inherit":
			style.clipRule = parent.clipRule
		case "nonzero":
			style.clipRule = d2scene.NonZero
		case "evenodd":
			style.clipRule = d2scene.EvenOdd
		default:
			return svgStyle{}, i.propertyError(element, "clip-rule", fmt.Errorf("unsupported value"))
		}
	}
	if value, ok := declarations["clip-path"]; ok {
		if equalASCIIEqualFold(value, "none") {
			style.clipPathID = ""
		} else {
			style.clipPathID, err = i.localClipPathID(value)
			if err != nil {
				return svgStyle{}, i.propertyError(element, "clip-path", err)
			}
		}
	}
	if value, ok := declarations["opacity"]; ok {
		if equalASCIIEqualFold(value, "inherit") {
			style.opacity = parent.opacity
		} else {
			style.opacity, err = parseUnitInterval(value)
			if err != nil {
				return svgStyle{}, i.propertyError(element, "opacity", err)
			}
		}
	}
	if value, ok := declarations["display"]; ok {
		keyword, keywordErr := styleKeyword(value)
		if keywordErr != nil {
			return svgStyle{}, i.propertyError(element, "display", keywordErr)
		}
		switch keyword {
		case "inherit":
			style.display = parent.display
		case "none":
			style.display = false
		case "inline", "block":
			style.display = true
		default:
			return svgStyle{}, i.propertyError(element, "display", fmt.Errorf("unsupported value"))
		}
	}
	if value, ok := declarations["visibility"]; ok {
		keyword, keywordErr := styleKeyword(value)
		if keywordErr != nil {
			return svgStyle{}, i.propertyError(element, "visibility", keywordErr)
		}
		switch keyword {
		case "inherit":
			style.visible = parent.visible
		case "visible":
			style.visible = true
		case "hidden", "collapse":
			style.visible = false
		default:
			return svgStyle{}, i.propertyError(element, "visibility", fmt.Errorf("unsupported value"))
		}
	}
	return style, nil
}

func styleKeyword(value string) (string, error) {
	if len(value) > 64 {
		return "", fmt.Errorf("keyword is too long")
	}
	return strings.ToLower(value), nil
}

func (i *svgImporter) inheritedOpacity(element *svgElement, declarations map[string]string, property string, inherited float64) (float64, error) {
	value, ok := declarations[property]
	if !ok || equalASCIIEqualFold(value, "inherit") {
		return inherited, nil
	}
	parsed, err := parseUnitInterval(value)
	if err != nil {
		return 0, i.propertyError(element, property, err)
	}
	return parsed, nil
}

func (i *svgImporter) propertyError(element *svgElement, property string, err error) error {
	return i.errorf("element <%s> has invalid %s: %v", element.name, property, err)
}

func (i *svgImporter) parseSVGPaint(value string) (d2scene.Paint, bool, error) {
	if len(value) > 4096 {
		return nil, false, fmt.Errorf("paint value is too long")
	}
	trimmed := strings.TrimSpace(value)
	switch {
	case equalASCIIEqualFold(trimmed, "none"):
		return nil, false, nil
	case equalASCIIEqualFold(trimmed, "currentColor"):
		return nil, true, nil
	case strings.Contains(strings.ToLower(trimmed), "url("):
		paint, err := i.localLinearGradientPaint(trimmed)
		return paint, false, err
	}
	parsed, err := parseSolidColor(trimmed)
	if err != nil {
		return nil, false, err
	}
	return d2scene.SolidPaint{Color: parsed}, false, nil
}

func parseSolidColor(value string) (color.NRGBA, error) {
	if len(value) > 4096 {
		return color.NRGBA{}, fmt.Errorf("solid color is too long")
	}
	parsed, err := csscolorparser.Parse(strings.TrimSpace(value))
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("unsupported solid color")
	}
	r, g, b, a := parsed.RGBA255()
	return color.NRGBA{R: r, G: g, B: b, A: a}, nil
}

func parseUnitInterval(value string) (float64, error) {
	if len(value) > 512 {
		return 0, fmt.Errorf("number is too long")
	}
	if strings.Contains(value, "%") {
		return 0, fmt.Errorf("percentages are unsupported")
	}
	number, err := parseSVGNumber(value)
	if err != nil {
		return 0, err
	}
	if number < 0 || number > 1 {
		return 0, fmt.Errorf("must be in [0,1]")
	}
	return number, nil
}

func parseSVGLength(value string, allowPX bool) (float64, error) {
	if len(value) > 512 {
		return 0, fmt.Errorf("number is too long")
	}
	trimmed := strings.TrimSpace(value)
	if strings.Contains(trimmed, "%") {
		return 0, fmt.Errorf("percentages are unsupported")
	}
	scale := 1.0
	unitBytes := 0
	if allowPX {
		absoluteUnits := []struct {
			suffix string
			scale  float64
		}{
			{suffix: "px", scale: 1},
			{suffix: "in", scale: 96},
			{suffix: "cm", scale: 96 / 2.54},
			{suffix: "mm", scale: 96 / 25.4},
			{suffix: "pt", scale: 96.0 / 72.0},
			{suffix: "pc", scale: 16},
			{suffix: "q", scale: 96 / 101.6},
		}
		for _, unit := range absoluteUnits {
			if len(trimmed) >= len(unit.suffix) && equalASCIIEqualFold(trimmed[len(trimmed)-len(unit.suffix):], unit.suffix) {
				unitBytes = len(unit.suffix)
				scale = unit.scale
				break
			}
		}
	}
	if unitBytes != 0 {
		number := trimmed[:len(trimmed)-unitBytes]
		if strings.TrimSpace(number) != number {
			return 0, fmt.Errorf("units must immediately follow their number")
		}
		trimmed = number
	}
	number, err := parseSVGNumber(trimmed)
	if err != nil {
		return 0, err
	}
	result := number * scale
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, fmt.Errorf("absolute length must be finite")
	}
	return result, nil
}

const mathJaxEXPixels = 8.0

// parseMathJaxEXLength implements the frozen mathjax-go contract used by
// d2latex.Measure: MathJax emits root dimensions and baseline offsets in ex,
// and its measurement adapter treats one ex as eight CSS pixels. It is kept
// separate from parseSVGLength so relative ex units remain rejected everywhere
// else in the imported SVG subset.
func parseMathJaxEXLength(value string) (float64, error) {
	if len(value) > 512 {
		return 0, fmt.Errorf("number is too long")
	}
	trimmed := strings.TrimSpace(value)
	if len(trimmed) <= len("ex") || !equalASCIIEqualFold(trimmed[len(trimmed)-len("ex"):], "ex") {
		return 0, fmt.Errorf("expected a finite ex length")
	}
	number := trimmed[:len(trimmed)-len("ex")]
	if strings.TrimSpace(number) != number {
		return 0, fmt.Errorf("units must immediately follow their number")
	}
	parsed, err := parseSVGNumber(number)
	if err != nil {
		return 0, fmt.Errorf("expected a finite ex length")
	}
	result := parsed * mathJaxEXPixels
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0, fmt.Errorf("ex length must be finite")
	}
	return result, nil
}

func parseSVGNumber(value string) (float64, error) {
	if len(value) > 512 {
		return 0, fmt.Errorf("number is too long")
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("empty number")
	}
	offset := 0
	if trimmed[offset] == '+' || trimmed[offset] == '-' {
		offset++
	}
	digits := 0
	for offset < len(trimmed) && trimmed[offset] >= '0' && trimmed[offset] <= '9' {
		offset++
		digits++
	}
	if offset < len(trimmed) && trimmed[offset] == '.' {
		offset++
		for offset < len(trimmed) && trimmed[offset] >= '0' && trimmed[offset] <= '9' {
			offset++
			digits++
		}
	}
	if digits == 0 {
		return 0, fmt.Errorf("expected a finite unitless number or px length")
	}
	if offset < len(trimmed) && (trimmed[offset] == 'e' || trimmed[offset] == 'E') {
		offset++
		if offset < len(trimmed) && (trimmed[offset] == '+' || trimmed[offset] == '-') {
			offset++
		}
		exponentDigits := 0
		for offset < len(trimmed) && trimmed[offset] >= '0' && trimmed[offset] <= '9' {
			offset++
			exponentDigits++
		}
		if exponentDigits == 0 {
			return 0, fmt.Errorf("expected a finite unitless number or px length")
		}
	}
	if offset != len(trimmed) {
		return 0, fmt.Errorf("expected a finite unitless number or px length")
	}
	number, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("expected a finite unitless number or px length")
	}
	return number, nil
}

func parseDashArray(ctx context.Context, value string) ([]float64, error) {
	parts, err := splitSVGNumberList(ctx, value, -1)
	if err != nil || len(parts) == 0 {
		if err == nil {
			err = fmt.Errorf("expected at least one dash length")
		}
		return nil, err
	}
	dashes := make([]float64, len(parts))
	positive := false
	hasZero := false
	for index, part := range parts {
		dashes[index], err = parseSVGLength(part, true)
		if err != nil {
			return nil, err
		}
		if dashes[index] < 0 {
			return nil, fmt.Errorf("dash lengths must not be negative")
		}
		positive = positive || dashes[index] > 0
		hasZero = hasZero || dashes[index] == 0
	}
	if !positive {
		return nil, nil
	}
	if hasZero {
		return nil, fmt.Errorf("mixed zero-length dash entries are unsupported")
	}
	if len(dashes)%2 == 1 {
		dashes = append(dashes, dashes...)
	}
	total := 0.0
	for _, dash := range dashes {
		total += dash
		if math.IsInf(total, 0) || math.IsNaN(total) {
			return nil, fmt.Errorf("dash total must be finite")
		}
	}
	return dashes, nil
}

// splitSVGNumberList preserves exponent signs while accepting SVG's comma or
// whitespace separators. Empty comma-separated fields are rejected.
var errSVGNumberListLimit = errors.New("SVG number list exceeds item limit")

func splitSVGNumberList(ctx context.Context, value string, maxFields int) ([]string, error) {
	var fields []string
	start := -1
	needValue := true
	flush := func(end int) error {
		if start >= 0 {
			if maxFields >= 0 && len(fields) >= maxFields {
				return errSVGNumberListLimit
			}
			fields = append(fields, value[start:end])
			start = -1
			needValue = false
		}
		return nil
	}
	for index := 0; index < len(value); index++ {
		if index&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		character := value[index]
		if (character == '+' || character == '-') && start >= 0 && value[index-1] != 'e' && value[index-1] != 'E' {
			if err := flush(index); err != nil {
				return nil, err
			}
			start = index
			continue
		}
		if character == ',' {
			if err := flush(index); err != nil {
				return nil, err
			}
			if needValue {
				return nil, fmt.Errorf("empty comma-separated number")
			}
			needValue = true
			continue
		}
		if isSVGListSpace(character) {
			if err := flush(index); err != nil {
				return nil, err
			}
			continue
		}
		if start < 0 {
			start = index
		}
	}
	if err := flush(len(value)); err != nil {
		return nil, err
	}
	if needValue && len(fields) != 0 {
		return nil, fmt.Errorf("trailing comma")
	}
	return fields, nil
}

func isSVGListSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

func paintWithOpacity(ctx context.Context, paint d2scene.Paint, opacity float64) (d2scene.Paint, error) {
	if paint == nil {
		return nil, nil
	}
	switch paint := paint.(type) {
	case d2scene.SolidPaint:
		paint.Color.A = uint8(math.Round(float64(paint.Color.A) * opacity))
		return paint, nil
	case d2scene.LinearGradient:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		stops := make([]d2scene.GradientStop, len(paint.Stops))
		for index, stop := range paint.Stops {
			if index&255 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			stops[index] = stop
		}
		paint.Stops = stops
		for index := range stops {
			if index&255 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			paint.Stops[index].Color.A = uint8(math.Round(float64(paint.Stops[index].Color.A) * opacity))
		}
		return paint, nil
	default:
		return nil, nil
	}
}
