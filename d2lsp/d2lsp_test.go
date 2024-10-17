package d2lsp_test

import (
	"testing"

	"oss.terrastruct.com/d2/d2lsp"
	"oss.terrastruct.com/util-go/assert"
)

func TestGetFieldRefs(t *testing.T) {
	script := `x
x.a
a.x
x -> y`
	fs := map[string]string{
		"index.d2": script,
	}
	refs, err := d2lsp.GetRefs("index.d2", fs, nil, "x")
	assert.Success(t, err)
	assert.Equal(t, 3, len(refs))
	assert.Equal(t, 0, refs[0].AST().GetRange().Start.Line)
	assert.Equal(t, 1, refs[1].AST().GetRange().Start.Line)
	assert.Equal(t, 3, refs[2].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetRefs("index.d2", fs, nil, "a.x")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 2, refs[0].AST().GetRange().Start.Line)
}

func TestGetEdgeRefs(t *testing.T) {
	script := `x
x.a
a.x
x -> y
y -> z
x -> z
b: {
  x -> y
}
`
	fs := map[string]string{
		"index.d2": script,
	}
	refs, err := d2lsp.GetRefs("index.d2", fs, nil, "x -> y")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 3, refs[0].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetRefs("index.d2", fs, nil, "y -> z")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 4, refs[0].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetRefs("index.d2", fs, nil, "x -> z")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 5, refs[0].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetRefs("index.d2", fs, nil, "a -> b")
	assert.Success(t, err)
	assert.Equal(t, 0, len(refs))

	refs, err = d2lsp.GetRefs("index.d2", fs, nil, "b.(x -> y)")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 7, refs[0].AST().GetRange().Start.Line)
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
	refs, err := d2lsp.GetRefs("index.d2", fs, nil, "hi")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 2, refs[0].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetRefs("index.d2", fs, nil, "okay")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, "ok.d2", refs[0].AST().GetRange().Path)

	refs, err = d2lsp.GetRefs("ok.d2", fs, nil, "hi")
	assert.Success(t, err)
	assert.Equal(t, 0, len(refs))

	refs, err = d2lsp.GetRefs("ok.d2", fs, nil, "okay")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
}

func TestGetRefsBoards(t *testing.T) {
	fs := map[string]string{
		"index.d2": `
hi
layers: {
  x: {
    hello
  }
}
`,
	}
	refs, err := d2lsp.GetRefs("index.d2", fs, []string{"x"}, "hello")
	assert.Success(t, err)
	assert.Equal(t, 1, len(refs))
	assert.Equal(t, 4, refs[0].AST().GetRange().Start.Line)

	refs, err = d2lsp.GetRefs("index.d2", fs, []string{"x"}, "hi")
	assert.Success(t, err)
	assert.Equal(t, 0, len(refs))

	_, err = d2lsp.GetRefs("index.d2", fs, []string{"y"}, "hello")
	assert.Equal(t, `board "[y]" not found`, err.Error())
}

func TestGetImportRanges(t *testing.T) {
	fs := map[string]string{
		"yes/index.d2": `
...@../fast/ok
hi
hey: {
  ...@pok
}
`,
		"fast/ok.d2": `
okay
`,
		"yes/pok.d2": `
des
`,
	}
	ranges, err := d2lsp.GetImportRanges("yes/index.d2", fs, "fast/ok.d2")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 1, ranges[0].Start.Line)

	ranges, err = d2lsp.GetImportRanges("yes/index.d2", fs, "yes/pok.d2")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 4, ranges[0].Start.Line)
}
