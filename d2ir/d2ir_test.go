package d2ir_test

import (
	"testing"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/internal/assert"
)

func TestCopy(t *testing.T) {
	t.Parallel()

	const scalStr = `Those who claim the dead never return to life haven't ever been around.`
	s := &d2ir.Scalar{
		parent: nil,
		Value:  d2ast.FlatUnquotedString(scalStr),
	}
	a := &d2ir.Array{
		Parent: nil,
		Values: []d2ir.Value{
			&d2ir.Scalar{
				parent: nil,
				Value: &d2ast.Boolean{
					Value: true,
				},
			},
		},
	}
	m2 := &d2ir.Map{
		Parent: nil,
		Fields: []*d2ir.Field{
			{Primary: s},
		},
	}

	const keyStr = `Absence makes the heart grow frantic.`
	f := &d2ir.Field{
		Parent: nil,
		Name:   keyStr,

		Primary:   s,
		Composite: a,
	}
	e := &d2ir.Edge{
		Parent: nil,

		Primary: s,
		Map:     m2,
	}
	m := &d2ir.Map{
		Parent: nil,

		Fields: []*d2ir.Field{f},
		Edges:  []*d2ir.Edge{e},
	}

	m = m.Copy(nil).(*d2ir.Map)
	f.Name = `Many a wife thinks her husband is the world's greatest lover.`

	assert.Equal(t, m, m.Fields[0].Parent)
	assert.Equal(t, keyStr, m.Fields[0].Name)
	assert.Equal(t, m.Fields[0], m.Fields[0].Primary.parent)
	assert.Equal(t, m.Fields[0], m.Fields[0].Composite.(*d2ir.Array).Parent)

	assert.Equal(t,
		m.Fields[0].Composite,
		m.Fields[0].Composite.(*d2ir.Array).Values[0].(*d2ir.Scalar).parent,
	)

	assert.Equal(t, m, m.Edges[0].Parent)
	assert.Equal(t, m.Edges[0], m.Edges[0].Primary.parent)
	assert.Equal(t, m.Edges[0], m.Edges[0].Map.Parent)

	assert.Equal(t, m.Edges[0].Map, m.Edges[0].Map.Fields[0].Parent)
	assert.Equal(t, m.Edges[0].Map.Fields[0], m.Edges[0].Map.Fields[0].Primary.parent)
}
