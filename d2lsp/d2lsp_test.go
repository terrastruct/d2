package d2lsp_test

import (
	"testing"

	"oss.terrastruct.com/d2/d2lsp"
	"oss.terrastruct.com/util-go/assert"
)

func TestGetRefs(t *testing.T) {
	script := `x
x.a
a.x
x -> y`
	fs := map[string]string{
		"index.d2": script,
	}
	refs, err := d2lsp.GetFieldRefs("", "index.d2", fs, "x")
	assert.Success(t, err)
	assert.Equal(t, 3, len(refs))
	assert.Equal(t, 0, refs[0].AST().GetRange().Start.Line)
	assert.Equal(t, 1, refs[1].AST().GetRange().Start.Line)
	assert.Equal(t, 3, refs[2].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetFieldRefs("", "index.d2", fs, "a.x")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 2, refs[0].AST().GetRange().Start.Line)
}

func TestGetRefsImported(t *testing.T) {
	fs := map[string]string{
		"index.d2": `
...@ok
hi
`,
		"ok.d2": `
okay
`,
	}
	refs, err := d2lsp.GetFieldRefs("", "index.d2", fs, "hi")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 2, refs[0].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetFieldRefs("", "index.d2", fs, "okay")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, "ok.d2", refs[0].AST().GetRange().Path)

	refs, err = d2lsp.GetFieldRefs("", "ok.d2", fs, "hi")
	assert.Success(t, err)
	assert.Equal(t, 0, len(refs))

	refs, err = d2lsp.GetFieldRefs("", "ok.d2", fs, "okay")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
}
