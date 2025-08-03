package textmeasure_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/lib/textmeasure"
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
	`a`:             {9, 24},
	`w`:             {12, 24},
	`ww`:            {24, 24},
	"`inline code`": {103, 24},
	"`code`":        {46, 24},
	"`a`":           {21, 24},
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
