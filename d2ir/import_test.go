package d2ir_test

import (
	"testing"

	"oss.terrastruct.com/util-go/assert"

	"oss.terrastruct.com/d2/d2ir"
)

func testCompileImports(t *testing.T) {
	t.Parallel()

	tca := []testCase{
		{
			name: "value",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "x: @x.d2",
					"x.d2": `shape: circle
label: meow`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "")
				assertQuery(t, m, 2, 0, nil, "x")
				assertQuery(t, m, 0, 0, "circle", "x.shape")
				assertQuery(t, m, 0, 0, "meow", "x.label")
			},
		},
		{
			name: "nested/map",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "x: @x.y",
					"x.d2": `y: {
	shape: circle
	label: meow
}`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "")
				assertQuery(t, m, 2, 0, nil, "x")
				assertQuery(t, m, 0, 0, "circle", "x.shape")
				assertQuery(t, m, 0, 0, "meow", "x.label")
			},
		},
		{
			name: "nested/array",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "x: @x.y",
					"x.d2":     `y: [1, 2]`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "")
				x := assertQuery(t, m, 0, 0, nil, "x")
				xf, ok := x.(*d2ir.Field)
				assert.True(t, ok)
				assert.Equal(t, `[1, 2]`, xf.Composite.String())
			},
		},
		{
			name: "nested/scalar",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "x: @x.y",
					"x.d2":     `y: meow`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "")
				assertQuery(t, m, 0, 0, "meow", "x")
			},
		},
		{
			name: "spread",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "...@x.d2",
					"x.d2":     "x: wowa",
				})
				assert.Success(t, err)
				assertQuery(t, m, 1, 0, nil, "")
				assertQuery(t, m, 0, 0, "wowa", "x")
			},
		},
		{
			name: "nested/spread",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "...@x.y",
					"x.d2":     "y: { jon; jan }",
				})
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "")
				assertQuery(t, m, 0, 0, nil, "jan")
				assertQuery(t, m, 0, 0, nil, "jon")
			},
		},
		{
			name: "nested/spread_primary",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "q: { ...@x.y }",
					"x.d2":     "y: meow { jon; jan }",
				})
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "")
				assertQuery(t, m, 2, 0, "meow", "q")
				assertQuery(t, m, 0, 0, nil, "q.jan")
				assertQuery(t, m, 0, 0, nil, "q.jon")
			},
		},
	}

	runa(t, tca)

	t.Run("errors", func(t *testing.T) {
		tca := []testCase{
			{
				name: "not_exist",
				run: func(t testing.TB) {
					_, err := compileFS(t, "index.d2", map[string]string{
						"index.d2": "...@x.d2",
					})
					assert.ErrorString(t, err, `index.d2:1:1: failed to import "x.d2": open x.d2: no such file or directory`)
				},
			},
			{
				name: "escape",
				run: func(t testing.TB) {
					_, err := compileFS(t, "index.d2", map[string]string{
						"index.d2": "...@'./../x.d2'",
					})
					assert.ErrorString(t, err, `index.d2:1:1: failed to import "../x.d2": stat ../x.d2: invalid argument`)
				},
			},
			{
				name: "absolute",
				run: func(t testing.TB) {
					_, err := compileFS(t, "index.d2", map[string]string{
						"index.d2": "...@/x.d2",
					})
					assert.ErrorString(t, err, `index.d2:1:1: import paths must be relative`)
				},
			},
			{
				name: "parse",
				run: func(t testing.TB) {
					_, err := compileFS(t, "index.d2", map[string]string{
						"index.d2": "...@x.d2",
						"x.d2":     "x<><><<>q",
					})
					assert.ErrorString(t, err, `x.d2:1:1: connection missing destination
x.d2:1:4: connection missing source
x.d2:1:4: connection missing destination
x.d2:1:6: connection missing source
x.d2:1:6: connection missing destination
x.d2:1:7: connection missing source`)
				},
			},
			{
				name: "cyclic",
				run: func(t testing.TB) {
					_, err := compileFS(t, "index.d2", map[string]string{
						"index.d2": "...@x",
						"x.d2":     "...@y",
						"y.d2":     "...@q",
						"q.d2":     "...@x",
					})
					assert.ErrorString(t, err, `q.d2:1:1: detected cyclic import chain: x -> y -> q -> x`)
				},
			},
			{
				name: "spread_non_map",
				run: func(t testing.TB) {
					_, err := compileFS(t, "index.d2", map[string]string{
						"index.d2": "...@x.y",
						"x.d2":     "y: meow",
					})
					assert.ErrorString(t, err, `index.d2:1:1: cannot spread import non map into map`)
				},
			},
		}
		runa(t, tca)
	})
}
