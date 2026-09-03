package d2svgimport

import (
	"fmt"
	"image/color"
	"math"
	"math/big"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

var stopPresentationProperties = map[string]struct{}{
	"stop-color":   {},
	"stop-opacity": {},
}

func isGradientResourceElement(name string) bool {
	return name == "linearGradient" || name == "stop"
}

// validateGradientTree keeps paint servers out of the rendering tree. The
// supported subset deliberately has no resource-to-resource edges: gradients
// are direct children of <defs>, stops are direct children of a gradient, and
// href inheritance is rejected by the exact attribute allowlist.
func (i *svgImporter) validateGradientTree(root *svgElement) error {
	type entry struct {
		element *svgElement
		parent  *svgElement
	}
	stack := []entry{{element: root}}
	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		switch current.element.name {
		case "linearGradient":
			if current.parent == nil || current.parent.name != "defs" {
				return i.errorf("element <linearGradient> must be a direct child of <defs>")
			}
			if current.element.id == "" {
				return i.errorf("element <linearGradient> must declare an id")
			}
			if len(current.element.children) == 0 {
				return i.errorf("element <linearGradient> must contain at least one <stop>")
			}
			for _, child := range current.element.children {
				if child.name != "stop" {
					return i.errorf("element <linearGradient> has unsupported child <%s>", displayXMLName(child.name))
				}
			}
		case "stop":
			if current.parent == nil || current.parent.name != "linearGradient" {
				return i.errorf("element <stop> is only supported inside <linearGradient>")
			}
			if len(current.element.children) != 0 {
				return i.errorf("element <stop> cannot contain child elements")
			}
		}
		for index := len(current.element.children) - 1; index >= 0; index-- {
			stack = append(stack, entry{element: current.element.children[index], parent: current.element})
		}
	}
	return nil
}

func (i *svgImporter) compileGradientElement(element *svgElement) error {
	allowed := elementAttributes[element.name]
	for _, name := range element.attrOrder {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		if _, ok := allowed[name]; !ok {
			return i.errorf("element <%s> has unsupported attribute %q", element.name, displayXMLName(name))
		}
	}

	switch element.name {
	case "linearGradient":
		return i.compileLinearGradient(element)
	case "stop":
		return i.compileGradientStop(element)
	default:
		return i.errorf("internal unsupported gradient element <%s>", element.name)
	}
}

func (i *svgImporter) compileLinearGradient(element *svgElement) error {
	for _, name := range []string{"x1", "y1", "x2", "y2", "gradientUnits"} {
		if _, ok := element.attrs[name]; !ok {
			return i.errorf("element <linearGradient> is missing required attribute %q", name)
		}
	}
	units, err := trimSVGSpace(i.ctx, element.attrs["gradientUnits"])
	if err != nil {
		return err
	}
	if units != "userSpaceOnUse" {
		return i.errorf("element <linearGradient> supports only gradientUnits=\"userSpaceOnUse\"")
	}

	coordinate := func(name string) (float64, error) {
		value, err := parseSVGLength(element.attrs[name], true)
		if err != nil {
			return 0, i.propertyError(element, name, err)
		}
		return value, nil
	}
	x1, err := coordinate("x1")
	if err != nil {
		return err
	}
	y1, err := coordinate("y1")
	if err != nil {
		return err
	}
	x2, err := coordinate("x2")
	if err != nil {
		return err
	}
	y2, err := coordinate("y2")
	if err != nil {
		return err
	}
	start := d2scene.Point{X: x1, Y: y1}
	end := d2scene.Point{X: x2, Y: y2}
	if start == end {
		return i.errorf("element <linearGradient> has a zero-length gradient vector")
	}

	if raw, ok := element.attrs["spreadMethod"]; ok {
		spread, err := trimSVGSpace(i.ctx, raw)
		if err != nil {
			return err
		}
		if spread != "pad" {
			return i.errorf("element <linearGradient> supports only the pad spread method")
		}
	}

	transform := d2scene.Identity()
	if raw, ok := element.attrs["gradientTransform"]; ok {
		trimmed, err := trimSVGSpace(i.ctx, raw)
		if err != nil {
			return err
		}
		if trimmed == "" {
			return i.errorf("element <linearGradient> has an empty gradientTransform")
		}
		remaining := i.limits.MaxTransformFunctions - i.parsedTransforms
		if remaining <= 0 {
			return i.errorf("transform function count exceeds limit %d", i.limits.MaxTransformFunctions)
		}
		var functions int
		transform, functions, err = parseTransformWithCount(i.ctx, i.source, trimmed, transformLimits{
			MaxBytes: i.limits.MaxAttributeBytes, MaxFunctions: remaining,
		})
		if err != nil {
			return err
		}
		i.parsedTransforms += functions
	}
	if !affineInverseIsFinite(transform) {
		return i.errorf("element <linearGradient> has a singular or unrepresentable gradientTransform")
	}
	direction := transform.Vector(d2scene.Point{X: end.X - start.X, Y: end.Y - start.Y})
	if math.IsNaN(direction.X) || math.IsInf(direction.X, 0) || math.IsNaN(direction.Y) || math.IsInf(direction.Y, 0) ||
		direction == (d2scene.Point{}) {
		return i.errorf("element <linearGradient> has invalid transformed gradient geometry")
	}

	element.gradient = &d2scene.LinearGradient{
		Start: start, End: end, Units: d2scene.UserSpaceOnUse,
		Transform: transform, Spread: d2scene.SpreadPad,
	}
	return nil
}

func (i *svgImporter) compileGradientStop(element *svgElement) error {
	classes, classSet, err := i.classTokens(element)
	if err != nil {
		return err
	}
	element.classes = classes
	element.classSet = classSet
	declarations, err := i.declarationsFor(element)
	if err != nil {
		return err
	}
	element.declarations = declarations
	for property := range declarations {
		if _, ok := stopPresentationProperties[property]; !ok {
			return i.errorf("element <stop> has unsupported style property %q", property)
		}
	}

	rawOffset, ok := element.attrs["offset"]
	if !ok {
		return i.errorf("element <stop> is missing offset")
	}
	offset, err := parseUnitInterval(rawOffset)
	if err != nil {
		return i.propertyError(element, "offset", err)
	}
	stop, err := i.stopFromDeclarations(element)
	if err != nil {
		return err
	}
	stop.Offset = offset
	element.gradientStop = &stop
	return nil
}

func (i *svgImporter) stopFromDeclarations(element *svgElement) (d2scene.GradientStop, error) {
	stopColor := color.NRGBA{A: 255}
	if raw, ok := element.declarations["stop-color"]; ok {
		if equalASCIIEqualFold(raw, "inherit") || equalASCIIEqualFold(raw, "currentColor") || strings.Contains(strings.ToLower(raw), "url(") {
			return d2scene.GradientStop{}, i.propertyError(element, "stop-color", fmt.Errorf("only an explicit solid color is supported"))
		}
		parsed, err := parseSolidColor(raw)
		if err != nil {
			return d2scene.GradientStop{}, i.propertyError(element, "stop-color", err)
		}
		stopColor = parsed
	}
	opacity := 1.0
	if raw, ok := element.declarations["stop-opacity"]; ok {
		if equalASCIIEqualFold(raw, "inherit") {
			return d2scene.GradientStop{}, i.propertyError(element, "stop-opacity", fmt.Errorf("inheritance is unsupported"))
		}
		parsed, err := parseUnitInterval(raw)
		if err != nil {
			return d2scene.GradientStop{}, i.propertyError(element, "stop-opacity", err)
		}
		opacity = parsed
	}
	stopColor.A = uint8(math.Round(float64(stopColor.A) * opacity))
	return d2scene.GradientStop{Color: stopColor}, nil
}

func (i *svgImporter) compileGradientResources(root *svgElement) error {
	stack := []*svgElement{root}
	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		element := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if element.name == "linearGradient" {
			if element.gradient == nil {
				return i.errorf("internal uncompiled linear gradient")
			}
			stops := make([]d2scene.GradientStop, len(element.children))
			previous := -1.0
			for index, child := range element.children {
				if err := i.ctx.Err(); err != nil {
					return err
				}
				if child.gradientStop == nil {
					return i.errorf("internal uncompiled gradient stop")
				}
				stop := *child.gradientStop
				if stop.Offset < previous {
					return i.errorf("element <linearGradient> has decreasing stop offsets")
				}
				stops[index] = stop
				previous = stop.Offset
			}
			element.gradient.Stops = stops
		}
		for index := len(element.children) - 1; index >= 0; index-- {
			stack = append(stack, element.children[index])
		}
	}
	return nil
}

func (i *svgImporter) validateRegularDeclarations(element *svgElement) error {
	for property := range element.declarations {
		if _, stopOnly := stopPresentationProperties[property]; stopOnly {
			return i.errorf("element <%s> has unsupported style property %q", element.name, property)
		}
		if element.name == "defs" && property == "clip-path" {
			return i.errorf("element <defs> cannot carry clip-path because it is not an emitted element")
		}
	}
	return nil
}

// validateStylesheetRules validates every class rule, including unmatched
// rules. Stop declarations and ordinary rendering declarations cannot be
// mixed because no supported element accepts both.
func (i *svgImporter) validateStylesheetRules() error {
	for _, rule := range i.stylesheetRules {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		regular := make(map[string]string)
		stops := make(map[string]string)
		for property, value := range rule.declarations {
			if _, stopOnly := stopPresentationProperties[property]; stopOnly {
				stops[property] = value
			} else {
				regular[property] = value
			}
		}
		if len(regular) != 0 && len(stops) != 0 {
			return i.errorf("stylesheet rule .%s mixes stop-only and rendering properties", rule.class)
		}
		if len(stops) != 0 {
			probe := &svgElement{name: "stop", declarations: stops}
			if _, err := i.stopFromDeclarations(probe); err != nil {
				return err
			}
			continue
		}
		probe := &svgElement{name: "style", declarations: regular}
		if _, err := i.computeStyle(defaultSVGStyle(), probe); err != nil {
			return err
		}
	}
	return nil
}

func (i *svgImporter) localLinearGradientPaint(value string) (d2scene.Paint, error) {
	if !strings.HasPrefix(value, "url(#") || !strings.HasSuffix(value, ")") {
		return nil, fmt.Errorf("paint servers require one local url(#id) reference; external references and fallbacks are forbidden")
	}
	id := value[len("url(#") : len(value)-1]
	valid, err := i.validLocalPaintID(id)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("paint servers require one local url(#id) reference; external references and fallbacks are forbidden")
	}
	target := i.ids[id]
	if target == nil {
		return nil, fmt.Errorf("paint server references an unknown local id")
	}
	if target.name != "linearGradient" || target.gradient == nil {
		return nil, fmt.Errorf("paint server local id does not name a supported linear gradient")
	}
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	gradient := *target.gradient
	gradient.Stops = make([]d2scene.GradientStop, len(target.gradient.Stops))
	for index, stop := range target.gradient.Stops {
		if index&255 == 0 {
			if err := i.ctx.Err(); err != nil {
				return nil, err
			}
		}
		gradient.Stops[index] = stop
	}
	return gradient, nil
}

// validLocalPaintID accepts the bounded unescaped identifier spelling used by
// the corpus. Broader SVG IDs would require a real CSS token/escape parser and
// are rejected instead of being reinterpreted by string slicing.
func (i *svgImporter) validLocalPaintID(id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	for index := 0; index < len(id); index++ {
		if index&4095 == 0 {
			if err := i.ctx.Err(); err != nil {
				return false, err
			}
		}
		character := id[index]
		if index == 0 {
			if !isASCIIAlpha(character) && character != '_' {
				return false, nil
			}
			continue
		}
		if !isASCIIAlpha(character) && (character < '0' || character > '9') &&
			character != '_' && character != '-' && character != '.' && character != ':' {
			return false, nil
		}
	}
	return true, i.ctx.Err()
}

// affineInverseIsFinite uses exact rational arithmetic for the determinant and
// inverse coefficients. Raw float64 determinants can underflow or overflow for
// finite, invertible SVG matrices such as scale(1e300,1e-300).
func affineInverseIsFinite(matrix d2scene.Matrix) bool {
	if !matrix.IsFinite() {
		return false
	}
	rat := func(value float64) *big.Rat {
		return new(big.Rat).SetFloat64(value)
	}
	mul := func(left, right *big.Rat) *big.Rat {
		return new(big.Rat).Mul(left, right)
	}
	sub := func(left, right *big.Rat) *big.Rat {
		return new(big.Rat).Sub(left, right)
	}
	a, b, c, d := rat(matrix.A), rat(matrix.B), rat(matrix.C), rat(matrix.D)
	e, f := rat(matrix.E), rat(matrix.F)
	determinant := sub(mul(a, d), mul(b, c))
	if determinant.Sign() == 0 {
		return false
	}
	numerators := []*big.Rat{
		d,
		new(big.Rat).Neg(b),
		new(big.Rat).Neg(c),
		a,
		sub(mul(c, f), mul(d, e)),
		sub(mul(b, e), mul(a, f)),
	}
	for _, numerator := range numerators {
		value, _ := new(big.Rat).Quo(numerator, determinant).Float64()
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}
