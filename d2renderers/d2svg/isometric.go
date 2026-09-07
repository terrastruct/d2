package d2svg

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric/d2isometricimg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/svg"
	"github.com/d2lang/d2/lib/version"
)

func renderIsometric(diagram *d2target.Diagram, opts *RenderOpts) ([]byte, error) {
	return renderIsometricValidated(context.Background(), diagram, opts, nil)
}

// RenderIsometric produces the native vector geometry with the same document
// options as Render. Native options can supply an asset resolver, fallback fonts,
// and a maximum canvas; their dimensions are unscaled. Rendering preserves
// the compiled layout, including nested containers. Like other native exports,
// this function does not load external assets unless a resolver is supplied.
//
// Responsive dark themes, SVG appendices, and multi-board SVG animation are not
// yet implemented by this renderer. Static multi-board SVG files are supported
// through RenderMultiboard.
func RenderIsometric(ctx context.Context, diagram *d2target.Diagram, opts *RenderOpts, native *d2isometricimg.Options) ([]byte, error) {
	if diagram == nil {
		return nil, fmt.Errorf("isometric rendering requires a diagram")
	}
	if err := validateRenderLinks(diagram); err != nil {
		return nil, err
	}
	if err := d2target.ValidateRenderTarget(diagram); err != nil {
		return nil, err
	}
	if err := validateRenderPaints(diagram, opts); err != nil {
		return nil, err
	}
	return renderIsometricValidated(ctx, diagram, opts, native)
}

func renderIsometricValidated(ctx context.Context, diagram *d2target.Diagram, opts *RenderOpts, native *d2isometricimg.Options) ([]byte, error) {
	if opts == nil {
		opts = &RenderOpts{}
	}
	if opts.Sketch != nil && *opts.Sketch {
		return nil, fmt.Errorf("sketch cannot be combined with isometric rendering")
	}
	if opts.DarkThemeID != nil || opts.DarkThemeOverrides != nil {
		return nil, fmt.Errorf("isometric rendering does not yet support responsive dark themes; select a single theme instead")
	}
	if opts.MasterID != "" {
		return nil, fmt.Errorf("isometric rendering does not yet support multi-board SVG animation")
	}
	if opts.Scale != nil && (math.IsNaN(*opts.Scale) || math.IsInf(*opts.Scale, 0) || *opts.Scale <= 0) {
		return nil, fmt.Errorf("isometric SVG scale must be finite and positive")
	}
	o := d2isometricimg.Options{FitContent: true}
	if native != nil {
		o = *native
	}
	o.Format = d2isometricimg.SVG
	o.Render.ThemeID, o.Render.ThemeOverrides = opts.ThemeID, opts.ThemeOverrides
	data, err := d2isometricimg.Render(ctx, diagram, &o)
	if err != nil {
		return nil, err
	}
	return wrapIsometricSVG(diagram, data, opts)
}

var isometricSVGRoot = regexp.MustCompile(`^<svg\b[^>]*>`)
var isometricSVGID = regexp.MustCompile(`\bid="([^"]+)"`)

// The native writer emits controlled SVG markup. Rewrite resource definitions
// and references together so multiple salted documents can share an HTML page.
// Actual link destinations are left alone unless they name a defined resource.
func namespaceIsometricSVG(data []byte, prefix string) []byte {
	ids := isometricSVGID.FindAllSubmatch(data, -1)
	if len(ids) == 0 {
		return data
	}
	replacements := make([]string, 0, len(ids)*6)
	seen := make(map[string]bool, len(ids))
	for _, match := range ids {
		id := string(match[1])
		if seen[id] {
			continue
		}
		seen[id] = true
		replacements = append(replacements,
			`id="`+id+`"`, `id="`+prefix+id+`"`,
			`url(#`+id+`)`, `url(#`+prefix+id+`)`,
			`href="#`+id+`"`, `href="#`+prefix+id+`"`)
	}
	return []byte(strings.NewReplacer(replacements...).Replace(string(data)))
}

func wrapIsometricSVG(diagram *d2target.Diagram, data []byte, opts *RenderOpts) ([]byte, error) {
	var root struct {
		Width  int64 `xml:"width,attr"`
		Height int64 `xml:"height,attr"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("read isometric SVG dimensions: %w", err)
	}
	start, ok := token.(xml.StartElement)
	if !ok || start.Name.Local != "svg" {
		return nil, fmt.Errorf("native isometric output is not an SVG")
	}
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "width":
			root.Width, err = strconv.ParseInt(attr.Value, 10, 64)
		case "height":
			root.Height, err = strconv.ParseInt(attr.Value, 10, 64)
		}
		if err != nil {
			return nil, fmt.Errorf("read isometric SVG dimensions: %w", err)
		}
	}
	pad := int64(DEFAULT_PADDING)
	if opts.Pad != nil {
		pad = *opts.Pad
	}
	// Bound arithmetic before calculating two-sided padding, just as the flat
	// SVG renderer does. Negative padding deliberately crops the viewport.
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	pad2, ok := checkedIntAdd(pad, pad, minInt, maxInt)
	if !ok {
		return nil, invalidPaddingError(pad)
	}
	width, ok := checkedIntAdd(root.Width, pad2, minInt, maxInt)
	if !ok || width <= 0 {
		return nil, invalidPaddingError(pad)
	}
	height, ok := checkedIntAdd(root.Height, pad2, minInt, maxInt)
	if !ok || height <= 0 {
		return nil, invalidPaddingError(pad)
	}
	var dimensions []svg.Attribute
	if opts.Scale != nil {
		w, h := math.Ceil(float64(width)**opts.Scale), math.Ceil(float64(height)**opts.Scale)
		if math.IsInf(w, 0) || math.IsInf(h, 0) || w > float64(maxInt) || h > float64(maxInt) {
			return nil, fmt.Errorf("isometric SVG scale produces invalid dimensions")
		}
		dimensions = []svg.Attribute{svg.Attr("width", strconv.FormatFloat(w, 'f', 0, 64)), svg.Attr("height", strconv.FormatFloat(h, 'f', 0, 64))}
	}
	hash, err := diagram.HashID(opts.Salt)
	if err != nil {
		return nil, err
	}
	data = namespaceIsometricSVG(data, hash+"-isometric-")
	opening := isometricSVGRoot.FindIndex(data)
	if opening == nil || !bytes.HasSuffix(data, []byte("</svg>")) {
		return nil, fmt.Errorf("native isometric output has an invalid SVG envelope")
	}
	body := data[opening[1] : len(data)-len("</svg>")]
	background := "<rect" + svg.RenderAttributes(svg.IntAttr("x", int(-pad)), svg.IntAttr("y", int(-pad)), svg.IntAttr("width", int(width)), svg.IntAttr("height", int(height)))
	body = bytes.Replace(body, []byte(`<rect width="100%" height="100%"`), []byte(background), 1)
	var output bytes.Buffer
	if opts.NoXMLTag == nil || !*opts.NoXMLTag {
		output.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	}
	alignment := "xMinYMin"
	if opts.Center != nil && *opts.Center {
		alignment = "xMidYMid"
	}
	attributes := []svg.Attribute{svg.Attr("xmlns", "http://www.w3.org/2000/svg"), svg.Attr("xmlns:xlink", "http://www.w3.org/1999/xlink")}
	if opts.OmitVersion == nil || !*opts.OmitVersion {
		attributes = append(attributes, svg.Attr("data-d2-version", version.Version))
	}
	attributes = append(attributes, svg.Attr("preserveAspectRatio", alignment+" meet"), svg.Attr("viewBox", fmt.Sprintf("0 0 %d %d", width, height)))
	attributes = append(attributes, dimensions...)
	output.WriteString(svg.OpenElement("svg", attributes...))
	output.WriteString(svg.OpenElement("svg", svg.Attr("class", hash+" d2-svg d2-isometric"), svg.IntAttr("width", int(width)), svg.IntAttr("height", int(height)), svg.Attr("viewBox", fmt.Sprintf("%d %d %d %d", -pad, -pad, width, height)), svg.Attr("role", "img")))
	output.Write(body)
	output.WriteString(`</svg></svg>`)
	if output.Len() > d2isometricimg.MaxOutputBytes {
		return nil, fmt.Errorf("isometric SVG document exceeds output byte limit")
	}
	return output.Bytes(), nil
}
