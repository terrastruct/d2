// appendix.go writes appendices/footnotes to SVG
// Intended to be run only for static exports, like PNG or PDF.
// SVG exports are already interactive.

package appendix

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

const (
	PAD_TOP     = 50
	PAD_SIDES   = 40
	FONT_SIZE   = 16
	SPACER      = 20
	ICON_RADIUS = 16
)

var viewboxRegex = regexp.MustCompile(`viewBox=\"([0-9\- ]+)\"`)
var widthRegex = regexp.MustCompile(`width=\"([0-9]+)\"`)
var heightRegex = regexp.MustCompile(`height=\"([0-9]+)\"`)

func AppendTooltips(diagram *d2target.Diagram, ruler *textmeasure.Ruler, in []byte) []byte {
	svg := string(in)

	appendix, w, h := generateTooltipAppendix(diagram, ruler, svg)

	if h == 0 {
		return in
	}

	viewboxMatch := viewboxRegex.FindStringSubmatch(svg)
	viewboxRaw := viewboxMatch[1]
	viewboxSlice := strings.Split(viewboxRaw, " ")
	viewboxPadLeft, _ := strconv.Atoi(viewboxSlice[0])
	viewboxWidth, _ := strconv.Atoi(viewboxSlice[2])
	viewboxHeight, _ := strconv.Atoi(viewboxSlice[3])

	tl, br := diagram.BoundingBox()
	seperator := fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#0A0F25" />`,
		tl.X-PAD_SIDES, br.Y+PAD_TOP, go2.IntMax(w, br.X)+PAD_SIDES, br.Y+PAD_TOP)
	appendix = seperator + appendix

	w -= viewboxPadLeft
	w += PAD_SIDES * 2
	if viewboxWidth < w {
		viewboxWidth = w
	}

	viewboxHeight += h + PAD_TOP

	newViewbox := fmt.Sprintf(`viewBox="%s %s %s %s"`, viewboxSlice[0], viewboxSlice[1], strconv.Itoa(viewboxWidth), strconv.Itoa(viewboxHeight))

	widthMatch := widthRegex.FindStringSubmatch(svg)
	heightMatch := heightRegex.FindStringSubmatch(svg)

	newWidth := fmt.Sprintf(`width="%s"`, strconv.Itoa(viewboxWidth))
	newHeight := fmt.Sprintf(`height="%s"`, strconv.Itoa(viewboxHeight))
	svg = strings.Replace(svg, viewboxMatch[0], newViewbox, 1)
	svg = strings.Replace(svg, widthMatch[0], newWidth, 1)
	svg = strings.Replace(svg, heightMatch[0], newHeight, 1)

	if !strings.Contains(svg, `font-family: "font-regular"`) {
		appendix += fmt.Sprintf(`<style type="text/css"><![CDATA[
.text {
	font-family: "font-regular";
}
@font-face {
	font-family: font-regular;
	src: url("%s");
}
]]></style>`, d2fonts.FontEncodings[d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_REGULAR)])
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
]]></style>`, d2fonts.FontEncodings[d2fonts.SourceSansPro.Font(0, d2fonts.FONT_STYLE_BOLD)])
	}

	closingIndex := strings.LastIndex(svg, "</svg>")
	svg = svg[:closingIndex] + appendix + svg[closingIndex:]

	i := 1
	for _, s := range diagram.Shapes {
		if s.Tooltip != "" {
			// The clip-path has a unique ID, so this won't replace any user icons
			// In the existing SVG, the transform places it top-left, so we adjust
			svg = strings.Replace(svg, d2svg.TooltipIcon, generateNumberedIcon(i, 0, ICON_RADIUS), 1)
			i++
		}
	}

	return []byte(svg)
}

func generateTooltipAppendix(diagram *d2target.Diagram, ruler *textmeasure.Ruler, svg string) (string, int, int) {
	tl, br := diagram.BoundingBox()

	maxWidth, totalHeight := 0, 0

	var tooltipLines []string
	i := 1
	for _, s := range diagram.Shapes {
		if s.Tooltip != "" {
			line, w, h := generateTooltipLine(i, br.Y+(PAD_TOP*2)+totalHeight, s.Tooltip, ruler)
			i++
			tooltipLines = append(tooltipLines, line)
			maxWidth = go2.IntMax(maxWidth, w)
			totalHeight += h + SPACER
		}
	}

	return fmt.Sprintf(`<g x="%d" y="%d" width="%d" height="100%%">%s</g>
`, tl.X, br.Y, (br.X - tl.X), strings.Join(tooltipLines, "\n")), maxWidth, totalHeight
}

func generateNumberedIcon(i, x, y int) string {
	line := fmt.Sprintf(`<circle cx="%d" cy="%d" r="%d" fill="white" stroke="#DEE1EB" />`,
		x+ICON_RADIUS, y, ICON_RADIUS)

	line += fmt.Sprintf(`<text class="text-bold" x="%d" y="%d" style="font-size: %dpx;text-anchor:middle;">%d</text>`,
		x+ICON_RADIUS, y+5, FONT_SIZE, i)

	return line
}

func generateTooltipLine(i, y int, text string, ruler *textmeasure.Ruler) (string, int, int) {
	mtext := &d2target.MText{
		Text:     text,
		FontSize: FONT_SIZE,
	}

	dims := d2graph.GetTextDimensions(nil, ruler, mtext, nil)

	// TODO box-shadow: 0px 0px 32px rgba(31, 36, 58, 0.1);
	line := fmt.Sprintf(`<g transform="translate(%d %d)">%s</g>`,
		0, y, generateNumberedIcon(i, 0, 0))

	line += fmt.Sprintf(`<text class="text" x="%d" y="%d" style="font-size: %dpx;">%s</text>`,
		ICON_RADIUS*3, y, FONT_SIZE, d2svg.RenderText(text, ICON_RADIUS*3, float64(dims.Height)))

	return line, dims.Width + ICON_RADIUS*3, dims.Height
}
