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
	t.Run("layers", testCompileLayers)
	t.Run("scenarios", testCompileScenarios)
	t.Run("steps", testCompileSteps)
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

func compile(t testing.TB, text string) (*d2ir.Map, error) {
	t.Helper()

	d2Path := fmt.Sprintf("%v.d2", t.Name())
	ast, err := d2parser.Parse(d2Path, strings.NewReader(text), nil)
	assert.Success(t, err)

	m, err := d2ir.Compile(ast)
	if err != nil {
		return nil, err
	}

	err = diff.TestdataJSON(filepath.Join("..", "testdata", "d2ir", t.Name()), m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func assertQuery(t testing.TB, n d2ir.Node, nfields, nedges int, primary interface{}, idStr string) d2ir.Node {
	t.Helper()

	m := n.Map()
	p := n.Primary()

	if idStr != "" {
		var err error
		n, err = m.Query(idStr)
		assert.Success(t, err)
		assert.NotEqual(t, n, nil)

		p = n.Primary()
		m = n.Map()
	}

	assert.Equal(t, nfields, m.FieldCountRecursive())
	assert.Equal(t, nedges, m.EdgeCountRecursive())
	if !makeScalar(p).Equal(makeScalar(primary)) {
		t.Fatalf("expected primary %#v but got %s", primary, p)
	}

	return n
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
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				m, err := compile(t, `x`)
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "")

				assertQuery(t, m, 0, 0, nil, "x")
			},
		},
		{
			name: "label",
			run: func(t testing.TB) {
				m, err := compile(t, `x: yes`)
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "")

				assertQuery(t, m, 0, 0, "yes", "x")
			},
		},
		{
			name: "nested",
			run: func(t testing.TB) {
				m, err := compile(t, `x.y: yes`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")

				assertQuery(t, m, 1, 0, nil, "x")
				assertQuery(t, m, 0, 0, "yes", "x.y")
			},
		},
		{
			name: "array",
			run: func(t testing.TB) {
				m, err := compile(t, `x: [1;2;3;4]`)
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "")

				f := assertQuery(t, m, 0, 0, nil, "x").(*d2ir.Field)
				assert.String(t, `[1; 2; 3; 4]`, f.Composite.String())
			},
		},
		{
			name: "null",
			run: func(t testing.TB) {
				m, err := compile(t, `pq: pq
pq: null`)
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "")
				// null doesn't delete pq from *Map so that for language tooling
				// we maintain the references.
				// Instead d2compiler will ensure it doesn't get rendered.
				assertQuery(t, m, 0, 0, nil, "pq")
			},
		},
	}
	runa(t, tca)
	t.Run("primary", func(t *testing.T) {
		t.Parallel()
		tca := []testCase{
			{
				name: "root",
				run: func(t testing.TB) {
					m, err := compile(t, `x: yes { pqrs }`)
					assert.Success(t, err)
					assertQuery(t, m, 2, 0, nil, "")

					assertQuery(t, m, 1, 0, "yes", "x")
					assertQuery(t, m, 0, 0, nil, "x.pqrs")
				},
			},
			{
				name: "nested",
				run: func(t testing.TB) {
					m, err := compile(t, `x.y: yes { pqrs }`)
					assert.Success(t, err)
					assertQuery(t, m, 3, 0, nil, "")

					assertQuery(t, m, 2, 0, nil, "x")
					assertQuery(t, m, 1, 0, "yes", "x.y")
					assertQuery(t, m, 0, 0, nil, "x.y.pqrs")
				},
			},
		}
		runa(t, tca)
	})
}

func testCompileEdges(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				m, err := compile(t, `x -> y`)
				assert.Success(t, err)
				assertQuery(t, m, 2, 1, nil, "")
				assertQuery(t, m, 0, 0, nil, `(x -> y)[0]`)

				assertQuery(t, m, 0, 0, nil, "x")
				assertQuery(t, m, 0, 0, nil, "y")
			},
		},
		{
			name: "nested",
			run: func(t testing.TB) {
				m, err := compile(t, `x.y -> z.p`)
				assert.Success(t, err)
				assertQuery(t, m, 4, 1, nil, "")

				assertQuery(t, m, 1, 0, nil, "x")
				assertQuery(t, m, 0, 0, nil, "x.y")

				assertQuery(t, m, 1, 0, nil, "z")
				assertQuery(t, m, 0, 0, nil, "z.p")

				assertQuery(t, m, 0, 0, nil, "(x.y -> z.p)[0]")
			},
		},
		{
			name: "underscore",
			run: func(t testing.TB) {
				m, err := compile(t, `p: { _.x -> z }`)
				assert.Success(t, err)
				assertQuery(t, m, 3, 1, nil, "")

				assertQuery(t, m, 0, 0, nil, "x")
				assertQuery(t, m, 1, 0, nil, "p")

				assertQuery(t, m, 0, 0, nil, "(x -> p.z)[0]")
			},
		},
		{
			name: "chain",
			run: func(t testing.TB) {
				m, err := compile(t, `a -> b -> c -> d`)
				assert.Success(t, err)
				assertQuery(t, m, 4, 3, nil, "")

				assertQuery(t, m, 0, 0, nil, "a")
				assertQuery(t, m, 0, 0, nil, "b")
				assertQuery(t, m, 0, 0, nil, "c")
				assertQuery(t, m, 0, 0, nil, "d")
				assertQuery(t, m, 0, 0, nil, "(a -> b)[0]")
				assertQuery(t, m, 0, 0, nil, "(b -> c)[0]")
				assertQuery(t, m, 0, 0, nil, "(c -> d)[0]")
			},
		},
	}
	runa(t, tca)
	t.Run("errs", func(t *testing.T) {
		t.Parallel()
		tca := []testCase{
			{
				name: "bad_edge",
				run: func(t testing.TB) {
					_, err := compile(t, `(x -> y): { p -> q }`)
					assert.ErrorString(t, err, `TestCompile/edges/errs/bad_edge.d2:1:13: cannot create edge inside edge`)
				},
			},
		}
		runa(t, tca)
	})
}

func testCompileLayers(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				m, err := compile(t, `x -> y
layers: {
	bingo: { p.q.z }
}`)
				assert.Success(t, err)

				assertQuery(t, m, 7, 1, nil, "")
				assertQuery(t, m, 0, 0, nil, `(x -> y)[0]`)

				assertQuery(t, m, 0, 0, nil, "x")
				assertQuery(t, m, 0, 0, nil, "y")

				assertQuery(t, m, 3, 0, nil, "layers.bingo")
			},
		},
	}
	runa(t, tca)
	t.Run("errs", func(t *testing.T) {
		t.Parallel()
		tca := []testCase{
			{
				name: "1/bad_edge",
				run: func(t testing.TB) {
					_, err := compile(t, `layers.x -> layers.y`)
					assert.ErrorString(t, err, `TestCompile/layers/errs/1/bad_edge.d2:1:1: cannot create edges between boards`)
				},
			},
			{
				name: "2/bad_edge",
				run: func(t testing.TB) {
					_, err := compile(t, `layers -> scenarios`)
					assert.ErrorString(t, err, `TestCompile/layers/errs/2/bad_edge.d2:1:1: cannot create edges between boards`)
				},
			},
			{
				name: "3/bad_edge",
				run: func(t testing.TB) {
					_, err := compile(t, `layers.x.y -> steps.z.p`)
					assert.ErrorString(t, err, `TestCompile/layers/errs/3/bad_edge.d2:1:1: cannot create edges between boards`)
				},
			},
			{
				name: "4/good_edge",
				run: func(t testing.TB) {
					_, err := compile(t, `layers.x.y -> layers.x.y`)
					assert.Success(t, err)
				},
			},
		}
		runa(t, tca)
	})
}

func testCompileScenarios(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				m, err := compile(t, `x -> y
scenarios: {
	bingo: { p.q.z }
	nuclear: { quiche }
}`)
				assert.Success(t, err)

				assertQuery(t, m, 13, 3, nil, "")

				assertQuery(t, m, 0, 0, nil, "x")
				assertQuery(t, m, 0, 0, nil, "y")
				assertQuery(t, m, 0, 0, nil, `(x -> y)[0]`)

				assertQuery(t, m, 5, 1, nil, "scenarios.bingo")
				assertQuery(t, m, 0, 0, nil, "scenarios.bingo.x")
				assertQuery(t, m, 0, 0, nil, "scenarios.bingo.y")
				assertQuery(t, m, 0, 0, nil, `scenarios.bingo.(x -> y)[0]`)
				assertQuery(t, m, 2, 0, nil, "scenarios.bingo.p")
				assertQuery(t, m, 1, 0, nil, "scenarios.bingo.p.q")
				assertQuery(t, m, 0, 0, nil, "scenarios.bingo.p.q.z")

				assertQuery(t, m, 3, 1, nil, "scenarios.nuclear")
				assertQuery(t, m, 0, 0, nil, "scenarios.nuclear.x")
				assertQuery(t, m, 0, 0, nil, "scenarios.nuclear.y")
				assertQuery(t, m, 0, 0, nil, `scenarios.nuclear.(x -> y)[0]`)
				assertQuery(t, m, 0, 0, nil, "scenarios.nuclear.quiche")
			},
		},
	}
	runa(t, tca)
}

func testCompileSteps(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "root",
			run: func(t testing.TB) {
				m, err := compile(t, `x -> y
steps: {
	bingo: { p.q.z }
	nuclear: { quiche }
}`)
				assert.Success(t, err)

				assertQuery(t, m, 16, 3, nil, "")

				assertQuery(t, m, 0, 0, nil, "x")
				assertQuery(t, m, 0, 0, nil, "y")
				assertQuery(t, m, 0, 0, nil, `(x -> y)[0]`)

				assertQuery(t, m, 5, 1, nil, "steps.bingo")
				assertQuery(t, m, 0, 0, nil, "steps.bingo.x")
				assertQuery(t, m, 0, 0, nil, "steps.bingo.y")
				assertQuery(t, m, 0, 0, nil, `steps.bingo.(x -> y)[0]`)
				assertQuery(t, m, 2, 0, nil, "steps.bingo.p")
				assertQuery(t, m, 1, 0, nil, "steps.bingo.p.q")
				assertQuery(t, m, 0, 0, nil, "steps.bingo.p.q.z")

				assertQuery(t, m, 6, 1, nil, "steps.nuclear")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.x")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.y")
				assertQuery(t, m, 0, 0, nil, `steps.nuclear.(x -> y)[0]`)
				assertQuery(t, m, 2, 0, nil, "steps.nuclear.p")
				assertQuery(t, m, 1, 0, nil, "steps.nuclear.p.q")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.p.q.z")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.quiche")
			},
		},
		{
			name: "recursive",
			run: func(t testing.TB) {
				m, err := compile(t, `x -> y
steps: {
	bingo: { p.q.z }
	nuclear: {
		quiche
		scenarios: {
			bavarian: {
				perseverance
			}
		}
	}
}`)
				assert.Success(t, err)

				assertQuery(t, m, 25, 4, nil, "")

				assertQuery(t, m, 0, 0, nil, "x")
				assertQuery(t, m, 0, 0, nil, "y")
				assertQuery(t, m, 0, 0, nil, `(x -> y)[0]`)

				assertQuery(t, m, 5, 1, nil, "steps.bingo")
				assertQuery(t, m, 0, 0, nil, "steps.bingo.x")
				assertQuery(t, m, 0, 0, nil, "steps.bingo.y")
				assertQuery(t, m, 0, 0, nil, `steps.bingo.(x -> y)[0]`)
				assertQuery(t, m, 2, 0, nil, "steps.bingo.p")
				assertQuery(t, m, 1, 0, nil, "steps.bingo.p.q")
				assertQuery(t, m, 0, 0, nil, "steps.bingo.p.q.z")

				assertQuery(t, m, 15, 2, nil, "steps.nuclear")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.x")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.y")
				assertQuery(t, m, 0, 0, nil, `steps.nuclear.(x -> y)[0]`)
				assertQuery(t, m, 2, 0, nil, "steps.nuclear.p")
				assertQuery(t, m, 1, 0, nil, "steps.nuclear.p.q")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.p.q.z")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.quiche")

				assertQuery(t, m, 7, 1, nil, "steps.nuclear.scenarios.bavarian")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.scenarios.bavarian.x")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.scenarios.bavarian.y")
				assertQuery(t, m, 0, 0, nil, `steps.nuclear.scenarios.bavarian.(x -> y)[0]`)
				assertQuery(t, m, 2, 0, nil, "steps.nuclear.scenarios.bavarian.p")
				assertQuery(t, m, 1, 0, nil, "steps.nuclear.scenarios.bavarian.p.q")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.scenarios.bavarian.p.q.z")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.scenarios.bavarian.quiche")
				assertQuery(t, m, 0, 0, nil, "steps.nuclear.scenarios.bavarian.perseverance")
			},
		},
	}
	runa(t, tca)
}
