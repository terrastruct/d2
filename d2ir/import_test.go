package d2ir_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oss.terrastruct.com/util-go/assert"

	"oss.terrastruct.com/d2/d2ir"
	"oss.terrastruct.com/d2/d2parser"
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
			name: "boards",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": `x.link: layers.x; layers: { x: @x }`,
					"x.d2":     `y.link: layers.y; layers: { y: @y }`,
					"y.d2":     `meow`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, "root.layers.x", "x.link")
				assertQuery(t, m, 0, 0, "root.layers.x.layers.y", "layers.x.y.link")
			},
		},
		{
			name: "boards-deep",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": `a.link: layers.b; layers: { b: @b }`,
					"b.d2":     `b.link: layers.c; layers: { c: @c }`,
					"c.d2":     `c.link: layers.d; layers: { d: @d }`,
					"d.d2":     `d`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, "root.layers.b.layers.c.layers.d", "layers.b.layers.c.c.link")
			},
		},
		{
			name: "steps-inheritence",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": `z; steps: { 1: @x; 2: @y }; scenarios: { x: @x; y: @y }`,
					"x.d2":     `a`,
					"y.d2":     `b`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 2, 0, nil, "scenarios.x")
				assertQuery(t, m, 2, 0, nil, "scenarios.y")
				assertQuery(t, m, 2, 0, nil, "steps.1")
				assertQuery(t, m, 3, 0, nil, "steps.2")
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
			name: "os/absolute_path",
			run: func(t testing.TB) {
				dir := t.TempDir()
				indexPath := filepath.Join(dir, "index.d2")
				err := os.WriteFile(indexPath, []byte("x: @x"), 0600)
				assert.Success(t, err)
				err = os.WriteFile(filepath.Join(dir, "x.d2"), []byte("shape: circle\nlabel: meow"), 0600)
				assert.Success(t, err)

				m, err := compileFile(t, indexPath)
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "")
				assertQuery(t, m, 2, 0, nil, "x")
				assertQuery(t, m, 0, 0, "circle", "x.shape")
				assertQuery(t, m, 0, 0, "meow", "x.label")
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
		{
			name: "vars/1",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "vars: { ...@x }; q: ${meow}",
					"x.d2":     "meow: var replaced",
				})
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, "var replaced", "q")
			},
		},
		{
			name: "vars/2",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "vars: { x: 1 }; ...@a",
					"a.d2":     "vars: { x: 2 }; hi: ${x}",
				})
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, 2, "hi")
			},
		},
		{
			name: "vars/3",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": "...@a; vars: { x: 1 }; hi: ${x}",
					"a.d2":     "vars: { x: 2 }",
				})
				assert.Success(t, err)
				assertQuery(t, m, 0, 0, 1, "hi")
			},
		},
		{
			name: "pattern-value",
			run: func(t testing.TB) {
				_, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": `userWebsite
userMobile

user*: @x
`,
					"x.d2": `shape: person
label: meow`,
				})
				assert.Success(t, err)
			},
		},
		{
			name: "merge-arrays",
			run: func(t testing.TB) {
				_, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": `x.class: [a]
...@d
`,
					"d.d2": `x.class: [b]`,
				})
				assert.Success(t, err)
			},
		},
		{
			name: "nested-scope",
			run: func(t testing.TB) {
				m, err := compileFS(t, "index.d2", map[string]string{
					"index.d2": `...@second
`,
					"second.d2": `elem: {
  ...@third
}`,
					"third.d2": `third: {
  elem
}`,
				})
				assert.Success(t, err)
				assertQuery(t, m, 3, 0, nil, "")
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
					assert.Error(t, err)
					errText := err.Error()
					if !strings.HasPrefix(errText, `index.d2:1:1: failed to import "x.d2": open x.d2: `) {
						t.Fatalf("unexpected import error: %v", err)
					}
					if !strings.Contains(errText, "no such file or directory") &&
						!strings.Contains(errText, "The system cannot find the file specified") &&
						!strings.Contains(errText, "file does not exist") {
						t.Fatalf("unexpected import error: %v", err)
					}
				},
			},
			{
				name: "escape",
				run: func(t testing.TB) {
					_, err := compileFS(t, "index.d2", map[string]string{
						"index.d2": "...@'./../x.d2'",
					})
					assert.ErrorString(t, err, `index.d2:1:1: failed to import "../x.d2": open ../x.d2: invalid argument`)
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
					assert.ErrorString(t, err, `q.d2:1:1: detected cyclic import chain: x.d2 -> y.d2 -> q.d2 -> x.d2`)
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

func compileFile(t testing.TB, filePath string) (*d2ir.Map, error) {
	t.Helper()

	b, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	ast, err := d2parser.Parse(filePath, strings.NewReader(string(b)), nil)
	if err != nil {
		return nil, err
	}
	m, _, err := d2ir.Compile(ast, nil)
	return m, err
}
