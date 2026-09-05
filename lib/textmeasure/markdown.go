package textmeasure

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"

	"github.com/d2lang/util-go/go2"

	"github.com/d2lang/d2/d2renderers/d2fonts"
)

var markdownRenderer goldmark.Markdown

// these are css values from github-markdown.css so we can accurately compute the rendered dimensions
const (
	MarkdownFontSize   = d2fonts.FONT_SIZE_M
	MarkdownLineHeight = 1.5

	PaddingLeft_ul_ol_em = 2.
	MarginBottom_ul      = 16.

	MarginTop_li_p  = 16.
	MarginTop_li_em = 0.25
	MarginBottom_p  = 16.

	LineHeight_h           = 1.25
	MarginTop_h            = 24
	MarginBottom_h         = 16
	PaddingBottom_h1_h2_em = 0.3
	BorderBottom_h1_h2     = 1

	Height_hr_em       = 0.25
	MarginTopBottom_hr = 24

	Padding_pre          = 16
	MarginBottom_pre     = 16
	MarginBottom_table   = 16
	LineHeight_pre       = 1.45
	FontSize_pre_code_em = 0.85

	PaddingTopBottom_code_em         = 0.2
	PaddingLeftRight_code_em         = 0.4
	PaddingLeftRight_heading_code_em = 0.2

	PaddingLR_blockquote_em  = 1.
	MarginBottom_blockquote  = 16
	BorderLeft_blockquote_em = 0.25

	h1_em = 2.
	h2_em = 1.5
	h3_em = 1.25
	h4_em = 1.
	h5_em = 0.875
	h6_em = 0.85
)

func HeaderToFontSize(baseFontSize int, header string) int {
	if !isMarkdownHeading(header) {
		return 0
	}
	return int(HeaderToFontScale(header) * float64(baseFontSize))
}

// HeaderToFontScale returns github-markdown.css's exact heading size. The
// legacy measurement path keeps HeaderToFontSize's integer truncation so graph
// dimensions remain stable, while native SVG painting uses this scale and can
// reproduce fractional CSS sizes such as h6's 13.6px at a 16px base.
func HeaderToFontScale(header string) float64 {
	switch header {
	case "h1":
		return h1_em
	case "h2":
		return h2_em
	case "h3":
		return h3_em
	case "h4":
		return h4_em
	case "h5":
		return h5_em
	case "h6":
		return h6_em
	default:
		return 1
	}
}

func isMarkdownHeading(element string) bool {
	switch element {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	default:
		return false
	}
}

func hasMarkdownHeadingAncestor(n *html.Node) bool {
	for ancestor := n.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Type == html.ElementNode && isMarkdownHeading(ancestor.Data) {
			return true
		}
	}
	return false
}

func RenderMarkdown(m string) (string, error) {
	var output bytes.Buffer
	if err := markdownRenderer.Convert([]byte(m), &output); err != nil {
		return "", err
	}
	sanitized, err := sanitizeLinks(output.String())
	if err != nil {
		return "", err
	}
	return sanitized, nil
}

func init() {
	markdownRenderer = goldmark.New(
		goldmark.WithRendererOptions(
			goldmarkHtml.WithUnsafe(),
			goldmarkHtml.WithXHTML(),
		),
		goldmark.WithExtensions(
			extension.Strikethrough,
			extension.Table,
		),
	)
}

func MeasureMarkdown(mdText string, ruler *Ruler, fontFamily *d2fonts.FontFamily, monoFontFamily *d2fonts.FontFamily, fontSize int) (width, height int, err error) {
	layout, err := LayoutMarkdown(mdText, ruler, fontFamily, monoFontFamily, fontSize)
	if err != nil {
		return 0, 0, err
	}
	return layout.Width, layout.Height, nil
}

func hasPrev(n *html.Node) bool {
	if n.PrevSibling == nil {
		return false
	}
	if strings.TrimSpace(n.PrevSibling.Data) == "" {
		return hasPrev(n.PrevSibling)
	}
	return true
}

func hasNext(n *html.Node) bool {
	if n.NextSibling == nil {
		return false
	}
	// skip over empty text nodes
	if strings.TrimSpace(n.NextSibling.Data) == "" {
		return hasNext(n.NextSibling)
	}
	return true
}

func getPrev(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if strings.TrimSpace(n.Data) == "" {
		if next := getNext(n.PrevSibling); next != nil {
			return next
		}
	}
	return n
}

func getNext(n *html.Node) *html.Node {
	if n == nil {
		return nil
	}
	if strings.TrimSpace(n.Data) == "" {
		if next := getNext(n.NextSibling); next != nil {
			return next
		}
	}
	return n
}

func isBlockElement(elType string) bool {
	switch elType {
	case "blockquote",
		"div",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"hr",
		"li",
		"ol",
		"p",
		"pre",
		"ul",
		"table", "thead", "tbody", "tr", "td", "th": // Added table elements here
		return true
	default:
		return false
	}
}

func hasAncestorElement(n *html.Node, elType string) bool {
	if n.Parent == nil {
		return false
	}
	if n.Parent.Type == html.ElementNode && n.Parent.Data == elType {
		return true
	}
	return hasAncestorElement(n.Parent, elType)
}

type blockAttrs struct {
	width, height, marginTop, marginBottom float64
	extraData                              interface{}
}

func (b *blockAttrs) isNotEmpty() bool {
	return b != nil && *b != blockAttrs{}
}

func isCollapsibleMarkdownWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}

// normalizeMarkdownWhitespaceTree applies CSS white-space: normal across
// inline element boundaries. A per-text-node pass is insufficient: HTML such
// as a<a> b</a> must preserve one space, while whitespace next to a block or
// explicit line break must disappear. Preformatted code is the sole Markdown
// context that retains source whitespace.
func normalizeMarkdownWhitespaceTree(root *html.Node) {
	type whitespaceState struct {
		atLineStart bool
		pending     *strings.Builder
	}

	outputs := make(map[*html.Node]*strings.Builder)
	outputFor := func(n *html.Node) *strings.Builder {
		if output := outputs[n]; output != nil {
			return output
		}
		output := &strings.Builder{}
		output.Grow(len(n.Data))
		outputs[n] = output
		return output
	}
	flushPending := func(state *whitespaceState) {
		if state.pending != nil && !state.atLineStart {
			state.pending.WriteByte(' ')
		}
		state.pending = nil
	}

	var normalizeBlock func(*html.Node)
	var walkInline func(*html.Node, *whitespaceState)
	walkInline = func(n *html.Node, state *whitespaceState) {
		switch n.Type {
		case html.TextNode:
			output := outputFor(n)
			for _, r := range n.Data {
				if isCollapsibleMarkdownWhitespace(r) {
					if state.pending == nil {
						state.pending = output
					}
					continue
				}
				flushPending(state)
				output.WriteRune(r)
				state.atLineStart = false
			}
		case html.ElementNode:
			switch {
			case n.Data == "br":
				state.pending = nil
				state.atLineStart = true
			case isBlockElement(n.Data):
				state.pending = nil
				normalizeBlock(n)
				state.atLineStart = true
			case n.Data == "img":
				// Markdown images are outside native SVG's supported subset.
				// Ignore the node and let whitespace collapse across it.
			default:
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					walkInline(child, state)
				}
			}
		}
	}
	normalizeBlock = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "pre" {
			return
		}
		state := whitespaceState{atLineStart: true}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walkInline(child, &state)
		}
		// A pending space at the end of a block or line is not painted.
		state.pending = nil
	}

	normalizeBlock(root)
	for node, output := range outputs {
		node.Data = output.String()
	}
}

// measures node dimensions to match rendering with styles in github-markdown.css
func (ruler *Ruler) measureNode(depth int, n *html.Node, fontFamily *d2fonts.FontFamily, monoFontFamily *d2fonts.FontFamily, fontSize int, fontStyle d2fonts.FontStyle, bold, italic bool) blockAttrs {
	return ruler.measureMarkdownNode(depth, n, fontFamily, monoFontFamily, fontSize, 1, fontStyle, bold, italic, false)
}

// measureCSSNode uses CSS line boxes and fractional heading scales for native
// SVG painting. measureNode above intentionally retains D2's historical box
// dimensions, which are part of graph layout compatibility.
func (ruler *Ruler) measureCSSNode(depth int, n *html.Node, fontFamily *d2fonts.FontFamily, monoFontFamily *d2fonts.FontFamily, fontSize int, fontScale float64, fontStyle d2fonts.FontStyle, bold, italic bool) blockAttrs {
	return ruler.measureMarkdownNode(depth, n, fontFamily, monoFontFamily, fontSize, fontScale, fontStyle, bold, italic, true)
}

func (ruler *Ruler) measureMarkdownNode(depth int, n *html.Node, fontFamily *d2fonts.FontFamily, monoFontFamily *d2fonts.FontFamily, fontSize int, fontScale float64, fontStyle d2fonts.FontStyle, bold, italic, cssLineBoxes bool) blockAttrs {
	if fontFamily == nil {
		fontFamily = go2.Pointer(d2fonts.SourceSansPro)
	}
	font := fontFamily.Font(fontSize, fontStyle)

	var parentElementType string
	if n.Parent != nil && n.Parent.Type == html.ElementNode {
		parentElementType = n.Parent.Data
	}

	debugMeasure := false
	var depthStr string
	if debugMeasure {
		if depth == 0 {
			fmt.Println()
		}
		depthStr = "┌"
		for i := 0; i < depth; i++ {
			depthStr += "-"
		}
	}

	switch n.Type {
	case html.TextNode:
		if !cssLineBoxes && strings.Trim(n.Data, "\n\t\b") == "" {
			return blockAttrs{}
		}
		str := n.Data
		isCode := parentElementType == "pre" || parentElementType == "code"
		isHeadingCode := parentElementType == "code" && hasMarkdownHeadingAncestor(n)
		spaceWidths := 0.

		if !isCode {
			spaceWidth := ruler.spaceWidth(font) * fontScale
			if cssLineBoxes {
				if advance, ok := ruler.measureFontAdvance(font, " "); ok {
					spaceWidth = advance * fontScale
				}
			}
			if cssLineBoxes {
				str = renderableMarkdownText(n)
				if str == "" {
					return blockAttrs{}
				}
				// MeasurePrecise omits edge whitespace, so account for it.
				if strings.HasPrefix(str, " ") {
					str = strings.TrimPrefix(str, " ")
					spaceWidths += spaceWidth
				}
				if strings.HasSuffix(str, " ") {
					str = strings.TrimSuffix(str, " ")
					spaceWidths += spaceWidth
				}
			} else {
				str = strings.ReplaceAll(str, "\n", " ")
				str = strings.ReplaceAll(str, "\t", " ")
				if strings.HasPrefix(str, " ") {
					str = strings.TrimPrefix(str, " ")
					if hasPrev(n) {
						spaceWidths += spaceWidth
					}
				}
				if strings.HasSuffix(str, " ") {
					str = strings.TrimSuffix(str, " ")
					if hasNext(n) {
						spaceWidths += spaceWidth
					}
				}
			}
		} else if str == "" {
			return blockAttrs{}
		}

		if parentElementType == "pre" {
			originalLineHeight := ruler.LineHeightFactor
			ruler.LineHeightFactor = LineHeight_pre
			defer func() {
				ruler.LineHeightFactor = originalLineHeight
			}()
		}
		w, h := ruler.MeasurePrecise(font, str)
		if cssLineBoxes {
			// CSS lays out adjacent inline runs by cursor advance, not glyph ink
			// bounds. Use the same advance as the SVG painter so table columns,
			// centered cells, and styled spans share one coordinate system.
			if advance, ok := ruler.measureFontAdvance(font, str); ok {
				w = advance
			} else {
				w = ruler.scaleUnicodeCSS(w, font, str)
			}
		} else if !isCode {
			// The browser-era Markdown measurer applied its Unicode fallback
			// correction only to proportional text. Preserve the raw mono
			// measurement for legacy graph dimensions; native painting still
			// reserves browser-like fallback advances above.
			w = ruler.scaleUnicodeLegacy(w, font, str)
		}
		w *= fontScale
		h *= fontScale
		if isCode && (!isHeadingCode || !cssLineBoxes) {
			w *= FontSize_pre_code_em
			h *= FontSize_pre_code_em
		}
		if debugMeasure {
			fmt.Printf("%stext(%v,%v)\n", depthStr, w, h)
		}
		return blockAttrs{w + spaceWidths, h, 0, 0, 0}
	case html.ElementNode:
		isCode := false
		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			if cssLineBoxes {
				fontScale *= HeaderToFontScale(n.Data)
			} else {
				fontSize = HeaderToFontSize(fontSize, n.Data)
			}
			fontStyle = d2fonts.FONT_STYLE_SEMIBOLD
			bold = false
			originalLineHeight := ruler.LineHeightFactor
			ruler.LineHeightFactor = LineHeight_h
			defer func() {
				ruler.LineHeightFactor = originalLineHeight
			}()
		case "em":
			fontStyle = d2fonts.FONT_STYLE_ITALIC
			italic = true
		case "b", "strong":
			fontStyle = d2fonts.FONT_STYLE_BOLD
			bold = true
		case "th":
			fontStyle = d2fonts.FONT_STYLE_SEMIBOLD
			bold = true
		case "pre", "code":
			if monoFontFamily != nil {
				fontFamily = monoFontFamily
			} else {
				fontFamily = go2.Pointer(d2fonts.SourceCodePro)
			}
			// .md code selects the regular mono family, while CSS weight/style
			// continue to inherit from surrounding strong/em elements.
			fontStyle = d2fonts.FONT_STYLE_REGULAR
			isCode = true
		}

		block := blockAttrs{}
		cssFontSize := float64(fontSize) * fontScale
		lineHeightPx := cssFontSize * ruler.LineHeightFactor

		if n.FirstChild != nil {
			first := getNext(n.FirstChild)
			last := getPrev(n.LastChild)

			var blocks []blockAttrs
			var inlineBlock *blockAttrs
			// first create blocks from combined inline elements, then combine all blocks
			// inlineBlock will be non-nil while inline elements are being combined into a block
			endInlineBlock := func() {
				if !isCode && inlineBlock.height > 0 {
					if cssLineBoxes {
						inlineBlock.height = lineHeightPx
					} else if inlineBlock.height < lineHeightPx {
						inlineBlock.height = lineHeightPx
					}
				}
				blocks = append(blocks, *inlineBlock)
				inlineBlock = nil
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				childBlock := ruler.measureMarkdownNode(depth+1, child, fontFamily, monoFontFamily, fontSize, fontScale, fontStyle, bold, italic, cssLineBoxes)

				if child.Type == html.ElementNode && isBlockElement(child.Data) {
					if inlineBlock != nil {
						endInlineBlock()
					}
					newBlock := &blockAttrs{}
					newBlock.width = childBlock.width
					newBlock.height = childBlock.height
					if child == first && n.Data == "blockquote" {
						newBlock.marginTop = 0.
					} else {
						newBlock.marginTop = childBlock.marginTop
					}
					if child == last && n.Data == "blockquote" {
						newBlock.marginBottom = 0.
					} else {
						newBlock.marginBottom = childBlock.marginBottom
					}

					blocks = append(blocks, *newBlock)
				} else if child.Type == html.ElementNode && child.Data == "br" {
					if inlineBlock != nil {
						endInlineBlock()
					} else {
						block.height += lineHeightPx
					}
				} else if childBlock.isNotEmpty() {
					if inlineBlock == nil {
						// start inline block with child
						inlineBlock = &childBlock
					} else {
						// stack inline element dimensions horizontally
						inlineBlock.width += childBlock.width
						inlineBlock.height = go2.Max(inlineBlock.height, childBlock.height)

						inlineBlock.marginTop = go2.Max(inlineBlock.marginTop, childBlock.marginTop)
						inlineBlock.marginBottom = go2.Max(inlineBlock.marginBottom, childBlock.marginBottom)
					}
				}
			}
			if inlineBlock != nil {
				endInlineBlock()
			}

			var prevMarginBottom float64
			for i, b := range blocks {
				if i == 0 {
					block.marginTop = go2.Max(block.marginTop, b.marginTop)
				} else {
					marginDiff := b.marginTop - prevMarginBottom
					if marginDiff > 0 {
						block.height += marginDiff
					}
				}
				if i == len(blocks)-1 {
					block.marginBottom = go2.Max(block.marginBottom, b.marginBottom)
				} else {
					block.height += b.marginBottom
					prevMarginBottom = b.marginBottom
				}

				block.height += b.height
				block.width = go2.Max(block.width, b.width)
			}
		}

		switch n.Data {
		case "img":
			// Preserve the browser-era measurement contract: image nodes have no
			// intrinsic dimensions here. Native SVG omits them rather than fetching.
		case "blockquote":
			borderWidth := BorderLeft_blockquote_em * cssFontSize
			if cssLineBoxes {
				borderWidth = go2.Max(1, math.Floor(borderWidth))
			}
			block.width += 2*PaddingLR_blockquote_em*cssFontSize + borderWidth
			block.marginBottom = go2.Max(block.marginBottom, MarginBottom_blockquote)
		case "p":
			if parentElementType == "li" {
				block.marginTop = go2.Max(block.marginTop, MarginTop_li_p)
			}
			block.marginBottom = go2.Max(block.marginBottom, MarginBottom_p)
		case "h1", "h2", "h3", "h4", "h5", "h6":
			block.marginTop = go2.Max(block.marginTop, MarginTop_h)
			block.marginBottom = go2.Max(block.marginBottom, MarginBottom_h)
			switch n.Data {
			case "h1", "h2":
				block.height += PaddingBottom_h1_h2_em*cssFontSize + BorderBottom_h1_h2
			}
		case "li":
			block.width += PaddingLeft_ul_ol_em * cssFontSize
			// CSS paints an empty list marker in a line box, but D2's historical
			// graph-layout measurement intentionally gives an empty item no height.
			// Preserve that legacy viewport while letting the CSS-only paint pass
			// position the (ultimately clipped) marker like Chromium.
			if cssLineBoxes && block.height == 0 {
				block.height = lineHeightPx
			}
			if hasPrev(n) {
				block.marginTop = go2.Max(block.marginTop, MarginTop_li_em*cssFontSize)
			}
		case "ol", "ul":
			if hasAncestorElement(n, "ul") || hasAncestorElement(n, "ol") {
				block.marginTop = 0
				block.marginBottom = 0
			} else {
				block.marginBottom = go2.Max(block.marginBottom, MarginBottom_ul)
			}
		case "pre":
			// CSS white-space: pre creates one line box per logical line, except
			// that a final newline does not create an extra empty line box. Glyph
			// bounds alone undercount leading and blank lines. The compatibility
			// measurement keeps its historical glyph-bound height; only native
			// painting uses the exact CSS content height.
			if lineCount := markdownPreLineCount(markdownNodeText(n)); cssLineBoxes && lineCount > 0 {
				cssContentHeight := float64(lineCount) * LineHeight_pre * FontSize_pre_code_em * cssFontSize
				block.height = go2.Max(block.height, cssContentHeight)
			}
			block.width += 2 * Padding_pre
			block.height += 2 * Padding_pre
			block.marginBottom = go2.Max(block.marginBottom, MarginBottom_pre)
		case "code":
			if parentElementType != "pre" {
				if cssLineBoxes && hasMarkdownHeadingAncestor(n) {
					block.width += 2 * PaddingLeftRight_heading_code_em * cssFontSize
					block.height = lineHeightPx
				} else if cssLineBoxes {
					codeFontSize := FontSize_pre_code_em * cssFontSize
					block.width += 2 * PaddingLeftRight_code_em * codeFontSize
					if ascent, descent, ok := ruler.cssFontBoxMetrics(fontFamily.Font(fontSize, fontStyle), codeFontSize); ok {
						block.height = ascent + descent
					}
					block.height += 2 * PaddingTopBottom_code_em * codeFontSize
				} else {
					block.width += 2 * PaddingLeftRight_code_em * cssFontSize
					block.height += 2 * PaddingTopBottom_code_em * cssFontSize
				}
			}
		case "hr":
			block.height += Height_hr_em * cssFontSize
			block.marginTop = go2.Max(block.marginTop, MarginTopBottom_hr)
			block.marginBottom = go2.Max(block.marginBottom, MarginTopBottom_hr)
		case "table":
			var columnWidths []float64
			var tableHeight float64

			// Border width for table (outer border)
			tableBorder := 1.0

			// Iterate over child nodes (tbody, thead, tr)
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && (child.Data == "tbody" || child.Data == "thead" || child.Data == "tfoot") {
					childAttrs := ruler.measureMarkdownNode(depth+1, child, fontFamily, monoFontFamily, fontSize, fontScale, fontStyle, bold, italic, cssLineBoxes)
					tableHeight += childAttrs.height

					if childColumnWidths, ok := childAttrs.extraData.([][]float64); ok {
						columnWidths = mergeColumnWidths(columnWidths, childColumnWidths)
					}
				} else if child.Type == html.ElementNode && child.Data == "tr" {
					rowAttrs := ruler.measureMarkdownNode(depth+1, child, fontFamily, monoFontFamily, fontSize, fontScale, fontStyle, bold, italic, cssLineBoxes)
					tableHeight += rowAttrs.height

					if rowCellWidths, ok := rowAttrs.extraData.([]float64); ok {
						columnWidths = mergeColumnWidths(columnWidths, [][]float64{rowCellWidths})
					}
				}
			}

			// Calculate total table width including ALL borders
			tableWidth := 0.0
			if len(columnWidths) > 0 {
				// Add widths of all columns
				for _, colWidth := range columnWidths {
					tableWidth += colWidth
				}

				// Add border for every column division (including outer borders)
				tableWidth += float64(len(columnWidths)+1) * tableBorder
			}

			// Add outer borders to height
			tableHeight += 2 * tableBorder

			block.width = tableWidth
			block.height = tableHeight
			if cssLineBoxes {
				block.marginBottom = go2.Max(block.marginBottom, MarginBottom_table)
			}

		case "thead", "tbody", "tfoot":
			var sectionWidth, sectionHeight float64
			var sectionColumnWidths [][]float64

			// Iterate over tr elements
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && child.Data == "tr" {
					childAttrs := ruler.measureMarkdownNode(depth+1, child, fontFamily, monoFontFamily, fontSize, fontScale, fontStyle, bold, italic, cssLineBoxes)
					sectionHeight += childAttrs.height
					sectionWidth = go2.Max(sectionWidth, childAttrs.width)

					if rowCellWidths, ok := childAttrs.extraData.([]float64); ok {
						sectionColumnWidths = append(sectionColumnWidths, rowCellWidths)
					}
				}
			}

			block.width = sectionWidth
			block.height = sectionHeight
			block.extraData = sectionColumnWidths // Pass column widths back to table

		case "td", "th":
			if !cssLineBoxes {
				// Preserve D2's historical graph-layout measurement for mixed
				// inline table-cell children: the widest child determines width,
				// while child heights accumulate. Browser-style painting uses the
				// generic inline aggregation calculated above.
				var cellContentWidth, cellContentHeight float64
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					childAttrs := ruler.measureMarkdownNode(
						depth+1, child, fontFamily, monoFontFamily, fontSize,
						fontScale, fontStyle, bold, italic, false,
					)
					cellContentWidth = go2.Max(cellContentWidth, childAttrs.width)
					cellContentHeight += childAttrs.height
				}
				block.width = cellContentWidth
				block.height = cellContentHeight
			}

		case "tr":
			var rowWidth, rowHeight float64
			var cellWidths []float64

			cellBorder := 1.0
			rowBorder := 1.0

			maxCellHeight := 0.0
			cellCount := 0

			// Check if this row is in a thead to determine default font style for cells
			inHeader := hasAncestorElement(n, "thead")
			rowFontStyle := fontStyle
			rowBold := bold
			if inHeader {
				rowFontStyle = d2fonts.FONT_STYLE_SEMIBOLD
				rowBold = true
			}

			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
					cellCount++

					// Use semibold for th elements regardless of location
					childFontStyle := rowFontStyle
					childBold := rowBold
					if child.Data == "th" {
						childFontStyle = d2fonts.FONT_STYLE_SEMIBOLD
						childBold = true
					}

					childAttrs := ruler.measureMarkdownNode(depth+1, child, fontFamily, monoFontFamily, fontSize, fontScale, childFontStyle, childBold, italic, cssLineBoxes)
					cellPaddingH := 13.0 * 2
					cellPaddingV := 6.0 * 2

					cellWidth := childAttrs.width + cellPaddingH
					cellHeight := childAttrs.height + cellPaddingV

					cellWidths = append(cellWidths, cellWidth)
					maxCellHeight = go2.Max(maxCellHeight, cellHeight)
				}
			}

			if cellCount > 0 {
				for _, w := range cellWidths {
					rowWidth += w
				}
				rowWidth += float64(cellCount+1) * cellBorder
			}

			rowHeight = maxCellHeight + rowBorder

			block.width = rowWidth
			block.height = rowHeight
			block.extraData = cellWidths
		}
		if !cssLineBoxes && block.height > 0 && block.height < lineHeightPx {
			block.height = lineHeightPx
		}
		if debugMeasure {
			fmt.Printf("%s%s(%v,%v) mt:%v mb:%v\n", depthStr, n.Data, block.width, block.height, block.marginTop, block.marginBottom)
		}
		return block
	}
	return blockAttrs{}
}

func markdownPreLineCount(text string) int {
	if text == "" {
		return 0
	}
	count := strings.Count(text, "\n") + 1
	if strings.HasSuffix(text, "\n") {
		count--
	}
	return count
}

func mergeColumnWidths(existing []float64, new [][]float64) []float64 {
	for _, rowWidths := range new {
		for i, width := range rowWidths {
			if i >= len(existing) {
				existing = append(existing, width)
			} else {
				existing[i] = go2.Max(existing[i], width)
			}
		}
	}
	return existing
}
