package d2ir_test

import (
	"testing"

	"github.com/d2lang/util-go/assert"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2ir"
)

func TestEdgeIDMatchReversed(t *testing.T) {
	t.Parallel()

	path := func(s string) []d2ast.String {
		return []d2ast.String{d2ast.FlatUnquotedString(s)}
	}
	testCases := []struct {
		name string
		a    *d2ir.EdgeID
		b    *d2ir.EdgeID
		want bool
	}{
		{
			name: "undirected",
			a:    &d2ir.EdgeID{SrcPath: path("a"), DstPath: path("b")},
			b:    &d2ir.EdgeID{SrcPath: path("b"), DstPath: path("a")},
			want: true,
		},
		{
			name: "bidirectional",
			a:    &d2ir.EdgeID{SrcPath: path("a"), SrcArrow: true, DstPath: path("b"), DstArrow: true},
			b:    &d2ir.EdgeID{SrcPath: path("b"), SrcArrow: true, DstPath: path("a"), DstArrow: true},
			want: true,
		},
		{
			name: "directed reversed notation",
			a:    &d2ir.EdgeID{SrcPath: path("a"), DstPath: path("b"), DstArrow: true},
			b:    &d2ir.EdgeID{SrcPath: path("b"), SrcArrow: true, DstPath: path("a")},
			want: true,
		},
		{
			name: "opposite direction",
			a:    &d2ir.EdgeID{SrcPath: path("a"), DstPath: path("b"), DstArrow: true},
			b:    &d2ir.EdgeID{SrcPath: path("b"), DstPath: path("a"), DstArrow: true},
			want: false,
		},
		{
			name: "different arrowheads",
			a:    &d2ir.EdgeID{SrcPath: path("a"), DstPath: path("b")},
			b:    &d2ir.EdgeID{SrcPath: path("b"), SrcArrow: true, DstPath: path("a"), DstArrow: true},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.a.Match(tc.b))
			assert.Equal(t, tc.want, tc.b.Match(tc.a))
		})
	}
}

func TestCopy(t *testing.T) {
	t.Parallel()

	const scalStr = `Those who claim the dead never return to life haven't ever been around.`
	s := &d2ir.Scalar{
		Value: d2ast.FlatUnquotedString(scalStr),
	}
	a := &d2ir.Array{
		Values: []d2ir.Value{
			&d2ir.Scalar{
				Value: &d2ast.Boolean{
					Value: true,
				},
			},
		},
	}
	m2 := &d2ir.Map{
		Fields: []*d2ir.Field{
			{Primary_: s},
		},
	}

	const keyStr = `Absence makes the heart grow frantic.`
	f := &d2ir.Field{
		Name: d2ast.FlatUnquotedString(keyStr),

		Primary_:  s,
		Composite: a,
	}
	e := &d2ir.Edge{
		Primary_: s,
		Map_:     m2,
	}
	m := &d2ir.Map{
		Fields: []*d2ir.Field{f},
		Edges:  []*d2ir.Edge{e},
	}

	m = m.Copy(nil).(*d2ir.Map)
	f.Name = d2ast.FlatUnquotedString(`Many a wife thinks her husband is the world's greatest lover.`)

	assert.Equal(t, m, m.Fields[0].Parent())
	assert.Equal(t, keyStr, m.Fields[0].Name.ScalarString())
	assert.Equal(t, m.Fields[0], m.Fields[0].Primary_.Parent())
	assert.Equal(t, m.Fields[0], m.Fields[0].Composite.(*d2ir.Array).Parent())

	assert.Equal(t,
		m.Fields[0].Composite,
		m.Fields[0].Composite.(*d2ir.Array).Values[0].(*d2ir.Scalar).Parent(),
	)

	assert.Equal(t, m, m.Edges[0].Parent())
	assert.Equal(t, m.Edges[0], m.Edges[0].Primary_.Parent())
	assert.Equal(t, m.Edges[0], m.Edges[0].Map_.Parent())

	assert.Equal(t, m.Edges[0].Map_, m.Edges[0].Map_.Fields[0].Parent())
	assert.Equal(t, m.Edges[0].Map_.Fields[0], m.Edges[0].Map_.Fields[0].Primary_.Parent())
}
