package d2compiler_test

import (
	"strings"
	"testing"

	"github.com/d2lang/util-go/assert"

	"github.com/d2lang/d2/d2compiler"
)

func TestCompileReversedEquivalentEdgeIndexes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		text string
	}{
		{
			name: "undirected",
			text: `a -- b: first
b -- a: second
(a -- b)[1].style.stroke: red
`,
		},
		{
			name: "bidirectional",
			text: `a <-> b: first
b <-> a: second
(a <-> b)[1].style.stroke: red
`,
		},
		{
			name: "directed reversed notation",
			text: `a -> b: first
b <- a: second
(a -> b)[1].style.stroke: red
`,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, _, err := d2compiler.Compile("test.d2", strings.NewReader(tc.text), nil)
			assert.Success(t, err)
			assert.Equal(t, 2, len(g.Edges))
			assert.Equal(t, 0, g.Edges[0].Index)
			assert.Equal(t, 1, g.Edges[1].Index)
			assert.Equal(t, "first", g.Edges[0].Label.Value)
			assert.Equal(t, "second", g.Edges[1].Label.Value)
			assert.Equal(t, "red", g.Edges[1].Style.Stroke.Value)
		})
	}
}
