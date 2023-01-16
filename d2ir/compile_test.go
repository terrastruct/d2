package d2ir_test

import (
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
)

type testCase struct {
	name string
	run  func(testing.TB, *d2ir.Map)
}

func TestCompile(t *testing.T) {
	t.Parallel()

	t.Run("roots", testCompileRoots)
}

func testCompileRoots(t *testing.T) {
	t.Parallel()

	tca := []testCase{
		{
			name: "field",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x`)
				assert.Success(t, err)
				assertField(t, m, 1, 0, nil)

				assertField(t, m, 0, 0, nil, "x")
			},
		},
		{
			name: "field/label",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x: yes`)
				assert.Success(t, err)
				assertField(t, m, 1, 0, nil)

				assertField(t, m, 0, 0, "yes", "x")
			},
		},
		{
			name: "field/label/nested",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x.y: yes`)
				assert.Success(t, err)
				assertField(t, m, 2, 0, nil)

				assertField(t, m, 1, 0, nil, "x")
				assertField(t, m, 0, 0, "yes", "x", "y")
			},
		},
		{
			name: "primary",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x: yes { pqrs }`)
				assert.Success(t, err)
				assertField(t, m, 2, 0, nil)

				assertField(t, m, 1, 0, "yes", "x")
				assertField(t, m, 0, 0, nil, "x", "pqrs")
			},
		},
		{
			name: "primary/nested",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x.y: yes { pqrs }`)
				assert.Success(t, err)
				assertField(t, m, 3, 0, nil)

				assertField(t, m, 2, 0, nil, "x")
				assertField(t, m, 1, 0, "yes", "x", "y")
				assertField(t, m, 0, 0, nil, "x", "y", "pqrs")
			},
		},
		{
			name: "edge",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x -> y`)
				assert.Success(t, err)
				assertField(t, m, 2, 1, nil)
				assertEdge(t, m, 0, nil, `(x -> y)[0]`)

				assertField(t, m, 0, 0, nil, "x")
				assertField(t, m, 0, 0, nil, "y")
			},
		},
		{
			name: "nested",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x.y -> z.p`)
				assert.Success(t, err)
				assertField(t, m, 4, 1, nil)

				assertField(t, m, 1, 0, nil, "x")
				assertField(t, m, 0, 0, nil, "x", "y")

				assertField(t, m, 1, 0, nil, "z")
				assertField(t, m, 0, 0, nil, "z", "p")

				assertEdge(t, m, 0, nil, "(x.y -> z.p)[0]")
			},
		},
		{
			name: "underscore_parent",
			run: func(t testing.TB, m *d2ir.Map) {
				err := parse(t, m, `x._ -> z`)
				assert.Success(t, err)
				assertField(t, m, 3, 1, nil)

				assertField(t, m, 0, 0, nil, "x")
				assertField(t, m, 0, 0, nil, "z")

				assertEdge(t, m, 0, nil, "(x -> z)[0]")
			},
		},
	}

	runa(t, tca)
}

func runa(t *testing.T, tca []testCase) {
	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &d2ir.Map{}
			tc.run(t, m)
		})
	}
}

func parse(t testing.TB, dst *d2ir.Map, text string) error {
	t.Helper()

	d2Path := fmt.Sprintf("d2/testdata/d2ir/%v.d2", t.Name())
	ast, err := d2parser.Parse(d2Path, strings.NewReader(text), nil)
	assert.Success(t, err)

	err = d2ir.Compile(dst, ast)
	if err != nil {
		return err
	}

	err = diff.TestdataJSON(filepath.Join("..", "testdata", "d2ir", t.Name()), dst)
	return err
}

func assertField(t testing.TB, n d2ir.Node, nfields, nedges int, primary interface{}, ida ...string) *d2ir.Field {
	t.Helper()

	var m *d2ir.Map
	p := &d2ir.Scalar{
		Value: &d2ast.Null{},
	}
	switch n := n.(type) {
	case *d2ir.Field:
		mm, ok := n.Composite.(*d2ir.Map)
		if ok {
			m = mm
		} else {
			t.Fatalf("unexpected d2ir.Field.Composite %T", n.Composite)
		}
		p = n.Primary
	case *d2ir.Map:
		m = n
		p.Value = &d2ast.Null{}
	default:
		t.Fatalf("unexpected d2ir.Node %T", n)
	}

	var f *d2ir.Field
	if len(ida) > 0 {
		f = m.Get(ida)
		if f == nil {
			t.Fatalf("expected field %v in map %s", ida, m)
		}
		p = f.Primary
		if f_m, ok := f.Composite.(*d2ir.Map); ok {
			m = f_m
		} else {
			m = &d2ir.Map{}
		}
	}

	assert.Equal(t, nfields, m.FieldCountRecursive())
	assert.Equal(t, nedges, m.EdgeCountRecursive())
	if !makeScalar(p).Equal(makeScalar(primary)) {
		t.Fatalf("expected primary %#v but got %s", primary, p)
	}

	return f
}

func assertEdge(t testing.TB, n d2ir.Node, nfields int, primary interface{}, eids string) *d2ir.Edge {
	t.Helper()

	k, err := d2parser.ParseMapKey(eids)
	assert.Success(t, err)

	eid := d2ir.NewEdgeIDs(k)[0]

	var m *d2ir.Map
	switch n := n.(type) {
	case *d2ir.Field:
		mm, ok := n.Composite.(*d2ir.Map)
		if ok {
			m = mm
		} else {
			t.Fatalf("unexpected d2ir.Field.Composite %T", n.Composite)
		}
	case *d2ir.Map:
		m = n
	default:
		t.Fatalf("unexpected d2ir.Node %T", n)
	}

	ea := m.GetEdges(eid)
	if len(ea) != 1 {
		t.Fatalf("expected single edge %v in map %s but not found", eid, m)
	}
	e := ea[0]

	assert.Equal(t, nfields, e.Map.FieldCountRecursive())
	if !makeScalar(e.Primary).Equal(makeScalar(primary)) {
		t.Fatalf("expected primary %#v but %s", primary, e.Primary)
	}

	return e
}

func makeScalar(v interface{}) *d2ir.Scalar {
	s := &d2ir.Scalar{}
	switch v := v.(type) {
	case *d2ir.Scalar:
		if v == nil {
			s.Value = &d2ast.Null{}
			return s
		}
		return v
	case bool:
		s.Value = &d2ast.Boolean{
			Value: v,
		}
	case float64:
		bv := &big.Rat{}
		bv.SetFloat64(v)
		s.Value = &d2ast.Number{
			Value: bv,
		}
	case int:
		s.Value = &d2ast.Number{
			Value: big.NewRat(int64(v), 1),
		}
	case string:
		s.Value = d2ast.FlatDoubleQuotedString(v)
	default:
		if v != nil {
			panic(fmt.Sprintf("d2ir: unexpected type to makeScalar: %#v", v))
		}
		s.Value = &d2ast.Null{}
	}
	return s
}
