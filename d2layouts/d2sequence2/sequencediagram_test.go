package sequencediagram_test

import (
	"context"
	"strings"
	"testing"

	"oss.terrastruct.com/d2/d2compiler"
	sequencediagram "oss.terrastruct.com/d2/d2layouts/d2sequence2"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/assert"
	"oss.terrastruct.com/util-go/mapfs"
)

func TestSequenceDiagrams(t *testing.T) {
	t.Parallel()

	t.Run("basic", testBasic)
}

func testBasic(t *testing.T) {
	t.Parallel()

	tca := []testCase{
		{
			name: "escaped",
			assert: func(t testing.TB, sd *sequencediagram.SequenceDiagram) {
				assert.True(t, 1 == 1)
			},
		},
	}

	runa(t, tca)
}

type testCase struct {
	name   string
	fs     map[string]string
	assert func(testing.TB, *sequencediagram.SequenceDiagram)
	expErr string
}

func runa(t *testing.T, tca []testCase) {
	for _, tc := range tca {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			ctx = log.WithTB(ctx, t)
			ruler, _ := textmeasure.NewRuler()
			fs, _ := mapfs.New(tc.fs)
			compileOpts := &d2lib.CompileOptions{
				Ruler: ruler,
				FS:    fs,
			}
			g, config, err := d2compiler.Compile(compileOpts.InputPath, strings.NewReader(tc.fs["index.d2"]), &d2compiler.CompileOptions{
				UTF16Pos: compileOpts.UTF16Pos,
				FS:       compileOpts.FS,
			})
			if tc.expErr != "" {
				assert.Error(t, err)
			} else {
				assert.Success(t, err)
			}
			if config != nil {
				g.Data = config.Data
			}
			err = g.SetDimensions(nil, compileOpts.Ruler, compileOpts.FontFamily, compileOpts.MonoFontFamily)
			if tc.expErr != "" {
				assert.Error(t, err)
			} else {
				assert.Success(t, err)
			}

			sd, err := sequencediagram.Layout(ctx, g)
			if tc.expErr != "" {
				assert.Error(t, err)
			} else {
				assert.Success(t, err)
			}
			tc.assert(t, sd)
		})
	}
}
