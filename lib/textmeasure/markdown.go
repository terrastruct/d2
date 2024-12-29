package textmeasure

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
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
	LineHeight_pre       = 1.45
	FontSize_pre_code_em = 0.85

	PaddingTopBottom_code_em = 0.2
	PaddingLeftRight_code_em = 0.4

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
	switch header {
	case "h1":
		return int(h1_em * float64(baseFontSize))
	case "h2":
		return int(h2_em * float64(baseFontSize))
	case "h3":
		return int(h3_em * float64(baseFontSize))
	case "h4":
		return int(h4_em * float64(baseFontSize))
	case "h5":
		return int(h5_em * float64(baseFontSize))
	case "h6":
		return int(h6_em * float64(baseFontSize))
	}
	return 0
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

func MeasureMarkdown(mdText string, ruler *Ruler, fontFamily *d2fonts.FontFamily, fontSize int) (width, height int, err error) {
	render, err := RenderMarkdown(mdText)
	if err != nil {
		return width, height, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(render))
	if err != nil {
		return width, height, err
	}

	{
		originalLineHeight := ruler.LineHeightFactor
		ruler.boundsWithDot = true
		ruler.LineHeightFactor = MarkdownLineHeight
		defer func() {
			ruler.LineHeightFactor = originalLineHeight
			ruler.boundsWithDot = false
		}()
	}

	// TODO consider setting a max width + (manual) text wrapping
	bodyNode := doc.Find("body").First().Nodes[0]
	bodyAttrs := ruler.measureNode(0, bodyNode, fontFamily, fontSize, d2fonts.FONT_STYLE_REGULAR)

	return int(math.Ceil(bodyAttrs.width)), int(math.Ceil(bodyAttrs.height)), nil
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

// measures node dimensions to match rendering with styles in github-markdown.css
func (ruler *Ruler) measureNode(depth int, n *html.Node, fontFamily *d2fonts.FontFamily, fontSize int, fontStyle d2fonts.FontStyle) blockAttrs {
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
		if strings.Trim(n.Data, "\n\t\b") == "" {
			return blockAttrs{}
		}
		str := n.Data
		isCode := parentElementType == "pre" || parentElementType == "code"
		spaceWidths := 0.

		if !isCode {
			spaceWidth := ruler.spaceWidth(font)
			// MeasurePrecise will not include leading or trailing whitespace, so we account for it here
			str = strings.ReplaceAll(str, "\n", " ")
			str = strings.ReplaceAll(str, "\t", " ")
			if strings.HasPrefix(str, " ") {
				// consecutive leading/trailing spaces end up rendered as a single space
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

		if parentElementType == "pre" {
			originalLineHeight := ruler.LineHeightFactor
			ruler.LineHeightFactor = LineHeight_pre
			defer func() {
				ruler.LineHeightFactor = originalLineHeight
			}()
		}
		w, h := ruler.MeasurePrecise(font, str)
		if isCode {
			w *= FontSize_pre_code_em
			h *= FontSize_pre_code_em
		} else {
			w = ruler.scaleUnicode(w, font, str)
		}
		if debugMeasure {
			fmt.Printf("%stext(%v,%v)\n", depthStr, w, h)
		}
		return blockAttrs{w + spaceWidths, h, 0, 0, 0}
	case html.ElementNode:
		isCode := false
		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			fontSize = HeaderToFontSize(fontSize, n.Data)
			fontStyle = d2fonts.FONT_STYLE_SEMIBOLD
			originalLineHeight := ruler.LineHeightFactor
			ruler.LineHeightFactor = LineHeight_h
			defer func() {
				ruler.LineHeightFactor = originalLineHeight
			}()
		case "em":
			fontStyle = d2fonts.FONT_STYLE_ITALIC
		case "b", "strong":
			fontStyle = d2fonts.FONT_STYLE_BOLD
		case "pre", "code":
			fontFamily = go2.Pointer(d2fonts.SourceCodePro)
			fontStyle = d2fonts.FONT_STYLE_REGULAR
			isCode = true
		}

		block := blockAttrs{}
		lineHeightPx := float64(fontSize) * ruler.LineHeightFactor

		if n.FirstChild != nil {
			first := getNext(n.FirstChild)
			last := getPrev(n.LastChild)

			var blocks []blockAttrs
			var inlineBlock *blockAttrs
			// first create blocks from combined inline elements, then combine all blocks
			// inlineBlock will be non-nil while inline elements are being combined into a block
			endInlineBlock := func() {
				if !isCode && inlineBlock.height > 0 && inlineBlock.height < lineHeightPx {
					inlineBlock.height = lineHeightPx
				}
				blocks = append(blocks, *inlineBlock)
				inlineBlock = nil
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				childBlock := ruler.measureNode(depth+1, child, fontFamily, fontSize, fontStyle)

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
		case "blockquote":
			block.width += (2*PaddingLR_blockquote_em + BorderLeft_blockquote_em) * float64(fontSize)
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
				block.height += PaddingBottom_h1_h2_em*float64(fontSize) + BorderBottom_h1_h2
			}
		case "li":
			block.width += PaddingLeft_ul_ol_em * float64(fontSize)
			if hasPrev(n) {
				block.marginTop = go2.Max(block.marginTop, MarginTop_li_em*float64(fontSize))
			}
		case "ol", "ul":
			if hasAncestorElement(n, "ul") || hasAncestorElement(n, "ol") {
				block.marginTop = 0
				block.marginBottom = 0
			} else {
				block.marginBottom = go2.Max(block.marginBottom, MarginBottom_ul)
			}
		case "pre":
			block.width += 2 * Padding_pre
			block.height += 2 * Padding_pre
			block.marginBottom = go2.Max(block.marginBottom, MarginBottom_pre)
		case "code":
			if parentElementType != "pre" {
				block.width += 2 * PaddingLeftRight_code_em * float64(fontSize)
				block.height += 2 * PaddingTopBottom_code_em * float64(fontSize)
			}
		case "hr":
			block.height += Height_hr_em * float64(fontSize)
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
					childAttrs := ruler.measureNode(depth+1, child, fontFamily, fontSize, fontStyle)
					tableHeight += childAttrs.height

					if childColumnWidths, ok := childAttrs.extraData.([][]float64); ok {
						columnWidths = mergeColumnWidths(columnWidths, childColumnWidths)
					}
				} else if child.Type == html.ElementNode && child.Data == "tr" {
					rowAttrs := ruler.measureNode(depth+1, child, fontFamily, fontSize, fontStyle)
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

		case "thead", "tbody", "tfoot":
			var sectionWidth, sectionHeight float64
			var sectionColumnWidths [][]float64

			// Iterate over tr elements
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && child.Data == "tr" {
					childAttrs := ruler.measureNode(depth+1, child, fontFamily, fontSize, fontStyle)
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
			// Apply semibold style to header cells
			cellFontStyle := fontStyle
			if n.Data == "th" {
				cellFontStyle = d2fonts.FONT_STYLE_SEMIBOLD
			}

			// Measure cell content with appropriate font style
			var cellContentWidth, cellContentHeight float64

			for child := n.FirstChild; child != nil; child = child.NextSibling {
				// Pass the header-specific font style to child measurements
				childAttrs := ruler.measureNode(depth+1, child, fontFamily, fontSize, cellFontStyle)
				cellContentWidth = go2.Max(cellContentWidth, childAttrs.width)
				cellContentHeight += childAttrs.height
			}

			block.width = cellContentWidth
			block.height = cellContentHeight

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
			if inHeader {
				rowFontStyle = d2fonts.FONT_STYLE_SEMIBOLD
			}

			for child := n.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
					cellCount++

					// Use semibold for th elements regardless of location
					childFontStyle := rowFontStyle
					if child.Data == "th" {
						childFontStyle = d2fonts.FONT_STYLE_SEMIBOLD
					}

					childAttrs := ruler.measureNode(depth+1, child, fontFamily, fontSize, childFontStyle)
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
		if block.height > 0 && block.height < lineHeightPx {
			block.height = lineHeightPx
		}
		if debugMeasure {
			fmt.Printf("%s%s(%v,%v) mt:%v mb:%v\n", depthStr, n.Data, block.width, block.height, block.marginTop, block.marginBottom)
		}
		return block
	}
	return blockAttrs{}
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
