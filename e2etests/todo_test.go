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
			script: `hi: |md
Inline math $\frac{1}{2}$
|
`,
		},
	}

	runa(t, tcs)
}
