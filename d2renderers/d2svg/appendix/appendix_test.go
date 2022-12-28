package appendix_test

import (
	"context"
	"encoding/xml"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cdr.dev/slog"

	tassert "github.com/stretchr/testify/assert"

	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/diff"

	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/d2svg/appendix"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func TestAppendix(t *testing.T) {
	t.Parallel()

	tcs := []testCase{
		{
			name: "basic",
			script: `x: { tooltip: Total abstinence is easier than perfect moderation }
y: { tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
`,
		},
	}
	runa(t, tcs)
}

type testCase struct {
	name   string
	script string
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
	ctx = log.WithTB(ctx, t, nil)
	ctx = log.Leveled(ctx, slog.LevelDebug)

	ruler, err := textmeasure.NewRuler()
	if !tassert.Nil(t, err) {
		return
	}

	diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
		Ruler:   ruler,
		ThemeID: 0,
		Layout:  d2dagrelayout.Layout,
	})
	if !tassert.Nil(t, err) {
		return
	}

	dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestAppendix/"))
	pathGotSVG := filepath.Join(dataPath, "sketch.got.svg")

	svgBytes, err := d2svg.Render(diagram, &d2svg.RenderOpts{
		Pad: d2svg.DEFAULT_PADDING,
	})
	assert.Success(t, err)
	svgBytes = appendix.AppendTooltips(diagram, ruler, svgBytes)

	err = os.MkdirAll(dataPath, 0755)
	assert.Success(t, err)
	err = ioutil.WriteFile(pathGotSVG, svgBytes, 0600)
	assert.Success(t, err)
	defer os.Remove(pathGotSVG)

	var xmlParsed interface{}
	err = xml.Unmarshal(svgBytes, &xmlParsed)
	assert.Success(t, err)

	err = diff.Testdata(filepath.Join(dataPath, "sketch"), ".svg", svgBytes)
	assert.Success(t, err)
}
