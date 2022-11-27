package e2etests

import (
	_ "embed"
	"testing"
)

func testTodo(t *testing.T) {
	tcs := []testCase{
		{
			// issue https://github.com/terrastruct/d2/issues/71
			name: "container_child_edge",
			script: `
container.first -> container.second: 1->2
container -> container.second: c->2
`,
		},
		{
			name: "latex",
			script: `a: |latex
\\frac{\\alpha g^2}{\\omega^5} e^{[ -0.74\\bigl\\{\\frac{\\omega U_\\omega 19.5}{g}\\bigr\\}^{\\!-4}\\,]}
|

b: |latex
e = mc^2
|

z: |latex
gibberish\\; math:\\sum_{i=0}^\\infty i^2
|

z -> a
z -> b

a -> c
b -> c
sugar -> c
c: mixed together

c -> solution: we get
`,
		},
	}

	runa(t, tcs)
}
