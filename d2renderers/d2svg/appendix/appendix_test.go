package appendix_test

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

	"oss.terrastruct.com/d2/d2graph"
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
			name: "tooltip_wider_than_diagram",
			script: `x: { tooltip: Total abstinence is easier than perfect moderation }
y: { tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
`,
		},
		{
			name: "diagram_wider_than_tooltip",
			script: `shape: sequence_diagram

customer
issuer
store: { tooltip: Like starbucks or something }
acquirer: { tooltip: I'm not sure what this is }
network
customer bank
store bank

customer: {shape: person}
customer bank: {
  shape: image
  icon: https://cdn-icons-png.flaticon.com/512/858/858170.png
}
store bank: {
  shape: image
  icon: https://cdn-icons-png.flaticon.com/512/858/858170.png
}

initial transaction: {
  customer -> store: 1 banana please
  store -> customer: '$10 dollars'
}
customer.internal -> customer.internal: "thinking: wow, inflation"
customer.internal -> customer bank: checks bank account
customer bank -> customer.internal: 'Savings: $11'
customer."An error in judgement is about to occur"
customer -> store: I can do that, here's my card
payment processor behind the scenes: {
  store -> acquirer: Run this card
  acquirer -> network: Process to card issuer
  simplified: {
    network -> issuer: Process this payment
    issuer -> customer bank: '$10 debit'
    acquirer -> store bank: '$10 credit'
  }
}
`,
		},
		{
			name: "links",
			script: `x: { link: https://d2lang.com }
			y: { link: https://terrastruct.com; tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
`,
		},
		{
			name:    "links dark",
			themeID: 200,
			script: `x: { link: https://d2lang.com }
			y: { link: https://fosny.eu; tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
`,
		},
		{
			name: "internal-links",
			script: `x: { link: layers.x }
layers: {
  x: {
    gooo
    home.link: _
    next.link: steps.next
    steps: {
      next: {
          hi
      }
    }
  }
}
`,
		},
		{
			name: "tooltip_fill",
			script: `x: { tooltip: Total abstinence is easier than perfect moderation }
y: { tooltip: Gee, I feel kind of LIGHT in the head now,\nknowing I can't make my satellite dish PAYMENTS! }
x -> y
style.fill: PaleVioletRed
`,
		},
	}
	runa(t, tcs)
}

type testCase struct {
	name    string
	themeID int64
	script  string
	skip    bool
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
		ThemeID: &tc.themeID,
	}

	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}
	diagram, _, err := d2lib.Compile(ctx, tc.script, &d2lib.CompileOptions{
		Ruler:          ruler,
		LayoutResolver: layoutResolver,
	}, renderOpts)
	if !tassert.Nil(t, err) {
		return
	}

	dataPath := filepath.Join("testdata", strings.TrimPrefix(t.Name(), "TestAppendix/"))
	pathGotSVG := filepath.Join(dataPath, "sketch.got.svg")

	svgBytes, err := d2svg.Render(diagram, renderOpts)
	assert.Success(t, err)
	svgBytes = appendix.Append(diagram, nil, ruler, svgBytes)

	err = os.MkdirAll(dataPath, 0755)
	assert.Success(t, err)
	err = os.WriteFile(pathGotSVG, svgBytes, 0600)
	assert.Success(t, err)
	defer os.Remove(pathGotSVG)

	var xmlParsed interface{}
	err = xml.Unmarshal(svgBytes, &xmlParsed)
	assert.Success(t, err)

	err = diff.Testdata(filepath.Join(dataPath, "sketch"), ".svg", svgBytes)
	assert.Success(t, err)
}
