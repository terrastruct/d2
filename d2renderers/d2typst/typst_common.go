package d2typst

import (
	"fmt"
	"math"
	"regexp"
	"strconv"

	"oss.terrastruct.com/util-go/xdefer"
)

var pxPerEx = 8

// Matches SVG width/height in pt units from Typst output
// <svg class="typst-doc" viewBox="0 0 595.2755905511812 841.8897637795276" width="595.2755905511812pt" height="841.8897637795276pt"
var svgRe = regexp.MustCompile(`<svg[^>]+width="([0-9\.]+)pt" height="([0-9\.]+)pt"[^>]*>`)

// typstJS and setupJS will be set by build-specific files (typst_embed.go or typst_embed_wasm.go)

func Render(s string) (_ string, err error) {
	defer xdefer.Errorf(&err, "typst failed to parse")
	return render(s)
}

func Measure(s string) (width, height int, err error) {
	defer xdefer.Errorf(&err, "typst failed to parse")
	svg, err := Render(s)
	if err != nil {
		return 0, 0, err
	}

	dims := svgRe.FindAllStringSubmatch(svg, -1)
	if len(dims) != 1 || len(dims[0]) != 3 {
		return 0, 0, fmt.Errorf("svg parsing failed for typst: %v", svg)
	}

	wStr := dims[0][1]
	hStr := dims[0][2]

	wf, err := strconv.ParseFloat(wStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("svg parsing failed for typst: %v", svg)
	}
	hf, err := strconv.ParseFloat(hStr, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("svg parsing failed for typst: %v", svg)
	}

	// Convert pt (Typst's default unit) to pixels
	// 1 pt = 1/72 inch, 96 DPI standard
	// 1 pt = 96/72 = 1.333... pixels
	ptToPx := 96.0 / 72.0

	return int(math.Ceil(wf * ptToPx)), int(math.Ceil(hf * ptToPx)), nil
}
