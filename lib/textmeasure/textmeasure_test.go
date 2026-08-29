package textmeasure_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/lib/textmeasure"
)

var txts = []string{
	"Jesus is my POSTMASTER GENERAL ...",
	"Don't let go of what you've got hold of, until you have hold of something else.",
	"To get something clean, one has to get something dirty.",
	"The notes blatted skyward as they rose over the Canada geese, feathered",
	"There is no such thing as a problem without a gift for you in its hands.",
	"Baseball is a skilled game.  It's America's game - it, and high taxes.",
	"He is truly wise who gains wisdom from another's mishap.",
	"If you have never been hated by your child, you have never been a parent.",
	"Your only obligation in any lifetime is to be true to yourself.  Being",
	"The computing field is always in need of new cliches.",
}

func TestTextMeasure(t *testing.T) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}

	// For a set of random strings, test each char increases width but not height
	for _, txt := range txts {
		txt = strings.ReplaceAll(txt, " ", "")
		for i := 1; i < len(txt)-1; i++ {
			w1, h1 := ruler.Measure(d2fonts.SourceSansPro.Font(d2fonts.FONT_SIZE_M, d2fonts.FONT_STYLE_REGULAR), txt[:i])
			w2, h2 := ruler.Measure(d2fonts.SourceSansPro.Font(d2fonts.FONT_SIZE_M, d2fonts.FONT_STYLE_REGULAR), txt[:i+1])
			assert.Equal(t, h1, h2)
			assert.Less(t, w1, w2, fmt.Sprintf(`"%s" vs "%s"`, txt[:i], txt[:i+1]))
		}
	}

	// For a set of random strings, test that adding newlines increases height each time
	for _, txt := range txts {
		whitespaces := strings.Count(txt, " ")
		for i := 0; i < whitespaces-1; i++ {
			txt1 := strings.Replace(txt, " ", "\n", i)
			txt2 := strings.Replace(txt, " ", "\n", i+1)

			w1, h1 := ruler.Measure(d2fonts.SourceSansPro.Font(d2fonts.FONT_SIZE_M, d2fonts.FONT_STYLE_REGULAR), txt1)
			w2, h2 := ruler.Measure(d2fonts.SourceSansPro.Font(d2fonts.FONT_SIZE_M, d2fonts.FONT_STYLE_REGULAR), txt2)

			assert.Less(t, h1, h2)
			assert.Less(t, w2, w1)
		}
	}
}

func TestMeasurePreservesOrdinaryUnicodeDimensions(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	font := d2fonts.SourceSansPro.Font(16, d2fonts.FONT_STYLE_BOLD)

	for text, wantWidth := range map[string]int{
		"🐵🐵🐵🐵🐵🐵🐵🐵": 160,
		"✊✊✊✊":     79,
		"☁️☁️☁️☁️": 87,
		"中文測試":     79,
	} {
		width, height := ruler.Measure(font, text)
		assert.Equal(t, wantWidth, width, text)
		assert.Equal(t, 21, height, text)
	}
}

func TestFontMeasure(t *testing.T) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}

	// For a set of random strings, test that font sizes are strictly increasing
	for _, txt := range txts {
		for i := 0; i < len(d2fonts.FontSizes)-1; i++ {
			w1, h1 := ruler.Measure(d2fonts.SourceSansPro.Font(d2fonts.FontSizes[i], d2fonts.FONT_STYLE_REGULAR), txt)
			w2, h2 := ruler.Measure(d2fonts.SourceSansPro.Font(d2fonts.FontSizes[i+1], d2fonts.FONT_STYLE_REGULAR), txt)
			assert.Less(t, h1, h2)
			assert.Less(t, w1, w2)
		}
	}

}

type dimensions struct {
	width, height int
}

var mdTexts = map[string]dimensions{
	`
- [Overview](#overview) ok _this is all measured_
`: {245, 24},
	`
_italics are all measured correctly_
`: {214, 24},
	`
**bold is measured correctly**
`: {188, 24},
	`
**Note:** This document
`: {143, 24},
	`
**Note:**
`: {39, 24},
	`a`:                  {9, 24},
	`w`:                  {12, 24},
	`ww`:                 {24, 24},
	"`inline code`":      {103, 24},
	"`code`":             {46, 24},
	"`a`":                {21, 24},
	"`日本語日本語`":           {62, 24},
	"```\n日本語日本語\n```":   {81, 56},
	"# `日本語日本語`":         {124, 58},
	"```\n👩🏼‍❤️‍👨🏼\n```": {98, 56},
	"👩🏼‍❤️‍👨🏼":           {84, 24},
	"\u0301x":            {18, 24},
	"# *italic*":         {65, 51},
	"**_reverse_**":      {48, 24},
	"_**forward**_":      {58, 24},
	"**`bold code`**":    {87, 24},
	"*`italic code`*":    {103, 24},
	"*":                  {32, 0},
	"-":                  {32, 0},
	"1.":                 {32, 0},
	`# Cloud Run Egress Architecture — Backend / Exporter / Autolayout / Fetcher`: {1018, 51},
}

func TestTextMeasureMarkdown(t *testing.T) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}

	for text, dims := range mdTexts {
		width, height, err := textmeasure.MeasureMarkdown(text, ruler, nil, nil, textmeasure.MarkdownFontSize)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, dims.width, width, text)
		assert.Equal(t, dims.height, height, text)
	}

}

func TestMarkdownUnsupportedEmojiMatchesLegacyViewport(t *testing.T) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}

	width, _, err := textmeasure.MeasureMarkdown("🛡 Some long enough text goes here", ruler, nil, nil, textmeasure.MarkdownFontSize)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the graph-layout dimension used by the former browser renderer.
	// Native painting reserves a browser-like fallback advance inside it rather
	// than expanding every Markdown box with global safety padding.
	assert.Equal(t, 236, width)
}

func TestMarkdownZeroWidthCharactersDoNotAddWidth(t *testing.T) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}

	plainWidth, _, err := textmeasure.MeasureMarkdown("x", ruler, nil, nil, textmeasure.MarkdownFontSize)
	if err != nil {
		t.Fatal(err)
	}
	zeroWidth, _, err := textmeasure.MeasureMarkdown("\u200b\u200b\u200b\u200b\u200bx\u200b\u200b\u200b\u200b\u200b", ruler, nil, nil, textmeasure.MarkdownFontSize)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, plainWidth, zeroWidth)
	for _, control := range []string{
		"\u061c",     // Arabic letter mark
		"\u200e",     // left-to-right mark
		"\u2066",     // left-to-right isolate
		"\u115f",     // zero-advance Hangul choseong filler
		"\u1160",     // zero-advance Hangul jungseong filler
		"\U000e0001", // language tag
	} {
		width, _, err := textmeasure.MeasureMarkdown(control+"x"+control, ruler, nil, nil, textmeasure.MarkdownFontSize)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, plainWidth, width, fmt.Sprintf("default-ignorable %U", []rune(control)[0]))
	}
	for _, text := range []string{
		"x\ufe0f",     // variation selector attached to the base cluster
		"x\u200d",     // zero-width joiner attached to the base cluster
		"x\u034f",     // combining grapheme joiner attached to the base cluster
		"x\U000e007f", // cancel tag attached to the base cluster
		"\U000e0001x\U000e007f",
	} {
		width, _, err := textmeasure.MeasureMarkdown(text, ruler, nil, nil, textmeasure.MarkdownFontSize)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, plainWidth, width, fmt.Sprintf("attached default-ignorable %q", text))
	}

	// These controls and fillers are nominally format/default-ignorable code
	// points, but Chromium paints them through fallback fonts. Do not collapse
	// their established viewport to the width of x.
	visibleFallbacks := map[string]int{
		"\ufff9x\ufffb":         29,
		"\U00013430x\U0001343f": 29,
		"\u0600":                11,
		"\u180fx\u180f":         29,
		"\u3164x\u3164":         47,
		"\uffa0x\uffa0":         29,
		"\U0001bca0x\U0001bca0": 29,
	}
	for text, want := range visibleFallbacks {
		width, _, err := textmeasure.MeasureMarkdown(text, ruler, nil, nil, textmeasure.MarkdownFontSize)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, want, width, fmt.Sprintf("visible fallback %U", []rune(text)[0]))
	}

	visible := "x\n👩🏼‍❤️‍👨🏼"
	visibleWidth, _, err := textmeasure.MeasureMarkdown(visible, ruler, nil, nil, textmeasure.MarkdownFontSize)
	if err != nil {
		t.Fatal(err)
	}
	mixedWidth, _, err := textmeasure.MeasureMarkdown("\u200b"+visible+"\u200b", ruler, nil, nil, textmeasure.MarkdownFontSize)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, visibleWidth, mixedWidth)
}
