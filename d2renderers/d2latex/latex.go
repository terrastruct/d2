//go:build cgo

package d2latex

import (
	_ "embed"
	"fmt"
	"math"
	"regexp"
	"strconv"

	"oss.terrastruct.com/util-go/xdefer"
	v8 "rogchap.com/v8go"
)

var pxPerEx = 8

//go:embed polyfills.js
var polyfillsJS string

//go:embed setup.js
var setupJS string

//go:embed mathjax.js
var mathjaxJS string

// Matches this
// <svg style="background: white;" xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="563" height="326" viewBox="-100 -100 563 326"><style type="text/css">
var svgRe = regexp.MustCompile(`<svg[^>]+width="([0-9\.]+)ex" height="([0-9\.]+)ex"[^>]+>`)

func Render(s string) (_ string, err error) {
	defer xdefer.Errorf(&err, "latex failed to parse")
	v8ctx := v8.NewContext()

	if _, err := v8ctx.RunScript(polyfillsJS, "polyfills.js"); err != nil {
		return "", err
	}

	if _, err := v8ctx.RunScript(mathjaxJS, "mathjax.js"); err != nil {
		return "", err
	}

	if _, err := v8ctx.RunScript(setupJS, "setup.js"); err != nil {
		return "", err
	}

	val, err := v8ctx.RunScript(fmt.Sprintf(`adaptor.innerHTML(html.convert(`+"`"+"%s`"+`, {
  em: %d,
  ex: %d,
}))`, s, pxPerEx*2, pxPerEx), "value.js")
	if err != nil {
		return "", err
	}

	return val.String(), nil
}

func Measure(s string) (width, height int, err error) {
	defer xdefer.Errorf(&err, "latex failed to parse")
	svg, err := Render(s)
	if err != nil {
		return 0, 0, err
	}

	dims := svgRe.FindAllStringSubmatch(svg, -1)
	if len(dims) != 1 || len(dims[0]) != 3 {
		return 0, 0, fmt.Errorf("svg parsing failed for latex: %v", svg)
	}

	wEx := dims[0][1]
	hEx := dims[0][2]

	wf, err := strconv.ParseFloat(wEx, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("svg parsing failed for latex: %v", svg)
	}
	hf, err := strconv.ParseFloat(hEx, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("svg parsing failed for latex: %v", svg)
	}

	return int(math.Ceil(wf * float64(pxPerEx))), int(math.Ceil(hf * float64(pxPerEx))), nil
}
