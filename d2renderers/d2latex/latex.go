package d2latex

import (
	_ "embed"
	"fmt"

	v8 "rogchap.com/v8go"
)

//go:embed polyfills.js
var polyfillsJS string

//go:embed setup.js
var setupJS string

//go:embed mathjax.js
var mathjaxJS string

func SVG(s string) (string, error) {
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

	val, err := v8ctx.RunScript(fmt.Sprintf(`adaptor.innerHTML(html.convert("%s", {
  em: 16,
  ex: 8,
}))`, s), "value.js")
	if err != nil {
		return "", err
	}

	return val.String(), nil
}
