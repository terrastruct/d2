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
	assert.Nil(t, err)

	asserts := func(g *d2graph.Graph) {
		a := g.Root.ChildrenArray[0]
		a_a := a.ChildrenArray[0]

		assert.Equal(t, 4, len(g.Objects))
		assert.Equal(t, 1, len(g.Root.ChildrenArray))
		assert.Equal(t, 1, len(a.ChildrenArray))
		assert.Equal(t, 2, len(a_a.ChildrenArray))
		assert.Equal(t, a, a_a.Parent)
		assert.Equal(t, g.Root, a.Parent)

		assert.Contains(t, a.Children, "a")
		assert.Contains(t, a_a.Children, "b")
		assert.Contains(t, a_a.Children, "c")

		assert.Equal(t, 1, len(g.Edges))
		assert.Equal(t, "b", g.Edges[0].Src.ID)
		assert.Equal(t, "c", g.Edges[0].Dst.ID)
	}

	asserts(g)

	b, err := d2graph.SerializeGraph(g)
	assert.Nil(t, err)

	var newG d2graph.Graph
	err = d2graph.DeserializeGraph(b, &newG)
	assert.Nil(t, err)

	asserts(&newG)
}

func TestCasingRegression(t *testing.T) {
	t.Parallel()

	script := `UserCreatedTypeField`

	g, err := d2compiler.Compile("", strings.NewReader(script), nil)
	assert.Nil(t, err)

	_, ok := g.Root.HasChild([]string{"UserCreatedTypeField"})
	assert.True(t, ok)

	b, err := d2graph.SerializeGraph(g)
	assert.Nil(t, err)

	var newG d2graph.Graph
	err = d2graph.DeserializeGraph(b, &newG)
	assert.Nil(t, err)

	_, ok = newG.Root.HasChild([]string{"UserCreatedTypeField"})
	assert.True(t, ok)
}
