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

complex: |latex
f(x) = \\begin{dcases*}
				x  & when $x$ is even\\\
				-x & when $x$ is odd
				\\end{dcases*}
|

complex -> c
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
