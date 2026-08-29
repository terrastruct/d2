package textmeasure_test

import (
	"encoding/xml"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/lib/textmeasure"
)

func TestLayoutMarkdownNativePrimitives(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	markdown := strings.TrimSpace(`
# Heading

Paragraph with **bold**, *italic*, ~~deleted~~, [a link](https://example.com/a?x=1&y=2), and ` + "`inline code`" + `.<br />Second line.

> Quoted **text**

3. ordered
4. list

- unordered
  - nested

---

` + "```go" + `
fmt.Println("hello")
return
` + "```" + `

| Name | Value |
| :--- | ----: |
| alpha | **one** |
| beta | two |
`)

	layout, err := textmeasure.LayoutMarkdown(markdown, ruler, nil, nil, textmeasure.MarkdownFontSize)
	require.NoError(t, err)
	require.Positive(t, layout.Width)
	require.Positive(t, layout.Height)
	require.NotEmpty(t, layout.Primitives)

	width, height, err := textmeasure.MeasureMarkdown(markdown, ruler, nil, nil, textmeasure.MarkdownFontSize)
	require.NoError(t, err)
	assert.Equal(t, width, layout.Width)
	assert.Equal(t, height, layout.Height)

	var (
		hasText, hasRect, hasMarker              bool
		hasSemibold, hasBold, hasItalic, hasMono bool
		hasStrike, hasLink, hasAccent            bool
		hasMuted, hasCanvas, hasBorder           bool
	)
	for _, primitive := range layout.Primitives {
		for _, value := range []float64{
			primitive.X, primitive.Y, primitive.X2, primitive.Y2,
			primitive.Width, primitive.Height, primitive.Radius, primitive.StrokeWidth,
		} {
			assert.False(t, math.IsNaN(value))
			assert.False(t, math.IsInf(value, 0))
		}
		assert.GreaterOrEqual(t, primitive.X, -0.5)
		assert.LessOrEqualf(t, primitive.X, float64(layout.Width)+0.5, "%+v", primitive)
		assert.GreaterOrEqual(t, primitive.Y, -0.5)
		assert.LessOrEqual(t, primitive.Y, float64(layout.Height)+0.5)
		switch primitive.Kind {
		case textmeasure.MarkdownTextPrimitive:
			hasText = true
		case textmeasure.MarkdownRectPrimitive:
			hasRect = true
		}
		switch primitive.Font {
		case textmeasure.MarkdownFontSemibold:
			hasSemibold = true
		case textmeasure.MarkdownFontBold:
			hasBold = true
		case textmeasure.MarkdownFontItalic:
			hasItalic = true
		case textmeasure.MarkdownFontMono:
			hasMono = true
		}
		hasMarker = hasMarker || (primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.Width == 5 && primitive.Height == 5)
		hasStrike = hasStrike || (primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.Height == 1 && primitive.FillRole == textmeasure.MarkdownColorForeground)
		hasLink = hasLink || primitive.Link == "https://example.com/a?x=1&y=2"
		hasAccent = hasAccent || primitive.FillRole == textmeasure.MarkdownColorAccent
		hasMuted = hasMuted || primitive.FillRole == textmeasure.MarkdownColorMuted
		hasCanvas = hasCanvas || primitive.FillRole == textmeasure.MarkdownColorCanvas || primitive.FillRole == textmeasure.MarkdownColorCanvasSubtle
		hasBorder = hasBorder || primitive.FillRole == textmeasure.MarkdownColorBorder || primitive.FillRole == textmeasure.MarkdownColorBorderMuted ||
			primitive.StrokeRole == textmeasure.MarkdownColorBorder || primitive.StrokeRole == textmeasure.MarkdownColorBorderMuted
	}
	assert.True(t, hasText)
	assert.True(t, hasRect)
	assert.True(t, hasMarker)
	assert.True(t, hasSemibold)
	assert.True(t, hasBold)
	assert.True(t, hasItalic)
	assert.True(t, hasMono)
	assert.True(t, hasStrike)
	assert.True(t, hasLink)
	assert.True(t, hasAccent)
	assert.True(t, hasMuted)
	assert.True(t, hasCanvas)
	assert.True(t, hasBorder)

	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
	assert.NotContains(t, fragment, "foreignObject")
	assert.NotContains(t, fragment, "http://www.w3.org/1999/xhtml")
	assert.Contains(t, fragment, `<g class="md md-native">`)
	assert.Contains(t, fragment, `<text`)
	assert.Contains(t, fragment, `<rect`)
	assert.Contains(t, fragment, `href="https://example.com/a?x=1&amp;y=2"`)

	wrapped := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">` + fragment + `</svg>`
	decoder := xml.NewDecoder(strings.NewReader(wrapped))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
	}
}

func TestLayoutMarkdownAllHeadingsAndBreak(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("# h1\n## h2\n### h3\n#### h4\n##### h5\n###### h6\n\nfirst<br>second", ruler, nil, nil, 20)
	require.NoError(t, err)

	fontSizes := make(map[float64]bool)
	var firstY, secondY float64
	for _, primitive := range layout.Primitives {
		if primitive.Kind != textmeasure.MarkdownTextPrimitive {
			continue
		}
		fontSizes[primitive.FontSize] = true
		if primitive.Text == "first" {
			firstY = primitive.Y
		}
		if primitive.Text == "second" {
			secondY = primitive.Y
		}
	}
	for _, want := range []float64{40, 30, 25, 20, 17} {
		assert.Truef(t, fontSizes[want], "missing heading font size %v", want)
	}
	assert.Greater(t, secondY, firstY)
}

func TestLayoutMarkdownOrderedListMarkers(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("3. outer\n   1. nested\n      1. deep", ruler, nil, nil, 16)
	require.NoError(t, err)
	var markers []string
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && strings.HasSuffix(primitive.Text, ".") {
			markers = append(markers, primitive.Text)
		}
	}
	assert.Contains(t, markers, "3.")
	assert.Contains(t, markers, "i.")
	assert.Contains(t, markers, "a.")
}

func TestLayoutMarkdownUnorderedListMarkers(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("- outer\n  - nested\n    - deep", ruler, nil, nil, 16)
	require.NoError(t, err)
	var markers []textmeasure.MarkdownPrimitive
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownRectPrimitive {
			markers = append(markers, primitive)
		}
	}
	require.Len(t, markers, 3)
	assert.Equal(t, []float64{15, 47, 79}, []float64{markers[0].X, markers[1].X, markers[2].X})
	for _, marker := range markers {
		assert.Equal(t, 5.0, marker.Width)
		assert.Equal(t, 5.0, marker.Height)
	}
	assert.Equal(t, 2.5, markers[0].Radius)
	assert.Equal(t, textmeasure.MarkdownColorForeground, markers[0].FillRole)
	assert.Equal(t, 2.5, markers[1].Radius)
	assert.Equal(t, textmeasure.MarkdownColorForegroundStroke, markers[1].StrokeRole)
	assert.Zero(t, markers[2].Radius)
	assert.Equal(t, textmeasure.MarkdownColorForeground, markers[2].FillRole)
}

func TestLayoutMarkdownUnorderedMarkerSizeMatchesBrowserAtFontSizes(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	for _, test := range []struct {
		fontSize int
		side     float64
	}{{8, 3}, {10, 3}, {16, 5}, {32, 10}} {
		layout, err := textmeasure.LayoutMarkdown("- item", ruler, nil, nil, test.fontSize)
		require.NoError(t, err)
		var markers []textmeasure.MarkdownPrimitive
		for _, primitive := range layout.Primitives {
			if primitive.Kind == textmeasure.MarkdownRectPrimitive {
				markers = append(markers, primitive)
			}
		}
		require.Len(t, markers, 1, "font size %d", test.fontSize)
		assert.Equal(t, test.side, markers[0].Width, "font size %d", test.fontSize)
		assert.Equal(t, test.side, markers[0].Height, "font size %d", test.fontSize)
		assert.Equal(t, test.side/2, markers[0].Radius, "font size %d", test.fontSize)
	}
}

func TestLayoutMarkdownHeadingListMarkerUsesFirstLineBaseline(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	for i, want := range []struct {
		markerY, baselineY float64
	}{{48, 55}, {41, 47}, {37, 44}, {35, 40}, {35, 37}, {35, 37}} {
		markdown := "- " + strings.Repeat("#", i+1) + " Heading item"
		layout, err := textmeasure.LayoutMarkdown(markdown, ruler, nil, nil, 16)
		require.NoError(t, err)
		var marker *textmeasure.MarkdownPrimitive
		var headingLines []textmeasure.MarkdownPrimitive
		for j := range layout.Primitives {
			primitive := &layout.Primitives[j]
			if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.Width == 5 && primitive.Height == 5 {
				marker = primitive
			}
			if primitive.Kind == textmeasure.MarkdownTextPrimitive && strings.Contains("Heading item", primitive.Text) {
				headingLines = append(headingLines, *primitive)
			}
		}
		require.NotNil(t, marker)
		require.NotEmpty(t, headingLines)
		var headingText []string
		for _, primitive := range headingLines {
			headingText = append(headingText, primitive.Text)
		}
		assert.Equal(t, "Heading item", strings.Join(headingText, " "), markdown)
		assert.Equal(t, want.markerY, marker.Y, markdown)
		assert.Equal(t, want.baselineY, headingLines[0].Y, markdown)
		if i == 5 {
			require.Len(t, headingLines, 2)
			assert.Equal(t, 54.0, headingLines[1].Y, markdown)
		}
	}

	ordered, err := textmeasure.LayoutMarkdown("1. # Heading item", ruler, nil, nil, 16)
	require.NoError(t, err)
	var markerY, headingY float64
	for _, primitive := range ordered.Primitives {
		if primitive.Kind != textmeasure.MarkdownTextPrimitive {
			continue
		}
		if primitive.Text == "1." {
			markerY = primitive.Y
		}
		if primitive.Text == "Heading item" {
			headingY = primitive.Y
		}
	}
	assert.Equal(t, 55.0, markerY)
	assert.Equal(t, headingY, markerY)
}

func TestLayoutMarkdownLongOrderedMarkerPreservesFixedIndent(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("123456789. ok", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 49, layout.Width)
	assert.Equal(t, 24, layout.Height)
	var marker, content *textmeasure.MarkdownPrimitive
	for i := range layout.Primitives {
		primitive := &layout.Primitives[i]
		if primitive.Kind != textmeasure.MarkdownTextPrimitive {
			continue
		}
		switch primitive.Text {
		case "123456789.":
			marker = primitive
		case "ok":
			content = primitive
		}
	}
	require.NotNil(t, marker)
	require.NotNil(t, content)
	assert.Less(t, marker.X, 0.0) // CSS clips a marker wider than its fixed 2em box.
	assert.Equal(t, 32.0, content.X)
}

func TestLayoutMarkdownEmptyListPreservesLegacyViewport(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	for _, markdown := range []string{"*", "-", "1."} {
		layout, err := textmeasure.LayoutMarkdown(markdown, ruler, nil, nil, 16)
		require.NoError(t, err)
		assert.Equal(t, 32, layout.Width, markdown)
		assert.Zero(t, layout.Height, markdown)
		// Chromium creates a marker line box, but the established zero-height
		// foreignObject viewport clips it. Native SVG keeps the same contract.
		require.NotEmpty(t, layout.Primitives, markdown)
	}
}

func TestLayoutMarkdownEmptyListItemDoesNotShiftLegacyViewport(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("- first\n-\n- third", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 65, layout.Width)
	assert.Equal(t, 56, layout.Height)
	var third *textmeasure.MarkdownPrimitive
	for i := range layout.Primitives {
		if layout.Primitives[i].Kind == textmeasure.MarkdownTextPrimitive && layout.Primitives[i].Text == "third" {
			third = &layout.Primitives[i]
		}
	}
	require.NotNil(t, third)
	assert.Greater(t, third.Y, float64(layout.Height))
}

func TestLayoutMarkdownRestoresRulerAndUsesCustomFonts(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)
	ruler.LineHeightFactor = 2.25
	fontFamily := d2fonts.HandDrawn
	monoFontFamily := d2fonts.SourceCodePro

	_, err = textmeasure.LayoutMarkdown("body `code`", ruler, &fontFamily, &monoFontFamily, 24)
	require.NoError(t, err)
	assert.Equal(t, 2.25, ruler.LineHeightFactor)
}

func TestLayoutMarkdownUsesMonoRolesForMonoBaseFont(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)
	monoFontFamily := d2fonts.SourceCodePro

	layout, err := textmeasure.LayoutMarkdown("# heading\n\nbody **bold** *italic*", ruler, &monoFontFamily, &monoFontFamily, 16)
	require.NoError(t, err)
	fonts := make(map[textmeasure.MarkdownFontRole]bool)
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive {
			fonts[primitive.Font] = true
		}
	}
	assert.True(t, fonts[textmeasure.MarkdownFontMono])
	assert.True(t, fonts[textmeasure.MarkdownFontMonoSemibold])
	assert.True(t, fonts[textmeasure.MarkdownFontMonoBold])
	assert.True(t, fonts[textmeasure.MarkdownFontMonoItalic])
	assert.False(t, fonts[textmeasure.MarkdownFontRegular])
}

func TestLayoutMarkdownEmpty(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	for _, markdown := range []string{"", " ", "\n\n"} {
		layout, err := textmeasure.LayoutMarkdown(markdown, ruler, nil, nil, 16)
		require.NoError(t, err)
		assert.Zero(t, layout.Width)
		assert.Zero(t, layout.Height)
		assert.Empty(t, layout.Primitives)
	}
}

func TestLayoutMarkdownPreservesLegacyViewportOrigin(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("plain", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Len(t, layout.Primitives, 1)
	assert.Equal(t, 34, layout.Width)
	assert.Equal(t, 24, layout.Height)
	assert.Zero(t, layout.Primitives[0].X)
	assert.Equal(t, 18.0, layout.Primitives[0].Y)
}

func TestLayoutMarkdownUsesCSSFontBoxBaseline(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	body, err := textmeasure.LayoutMarkdown("plain", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Len(t, body.Primitives, 1)
	// Source Sans Pro's rounded CSS font box is 16px ascent + 4px descent.
	// In a 24px line box that places the baseline 18px from its top.
	assert.InDelta(t, 18.0, body.Primitives[0].Y, 0.001)

	pre, err := textmeasure.LayoutMarkdown("```\nplain\n```", ruler, nil, nil, 16)
	require.NoError(t, err)
	var code *textmeasure.MarkdownPrimitive
	for i := range pre.Primitives {
		if pre.Primitives[i].Kind == textmeasure.MarkdownTextPrimitive {
			code = &pre.Primitives[i]
			break
		}
	}
	require.NotNil(t, code)
	// Source Code Pro at 13.6px rounds to a 13px ascent + 4px descent.
	// The pre line box is 19.72px with 16px of inner top padding.
	assert.InDelta(t, 16+(19.72-17)/2+13, code.Y, 0.001)
}

func TestLayoutMarkdownReservesCSSFallbackAdvance(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	plain, err := textmeasure.LayoutMarkdown("xxxx", ruler, nil, nil, 16)
	require.NoError(t, err)
	emoji, err := textmeasure.LayoutMarkdown("🐵🐵🐵🐵", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Len(t, emoji.Primitives, 4)
	assert.Equal(t, []float64{0, 20, 40, 0}, []float64{
		emoji.Primitives[0].X, emoji.Primitives[1].X, emoji.Primitives[2].X, emoji.Primitives[3].X,
	})
	assert.Equal(t, []float64{18, 18, 18, 42}, []float64{
		emoji.Primitives[0].Y, emoji.Primitives[1].Y, emoji.Primitives[2].Y, emoji.Primitives[3].Y,
	})
	for _, primitive := range emoji.Primitives {
		assert.Equal(t, 20.0, primitive.Width)
	}
	assert.Equal(t, plain.Height, emoji.Height)
	assert.Equal(t, 79, emoji.Width)
	assert.Greater(t, emoji.Width, plain.Width)
}

func TestLayoutMarkdownCSSFallbackIgnoresVariationSelectorOnText(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	plain, err := textmeasure.LayoutMarkdown("xx", ruler, nil, nil, 16)
	require.NoError(t, err)
	withSelector, err := textmeasure.LayoutMarkdown("x\ufe0fx", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Len(t, plain.Primitives, 1)
	require.Len(t, withSelector.Primitives, 1)
	assert.InDelta(t, plain.Primitives[0].Width, withSelector.Primitives[0].Width, 0.001)
	assert.Equal(t, plain.Primitives[0].Y, withSelector.Primitives[0].Y)
}

func TestLayoutMarkdownImagesDoNotResizeLegacyViewport(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)
	markdown := "| Project | Priority | Progress | Due Date | Owner |\n" +
		"|---------|:--------:|:--------:|:---------|:------|\n" +
		"| Alpha | HIGH | ![p](https://progress.com/80) 80% | 2024-04-01 | Alice |\n" +
		"| Beta | MEDIUM | ![p](https://progress.com/45) 45% | 2024-05-15 | Bob |\n" +
		"| Gamma | LOW | ![p](https://progress.com/20) 20% | 2024-06-30 | Carol |"

	layout, err := textmeasure.LayoutMarkdown(markdown, ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 425, layout.Width)
	assert.Equal(t, 150, layout.Height)
	assert.Contains(t, layout.Corpus, "80%")
}

func TestLayoutMarkdownDoesNotSplitEmojiZWJGraphemeAtLineBreak(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)
	fontFamily := d2fonts.HandDrawn

	layout, err := textmeasure.LayoutMarkdown("🐵🐵🐵🐵 🛡️ ☁️ 👩🏼‍❤️‍👨🏼", ruler, &fontFamily, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 156, layout.Width)
	assert.Equal(t, 24, layout.Height)

	var familyEmoji []textmeasure.MarkdownPrimitive
	for _, primitive := range layout.Primitives {
		if strings.Contains(primitive.Text, "👩") || strings.Contains(primitive.Text, "👨") {
			familyEmoji = append(familyEmoji, primitive)
		}
	}
	require.Len(t, familyEmoji, 1)
	assert.Equal(t, "👩🏼‍❤️‍👨🏼", familyEmoji[0].Text)
	assert.Equal(t, 20.0, familyEmoji[0].Width)
	assert.Equal(t, 0.0, familyEmoji[0].X)
	assert.Equal(t, 40.0, familyEmoji[0].Y)
}

func TestLayoutMarkdownHeadingCodeMatchesLegacyClipping(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("# `mono heading`", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Positive(t, layout.Height)

	var codeBackgrounds []textmeasure.MarkdownPrimitive
	var codeTexts []textmeasure.MarkdownPrimitive
	for i := range layout.Primitives {
		primitive := &layout.Primitives[i]
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
			codeBackgrounds = append(codeBackgrounds, *primitive)
		}
		if primitive.Kind == textmeasure.MarkdownTextPrimitive {
			codeTexts = append(codeTexts, *primitive)
		}
	}
	assert.Equal(t, 222, layout.Width)
	assert.Equal(t, 58, layout.Height)
	require.Len(t, codeBackgrounds, 2)
	assert.Equal(t, 0.0, codeBackgrounds[0].Y)
	assert.Equal(t, 40.0, codeBackgrounds[1].Y)
	assert.Greater(t, codeBackgrounds[1].Y+codeBackgrounds[1].Height, float64(layout.Height))
	require.Len(t, codeTexts, 2)
	assert.Equal(t, []string{"mono", "heading"}, []string{codeTexts[0].Text, codeTexts[1].Text})
	for _, primitive := range codeTexts {
		assert.Equal(t, 32.0, primitive.FontSize)
		assert.Equal(t, textmeasure.MarkdownFontMono, primitive.Font)
	}
}

func TestLayoutMarkdownWrapsHeadingCodeAtUnicodeBreaks(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	tests := []struct {
		markdown string
		width    int
		texts    []string
	}{
		{markdown: "# `long-long`", width: 173, texts: []string{"long-", "long"}},
		{markdown: "# `日本語日本語`", width: 124, texts: []string{"日本語", "日本語"}},
	}
	for _, test := range tests {
		layout, err := textmeasure.LayoutMarkdown(test.markdown, ruler, nil, nil, 16)
		require.NoError(t, err)
		assert.Equal(t, test.width, layout.Width)
		assert.Equal(t, 58, layout.Height)
		var texts, backgrounds []textmeasure.MarkdownPrimitive
		for _, primitive := range layout.Primitives {
			if primitive.Kind == textmeasure.MarkdownTextPrimitive {
				texts = append(texts, primitive)
			}
			if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
				backgrounds = append(backgrounds, primitive)
			}
		}
		var lineTexts []string
		var lineYs []float64
		for _, primitive := range texts {
			if len(lineYs) == 0 || primitive.Y != lineYs[len(lineYs)-1] {
				lineYs = append(lineYs, primitive.Y)
				lineTexts = append(lineTexts, primitive.Text)
			} else {
				lineTexts[len(lineTexts)-1] += primitive.Text
			}
		}
		assert.Equal(t, test.texts, lineTexts)
		assert.Equal(t, []float64{31, 71}, lineYs)
		require.Len(t, backgrounds, 2)
		assert.Equal(t, []float64{0, 40}, []float64{backgrounds[0].Y, backgrounds[1].Y})
	}
}

func TestLayoutMarkdownSoftHyphenIsConditional(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("# `long\u00adlong`", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 173, layout.Width)
	assert.Equal(t, 58, layout.Height)
	var texts, backgrounds []textmeasure.MarkdownPrimitive
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive {
			texts = append(texts, primitive)
		}
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
			backgrounds = append(backgrounds, primitive)
		}
	}
	require.Len(t, texts, 1)
	assert.Equal(t, "long\u00adlong", texts[0].Text)
	assert.Equal(t, 31.0, texts[0].Y)
	require.Len(t, backgrounds, 1)
	assert.Equal(t, 0.0, backgrounds[0].Y)

	wrapped, err := textmeasure.LayoutMarkdown("# `long\u00adlonglong`", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 238, wrapped.Width)
	var wrappedBackgrounds []textmeasure.MarkdownPrimitive
	var hyphen *textmeasure.MarkdownPrimitive
	for i := range wrapped.Primitives {
		primitive := &wrapped.Primitives[i]
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
			wrappedBackgrounds = append(wrappedBackgrounds, *primitive)
		}
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "-" {
			hyphen = primitive
		}
	}
	require.Len(t, wrappedBackgrounds, 2)
	assert.InDelta(t, 93.817, wrappedBackgrounds[0].Width, 0.001)
	require.NotNil(t, hyphen)
	assert.InDelta(t, 32.0/3, hyphen.Width, 0.001)
	assert.True(t, hyphen.TextLength)
	assert.Contains(t, wrapped.SVG(textmeasure.MarkdownSVGOptions{}), `textLength="10.667" lengthAdjust="spacingAndGlyphs"`)
}

func TestLayoutMarkdownMovesLeadingCodePaddingWithWrappedFragment(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("# long-`abcdefghijk`", ruler, nil, nil, 16)
	require.NoError(t, err)
	var backgrounds []textmeasure.MarkdownPrimitive
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
			backgrounds = append(backgrounds, primitive)
		}
	}
	require.Len(t, backgrounds, 1)
	assert.Equal(t, 0.0, backgrounds[0].X)
	assert.Equal(t, 40.0, backgrounds[0].Y)
}

func TestLayoutMarkdownFencedCodePreservesCSSLineHeightAndBlankLines(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("```\nfirst\n\nsecond\n```", ruler, nil, nil, 16)
	require.NoError(t, err)
	var firstY, secondY float64
	for _, primitive := range layout.Primitives {
		if primitive.Kind != textmeasure.MarkdownTextPrimitive {
			continue
		}
		switch primitive.Text {
		case "first":
			firstY = primitive.Y
		case "second":
			secondY = primitive.Y
		}
	}
	require.NotZero(t, firstY)
	require.NotZero(t, secondY)
	wantLineHeight := textmeasure.LineHeight_pre * textmeasure.FontSize_pre_code_em * 16
	assert.InDelta(t, 2*wantLineHeight, secondY-firstY, 0.001)
}

func TestLayoutMarkdownFencedCodeExpandsTabsAtFourColumnStops(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("```\na\tb\na   b\n\tindented\n```", ruler, nil, nil, 16)
	require.NoError(t, err)
	var texts []string
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive {
			texts = append(texts, primitive.Text)
		}
	}
	assert.Equal(t, []string{"a   b", "a   b", "    indented"}, texts)
}

func TestLayoutMarkdownEmptyTableRowsMatchPaintedHeight(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| | |\n|---|---|\n| | |", ruler, nil, nil, 16)
	require.NoError(t, err)
	var horizontalBorders []float64
	var interiorBorders []float64
	var lastBorderBottom float64
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorBorder {
			lastBorderBottom = math.Max(lastBorderBottom, primitive.Y+primitive.Height)
			if primitive.Width == 55 && primitive.Height == 1 {
				horizontalBorders = append(horizontalBorders, primitive.Y)
			}
			if primitive.Width == 1 && primitive.Height == 13 && primitive.X != 0 && primitive.X != 54 {
				interiorBorders = append(interiorBorders, primitive.X)
			}
		}
	}
	assert.Equal(t, 50, layout.Height) // Established graph-layout viewport.
	assert.Equal(t, []float64{0, 13, 26}, horizontalBorders)
	assert.Equal(t, []float64{27, 27}, interiorBorders)
	assert.Equal(t, 27.0, lastBorderBottom)
}

func TestLayoutMarkdownMixedEmptyTableMatchesBrowserRows(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| H1 | H2 |\n|---|---|\n| | |\n| a | |\n| | b |", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 93, layout.Width)
	assert.Equal(t, 137, layout.Height)
	var horizontalBorders []float64
	var bottom float64
	for _, primitive := range layout.Primitives {
		if primitive.Kind != textmeasure.MarkdownRectPrimitive || primitive.FillRole != textmeasure.MarkdownColorBorder {
			continue
		}
		bottom = math.Max(bottom, primitive.Y+primitive.Height)
		if primitive.Width == 93 && primitive.Height == 1 {
			horizontalBorders = append(horizontalBorders, primitive.Y)
		}
	}
	assert.Equal(t, []float64{0, 33, 46, 79, 112}, horizontalBorders)
	assert.Equal(t, 113.0, bottom)
}

func TestLayoutMarkdownConstrainedCodeTableMatchesBrowser(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| H |\n|---|\n| `日本語日本語` |", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 90, layout.Width)
	assert.Equal(t, 76, layout.Height) // Established graph-layout viewport.

	var canvas textmeasure.MarkdownPrimitive
	var codeBackgrounds []textmeasure.MarkdownPrimitive
	var codeBaselines []float64
	var horizontalBorders []float64
	for _, primitive := range layout.Primitives {
		switch {
		case primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorCanvas:
			canvas = primitive
		case primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted:
			codeBackgrounds = append(codeBackgrounds, primitive)
		case primitive.Kind == textmeasure.MarkdownTextPrimitive && strings.Contains("日本語", primitive.Text):
			codeBaselines = append(codeBaselines, primitive.Y)
		case primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorBorder && primitive.Width == 90 && primitive.Height == 1:
			horizontalBorders = append(horizontalBorders, primitive.Y)
		}
	}
	assert.Equal(t, 90.0, canvas.Width)
	assert.Equal(t, 85.0, canvas.Height) // CSS table paint overflows the legacy viewport.
	require.Len(t, codeBackgrounds, 2)
	assert.InDelta(t, 14, codeBackgrounds[0].X, 0.01)
	assert.InDelta(t, 38.28, codeBackgrounds[0].Y, 0.01)
	assert.InDelta(t, 59.84, codeBackgrounds[0].Width, 0.01)
	assert.InDelta(t, 14, codeBackgrounds[1].X, 0.01)
	assert.InDelta(t, 57.28, codeBackgrounds[1].Y, 0.01)
	assert.InDelta(t, 32.64, codeBackgrounds[1].Width, 0.01)
	assert.Equal(t, []float64{54, 54, 54, 54, 73, 73}, codeBaselines)
	assert.Equal(t, []float64{0, 33, 84}, horizontalBorders)
}

func TestLayoutMarkdownTableColumnMinContentAndVerticalCentering(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| H1 | H2 |\n|---|---|\n| `日本語日本語` | x |", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 136, layout.Width)
	assert.Equal(t, 76, layout.Height)

	var h2, bodyX []textmeasure.MarkdownPrimitive
	var interiorBorders []textmeasure.MarkdownPrimitive
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "H2" {
			h2 = append(h2, primitive)
		}
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "x" {
			bodyX = append(bodyX, primitive)
		}
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorBorder && primitive.X == 89 && primitive.Width == 1 {
			interiorBorders = append(interiorBorders, primitive)
		}
	}
	require.Len(t, h2, 1) // The flexible code column shrinks; H2 remains intact.
	assert.InDelta(t, 103.17, h2[0].X, 0.03)
	require.Len(t, bodyX, 1)
	assert.InDelta(t, 103.17, bodyX[0].X, 0.03)
	assert.Equal(t, 65.0, bodyX[0].Y) // Centered against the two-line code cell.
	require.Len(t, interiorBorders, 2)
	assert.Equal(t, 33.0, interiorBorders[0].Height)
	assert.Equal(t, 51.0, interiorBorders[1].Height)
}

func TestLayoutMarkdownCenteredTableAlignsEachWrappedLine(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| H |\n|:---:|\n| `日本語日本語` |", ruler, nil, nil, 16)
	require.NoError(t, err)
	var backgrounds []textmeasure.MarkdownPrimitive
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
			backgrounds = append(backgrounds, primitive)
		}
	}
	require.Len(t, backgrounds, 2)
	assert.InDelta(t, 15.08, backgrounds[0].X, 0.01)
	assert.InDelta(t, 28.68, backgrounds[1].X, 0.01)
}

func TestLayoutMarkdownTableDefaultsAndFollowingMarginMatchBrowser(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	centered, err := textmeasure.LayoutMarkdown("| H |\n|---|\n| longer |", ruler, nil, nil, 16)
	require.NoError(t, err)
	var heading textmeasure.MarkdownPrimitive
	for _, primitive := range centered.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "H" {
			heading = primitive
		}
	}
	assert.InDelta(t, 30.22, heading.X, 0.01) // th defaults to centered.
	assert.Equal(t, 23.0, heading.Y)

	withFollowingParagraph, err := textmeasure.LayoutMarkdown("| H |\n|---|\n| a |\n\nafter", ruler, nil, nil, 16)
	require.NoError(t, err)
	var tableHeight, afterBaseline float64
	for _, primitive := range withFollowingParagraph.Primitives {
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorCanvas {
			tableHeight = primitive.Height
		}
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "after" {
			afterBaseline = primitive.Y
		}
	}
	assert.Equal(t, 67.0, tableHeight)
	assert.Equal(t, 101.0, afterBaseline) // 16px collapsed table bottom margin.
}

func TestLayoutMarkdownTableColorEmojiFallbackMatchesBrowser(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| Status |\n|---|\n| ✅ |\n| ⚠️ |\n| 🔴 |", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 73, layout.Width)
	assert.Equal(t, 150, layout.Height)
	var emoji []textmeasure.MarkdownPrimitive
	var borders []float64
	var canvasHeight float64
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && (primitive.Text == "✅" || primitive.Text == "⚠️" || primitive.Text == "🔴") {
			emoji = append(emoji, primitive)
		}
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorCanvas {
			canvasHeight = primitive.Height
		}
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorBorder && primitive.Width == 72 && primitive.Height == 1 {
			borders = append(borders, primitive.Y)
		}
	}
	require.Len(t, emoji, 3)
	for _, primitive := range emoji {
		assert.Equal(t, 20.0, primitive.Width)
	}
	assert.Equal(t, []float64{59, 98, 137}, []float64{emoji[0].Y, emoji[1].Y, emoji[2].Y})
	assert.Equal(t, 151.0, canvasHeight)
	assert.Equal(t, []float64{0, 33, 72, 111, 150}, borders)

	impact, err := textmeasure.LayoutMarkdown("| Impact | Next |\n|---|---|\n| 🔴 High | x |", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 142, impact.Width)
	assert.Equal(t, 76, impact.Height)
	var impactText, nextText textmeasure.MarkdownPrimitive
	var dividerFound bool
	for _, primitive := range impact.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "🔴 High" {
			impactText = primitive
		}
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "Next" {
			nextText = primitive
		}
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorBorder && primitive.X == 81 && primitive.Width == 1 {
			dividerFound = true
		}
	}
	assert.InDelta(t, 54.344, impactText.Width, 0.001)
	assert.Equal(t, 59.0, impactText.Y)
	assert.InDelta(t, 95.344, nextText.X, 0.001)
	assert.True(t, dividerFound)

	warnings, err := textmeasure.LayoutMarkdown("| Mark |\n|---|\n| ⚠ |\n| ⚠️ |", ruler, nil, nil, 16)
	require.NoError(t, err)
	var warningBorders []float64
	for _, primitive := range warnings.Primitives {
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorBorder && primitive.Width == 63 && primitive.Height == 1 {
			warningBorders = append(warningBorders, primitive.Y)
		}
	}
	// Text-presentation ⚠ uses a 24px normal line box; VS16 selects the
	// 26px color-emoji fallback line box.
	assert.Equal(t, []float64{0, 33, 70, 109}, warningBorders)
}

func TestLayoutMarkdownMixedTableCellPreservesLegacyViewport(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| H1 | H2 |\n|---|---|\n| x `日本語日本語` | y |", ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, 136, layout.Width)
	assert.Equal(t, 97, layout.Height)
}

func TestLayoutMarkdownTableAlignmentUsesTextAdvance(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("Left | Center | Right\n:--- | :----: | ----:\na | **b** | `c`\nlonger | middle | 123", ruler, nil, nil, 16)
	require.NoError(t, err)
	wantX := map[string]float64{
		"Left": 14, "Center": 84.469, "Right": 158.047,
		"a": 14, "middle": 84.063, "123": 170.891,
	}
	for _, primitive := range layout.Primitives {
		want, ok := wantX[primitive.Text]
		if !ok {
			continue
		}
		assert.InDelta(t, want, primitive.X, 0.1, primitive.Text)
		delete(wantX, primitive.Text)
	}
	assert.Empty(t, wantX)
}

func TestLayoutMarkdownTableStripesRestartPerSection(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("| H |\n|---|\n| first |\n| second |", ruler, nil, nil, 16)
	require.NoError(t, err)
	var stripes []textmeasure.MarkdownPrimitive
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorCanvasSubtle {
			stripes = append(stripes, primitive)
		}
	}
	require.Len(t, stripes, 1)
	assert.Greater(t, stripes[0].Y, float64(layout.Height)/2)
}

func TestMarkdownSVGRejectsDangerousLinks(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("[safe](https://example.com) [relative](../docs) [bad](javascript:alert(1))", ruler, nil, nil, 16)
	require.NoError(t, err)
	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
	assert.Contains(t, fragment, `href="https://example.com"`)
	assert.Contains(t, fragment, `href="../docs"`)
	assert.Contains(t, fragment, `>bad</text>`)
	assert.NotContains(t, strings.ToLower(fragment), "javascript:")

	manual := (&textmeasure.MarkdownLayout{Primitives: []textmeasure.MarkdownPrimitive{{
		Kind: textmeasure.MarkdownTextPrimitive, Text: "bad", Link: "java\tscript:alert(1)",
	}}}).SVG(textmeasure.MarkdownSVGOptions{})
	assert.NotContains(t, manual, "<a ")
}

func TestLayoutMarkdownOmitsImagesWithoutFetching(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown(`before ![build passing](https://example.invalid/status.svg) after`, ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Equal(t, "before after", layout.Corpus)
	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
	assert.NotContains(t, fragment, "example.invalid")
	assert.NotContains(t, fragment, "build passing")
	assert.NotContains(t, fragment, "<image")
}

func TestLayoutMarkdownRejectsUnsupportedHTML(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	_, err = textmeasure.LayoutMarkdown(`<video src="movie.mp4"></video>`, ruler, nil, nil, 16)
	require.ErrorContains(t, err, "does not support HTML element <video>")
}

func TestMarkdownSVGOptionsOverrideThemeFontsAndLinks(t *testing.T) {
	t.Parallel()
	layout := &textmeasure.MarkdownLayout{
		Width: 10, Height: 10,
		Primitives: []textmeasure.MarkdownPrimitive{{
			Kind: textmeasure.MarkdownTextPrimitive, X: 1, Y: 9, Text: "x", Link: "https://example.com",
			Font: textmeasure.MarkdownFontSemibold, FontSize: 8,
			FillRole: textmeasure.MarkdownColorAccent,
		}},
	}
	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{
		Class: "custom-md",
		RolePaint: map[textmeasure.MarkdownColorRole]textmeasure.MarkdownSVGPaint{
			textmeasure.MarkdownColorAccent: {Class: "accent-N1", Color: "#123456"},
		},
		FontClasses: map[textmeasure.MarkdownFontRole]string{
			textmeasure.MarkdownFontSemibold: "custom-semibold",
		},
		DisableLinks: true,
		Underline:    true,
	})
	assert.Contains(t, fragment, `class="custom-md"`)
	assert.Contains(t, fragment, `class="md-text custom-semibold accent-N1"`)
	assert.Contains(t, fragment, `fill="#123456"`)
	assert.Contains(t, fragment, `text-decoration="underline"`)
	assert.NotContains(t, fragment, `<a `)
}

func TestMarkdownSVGCombinesUnderlineAndStrikethrough(t *testing.T) {
	t.Parallel()
	layout := &textmeasure.MarkdownLayout{Primitives: []textmeasure.MarkdownPrimitive{{
		Kind:       textmeasure.MarkdownTextPrimitive,
		Text:       "both",
		Decoration: textmeasure.MarkdownTextDecorationLineThrough,
	}}}
	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{Underline: true})
	assert.Contains(t, fragment, `text-decoration="underline line-through"`)
}

func TestLayoutMarkdownPaintsContinuousStrikethrough(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown(`~~a [b](https://example.com) **c**~~`, ruler, nil, nil, 16)
	require.NoError(t, err)
	var (
		strike    *textmeasure.MarkdownPrimitive
		textWidth float64
		hasLink   bool
	)
	for i := range layout.Primitives {
		primitive := &layout.Primitives[i]
		if primitive.Kind == textmeasure.MarkdownTextPrimitive {
			textWidth += primitive.Width
			hasLink = hasLink || primitive.Link == "https://example.com"
		}
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.Height == 1 && primitive.FillRole == textmeasure.MarkdownColorForeground {
			require.Nil(t, strike, "strikethrough must be one continuous rule")
			strike = primitive
		}
	}
	require.NotNil(t, strike)
	assert.True(t, hasLink)
	assert.Zero(t, strike.X)
	assert.InDelta(t, textWidth, strike.Width, 0.001)
	assert.Equal(t, 12.0, strike.Y)
}

func TestLayoutMarkdownCollapsesInlineWhitespace(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	tests := []struct {
		name     string
		markdown string
		want     string
		lines    int
	}{
		{name: "run", markdown: "hello     world", want: "hello world", lines: 1},
		{name: "soft break between styles", markdown: "**foo**\n*bar*", want: "foobar", lines: 2},
		{name: "run between styles", markdown: "**foo**   *bar*", want: "foo bar", lines: 1},
		{name: "nested edges", markdown: "left  **bold**  right", want: "left bold right", lines: 1},
		{name: "leading link whitespace", markdown: "a[ b](https://example.com)c", want: "abc", lines: 2},
		{name: "trailing link whitespace", markdown: "a[b ](https://example.com)c", want: "abc", lines: 2},
		{name: "inline code", markdown: "a`b   c`d", want: "ab cd", lines: 1},
		{name: "non-breaking spaces", markdown: "left&nbsp;&nbsp;right", want: "left\u00a0\u00a0right", lines: 1},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			layout, err := textmeasure.LayoutMarkdown(tc.markdown, ruler, nil, nil, 16)
			require.NoError(t, err)
			var rendered strings.Builder
			lineYs := make(map[float64]bool)
			for _, primitive := range layout.Primitives {
				if primitive.Kind == textmeasure.MarkdownTextPrimitive {
					rendered.WriteString(primitive.Text)
					lineYs[primitive.Y] = true
				}
			}
			assert.Equal(t, tc.want, rendered.String())
			assert.Equal(t, tc.want, layout.Corpus)
			assert.Len(t, lineYs, tc.lines)
		})
	}
}

func TestLayoutMarkdownComposesWeightItalicAndCode(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("# *heading italic* ***heading bold italic***\n\n***both*** **_reverse_** **`bold code`** *`italic code`* ***`both code`***", ruler, nil, nil, 16)
	require.NoError(t, err)
	primitives := make(map[string]textmeasure.MarkdownPrimitive)
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive {
			primitives[primitive.Text] = primitive
		}
	}

	require.Contains(t, primitives, "heading italic")
	assert.Equal(t, textmeasure.MarkdownFontItalic, primitives["heading italic"].Font)
	assert.False(t, primitives["heading italic"].SyntheticItalic)
	require.Contains(t, primitives, "heading bold italic")
	assert.Equal(t, textmeasure.MarkdownFontBold, primitives["heading bold italic"].Font)
	assert.True(t, primitives["heading bold italic"].SyntheticItalic)
	require.Contains(t, primitives, "both")
	assert.Equal(t, textmeasure.MarkdownFontBold, primitives["both"].Font)
	assert.True(t, primitives["both"].SyntheticItalic)
	require.Contains(t, primitives, "reverse")
	assert.Equal(t, textmeasure.MarkdownFontItalic, primitives["reverse"].Font)
	assert.True(t, primitives["reverse"].SyntheticBold)
	require.Contains(t, primitives, "bold code")
	assert.Equal(t, textmeasure.MarkdownFontMono, primitives["bold code"].Font)
	assert.True(t, primitives["bold code"].SyntheticBold)
	assert.False(t, primitives["bold code"].SyntheticItalic)
	require.Contains(t, primitives, "italic code")
	assert.Equal(t, textmeasure.MarkdownFontMono, primitives["italic code"].Font)
	assert.True(t, primitives["italic code"].SyntheticItalic)
	assert.False(t, primitives["italic code"].SyntheticBold)
	require.Contains(t, primitives, "both code")
	assert.Equal(t, textmeasure.MarkdownFontMono, primitives["both code"].Font)
	assert.True(t, primitives["both code"].SyntheticBold)
	assert.True(t, primitives["both code"].SyntheticItalic)

	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
	assert.Contains(t, fragment, `class="md-text text-bold`)
	assert.Contains(t, fragment, `font-weight="bold"`)
	assert.Contains(t, fragment, `font-style="italic"`)
}

func TestMarkdownSVGPreservesLinkTitles(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown(`[label](https://example.com "help <&>")`, ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Len(t, layout.Primitives, 1)
	assert.Equal(t, "help <&>", layout.Primitives[0].LinkTitle)
	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
	assert.Contains(t, fragment, `<a href="https://example.com" xlink:href="https://example.com"><title>help &lt;&amp;&gt;</title>`)

	disabled := layout.SVG(textmeasure.MarkdownSVGOptions{DisableLinks: true})
	assert.NotContains(t, disabled, `<a `)
	assert.NotContains(t, disabled, `<title>`)
}

func TestMarkdownSVGLinkedInlineCodeCoversPaddedBackground(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("[`code`](https://example.com \"help\")", ruler, nil, nil, 16)
	require.NoError(t, err)
	var background *textmeasure.MarkdownPrimitive
	for i := range layout.Primitives {
		primitive := &layout.Primitives[i]
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
			background = primitive
			break
		}
	}
	require.NotNil(t, background)
	assert.InDelta(t, 43.5, background.Width, 0.02)
	assert.Equal(t, "https://example.com", background.Link)
	assert.Equal(t, "help", background.LinkTitle)

	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
	assert.Contains(t, fragment, `<a href="https://example.com" xlink:href="https://example.com"><title>help</title><rect`)
	disabled := layout.SVG(textmeasure.MarkdownSVGOptions{DisableLinks: true})
	assert.NotContains(t, disabled, `<a `)
}

func TestLayoutMarkdownRealAndHistoricalRegressionCorpus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		markdown string
		contains []string
	}{
		{
			name: "src onboarding template",
			markdown: "# Level 1: Shapes and connections ([docs](https://d2lang.com/tour/shapes))\n\n" +
				"The fundamental building blocks of diagrams are shapes and connections.\n\n" +
				"Try the following:\n  1. Declare a new shape `Data lake`.\\\n    Continue on the next line.\n  2. Change the shape.\n  3. Select all and delete.",
			contains: []string{"Level 1", "docs", "1.", "Data lake", "Continue on the next line."},
		},
		{
			name:     "src nested fenced code",
			markdown: "# md title\n\n````d2\nx -> y\n\nx: ||md\n\n# inner md title\n\n```d2\nx -> y\n\nx: |md\n# inner inner md title\n|\n\ny -> z\n```\n||\n\ny -> z\n````",
			contains: []string{"md title", "```d2", "inner inner md title", "y -> z"},
		},
		{
			name:     "src entity and strong headings",
			markdown: "# **Web Service**\n## - Single instance\n## - 512&thinsp;MB memory",
			contains: []string{"Web Service", "Single instance", "512\u2009MB memory"},
		},
		{
			name:     "issue 1842 sketch commas and list",
			markdown: "# hey\nthere is ok, yes, sure\n\n- 1\n- 2\n- 3",
			contains: []string{"there is ok, yes, sure", "1", "3"},
		},
		{
			name: "issue 2340 content after table and second table",
			markdown: "# Status\n\n| Status | Count |\n| --- | --- |\n| Done | 42 |\n| Todo | 17 |\n\n" +
				"**status**\n\n| Status | Count |\n| --- | --- |\n| Done | 42 |\n| Todo | 17 |",
			contains: []string{"Status", "Done", "42", "status"},
		},
		{
			name:     "issues 749 and 2680 emoji runs",
			markdown: "🐵🐵🐵🐵 🛡️ ☁️ 👩🏼‍❤️‍👨🏼",
			contains: []string{"🐵🐵🐵🐵", "🛡️", "☁️", "👩🏼‍❤️‍👨🏼"},
		},
		{
			name:     "issue 2734 zero width characters",
			markdown: "\u200b\u200b\u200b\u200b\u200bx\u200b\u200b\u200b\u200b\u200b",
			contains: []string{"x"},
		},
		{
			name:     "src indented code tabs and following content",
			markdown: "# a header\n\na line of text and an\n\n\t{\n\t\tindented: \"block\",\n\t\tof: \"json\",\n\t}\n\nwalk into a bar.",
			contains: []string{"a header", "{", "    indented", "walk into a bar."},
		},
	}

	for _, fontFamily := range []d2fonts.FontFamily{d2fonts.SourceSansPro, d2fonts.HandDrawn} {
		fontFamily := fontFamily
		for _, tc := range tests {
			tc := tc
			t.Run(string(fontFamily)+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				ruler, err := textmeasure.NewRuler()
				require.NoError(t, err)
				layout, err := textmeasure.LayoutMarkdown(tc.markdown, ruler, &fontFamily, nil, 16)
				require.NoError(t, err)
				require.Positive(t, layout.Width)
				require.Positive(t, layout.Height)
				for _, want := range tc.contains {
					assert.Contains(t, layout.Corpus, want)
				}
				for _, primitive := range layout.Primitives {
					assert.GreaterOrEqual(t, primitive.X, 0.0)
					assert.GreaterOrEqual(t, primitive.Y, 0.0)
					if tc.name != "issues 749 and 2680 emoji runs" {
						assert.LessOrEqual(t, primitive.X, float64(layout.Width))
						assert.LessOrEqual(t, primitive.Y, float64(layout.Height))
						if primitive.Kind == textmeasure.MarkdownRectPrimitive {
							assert.LessOrEqual(t, primitive.X+primitive.Width, float64(layout.Width)+0.001)
							assert.LessOrEqual(t, primitive.Y+primitive.Height, float64(layout.Height)+0.001)
						}
					}
				}
				fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
				assert.NotContains(t, fragment, "foreignObject")
				var parsed any
				require.NoError(t, xml.Unmarshal([]byte(`<svg xmlns="http://www.w3.org/2000/svg">`+fragment+`</svg>`), &parsed))
			})
		}
	}
}

func FuzzLayoutMarkdownNative(f *testing.F) {
	for _, seed := range []string{
		"plain **bold** *italic*",
		"**foo**\n*bar*",
		"a[ b](https://example.com \"title\")c",
		"***bold italic*** **`bold code`**",
		"# heading\n\n- outer\n  - inner",
		"```\nfirst\n\n\tsecond\n```",
		"| A | B |\n|---|---|\n| 1 | 2 |",
		"🐵 e\u0301 \u200b",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, markdown string) {
		if len(markdown) > 64*1024 {
			t.Skip()
		}
		ruler, err := textmeasure.NewRuler()
		require.NoError(t, err)
		layout, err := textmeasure.LayoutMarkdown(markdown, ruler, nil, nil, 16)
		if err != nil {
			return
		}
		assert.GreaterOrEqual(t, layout.Width, 0)
		assert.GreaterOrEqual(t, layout.Height, 0)
		for _, primitive := range layout.Primitives {
			for _, value := range []float64{primitive.X, primitive.Y, primitive.X2, primitive.Y2, primitive.Width, primitive.Height} {
				assert.False(t, math.IsNaN(value))
				assert.False(t, math.IsInf(value, 0))
			}
		}
		fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
		var parsed any
		require.NoError(t, xml.Unmarshal([]byte(`<svg xmlns="http://www.w3.org/2000/svg">`+fragment+`</svg>`), &parsed))
	})
}
