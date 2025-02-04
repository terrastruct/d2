package d2animate

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"oss.terrastruct.com/d2/d2renderers/d2sketch"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/version"
)

var transitionDurationMS = 1

func makeKeyframe(delayMS, durationMS, totalMS, identifier int, diagramHash string) string {
	percentageBefore := (math.Max(0, float64(delayMS-transitionDurationMS)) / float64(totalMS)) * 100.
	percentageStart := (float64(delayMS) / float64(totalMS)) * 100.
	percentageEnd := (float64(delayMS+durationMS-transitionDurationMS) / float64(totalMS)) * 100.
	if int(math.Ceil(percentageEnd)) == 100 {
		return fmt.Sprintf(`@keyframes d2Transition-%s-%d {
		0%%, %f%% {
				opacity: 0;
		}
		%f%%, %f%% {
				opacity: 1;
		}
}`, diagramHash, identifier, percentageBefore, percentageStart, math.Ceil(percentageEnd))
	}

	percentageAfter := (float64(delayMS+durationMS) / float64(totalMS)) * 100.
	return fmt.Sprintf(`@keyframes d2Transition-%s-%d {
		0%%, %f%% {
				opacity: 0;
		}
		%f%%, %f%% {
				opacity: 1;
		}
		%f%%, 100%% {
				opacity: 0;
		}
}`, diagramHash, identifier, percentageBefore, percentageStart, percentageEnd, percentageAfter)
}

func Wrap(rootDiagram *d2target.Diagram, svgs [][]byte, renderOpts d2svg.RenderOpts, intervalMS int) ([]byte, error) {
	buf := &bytes.Buffer{}

	// TODO account for stroke width of root border

	tl, br := rootDiagram.NestedBoundingBox()
	left := tl.X - int(*renderOpts.Pad)
	top := tl.Y - int(*renderOpts.Pad)
	width := br.X - tl.X + int(*renderOpts.Pad)*2
	height := br.Y - tl.Y + int(*renderOpts.Pad)*2

	var dimensions string
	if renderOpts.Scale != nil {
		dimensions = fmt.Sprintf(` width="%d" height="%d"`,
			int(math.Ceil((*renderOpts.Scale)*float64(width))),
			int(math.Ceil((*renderOpts.Scale)*float64(height))),
		)
	}

	fitToScreenWrapperOpening := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" d2Version="%s" preserveAspectRatio="xMinYMin meet" viewBox="0 0 %d %d"%s>`,
		version.Version,
		width, height, dimensions,
	)
	fmt.Fprint(buf, fitToScreenWrapperOpening)

	innerOpening := fmt.Sprintf(`<svg class="d2-svg" width="%d" height="%d" viewBox="%d %d %d %d">`,
		width, height, left, top, width, height)
	fmt.Fprint(buf, innerOpening)

	svgsStr := ""
	for _, svg := range svgs {
		svgsStr += string(svg) + " "
	}

	diagramHash, err := rootDiagram.HashID(renderOpts.Salt)
	if err != nil {
		return nil, err
	}

	d2svg.EmbedFonts(buf, diagramHash, svgsStr, rootDiagram.FontFamily, rootDiagram.GetNestedCorpus())

	themeStylesheet, err := d2svg.ThemeCSS(diagramHash, renderOpts.ThemeID, renderOpts.DarkThemeID, renderOpts.ThemeOverrides, renderOpts.DarkThemeOverrides)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(buf, `<style type="text/css"><![CDATA[%s%s]]></style>`, d2svg.BaseStylesheet, themeStylesheet)

	if rootDiagram.HasShape(func(s d2target.Shape) bool {
		return s.Label != "" && s.Type == d2target.ShapeText
	}) {
		css := d2svg.MarkdownCSS
		css = strings.ReplaceAll(css, "font-italic", fmt.Sprintf("%s-font-italic", diagramHash))
		css = strings.ReplaceAll(css, "font-bold", fmt.Sprintf("%s-font-bold", diagramHash))
		css = strings.ReplaceAll(css, "font-mono", fmt.Sprintf("%s-font-mono", diagramHash))
		css = strings.ReplaceAll(css, "font-regular", fmt.Sprintf("%s-font-regular", diagramHash))
		fmt.Fprintf(buf, `<style type="text/css">%s</style>`, css)
	}

	if renderOpts.Sketch != nil && *renderOpts.Sketch {
		d2sketch.DefineFillPatterns(buf, diagramHash)
	}

	fmt.Fprint(buf, `<style type="text/css"><![CDATA[`)
	for i := range svgs {
		fmt.Fprint(buf, makeKeyframe(i*intervalMS, intervalMS, len(svgs)*intervalMS, i, diagramHash))
	}
	fmt.Fprint(buf, `]]></style>`)

	for i, svg := range svgs {
		str := string(svg)
		str = strings.Replace(str, "<g", fmt.Sprintf(`<g style="animation: d2Transition-%s-%d %dms infinite"`, diagramHash, i, len(svgs)*intervalMS), 1)
		buf.Write([]byte(str))
	}

	fmt.Fprint(buf, "</svg>")
	fmt.Fprint(buf, "</svg>")

	return buf.Bytes(), nil
}
