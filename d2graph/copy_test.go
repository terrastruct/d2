package d2graph_test

import (
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
)

func TestCopy(t *testing.T) {
	t.Parallel()

	tca := []struct {
		name   string
		d2str  string
		assert func(t *testing.T, g, g2 *d2graph.Graph)
	}{
		{
			name: `objects`,
			d2str: `a
b
c
d`,
			assert: func(t *testing.T, g, g2 *d2graph.Graph) {
				g2.Root.IDVal = `jingleberry`
				assert.String(t, ``, g.Root.IDVal)
				g2.Root.ChildrenArray[0].IDVal = `saltedmeat`
				assert.String(t, `a`, g.Root.ChildrenArray[0].IDVal)
				assert.NotEqual(t,
					g.Root.ChildrenArray[0].References[0].ScopeObj,
					g2.Root.ChildrenArray[0].References[0].ScopeObj,
				)
			},
		},
		{
			name: `edges`,
			d2str: `a -> b
b -> c
c -> d
d -> a`,
			assert: func(t *testing.T, g, g2 *d2graph.Graph) {
				g2.Edges[0].DstArrow = false
				assert.Equal(t, true, g.Edges[0].DstArrow)
				assert.NotEqual(t,
					g.Edges[0].References[0].ScopeObj,
					g2.Edges[0].References[0].ScopeObj,
				)
			},
		},
		{
			name:  `nested`,
			d2str: `a.b -> c.d`,
			assert: func(t *testing.T, g, g2 *d2graph.Graph) {
				g2.Root.ChildrenArray[0].ChildrenArray[0].IDVal = `saltedmeat`
				assert.String(t, `b`, g.Root.ChildrenArray[0].ChildrenArray[0].IDVal)
				assert.NotEqual(t,
					g.Root.ChildrenArray[0].ChildrenArray[0].References[0].ScopeObj,
					g2.Root.ChildrenArray[0].ChildrenArray[0].References[0].ScopeObj,
				)

				g2.Edges[0].DstArrow = false
				assert.Equal(t, true, g.Edges[0].DstArrow)
				assert.NotEqual(t,
					g.Edges[0].References[0].ScopeObj,
					g2.Edges[0].References[0].ScopeObj,
				)
			},
		},
	}

	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g, err := d2compiler.Compile("", strings.NewReader(tc.d2str), nil)
			assert.Success(t, err)

			g2 := g.Copy()
			tc.assert(t, g, g2)
		})
	}
}
