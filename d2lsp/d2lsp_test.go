package d2lsp_test

import (
	"slices"
	"testing"

	"oss.terrastruct.com/d2/d2ast"
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
	assert.Equal(t, 5, importRanges[0].Start.Column)

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

    layers: {
      y: {
        qwer
      }
    }
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

	ranges, _, err = d2lsp.GetRefRanges("index.d2", fs, []string{"x", "y"}, "qwer")
	assert.Success(t, err)
	assert.Equal(t, 1, len(ranges))

	_, _, err = d2lsp.GetRefRanges("index.d2", fs, []string{"y"}, "hello")
	assert.Equal(t, `board "[y]" not found`, err.Error())
}

func TestGetBoardAtPosition(t *testing.T) {
	tests := []struct {
		name     string
		fs       map[string]string
		path     string
		position d2ast.Position
		want     []string
	}{
		{
			name: "cursor in layer",
			fs: map[string]string{
				"index.d2": `x
layers: {
  basic: {
    x -> y
  }
}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 3, Column: 4},
			want:     []string{"layers", "basic"},
		},
		{
			name: "cursor in nested layer",
			fs: map[string]string{
				"index.d2": `
layers: {
  outer: {
    layers: {
      inner: {
        x -> y
      }
    }
  }
}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 5, Column: 4},
			want:     []string{"layers", "outer", "layers", "inner"},
		},
		{
			name: "cursor in second sibling nested layer",
			fs: map[string]string{
				"index.d2": `
layers: {
	outer: {
		layers: {
			first: {
				a -> b
			}
			second: {
				x -> y
			}
		}
	}
}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 8, Column: 4},
			want:     []string{"layers", "outer", "layers", "second"},
		},
		{
			name: "cursor in root container",
			fs: map[string]string{
				"index.d2": `
wumbo: {
  car
}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 2, Column: 4},
			want:     nil,
		},
		{
			name: "cursor in layer container",
			fs: map[string]string{
				"index.d2": `
layers: {
	x: {
    wumbo: {
      car
    }
  }
}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 4, Column: 4},
			want:     []string{"layers", "x"},
		},
		{
			name: "cursor in scenario",
			fs: map[string]string{
				"index.d2": `
		scenarios: {
			happy: {
				x -> y
			}
		}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 3, Column: 4},
			want:     []string{"scenarios", "happy"},
		},
		{
			name: "cursor in step",
			fs: map[string]string{
				"index.d2": `
		steps: {
			first: {
				x -> y
			}
		}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 3, Column: 4},
			want:     []string{"steps", "first"},
		},
		{
			name: "cursor outside any board",
			fs: map[string]string{
				"index.d2": `
		x -> y
		layers: {
			basic: {
				a -> b
			}
		}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 1, Column: 1},
			want:     nil,
		},
		{
			name: "cursor in empty board",
			fs: map[string]string{
				"index.d2": `
		layers: {
			basic: {
			}
		}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 3, Column: 2},
			want:     []string{"layers", "basic"},
		},
		{
			name: "cursor in between",
			fs: map[string]string{
				"index.d2": `
		layers: {
			basic: {
			}
		}`,
			},
			path:     "index.d2",
			position: d2ast.Position{Line: 2, Column: 2},
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d2lsp.GetBoardAtPosition(tt.fs[tt.path], tt.position)
			assert.Success(t, err)
			if tt.want == nil {
				assert.Equal(t, true, got == nil)
			} else {
				assert.Equal(t, len(tt.want), len(got))
				assert.True(t, slices.Equal(tt.want, got))
			}
		})
	}
}
