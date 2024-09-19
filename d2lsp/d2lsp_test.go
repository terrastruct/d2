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
		"index": script,
	}
	refs, err := d2lsp.GetFieldRefs("", fs, "x")
	assert.Success(t, err)
	assert.Equal(t, 3, len(refs))
	assert.Equal(t, 0, refs[0].AST().GetRange().Start.Line)
	assert.Equal(t, 1, refs[1].AST().GetRange().Start.Line)
	assert.Equal(t, 3, refs[2].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetFieldRefs("", fs, "a.x")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 2, refs[0].AST().GetRange().Start.Line)
}
