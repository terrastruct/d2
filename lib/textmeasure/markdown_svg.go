package textmeasure

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

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
	MarkdownColorNone             MarkdownColorRole = ""
	MarkdownColorForeground       MarkdownColorRole = "foreground"
	MarkdownColorForegroundStroke MarkdownColorRole = "foreground-stroke"
	MarkdownColorMuted            MarkdownColorRole = "muted"
	MarkdownColorMutedStroke      MarkdownColorRole = "muted-stroke"
	MarkdownColorAccent           MarkdownColorRole = "accent"
	MarkdownColorBorder           MarkdownColorRole = "border"
	MarkdownColorBorderMuted      MarkdownColorRole = "border-muted"
	MarkdownColorCanvas           MarkdownColorRole = "canvas"
	MarkdownColorCanvasSubtle     MarkdownColorRole = "canvas-subtle"
	MarkdownColorNeutralMuted     MarkdownColorRole = "neutral-muted"
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
	// SyntheticBold/SyntheticItalic preserve inherited CSS axes when the
	// innermost Markdown element selects a different concrete D2 font family.
	// For example, em > strong uses the bold face with a synthetic slant, while
	// strong > em uses the italic face with synthetic weight.
	SyntheticBold   bool
	SyntheticItalic bool
	// TextLength fits the glyphs to Width. CSS paints a discretionary soft
	// hyphen at one third of an em even inside D2's monospace code face.
	TextLength bool
}

// MarkdownLayout contains the exact dimensions used by D2 layout and the
// positioned primitives used to paint those dimensions. MeasureMarkdown calls
// LayoutMarkdown, ensuring measurement and rendering share one code path.
type MarkdownLayout struct {
	Width, Height int
	Primitives    []MarkdownPrimitive
	// Corpus contains the visible source text used by the primitives. Renderers
	// embedding subset fonts should include it.
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
	// Keep an untouched parse tree for D2's established graph-layout
	// measurement. Native painting normalizes whitespace to CSS semantics, but
	// measuring that normalized tree would subtly resize old diagrams (notably
	// a newline immediately after <br> in mixed-Unicode content).
	legacyDoc, err := goquery.NewDocumentFromReader(strings.NewReader(render))
	if err != nil {
		return nil, err
	}
	legacyBody := legacyDoc.Find("body").First()
	if len(legacyBody.Nodes) == 0 {
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
		fontScale:        1,
		fontStyle:        d2fonts.FONT_STYLE_REGULAR,
		lineHeightFactor: MarkdownLineHeight,
		mono:             *fontFamily == *monoFontFamily,
		role:             MarkdownColorForeground,
	}
	p := markdownPainter{ruler: ruler, layout: &MarkdownLayout{}}
	// D2's historical Markdown measurements are graph-layout API: changing them
	// moves shapes and routes. Paint with browser-style CSS line boxes below,
	// but retain those established outer dimensions.
	rootAttrs := ruler.measureNode(0, legacyBody.Nodes[0], fontFamily, monoFontFamily, fontSize, d2fonts.FONT_STYLE_REGULAR, false, false)
	contentWidth := int(math.Ceil(rootAttrs.width))
	contentHeight := int(math.Ceil(rootAttrs.height))
	p.layout.Width = contentWidth
	p.layout.Height = contentHeight
	p.paintBlock(body.Nodes[0], 0, 0, ctx, float64(contentWidth))
	var corpus strings.Builder
	for _, primitive := range p.layout.Primitives {
		if primitive.Kind == MarkdownTextPrimitive {
			corpus.WriteString(primitive.Text)
		}
	}
	p.layout.Corpus = corpus.String()
	return p.layout, nil
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
		link := SafeMarkdownLink(primitive.Link)
		if link != "" && !opts.DisableLinks {
			fmt.Fprintf(&out, `<a href="%s" xlink:href="%[1]s">`, svg.EscapeText(link))
			if primitive.LinkTitle != "" {
				fmt.Fprintf(&out, `<title>%s</title>`, svg.EscapeText(primitive.LinkTitle))
			}
		}

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
			if primitive.SyntheticBold {
				out.WriteString(` font-weight="bold"`)
			}
			if primitive.SyntheticItalic {
				out.WriteString(` font-style="italic"`)
			}
			if primitive.TextLength {
				fmt.Fprintf(&out, ` textLength="%s" lengthAdjust="spacingAndGlyphs"`, mdFloat(primitive.Width))
			}
			fmt.Fprintf(&out, `>%s</text>`, svg.EscapeText(primitive.Text))
		}
		if link != "" && !opts.DisableLinks {
			out.WriteString(`</a>`)
		}
	}
	out.WriteString(`</g>`)
	return out.String()
}

// SafeMarkdownLink returns link when it is safe to expose as interactive
// metadata and an empty string for unsafe Markdown URL schemes. PDF and PPTX
// annotations use the same policy.
func SafeMarkdownLink(link string) string {
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
		MarkdownColorForeground:       {Class: "md-color-fg", Color: "currentColor"},
		MarkdownColorForegroundStroke: {Class: "md-stroke-fg", Color: "currentColor"},
		MarkdownColorMuted:            {Class: "md-color-muted", Color: "var(--color-fg-muted, currentColor)"},
		MarkdownColorMutedStroke:      {Class: "md-stroke-muted", Color: "var(--color-fg-muted, currentColor)"},
		MarkdownColorAccent:           {Class: "md-color-accent", Color: "var(--color-accent-fg, currentColor)"},
		MarkdownColorBorder:           {Class: "md-color-border", Color: "var(--color-border-default, currentColor)"},
		MarkdownColorBorderMuted:      {Class: "md-color-border-muted", Color: "var(--color-border-muted, currentColor)"},
		MarkdownColorCanvas:           {Class: "md-color-canvas", Color: "var(--color-canvas-default, none)"},
		MarkdownColorCanvasSubtle:     {Class: "md-color-canvas-subtle", Color: "var(--color-canvas-subtle, none)"},
		MarkdownColorNeutralMuted:     {Class: "md-color-neutral-muted", Color: "var(--color-neutral-muted, none)"},
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

func markdownSnap(v float64) float64 {
	return math.Floor(v + 0.5)
}

type markdownPaintContext struct {
	fontFamily       *d2fonts.FontFamily
	monoFontFamily   *d2fonts.FontFamily
	fontSize         int
	fontScale        float64
	fontStyle        d2fonts.FontStyle
	bold             bool
	italic           bool
	lineHeightFactor float64
	mono             bool
	inlineCode       bool
	headingCode      bool
	heading          string
	role             MarkdownColorRole
	link             string
	linkTitle        string
	decoration       MarkdownTextDecoration
	decorationGroup  int
	decorationRole   MarkdownColorRole
	tableCell        bool
	textAlign        string
}

func (ctx markdownPaintContext) cssFontSize() float64 {
	return float64(ctx.fontSize) * ctx.fontScale
}

func (ctx markdownPaintContext) effectiveFontStyle() d2fonts.FontStyle {
	return ctx.fontStyle
}

func (ctx markdownPaintContext) syntheticBold() bool {
	return ctx.bold && ctx.fontStyle != d2fonts.FONT_STYLE_BOLD && ctx.fontStyle != d2fonts.FONT_STYLE_SEMIBOLD
}

func (ctx markdownPaintContext) syntheticItalic() bool {
	return ctx.italic && ctx.fontStyle != d2fonts.FONT_STYLE_ITALIC
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
	attrs := p.ruler.measureCSSNode(0, n, ctx.fontFamily, ctx.monoFontFamily, ctx.fontSize, ctx.fontScale, ctx.fontStyle, ctx.bold, ctx.italic)
	p.ruler.LineHeightFactor = original
	return attrs
}

func (p *markdownPainter) childContext(n *html.Node, ctx markdownPaintContext) markdownPaintContext {
	if n.Type != html.ElementNode {
		return ctx
	}
	switch n.Data {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		ctx.heading = n.Data
		ctx.fontScale *= HeaderToFontScale(n.Data)
		ctx.fontStyle = d2fonts.FONT_STYLE_SEMIBOLD
		ctx.bold = false
		ctx.lineHeightFactor = LineHeight_h
		if n.Data == "h6" {
			ctx.role = MarkdownColorMuted
		}
	case "em":
		ctx.fontStyle = d2fonts.FONT_STYLE_ITALIC
		ctx.italic = true
	case "b", "strong":
		ctx.fontStyle = d2fonts.FONT_STYLE_BOLD
		ctx.bold = true
	case "pre", "code":
		ctx.fontFamily = ctx.monoFontFamily
		ctx.mono = true
		ctx.fontStyle = d2fonts.FONT_STYLE_REGULAR
		ctx.inlineCode = n.Data == "code" && (n.Parent == nil || n.Parent.Data != "pre")
		ctx.headingCode = ctx.inlineCode && hasMarkdownHeadingAncestor(n)
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

func (p *markdownPainter) paintBlock(n *html.Node, x, y float64, parentCtx markdownPaintContext, availableWidth float64) float64 {
	attrs := p.measure(n, parentCtx)
	ctx := p.childContext(n, parentCtx)
	if n.Type != html.ElementNode {
		return p.paintInlineNodes([]*html.Node{n}, x, y, parentCtx, availableWidth)
	}

	switch n.Data {
	case "hr":
		width := availableWidth
		if width <= 0 {
			width = attrs.width
		}
		x1, x2 := markdownSnap(x), markdownSnap(x+width)
		y1, y2 := markdownSnap(y), markdownSnap(y+Height_hr_em*ctx.cssFontSize())
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownRectPrimitive, X: x1, Y: y1, Width: x2 - x1,
			Height: y2 - y1, FillRole: MarkdownColorBorder,
		})
		return attrs.height
	case "pre":
		return p.paintPre(n, x, y, attrs, ctx, availableWidth)
	case "table":
		return p.paintTable(n, x, y, attrs, ctx, availableWidth)
	}

	contentX, contentY := x, y
	blockquoteBorderWidth := 0.0
	switch n.Data {
	case "blockquote":
		blockquoteBorderWidth = goMax(1, math.Floor(BorderLeft_blockquote_em*ctx.cssFontSize()))
		inset := blockquoteBorderWidth + PaddingLR_blockquote_em*ctx.cssFontSize()
		contentX += inset
		availableWidth = goMax(0, availableWidth-inset-PaddingLR_blockquote_em*ctx.cssFontSize())
	case "li":
		inset := p.listIndent(n, ctx)
		contentX += inset
		availableWidth = goMax(0, availableWidth-inset)
		p.paintListMarker(n, x, y, ctx, inset)
	}

	contentHeight := p.paintFlow(n, contentX, contentY, ctx, availableWidth)
	paintedHeight := goMax(attrs.height, contentHeight)
	if n.Data == "h1" || n.Data == "h2" {
		lineHeight := ctx.cssFontSize() * ctx.lineHeightFactor
		if contentHeight > lineHeight {
			paintedHeight = attrs.height + contentHeight - lineHeight
		}
	}

	if n.Data == "blockquote" {
		barY1, barY2 := markdownSnap(y), markdownSnap(y+paintedHeight)
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownRectPrimitive, X: markdownSnap(x), Y: barY1, Width: blockquoteBorderWidth, Height: barY2 - barY1,
			FillRole: MarkdownColorBorder,
		})
	}

	if n.Data == "h1" || n.Data == "h2" {
		width := availableWidth
		if width <= 0 {
			width = attrs.width
		}
		x1, x2 := markdownSnap(x), markdownSnap(x+width)
		rawTop := y + paintedHeight - BorderBottom_h1_h2
		y1, y2 := markdownSnap(rawTop), markdownSnap(rawTop+BorderBottom_h1_h2)
		p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
			Kind: MarkdownRectPrimitive, X: x1, Y: y1,
			Width: x2 - x1, Height: y2 - y1, FillRole: MarkdownColorBorderMuted,
		})
	}
	return paintedHeight
}

type markdownFlowUnit struct {
	block  *html.Node
	inline []*html.Node
	attrs  blockAttrs
}

func (p *markdownPainter) paintFlow(n *html.Node, x, y float64, ctx markdownPaintContext, availableWidth float64) float64 {
	units := p.flowUnits(n, ctx)
	currentY := y
	previousMarginBottom := 0.0
	for i, unit := range units {
		if i == 0 && n.Data == "body" && unit.block != nil && (unit.block.Data == "ul" || unit.block.Data == "ol") {
			// The root has no parent flow to consume a collapsed first-child
			// margin. Chromium still paints that margin inside the foreignObject
			// viewport when a top-level loose list's first li starts with p,
			// while D2's legacy outer height intentionally excludes it.
			currentY += unit.attrs.marginTop
		} else if i > 0 {
			currentY += goMax(previousMarginBottom, unit.attrs.marginTop)
		}
		paintedHeight := unit.attrs.height
		if unit.block != nil {
			paintedHeight = p.paintBlock(unit.block, x, currentY, ctx, availableWidth)
		} else {
			paintedHeight = goMax(paintedHeight, p.paintInlineNodes(unit.inline, x, currentY, ctx, availableWidth))
		}
		currentY += paintedHeight
		previousMarginBottom = unit.attrs.marginBottom
	}
	return currentY - y
}

func (p *markdownPainter) flowUnits(n *html.Node, ctx markdownPaintContext) []markdownFlowUnit {
	var units []markdownFlowUnit
	var inlineNodes []*html.Node
	inlineAttrs := blockAttrs{}
	lineHeight := ctx.cssFontSize() * ctx.lineHeightFactor

	flushInline := func() {
		if len(inlineNodes) == 0 || !inlineAttrs.isNotEmpty() {
			inlineNodes = nil
			inlineAttrs = blockAttrs{}
			return
		}
		inlineAttrs.height = lineHeight
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
	breakLine            bool
	breakBefore          bool
	breakAfterSoftHyphen bool
	emergencyBreakBefore bool
	softHyphenEnd        bool
	discretionaryHyphen  bool
	padding              bool
	leadingPadding       bool
	text                 string
	width                float64
	height               float64
	ctx                  markdownPaintContext
	codeGroup            int
	codeHeight           float64
	decorationGroup      int
}

func (p *markdownPainter) paintInlineNodes(nodes []*html.Node, x, y float64, ctx markdownPaintContext, availableWidth float64) float64 {
	var tokens []markdownInlineToken
	nextCodeGroup := 0
	nextDecorationGroup := 0
	for _, n := range nodes {
		p.collectInlineTokens(n, ctx, &tokens, &nextCodeGroup, &nextDecorationGroup, 0, 0)
	}
	if len(tokens) == 0 {
		return 0
	}

	lines := p.wrapInlineTokens(tokens, availableWidth)
	paintedHeight := 0.0
	for _, line := range lines {
		lineHeight := p.markdownInlineLineHeight(line, ctx)
		lineY := y + paintedHeight
		lineX := x
		if ctx.textAlign != "" && availableWidth > 0 {
			lineWidth := 0.0
			for _, token := range line {
				lineWidth += token.width
			}
			extra := goMax(0, availableWidth-lineWidth)
			if ctx.textAlign == "center" {
				lineX += extra / 2
			} else if ctx.textAlign == "right" {
				lineX += extra
			}
		}
		cursorX := lineX
		codeSpans := make(map[int][3]float64)
		codeShifts := make(map[int]float64)
		codeLinks := make(map[int][2]string)
		decorationSpans := make(map[int]MarkdownPrimitive)
		for _, token := range line {
			if token.codeGroup > 0 {
				span, ok := codeSpans[token.codeGroup]
				if !ok {
					span = [3]float64{cursorX, cursorX, token.codeHeight}
					codeShifts[token.codeGroup] = p.inlineCodeVerticalShift(ctx, token.ctx, lineHeight, token.codeHeight)
					codeLinks[token.codeGroup] = [2]string{token.ctx.link, token.ctx.linkTitle}
				}
				span[1] = cursorX + token.width
				span[2] = goMax(span[2], token.codeHeight)
				codeSpans[token.codeGroup] = span
			}
			if token.decorationGroup > 0 {
				span, ok := decorationSpans[token.decorationGroup]
				if !ok {
					span = MarkdownPrimitive{
						Kind: MarkdownRectPrimitive, X: cursorX,
						Y: markdownSnap(lineY + lineHeight/2), Height: 1,
						FillRole: token.ctx.decorationRole,
					}
				}
				span.Width = cursorX + token.width - span.X
				decorationSpans[token.decorationGroup] = span
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
				Kind: MarkdownRectPrimitive, X: span[0], Y: lineY + (lineHeight-height)/2 + codeShifts[groupID],
				Width: span[1] - span[0], Height: height, Radius: 6, FillRole: MarkdownColorNeutralMuted,
				Link: codeLinks[groupID][0], LinkTitle: codeLinks[groupID][1],
			})
		}

		cursorX = lineX
		for _, token := range line {
			if token.padding {
				cursorX += token.width
				continue
			}
			fontSize := token.ctx.cssFontSize()
			fontScale := token.ctx.fontScale
			if token.ctx.inlineCode && !token.ctx.headingCode {
				fontScale *= FontSize_pre_code_em
				fontSize *= FontSize_pre_code_em
			}
			baselineY := p.textBaseline(token.ctx, lineY, lineHeight, fontScale)
			if token.ctx.inlineCode && token.codeHeight > 0 {
				codeTop := lineY + (lineHeight-token.codeHeight)/2
				verticalPadding := 0.0
				if !token.ctx.headingCode {
					verticalPadding = PaddingTopBottom_code_em * fontSize
				}
				contentTop := codeTop + verticalPadding
				contentHeight := goMax(0, token.codeHeight-2*verticalPadding)
				baselineY = p.textBaseline(token.ctx, contentTop, contentHeight, fontScale)
				baselineY += codeShifts[token.codeGroup]
			}
			baselineY -= markdownHeadingInkCorrection(token.ctx)
			p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
				Kind: MarkdownTextPrimitive, X: cursorX, Y: baselineY, Width: token.width, Height: token.height,
				Text: token.text, Font: token.ctx.fontRole(), FontSize: fontSize,
				FillRole: token.ctx.role, Link: token.ctx.link, LinkTitle: token.ctx.linkTitle,
				SyntheticBold: token.ctx.syntheticBold(), SyntheticItalic: token.ctx.syntheticItalic(),
				TextLength: token.discretionaryHyphen,
			})
			cursorX += token.width
		}
		decorationGroupIDs := make([]int, 0, len(decorationSpans))
		for groupID := range decorationSpans {
			decorationGroupIDs = append(decorationGroupIDs, groupID)
		}
		sort.Ints(decorationGroupIDs)
		for _, groupID := range decorationGroupIDs {
			p.layout.Primitives = append(p.layout.Primitives, decorationSpans[groupID])
		}
		paintedHeight += lineHeight
	}
	return paintedHeight
}

func (p *markdownPainter) markdownInlineLineHeight(line []markdownInlineToken, ctx markdownPaintContext) float64 {
	lineHeight := ctx.cssFontSize() * ctx.lineHeightFactor
	if !ctx.tableCell {
		return lineHeight
	}
	lineCtx := ctx
	hasContent := false
	allCode := true
	hasFallback := false
	hasSymbolFallback := false
	hasColorEmojiFallback := false
	for _, token := range line {
		if token.padding || token.text == "" {
			continue
		}
		hasContent = true
		if !token.ctx.inlineCode {
			fontFamily := token.ctx.fontFamily
			if token.ctx.mono {
				fontFamily = token.ctx.monoFontFamily
			}
			font := fontFamily.Font(token.ctx.fontSize, token.ctx.effectiveFontStyle())
			measurementText := strings.ReplaceAll(token.text, "\u00ad", "")
			if _, ok := p.ruler.measureFontAdvance(font, measurementText); !ok {
				graphemes := uniseg.NewGraphemes(measurementText)
				for graphemes.Next() {
					grapheme := graphemes.Str()
					if _, supported := p.ruler.measureFontAdvance(font, grapheme); supported {
						continue
					}
					colorEmoji := isColorEmojiGrapheme(grapheme)
					if !colorEmoji {
						filtered := stripDefaultIgnorableRunes(grapheme)
						if filtered != grapheme {
							if filtered == "" {
								continue
							}
							if _, supported := p.ruler.measureFontAdvance(font, filtered); supported {
								continue
							}
							grapheme = filtered
						}
					}
					hasFallback = true
					if colorEmoji {
						hasColorEmojiFallback = true
					} else if hasUnicodeSymbol(grapheme) {
						hasSymbolFallback = true
					}
				}
			}
			allCode = false
			continue
		}
		if lineCtx.inlineCode == false {
			lineCtx = token.ctx
		}
	}
	allCode = hasContent && allCode
	if !allCode {
		lineCtx = ctx
	}
	fontFamily := lineCtx.fontFamily
	if lineCtx.mono {
		fontFamily = lineCtx.monoFontFamily
	}
	fontSize := lineCtx.cssFontSize()
	if allCode {
		fontSize *= FontSize_pre_code_em
	}
	font := fontFamily.Font(lineCtx.fontSize, lineCtx.effectiveFontStyle())
	if normal, ok := p.ruler.cssNormalLineHeight(font, fontSize); ok {
		lineHeight = normal
	}
	if allCode {
		// Chromium's normal inline-formatting strut leaves two pixels of line
		// gap around Source Code Pro at the 85% code size.
		lineHeight = goMax(lineHeight, math.Round(fontSize*1.4))
	}
	if hasColorEmojiFallback && !allCode {
		// Chromium's color-emoji fallback uses a 26px normal line box at 16px.
		lineHeight = goMax(lineHeight, math.Round(ctx.cssFontSize()*1.625))
	} else if hasSymbolFallback && !allCode {
		// Text-presentation emoji-capable symbols use a 24px fallback line box.
		lineHeight = goMax(lineHeight, math.Round(ctx.cssFontSize()*1.5))
	} else if hasFallback && !allCode {
		// Host fallback fonts use a slightly taller normal line box for missing
		// proportional glyphs (22px versus Source Sans Pro's 20px at 16px).
		lineHeight = goMax(lineHeight, math.Round(ctx.cssFontSize()*1.375))
	}
	return lineHeight
}

// HTML line boxes at h2's 24px and h5's 14px font sizes expose ink one pixel
// above the baseline produced by rounded OpenType font-box metrics. This is a
// paint-only correction: list markers still align to the shared CSS baseline.
func markdownHeadingInkCorrection(ctx markdownPaintContext) float64 {
	if ctx.heading == "h5" || (ctx.heading == "h2" && *ctx.fontFamily == d2fonts.SourceSansPro) {
		return 1
	}
	return 0
}

// wrapInlineTokens implements CSS white-space wrapping at Unicode line-break
// opportunities, with overflow-wrap fallbacks at inline element boundaries.
// The outer viewport remains D2's legacy measured box: extra lines are painted
// at the browser line height and intentionally clip when historical measurement
// underestimated styled content.
func (p *markdownPainter) wrapInlineTokens(tokens []markdownInlineToken, availableWidth float64) [][]markdownInlineToken {
	lines := [][]markdownInlineToken{{}}
	lineWidth := 0.0
	needsWrap := false
	for _, token := range tokens {
		if token.breakLine {
			lines = append(lines, nil)
			lineWidth = 0
			continue
		}
		lines[len(lines)-1] = append(lines[len(lines)-1], token)
		lineWidth += token.width
		needsWrap = needsWrap || availableWidth > 0 && lineWidth > availableWidth+0.001
	}
	if !needsWrap {
		return lines
	}

	atoms := p.atomizeInlineTokens(tokens)

	lines = [][]markdownInlineToken{{}}
	lineWidth = 0
	var pendingSpace *markdownInlineToken
	var pendingPadding []markdownInlineToken
	pendingPaddingWidth := 0.0
	lineHasContent := false
	for atomIndex := 0; atomIndex < len(atoms); atomIndex++ {
		atom := atoms[atomIndex]
		if atom.breakLine {
			pendingSpace = nil
			pendingPadding = nil
			pendingPaddingWidth = 0
			lines = append(lines, nil)
			lineWidth = 0
			lineHasContent = false
			continue
		}
		if atom.text == " " && !atom.padding {
			copy := atom
			pendingSpace = &copy
			continue
		}
		if atom.padding && atom.leadingPadding {
			pendingPadding = append(pendingPadding, atom)
			pendingPaddingWidth += atom.width
			continue
		}

	retryAtom:
		trailingPaddingWidth := inlineTrailingPaddingWidth(atoms, atomIndex)
		needed := inlineUnbrokenWidth(atoms, atomIndex) + pendingPaddingWidth
		if pendingSpace != nil {
			needed += pendingSpace.width
		}
		canWrap := atom.breakBefore || pendingSpace != nil
		if availableWidth > 0 && lineHasContent && !atom.padding && canWrap && lineWidth+needed > availableWidth+0.001 {
			if atom.breakAfterSoftHyphen {
				line := lines[len(lines)-1]
				for i := len(line) - 1; i >= 0; i-- {
					if !line[i].softHyphenEnd {
						continue
					}
					line[i].softHyphenEnd = false
					hyphen := line[i]
					hyphen.text = "-"
					hyphen.width = hyphen.ctx.cssFontSize() / 3
					hyphen.softHyphenEnd = false
					hyphen.discretionaryHyphen = true
					hyphen.breakBefore = false
					hyphen.breakAfterSoftHyphen = false
					hyphen.emergencyBreakBefore = false
					line = append(line, markdownInlineToken{})
					copy(line[i+2:], line[i+1:])
					line[i+1] = hyphen
					lineWidth += hyphen.width
					lines[len(lines)-1] = line
					break
				}
			}
			lines = append(lines, nil)
			lineWidth = 0
			lineHasContent = false
			pendingSpace = nil
		}

		occupiedWidth := lineWidth + pendingPaddingWidth
		if pendingSpace != nil && lineHasContent {
			occupiedWidth += pendingSpace.width
		}
		if availableWidth > 0 && !atom.padding && occupiedWidth+atom.width+trailingPaddingWidth > availableWidth+0.001 {
			maxTextWidth := goMax(0, availableWidth-occupiedWidth)
			head, tail, ok := p.splitInlineTokenAtWidth(atom, maxTextWidth, trailingPaddingWidth > 0)
			if !ok && lineHasContent {
				// overflow-wrap: break-word falls back to the preceding grapheme
				// boundary when no normal Unicode opportunity is available.
				lines = append(lines, nil)
				lineWidth = 0
				lineHasContent = false
				pendingSpace = nil
				goto retryAtom
			}
			if ok {
				atom = head
				atoms = append(atoms, markdownInlineToken{})
				copy(atoms[atomIndex+2:], atoms[atomIndex+1:])
				atoms[atomIndex+1] = tail
			}
		}

		if pendingSpace != nil && lineHasContent {
			lines[len(lines)-1] = append(lines[len(lines)-1], *pendingSpace)
			lineWidth += pendingSpace.width
			pendingSpace = nil
		} else if pendingSpace != nil {
			// CSS discards leading collapsible whitespace, but inline-box
			// padding still belongs to the first painted fragment.
			pendingSpace = nil
		}
		lines[len(lines)-1] = append(lines[len(lines)-1], pendingPadding...)
		lineWidth += pendingPaddingWidth
		pendingPadding = nil
		pendingPaddingWidth = 0

		lines[len(lines)-1] = append(lines[len(lines)-1], atom)
		lineWidth += atom.width
		if !atom.padding {
			lineHasContent = true
		}
	}
	return lines
}

func inlineTrailingPaddingWidth(atoms []markdownInlineToken, atomIndex int) float64 {
	width := 0.0
	for i := atomIndex + 1; i < len(atoms) && atoms[i].padding && !atoms[i].leadingPadding; i++ {
		width += atoms[i].width
	}
	return width
}

// inlineUnbrokenWidth looks through style/link token boundaries until the next
// real Unicode break. CSS decides whether to move the whole word before laying
// out its individual styled runs; considering only the first run can leave half
// a word on the preceding line.
func inlineUnbrokenWidth(atoms []markdownInlineToken, atomIndex int) float64 {
	width := 0.0
	for i := atomIndex; i < len(atoms); i++ {
		atom := atoms[i]
		if atom.breakLine || (i > atomIndex && !atom.padding && (atom.breakBefore || atom.text == " ")) {
			break
		}
		if i == atomIndex && atom.text == " " && !atom.padding {
			break
		}
		width += atom.width
	}
	return width
}

func (p *markdownPainter) splitInlineTokenAtWidth(token markdownInlineToken, maxWidth float64, forceSplit bool) (head, tail markdownInlineToken, ok bool) {
	graphemes := uniseg.NewGraphemes(token.text)
	var boundaries []int
	for graphemes.Next() {
		_, end := graphemes.Positions()
		boundaries = append(boundaries, end)
	}
	if len(boundaries) < 2 {
		return markdownInlineToken{}, markdownInlineToken{}, false
	}

	best := -1
	for i, boundary := range boundaries {
		width := p.inlineTextAdvance(token.text[:boundary], token.ctx, token.width)
		if width > maxWidth+0.001 {
			break
		}
		best = i
	}
	if best < 0 {
		return markdownInlineToken{}, markdownInlineToken{}, false
	}
	if best == len(boundaries)-1 {
		if !forceSplit {
			return markdownInlineToken{}, markdownInlineToken{}, false
		}
		best--
		if best < 0 {
			return markdownInlineToken{}, markdownInlineToken{}, false
		}
	}

	boundary := boundaries[best]
	head = token
	head.text = token.text[:boundary]
	head.width = p.inlineTextAdvance(head.text, head.ctx, token.width)
	head.softHyphenEnd = false

	tail = token
	tail.text = token.text[boundary:]
	tail.width = p.inlineTextAdvance(tail.text, tail.ctx, token.width)
	tail.breakBefore = false
	tail.breakAfterSoftHyphen = false
	tail.emergencyBreakBefore = true
	return head, tail, true
}

// atomizeInlineTokens splits text only at Unicode line-break opportunities.
// Computing boundaries across adjacent styled runs is important: an element
// boundary alone is not a valid place to wrap a Latin word. The split tokens
// retain their original paint context, code group, and decoration group.
func (p *markdownPainter) atomizeInlineTokens(tokens []markdownInlineToken) []markdownInlineToken {
	var atoms []markdownInlineToken
	for start := 0; start < len(tokens); {
		if tokens[start].breakLine {
			atoms = append(atoms, tokens[start])
			start++
			continue
		}

		end := start
		var combined strings.Builder
		for end < len(tokens) && !tokens[end].breakLine {
			if !tokens[end].padding {
				combined.WriteString(tokens[end].text)
			}
			end++
		}

		breakOffsets := make(map[int]bool)
		combinedText := combined.String()
		// Unicode line-breaking and grapheme segmentation are related but
		// distinct algorithms. Never accept a line-break offset that falls
		// inside an extended grapheme cluster (notably emoji ZWJ sequences),
		// because SVG text would otherwise paint the pieces as separate glyphs.
		graphemeBoundaries := map[int]bool{0: true}
		graphemes := uniseg.NewGraphemes(combinedText)
		graphemeOffset := 0
		for graphemes.Next() {
			graphemeOffset += len(graphemes.Str())
			graphemeBoundaries[graphemeOffset] = true
		}
		remaining := combinedText
		state := -1
		consumed := 0
		for remaining != "" {
			segment, rest, _, nextState := uniseg.FirstLineSegmentInString(remaining, state)
			if segment == "" {
				break
			}
			consumed += len(segment)
			if rest != "" && graphemeBoundaries[consumed] {
				breakOffsets[consumed] = true
			}
			remaining = rest
			state = nextState
		}

		textOffset := 0
		for _, token := range tokens[start:end] {
			if token.padding || token.text == "" {
				atoms = append(atoms, token)
				continue
			}

			tokenStart := textOffset
			tokenEnd := tokenStart + len(token.text)
			pieceStart := tokenStart
			for offset := tokenStart + 1; offset <= tokenEnd; offset++ {
				if offset != tokenEnd && !breakOffsets[offset] {
					continue
				}
				piece := token.text[pieceStart-tokenStart : offset-tokenStart]
				atom := token
				atom.breakBefore = breakOffsets[pieceStart]
				atom.breakAfterSoftHyphen = atom.breakBefore && strings.HasSuffix(combinedText[:pieceStart], "\u00ad")
				atom.emergencyBreakBefore = pieceStart == tokenStart && tokenStart > 0
				if strings.HasSuffix(piece, "\u00ad") {
					piece = strings.TrimSuffix(piece, "\u00ad")
					atom.softHyphenEnd = true
				}
				p.appendInlineTextAtoms(&atoms, atom, piece)
				pieceStart = offset
			}
			textOffset = tokenEnd
		}
		start = end
	}
	return atoms
}

func (p *markdownPainter) appendInlineTextAtoms(atoms *[]markdownInlineToken, token markdownInlineToken, text string) {
	if text == "" && token.softHyphenEnd {
		token.width = 0
		*atoms = append(*atoms, token)
		return
	}
	content := strings.TrimRight(text, " ")
	if content != "" {
		atom := token
		atom.text = content
		atom.width = p.inlineTextAdvance(content, token.ctx, token.width)
		*atoms = append(*atoms, atom)
		token.breakBefore = false
	}
	for range len(text) - len(content) {
		atom := token
		atom.text = " "
		atom.width = p.inlineTextAdvance(" ", token.ctx, token.width)
		*atoms = append(*atoms, atom)
		token.breakBefore = false
	}
}

func (p *markdownPainter) inlineCodeVerticalShift(parentCtx, codeCtx markdownPaintContext, lineHeight, codeHeight float64) float64 {
	if parentCtx.tableCell {
		return 0
	}
	codeScale := codeCtx.fontScale
	codeFontSize := codeCtx.cssFontSize()
	verticalPadding := 0.0
	if !codeCtx.headingCode {
		codeScale *= FontSize_pre_code_em
		codeFontSize *= FontSize_pre_code_em
		verticalPadding = PaddingTopBottom_code_em * codeFontSize
	}
	codeTop := (lineHeight-codeHeight)/2 + verticalPadding
	codeContentHeight := goMax(0, codeHeight-2*verticalPadding)
	codeBaseline := p.textBaseline(codeCtx, codeTop, codeContentHeight, codeScale)
	parentBaseline := p.textBaseline(parentCtx, 0, lineHeight, parentCtx.fontScale)
	return parentBaseline - codeBaseline
}

func (p *markdownPainter) collectInlineTokens(n *html.Node, ctx markdownPaintContext, tokens *[]markdownInlineToken, nextCodeGroup, nextDecorationGroup *int, codeGroup int, codeHeight float64) {
	if n.Type == html.TextNode {
		text := renderableMarkdownText(n)
		attrs := p.measure(n, ctx)
		if text == "" && attrs.width == 0 {
			return
		}
		width := p.inlineTextAdvance(text, ctx, attrs.width)
		*tokens = append(*tokens, markdownInlineToken{
			text: text, width: width, height: attrs.height, ctx: ctx,
			codeGroup: codeGroup, codeHeight: codeHeight, decorationGroup: ctx.decorationGroup,
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
		return
	}

	childCtx := p.childContext(n, ctx)
	if childCtx.decoration != MarkdownTextDecorationNone && ctx.decoration == MarkdownTextDecorationNone {
		*nextDecorationGroup++
		childCtx.decorationGroup = *nextDecorationGroup
		childCtx.decorationRole = childCtx.role
	}
	if n.Data == "code" && (n.Parent == nil || n.Parent.Data != "pre") {
		*nextCodeGroup++
		codeGroup = *nextCodeGroup
		attrs := p.measure(n, ctx)
		codeHeight = attrs.height
		padding := inlineCodeHorizontalPadding(childCtx)
		*tokens = append(*tokens, markdownInlineToken{
			padding: true, leadingPadding: true, width: padding, ctx: childCtx, codeGroup: codeGroup, codeHeight: codeHeight, decorationGroup: childCtx.decorationGroup,
		})
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		p.collectInlineTokens(child, childCtx, tokens, nextCodeGroup, nextDecorationGroup, codeGroup, codeHeight)
	}
	if n.Data == "code" && (n.Parent == nil || n.Parent.Data != "pre") {
		padding := inlineCodeHorizontalPadding(childCtx)
		*tokens = append(*tokens, markdownInlineToken{
			padding: true, width: padding, ctx: childCtx, codeGroup: codeGroup, codeHeight: codeHeight, decorationGroup: childCtx.decorationGroup,
		})
	}
}

func inlineCodeHorizontalPadding(ctx markdownPaintContext) float64 {
	if ctx.headingCode {
		return PaddingLeftRight_heading_code_em * ctx.cssFontSize()
	}
	return PaddingLeftRight_code_em * FontSize_pre_code_em * ctx.cssFontSize()
}

func (p *markdownPainter) inlineTextAdvance(text string, ctx markdownPaintContext, fallback float64) float64 {
	fontFamily := ctx.fontFamily
	if ctx.mono {
		fontFamily = ctx.monoFontFamily
	}
	font := fontFamily.Font(ctx.fontSize, ctx.effectiveFontStyle())
	measurementText := strings.ReplaceAll(text, "\u00ad", "")
	advance, ok := p.ruler.measureFontAdvance(font, measurementText)
	if !ok {
		advance, _ = p.ruler.MeasurePrecise(font, measurementText)
		advance = p.ruler.scaleUnicodeCSS(advance, font, measurementText)
		if advance == 0 && measurementText != "" && !isZeroWidthGrapheme(measurementText) {
			return fallback
		}
	}
	advance *= ctx.fontScale
	if ctx.inlineCode && !ctx.headingCode {
		advance *= FontSize_pre_code_em
	}
	return advance
}

func renderableMarkdownText(n *html.Node) string {
	return n.Data
}

func (p *markdownPainter) paintPre(n *html.Node, x, y float64, attrs blockAttrs, ctx markdownPaintContext, availableWidth float64) float64 {
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
	fontSize := FontSize_pre_code_em * ctx.cssFontSize()
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
			Y:    p.textBaseline(ctx, y+Padding_pre+float64(i)*lineHeight, lineHeight, FontSize_pre_code_em*ctx.fontScale),
			Text: line, Font: ctx.fontRole(), FontSize: fontSize, FillRole: ctx.role,
			SyntheticBold: ctx.syntheticBold(), SyntheticItalic: ctx.syntheticItalic(),
		})
	}
	return backgroundHeight
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
	node    *html.Node
	cells   []*html.Node
	header  bool
	height  float64
	widths  []float64
	heights []float64
	stripe  bool
}

func (p *markdownPainter) paintTable(n *html.Node, x, y float64, attrs blockAttrs, ctx markdownPaintContext, availableWidth float64) float64 {
	rows := collectMarkdownTableRows(n)
	columnWidths := make([]float64, 0)
	columnMinWidths := make([]float64, 0)
	for i := range rows {
		rowMinWidths := make([]float64, 0, len(rows[i].cells))
		for _, cell := range rows[i].cells {
			cellCtx := markdownTableCellContext(ctx, rows[i], cell)
			cellAttrs := p.measure(cell, cellCtx)
			rows[i].widths = append(rows[i].widths, cellAttrs.width+26)
			rowMinWidths = append(rowMinWidths, p.markdownMinContentWidth(cell, cellCtx)+26)
		}
		columnWidths = mergeColumnWidths(columnWidths, [][]float64{rows[i].widths})
		columnMinWidths = mergeColumnWidths(columnMinWidths, [][]float64{rowMinWidths})
	}
	tableWidth := attrs.width
	if availableWidth > 0 {
		tableWidth = math.Min(tableWidth, availableWidth)
	}
	tableWidth = goMax(1, tableWidth)
	columnWidths = fitMarkdownTableColumns(columnWidths, columnMinWidths, tableWidth)
	paintedTableWidth := float64(len(columnWidths) + 1)
	for _, width := range columnWidths {
		paintedTableWidth += width
	}
	paintedTableWidth = goMax(tableWidth, paintedTableWidth)

	for i := range rows {
		maxCellHeight := 0.0
		for cellIndex, cell := range rows[i].cells {
			cellCtx := markdownTableCellContext(ctx, rows[i], cell)
			contentWidth := goMax(0, columnWidths[cellIndex]-26)
			probe := markdownPainter{ruler: p.ruler, layout: &MarkdownLayout{}}
			contentHeight := probe.paintFlow(cell, 0, 0, cellCtx, contentWidth)
			rows[i].heights = append(rows[i].heights, contentHeight)
			maxCellHeight = goMax(maxCellHeight, contentHeight+12)
		}
		// Empty cells have only their 6px vertical padding and the collapsed
		// one-pixel row border. Chromium therefore paints an empty row at 13px.
		rows[i].height = maxCellHeight + 1
	}
	finalBoundaryY := y
	for _, row := range rows {
		finalBoundaryY += row.height
	}
	leftBorderX := markdownSnap(x)
	topBorderY := markdownSnap(y)
	rightBorderX := markdownSnap(x + paintedTableWidth - 1)
	bottomBorderY := markdownSnap(finalBoundaryY)
	gridWidth := rightBorderX - leftBorderX + 1
	gridHeight := bottomBorderY - topBorderY + 1

	p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
		Kind: MarkdownRectPrimitive, X: leftBorderX, Y: topBorderY, Width: gridWidth, Height: gridHeight,
		FillRole: MarkdownColorCanvas,
	})
	stripeY := y
	for _, row := range rows {
		if row.stripe {
			stripeTop := markdownSnap(stripeY)
			stripeBottom := markdownSnap(stripeY + row.height)
			p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
				Kind: MarkdownRectPrimitive, X: leftBorderX + 1, Y: stripeTop, Width: goMax(0, rightBorderX-leftBorderX-1), Height: stripeBottom - stripeTop,
				FillRole: MarkdownColorCanvasSubtle,
			})
		}
		stripeY += row.height
	}
	grid := []MarkdownPrimitive{
		{
			Kind: MarkdownRectPrimitive, X: leftBorderX, Y: topBorderY, Width: gridWidth, Height: 1,
			FillRole: MarkdownColorBorder,
		},
		{
			Kind: MarkdownRectPrimitive, X: leftBorderX, Y: topBorderY, Width: 1, Height: gridHeight,
			FillRole: MarkdownColorBorder,
		},
		{
			Kind: MarkdownRectPrimitive, X: rightBorderX, Y: topBorderY, Width: 1, Height: gridHeight,
			FillRole: MarkdownColorBorder,
		},
	}
	rowY := y
	for _, row := range rows {
		cursorX := x + 1
		for cellIndex, cell := range row.cells {
			cellCtx := markdownTableCellContext(ctx, row, cell)
			contentX := cursorX + 13
			available := columnWidths[cellIndex] - 26
			contentHeight := 0.0
			if cellIndex < len(row.heights) {
				contentHeight = row.heights[cellIndex]
			}
			contentY := rowY + 7 + goMax(0, row.height-13-contentHeight)/2
			// The collapsed top border occupies the first CSS pixel of each row;
			// cell padding starts after it. Table cells vertically center content
			// when another cell makes the shared row taller.
			p.paintFlow(cell, contentX, contentY, cellCtx, available)
			cursorX += columnWidths[cellIndex]
			if cellIndex < len(row.cells)-1 {
				segmentTop := markdownSnap(rowY)
				segmentBottom := markdownSnap(rowY + row.height)
				grid = append(grid, MarkdownPrimitive{
					Kind: MarkdownRectPrimitive, X: markdownSnap(cursorX), Y: segmentTop, Width: 1, Height: segmentBottom - segmentTop,
					FillRole: MarkdownColorBorder,
				})
			}
			cursorX++
		}
		rowY += row.height
		grid = append(grid, MarkdownPrimitive{
			Kind: MarkdownRectPrimitive, X: leftBorderX, Y: markdownSnap(rowY), Width: gridWidth, Height: 1,
			FillRole: MarkdownColorBorder,
		})
	}
	p.layout.Primitives = append(p.layout.Primitives, grid...)
	return gridHeight
}

// markdownMinContentWidth returns the narrowest width allowed by ordinary CSS
// line-break opportunities. In particular, overflow-wrap: break-word does not
// reduce an element's min-content contribution, while spaces and CJK boundaries
// do. Inline padding stays attached to the first or last fragment it surrounds.
func (p *markdownPainter) markdownMinContentWidth(n *html.Node, ctx markdownPaintContext) float64 {
	minWidth := 0.0
	for _, unit := range p.flowUnits(n, ctx) {
		if unit.block != nil {
			minWidth = goMax(minWidth, p.measure(unit.block, ctx).width)
			continue
		}
		var tokens []markdownInlineToken
		nextCodeGroup := 0
		nextDecorationGroup := 0
		for _, inline := range unit.inline {
			p.collectInlineTokens(inline, ctx, &tokens, &nextCodeGroup, &nextDecorationGroup, 0, 0)
		}
		atoms := p.atomizeInlineTokens(tokens)
		segmentWidth := 0.0
		pendingLeadingPadding := 0.0
		var previousContentCtx *markdownPaintContext
		commit := func(extra float64) {
			minWidth = goMax(minWidth, segmentWidth+extra)
			segmentWidth = 0
			previousContentCtx = nil
		}
		for _, atom := range atoms {
			if atom.breakLine {
				segmentWidth += pendingLeadingPadding
				pendingLeadingPadding = 0
				commit(0)
				continue
			}
			if atom.text == " " && !atom.padding {
				segmentWidth += pendingLeadingPadding
				pendingLeadingPadding = 0
				commit(0)
				continue
			}
			if atom.padding && atom.leadingPadding {
				pendingLeadingPadding += atom.width
				continue
			}
			if !atom.padding && atom.breakBefore {
				extra := 0.0
				if atom.breakAfterSoftHyphen && previousContentCtx != nil {
					extra = previousContentCtx.cssFontSize() / 3
				}
				commit(extra)
			}
			segmentWidth += pendingLeadingPadding
			pendingLeadingPadding = 0
			segmentWidth += atom.width
			if !atom.padding {
				copy := atom.ctx
				previousContentCtx = &copy
			}
		}
		segmentWidth += pendingLeadingPadding
		commit(0)
	}
	return minWidth
}

func markdownTableCellContext(ctx markdownPaintContext, row markdownTableRow, cell *html.Node) markdownPaintContext {
	ctx.tableCell = true
	// The browser UA stylesheet resets tables to line-height: normal. Source
	// Sans Pro's normal 16px line box is 20px rather than Markdown's 24px.
	ctx.lineHeightFactor = 1.25
	if row.header || cell.Data == "th" {
		ctx.fontStyle = d2fonts.FONT_STYLE_SEMIBOLD
		ctx.bold = true
	}
	ctx.textAlign = nodeAttr(cell, "align")
	if ctx.textAlign == "" && (row.header || cell.Data == "th") {
		ctx.textAlign = "center"
	}
	return ctx
}

func fitMarkdownTableColumns(widths, minWidths []float64, tableWidth float64) []float64 {
	if len(widths) == 0 {
		return widths
	}
	fitted := append([]float64(nil), widths...)
	columnBudget := goMax(0, tableWidth-float64(len(widths)+1))
	naturalWidth := 0.0
	for _, width := range widths {
		naturalWidth += width
	}
	if naturalWidth <= columnBudget+0.001 {
		return fitted
	}
	minWidth := 0.0
	shrinkCapacity := 0.0
	for i, width := range widths {
		minimum := 0.0
		if i < len(minWidths) {
			minimum = math.Min(width, minWidths[i])
		}
		minWidth += minimum
		shrinkCapacity += width - minimum
	}
	if minWidth <= columnBudget+0.001 && shrinkCapacity > 0 {
		shrink := naturalWidth - columnBudget
		for i, width := range widths {
			minimum := 0.0
			if i < len(minWidths) {
				minimum = math.Min(width, minWidths[i])
			}
			capacity := width - minimum
			fitted[i] = width - shrink*capacity/shrinkCapacity
		}
		return fitted
	}
	if minWidth > columnBudget+0.001 {
		// CSS automatic table layout does not compress columns below their
		// min-content widths. The row can overflow a max-width-constrained table.
		for i, width := range widths {
			if i < len(minWidths) {
				fitted[i] = math.Min(width, minWidths[i])
			}
		}
		return fitted
	}
	paddingWidth := 26.0 * float64(len(widths))
	if columnBudget <= paddingWidth {
		for i := range fitted {
			fitted[i] = columnBudget / float64(len(fitted))
		}
		return fitted
	}
	contentBudget := columnBudget - paddingWidth
	naturalContentWidth := 0.0
	for _, width := range widths {
		naturalContentWidth += goMax(0, width-26)
	}
	if naturalContentWidth == 0 {
		for i := range fitted {
			fitted[i] = 26 + contentBudget/float64(len(fitted))
		}
		return fitted
	}
	for i, width := range widths {
		share := goMax(0, width-26) / naturalContentWidth
		fitted[i] = 26 + contentBudget*share
	}
	return fitted
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
	if n.Parent.Data == "ul" {
		// Browser list markers are geometric shapes, not font glyphs. They are
		// painted by paintListMarker and deliberately excluded from font corpus.
		return ""
	}
	marker := ""
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

func (p *markdownPainter) listIndent(n *html.Node, ctx markdownPaintContext) float64 {
	return PaddingLeft_ul_ol_em * ctx.cssFontSize()
}

func (p *markdownPainter) paintListMarker(n *html.Node, x, y float64, ctx markdownPaintContext, inset float64) {
	baselineY := p.listMarkerBaseline(n, y, ctx)
	if n.Parent != nil && n.Parent.Type == html.ElementNode && n.Parent.Data == "ul" {
		fontSize := ctx.cssFontSize()
		// Blink paints list markers as pixel-snapped geometry. Its disc/square
		// box is about .3em, with a three-pixel minimum at small font sizes.
		side := goMax(3, markdownSnap(0.3*fontSize))
		// CSS places outside markers in a marker box whose center sits about
		// one em before the list content. The 1.5px correction comes from the
		// browser marker box; another 5px aligns the geometric marker with that
		// box instead of the text-glyph origin used by the first native version.
		centerX := x + inset - fontSize/2 - 6.5
		primitive := MarkdownPrimitive{
			Kind: MarkdownRectPrimitive,
			X:    markdownSnap(centerX - side/2), Y: baselineY - math.Floor(1.5*side),
			Width: side, Height: side,
		}
		depth := markdownListDepth(n)
		switch depth {
		case 0:
			primitive.Radius = side / 2
			primitive.FillRole = ctx.role
		case 1:
			primitive.Radius = side / 2
			primitive.StrokeWidth = 1
			if ctx.role == MarkdownColorMuted {
				primitive.StrokeRole = MarkdownColorMutedStroke
			} else {
				primitive.StrokeRole = MarkdownColorForegroundStroke
			}
		default:
			primitive.FillRole = ctx.role
		}
		p.layout.Primitives = append(p.layout.Primitives, primitive)
		return
	}
	marker := markdownListMarker(n)
	if marker == "" {
		return
	}
	fontFamily := ctx.fontFamily
	if ctx.mono {
		fontFamily = ctx.monoFontFamily
	}
	font := fontFamily.Font(ctx.fontSize, ctx.effectiveFontStyle())
	markerWidth, markerHeight := p.ruler.MeasurePrecise(font, marker)
	markerWidth *= ctx.fontScale
	markerHeight *= ctx.fontScale
	p.layout.Primitives = append(p.layout.Primitives, MarkdownPrimitive{
		Kind: MarkdownTextPrimitive,
		X:    x + inset - markerWidth - p.ruler.spaceWidth(font)*ctx.fontScale,
		Y:    baselineY, Width: markerWidth, Height: markerHeight,
		Text: marker, Font: ctx.fontRole(), FontSize: ctx.cssFontSize(),
		FillRole: ctx.role, SyntheticBold: ctx.syntheticBold(), SyntheticItalic: ctx.syntheticItalic(),
	})
}

// A list marker participates in the list item's first line box. When that line
// is introduced by a heading, its larger baseline pulls the marker down, but
// the marker itself still inherits the list item's (body) font size.
func (p *markdownPainter) listMarkerBaseline(n *html.Node, y float64, ctx markdownPaintContext) float64 {
	baseline := p.textBaseline(ctx, y, ctx.cssFontSize()*ctx.lineHeightFactor, ctx.fontScale)
	first := getNext(n.FirstChild)
	if first == nil || first.Type != html.ElementNode || !isMarkdownHeading(first.Data) {
		return baseline
	}
	headingCtx := p.childContext(first, ctx)
	headingBaseline := p.textBaseline(
		headingCtx,
		y,
		headingCtx.cssFontSize()*headingCtx.lineHeightFactor,
		headingCtx.fontScale,
	)
	return goMax(baseline, headingBaseline)
}

func markdownListDepth(n *html.Node) int {
	depth := 0
	if n == nil || n.Parent == nil {
		return depth
	}
	for ancestor := n.Parent.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type == html.ElementNode && (ancestor.Data == "ol" || ancestor.Data == "ul") {
			depth++
		}
	}
	return depth
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
