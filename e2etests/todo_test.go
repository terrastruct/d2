package e2etests

import (
	_ "embed"
	"testing"
)

func testTodo(t *testing.T) {
	tcs := []testCase{
		// https://github.com/terrastruct/d2/issues/24
		// string monstrosity from not being able to escape backticks within string literals
		{
			skip: true,
			name: "backtick",
			script: `md: |md
  ` + "`" + "code`" + `
|
a -> md -> b
`,
		},
	}

	runa(t, tcs)
}
