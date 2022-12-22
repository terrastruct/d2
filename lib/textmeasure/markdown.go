package textmeasure

import (
	"bytes"
	"math"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	goldmarkHtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"

	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
)

var markdownRenderer goldmark.Markdown

// these are css values from github-markdown.css so we can accurately compute the rendered dimensions
const (
	MarkdownFontSize     = d2fonts.FONT_SIZE_M
	MarkdownLineHeight   = 1.5
	MarkdownLineHeightPx = MarkdownFontSize * MarkdownLineHeight

	PaddingLeft_ul_ol = 32
	MarginBottom_ul   = MarkdownFontSize

	MarginTop_li_p = MarkdownFontSize
	MarginBottom_p = MarkdownFontSize

	LineHeight_h           = 1.25
	MarginTop_h            = 24
	MarginBottom_h         = 16
	PaddingBottom_h1_h2_em = 0.3

	Height_hr          = 4
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

	FONT_SIZE_H1 = d2fonts.FONT_SIZE_XXXL
	FONT_SIZE_H2 = d2fonts.FONT_SIZE_XL
	FONT_SIZE_H3 = d2fonts.FONT_SIZE_L
	FONT_SIZE_H4 = d2fonts.FONT_SIZE_M
	FONT_SIZE_H5 = d2fonts.FONT_SIZE_S
	FONT_SIZE_H6 = d2fonts.FONT_SIZE_XS
)

var HeaderToFontSize = map[string]int{
	"h1": FONT_SIZE_H1,
	"h2": FONT_SIZE_H2,
	"h3": FONT_SIZE_H3,
	"h4": FONT_SIZE_H4,
	"h5": FONT_SIZE_H5,
	"h6": FONT_SIZE_H6,
}

var HeaderFonts map[string]d2fonts.Font

func RenderMarkdown(m string) (string, error) {
	var output bytes.Buffer
	if err := markdownRenderer.Convert([]byte(m), &output); err != nil {
		return "", err
	}
	return output.String(), nil
}

func init() {
	HeaderFonts = make(map[string]d2fonts.Font)
	for header, fontSize := range HeaderToFontSize {
		HeaderFonts[header] = d2fonts.HandDrawn.Font(fontSize, d2fonts.FONT_STYLE_BOLD)
	}

	markdownRenderer = goldmark.New(
		goldmark.WithRendererOptions(
			goldmarkHtml.WithUnsafe(),
			goldmarkHtml.WithXHTML(),
		),
	)
}

func MeasureMarkdown(mdText string, ruler *Ruler) (width, height int, err error) {
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

	font := d2fonts.HandDrawn.Font(MarkdownFontSize, d2fonts.FONT_STYLE_REGULAR)

	// TODO consider setting a max width + (manual) text wrapping
	bodyNode := doc.Find("body").First().Nodes[0]
	bodyAttrs := ruler.measureNode(0, bodyNode, font)

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
		"ul":
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
}

func (b *blockAttrs) isNotEmpty() bool {
	return b != nil && *b != blockAttrs{}
}

// measures node dimensions to match rendering with styles in github-markdown.css
func (ruler *Ruler) measureNode(depth int, n *html.Node, font d2fonts.Font) blockAttrs {
	var parentElementType string
	if n.Parent != nil && n.Parent.Type == html.ElementNode {
		parentElementType = n.Parent.Data
	}

	switch n.Type {
	case html.TextNode:
		if strings.TrimSpace(n.Data) == "" {
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
		}
		return blockAttrs{w + spaceWidths, h, 0, 0}
	case html.ElementNode:
		isCode := false
		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			font = HeaderFonts[n.Data]
			originalLineHeight := ruler.LineHeightFactor
			ruler.LineHeightFactor = LineHeight_h
			defer func() {
				ruler.LineHeightFactor = originalLineHeight
			}()
		case "em":
			font.Style = d2fonts.FONT_STYLE_ITALIC
		case "b", "strong":
			font.Style = d2fonts.FONT_STYLE_BOLD
		case "pre", "code":
			font.Family = d2fonts.SourceCodePro
			font.Style = d2fonts.FONT_STYLE_REGULAR
			isCode = true
		}

		block := blockAttrs{}

		if n.FirstChild != nil {
			first := getNext(n.FirstChild)
			last := getPrev(n.LastChild)

			var blocks []blockAttrs
			var current *blockAttrs
			// first create blocks from combined inline elements, then combine all blocks
			// current will be non-nil while inline elements are being combined into a block
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				childBlock := ruler.measureNode(depth+1, child, font)

				if child.Type == html.ElementNode && isBlockElement(child.Data) {
					if current != nil {
						blocks = append(blocks, *current)
					}
					current = &blockAttrs{}
					if child == first && n.Data == "blockquote" {
						current.marginTop = 0.
					} else {
						current.marginTop = childBlock.marginTop
					}
					if child == last && n.Data == "blockquote" {
						current.marginBottom = 0.
					} else {
						current.marginBottom = childBlock.marginBottom
					}

					current.width = childBlock.width
					current.height = childBlock.height
					blocks = append(blocks, *current)
					current = nil
				} else if child.Type == html.ElementNode && child.Data == "br" {
					if current != nil {
						if !isCode && current.height > 0 && current.height < MarkdownLineHeightPx {
							current.height = MarkdownLineHeightPx
						}
						blocks = append(blocks, *current)
						current = nil
					}
				} else if childBlock.isNotEmpty() {
					if current == nil {
						current = &childBlock
					} else {
						current.marginTop = go2.Max(current.marginTop, childBlock.marginTop)
						current.marginBottom = go2.Max(current.marginBottom, childBlock.marginBottom)

						current.width += childBlock.width
						current.height = go2.Max(current.height, childBlock.height)
					}
				}
			}
			if current != nil {
				if !isCode && current.height > 0 && current.height < MarkdownLineHeightPx {
					current.height = MarkdownLineHeightPx
				}
				blocks = append(blocks, *current)
				current = nil
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
			block.width += (2*PaddingLR_blockquote_em + BorderLeft_blockquote_em) * float64(font.Size)
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
				block.height += PaddingBottom_h1_h2_em * float64(font.Size)
			}
		case "li":
			block.width += PaddingLeft_ul_ol
			if hasPrev(n) {
				block.marginTop = go2.Max(block.marginTop, 4)
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
				block.width += 2 * PaddingLeftRight_code_em * float64(font.Size)
				block.height += 2 * PaddingTopBottom_code_em * float64(font.Size)
			}
		case "hr":
			block.height += Height_hr
			block.marginTop = go2.Max(block.marginTop, MarginTopBottom_hr)
			block.marginBottom = go2.Max(block.marginBottom, MarginTopBottom_hr)
		}
		if block.height > 0 && block.height < MarkdownLineHeightPx {
			block.height = MarkdownLineHeightPx
		}
		return block
	}
	return blockAttrs{}
}
