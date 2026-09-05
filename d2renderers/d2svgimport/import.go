package d2svgimport

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/charmap"

	"github.com/d2lang/d2/d2renderers/d2scene"
	libsvg "github.com/d2lang/d2/lib/svg"
)

const (
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
	xmlnsNamespace = "xmlns"
	xmlNamespace   = "http://www.w3.org/XML/1998/namespace"
	xmlnsURI       = "http://www.w3.org/2000/xmlns/"
)

// Limits are required caller-selected hard limits for one SVG import. Every
// field must be positive. MaxElements and MaxPathCommands independently cap
// both parsed source and emitted scene totals. MaxResources caps declared IDs
// plus distinct embedded raster assets, and independently caps expanded
// local-use instances. MaxBytes also caps the retained encoded and decoded
// raster-asset totals so callers gain a finite image budget without a
// new required limit.
type Limits struct {
	MaxBytes              int
	MaxDepth              int
	MaxElements           int
	MaxAttributes         int
	MaxAttributeBytes     int
	MaxPathCommands       int
	MaxTransformFunctions int
	MaxUseDepth           int
	MaxResources          int
}

// Result is an owned, network-free SVG subtree plus its viewport metadata.
// Root remains in SVG user coordinates. ViewportTransform maps ViewBox into
// the intrinsic Width x Height viewport using Aspect. A caller embedding the
// subtree must clip final painting to [0,Width] x [0,Height]; in particular,
// AspectSlice intentionally maps content outside that viewport.
type Result struct {
	Root              *d2scene.Node
	ViewBox           d2scene.Box
	Width             float64
	Height            float64
	Aspect            d2scene.AspectRatio
	ViewportTransform d2scene.Matrix
	// Assets owns every immutable raster resource referenced by Image
	// primitives below Root. Keys are stable content IDs; callers embedding the
	// subtree must merge these assets into their document without mutation.
	Assets  map[d2scene.AssetID]d2scene.Asset
	Metrics Metrics
}

// ImportOptions selects caller-owned presentation context that is outside the
// imported SVG subtree. A nil CurrentColor preserves SVG's opaque-black
// initial color. ImportNodeWithOptions copies the pointed-to value before
// parsing and never retains the pointer.
type ImportOptions struct {
	CurrentColor *color.NRGBA
}

// Metrics reports bounded source and emitted-scene work for one successful
// import. Callers retaining multiple imported subscenes use these counters to
// enforce a shared document budget in addition to Limits' per-import ceilings.
type Metrics struct {
	SourceBytes          int
	ParsedElements       int
	ParsedAttributes     int
	ParsedAttributeBytes int
	ParsedPathCommands   int
	ParsedTransformFuncs int
	DeclaredResources    int
	ExpandedUseInstances int
	EmittedElements      int
	EmittedPathCommands  int
	EmbeddedRasterAssets int
	EmbeddedRasterBytes  int
	DecodedRasterBytes   int64
}

// ImportNode imports a strict SVG subset. It never reads files, resolves a
// URL, or performs network I/O. On every error it returns a nil result.
// Nested svg viewports are limited to finite absolute placement and dimensions
// with an explicit viewBox. Paint servers other than bounded local user-space
// linear gradients, clipping outside the bounded local user-space subset,
// masking, general painted text, nested SVG images, and external references
// are intentionally rejected rather than silently approximated. Embedded
// images are restricted to canonical base64 data URIs containing one static
// PNG, JPEG, GIF, or WebP resource. The sole painted
// text exception is the exact U+00B5 fallback emitted by D2's frozen MathJax
// renderer; it is converted to a pinned deterministic outline. Root ex lengths
// are accepted only with MathJax's exact inert vertical-align form, where one
// ex is deterministically eight CSS pixels.
// Non-painting title and corpus-bounded editor metadata are ignored after
// strict validation. Stylesheets are limited to a bounded simple
// class-selector subset; see stylesheet.go.
func ImportNode(ctx context.Context, source string, data []byte, limits Limits) (*Result, error) {
	return ImportNodeWithOptions(ctx, source, data, limits, ImportOptions{})
}

// ImportNodeWithOptions imports the same strict subset as ImportNode with an
// explicit initial currentColor. An SVG color declaration still takes normal
// cascade precedence over this caller-supplied value.
func ImportNodeWithOptions(ctx context.Context, source string, data []byte, limits Limits, options ImportOptions) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateImportLimits(limits); err != nil {
		return nil, err
	}
	if len(data) > limits.MaxBytes {
		return nil, fmt.Errorf("d2svgimport: %s is %d bytes, exceeding limit %d", redactImportSource(source), len(data), limits.MaxBytes)
	}
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		data = data[3:]
	}

	rootCurrentColor := color.NRGBA{A: 255}
	if options.CurrentColor != nil {
		rootCurrentColor = *options.CurrentColor
	}
	importer := &svgImporter{
		ctx: ctx, source: redactImportSource(source), limits: limits,
		ids: make(map[string]*svgElement), assets: make(map[d2scene.AssetID]d2scene.Asset), rootCurrentColor: rootCurrentColor,
	}
	root, err := importer.parse(data)
	if err != nil {
		return nil, err
	}
	if err := importer.compile(root); err != nil {
		return nil, err
	}
	viewport, err := importer.viewport(root)
	if err != nil {
		return nil, err
	}
	node, err := importer.instantiate(root, defaultSVGStyleWithColor(rootCurrentColor), "", 0, false)
	if err != nil {
		return nil, err
	}
	if node == nil {
		node = d2scene.NewNode(nil)
		node.ID = root.id
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateImportedBounds(ctx, node, viewport.transform); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, importer.errorf("imported scene has invalid or non-finite visual bounds")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Result{
		Root: node, ViewBox: viewport.viewBox, Width: viewport.width, Height: viewport.height,
		Aspect: viewport.aspect, ViewportTransform: viewport.transform, Assets: importer.assets,
		Metrics: Metrics{
			SourceBytes:          len(data),
			ParsedElements:       importer.parsedElements,
			ParsedAttributes:     importer.parsedAttributes,
			ParsedAttributeBytes: importer.parsedAttrBytes,
			ParsedPathCommands:   importer.parsedPathCommand,
			ParsedTransformFuncs: importer.parsedTransforms,
			DeclaredResources:    len(importer.ids) + len(importer.assets),
			ExpandedUseInstances: importer.useInstances,
			EmittedElements:      importer.emittedElements,
			EmittedPathCommands:  importer.emittedCommands,
			EmbeddedRasterAssets: len(importer.assets),
			EmbeddedRasterBytes:  importer.rasterBytes,
			DecodedRasterBytes:   importer.rasterDecodedBytes,
		},
	}, nil
}

// validateImportedBounds is iterative so a caller-selected depth limit cannot
// turn otherwise bounded input into a Go call-stack failure. Primitive and
// clip bounds under their composed affine transforms form the complete
// finite-value preflight; this importer does not produce masks, filters, or
// animations.
func validateImportedBounds(ctx context.Context, root *d2scene.Node, viewport d2scene.Matrix) error {
	type entry struct {
		node      *d2scene.Node
		transform d2scene.Matrix
	}
	stack := []entry{{node: root, transform: viewport}}
	for len(stack) != 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.node == nil {
			continue
		}
		transform := current.transform.Mul(current.node.Transform)
		if !transform.IsFinite() {
			return fmt.Errorf("non-finite composed transform")
		}
		if err := validatePrimitiveBounds(ctx, current.node.Primitive, transform); err != nil {
			return fmt.Errorf("invalid primitive bounds")
		}
		if current.node.Clip != nil {
			if !current.node.Clip.Transform.IsFinite() || current.node.Clip.Path.FillRule > d2scene.EvenOdd {
				return fmt.Errorf("invalid clip")
			}
			if err := validatePrimitiveBounds(ctx, current.node.Clip.Path, transform.Mul(current.node.Clip.Transform)); err != nil {
				if contextErr := ctx.Err(); contextErr != nil {
					return contextErr
				}
				return fmt.Errorf("invalid clip bounds")
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		for index := len(current.node.Children) - 1; index >= 0; index-- {
			stack = append(stack, entry{node: current.node.Children[index], transform: transform})
		}
	}
	return nil
}

func validatePrimitiveBounds(ctx context.Context, primitive d2scene.Primitive, transform d2scene.Matrix) error {
	var path d2scene.Path
	switch typed := primitive.(type) {
	case d2scene.Path:
		path = typed
	case *d2scene.Path:
		if typed == nil {
			return nil
		}
		path = *typed
	default:
		bounds, err := d2scene.PrimitiveBounds(primitive, transform)
		if err != nil || !bounds.IsFinite() {
			return fmt.Errorf("invalid primitive bounds")
		}
		return nil
	}

	// d2scene's analytic path bounds scan is intentionally independent of a
	// context. Validate large imported paths in bounded chunks, carrying their
	// current point across chunks and replacing close with its explicit segment.
	const commandsPerChunk = 256
	chunk := make([]d2scene.PathCommand, 0, commandsPerChunk+1)
	var current, subpathStart d2scene.Point
	haveCurrent := false
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		candidate := path
		candidate.Commands = chunk
		bounds, err := d2scene.PrimitiveBounds(candidate, transform)
		if err != nil || !bounds.IsFinite() {
			return fmt.Errorf("invalid path bounds")
		}
		chunk = chunk[:0]
		return ctx.Err()
	}
	for index, command := range path.Commands {
		if index%commandsPerChunk == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if len(chunk) == 0 && haveCurrent && command.Kind != d2scene.MoveCommand {
			chunk = append(chunk, d2scene.MoveTo(current.X, current.Y))
		}
		switch command.Kind {
		case d2scene.MoveCommand:
			current, subpathStart, haveCurrent = command.P1, command.P1, true
		case d2scene.LineCommand:
			current = command.P1
		case d2scene.QuadraticCommand:
			current = command.P2
		case d2scene.CubicCommand:
			current = command.P3
		case d2scene.ArcCommand:
			current = command.P1
		case d2scene.CloseCommand:
			command = d2scene.LineTo(subpathStart.X, subpathStart.Y)
			current = subpathStart
		}
		chunk = append(chunk, command)
		if (index+1)%commandsPerChunk == 0 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func validateImportLimits(limits Limits) error {
	if limits.MaxBytes <= 0 || limits.MaxDepth <= 0 || limits.MaxElements <= 0 ||
		limits.MaxAttributes <= 0 || limits.MaxAttributeBytes <= 0 || limits.MaxPathCommands <= 0 ||
		limits.MaxTransformFunctions <= 0 || limits.MaxUseDepth <= 0 || limits.MaxResources <= 0 {
		return fmt.Errorf("d2svgimport: all import limits must be positive")
	}
	return nil
}

type svgElement struct {
	name      string
	id        string
	isRoot    bool
	parent    *svgElement
	attrs     map[string]string
	metadata  map[string]string
	attrOrder []string
	children  []*svgElement
	text      []byte
	classes   []string
	classSet  map[string]struct{}

	declarations map[string]string
	transform    d2scene.Matrix
	geometry     svgGeometry
	href         string
	useX         float64
	useY         float64
	gradient     *d2scene.LinearGradient
	gradientStop *d2scene.GradientStop
	clipPath     *compiledClipPath
	viewport     *svgViewport
	viewportX    float64
	viewportY    float64
}

type svgImporter struct {
	ctx              context.Context
	source           string
	limits           Limits
	ids              map[string]*svgElement
	assets           map[d2scene.AssetID]d2scene.Asset
	stylesheetRules  []stylesheetRule
	elementNamespace string
	namespaceSet     bool

	parsedElements     int
	parsedAttributes   int
	parsedAttrBytes    int
	parsedPathCommand  int
	parsedTransforms   int
	emittedElements    int
	emittedCommands    int
	useInstances       int
	stylesheetBytes    int
	mathJaxRoot        *svgElement
	rootCurrentColor   color.NRGBA
	rasterBytes        int
	rasterDecodedBytes int64
}

type contextReader struct {
	ctx    context.Context
	reader *bytes.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	// Keep work between cancellation checkpoints bounded even when the XML
	// decoder requests a very large read for one attacker-controlled token.
	const checkpointBytes = 4096
	if len(buffer) > checkpointBytes {
		buffer = buffer[:checkpointBytes]
	}
	return r.reader.Read(buffer)
}

func (i *svgImporter) parse(data []byte) (*svgElement, error) {
	decoder := xml.NewDecoder(&contextReader{ctx: i.ctx, reader: bytes.NewReader(data)})
	decoder.Strict = true
	decoder.CharsetReader = iso88591XMLReader
	var root *svgElement
	var stack []*svgElement
	closedRoot := false
	seenXMLDeclaration := false
	seenDoctype := false
	declarationAllowed := true
	for {
		if err := i.ctx.Err(); err != nil {
			return nil, err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if contextErr := i.ctx.Err(); contextErr != nil {
				return nil, contextErr
			}
			return nil, i.errorf("invalid XML near byte %d", decoder.InputOffset())
		}
		switch token := token.(type) {
		case xml.StartElement:
			declarationAllowed = false
			if closedRoot {
				return nil, i.errorf("multiple root elements near byte %d", decoder.InputOffset())
			}
			if len(stack)+1 > i.limits.MaxDepth {
				return nil, i.errorf("XML depth exceeds limit %d", i.limits.MaxDepth)
			}
			i.parsedElements++
			if i.parsedElements > i.limits.MaxElements {
				return nil, i.errorf("element count exceeds limit %d", i.limits.MaxElements)
			}
			name, err := i.elementName(token.Name)
			if err != nil {
				return nil, err
			}
			if root == nil && name != "svg" {
				return nil, i.errorf("root element must be <svg>, got <%s>", name)
			}
			attributes, attributeOrder, metadata, id, err := i.attributes(name, token.Attr)
			if err != nil {
				return nil, err
			}
			element := &svgElement{name: name, id: id, isRoot: root == nil, attrs: attributes, metadata: metadata, attrOrder: attributeOrder}
			if id != "" {
				if _, exists := i.ids[id]; exists {
					return nil, i.errorf("duplicate id")
				}
				if len(i.ids) >= i.limits.MaxResources {
					return nil, i.errorf("ID resource count exceeds limit %d", i.limits.MaxResources)
				}
				i.ids[id] = element
			}
			if len(stack) == 0 {
				root = element
			} else {
				parent := stack[len(stack)-1]
				element.parent = parent
				parent.children = append(parent.children, element)
			}
			stack = append(stack, element)
		case xml.EndElement:
			declarationAllowed = false
			if len(stack) == 0 {
				return nil, i.errorf("unexpected closing element near byte %d", decoder.InputOffset())
			}
			name, err := i.elementName(token.Name)
			if err != nil {
				return nil, err
			}
			current := stack[len(stack)-1]
			if current.name != name {
				return nil, i.errorf("closing </%s> does not match <%s>", name, current.name)
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				closedRoot = true
			}
		case xml.CharData:
			if len(stack) != 0 && stack[len(stack)-1].name == "style" {
				if err := i.appendStylesheetText(stack[len(stack)-1], token); err != nil {
					return nil, err
				}
				if len(token) != 0 {
					declarationAllowed = false
				}
				continue
			}
			if len(stack) != 0 && stack[len(stack)-1].name == "title" {
				if err := checkpointXMLText(i.ctx, token); err != nil {
					return nil, err
				}
				if len(token) != 0 {
					declarationAllowed = false
				}
				continue
			}
			if len(stack) != 0 && stack[len(stack)-1].name == "dc:format" {
				if err := i.appendMetadataText(stack[len(stack)-1], token); err != nil {
					return nil, err
				}
				if len(token) != 0 {
					declarationAllowed = false
				}
				continue
			}
			if len(stack) != 0 && stack[len(stack)-1].name == "text" {
				if err := i.appendPaintedText(stack[len(stack)-1], token); err != nil {
					return nil, err
				}
				if len(token) != 0 {
					declarationAllowed = false
				}
				continue
			}
			nonSpace, err := hasNonXMLSpace(i.ctx, token)
			if err != nil {
				return nil, err
			}
			if nonSpace {
				return nil, i.errorf("text content is unsupported near byte %d", decoder.InputOffset())
			}
			if len(token) != 0 {
				declarationAllowed = false
			}
		case xml.Comment:
			if len(stack) != 0 && stack[len(stack)-1].name == "style" {
				return nil, i.errorf("CSS comments in <style> are unsupported")
			}
			// Comments carry no rendering semantics and remain bounded by MaxBytes.
			declarationAllowed = false
		case xml.Directive:
			if root != nil || len(stack) != 0 || seenDoctype || !libsvg.IsSupportedExternalDoctype(data, token) {
				return nil, i.errorf("unsupported XML directive or DTD")
			}
			seenDoctype = true
			declarationAllowed = false
		case xml.ProcInst:
			// encoding/xml exposes the optional declaration as a processing
			// instruction. Permit only that declaration, before the root.
			if token.Target != "xml" || seenXMLDeclaration || !declarationAllowed || root != nil || len(stack) != 0 {
				return nil, i.errorf("XML processing instructions are forbidden")
			}
			if err := i.validateXMLDeclaration(token.Inst); err != nil {
				return nil, err
			}
			seenXMLDeclaration = true
			declarationAllowed = false
		default:
			return nil, i.errorf("unsupported XML token near byte %d", decoder.InputOffset())
		}
	}
	if root == nil {
		return nil, i.errorf("document has no root <svg> element")
	}
	if len(stack) != 0 {
		return nil, i.errorf("unterminated <%s> element", stack[len(stack)-1].name)
	}
	if !closedRoot {
		return nil, i.errorf("unterminated root element")
	}
	return root, nil
}

func (i *svgImporter) appendStylesheetText(element *svgElement, value []byte) error {
	const checkpointBytes = 4096
	for len(value) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		length := len(value)
		if length > checkpointBytes {
			length = checkpointBytes
		}
		if length > i.limits.MaxBytes-i.stylesheetBytes {
			return i.errorf("stylesheet text exceeds byte limit %d", i.limits.MaxBytes)
		}
		element.text = append(element.text, value[:length]...)
		i.stylesheetBytes += length
		value = value[length:]
	}
	return i.ctx.Err()
}

func (i *svgImporter) appendPaintedText(element *svgElement, value []byte) error {
	const checkpointBytes = 4096
	for len(value) != 0 {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		length := len(value)
		if length > checkpointBytes {
			length = checkpointBytes
		}
		// The XML source itself has already been bounded by MaxBytes. This
		// subtraction additionally prevents one element from retaining more
		// decoded character data than that same caller-selected ceiling.
		if length > i.limits.MaxBytes-len(element.text) {
			return i.errorf("painted text exceeds byte limit %d", i.limits.MaxBytes)
		}
		element.text = append(element.text, value[:length]...)
		value = value[length:]
	}
	return i.ctx.Err()
}

func (i *svgImporter) validateXMLDeclaration(input []byte) error {
	offset := 0
	skipSpace := func() error {
		for offset < len(input) && isXMLSpace(input[offset]) {
			offset++
			if offset&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := skipSpace(); err != nil {
		return err
	}
	type declarationField struct {
		name  string
		value string
	}
	var fields []declarationField
	for offset < len(input) {
		if err := i.ctx.Err(); err != nil {
			return err
		}
		nameStart := offset
		for offset < len(input) && isASCIIAlpha(input[offset]) {
			offset++
			if offset&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return err
				}
			}
		}
		if nameStart == offset {
			return i.errorf("invalid XML declaration")
		}
		name := string(input[nameStart:offset])
		if err := skipSpace(); err != nil {
			return err
		}
		if offset >= len(input) || input[offset] != '=' {
			return i.errorf("invalid XML declaration")
		}
		offset++
		if err := skipSpace(); err != nil {
			return err
		}
		if offset >= len(input) || input[offset] != '\'' && input[offset] != '"' {
			return i.errorf("invalid XML declaration")
		}
		quote := input[offset]
		offset++
		valueStart := offset
		for offset < len(input) && input[offset] != quote {
			offset++
			if offset&4095 == 0 {
				if err := i.ctx.Err(); err != nil {
					return err
				}
			}
		}
		if offset >= len(input) {
			return i.errorf("invalid XML declaration")
		}
		value := string(input[valueStart:offset])
		offset++
		fields = append(fields, declarationField{name: name, value: value})
		if offset < len(input) && !isXMLSpace(input[offset]) {
			return i.errorf("invalid XML declaration")
		}
		if err := skipSpace(); err != nil {
			return err
		}
	}
	if len(fields) == 0 || len(fields) > 3 || fields[0].name != "version" || fields[0].value != "1.0" {
		return i.errorf("invalid XML declaration")
	}
	next := 1
	if next < len(fields) && fields[next].name == "encoding" {
		if !equalASCIIEqualFold(fields[next].value, "UTF-8") && !equalASCIIEqualFold(fields[next].value, "ISO-8859-1") {
			return i.errorf("only UTF-8 and ISO-8859-1 XML declarations are supported")
		}
		next++
	}
	if next < len(fields) && fields[next].name == "standalone" {
		if fields[next].value != "yes" && fields[next].value != "no" {
			return i.errorf("invalid XML declaration")
		}
		next++
	}
	if next != len(fields) {
		return i.errorf("invalid XML declaration")
	}
	return nil
}

func iso88591XMLReader(charset string, input io.Reader) (io.Reader, error) {
	if !equalASCIIEqualFold(charset, "ISO-8859-1") {
		return nil, errors.New("unsupported XML character encoding")
	}
	return charmap.ISO8859_1.NewDecoder().Reader(input), nil
}

func isXMLSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func hasNonXMLSpace(ctx context.Context, value []byte) (bool, error) {
	for index, character := range value {
		if !isXMLSpace(character) {
			return true, nil
		}
		if index&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
	}
	return false, nil
}

func (i *svgImporter) elementName(name xml.Name) (string, error) {
	switch name.Space {
	case "", svgNamespace:
		if !i.namespaceSet {
			i.elementNamespace = name.Space
			i.namespaceSet = true
		} else if name.Space != i.elementNamespace {
			return "", i.errorf("element <%s> mixes SVG and empty namespaces", displayXMLName(name.Local))
		}
		switch name.Local {
		case "svg", "g", "defs", "style", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "image", "text", "use", "clipPath", "linearGradient", "stop", "title", "metadata":
			return name.Local, nil
		case "script", "foreignObject":
			return "", i.errorf("forbidden element <%s>", name.Local)
		case "radialGradient", "pattern":
			return "", i.errorf("unsupported paint server <%s>", name.Local)
		default:
			return "", i.errorf("unknown element <%s>", displayXMLName(name.Local))
		}
	case rdfNamespace:
		if name.Local == "RDF" {
			return "rdf:RDF", nil
		}
	case creativeCommonsNS:
		if name.Local == "Work" {
			return "cc:Work", nil
		}
	case dublinCoreNamespace:
		if name.Local == "format" || name.Local == "type" {
			return "dc:" + name.Local, nil
		}
	case sodipodiNamespace:
		if name.Local == "namedview" {
			return "sodipodi:namedview", nil
		}
	}
	return "", i.errorf("element <%s> uses unsupported namespace or metadata name", displayXMLName(name.Local))
}

func (i *svgImporter) attributes(element string, input []xml.Attr) (map[string]string, []string, map[string]string, string, error) {
	attributes := make(map[string]string, len(input))
	attributeOrder := make([]string, 0, len(input))
	metadata := make(map[string]string)
	expandedNames := make(map[xml.Name]struct{}, len(input))
	var id string
	for _, attribute := range input {
		if err := i.ctx.Err(); err != nil {
			return nil, nil, nil, "", err
		}
		i.parsedAttributes++
		if i.parsedAttributes > i.limits.MaxAttributes {
			return nil, nil, nil, "", i.errorf("attribute count exceeds limit %d", i.limits.MaxAttributes)
		}
		attributeBytes := len(attribute.Name.Space) + len(attribute.Name.Local) + len(attribute.Value)
		if attributeBytes > i.limits.MaxAttributeBytes-i.parsedAttrBytes {
			return nil, nil, nil, "", i.errorf("attribute bytes exceed limit %d", i.limits.MaxAttributeBytes)
		}
		i.parsedAttrBytes += attributeBytes
		if _, duplicate := expandedNames[attribute.Name]; duplicate {
			return nil, nil, nil, "", i.errorf("element <%s> has duplicate XML attribute %q", element, displayXMLName(attribute.Name.Local))
		}
		expandedNames[attribute.Name] = struct{}{}

		if attribute.Name.Space == xmlnsNamespace || (attribute.Name.Space == "" && attribute.Name.Local == "xmlns") {
			if err := i.validateNamespaceDeclaration(element, attribute); err != nil {
				return nil, nil, nil, "", err
			}
			continue
		}
		ignored, err := i.ignoredNamespacedAttribute(element, attribute)
		if err != nil {
			return nil, nil, nil, "", err
		}
		if ignored {
			if attribute.Name.Space == "" {
				metadata[attribute.Name.Local] = attribute.Value
			}
			continue
		}
		name := attribute.Name.Local
		switch attribute.Name.Space {
		case "":
		case xlinkNamespace:
			if name != "href" {
				return nil, nil, nil, "", i.errorf("element <%s> has unsupported xlink attribute %q", element, displayXMLName(name))
			}
		default:
			return nil, nil, nil, "", i.errorf("element <%s> attribute %q uses unsupported namespace", element, displayXMLName(name))
		}
		if hasASCIIEqualFoldPrefix(name, "on") {
			return nil, nil, nil, "", i.errorf("element <%s> has forbidden event attribute %q", element, displayXMLName(name))
		}
		ignored, err = i.validateIgnoredUnnamespacedAttribute(element, name, attribute.Value)
		if err != nil {
			return nil, nil, nil, "", err
		}
		if ignored {
			metadata[name] = attribute.Value
			continue
		}
		if _, duplicate := attributes[name]; duplicate {
			return nil, nil, nil, "", i.errorf("element <%s> has duplicate attribute %q", element, displayXMLName(name))
		}
		attributes[name] = attribute.Value
		attributeOrder = append(attributeOrder, name)
		if name == "id" {
			id = attribute.Value
			valid, err := validSVGID(i.ctx, id)
			if err != nil {
				return nil, nil, nil, "", err
			}
			if !valid {
				return nil, nil, nil, "", i.errorf("element <%s> has invalid id", element)
			}
		}
	}
	return attributes, attributeOrder, metadata, id, nil
}

func displayXMLName(name string) string {
	if len(name) > 64 {
		return "<oversized-name>"
	}
	return name
}

func (i *svgImporter) validateNamespaceDeclaration(element string, attribute xml.Attr) error {
	prefix := ""
	if attribute.Name.Space == xmlnsNamespace {
		prefix = attribute.Name.Local
	}
	if prefix != "" && attribute.Value == "" {
		return i.errorf("element <%s> has an empty prefixed namespace binding", element)
	}
	if equalASCIIEqualFold(prefix, "xmlns") || attribute.Value == xmlnsURI {
		return i.errorf("element <%s> has a forbidden xmlns namespace binding", element)
	}
	if prefix == "xml" {
		if attribute.Value != xmlNamespace {
			return i.errorf("element <%s> has an invalid xml namespace binding", element)
		}
		return nil
	}
	if hasASCIIEqualFoldPrefix(prefix, "xml") || attribute.Value == xmlNamespace {
		return i.errorf("element <%s> has a reserved xml namespace binding", element)
	}
	return nil
}

func validSVGID(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	for offset, value := range id {
		if offset&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		if value == '#' || unicode.IsSpace(value) || unicode.IsControl(value) {
			return false, nil
		}
	}
	return true, ctx.Err()
}

func (i *svgImporter) errorf(format string, arguments ...any) error {
	return fmt.Errorf("d2svgimport: %s: %s", i.source, fmt.Sprintf(format, arguments...))
}

func redactImportSource(source string) string {
	if source == "" {
		return "SVG asset"
	}
	if len(source) > 4096 {
		return "<oversized source locator>"
	}
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "SVG asset"
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		return "data URI"
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && (parsed.Scheme != "" || parsed.Host != "") {
		// Opaque URIs can place arbitrary credentials or bearer material in
		// Opaque, where URL user/query redaction does not reach it.
		if parsed.Opaque != "" {
			return parsed.Scheme + ":<redacted>"
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		return parsed.String()
	}
	if strings.HasPrefix(trimmed, "//") {
		return "<malformed network-path reference>"
	}
	if err != nil && hasURIScheme(trimmed) {
		return "<malformed URI>"
	}
	if index := strings.IndexAny(trimmed, "?#"); index >= 0 {
		trimmed = trimmed[:index]
	}
	return trimmed
}

func equalASCIIEqualFold(value, pattern string) bool {
	return len(value) == len(pattern) && hasASCIIEqualFoldPrefix(value, pattern)
}

func hasASCIIEqualFoldPrefix(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	for index := range prefix {
		left := value[index]
		right := prefix[index]
		if left >= 'A' && left <= 'Z' {
			left += 'a' - 'A'
		}
		if right >= 'A' && right <= 'Z' {
			right += 'a' - 'A'
		}
		if left != right {
			return false
		}
	}
	return true
}

func hasURIScheme(value string) bool {
	if value == "" || !isASCIIAlpha(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character == ':' {
			return true
		}
		if !isASCIIAlpha(character) && (character < '0' || character > '9') && character != '+' && character != '-' && character != '.' {
			return false
		}
	}
	return false
}

func isASCIIAlpha(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}
