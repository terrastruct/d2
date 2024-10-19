package d2lsp_test

import (
	"testing"

	"oss.terrastruct.com/d2/d2lsp"
	"oss.terrastruct.com/util-go/assert"
)

func TestGetFieldRanges(t *testing.T) {
	script := `x
x.a
a.x
x -> y`
	fs := map[string]string{
		"index.d2": script,
	}
	ranges, _, err := d2lsp.GetRefRanges("index.d2", fs, nil, "x")
	assert.Success(t, err)
	assert.Equal(t, 3, len(ranges))
	assert.Equal(t, 0, ranges[0].Start.Line)
	assert.Equal(t, 1, ranges[1].Start.Line)
	assert.Equal(t, 3, ranges[2].Start.Line)

	ranges, _, err = d2lsp.GetRefRanges("index.d2", fs, nil, "a.x")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 2, ranges[0].Start.Line)
}

func TestGetEdgeRanges(t *testing.T) {
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
	ranges, _, err := d2lsp.GetRefRanges("index.d2", fs, nil, "x -> y")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 3, ranges[0].Start.Line)

	ranges, _, err = d2lsp.GetRefRanges("index.d2", fs, nil, "y -> z")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 4, ranges[0].Start.Line)

	ranges, _, err = d2lsp.GetRefRanges("index.d2", fs, nil, "x -> z")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 5, ranges[0].Start.Line)

	ranges, _, err = d2lsp.GetRefRanges("index.d2", fs, nil, "a -> b")
	assert.Success(t, err)
	assert.Equal(t, 0, len(ranges))

	ranges, _, err = d2lsp.GetRefRanges("index.d2", fs, nil, "b.(x -> y)")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 7, ranges[0].Start.Line)
}

func TestGetRangesImported(t *testing.T) {
	fs := map[string]string{
		"index.d2": `
...@ok
hi
hey: @ok
`,
		"ok.d2": `
what
lala
okay
`,
	}
	ranges, importRanges, err := d2lsp.GetRefRanges("index.d2", fs, nil, "hi")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 2, ranges[0].Start.Line)
	assert.Equal(t, 0, len(importRanges))

	ranges, importRanges, err = d2lsp.GetRefRanges("index.d2", fs, nil, "okay")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, "ok.d2", ranges[0].Path)
	assert.Equal(t, 1, len(importRanges))
	assert.Equal(t, 1, importRanges[0].Start.Line)

	ranges, importRanges, err = d2lsp.GetRefRanges("index.d2", fs, nil, "hey.okay")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, "ok.d2", ranges[0].Path)
	assert.Equal(t, 1, len(importRanges))
	assert.Equal(t, 3, importRanges[0].Start.Line)

	ranges, _, err = d2lsp.GetRefRanges("ok.d2", fs, nil, "hi")
	assert.Success(t, err)
	assert.Equal(t, 0, len(ranges))

	ranges, _, err = d2lsp.GetRefRanges("ok.d2", fs, nil, "okay")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
}

func TestGetRangesBoards(t *testing.T) {
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
	ranges, _, err := d2lsp.GetRefRanges("index.d2", fs, []string{"x"}, "hello")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))
	assert.Equal(t, 4, ranges[0].Start.Line)

	ranges, _, err = d2lsp.GetRefRanges("index.d2", fs, []string{"x"}, "hi")
	assert.Success(t, err)
	assert.Equal(t, 0, len(ranges))

	_, _, err = d2lsp.GetRefRanges("index.d2", fs, []string{"y"}, "hello")
	assert.Equal(t, `board "[y]" not found`, err.Error())
}
