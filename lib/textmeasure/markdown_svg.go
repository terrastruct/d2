package textmeasure

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/rivo/uniseg"
	goldmarkHTML "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/lib/svg"
)

// MarkdownPrimitiveKind identifies the kind of native SVG primitive in a
// MarkdownLayout. The layout package intentionally does not choose concrete
// colors so that renderers can map the semantic roles to their active theme.
type MarkdownPrimitiveKind string

const (
	MarkdownTextPrimitive MarkdownPrimitiveKind = "text"
	MarkdownRectPrimitive MarkdownPrimitiveKind = "rect"
	MarkdownLinePrimitive MarkdownPrimitiveKind = "line"
)

// MarkdownColorRole is a semantic Markdown color. It mirrors the roles used by
// github-markdown.css without coupling text measurement to a D2 theme.
type MarkdownColorRole string

const (
	MarkdownColorNone         MarkdownColorRole = ""
	MarkdownColorForeground   MarkdownColorRole = "foreground"
	MarkdownColorMuted        MarkdownColorRole = "muted"
	MarkdownColorAccent       MarkdownColorRole = "accent"
	MarkdownColorBorder       MarkdownColorRole = "border"
	MarkdownColorBorderMuted  MarkdownColorRole = "border-muted"
	MarkdownColorCanvas       MarkdownColorRole = "canvas"
	MarkdownColorCanvasSubtle MarkdownColorRole = "canvas-subtle"
	MarkdownColorNeutralMuted MarkdownColorRole = "neutral-muted"
)

// MarkdownFontRole lets an SVG renderer select the correct embedded D2 font.
// Font sizes are kept separately on each primitive.
type MarkdownFontRole string

const (
	MarkdownFontRegular      MarkdownFontRole = "regular"
	MarkdownFontSemibold     MarkdownFontRole = "semibold"
	MarkdownFontBold         MarkdownFontRole = "bold"
	MarkdownFontItalic       MarkdownFontRole = "italic"
	MarkdownFontMono         MarkdownFontRole = "mono"
	MarkdownFontMonoSemibold MarkdownFontRole = "mono-semibold"
	MarkdownFontMonoBold     MarkdownFontRole = "mono-bold"
	MarkdownFontMonoItalic   MarkdownFontRole = "mono-italic"
)

type MarkdownTextDecoration string

const (
	MarkdownTextDecorationNone        MarkdownTextDecoration = ""
	MarkdownTextDecorationLineThrough MarkdownTextDecoration = "line-through"
)

// MarkdownPrimitive is a serializer-neutral native SVG drawing operation.
//
// Text uses X/Y as the baseline origin and Width/Height as its measured box.
// Rect uses X/Y/Width/Height/Radius. Line uses X/Y and X2/Y2. FillRole and
// StrokeRole are semantic so light/dark themes can serialize the same layout.
type MarkdownPrimitive struct {
	Kind MarkdownPrimitiveKind

	X, Y, X2, Y2        float64
	Width, Height       float64
	Radius, StrokeWidth float64

	Text       string
	Font       MarkdownFontRole
	FontSize   float64
	FillRole   MarkdownColorRole
	StrokeRole MarkdownColorRole
	Link       string
	LinkTitle  string
	Decoration MarkdownTextDecoration
	// SyntheticItalic is used when CSS asks for both italic and a weighted
	// face. D2 embeds bold and italic faces, but not a separate bold-italic
	// face, so the SVG renderer slants the measured weighted face.
	SyntheticItalic bool
}

// MarkdownLayout contains the exact dimensions used by D2 layout and the
// positioned primitives used to paint those dimensions. MeasureMarkdown calls
// LayoutMarkdown, ensuring measurement and rendering share one code path.
type MarkdownLayout struct {
	Width, Height int
	Primitives    []MarkdownPrimitive
	// Corpus includes generated text (notably list marker glyphs) as well as
	// source text. Renderers embedding subset fonts should include it.
	Corpus string
}

// MarkdownSVGPaint controls how a semantic role is emitted by SVG. Class is
// optional; Color is an SVG paint value such as "currentColor", "#fff", or a
// CSS variable.
type MarkdownSVGPaint struct {
	Class string
	Color string
}

// MarkdownSVGOptions controls native SVG serialization without affecting
// layout. A renderer can override role paints and font classes to match its
// theme/font embedding scheme.
type MarkdownSVGOptions struct {
	Class       string
	RolePaint   map[MarkdownColorRole]MarkdownSVGPaint
	FontClasses map[MarkdownFontRole]string
	// DisableLinks emits linked text without inner <a> elements. d2svg uses
	// this when a Markdown label is already wrapped by a shape/connection link.
	DisableLinks bool
	// Underline applies D2's style.underline to every Markdown text primitive.
	Underline bool
}

const (
	// SVG viewers are allowed to ignore embedded web fonts. Historical D2
	// cylinder fixtures need up to 4.632px of horizontal reserve when that
	// happens. Sketch fonts can extend 2.343px above their nominal font box.
	// Keep additional whole-pixel headroom for renderer and platform rounding.
	markdownInkSafetyX = 6.0
	markdownInkSafetyY = 4.0
	// Unsupported glyphs are painted by the SVG viewer's host fallback font.
	// Those glyphs can have substantially different bearings than D2's fonts.
	// These conservative em paddings cover the system fonts exercised by the
	// Unicode regression corpus while keeping ordinary Markdown unchanged apart
	// from the generic ink safety above.
	markdownFallbackPaddingX = 1.0
	markdownFallbackPaddingY = 0.625
)

// LayoutMarkdown parses Markdown once, measures it with D2's existing box
// model, and paints that same box model into native SVG primitives.
func LayoutMarkdown(mdText string, ruler *Ruler, fontFamily *d2fonts.FontFamily, monoFontFamily *d2fonts.FontFamily, fontSize int) (*MarkdownLayout, error) {
	render, err := RenderMarkdown(mdText)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(render))
	if err != nil {
		return nil, err
	}
	body := doc.Find("body").First()
	if len(body.Nodes) == 0 {
		return &MarkdownLayout{}, nil
	}
	if err := validateMarkdownSVGNodes(body.Nodes[0]); err != nil {
		return nil, err
	}
	normalizeMarkdownWhitespaceTree(body.Nodes[0])

	originalLineHeight := ruler.LineHeightFactor
	originalBoundsWithDot := ruler.boundsWithDot
	ruler.LineHeightFactor = MarkdownLineHeight
	ruler.boundsWithDot = true
	defer func() {
		ruler.LineHeightFactor = originalLineHeight
		ruler.boundsWithDot = originalBoundsWithDot
	}()

	if fontFamily == nil {
		family := d2fonts.SourceSansPro
		fontFamily = &family
	}
	if monoFontFamily == nil {
		family := d2fonts.SourceCodePro
		monoFontFamily = &family
	}

	ctx := markdownPaintContext{
		fontFamily:       fontFamily,
		monoFontFamily:   monoFontFamily,
		fontSize:         fontSize,
		fontWeight:       d2fonts.FONT_STYLE_REGULAR,
		lineHeightFactor: MarkdownLineHeight,
		mono:             *fontFamily == *monoFontFamily,
		role:             MarkdownColorForeground,
	}
	p := markdownPainter{ruler: ruler, layout: &MarkdownLayout{}}
	rootAttrs := p.measure(body.Nodes[0], ctx)
	contentWidth := int(math.Ceil(rootAttrs.width))
	contentHeight := int(math.Ceil(rootAttrs.height))
	p.paintBlock(body.Nodes[0], 0, 0, ctx, float64(contentWidth))
	if len(p.layout.Primitives) > 0 {
		paddingX := markdownInkSafetyX
		paddingY := markdownInkSafetyY
		if fallbackSize := p.maxFallbackFontSize(fontFamily, monoFontFamily); fallbackSize > 0 {
			paddingX += markdownFallbackPaddingX * fallbackSize
			paddingY += markdownFallbackPaddingY * fallbackSize
		}
		p.translate(paddingX, paddingY)
		p.layout.Width = contentWidth + int(math.Ceil(2*paddingX))
		p.layout.Height = contentHeight + int(math.Ceil(2*paddingY))
	}
	var corpus strings.Builder
	for _, primitive := range p.layout.Primitives {
		if primitive.Kind == MarkdownTextPrimitive {
			corpus.WriteString(primitive.Text)
		}
	}
	p.layout.Corpus = corpus.String()
	return p.layout, nil
}

func (p *markdownPainter) translate(dx, dy float64) {
	for i := range p.layout.Primitives {
		primitive := &p.layout.Primitives[i]
		primitive.X += dx
		primitive.Y += dy
		if primitive.Kind == MarkdownLinePrimitive {
			primitive.X2 += dx
			primitive.Y2 += dy
		}
	}
}

func (p *markdownPainter) maxFallbackFontSize(fontFamily, monoFontFamily *d2fonts.FontFamily) float64 {
	maxSize := 0.0
	for _, primitive := range p.layout.Primitives {
		if primitive.Kind != MarkdownTextPrimitive || primitive.Text == "" {
			continue
		}
		family := fontFamily
		style := d2fonts.FONT_STYLE_REGULAR
		switch primitive.Font {
		case MarkdownFontSemibold:
			style = d2fonts.FONT_STYLE_SEMIBOLD
		case MarkdownFontBold:
			style = d2fonts.FONT_STYLE_BOLD
		case MarkdownFontItalic:
			style = d2fonts.FONT_STYLE_ITALIC
		case MarkdownFontMono:
			family = monoFontFamily
		case MarkdownFontMonoSemibold:
			family = monoFontFamily
			style = d2fonts.FONT_STYLE_SEMIBOLD
		case MarkdownFontMonoBold:
			family = monoFontFamily
			style = d2fonts.FONT_STYLE_BOLD
		case MarkdownFontMonoItalic:
			family = monoFontFamily
			style = d2fonts.FONT_STYLE_ITALIC
		}
		fontSize := int(math.Ceil(primitive.FontSize))
		if fontSize < 1 {
			fontSize = 1
		}
		font := family.Font(fontSize, style)
		graphemes := uniseg.NewGraphemes(primitive.Text)
		for graphemes.Next() {
			grapheme := graphemes.Str()
			if isZeroWidthGrapheme(grapheme) {
				continue
			}
			_, supported := p.ruler.measureFontWidth(font, grapheme)
			// Multi-rune clusters need host shaping even when each component is
			// present in D2's font (for example combining marks and variation
			// selectors), so give them the same safety margin as missing glyphs.
			if !supported || utf8.RuneCountInString(grapheme) > 1 {
				maxSize = goMax(maxSize, primitive.FontSize)
				break
			}
		}
	}
	return maxSize
}

// SVG serializes a MarkdownLayout as native SVG elements. It deliberately
// emits a fragment rather than an <svg> document so d2svg can translate and
// theme it inside a diagram.
func (l *MarkdownLayout) SVG(opts MarkdownSVGOptions) string {
	if opts.Class == "" {
		opts.Class = "md md-native"
	}
	rolePaint := defaultMarkdownSVGRolePaint()
	for role, paint := range opts.RolePaint {
		rolePaint[role] = paint
	}
	fontClasses := defaultMarkdownSVGFontClasses()
	for role, class := range opts.FontClasses {
		fontClasses[role] = class
	}

	var out strings.Builder
	fmt.Fprintf(&out, `<g class="%s">`, svg.EscapeText(opts.Class))
	for _, primitive := range l.Primitives {
		classes := make([]string, 0, 3)
		if primitive.Kind == MarkdownTextPrimitive {
			classes = append(classes, "md-text")
			if class := fontClasses[primitive.Font]; class != "" {
				classes = append(classes, class)
			}
		}
		if paint := rolePaint[primitive.FillRole]; paint.Class != "" {
			classes = append(classes, paint.Class)
		}
		if paint := rolePaint[primitive.StrokeRole]; paint.Class != "" {
			classes = append(classes, paint.Class)
		}
		classAttr := ""
		if len(classes) > 0 {
			classAttr = fmt.Sprintf(` class="%s"`, svg.EscapeText(strings.Join(classes, " ")))
		}
		fill := rolePaint[primitive.FillRole].Color
		stroke := rolePaint[primitive.StrokeRole].Color

		switch primitive.Kind {
		case MarkdownRectPrimitive:
			fmt.Fprintf(&out, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s"%s`,
				mdFloat(primitive.X), mdFloat(primitive.Y), mdFloat(primitive.Width), mdFloat(primitive.Height), mdFloat(primitive.Radius), classAttr)
			if fill == "" {
				fill = "none"
			}
			fmt.Fprintf(&out, ` fill="%s"`, svg.EscapeText(fill))
			if stroke != "" && primitive.StrokeWidth > 0 {
				fmt.Fprintf(&out, ` stroke="%s" stroke-width="%s"`, svg.EscapeText(stroke), mdFloat(primitive.StrokeWidth))
			}
			out.WriteString(`/>`)
		case MarkdownLinePrimitive:
			if stroke == "" {
				stroke = "currentColor"
			}
			fmt.Fprintf(&out, `<line x1="%s" y1="%s" x2="%s" y2="%s"%s stroke="%s" stroke-width="%s"/>`,
				mdFloat(primitive.X), mdFloat(primitive.Y), mdFloat(primitive.X2), mdFloat(primitive.Y2), classAttr,
				svg.EscapeText(stroke), mdFloat(primitive.StrokeWidth))
		case MarkdownTextPrimitive:
			if fill == "" {
				fill = "currentColor"
			}
			link := safeMarkdownLink(primitive.Link)
			if link != "" && !opts.DisableLinks {
				fmt.Fprintf(&out, `<a href="%s" xlink:href="%[1]s">`, svg.EscapeText(link))
				if primitive.LinkTitle != "" {
					fmt.Fprintf(&out, `<title>%s</title>`, svg.EscapeText(primitive.LinkTitle))
				}
			}
			fmt.Fprintf(&out, `<text x="%s" y="%s"%s fill="%s" font-size="%s" xml:space="preserve"`,
				mdFloat(primitive.X), mdFloat(primitive.Y), classAttr, svg.EscapeText(fill), mdFloat(primitive.FontSize))
			decoration := string(primitive.Decoration)
			if opts.Underline {
				if decoration == "" {
					decoration = "underline"
				} else {
					decoration = "underline " + decoration
				}
			}
			if decoration != "" {
				fmt.Fprintf(&out, ` text-decoration="%s"`, svg.EscapeText(decoration))
			}
			if primitive.SyntheticItalic {
				out.WriteString(` font-style="italic"`)
			}
			fmt.Fprintf(&out, `>%s</text>`, svg.EscapeText(primitive.Text))
			if link != "" && !opts.DisableLinks {
				out.WriteString(`</a>`)
			}
		}
	}
	out.WriteString(`</g>`)
	return out.String()
}

func safeMarkdownLink(link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	// Browsers ignore ASCII controls and whitespace in URI schemes. Normalize
	// them before applying goldmark's dangerous-URL policy so variants such as
	// "java\tscript:" cannot become active links in an embedded SVG.
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return -1
		}
		return r
	}, link)
	if goldmarkHTML.IsDangerousURL([]byte(normalized)) {
		return ""
	}
	return link
}

func defaultMarkdownSVGRolePaint() map[MarkdownColorRole]MarkdownSVGPaint {
	return map[MarkdownColorRole]MarkdownSVGPaint{
		MarkdownColorForeground:   {Class: "md-color-fg", Color: "currentColor"},
		MarkdownColorMuted:        {Class: "md-color-muted", Color: "var(--color-fg-muted, currentColor)"},
		MarkdownColorAccent:       {Class: "md-color-accent", Color: "var(--color-accent-fg, currentColor)"},
		MarkdownColorBorder:       {Class: "md-color-border", Color: "var(--color-border-default, currentColor)"},
		MarkdownColorBorderMuted:  {Class: "md-color-border-muted", Color: "var(--color-border-muted, currentColor)"},
		MarkdownColorCanvas:       {Class: "md-color-canvas", Color: "var(--color-canvas-default, none)"},
		MarkdownColorCanvasSubtle: {Class: "md-color-canvas-subtle", Color: "var(--color-canvas-subtle, none)"},
		MarkdownColorNeutralMuted: {Class: "md-color-neutral-muted", Color: "var(--color-neutral-muted, none)"},
	}
}

func defaultMarkdownSVGFontClasses() map[MarkdownFontRole]string {
	return map[MarkdownFontRole]string{
		MarkdownFontRegular:      "text",
		MarkdownFontSemibold:     "text-semibold",
		MarkdownFontBold:         "text-bold",
		MarkdownFontItalic:       "text-italic",
		MarkdownFontMono:         "text-mono",
		MarkdownFontMonoSemibold: "text-mono-semibold",
		MarkdownFontMonoBold:     "text-mono-bold",
		MarkdownFontMonoItalic:   "text-mono-italic",
	}
}

func mdFloat(v float64) string {
	if math.Abs(v) < 0.0005 {
		v = 0
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}

type markdownPaintContext struct {
	fontFamily       *d2fonts.FontFamily
	monoFontFamily   *d2fonts.FontFamily
	fontSize         int
	fontWeight       d2fonts.FontStyle
	italic           bool
	lineHeightFactor float64
	mono             bool
	inlineCode       bool
	headingCode      bool
	role             MarkdownColorRole
	link             string
	linkTitle        string
	decoration       MarkdownTextDecoration
}

func (ctx markdownPaintContext) effectiveFontStyle() d2fonts.FontStyle {
	return markdownEffectiveFontStyle(ctx.fontWeight, ctx.italic)
}

func (ctx markdownPaintContext) syntheticItalic() bool {
	return ctx.italic && ctx.fontWeight != d2fonts.FONT_STYLE_REGULAR
}

func (ctx markdownPaintContext) fontRole() MarkdownFontRole {
	fontStyle := ctx.effectiveFontStyle()
	if ctx.mono {
		switch fontStyle {
		case d2fonts.FONT_STYLE_SEMIBOLD:
			return MarkdownFontMonoSemibold
		case d2fonts.FONT_STYLE_BOLD:
			return MarkdownFontMonoBold
		case d2fonts.FONT_STYLE_ITALIC:
			return MarkdownFontMonoItalic
		default:
			return MarkdownFontMono
		}
	}
	switch fontStyle {
	case d2fonts.FONT_STYLE_SEMIBOLD:
		return MarkdownFontSemibold
	case d2fonts.FONT_STYLE_BOLD:
		return MarkdownFontBold
	case d2fonts.FONT_STYLE_ITALIC:
		return MarkdownFontItalic
	default:
		return MarkdownFontRegular
	}
}

type markdownPainter struct {
	ruler  *Ruler
	layout *MarkdownLayout
}

func (p *markdownPainter) measure(n *html.Node, ctx markdownPaintContext) blockAttrs {
	original := p.ruler.LineHeightFactor
	p.ruler.LineHeightFactor = ctx.lineHeightFactor
	attrs := p.ruler.measureNode(0, n, ctx.fontFamily, ctx.monoFontFamily, ctx.fontSize, ctx.fontWeight, ctx.italic)
	p.ruler.LineHeightFactor = original
	return attrs
}

func (p *markdownPainter) childContext(n *html.Node, ctx markdownPaintContext) markdownPaintContext {
	if n.Type != html.ElementNode {
		return ctx
	}
	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		ctx.fontSize = HeaderToFontSize(ctx.fontSize, n.Data)
		ctx.fontWeight = d2fonts.FONT_STYLE_SEMIBOLD
		ctx.lineHeightFactor = LineHeight_h
		if n.Data == "h6" {
			ctx.role = MarkdownColorMuted
		}
	case "em":
		ctx.italic = true
	case "b", "strong":
		ctx.fontWeight = d2fonts.FONT_STYLE_BOLD
	case "pre", "code":
		ctx.fontFamily = ctx.monoFontFamily
		ctx.mono = true
		ctx.inlineCode = n.Data == "code" && (n.Parent == nil || n.Parent.Data != "pre")
		ctx.headingCode = ctx.inlineCode && n.Parent != nil && isMarkdownHeading(n.Parent.Data)
	case "a":
		ctx.link = nodeAttr(n, "href")
		ctx.linkTitle = nodeAttr(n, "title")
		ctx.role = MarkdownColorAccent
	case "del", "s", "strike":
		ctx.decoration = MarkdownTextDecorationLineThrough
	case "blockquote":
		ctx.role = MarkdownColorMuted
	}
	return ctx
}

func (p *markdownPainter) paintBlock(n *html.Node, x, y float64, parentCtx markdownPaintContext, availableWidth float64) {
	attrs := p.measure(n, parentCtx)
	ctx := p.childContext(n, parentCtx)
	if n.Type != html.ElementNode {
		p.paintInlineNodes([]*html.Node{n}, x, y, parentCtx, attrs.height)
		return
	}

	switch n.Data {
	case "hr":
		width := availableWidth
		if width <= 0 {
			width = attrs.width
		}
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownLinePrimitive, X: x, Y: y + attrs.height/2, X2: x + width, Y2: y + attrs.height/2,
			StrokeWidth: goMax(1, Height_hr_em*float64(ctx.fontSize)), StrokeRole: MarkdownColorBorder,
		})
		return
	case "pre":
		p.paintPre(n, x, y, attrs, ctx, availableWidth)
		return
	case "table":
		p.paintTable(n, x, y, attrs, ctx)
		return
	}

	contentX, contentY := x, y
	switch n.Data {
	case "blockquote":
		borderWidth := BorderLeft_blockquote_em * float64(ctx.fontSize)
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownRectPrimitive, X: x, Y: y, Width: borderWidth, Height: attrs.height,
			FillRole: MarkdownColorBorder,
		})
		inset := borderWidth + PaddingLR_blockquote_em*float64(ctx.fontSize)
		contentX += inset
		availableWidth = goMax(0, availableWidth-inset-PaddingLR_blockquote_em*float64(ctx.fontSize))
	case "li":
		inset := p.listIndent(n, ctx)
		contentX += inset
		availableWidth = goMax(0, availableWidth-inset)
		p.paintListMarker(n, x, y, ctx, inset)
	}

	p.paintFlow(n, contentX, contentY, ctx, availableWidth)

	if n.Data == "h1" || n.Data == "h2" {
		width := availableWidth
		if width <= 0 {
			width = attrs.width
		}
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownLinePrimitive, X: x, Y: y + attrs.height - BorderBottom_h1_h2/2,
			X2: x + width, Y2: y + attrs.height - BorderBottom_h1_h2/2,
			StrokeWidth: BorderBottom_h1_h2, StrokeRole: MarkdownColorBorderMuted,
		})
	}
}

type markdownFlowUnit struct {
	block  *html.Node
	inline []*html.Node
	attrs  blockAttrs
}

func (p *markdownPainter) paintFlow(n *html.Node, x, y float64, ctx markdownPaintContext, availableWidth float64) {
	units := p.flowUnits(n, ctx)
	currentY := y
	previousMarginBottom := 0.0
	for i, unit := range units {
		if i > 0 {
			currentY += goMax(previousMarginBottom, unit.attrs.marginTop)
		}
		if unit.block != nil {
			p.paintBlock(unit.block, x, currentY, ctx, availableWidth)
		} else {
			p.paintInlineNodes(unit.inline, x, currentY, ctx, unit.attrs.height)
		}
		currentY += unit.attrs.height
		previousMarginBottom = unit.attrs.marginBottom
	}
}

func (p *markdownPainter) flowUnits(n *html.Node, ctx markdownPaintContext) []markdownFlowUnit {
	var units []markdownFlowUnit
	var inlineNodes []*html.Node
	inlineAttrs := blockAttrs{}
	lineHeight := float64(ctx.fontSize) * ctx.lineHeightFactor

	flushInline := func() {
		if len(inlineNodes) == 0 || !inlineAttrs.isNotEmpty() {
			inlineNodes = nil
			inlineAttrs = blockAttrs{}
			return
		}
		if inlineAttrs.height < lineHeight {
			inlineAttrs.height = lineHeight
		}
		units = append(units, markdownFlowUnit{inline: inlineNodes, attrs: inlineAttrs})
		inlineNodes = nil
		inlineAttrs = blockAttrs{}
	}

	first := getNext(n.FirstChild)
	last := getPrev(n.LastChild)
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && isBlockElement(child.Data) {
			flushInline()
			attrs := p.measure(child, ctx)
			if child == first && n.Data == "blockquote" {
				attrs.marginTop = 0
			}
			if child == last && n.Data == "blockquote" {
				attrs.marginBottom = 0
			}
			units = append(units, markdownFlowUnit{block: child, attrs: attrs})
			continue
		}
		if child.Type == html.ElementNode && child.Data == "br" {
			if inlineAttrs.isNotEmpty() {
				flushInline()
			} else {
				units = append(units, markdownFlowUnit{attrs: blockAttrs{height: lineHeight}})
			}
			continue
		}

		attrs := p.measure(child, ctx)
		inlineNodes = append(inlineNodes, child)
		if attrs.isNotEmpty() {
			inlineAttrs.width += attrs.width
			inlineAttrs.height = goMax(inlineAttrs.height, attrs.height)
			inlineAttrs.marginTop = goMax(inlineAttrs.marginTop, attrs.marginTop)
			inlineAttrs.marginBottom = goMax(inlineAttrs.marginBottom, attrs.marginBottom)
		}
	}
	flushInline()
	return units
}

type markdownInlineToken struct {
	breakLine  bool
	padding    bool
	text       string
	width      float64
	height     float64
	ctx        markdownPaintContext
	codeGroup  int
	codeHeight float64
}

func (p *markdownPainter) paintInlineNodes(nodes []*html.Node, x, y float64, ctx markdownPaintContext, measuredHeight float64) {
	var tokens []markdownInlineToken
	nextCodeGroup := 0
	for _, n := range nodes {
		p.collectInlineTokens(n, ctx, &tokens, &nextCodeGroup, 0, 0)
	}
	if len(tokens) == 0 {
		return
	}

	lines := [][]markdownInlineToken{{}}
	for _, token := range tokens {
		if token.breakLine {
			lines = append(lines, nil)
			continue
		}
		lines[len(lines)-1] = append(lines[len(lines)-1], token)
	}
	lineHeight := float64(ctx.fontSize) * ctx.lineHeightFactor
	if len(lines) > 0 && measuredHeight > 0 && float64(len(lines))*lineHeight > measuredHeight+0.5 {
		lineHeight = measuredHeight / float64(len(lines))
	}
	for lineIndex, line := range lines {
		lineY := y + float64(lineIndex)*lineHeight
		cursorX := x
		codeSpans := make(map[int][3]float64)
		for _, token := range line {
			if token.codeGroup > 0 {
				span, ok := codeSpans[token.codeGroup]
				if !ok {
					span = [3]float64{cursorX, cursorX, token.codeHeight}
				}
				span[1] = cursorX + token.width
				span[2] = goMax(span[2], token.codeHeight)
				codeSpans[token.codeGroup] = span
			}
			cursorX += token.width
		}
		groupIDs := make([]int, 0, len(codeSpans))
		for groupID := range codeSpans {
			groupIDs = append(groupIDs, groupID)
		}
		sort.Ints(groupIDs)
		for _, groupID := range groupIDs {
			span := codeSpans[groupID]
			height := span[2]
			p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
				Kind: MarkdownRectPrimitive, X: span[0], Y: lineY + (lineHeight-height)/2,
				Width: span[1] - span[0], Height: height, Radius: 6, FillRole: MarkdownColorNeutralMuted,
			})
		}

		cursorX = x
		for _, token := range line {
			if token.padding {
				cursorX += token.width
				continue
			}
			fontSize := float64(token.ctx.fontSize)
			fontScale := 1.0
			if token.ctx.inlineCode && !token.ctx.headingCode {
				fontScale = FontSize_pre_code_em
				fontSize *= fontScale
			}
			baselineY := p.textBaseline(token.ctx, lineY, lineHeight, fontScale)
			if token.ctx.inlineCode && token.codeHeight > 0 {
				codeTop := lineY + (lineHeight-token.codeHeight)/2
				verticalPadding := 0.0
				if !token.ctx.headingCode {
					verticalPadding = PaddingTopBottom_code_em * float64(token.ctx.fontSize)
				}
				contentTop := codeTop + verticalPadding
				contentHeight := goMax(0, token.codeHeight-2*verticalPadding)
				baselineY = p.textBaseline(token.ctx, contentTop, contentHeight, fontScale)
			}
			p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
				Kind: MarkdownTextPrimitive, X: cursorX, Y: baselineY, Width: token.width, Height: token.height,
				Text: token.text, Font: token.ctx.fontRole(), FontSize: fontSize,
				FillRole: token.ctx.role, Link: token.ctx.link, LinkTitle: token.ctx.linkTitle,
				Decoration: token.ctx.decoration, SyntheticItalic: token.ctx.syntheticItalic(),
			})
			cursorX += token.width
		}
	}
}

func (p *markdownPainter) collectInlineTokens(n *html.Node, ctx markdownPaintContext, tokens *[]markdownInlineToken, nextCodeGroup *int, codeGroup int, codeHeight float64) {
	if n.Type == html.TextNode {
		text := renderableMarkdownText(n)
		attrs := p.measure(n, ctx)
		if text == "" && attrs.width == 0 {
			return
		}
		*tokens = append(*tokens, markdownInlineToken{
			text: text, width: attrs.width, height: attrs.height, ctx: ctx,
			codeGroup: codeGroup, codeHeight: codeHeight,
		})
		return
	}
	if n.Type != html.ElementNode {
		return
	}
	if n.Data == "br" {
		if !ctx.inlineCode {
			*tokens = append(*tokens, markdownInlineToken{breakLine: true})
		}
		return
	}
	if n.Data == "img" {
		alt := nodeAttr(n, "alt")
		attrs := p.measure(n, ctx)
		if alt != "" {
			*tokens = append(*tokens, markdownInlineToken{
				text: alt, width: attrs.width, height: attrs.height, ctx: ctx,
				codeGroup: codeGroup, codeHeight: codeHeight,
			})
		}
		return
	}

	childCtx := p.childContext(n, ctx)
	if n.Data == "code" && (n.Parent == nil || n.Parent.Data != "pre") {
		*nextCodeGroup++
		codeGroup = *nextCodeGroup
		attrs := p.measure(n, ctx)
		codeHeight = attrs.height
		padding := inlineCodeHorizontalPadding(childCtx)
		*tokens = append(*tokens, markdownInlineToken{
			padding: true, width: padding, ctx: childCtx, codeGroup: codeGroup, codeHeight: codeHeight,
		})
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		p.collectInlineTokens(child, childCtx, tokens, nextCodeGroup, codeGroup, codeHeight)
	}
	if n.Data == "code" && (n.Parent == nil || n.Parent.Data != "pre") {
		padding := inlineCodeHorizontalPadding(childCtx)
		*tokens = append(*tokens, markdownInlineToken{
			padding: true, width: padding, ctx: childCtx, codeGroup: codeGroup, codeHeight: codeHeight,
		})
	}
}

func inlineCodeHorizontalPadding(ctx markdownPaintContext) float64 {
	if ctx.headingCode {
		return PaddingLeftRight_heading_code_em * float64(ctx.fontSize)
	}
	return PaddingLeftRight_code_em * float64(ctx.fontSize)
}

func renderableMarkdownText(n *html.Node) string {
	return n.Data
}

func (p *markdownPainter) paintPre(n *html.Node, x, y float64, attrs blockAttrs, ctx markdownPaintContext, availableWidth float64) {
	backgroundWidth := goMax(attrs.width, availableWidth)
	p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
		Kind: MarkdownRectPrimitive, X: x, Y: y, Width: backgroundWidth, Height: attrs.height,
		Radius: 6, FillRole: MarkdownColorCanvasSubtle,
	})
	text := markdownNodeText(n)
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" && strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	fontSize := FontSize_pre_code_em * float64(ctx.fontSize)
	lineHeight := LineHeight_pre * fontSize
	backgroundHeight := float64(len(lines))*lineHeight + 2*Padding_pre
	if len(lines) == 0 {
		backgroundHeight = 2 * Padding_pre
	}
	p.layout.Primitives[len(p.layout.Primitives)-1].Height = backgroundHeight
	for i, line := range lines {
		if line == "" {
			continue
		}
		line = expandMarkdownTabs(line)
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownTextPrimitive, X: x + Padding_pre,
			Y:    p.textBaseline(ctx, y+Padding_pre+float64(i)*lineHeight, lineHeight, FontSize_pre_code_em),
			Text: line, Font: ctx.fontRole(), FontSize: fontSize, FillRole: ctx.role,
			SyntheticItalic: ctx.syntheticItalic(),
		})
	}
}

func expandMarkdownTabs(line string) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}
	var out strings.Builder
	column := 0
	graphemes := uniseg.NewGraphemes(line)
	for graphemes.Next() {
		grapheme := graphemes.Str()
		if grapheme == "\t" {
			spaces := TAB_SIZE - column%TAB_SIZE
			out.WriteString(strings.Repeat(" ", spaces))
			column += spaces
			continue
		}
		out.WriteString(grapheme)
		width := graphemes.Width()
		if width < 1 {
			width = 1
		}
		column += width
	}
	return out.String()
}

type markdownTableRow struct {
	node   *html.Node
	cells  []*html.Node
	header bool
	height float64
	widths []float64
	stripe bool
}

func (p *markdownPainter) paintTable(n *html.Node, x, y float64, attrs blockAttrs, ctx markdownPaintContext) {
	rows := collectMarkdownTableRows(n)
	columnWidths := make([]float64, 0)
	for i := range rows {
		rowCtx := ctx
		if rows[i].header {
			rowCtx.fontWeight = d2fonts.FONT_STYLE_SEMIBOLD
		}
		maxCellHeight := 0.0
		for _, cell := range rows[i].cells {
			cellCtx := rowCtx
			if cell.Data == "th" {
				cellCtx.fontWeight = d2fonts.FONT_STYLE_SEMIBOLD
			}
			cellAttrs := p.measure(cell, cellCtx)
			rows[i].widths = append(rows[i].widths, cellAttrs.width+26)
			maxCellHeight = goMax(maxCellHeight, cellAttrs.height+12)
		}
		rows[i].height = goMax(maxCellHeight+1, float64(ctx.fontSize)*ctx.lineHeightFactor)
		columnWidths = mergeColumnWidths(columnWidths, [][]float64{rows[i].widths})
	}

	p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
		Kind: MarkdownRectPrimitive, X: x, Y: y, Width: attrs.width, Height: attrs.height,
		FillRole: MarkdownColorCanvas, StrokeRole: MarkdownColorBorder, StrokeWidth: 1,
	})
	rowY := y + 1
	for _, row := range rows {
		if row.stripe {
			p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
				Kind: MarkdownRectPrimitive, X: x + 1, Y: rowY, Width: goMax(0, attrs.width-2), Height: row.height,
				FillRole: MarkdownColorCanvasSubtle,
			})
		}
		cursorX := x + 1
		for cellIndex, cell := range row.cells {
			cellCtx := ctx
			if row.header || cell.Data == "th" {
				cellCtx.fontWeight = d2fonts.FONT_STYLE_SEMIBOLD
			}
			contentAttrs := p.measure(cell, cellCtx)
			contentX := cursorX + 13
			available := columnWidths[cellIndex] - 26
			switch nodeAttr(cell, "align") {
			case "right":
				contentX += goMax(0, available-contentAttrs.width)
			case "center":
				contentX += goMax(0, available-contentAttrs.width) / 2
			}
			p.paintFlow(cell, contentX, rowY+6, cellCtx, available)
			cursorX += columnWidths[cellIndex]
			p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
				Kind: MarkdownLinePrimitive, X: cursorX, Y: rowY, X2: cursorX, Y2: rowY + row.height,
				StrokeRole: MarkdownColorBorder, StrokeWidth: 1,
			})
			cursorX++
		}
		rowY += row.height
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownLinePrimitive, X: x, Y: rowY, X2: x + attrs.width, Y2: rowY,
			StrokeRole: MarkdownColorBorder, StrokeWidth: 1,
		})
	}
}

func collectMarkdownTableRows(table *html.Node) []markdownTableRow {
	var rows []markdownTableRow
	appendRow := func(n *html.Node, inHeader, stripe bool) {
		row := markdownTableRow{node: n, header: inHeader, stripe: stripe}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
				row.cells = append(row.cells, child)
				if child.Data == "th" {
					row.header = true
				}
			}
		}
		rows = append(rows, row)
	}
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inHeader bool) {
		if n.Type != html.ElementNode {
			return
		}
		if n.Data == "thead" || n.Data == "tbody" || n.Data == "tfoot" || n.Data == "table" {
			if n.Data == "thead" {
				inHeader = true
			}
			rowIndex := 0
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && child.Data == "tr" {
					appendRow(child, inHeader, rowIndex%2 == 1)
					rowIndex++
				} else {
					walk(child, inHeader)
				}
			}
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inHeader)
		}
	}
	walk(table, false)
	return rows
}

func markdownListMarker(n *html.Node) string {
	if n.Parent == nil || n.Parent.Type != html.ElementNode {
		return ""
	}
	marker := "•"
	if n.Parent.Data == "ul" {
		depth := 0
		for ancestor := n.Parent.Parent; ancestor != nil; ancestor = ancestor.Parent {
			if ancestor.Type == html.ElementNode && (ancestor.Data == "ol" || ancestor.Data == "ul") {
				depth++
			}
		}
		switch depth % 3 {
		case 1:
			marker = "◦"
		case 2:
			marker = "▪"
		}
	}
	if n.Parent.Data == "ol" {
		index := 1
		if start, err := strconv.Atoi(nodeAttr(n.Parent, "start")); err == nil {
			index = start
		}
		for sibling := n.PrevSibling; sibling != nil; sibling = sibling.PrevSibling {
			if sibling.Type == html.ElementNode && sibling.Data == "li" {
				index++
			}
		}
		style := nodeAttr(n.Parent, "type")
		if style == "" {
			depth := 0
			for ancestor := n.Parent.Parent; ancestor != nil; ancestor = ancestor.Parent {
				if ancestor.Type == html.ElementNode && (ancestor.Data == "ol" || ancestor.Data == "ul") {
					depth++
				}
			}
			switch depth {
			case 0:
				style = "1"
			case 1:
				style = "i"
			default:
				style = "a"
			}
		}
		marker = orderedListMarker(index, style)
	}
	return marker
}

func markdownListIndent(markerWidth float64, fontSize int) float64 {
	return goMax(PaddingLeft_ul_ol_em*float64(fontSize), markerWidth+float64(fontSize)/4)
}

func (p *markdownPainter) listIndent(n *html.Node, ctx markdownPaintContext) float64 {
	marker := markdownListMarker(n)
	if marker == "" {
		return PaddingLeft_ul_ol_em * float64(ctx.fontSize)
	}
	fontFamily := ctx.fontFamily
	if ctx.mono {
		fontFamily = ctx.monoFontFamily
	}
	markerWidth, _ := p.ruler.MeasurePrecise(fontFamily.Font(ctx.fontSize, ctx.effectiveFontStyle()), marker)
	return markdownListIndent(markerWidth, ctx.fontSize)
}

func (p *markdownPainter) paintListMarker(n *html.Node, x, y float64, ctx markdownPaintContext, inset float64) {
	marker := markdownListMarker(n)
	if marker == "" {
		return
	}
	fontFamily := ctx.fontFamily
	if ctx.mono {
		fontFamily = ctx.monoFontFamily
	}
	markerWidth, markerHeight := p.ruler.MeasurePrecise(fontFamily.Font(ctx.fontSize, ctx.effectiveFontStyle()), marker)
	p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
		Kind: MarkdownTextPrimitive,
		X:    x + inset - markerWidth - float64(ctx.fontSize)/4,
		Y:    p.textBaseline(ctx, y, float64(ctx.fontSize)*ctx.lineHeightFactor, 1), Width: markerWidth, Height: markerHeight,
		Text: marker, Font: ctx.fontRole(), FontSize: float64(ctx.fontSize),
		FillRole: ctx.role, SyntheticItalic: ctx.syntheticItalic(),
	})
}

func orderedListMarker(index int, style string) string {
	switch strings.ToLower(style) {
	case "a":
		return lowerAlpha(index) + "."
	case "i":
		return lowerRoman(index) + "."
	default:
		return strconv.Itoa(index) + "."
	}
}

func lowerAlpha(index int) string {
	if index <= 0 {
		return strconv.Itoa(index)
	}
	var out []byte
	for index > 0 {
		index--
		out = append(out, byte('a'+index%26))
		index /= 26
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func lowerRoman(index int) string {
	if index <= 0 || index > 3999 {
		return strconv.Itoa(index)
	}
	values := []struct {
		value int
		text  string
	}{
		{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"},
		{100, "c"}, {90, "xc"}, {50, "l"}, {40, "xl"},
		{10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"},
	}
	var out strings.Builder
	for _, numeral := range values {
		for index >= numeral.value {
			out.WriteString(numeral.text)
			index -= numeral.value
		}
	}
	return out.String()
}

func markdownNodeText(n *html.Node) string {
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
			return
		}
		if node.Type == html.ElementNode && node.Data == "br" {
			out.WriteByte('\n')
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return out.String()
}

func nodeAttr(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func validateMarkdownSVGNodes(n *html.Node) error {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "html", "head", "body",
			"p", "h1", "h2", "h3", "h4", "h5", "h6",
			"blockquote", "ul", "ol", "li", "pre", "code",
			"em", "b", "strong", "del", "s", "strike", "a", "br", "hr", "img",
			"table", "thead", "tbody", "tfoot", "tr", "td", "th":
		default:
			return fmt.Errorf("native Markdown SVG does not support HTML element <%s>", n.Data)
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if err := validateMarkdownSVGNodes(child); err != nil {
			return err
		}
	}
	return nil
}

func goMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// textBaseline matches CSS line-box placement: half of the extra line-height
// sits above the rounded OpenType font box, followed by its ascent. This is the
// same box exposed by Canvas TextMetrics for D2's embedded fonts.
func (p *markdownPainter) textBaseline(ctx markdownPaintContext, top, lineHeight, scale float64) float64 {
	fontFamily := ctx.fontFamily
	if ctx.mono {
		fontFamily = ctx.monoFontFamily
	}
	font := fontFamily.Font(ctx.fontSize, ctx.effectiveFontStyle())
	if ascent, descent, ok := p.ruler.cssFontBoxMetrics(font, float64(ctx.fontSize)*scale); ok {
		leading := goMax(0, lineHeight-ascent-descent)
		return top + leading/2 + ascent
	}
	if _, ok := p.ruler.atlases[font]; !ok {
		p.ruler.addFontSize(font)
	}
	atlas := p.ruler.atlases[font]
	fontHeight := atlas.lineHeight * scale
	leading := goMax(0, lineHeight-fontHeight)
	return top + leading/2 + atlas.Ascent()*scale
}
