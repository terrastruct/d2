package e2etests

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cdr.dev/slog"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/diff"

	"oss.terrastruct.com/d2"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2target"
	xdiff "oss.terrastruct.com/d2/lib/diff"
	"oss.terrastruct.com/d2/lib/log"
)

func TestE2E(t *testing.T) {
	t.Parallel()

	t.Run("sanity", testSanity)
	t.Run("stable", testStable)
	t.Run("regression", testRegression)
	t.Run("todo", testTodo)
}

func testSanity(t *testing.T) {
	tcs := []testCase{
		{
			name: "basic",
			script: `a -> b
`,
		},
		{
			name: "1 to 2",
			script: `a -> b
a -> c
`,
		},
		{
			name: "child to child",
			script: `a.b -> c.d
`,
		},
		{
			name: "connection label",
			script: `a -> b: hello
`,
		},
	}
	runa(t, tcs)
}

type testCase struct {
	name       string
	script     string
	assertions func(t *testing.T, diagram *d2target.Diagram)
	skip       bool
}

func runa(t *testing.T, tcs []testCase) {
	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.skip {
				t.Skip()
			}
			t.Parallel()

			run(t, tc)
		})
	}
}

func run(t *testing.T, tc testCase) {
	ctx := context.Background()
	ctx = log.WithTB(ctx, t, nil)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	ruler, err := textmeasure.NewRuler()
	if !assert.Nil(t, err) {
		return
	}

	layoutsTested := []string{"dagre", "elk"}

	for _, layoutName := range layoutsTested {

		var layout func(context.Context, *d2graph.Graph) error
		if layoutName == "dagre" {
			layout = d2dagrelayout.Layout
		} else if layoutName == "elk" {
			layout = d2elklayout.Layout
		}
		diagram, err := d2.Compile(ctx, tc.script, &d2.CompileOptions{
			UTF16:   true,
			Ruler:   ruler,
			ThemeID: 0,
			Layout:  layout,
		})
		if !assert.Nil(t, err) {
			return
		}

		if tc.assertions != nil {
			t.Run("assertions", func(t *testing.T) {
				tc.assertions(t, diagram)
			})
		}

		dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestE2E/"), layoutName)
		pathGotSVG := filepath.Join(dataPath, "sketch.got.svg")
		pathExpSVG := filepath.Join(dataPath, "sketch.exp.svg")
		svgBytes, err := d2svg.Render(diagram)
		if err != nil {
			t.Fatal(err)
		}

		err = diff.Testdata(filepath.Join(dataPath, "board"), diagram)
		if err != nil {
			ioutil.WriteFile(pathGotSVG, svgBytes, 0600)
			t.Fatal(err)
		}
		if os.Getenv("SKIP_SVG_CHECK") == "" {
			err = xdiff.TestdataGeneric(filepath.Join(dataPath, "sketch"), ".svg", svgBytes)
			if err != nil {
				ioutil.WriteFile(pathGotSVG, svgBytes, 0600)
				t.Fatal(err)
			}
		}
		err = ioutil.WriteFile(pathExpSVG, svgBytes, 0600)
		if err != nil {
			t.Fatal(err)
		}
		os.Remove(filepath.Join(dataPath, "sketch.got.svg"))
	}
}

func getShape(t *testing.T, diagram *d2target.Diagram, id string) d2target.Shape {
	for _, shape := range diagram.Shapes {
		if shape.ID == id {
			return shape
		}
	}
	t.Fatalf(`Shape "%s" not found`, id)
	return d2target.Shape{}
}

func mdTestScript(md string) string {
	return fmt.Sprintf(`
md: |md
%s
|
a -> md -> b
`, md)
}
