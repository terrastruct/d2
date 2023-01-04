package e2etests

import (
	"context"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cdr.dev/slog"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2layouts/d2near"
	"oss.terrastruct.com/d2/d2layouts/d2sequence"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
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
			name:   "empty",
			script: ``,
		},
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

// serde exercises serializing and deserializing the graph
// We want to run all the steps leading up to serialization in the course of regular layout
func serde(t *testing.T, tc testCase, ruler *textmeasure.Ruler) {
	ctx := context.Background()
	ctx = log.WithTB(ctx, t, nil)
	g, err := d2compiler.Compile("", strings.NewReader(tc.script), &d2compiler.CompileOptions{
		UTF16: false,
	})
	tassert.Nil(t, err)
	if len(g.Objects) > 0 {
		err = g.SetDimensions(nil, ruler, nil)
		tassert.Nil(t, err)
		d2near.WithoutConstantNears(ctx, g)
		d2sequence.WithoutSequenceDiagrams(ctx, g)
	}
	b, err := d2graph.SerializeGraph(g)
	tassert.Nil(t, err)
	var newG d2graph.Graph
	err = d2graph.DeserializeGraph(b, &newG)
	tassert.Nil(t, err)
}

func run(t *testing.T, tc testCase) {
	ctx := context.Background()
	ctx = log.WithTB(ctx, t, nil)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	ruler, err := textmeasure.NewRuler()
	if !tassert.Nil(t, err) {
		return
	}

	serde(t, tc, ruler)

	layoutsTested := []string{"dagre", "elk"}

	for _, layoutName := range layoutsTested {
		var layout func(context.Context, *d2graph.Graph) error
		if layoutName == "dagre" {
			layout = d2dagrelayout.DefaultLayout
		} else if layoutName == "elk" {
			layout = d2elklayout.DefaultLayout
		}
		diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
			Ruler:   ruler,
			ThemeID: 0,
			Layout:  layout,
		})
		if !tassert.Nil(t, err) {
			return
		}

		if tc.assertions != nil {
			t.Run("assertions", func(t *testing.T) {
				tc.assertions(t, diagram)
			})
		}

		dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestE2E/"), layoutName)
		pathGotSVG := filepath.Join(dataPath, "sketch.got.svg")

		svgBytes, err := d2svg.Render(diagram, &d2svg.RenderOpts{
			Pad: d2svg.DEFAULT_PADDING,
		})
		assert.Success(t, err)
		err = os.MkdirAll(dataPath, 0755)
		assert.Success(t, err)
		err = ioutil.WriteFile(pathGotSVG, svgBytes, 0600)
		assert.Success(t, err)
		// if running from e2ereport.sh, we want to keep .got.svg on a failure
		forReport := os.Getenv("E2E_REPORT") != ""
		if !forReport {
			defer os.Remove(pathGotSVG)
		}

		// Check that it's valid SVG
		var xmlParsed interface{}
		err = xml.Unmarshal(svgBytes, &xmlParsed)
		assert.Success(t, err)

		err = diff.TestdataJSON(filepath.Join(dataPath, "board"), diagram)
		assert.Success(t, err)
		if os.Getenv("SKIP_SVG_CHECK") == "" {
			err = diff.Testdata(filepath.Join(dataPath, "sketch"), ".svg", svgBytes)
			assert.Success(t, err)
		}
		if forReport {
			os.Remove(pathGotSVG)
		}
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
