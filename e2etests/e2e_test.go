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

	trequire "github.com/stretchr/testify/require"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2layouts/d2near"
	"oss.terrastruct.com/d2/d2layouts/d2sequence"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2plugin"
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
	t.Run("measured", testMeasured)
	t.Run("unicode", testUnicode)
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
	name              string
	script            string
	mtexts            []*d2target.MText
	assertions        func(t *testing.T, diagram *d2target.Diagram)
	skip              bool
	dagreFeatureError string
	elkFeatureError   string
	expErr            string
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
	trequire.Nil(t, err)
	if len(g.Objects) > 0 {
		err = g.SetDimensions(nil, ruler, nil)
		trequire.Nil(t, err)
		d2near.WithoutConstantNears(ctx, g)
		d2sequence.WithoutSequenceDiagrams(ctx, g)
	}
	b, err := d2graph.SerializeGraph(g)
	trequire.Nil(t, err)
	var newG d2graph.Graph
	err = d2graph.DeserializeGraph(b, &newG)
	trequire.Nil(t, err)
	trequire.Nil(t, d2graph.CompareSerializedGraph(g, &newG))
}

func run(t *testing.T, tc testCase) {
	ctx := context.Background()
	ctx = log.WithTB(ctx, t, nil)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	var ruler *textmeasure.Ruler
	var err error
	if tc.mtexts == nil {
		ruler, err = textmeasure.NewRuler()
		trequire.Nil(t, err)

		serde(t, tc, ruler)
	}

	layoutsTested := []string{"dagre", "elk"}

	for _, layoutName := range layoutsTested {
		var layout func(context.Context, *d2graph.Graph) error
		var plugin d2plugin.Plugin
		if layoutName == "dagre" {
			layout = d2dagrelayout.DefaultLayout
			plugin = &d2plugin.DagrePlugin
		} else if layoutName == "elk" {
			// If measured texts exists, we are specifically exercising text measurements, no need to run on both layouts
			if tc.mtexts != nil {
				continue
			}
			layout = d2elklayout.DefaultLayout
			plugin = &d2plugin.ELKPlugin
		}

		diagram, g, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
			Ruler:         ruler,
			MeasuredTexts: tc.mtexts,
			Layout:        layout,
		})

		if tc.expErr != "" {
			assert.Error(t, err)
			assert.ErrorString(t, err, tc.expErr)
			return
		} else {
			assert.Success(t, err)
		}

		pluginInfo, err := plugin.Info(ctx)
		assert.Success(t, err)

		err = d2plugin.FeatureSupportCheck(pluginInfo, g)
		switch layoutName {
		case "dagre":
			if tc.dagreFeatureError != "" {
				assert.Error(t, err)
				assert.ErrorString(t, err, tc.dagreFeatureError)
				return
			}
		case "elk":
			if tc.elkFeatureError != "" {
				assert.Error(t, err)
				assert.ErrorString(t, err, tc.elkFeatureError)
				return
			}
		}
		assert.Success(t, err)

		if tc.assertions != nil {
			t.Run("assertions", func(t *testing.T) {
				tc.assertions(t, diagram)
			})
		}

		dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestE2E/"), layoutName)
		pathGotSVG := filepath.Join(dataPath, "sketch.got.svg")

		svgBytes, err := d2svg.Render(diagram, &d2svg.RenderOpts{
			Pad:     d2svg.DEFAULT_PADDING,
			ThemeID: 0,
		})
		assert.Success(t, err)
		err = os.MkdirAll(dataPath, 0755)
		assert.Success(t, err)
		err = ioutil.WriteFile(pathGotSVG, svgBytes, 0600)
		assert.Success(t, err)

		// Check that it's valid SVG
		var xmlParsed interface{}
		err = xml.Unmarshal(svgBytes, &xmlParsed)
		assert.Success(t, err)

		var err2 error
		err = diff.TestdataJSON(filepath.Join(dataPath, "board"), diagram)
		if os.Getenv("SKIP_SVG_CHECK") == "" {
			err2 = diff.Testdata(filepath.Join(dataPath, "sketch"), ".svg", svgBytes)
		}
		assert.Success(t, err)
		assert.Success(t, err2)
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
