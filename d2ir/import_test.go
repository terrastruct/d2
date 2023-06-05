package d2ir_test

import (
	"testing"

	"oss.terrastruct.com/util-go/assert"
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
			name: "nested",
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
	}

	runa(t, tca)

	t.Run("errors", func(t *testing.T) {
		tca := []testCase{
			{
				name: "parse_error",
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
		}
		runa(t, tca)
	})
}
