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
			name: "md_code_inline",
			script: `md: |md
` + "`code`" + `
|
a -> md -> b
`,
		},
		{
			name: "md_code_block_fenced",
			script: `md: |md
` + "```" + `
{
	fenced: "block",
	of: "json",
}
` + "```" + `
|
a -> md -> b
`,
		},
		{
			name: "md_code_block_indented",
			script: `md: |md
a line of text and an

	{
		indented: "block",
		of: "json",
	}

|
a -> md -> b
`,
		},
	}

	runa(t, tcs)
}
