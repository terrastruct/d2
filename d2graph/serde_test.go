package d2graph_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
)

func TestSerialization(t *testing.T) {
	t.Parallel()

	g, err := d2compiler.Compile("", strings.NewReader("a.a.b -> a.a.c"), nil)
	if err != nil {
		t.Fatal(err)
	}

	asserts := func(g *d2graph.Graph) {
		assert.Equal(t, 4, len(g.Objects))
		assert.Equal(t, 1, len(g.Root.ChildrenArray))
		assert.Equal(t, 1, len(g.Root.ChildrenArray[0].ChildrenArray))
		assert.Equal(t, 2, len(g.Root.ChildrenArray[0].ChildrenArray[0].ChildrenArray))
		assert.Equal(t,
			g.Root.ChildrenArray[0],
			g.Root.ChildrenArray[0].ChildrenArray[0].Parent,
		)

		assert.Equal(t,
			g.Root,
			g.Root.ChildrenArray[0].Parent,
		)

		assert.Equal(t, 1, len(g.Edges))
		assert.Equal(t, "b", g.Edges[0].Src.ID)
		assert.Equal(t, "c", g.Edges[0].Dst.ID)
	}

	asserts(g)

	b, err := d2graph.SerializeGraph(g)
	if err != nil {
		t.Fatal(err)
	}

	var newG d2graph.Graph
	err = d2graph.DeserializeGraph(b, &newG)
	if err != nil {
		t.Fatal(err)
	}

	asserts(&newG)
}
