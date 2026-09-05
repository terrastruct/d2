package d2latex

import (
	mathjax "github.com/d2lang/mathjax-go"
	"github.com/d2lang/util-go/xdefer"
)

func Render(s string) (_ string, err error) {
	defer xdefer.Errorf(&err, "latex failed to parse")
	return mathjax.Render(s)
}

func Measure(s string) (width, height int, err error) {
	defer xdefer.Errorf(&err, "latex failed to parse")
	return mathjax.Measure(s)
}
