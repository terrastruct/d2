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

func TestCompile(t *testing.T) {
	t.Parallel()

	t.Run("fields", testCompileFields)
	t.Run("edges", testCompileEdges)
	t.Run("layer", testCompileLayers)
}

type testCase struct {
	name string
	run  func(testing.TB)
}

func runa(t *testing.T, tca []testCase) {
	for _, tc := range tca {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.run(t)
		})
	}
}

func compile(t testing.TB, text string) (*d2ir.Layer, error) {
	t.Helper()

	d2Path := fmt.Sprintf("%v.d2", t.Name())
	ast, err := d2parser.Parse(d2Path, strings.NewReader(text), nil)
	assert.Success(t, err)

	l, err := d2ir.Compile(ast)
	if err != nil {
		return nil, err
	}

	err = diff.TestdataJSON(filepath.Join("..", "testdata", "d2ir", t.Name()), l)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func assertField(t testing.TB, n d2ir.Node, nfields, nedges int, primary interface{}, ida ...string) *d2ir.Field {
	t.Helper()

	m := d2ir.ToMap(n)
	if m == nil {
		t.Fatalf("nil m from %T", n)
	}
	p := d2ir.ToScalar(n)

	var f *d2ir.Field
	if len(ida) > 0 {
		f = m.GetField(ida...)
		if f == nil {
			t.Fatalf("expected field %v in map %s", ida, m)
		}
		p = f.Primary
		m = d2ir.ToMap(f)
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

	m := d2ir.ToMap(n)
	if m == nil {
		t.Fatalf("nil m from %T", n)
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

func testCompileFields(t *testing.T) {
	t.Parallel()
	t.Run("primary", testCompileFieldPrimary)
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				l, err := compile(t, `x`)
				assert.Success(t, err)
				assertField(t, l, 1, 0, nil)

				assertField(t, l, 0, 0, nil, "x")
			},
		},
		{
			name: "label",
			run: func(t testing.TB) {
				l, err := compile(t, `x: yes`)
				assert.Success(t, err)
				assertField(t, l, 1, 0, nil)

				assertField(t, l, 0, 0, "yes", "x")
			},
		},
		{
			name: "nested",
			run: func(t testing.TB) {
				l, err := compile(t, `x.y: yes`)
				assert.Success(t, err)
				assertField(t, l, 2, 0, nil)

				assertField(t, l, 1, 0, nil, "x")
				assertField(t, l, 0, 0, "yes", "x", "y")
			},
		},
		{
			name: "array",
			run: func(t testing.TB) {
				l, err := compile(t, `x: [1;2;3;4]`)
				assert.Success(t, err)
				assertField(t, l, 1, 0, nil)

				f := assertField(t, l, 0, 0, nil, "x")
				assert.String(t, `[1; 2; 3; 4]`, f.Composite.String())
			},
		},
	}
	runa(t, tca)
}

func testCompileFieldPrimary(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				l, err := compile(t, `x: yes { pqrs }`)
				assert.Success(t, err)
				assertField(t, l, 2, 0, nil)

				assertField(t, l, 1, 0, "yes", "x")
				assertField(t, l, 0, 0, nil, "x", "pqrs")
			},
		},
		{
			name: "nested",
			run: func(t testing.TB) {
				l, err := compile(t, `x.y: yes { pqrs }`)
				assert.Success(t, err)
				assertField(t, l, 3, 0, nil)

				assertField(t, l, 2, 0, nil, "x")
				assertField(t, l, 1, 0, "yes", "x", "y")
				assertField(t, l, 0, 0, nil, "x", "y", "pqrs")
			},
		},
	}
	runa(t, tca)
}

func testCompileEdges(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				l, err := compile(t, `x -> y`)
				assert.Success(t, err)
				assertField(t, l, 2, 1, nil)
				assertEdge(t, l, 0, nil, `(x -> y)[0]`)

				assertField(t, l, 0, 0, nil, "x")
				assertField(t, l, 0, 0, nil, "y")
			},
		},
		{
			name: "nested",
			run: func(t testing.TB) {
				l, err := compile(t, `x.y -> z.p`)
				assert.Success(t, err)
				assertField(t, l, 4, 1, nil)

				assertField(t, l, 1, 0, nil, "x")
				assertField(t, l, 0, 0, nil, "x", "y")

				assertField(t, l, 1, 0, nil, "z")
				assertField(t, l, 0, 0, nil, "z", "p")

				assertEdge(t, l, 0, nil, "(x.y -> z.p)[0]")
			},
		},
		{
			name: "underscore",
			run: func(t testing.TB) {
				l, err := compile(t, `p: { _.x -> z }`)
				assert.Success(t, err)
				assertField(t, l, 3, 1, nil)

				assertField(t, l, 0, 0, nil, "x")
				assertField(t, l, 1, 0, nil, "p")

				assertEdge(t, l, 0, nil, "(x -> p.z)[0]")
			},
		},
	}
	runa(t, tca)
}

func testCompileLayers(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				l, err := compile(t, `x -> y
layers: {
	bingo: { p.q.z }
}`)
				assert.Success(t, err)

				assertField(t, l, 5, 1, nil)
				assertEdge(t, l, 0, nil, `(x -> y)[0]`)

				assertField(t, l, 0, 0, nil, "x")
				assertField(t, l, 0, 0, nil, "y")

				assertField(t, l, 0, 0, nil, "layers", "bingo")
			},
		},
	}
	runa(t, tca)
}
