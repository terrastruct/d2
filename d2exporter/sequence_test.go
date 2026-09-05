package d2exporter_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/textmeasure"
)

func TestSequenceV2RepeatedActorAppearance(t *testing.T) {
	t.Parallel()
	for _, theme := range []int64{0, 303} {
		ruler, err := textmeasure.NewRuler()
		require.NoError(t, err)
		source := `shape: sequence-diagram
vars: {mirror: true}
a: Person {shape: person}
group: {shape: actor-group; b: Service}
a -> group.b
 a.note: Note {shape: page}
a.again: {shape: actor}
group.b -> a
`
		opts := &d2lib.CompileOptions{Ruler: ruler, LayoutResolver: func(string) (d2graph.LayoutGraph, error) { return d2dagrelayout.DefaultLayout, nil }}
		d, g, err := d2lib.Compile(context.Background(), source, opts, &d2svg.RenderOpts{ThemeID: &theme})
		require.NoError(t, err)
		byID := make(map[string]d2target.Shape)
		for _, s := range d.Shapes {
			byID[s.ID] = s
		}
		count := 0
		for _, obj := range g.Objects {
			if !obj.IsSequenceDiagramActorRepeat() {
				continue
			}
			count++
			repeated, ok := byID[obj.AbsID()]
			require.True(t, ok)
			original := byID[obj.SequenceDiagramActor().AbsID()]
			repeated.ID = original.ID
			repeated.Pos = original.Pos
			repeated.Level = original.Level
			repeated.ZIndex = original.ZIndex
			require.Equal(t, original, repeated, "theme %d repeat %s", theme, obj.AbsID())
		}
		require.Equal(t, 3, count)
		plain, _, err := d2lib.Compile(context.Background(), strings.Replace(source, "mirror: true", "mirror: false", 1), opts, &d2svg.RenderOpts{ThemeID: &theme})
		require.NoError(t, err)
		for _, original := range plain.Shapes {
			if original.ID != "group.b" {
				continue
			}
			mirrored := byID[original.ID]
			mirrored.Pos = original.Pos
			require.Equal(t, original, mirrored, "mirror must not change the original actor in theme %d", theme)
		}

	}
}
