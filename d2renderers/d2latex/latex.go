//go:build !wasm

package d2latex

import (
	_ "embed"
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/dop251/goja"

	"oss.terrastruct.com/util-go/xdefer"
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
	vm := goja.New()

	if _, err := vm.RunString(polyfillsJS); err != nil {
		return "", err
	}

	if _, err := vm.RunString(mathjaxJS); err != nil {
		return "", err
	}

	if _, err := vm.RunString(setupJS); err != nil {
		return "", err
	}

	val, err := vm.RunString(fmt.Sprintf(`adaptor.innerHTML(html.convert(`+"`"+"%s`"+`, {
  em: %d,
  ex: %d,
}))`, s, pxPerEx*2, pxPerEx))
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
