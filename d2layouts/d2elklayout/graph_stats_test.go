package d2elklayout

import (
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
)

func TestCollectELKGraphStatsMatchesRepeatedScans(t *testing.T) {
	const input = `
a
b
a -> b
a -> b
a -> a: root loop
container: {
  x
  y
  x -> y
  x -> x: nested loop
  y -> y: second nested loop
}
`
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, edge := range g.Edges {
		edge.LabelDimensions.Width = 20 + i*7
		edge.LabelDimensions.Height = 10 + i*3
	}

	stats := collectELKGraphStats(g)
	for _, obj := range g.Objects {
		var incoming, outgoing float64
		for _, edge := range g.Edges {
			if edge.Src == obj {
				outgoing++
			}
			if edge.Dst == obj {
				incoming++
			}
		}
		if got := stats.degrees[obj]; got.incoming != incoming || got.outgoing != outgoing {
			t.Errorf("object %q degree = (%g,%g), want (%g,%g)", obj.AbsID(), got.incoming, got.outgoing, incoming, outgoing)
		}
	}

	parents := []string{"", "container"}
	for _, parentName := range parents {
		parent := g.Root
		if parentName != "" {
			parent = g.Root.Children[parentName]
		}
		for _, isWidth := range []bool{false, true} {
			want := 0
			for _, child := range parent.Children {
				for _, edge := range g.Edges {
					if edge.Src != edge.Dst || edge.Src != child || edge.Label.Value == "" {
						continue
					}
					value := edge.LabelDimensions.Height
					if isWidth {
						value = edge.LabelDimensions.Width
					}
					if value > want {
						want = value
					}
				}
			}
			if got := stats.maxSelfLoopLabel(parent, isWidth); got != want {
				t.Errorf("parent %q isWidth=%v max self-loop = %d, want %d", parentName, isWidth, got, want)
			}
		}
	}
}
