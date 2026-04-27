package dual_theme_test

import (
	"context"
	"encoding/xml"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"
	"oss.terrastruct.com/util-go/go2"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/d2svg/svgtestdata"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func TestDualTheme(t *testing.T) {
	t.Parallel()

	var tcs []testCase
	// Create test cases for both sketch and non-sketch modes from shared scripts
	for name, script := range svgtestdata.Scripts {
		tcs = append(tcs, testCase{
			name:   name,
			script: script,
			sketch: false,
		})
		tcs = append(tcs, testCase{
			name:   name + "_sketch",
			script: script,
			sketch: true,
		})
	}

	runa(t, tcs)
}

type testCase struct {
	name   string
	script string
	sketch bool
	skip   bool
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
	ctx = log.WithTB(ctx, t)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	ruler, err := textmeasure.NewRuler()
	if !tassert.Nil(t, err) {
		return
	}

	renderOpts := &d2svg.RenderOpts{
		ThemeID:     go2.Pointer(int64(0)),   // NeutralDefault light theme
		DarkThemeID: go2.Pointer(int64(200)), // DarkMauve dark theme
	}
	if tc.sketch {
		renderOpts.Sketch = go2.Pointer(true)
	}

	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}

	diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
		Ruler:          ruler,
		LayoutResolver: layoutResolver,
		FontFamily:     go2.Pointer(d2fonts.HandDrawn),
	}, renderOpts)
	if !tassert.Nil(t, err) {
		return
	}

	dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestDualTheme/"))
	pathGotSVG := filepath.Join(dataPath, "dual_theme.got.svg")

	svgBytes, err := d2svg.Render(diagram, renderOpts)
	assert.Success(t, err)
	err = os.MkdirAll(dataPath, 0755)
	assert.Success(t, err)
	err = os.WriteFile(pathGotSVG, svgBytes, 0600)
	assert.Success(t, err)
	defer os.Remove(pathGotSVG)

	var xmlParsed interface{}
	err = xml.Unmarshal(svgBytes, &xmlParsed)
	assert.Success(t, err)

	err = diff.Testdata(filepath.Join(dataPath, "dual_theme"), ".svg", svgBytes)
	assert.Success(t, err)
}
