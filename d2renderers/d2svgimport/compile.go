package d2svgimport

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

var commonAttributes = func() map[string]struct{} {
	attributes := map[string]struct{}{"id": {}, "class": {}, "style": {}}
	for property := range presentationProperties {
		attributes[property] = struct{}{}
	}
	return attributes
}()

var elementAttributes = map[string]map[string]struct{}{
	"svg": {
		"width": {}, "height": {}, "viewBox": {}, "preserveAspectRatio": {},
		"version": {}, "x": {}, "y": {}, "enable-background": {},
	},
	"g": {}, "defs": {}, "style": {"type": {}},
	"title":     {"id": {}},
	"metadata":  {"id": {}},
	"rdf:RDF":   {},
	"cc:Work":   {},
	"dc:format": {},
	"dc:type":   {},
	"sodipodi:namedview": {
		"id": {}, "pagecolor": {}, "bordercolor": {}, "borderopacity": {},
		"objecttolerance": {}, "gridtolerance": {}, "guidetolerance": {},
		"fit-margin-bottom": {}, "fit-margin-left": {}, "fit-margin-right": {}, "fit-margin-top": {},
		"showgrid": {},
	},
	"path":     {"d": {}},
	"rect":     {"x": {}, "y": {}, "width": {}, "height": {}, "rx": {}, "ry": {}},
	"circle":   {"cx": {}, "cy": {}, "r": {}},
	"ellipse":  {"cx": {}, "cy": {}, "rx": {}, "ry": {}},
	"line":     {"x1": {}, "y1": {}, "x2": {}, "y2": {}},
	"polyline": {"points": {}},
	"polygon":  {"points": {}},
	"image":    {"x": {}, "y": {}, "width": {}, "height": {}, "href": {}, "preserveAspectRatio": {}},
	"text":     {"font-size": {}, "font-family": {}},
	"use":      {"href": {}, "x": {}, "y": {}, "overflow": {}},
	"clipPath": {"id": {}, "clipPathUnits": {}},
	"linearGradient": {
		"id": {}, "x1": {}, "y1": {}, "x2": {}, "y2": {},
		"gradientUnits": {}, "gradientTransform": {}, "spreadMethod": {},
	},
	"stop": {"offset": {}, "class": {}, "style": {}, "stop-color": {}, "stop-opacity": {}},
}

func (i *svgImporter) compile(root *svgElement) error {
	if err := i.validateNonRenderingTree(root); err != nil {
		return err
	}
	if err := i.validateGradientTree(root); err != nil {
		return err
	}
	if err := i.validateClipTree(root); err != nil {
		return err
	}
	if err := i.compileStylesheets(root); err != nil {
		return err
	}
	stack := []*svgElement{root}
	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		element := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if err := i.compileElement(element); err != nil {
			return err
		}
		for index := len(element.children) - 1; index >= 0; index-- {
			stack = append(stack, element.children[index])
		}
	}
	if err := i.compileGradientResources(root); err != nil {
		return err
	}
	if err := i.validateStylesheetRules(); err != nil {
		return err
	}
	if err := i.validateUseGraph(root); err != nil {
		return err
	}
	if err := i.validateStyleTree(root); err != nil {
		return err
	}
	return i.compileClipPathResources(root)
}

func (i *svgImporter) validateUseGraph(root *svgElement) error {
	type frame struct {
		element *svgElement
		next    int
	}
	state := make(map[*svgElement]uint8, i.parsedElements)
	finishOrder := make([]*svgElement, 0, i.parsedElements)
	state[root] = 1
	stack := []frame{{element: root}}
	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		current := &stack[len(stack)-1]
		edgeCount := len(current.element.children)
		if current.element.name == "use" {
			edgeCount++
		}
		if current.next == edgeCount {
			state[current.element] = 2
			finishOrder = append(finishOrder, current.element)
			stack = stack[:len(stack)-1]
			continue
		}
		var next *svgElement
		if current.next < len(current.element.children) {
			next = current.element.children[current.next]
		} else {
			next = i.ids[current.element.href]
		}
		current.next++
		if next == nil {
			return i.errorf("element <use> references an unknown local id")
		}
		switch state[next] {
		case 1:
			return i.errorf("local <use> reference cycle detected")
		case 2:
			continue
		default:
			state[next] = 1
			stack = append(stack, frame{element: next})
		}
	}
	maxUseDepth := make(map[*svgElement]int, len(finishOrder))
	for _, element := range finishOrder {
		depth := 0
		for _, child := range element.children {
			if maxUseDepth[child] > depth {
				depth = maxUseDepth[child]
			}
		}
		if element.name == "use" {
			depth = 1 + maxUseDepth[i.ids[element.href]]
		}
		if depth > i.limits.MaxUseDepth {
			return i.errorf("local <use> expansion depth exceeds limit %d", i.limits.MaxUseDepth)
		}
		maxUseDepth[element] = depth
	}
	return nil
}

func (i *svgImporter) validateStyleTree(root *svgElement) error {
	type entry struct {
		element *svgElement
		parent  svgStyle
	}
	stack := []entry{{element: root, parent: defaultSVGStyleWithColor(i.rootCurrentColor)}}
	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if isNonRenderingElement(current.element.name) || isGradientResourceElement(current.element.name) || isClipPathResourceElement(current.element.name) {
			continue
		}
		style, err := i.computeStyle(current.parent, current.element)
		if err != nil {
			return err
		}
		if current.element.name == "text" {
			if err := i.validateFrozenMathJaxTextStyle(current.element, style); err != nil {
				return err
			}
		}
		if err := i.validateDegenerateStrokeCaps(current.element, style); err != nil {
			return err
		}
		for index := len(current.element.children) - 1; index >= 0; index-- {
			stack = append(stack, entry{element: current.element.children[index], parent: style})
		}
	}
	return nil
}

func (i *svgImporter) validateDegenerateStrokeCaps(element *svgElement, style svgStyle) error {
	if element.geometry.kind != geometryPath || style.lineCap == d2scene.CapButt || style.strokeWidth <= 0 ||
		(style.stroke == nil && !style.strokeCurrent) {
		return nil
	}
	var current, start d2scene.Point
	haveCurrent := false
	hasExtent := false
	closed := false
	finish := func() error {
		if haveCurrent && !hasExtent && !closed {
			return i.errorf("element <%s> has a zero-length open stroked subpath with unsupported round or square linecap", element.name)
		}
		return nil
	}
	for index, command := range element.geometry.commands {
		if index&255 == 0 {
			if err := i.ctx.Err(); err != nil {
				return err
			}
		}
		switch command.Kind {
		case d2scene.MoveCommand:
			if err := finish(); err != nil {
				return err
			}
			current, start = command.P1, command.P1
			haveCurrent, hasExtent, closed = true, false, false
		case d2scene.LineCommand:
			hasExtent = hasExtent || command.P1 != current
			current = command.P1
		case d2scene.QuadraticCommand:
			hasExtent = hasExtent || command.P1 != current || command.P2 != current
			current = command.P2
		case d2scene.CubicCommand:
			hasExtent = hasExtent || command.P1 != current || command.P2 != current || command.P3 != current
			current = command.P3
		case d2scene.ArcCommand:
			hasExtent = hasExtent || command.P1 != current
			current = command.P1
		case d2scene.CloseCommand:
			hasExtent = hasExtent || current != start
			current = start
			closed = true
		}
	}
	return finish()
}

func (i *svgImporter) compileElement(element *svgElement) error {
	allowed, exists := elementAttributes[element.name]
	if !exists {
		return i.errorf("internal unsupported element <%s>", element.name)
	}
	if isGradientResourceElement(element.name) {
		return i.compileGradientElement(element)
	}
	for _, name := range element.attrOrder {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		if _, common := commonAttributes[name]; common && element.name != "style" && !isNonRenderingElement(element.name) {
			continue
		}
		if _, specific := allowed[name]; !specific {
			return i.errorf("element <%s> has unsupported attribute %q", element.name, displayXMLName(name))
		}
	}
	if isNonRenderingElement(element.name) {
		element.transform = d2scene.Identity()
		return nil
	}
	if element.name != "svg" && element.name != "g" && element.name != "defs" && element.name != "clipPath" && hasRenderingChildren(element) {
		return i.errorf("element <%s> cannot contain child elements", element.name)
	}
	if element.name == "style" {
		element.transform = d2scene.Identity()
		return nil
	}
	if element.name == "svg" {
		if element.isRoot {
			if err := i.validateRootBoilerplate(element); err != nil {
				return err
			}
		} else if _, ok := element.attrs["version"]; ok {
			return i.errorf("nested <svg> version is unsupported")
		} else if _, ok := element.attrs["enable-background"]; ok {
			return i.errorf("nested <svg> enable-background is unsupported")
		}
	}

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
	if err := i.validateRegularDeclarations(element); err != nil {
		return err
	}
	element.transform = d2scene.Identity()
	if raw, ok := declarations["transform"]; element.name == "svg" && ok && raw != "" && !equalASCIIEqualFold(raw, "none") {
		if element.isRoot {
			return i.errorf("root <svg> transform is unsupported because its outer transform order and origin cannot be represented")
		}
		return i.errorf("nested <svg> transform is unsupported")
	}
	if raw, ok := declarations["transform"]; ok && raw != "" && !equalASCIIEqualFold(raw, "none") {
		remaining := i.limits.MaxTransformFunctions - i.parsedTransforms
		if remaining <= 0 {
			return i.errorf("transform function count exceeds limit %d", i.limits.MaxTransformFunctions)
		}
		var functions int
		element.transform, functions, err = parseTransformWithCount(i.ctx, i.source, raw, transformLimits{
			MaxBytes: i.limits.MaxAttributeBytes, MaxFunctions: remaining,
		})
		if err != nil {
			return err
		}
		i.parsedTransforms += functions
	}
	if element.name == "svg" && !element.isRoot {
		if _, ok := declarations["clip-path"]; ok {
			return i.errorf("nested <svg> clip-path is unsupported because the viewport clip is already active")
		}
		viewport, x, y, err := i.nestedViewport(element)
		if err != nil {
			return err
		}
		element.viewport = &viewport
		element.viewportX = x
		element.viewportY = y
	}

	element.geometry, err = i.compileGeometry(element)
	if err != nil {
		return err
	}
	if element.name == "use" {
		return i.compileUse(element)
	}
	return nil
}

func (i *svgImporter) compileUse(element *svgElement) error {
	raw, exists := element.attrs["href"]
	if !exists {
		return i.errorf("element <use> is missing href")
	}
	if raw == "" || raw[0] != '#' || len(raw) == 1 {
		return i.errorf("element <use> href must be one local #id reference; external references are forbidden")
	}
	valid, err := validSVGID(i.ctx, raw[1:])
	if err != nil {
		return err
	}
	if !valid {
		return i.errorf("element <use> href must be one local #id reference; external references are forbidden")
	}
	target := i.ids[raw[1:]]
	if target == nil {
		return i.errorf("element <use> references an unknown local id")
	}
	if target.name == "svg" || target.name == "defs" || isNonRenderingElement(target.name) || isGradientResourceElement(target.name) || isClipPathResourceElement(target.name) {
		return i.errorf("element <use> references unsupported <%s> resource", target.name)
	}
	element.href = raw[1:]
	element.useX, err = i.lengthAttribute(element, "x", 0, false)
	if err != nil {
		return err
	}
	element.useY, err = i.lengthAttribute(element, "y", 0, false)
	if err != nil {
		return err
	}
	if raw, ok := element.attrs["overflow"]; ok {
		value, err := trimSVGSpace(i.ctx, raw)
		if err != nil {
			return err
		}
		if value != "visible" {
			return i.errorf("element <use> supports only overflow=\"visible\" inside a clipping resource")
		}
	}
	return nil
}

func (i *svgImporter) instantiate(root *svgElement, inherited svgStyle, prefix string, useDepth int, referenced bool) (*d2scene.Node, error) {
	type frame struct {
		element    *svgElement
		inherited  svgStyle
		style      svgStyle
		prefix     string
		useDepth   int
		referenced bool
		phase      uint8
		nextChild  int
		node       *d2scene.Node
		childNode  *d2scene.Node
		useTarget  *svgElement
	}
	const (
		phaseEnter uint8 = iota
		phaseChildren
		phaseUseTarget
	)
	stack := []frame{{element: root, inherited: inherited, prefix: prefix, useDepth: useDepth, referenced: referenced}}
	active := make(map[*svgElement]bool)
	var returned *d2scene.Node
	returning := false

	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return nil, err
		}
		if returning {
			parent := &stack[len(stack)-1]
			switch parent.phase {
			case phaseChildren:
				if returned != nil {
					parent.childNode.Children = append(parent.childNode.Children, returned)
				}
				returned, returning = nil, false
				continue
			case phaseUseTarget:
				delete(active, parent.useTarget)
				if returned != nil {
					node, err := i.newNode(nil)
					if err != nil {
						return nil, err
					}
					node.ID = parent.prefix + parent.element.id
					node.Classes = append([]string(nil), parent.element.classes...)
					node.Transform = parent.element.transform.Mul(d2scene.Translate(parent.element.useX, parent.element.useY))
					node.Opacity = parent.style.opacity
					node.Clip, err = i.instantiateClipPath(parent.style.clipPathID)
					if err != nil {
						return nil, err
					}
					node.Children = []*d2scene.Node{returned}
					returned = node
				}
				stack = stack[:len(stack)-1]
				continue
			default:
				return nil, i.errorf("internal scene instantiation state")
			}
		}

		current := &stack[len(stack)-1]
		switch current.phase {
		case phaseEnter:
			if isNonRenderingElement(current.element.name) || isGradientResourceElement(current.element.name) || isClipPathResourceElement(current.element.name) {
				stack = stack[:len(stack)-1]
				returned, returning = nil, true
				continue
			}
			style, err := i.computeStyle(current.inherited, current.element)
			if err != nil {
				return nil, err
			}
			if err := i.validateDegenerateStrokeCaps(current.element, style); err != nil {
				return nil, err
			}
			current.style = style
			if !style.display || (current.element.name == "defs" && !current.referenced) {
				stack = stack[:len(stack)-1]
				returned, returning = nil, true
				continue
			}
			if current.element.name == "use" {
				if current.useDepth >= i.limits.MaxUseDepth {
					return nil, i.errorf("local <use> expansion depth exceeds limit %d", i.limits.MaxUseDepth)
				}
				target := i.ids[current.element.href]
				if target == nil {
					return nil, i.errorf("element <use> references an unknown local id")
				}
				if active[target] {
					return nil, i.errorf("local <use> reference cycle detected")
				}
				if i.useInstances >= i.limits.MaxResources {
					return nil, i.errorf("expanded local-use resource count exceeds limit %d", i.limits.MaxResources)
				}
				i.useInstances++
				instanceName := current.element.id
				if instanceName == "" {
					instanceName = fmt.Sprintf("@use%d", i.useInstances)
				}
				instancePrefix := current.prefix + instanceName + "/"
				current.phase = phaseUseTarget
				current.useTarget = target
				active[target] = true
				stack = append(stack, frame{
					element: target, inherited: style, prefix: instancePrefix,
					useDepth: current.useDepth + 1, referenced: true,
				})
				continue
			}
			if current.element.name == "svg" && current.referenced {
				return nil, i.errorf("referenced <svg> resources are unsupported")
			}
			primitive, commands, err := current.element.geometry.primitive(i.ctx, style)
			if err != nil {
				return nil, err
			}
			if current.element.name != "svg" && current.element.name != "g" && primitive == nil {
				stack = stack[:len(stack)-1]
				returned, returning = nil, true
				continue
			}
			if primitive != nil {
				if commands > i.limits.MaxPathCommands-i.emittedCommands {
					return nil, i.errorf("emitted path command count exceeds limit %d", i.limits.MaxPathCommands)
				}
				i.emittedCommands += commands
			}
			node, err := i.newNode(primitive)
			if err != nil {
				return nil, err
			}
			node.ID = current.prefix + current.element.id
			node.Classes = append([]string(nil), current.element.classes...)
			node.Opacity = style.opacity
			if current.element.name == "svg" && !current.element.isRoot {
				viewport := current.element.viewport
				if viewport == nil {
					return nil, i.errorf("internal uncompiled nested <svg> viewport")
				}
				const viewportClipCommands = 5
				if viewportClipCommands > i.limits.MaxPathCommands-i.emittedCommands {
					return nil, i.errorf("emitted path command count exceeds limit %d", i.limits.MaxPathCommands)
				}
				content, err := i.newNode(nil)
				if err != nil {
					return nil, err
				}
				i.emittedCommands += viewportClipCommands
				node.Transform = d2scene.Translate(current.element.viewportX, current.element.viewportY)
				node.Clip = nestedViewportClip(viewport.width, viewport.height)
				content.Transform = viewport.transform
				node.Children = []*d2scene.Node{content}
				current.childNode = content
			} else {
				node.Transform = current.element.transform
				node.Clip, err = i.instantiateClipPath(style.clipPathID)
				if err != nil {
					return nil, err
				}
				current.childNode = node
			}
			current.node = node
			current.phase = phaseChildren

		case phaseChildren:
			if current.nextChild == len(current.element.children) {
				returned = current.node
				stack = stack[:len(stack)-1]
				returning = true
				continue
			}
			child := current.element.children[current.nextChild]
			current.nextChild++
			stack = append(stack, frame{
				element: child, inherited: current.style, prefix: current.prefix,
				useDepth: current.useDepth,
			})
		default:
			return nil, i.errorf("internal scene instantiation state")
		}
	}
	return returned, nil
}

func nestedViewportClip(width, height float64) *d2scene.Clip {
	return &d2scene.Clip{
		Path: d2scene.Path{
			FillRule: d2scene.NonZero,
			Commands: []d2scene.PathCommand{
				d2scene.MoveTo(0, 0),
				d2scene.LineTo(width, 0),
				d2scene.LineTo(width, height),
				d2scene.LineTo(0, height),
				d2scene.ClosePath(),
			},
		},
		Transform: d2scene.Identity(),
	}
}

func (i *svgImporter) newNode(primitive d2scene.Primitive) (*d2scene.Node, error) {
	if i.emittedElements >= i.limits.MaxElements {
		return nil, i.errorf("emitted element count exceeds limit %d", i.limits.MaxElements)
	}
	i.emittedElements++
	return d2scene.NewNode(primitive), nil
}

func (geometry svgGeometry) primitive(ctx context.Context, style svgStyle) (d2scene.Primitive, int, error) {
	if !style.visible {
		return nil, 0, nil
	}
	if geometry.kind == geometryImage {
		return d2scene.Image{Asset: geometry.asset, Box: geometry.box, Aspect: geometry.aspect}, 0, nil
	}
	fillPaint := style.fill
	if style.fillCurrent {
		fillPaint = d2scene.SolidPaint{Color: style.color}
	}
	fill, err := paintWithOpacity(ctx, fillPaint, style.fillOpacity)
	if err != nil {
		return nil, 0, err
	}
	if geometry.forceNoFill {
		fill = nil
	}
	var stroke *d2scene.Stroke
	strokeSource := style.stroke
	if style.strokeCurrent {
		strokeSource = d2scene.SolidPaint{Color: style.color}
	}
	strokePaint, err := paintWithOpacity(ctx, strokeSource, style.strokeOpacity)
	if err != nil {
		return nil, 0, err
	}
	if strokePaint != nil && style.strokeWidth > 0 {
		stroke = &d2scene.Stroke{
			Paint: strokePaint, Width: style.strokeWidth, Cap: style.lineCap, Join: style.lineJoin,
			MiterLimit: style.miterLimit, Dashes: append([]float64(nil), style.dashes...), DashOffset: style.dashOffset,
		}
	}
	switch geometry.kind {
	case geometryRect:
		if geometry.box.Width == 0 || geometry.box.Height == 0 || (fill == nil && stroke == nil) {
			return nil, 0, nil
		}
		return d2scene.Rect{Box: geometry.box, RadiusX: geometry.radiusX, RadiusY: geometry.radiusY, Fill: fill, Stroke: stroke}, 0, nil
	case geometryEllipse:
		if geometry.radiusX == 0 || geometry.radiusY == 0 || (fill == nil && stroke == nil) {
			return nil, 0, nil
		}
		return d2scene.Ellipse{Center: geometry.center, RadiusX: geometry.radiusX, RadiusY: geometry.radiusY, Fill: fill, Stroke: stroke}, 0, nil
	case geometryPath:
		if len(geometry.commands) == 0 || (fill == nil && stroke == nil) {
			return nil, 0, nil
		}
		return d2scene.Path{
			Commands: append([]d2scene.PathCommand(nil), geometry.commands...), FillRule: style.fillRule, Fill: fill, Stroke: stroke,
		}, len(geometry.commands), nil
	default:
		return nil, 0, nil
	}
}
