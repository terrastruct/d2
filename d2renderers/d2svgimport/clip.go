package d2svgimport

import (
	"fmt"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// compiledClipPath is an immutable resource template. commands is cloned for
// every scene application, while useInstances records the local <use> work
// that must be charged again for every independent expansion.
type compiledClipPath struct {
	path         d2scene.Path
	transform    d2scene.Matrix
	useInstances int
}

func isClipPathResourceElement(name string) bool {
	return name == "clipPath"
}

func supportedClipGeometry(name string) bool {
	switch name {
	case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
		return true
	default:
		return false
	}
}

// validateClipTree constrains clipping resources to the corpus subset before
// any resource is expanded. A clipPath may appear under svg, g, or defs (the
// active Go icon places it next to defs inside a group), but it must contain
// exactly one geometry source. That source may be one direct geometry element
// or one local-use chain ending at geometry. The one-shape restriction is
// deliberate: d2scene.Clip stores one path, and concatenating independently
// styled SVG clip children would turn their union into winding interaction.
func (i *svgImporter) validateClipTree(root *svgElement) error {
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
		element := current.element
		parent := current.parent

		if element.name == "clipPath" {
			if element.id == "" {
				return i.errorf("element <clipPath> must declare an id")
			}
			if parent == nil || (parent.name != "svg" && parent.name != "g" && parent.name != "defs") {
				return i.errorf("element <clipPath> is only supported under <svg>, <g>, or <defs>")
			}
			if raw, ok := element.attrs["clipPathUnits"]; ok {
				units, err := trimSVGSpace(i.ctx, raw)
				if err != nil {
					return err
				}
				if units != "userSpaceOnUse" {
					return i.errorf("element <clipPath> supports only clipPathUnits=\"userSpaceOnUse\"")
				}
			}
			if len(element.children) != 1 {
				return i.errorf("element <clipPath> must contain exactly one supported geometry or local <use>")
			}
			child := element.children[0]
			if child.name != "use" && !supportedClipGeometry(child.name) {
				return i.errorf("element <clipPath> has unsupported child <%s>", displayXMLName(child.name))
			}
		}

		if _, ok := element.attrs["overflow"]; ok {
			if element.name != "use" || parent == nil || parent.name != "clipPath" {
				return i.errorf("overflow metadata is supported only on a direct <use> child of <clipPath>")
			}
		}

		for index := len(element.children) - 1; index >= 0; index-- {
			stack = append(stack, entry{element: element.children[index], parent: element})
		}
	}
	return nil
}

func (i *svgImporter) compileClipPathResources(root *svgElement) error {
	stack := []*svgElement{root}
	for len(stack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		element := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if element.name == "clipPath" {
			compiled, err := i.compileClipPathResource(element)
			if err != nil {
				return err
			}
			element.clipPath = compiled
		}
		for index := len(element.children) - 1; index >= 0; index-- {
			stack = append(stack, element.children[index])
		}
	}
	return nil
}

func (i *svgImporter) compileClipPathResource(element *svgElement) (*compiledClipPath, error) {
	if err := i.validateClipDeclarations(element); err != nil {
		return nil, err
	}
	style, err := i.computeStyle(defaultSVGStyle(), element)
	if err != nil {
		return nil, err
	}
	if style.clipPathID != "" {
		return nil, i.errorf("nested clip-path resources are unsupported")
	}

	current := element.children[0]
	inherited := style
	transform := element.transform
	active := make(map[*svgElement]bool)
	useInstances := 0
	useDepth := 0
	for {
		if err := i.ctx.Err(); err != nil {
			return nil, err
		}
		if err := i.validateClipDeclarations(current); err != nil {
			return nil, err
		}
		style, err = i.computeStyle(inherited, current)
		if err != nil {
			return nil, err
		}
		if style.clipPathID != "" {
			if style.clipPathID == element.id {
				return nil, i.errorf("local clip-path reference cycle detected")
			}
			return nil, i.errorf("nested clip-path resources are unsupported")
		}
		transform = transform.Mul(current.transform)
		if !transform.IsFinite() {
			return nil, i.errorf("clip-path has a non-finite composed transform")
		}

		if current.name != "use" {
			if !supportedClipGeometry(current.name) {
				return nil, i.errorf("clip-path local <use> resolves to unsupported <%s> resource", current.name)
			}
			commands, err := i.clipCommands(current)
			if err != nil {
				return nil, err
			}
			if len(commands) > i.limits.MaxPathCommands {
				return nil, i.errorf("clip-path command count exceeds limit %d", i.limits.MaxPathCommands)
			}
			path := d2scene.Path{Commands: commands, FillRule: style.clipRule}
			if err := validatePrimitiveBounds(i.ctx, path, transform); err != nil {
				if contextErr := i.ctx.Err(); contextErr != nil {
					return nil, contextErr
				}
				return nil, i.errorf("clip-path has invalid or non-finite geometry")
			}
			return &compiledClipPath{path: path, transform: transform, useInstances: useInstances}, nil
		}

		if useDepth >= i.limits.MaxUseDepth {
			return nil, i.errorf("local <use> expansion depth exceeds limit %d", i.limits.MaxUseDepth)
		}
		useDepth++
		useInstances++
		if useInstances > i.limits.MaxResources {
			return nil, i.errorf("expanded local-use resource count exceeds limit %d", i.limits.MaxResources)
		}
		transform = transform.Mul(d2scene.Translate(current.useX, current.useY))
		if !transform.IsFinite() {
			return nil, i.errorf("clip-path has a non-finite composed transform")
		}
		target := i.ids[current.href]
		if target == nil {
			return nil, i.errorf("element <use> references an unknown local id")
		}
		if active[target] {
			return nil, i.errorf("local <use> reference cycle detected")
		}
		active[target] = true
		inherited = style
		current = target
	}
}

// validateClipDeclarations rejects rendering features whose effect inside a
// clipping resource is not represented by d2scene.Clip. Geometry, transforms,
// and the inherited clip-rule are the complete supported subset.
func (i *svgImporter) validateClipDeclarations(element *svgElement) error {
	for property := range element.declarations {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		if property != "transform" && property != "clip-rule" {
			return i.errorf("element <%s> has unsupported clipping-resource property %q", element.name, property)
		}
	}
	return nil
}

func (i *svgImporter) clipCommands(element *svgElement) ([]d2scene.PathCommand, error) {
	geometry := element.geometry
	switch geometry.kind {
	case geometryPath:
		commands := make([]d2scene.PathCommand, len(geometry.commands))
		for start := 0; start < len(commands); start += 256 {
			if err := i.ctx.Err(); err != nil {
				return nil, err
			}
			end := start + 256
			if end > len(commands) {
				end = len(commands)
			}
			copy(commands[start:end], geometry.commands[start:end])
		}
		return commands, i.ctx.Err()
	case geometryRect:
		box := geometry.box
		if box.Width == 0 || box.Height == 0 {
			return nil, nil
		}
		if geometry.radiusX == 0 || geometry.radiusY == 0 {
			return []d2scene.PathCommand{
				d2scene.MoveTo(box.X, box.Y),
				d2scene.LineTo(box.X+box.Width, box.Y),
				d2scene.LineTo(box.X+box.Width, box.Y+box.Height),
				d2scene.LineTo(box.X, box.Y+box.Height),
				d2scene.ClosePath(),
			}, nil
		}
		rx, ry := geometry.radiusX, geometry.radiusY
		return []d2scene.PathCommand{
			d2scene.MoveTo(box.X+rx, box.Y),
			d2scene.LineTo(box.X+box.Width-rx, box.Y),
			d2scene.ArcTo(rx, ry, 0, false, true, box.X+box.Width, box.Y+ry),
			d2scene.LineTo(box.X+box.Width, box.Y+box.Height-ry),
			d2scene.ArcTo(rx, ry, 0, false, true, box.X+box.Width-rx, box.Y+box.Height),
			d2scene.LineTo(box.X+rx, box.Y+box.Height),
			d2scene.ArcTo(rx, ry, 0, false, true, box.X, box.Y+box.Height-ry),
			d2scene.LineTo(box.X, box.Y+ry),
			d2scene.ArcTo(rx, ry, 0, false, true, box.X+rx, box.Y),
			d2scene.ClosePath(),
		}, nil
	case geometryEllipse:
		if geometry.radiusX == 0 || geometry.radiusY == 0 {
			return nil, nil
		}
		center, rx, ry := geometry.center, geometry.radiusX, geometry.radiusY
		return []d2scene.PathCommand{
			d2scene.MoveTo(center.X+rx, center.Y),
			d2scene.ArcTo(rx, ry, 0, false, true, center.X-rx, center.Y),
			d2scene.ArcTo(rx, ry, 0, false, true, center.X+rx, center.Y),
			d2scene.ClosePath(),
		}, nil
	default:
		return nil, fmt.Errorf("d2svgimport: internal unsupported clip geometry <%s>", element.name)
	}
}

func (i *svgImporter) localClipPathID(value string) (string, error) {
	if err := i.ctx.Err(); err != nil {
		return "", err
	}
	if len(value) < len("url(#x)") || value[:len("url(#")] != "url(#" || value[len(value)-1] != ')' {
		return "", fmt.Errorf("requires one local url(#id) reference; external, quoted, and fallback URLs are forbidden")
	}
	id := value[len("url(#") : len(value)-1]
	valid, err := validSVGID(i.ctx, id)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", fmt.Errorf("requires one local url(#id) reference; external, quoted, and fallback URLs are forbidden")
	}
	target := i.ids[id]
	if target == nil {
		return "", fmt.Errorf("references an unknown local id")
	}
	if target.name != "clipPath" {
		return "", fmt.Errorf("local id does not name a <clipPath> resource")
	}
	return id, nil
}

func (i *svgImporter) instantiateClipPath(id string) (*d2scene.Clip, error) {
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, nil
	}
	resource := i.ids[id]
	if resource == nil || resource.name != "clipPath" || resource.clipPath == nil {
		return nil, i.errorf("internal uncompiled clip-path resource")
	}
	template := resource.clipPath
	if template.useInstances > i.limits.MaxResources-i.useInstances {
		return nil, i.errorf("expanded local-use resource count exceeds limit %d", i.limits.MaxResources)
	}
	if len(template.path.Commands) > i.limits.MaxPathCommands-i.emittedCommands {
		return nil, i.errorf("emitted path command count exceeds limit %d", i.limits.MaxPathCommands)
	}
	path := template.path
	path.Commands = make([]d2scene.PathCommand, len(template.path.Commands))
	for start := 0; start < len(path.Commands); start += 256 {
		if err := i.ctx.Err(); err != nil {
			return nil, err
		}
		end := start + 256
		if end > len(path.Commands) {
			end = len(path.Commands)
		}
		copy(path.Commands[start:end], template.path.Commands[start:end])
	}
	if err := i.ctx.Err(); err != nil {
		return nil, err
	}
	i.useInstances += template.useInstances
	i.emittedCommands += len(template.path.Commands)
	return &d2scene.Clip{Path: path, Transform: template.transform}, nil
}
