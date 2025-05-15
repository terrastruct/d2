// appendix.go writes appendices/footnotes to SVG
// Intended to be run only for static exports, like PNG or PDF.
// SVG exports are already interactive.

package appendix

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/d2themes"
	"oss.terrastruct.com/d2/lib/color"
	svglib "oss.terrastruct.com/d2/lib/svg"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

//        ┌──────────────┐
//        │              │
//        │   DIAGRAM    │
//        │              │
// PAD_   │              │
// SIDES  │              │
//    │   │              │
//    │   └──────────────┘
//    ▼                   ◄──────  PAD_TOP
//
//    ─────────────────────────
//
//
//         1. asdfasdf
//
//                        ◄──── SPACER
//         2. qwerqwer
//
//

const (
	PAD_TOP   = 50
	PAD_SIDES = 40
	SPACER    = 20

	FONT_SIZE   = 16
	ICON_RADIUS = 16
)

var viewboxRegex = regexp.MustCompile(`viewBox=\"([0-9\- ]+)\"`)
var widthRegex = regexp.MustCompile(`width=\"([.0-9]+)\"`)
var heightRegex = regexp.MustCompile(`height=\"([.0-9]+)\"`)
var svgRegex = regexp.MustCompile(`<svg(.*?)>`)

func FindViewboxSlice(svg []byte) []string {
	viewboxMatches := viewboxRegex.FindAllStringSubmatch(string(svg), 2)
	viewboxMatch := viewboxMatches[1]
	viewboxRaw := viewboxMatch[1]
	return strings.Split(viewboxRaw, " ")
}

func Append(diagram *d2target.Diagram, renderOpts *d2svg.RenderOpts, ruler *textmeasure.Ruler, in []byte) []byte {
	svg := string(in)

	appendix, w, h := generateAppendix(diagram, ruler, svg)

	if h == 0 {
		return in
	}

	// match 1st two viewboxes, 1st is outer fit-to-screen viewbox="0 0 innerWidth innerHeight"
	viewboxMatches := viewboxRegex.FindAllStringSubmatch(svg, 2)
	viewboxMatch := viewboxMatches[1]
	viewboxRaw := viewboxMatch[1]
	viewboxSlice := strings.Split(viewboxRaw, " ")
	viewboxPadLeft, _ := strconv.Atoi(viewboxSlice[0])
	viewboxWidth, _ := strconv.Atoi(viewboxSlice[2])
	viewboxHeight, _ := strconv.Atoi(viewboxSlice[3])

	tl, br := diagram.BoundingBox()
	separatorEl := d2themes.NewThemableElement("line", nil)
	separatorEl.X1 = float64(tl.X - PAD_SIDES)
	separatorEl.Y1 = float64(br.Y + PAD_TOP)
	separatorEl.X2 = float64(go2.IntMax(w, br.X) + PAD_SIDES)
	separatorEl.Y2 = float64(br.Y + PAD_TOP)
	separatorEl.Stroke = color.B2 // same as --color-border-muted in markdown
	appendix = separatorEl.Render() + appendix

	w -= viewboxPadLeft
	w += PAD_SIDES * 2
	if viewboxWidth < w {
		viewboxWidth = w
	}

	viewboxHeight += h + PAD_TOP

	newOuterViewbox := fmt.Sprintf(`viewBox="0 0 %d %d"`, viewboxWidth, viewboxHeight)
	newViewbox := fmt.Sprintf(`viewBox="%s %s %s %s"`, viewboxSlice[0], viewboxSlice[1], strconv.Itoa(viewboxWidth), strconv.Itoa(viewboxHeight))

	dimensionsToUpdate := 2
	outerSVG := svgRegex.FindString(svg)
	// if outer svg has dimensions set we also need to update it
	if widthRegex.FindString(outerSVG) != "" {
		dimensionsToUpdate++
	}

	// update 1st 3 matches of width and height 1st is outer svg (if dimensions are set), 2nd inner svg, 3rd is background color rect
	widthMatches := widthRegex.FindAllStringSubmatch(svg, dimensionsToUpdate)
	heightMatches := heightRegex.FindAllStringSubmatch(svg, dimensionsToUpdate)
	newWidth := fmt.Sprintf(`width="%s"`, strconv.Itoa(viewboxWidth))
	newHeight := fmt.Sprintf(`height="%s"`, strconv.Itoa(viewboxHeight))

	svg = strings.Replace(svg, viewboxMatches[0][0], newOuterViewbox, 1)
	svg = strings.Replace(svg, viewboxMatch[0], newViewbox, 1)
	for i := 0; i < dimensionsToUpdate; i++ {
		svg = strings.Replace(svg, widthMatches[i][0], newWidth, 1)
		svg = strings.Replace(svg, heightMatches[i][0], newHeight, 1)
	}

	if !strings.Contains(svg, `font-family: "font-regular"`) {
		appendix += fmt.Sprintf(`<style type="text/css"><![CDATA[
.text {
	font-family: "font-regular";
}
@font-face {
	font-family: font-regular;
	src: url("%s");
}
]]></style>`, d2fonts.FontEncodings.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)))
	}
	if !strings.Contains(svg, `font-family: "font-bold"`) {
		appendix += fmt.Sprintf(`<style type="text/css"><![CDATA[
.text-bold {
	font-family: "font-bold";
}
@font-face {
	font-family: font-bold;
	src: url("%s");
}
]]></style>`, d2fonts.FontEncodings.Get(d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_BOLD)))
	}

	closingIndex := strings.LastIndex(svg, "</svg></svg>")
	svg = svg[:closingIndex] + appendix + svg[closingIndex:]

	// icons are numbered according to diagram.Shapes which is based on their order of definition,
	// but they appear in the svg according to renderOrder so we have to replace in that order
	type appendixIcon struct {
		number    int
		isTooltip bool
		shape     d2target.Shape
	}
	var renderOrder []appendixIcon

	i := 1
	for _, s := range diagram.Shapes {
		if s.Tooltip != "" {
			renderOrder = append(renderOrder, appendixIcon{i, true, s})
			i++
		}
		if s.Link != "" {
			renderOrder = append(renderOrder, appendixIcon{i, false, s})
			i++
		}
	}
	// sort to match render order
	sort.SliceStable(renderOrder, func(i, j int) bool {
		iZIndex := renderOrder[i].shape.GetZIndex()
		jZIndex := renderOrder[j].shape.GetZIndex()
		if iZIndex != jZIndex {
			return iZIndex < jZIndex
		}
		return renderOrder[i].shape.Level < renderOrder[j].shape.Level
	})

	var salt *string
	if renderOpts != nil {
		salt = renderOpts.Salt
	}
	diagramHash, err := diagram.HashID(salt)
	if err != nil {
		return nil
	}
	// replace each rendered svg icon
	for _, icon := range renderOrder {
		// The clip-path has a unique ID, so this won't replace any user icons
		// In the existing SVG, the transform places it top-left, so we adjust
		var iconStr string
		if icon.isTooltip {
			iconStr = fmt.Sprintf(d2svg.TooltipIcon, diagramHash, svglib.SVGID(icon.shape.ID))
		} else {
			iconStr = fmt.Sprintf(d2svg.LinkIcon, diagramHash, svglib.SVGID(icon.shape.ID))
		}
		svg = strings.Replace(svg, iconStr, generateNumberedIcon(icon.number, 0, ICON_RADIUS), 1)
	}

	return []byte(svg)
}

func generateAppendix(diagram *d2target.Diagram, ruler *textmeasure.Ruler, svg string) (string, int, int) {
	tl, br := diagram.BoundingBox()

	maxWidth, totalHeight := 0, 0

	var lines []string
	i := 1

	for _, s := range diagram.Shapes {
		for _, txt := range []string{s.Tooltip, s.PrettyLink} {
			if txt != "" {
				line, w, h := generateLine(i, br.Y+(PAD_TOP*2)+totalHeight, txt, ruler)
				i++
				lines = append(lines, line)
				maxWidth = go2.IntMax(maxWidth, w)
				totalHeight += h + SPACER
			}
		}
	}
	if len(lines) == 0 {
		return "", 0, 0
	}
	totalHeight += SPACER

	return fmt.Sprintf(`<g class="appendix" x="%d" y="%d" width="%d" height="100%%">%s</g>
`, tl.X, br.Y, (br.X - tl.X), strings.Join(lines, "\n")), maxWidth, totalHeight
}

func generateNumberedIcon(i, x, y int) string {
	line := fmt.Sprintf(`<circle cx="%d" cy="%d" r="%d" fill="white" stroke="#DEE1EB" />`,
		x+ICON_RADIUS, y, ICON_RADIUS)

	line += fmt.Sprintf(`<text class="text-bold" x="%d" y="%d" style="font-size: %dpx;text-anchor:middle;">%d</text>`,
		x+ICON_RADIUS, y+5, FONT_SIZE, i)

	return line
}

func generateLine(i, y int, text string, ruler *textmeasure.Ruler) (string, int, int) {
	mtext := &d2target.MText{
		Text:     text,
		FontSize: FONT_SIZE,
	}

	dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)

	line := fmt.Sprintf(`<g transform="translate(%d %d)" class="appendix-icon">%s</g>`,
		0, y, generateNumberedIcon(i, 0, 0))

	line += fmt.Sprintf(`<text class="text" x="%d" y="%d" style="font-size: %dpx;">%s</text>`,
		ICON_RADIUS*3, y+5, FONT_SIZE, d2svg.RenderText(text, ICON_RADIUS*3, float64(dims.Height)))

	return line, dims.Width + ICON_RADIUS*3, go2.IntMax(dims.Height, ICON_RADIUS*2)
}
