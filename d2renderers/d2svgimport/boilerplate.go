package d2svgimport

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
)

const (
	dublinCoreNamespace     = "http://purl.org/dc/elements/1.1/"
	creativeCommonsNS       = "http://creativecommons.org/ns#"
	rdfNamespace            = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	sodipodiNamespace       = "http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd"
	inkscapeNamespace       = "http://www.inkscape.org/namespaces/inkscape"
	stillImageResource      = "http://purl.org/dc/dcmitype/StillImage"
	metadataFormatMediaType = "image/svg+xml"
)

var inkscapeMetadataAttributes = map[string]map[string]struct{}{
	"svg": {
		"export-filename": {}, "export-xdpi": {}, "export-ydpi": {}, "version": {},
	},
	"g": {
		"groupmode": {}, "label": {},
	},
	"path": {
		"connector-curvature": {}, "export-filename": {}, "export-xdpi": {}, "export-ydpi": {},
	},
	"sodipodi:namedview": {
		"current-layer": {}, "cx": {}, "cy": {}, "document-units": {}, "pageopacity": {},
		"pageshadow": {}, "snap-global": {}, "window-height": {}, "window-maximized": {},
		"window-width": {}, "window-x": {}, "window-y": {}, "zoom": {},
	},
}

var sodipodiMetadataAttributes = map[string]map[string]struct{}{
	"svg":  {"docname": {}},
	"path": {"nodetypes": {}},
}

func isNonRenderingElement(name string) bool {
	switch name {
	case "title", "metadata", "rdf:RDF", "cc:Work", "dc:format", "dc:type", "sodipodi:namedview":
		return true
	default:
		return false
	}
}

func hasRenderingChildren(element *svgElement) bool {
	for _, child := range element.children {
		if !isNonRenderingElement(child.name) {
			return true
		}
	}
	return false
}

// validateNonRenderingTree constrains ignored XML to the exact non-painting
// structures found in the renderer corpus. Recognized namespace names are not
// enough by themselves: foreign elements in any other position remain errors.
func (i *svgImporter) validateNonRenderingTree(root *svgElement) error {
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

		if parent != nil {
			switch parent.name {
			case "metadata":
				if element.name != "rdf:RDF" {
					return i.errorf("element <metadata> has unsupported child <%s>", displayXMLName(element.name))
				}
			case "rdf:RDF":
				if element.name != "cc:Work" {
					return i.errorf("element <rdf:RDF> has unsupported child <%s>", displayXMLName(element.name))
				}
			case "cc:Work":
				if element.name != "dc:format" && element.name != "dc:type" {
					return i.errorf("element <cc:Work> has unsupported child <%s>", displayXMLName(element.name))
				}
			case "title", "dc:format", "dc:type", "sodipodi:namedview":
				return i.errorf("non-rendering element <%s> cannot contain child elements", parent.name)
			}
		}

		switch element.name {
		case "metadata", "sodipodi:namedview":
			if parent != root {
				return i.errorf("non-rendering element <%s> must be a direct child of the root <svg>", element.name)
			}
		case "rdf:RDF":
			if parent == nil || parent.name != "metadata" {
				return i.errorf("element <rdf:RDF> is only supported inside <metadata>")
			}
		case "cc:Work":
			if parent == nil || parent.name != "rdf:RDF" {
				return i.errorf("element <cc:Work> is only supported inside <rdf:RDF>")
			}
		case "dc:format", "dc:type":
			if parent == nil || parent.name != "cc:Work" {
				return i.errorf("element <%s> is only supported inside <cc:Work>", element.name)
			}
		}
		if element.name == "dc:format" {
			value, err := trimSVGSpace(i.ctx, string(element.text))
			if err != nil {
				return err
			}
			if value != metadataFormatMediaType {
				return i.errorf("element <dc:format> has unsupported metadata text")
			}
		}
		for index := len(element.children) - 1; index >= 0; index-- {
			stack = append(stack, entry{element: element.children[index], parent: element})
		}
	}
	cardinalityStack := []*svgElement{root}
	for len(cardinalityStack) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		element := cardinalityStack[len(cardinalityStack)-1]
		cardinalityStack = cardinalityStack[:len(cardinalityStack)-1]
		if err := i.validateMetadataCardinality(element); err != nil {
			return err
		}
		cardinalityStack = append(cardinalityStack, element.children...)
	}
	return nil
}

func (i *svgImporter) validateMetadataCardinality(element *svgElement) error {
	switch element.name {
	case "metadata":
		if len(element.children) != 1 || element.children[0].name != "rdf:RDF" {
			return i.errorf("element <metadata> must contain exactly one <rdf:RDF>")
		}
	case "rdf:RDF":
		if len(element.children) != 1 || element.children[0].name != "cc:Work" {
			return i.errorf("element <rdf:RDF> must contain exactly one <cc:Work>")
		}
	case "cc:Work":
		if len(element.children) != 2 {
			return i.errorf("element <cc:Work> must contain exactly one <dc:format> and one <dc:type>")
		}
		formats, types := 0, 0
		for _, child := range element.children {
			switch child.name {
			case "dc:format":
				formats++
			case "dc:type":
				types++
			}
		}
		if formats != 1 || types != 1 {
			return i.errorf("element <cc:Work> must contain exactly one <dc:format> and one <dc:type>")
		}
	}
	return nil
}

func (i *svgImporter) appendMetadataText(element *svgElement, value []byte) error {
	const checkpointBytes = 4096
	for len(value) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		length := len(value)
		if length > checkpointBytes {
			length = checkpointBytes
		}
		if length > i.limits.MaxBytes-len(element.text) {
			return i.errorf("metadata text exceeds byte limit %d", i.limits.MaxBytes)
		}
		element.text = append(element.text, value[:length]...)
		value = value[length:]
	}
	return i.ctx.Err()
}

func checkpointXMLText(ctx context.Context, value []byte) error {
	for offset := 0; offset < len(value); offset += 4096 {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// ignoredNamespacedAttribute accepts only namespace-qualified editor and RDF
// metadata proven non-painting by the bounded element/topology allowlist.
func (i *svgImporter) ignoredNamespacedAttribute(element string, attribute xml.Attr) (bool, error) {
	switch attribute.Name.Space {
	case xmlNamespace:
		if attribute.Name.Local != "space" {
			return false, i.errorf("element <%s> has unsupported xml attribute %q", element, displayXMLName(attribute.Name.Local))
		}
		value, err := trimSVGSpace(i.ctx, attribute.Value)
		if err != nil {
			return false, err
		}
		if value != "default" && value != "preserve" {
			return false, i.errorf("element <%s> has invalid xml:space", element)
		}
		return true, nil
	case inkscapeNamespace:
		allowed := inkscapeMetadataAttributes[element]
		if _, ok := allowed[attribute.Name.Local]; !ok {
			return false, i.errorf("element <%s> has unsupported Inkscape metadata attribute %q", element, displayXMLName(attribute.Name.Local))
		}
		return true, nil
	case sodipodiNamespace:
		allowed := sodipodiMetadataAttributes[element]
		if _, ok := allowed[attribute.Name.Local]; !ok {
			return false, i.errorf("element <%s> has unsupported Sodipodi metadata attribute %q", element, displayXMLName(attribute.Name.Local))
		}
		return true, nil
	case rdfNamespace:
		switch {
		case element == "cc:Work" && attribute.Name.Local == "about" && attribute.Value == "":
			return true, nil
		case element == "dc:type" && attribute.Name.Local == "resource" && attribute.Value == stillImageResource:
			return true, nil
		default:
			return false, i.errorf("element <%s> has unsupported RDF metadata attribute %q", element, displayXMLName(attribute.Name.Local))
		}
	default:
		return false, nil
	}
}

func (i *svgImporter) validateIgnoredUnnamespacedAttribute(element, name, value string) (bool, error) {
	if strings.HasPrefix(name, "data-") {
		if len(name) == len("data-") {
			return false, i.errorf("element <%s> has invalid data-* metadata attribute", element)
		}
		for index := len("data-"); index < len(name); index++ {
			if index&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return false, err
				}
			}
			character := name[index]
			if !isASCIIAlpha(character) && (character < '0' || character > '9') && character != '-' && character != '_' {
				return false, i.errorf("element <%s> has invalid data-* metadata attribute", element)
			}
		}
		return true, nil
	}
	if name == "role" {
		trimmed, err := trimSVGSpace(i.ctx, value)
		if err != nil {
			return false, err
		}
		if trimmed != "img" && trimmed != "none" && trimmed != "presentation" {
			return false, i.errorf("element <%s> has unsupported non-rendering role", element)
		}
		return true, nil
	}
	if name == "focusable" {
		trimmed, err := trimSVGSpace(i.ctx, value)
		if err != nil {
			return false, err
		}
		if trimmed != "false" {
			return false, i.errorf("element <%s> focusable metadata must be false", element)
		}
		return true, nil
	}
	return false, nil
}

func (i *svgImporter) validateRootBoilerplate(element *svgElement) error {
	if raw, ok := element.attrs["version"]; ok {
		version, err := trimSVGSpace(i.ctx, raw)
		if err != nil {
			return err
		}
		if version != "1.0" && version != "1.1" {
			return i.errorf("root <svg> has unsupported version")
		}
	}
	for _, name := range []string{"x", "y"} {
		if raw, ok := element.attrs[name]; ok {
			value, err := parseSVGLength(raw, true)
			if err != nil || value != 0 {
				return i.errorf("root <svg> %s must be a zero absolute length", name)
			}
		}
	}
	if raw, ok := element.attrs["enable-background"]; ok {
		if err := validateEnableBackground(i.ctx, raw); err != nil {
			if contextErr := i.ctx.Err(); contextErr != nil {
				return contextErr
			}
			return i.errorf("root <svg> has invalid enable-background: %v", err)
		}
	}
	return nil
}

func validateEnableBackground(ctx context.Context, input string) error {
	trimmed, err := trimSVGSpace(ctx, input)
	if err != nil {
		return err
	}
	keywordEnd := 0
	for keywordEnd < len(trimmed) && !isSVGListSpace(trimmed[keywordEnd]) {
		keywordEnd++
		if keywordEnd&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
	}
	if !equalASCIIEqualFold(trimmed[:keywordEnd], "new") || keywordEnd == len(trimmed) {
		return fmt.Errorf("only a finite new x y width height region is supported")
	}
	fields, err := splitEnableBackgroundFields(ctx, trimmed[keywordEnd:])
	if err != nil {
		return err
	}
	if len(fields) != 4 {
		return fmt.Errorf("only a finite new x y width height region is supported")
	}
	values := make([]float64, len(fields))
	for index, field := range fields {
		if err := ctx.Err(); err != nil {
			return err
		}
		values[index], err = parseSVGNumber(field)
		if err != nil {
			return fmt.Errorf("only a finite new x y width height region is supported")
		}
	}
	if values[2] <= 0 || values[3] <= 0 {
		return fmt.Errorf("enable-background width and height must be positive")
	}
	return ctx.Err()
}

func splitEnableBackgroundFields(ctx context.Context, input string) ([]string, error) {
	fields := make([]string, 0, 4)
	start := -1
	for offset := 0; offset <= len(input); offset++ {
		if offset&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		separator := offset == len(input) || isSVGListSpace(input[offset])
		if !separator {
			if input[offset] == ',' {
				return nil, fmt.Errorf("only whitespace-separated enable-background coordinates are supported")
			}
			if start < 0 {
				start = offset
			}
			continue
		}
		if start < 0 {
			continue
		}
		if len(fields) == cap(fields) {
			return nil, fmt.Errorf("only a finite new x y width height region is supported")
		}
		fields = append(fields, input[start:offset])
		start = -1
	}
	return fields, ctx.Err()
}
