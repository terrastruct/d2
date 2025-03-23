package d2ir_test

import (
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
	"oss.terrastruct.com/util-go/mapfs"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
)

func TestCompile(t *testing.T) {
	t.Parallel()

	t.Run("fields", testCompileFields)
	t.Run("classes", testCompileClasses)
	t.Run("edges", testCompileEdges)
	t.Run("layers", testCompileLayers)
	t.Run("scenarios", testCompileScenarios)
	t.Run("steps", testCompileSteps)
	t.Run("imports", testCompileImports)
	t.Run("patterns", testCompilePatterns)
	t.Run("filters", testCompileFilters)
	t.Run("vars", testCompileVars)
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
	return compileFS(t, d2Path, map[string]string{d2Path: text})
}

func compileFS(t testing.TB, path string, mfs map[string]string) (*d2ir.Map, error) {
	t.Helper()

	ast, err := d2parser.Parse(path, strings.NewReader(mfs[path]), nil)
	if err != nil {
		return nil, err
	}

	fs, err := mapfs.New(mfs)
	assert.Success(t, err)
	t.Cleanup(func() {
		err = fs.Close()
		assert.Success(t, err)
	})
	m, _, err := d2ir.Compile(ast, &d2ir.CompileOptions{
		FS: fs,
	})
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

	var na []d2ir.Node
	if idStr != "" {
		var err error
		na, err = m.QueryAll(idStr)
		assert.Success(t, err)
		assert.NotEqual(t, n, nil)
	} else {
		na = append(na, n)
	}

	for _, n := range na {
		m = n.Map()
		p = n.Primary()
		assert.Equal(t, nfields, m.FieldCountRecursive())
		assert.Equal(t, nedges, m.EdgeCountRecursive())
		if !makeScalar(p).Equal(makeScalar(primary)) {
			t.Fatalf("expected primary %#v but got %s", primary, p)
		}
	}

	if len(na) == 0 {
		t.Fatalf("query didn't match anything")
	}

	return na[0]
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
			Raw:   fmt.Sprint(v),
			Value: bv,
		}
	case int:
		s.Value = &d2ast.Number{
			Raw:   fmt.Sprint(v),
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
			name: "quoted",
			run: func(t testing.TB) {
				m, err := compile(t, `my_table: {
  shape: sql_table
  width: 200
  height: 200
  "shape": string
  "icon": string
  "width": int
  "height": int
}`)
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, "sql_table", "my_table.shape")
				assertQuery(t, m, 0, 0, "string", `my_table."shape"`)
			},
		},
		{
			name: "null",
			run: func(t testing.TB) {
				m, err := compile(t, `pq: pq
pq: null`)
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, nil, "")
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
					assert.ErrorString(t, err, `TestCompile/layers/errs/2/bad_edge.d2:1:1: edge with board keyword alone doesn't make sense`)
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
		{
			name: "edge",
			run: func(t testing.TB) {
				m, err := compile(t, `a -> b
scenarios: {
  1: {
    (a -> b)[0].style.opacity: 0.1
  }
}`)
				assert.Success(t, err)
				assertQuery(t, m, 8, 2, nil, "")
				assertQuery(t, m, 0, 0, nil, "(a -> b)[0]")
			},
		},
		{
			name: "multiple-scenario-map",
			run: func(t testing.TB) {
				m, err := compile(t, `a -> b: { style.opacity: 0.3 }
scenarios: {
  1: {
    (a -> b)[0].style.opacity: 0.1
  }
  1: {
	z
  }
}`)
				assert.Success(t, err)
				assertQuery(t, m, 11, 2, nil, "")
				assertQuery(t, m, 0, 0, 0.1, "scenarios.1.(a -> b)[0].style.opacity")
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
			name: "steps_panic",
			run: func(t testing.TB) {
				_, err := compile(t, `steps: {
  shape: sql_table
  id: int {constraint: primary_key}
}
scenarios: {
  shape: sql_table
  hey: int {constraint: primary_key}
}`)
				assert.ErrorString(t, err, `TestCompile/steps/steps_panic.d2:3:3: invalid step
TestCompile/steps/steps_panic.d2:7:3: invalid scenario`)
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

func testCompileClasses(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "basic",
			run: func(t testing.TB) {
				_, err := compile(t, `x
classes: {
  mango: {
    style.fill: orange
  }
}
`)
				assert.Success(t, err)
			},
		},
		{
			name: "nonroot",
			run: func(t testing.TB) {
				_, err := compile(t, `x: {
  classes: {
    mango: {
      style.fill: orange
    }
  }
}
`)
				assert.ErrorString(t, err, `TestCompile/classes/nonroot.d2:2:3: classes must be declared at a board root scope`)
			},
		},
		{
			name: "merge",
			run: func(t testing.TB) {
				m, err := compile(t, `classes: {
  mango: {
    style.fill: orange
		width: 10
  }
}
layers: {
  hawaii: {
    classes: {
      mango: {
        width: 9000
      }
    }
  }
}
`)
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "layers.hawaii.classes.mango")
				assertQuery(t, m, 0, 0, "orange", "layers.hawaii.classes.mango.style.fill")
				assertQuery(t, m, 0, 0, 9000, "layers.hawaii.classes.mango.width")
			},
		},
		{
			name: "nested",
			run: func(t testing.TB) {
				m, err := compile(t, `classes: {
  mango: {
    style.fill: orange
  }
}
layers: {
  hawaii: {
		layers: {
      maui: {
        x
      }
    }
  }
}
`)
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "layers.hawaii.classes")
				assertQuery(t, m, 3, 0, nil, "layers.hawaii.layers.maui.classes")
			},
		},
		{
			name: "inherited",
			run: func(t testing.TB) {
				m, err := compile(t, `classes: {
  mango: {
    style.fill: orange
  }
}
scenarios: {
  hawaii: {
		steps: {
      1: {
        classes: {
          cherry: {
            style.fill: red
          }
        }
        x
      }
      2: {
        y
      }
      3: {
        classes: {
          cherry: {
            style.fill: blue
          }
        }
        y
      }
      4: {
        layers: {
          deep: {
            x
          }
        }
        x
      }
    }
  }
}
`)
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "scenarios.hawaii.classes")
				assertQuery(t, m, 2, 0, nil, "scenarios.hawaii.steps.2.classes.mango")
				assertQuery(t, m, 2, 0, nil, "scenarios.hawaii.steps.2.classes.cherry")
				assertQuery(t, m, 0, 0, "blue", "scenarios.hawaii.steps.4.classes.cherry.style.fill")
				assertQuery(t, m, 0, 0, "blue", "scenarios.hawaii.steps.4.layers.deep.classes.cherry.style.fill")
			},
		},
		{
			name: "layer-modify",
			run: func(t testing.TB) {
				m, err := compile(t, `classes: {
  orb: {
    style.fill: yellow
  }
}
layers: {
  x: {
    classes.orb.style.stroke: red
  }
}
`)
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, "yellow", "layers.x.classes.orb.style.fill")
				assertQuery(t, m, 0, 0, "red", "layers.x.classes.orb.style.stroke")
			},
		},
	}
	runa(t, tca)
}

func testCompileVars(t *testing.T) {
	t.Parallel()
	tca := []testCase{
		{
			name: "spread-in-place",
			run: func(t testing.TB) {
				m, err := compile(t, `vars: {
  person-shape: {
    grid-columns: 1
    grid-rows: 2
    grid-gap: 0
    head
    body
  }
}

dora: {
  ...${person-shape}
  body
}
`)
				assert.Success(t, err)
				assert.Equal(t, "grid-columns", m.Fields[1].Map().Fields[0].Name.ScalarString())
			},
		},
	}
	runa(t, tca)
}
