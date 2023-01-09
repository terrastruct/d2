package d2ir_test

import (
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/diff"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
	"oss.terrastruct.com/d2/internal/assert"
)

type testCase struct {
	name string
	text string
	base *d2ir.Map

	exp func(testing.TB, *d2ir.Map, error)
}

func TestApply(t *testing.T) {
	t.Parallel()

	t.Run("simple", testApplySimple)
}

func testApplySimple(t *testing.T) {
	tcs := []testCase{
		{
			name: "one",
			text: `x`,

			exp: func(t testing.TB, m *d2ir.Map, err error) {
				assert.Success(t, err)
				assertField(t, m, 1, 0, nil)

				assertField(t, m, 0, 0, nil, "x")
			},
		},
		{
			name: "nested",
			text: `x.y -> z.p`,

			exp: func(t testing.TB, m *d2ir.Map, err error) {
				assert.Success(t, err)
				assertField(t, m, 4, 1, nil)

				assertField(t, m, 1, 0, nil, "x")
				assertField(t, m, 0, 0, nil, "x", "y")

				assertField(t, m, 1, 0, nil, "z")
				assertField(t, m, 0, 0, nil, "z", "p")

				assertEdge(t, m, 0, nil, &d2ir.EdgeID{
					[]string{"x", "y"}, false,
					[]string{"z", "p"}, true,
					-1,
				})
			},
		},
		{
			name: "underscore_parent",
			text: `x._ -> z`,

			exp: func(t testing.TB, m *d2ir.Map, err error) {
				assert.Success(t, err)
				assertField(t, m, 2, 1, nil)

				assertField(t, m, 0, 0, nil, "x")
				assertField(t, m, 0, 0, nil, "z")

				assertEdge(t, m, 0, nil, &d2ir.EdgeID{
					[]string{"x"}, false,
					[]string{"z"}, true,
					-1,
				})
			},
		},
	}

	runa(t, tcs)
}

func runa(t *testing.T, tcs []testCase) {
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			run(t, tc)
		})
	}
}

func run(t testing.TB, tc testCase) {
	d2Path := fmt.Sprintf("d2/testdata/d2ir/%v.d2", t.Name())
	ast, err := d2parser.Parse(d2Path, strings.NewReader(tc.text), nil)
	if err != nil {
		tc.exp(t, nil, err)
		t.FailNow()
		return
	}

	dst := tc.base.Copy(nil).(*d2ir.Map)
	err = d2ir.Apply(dst, ast)
	tc.exp(t, dst, err)

	err = diff.Testdata(filepath.Join("..", "testdata", "d2ir", t.Name()), dst)
	if err != nil {
		tc.exp(t, nil, err)
		t.FailNow()
		return
	}
}

func assertField(t testing.TB, n d2ir.Node, nfields, nedges int, primary interface{}, ida ...string) *d2ir.Field {
	t.Helper()

	m := d2ir.NodeToMap(n)
	p := d2ir.NodeToPrimary(n)

	var f *d2ir.Field
	if len(ida) > 0 {
		f = m.Get(ida)
		if f == nil {
			t.Fatalf("expected field %#v in map %#v but not found", ida, m)
		}
		p = f.Primary
		m = d2ir.NodeToMap(f)
	}

	if m.FieldCount() != nfields {
		t.Fatalf("expected %d fields but got %d", nfields, m.FieldCount())
	}
	if m.EdgeCount() != nedges {
		t.Fatalf("expected %d edges but got %d", nedges, m.EdgeCount())
	}
	if !p.Equal(makeScalar(primary)) {
		t.Fatalf("expected primary %#v but %#v", primary, p)
	}

	return f
}

func assertEdge(t testing.TB, n d2ir.Node, nfields int, primary interface{}, eid *d2ir.EdgeID) *d2ir.Edge {
	t.Helper()

	m := d2ir.NodeToMap(n)

	e := m.GetEdge(eid)
	if e == nil {
		t.Fatalf("expected edge %#v in map %#v but not found", eid, m)
	}

	if e.Map.FieldCount() != nfields {
		t.Fatalf("expected %d fields but got %d", nfields, e.Map.FieldCount())
	}
	if e.Map.EdgeCount() != 0 {
		t.Fatalf("expected %d edges but got %d", 0, e.Map.EdgeCount())
	}
	if !e.Primary.Equal(makeScalar(primary)) {
		t.Fatalf("expected primary %#v but %#v", primary, e.Primary)
	}

	return e
}

func makeScalar(v interface{}) *d2ir.Scalar {
	s := &d2ir.Scalar{}
	switch v := v.(type) {
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
