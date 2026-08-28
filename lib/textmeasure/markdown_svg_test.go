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
	assert.Contains(t, layout.Corpus, "•")

	width, height, err := textmeasure.MeasureMarkdown(markdown, ruler, nil, nil, textmeasure.MarkdownFontSize)
	require.NoError(t, err)
	assert.Equal(t, width, layout.Width)
	assert.Equal(t, height, layout.Height)

	var (
		hasText, hasRect, hasLine                bool
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
		case textmeasure.MarkdownLinePrimitive:
			hasLine = true
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
		hasStrike = hasStrike || primitive.Decoration == textmeasure.MarkdownTextDecorationLineThrough
		hasLink = hasLink || primitive.Link == "https://example.com/a?x=1&y=2"
		hasAccent = hasAccent || primitive.FillRole == textmeasure.MarkdownColorAccent
		hasMuted = hasMuted || primitive.FillRole == textmeasure.MarkdownColorMuted
		hasCanvas = hasCanvas || primitive.FillRole == textmeasure.MarkdownColorCanvas || primitive.FillRole == textmeasure.MarkdownColorCanvasSubtle
		hasBorder = hasBorder || primitive.StrokeRole == textmeasure.MarkdownColorBorder || primitive.StrokeRole == textmeasure.MarkdownColorBorderMuted
	}
	assert.True(t, hasText)
	assert.True(t, hasRect)
	assert.True(t, hasLine)
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
	assert.Contains(t, fragment, `<line`)
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
	var markers []string
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && strings.Contains("•◦▪", primitive.Text) {
			markers = append(markers, primitive.Text)
		}
	}
	assert.Equal(t, []string{"•", "◦", "▪"}, markers)
}

func TestLayoutMarkdownListMarkersStayInsideViewport(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	for _, markdown := range []string{"123456789. ok", "*"} {
		layout, err := textmeasure.LayoutMarkdown(markdown, ruler, nil, nil, 16)
		require.NoError(t, err)
		require.Positive(t, layout.Width)
		require.Positive(t, layout.Height)
		require.NotEmpty(t, layout.Primitives)
		for _, primitive := range layout.Primitives {
			assert.GreaterOrEqual(t, primitive.X, 0.0, markdown)
			assert.LessOrEqual(t, primitive.X, float64(layout.Width), markdown)
			assert.GreaterOrEqual(t, primitive.Y, 0.0, markdown)
			assert.LessOrEqual(t, primitive.Y, float64(layout.Height), markdown)
		}
	}
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

func TestLayoutMarkdownAddsInkSafety(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("plain", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.NotEmpty(t, layout.Primitives)
	for _, primitive := range layout.Primitives {
		assert.GreaterOrEqual(t, primitive.X, 6.0)
		assert.GreaterOrEqual(t, primitive.Y, 4.0)
	}
}

func TestLayoutMarkdownUsesCSSFontBoxBaseline(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	body, err := textmeasure.LayoutMarkdown("plain", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Len(t, body.Primitives, 1)
	// Source Sans Pro's rounded CSS font box is 16px ascent + 4px descent.
	// In a 24px line box that places the baseline 18px from its top, plus the
	// four-pixel vertical safety translation.
	assert.InDelta(t, 22.0, body.Primitives[0].Y, 0.001)

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
	assert.InDelta(t, 4+16+(19.72-17)/2+13, code.Y, 0.001)
}

func TestLayoutMarkdownPadsHostFallbackGraphemes(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	plain, err := textmeasure.LayoutMarkdown("xxxx", ruler, nil, nil, 16)
	require.NoError(t, err)
	emoji, err := textmeasure.LayoutMarkdown("🐵🐵🐵🐵", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Len(t, emoji.Primitives, 1)
	assert.GreaterOrEqual(t, emoji.Primitives[0].X, 17.0)
	assert.GreaterOrEqual(t, emoji.Primitives[0].Y, 11.0)
	assert.Greater(t, emoji.Height, plain.Height)
	assert.GreaterOrEqual(t, emoji.Width, 130)
}

func TestLayoutMarkdownHeadingCodeStaysInsideViewport(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("# `mono heading`", ruler, nil, nil, 16)
	require.NoError(t, err)
	require.Positive(t, layout.Height)

	var codeBackground *textmeasure.MarkdownPrimitive
	for i := range layout.Primitives {
		primitive := &layout.Primitives[i]
		if primitive.Kind == textmeasure.MarkdownRectPrimitive && primitive.FillRole == textmeasure.MarkdownColorNeutralMuted {
			codeBackground = primitive
		}
	}
	require.NotNil(t, codeBackground)
	assert.GreaterOrEqual(t, codeBackground.Y, 0.0)
	assert.LessOrEqual(t, codeBackground.Y+codeBackground.Height, float64(layout.Height))
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive && primitive.Text == "mono heading" {
			assert.Equal(t, 32.0, primitive.FontSize)
			assert.Equal(t, textmeasure.MarkdownFontMonoSemibold, primitive.Font)
		}
	}
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
	var lastBorderY float64
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownLinePrimitive {
			lastBorderY = math.Max(lastBorderY, math.Max(primitive.Y, primitive.Y2))
		}
	}
	assert.LessOrEqual(t, lastBorderY, float64(layout.Height))
	assert.LessOrEqual(t, float64(layout.Height)-lastBorderY, 5.0)
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

func TestLayoutMarkdownUsesImageAltTextWithoutFetching(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown(`before ![build passing](https://example.invalid/status.svg) after`, ruler, nil, nil, 16)
	require.NoError(t, err)
	assert.Contains(t, layout.Corpus, "build passing")
	assert.NotContains(t, layout.SVG(textmeasure.MarkdownSVGOptions{}), "example.invalid")
	assert.Contains(t, layout.SVG(textmeasure.MarkdownSVGOptions{}), ">build passing</text>")
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

func TestLayoutMarkdownCollapsesInlineWhitespace(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{name: "run", markdown: "hello     world", want: "hello world"},
		{name: "soft break between styles", markdown: "**foo**\n*bar*", want: "foo bar"},
		{name: "run between styles", markdown: "**foo**   *bar*", want: "foo bar"},
		{name: "nested edges", markdown: "left  **bold**  right", want: "left bold right"},
		{name: "leading link whitespace", markdown: "a[ b](https://example.com)c", want: "a bc"},
		{name: "trailing link whitespace", markdown: "a[b ](https://example.com)c", want: "ab c"},
		{name: "inline code", markdown: "a`b   c`d", want: "ab cd"},
		{name: "non-breaking spaces", markdown: "left&nbsp;&nbsp;right", want: "left\u00a0\u00a0right"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			layout, err := textmeasure.LayoutMarkdown(tc.markdown, ruler, nil, nil, 16)
			require.NoError(t, err)
			var rendered strings.Builder
			for _, primitive := range layout.Primitives {
				if primitive.Kind == textmeasure.MarkdownTextPrimitive {
					rendered.WriteString(primitive.Text)
				}
			}
			assert.Equal(t, tc.want, rendered.String())
			assert.Equal(t, tc.want, layout.Corpus)
		})
	}
}

func TestLayoutMarkdownComposesWeightItalicAndCode(t *testing.T) {
	t.Parallel()
	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	layout, err := textmeasure.LayoutMarkdown("# *heading italic* ***heading bold italic***\n\n***both*** **`bold code`** *`italic code`*", ruler, nil, nil, 16)
	require.NoError(t, err)
	primitives := make(map[string]textmeasure.MarkdownPrimitive)
	for _, primitive := range layout.Primitives {
		if primitive.Kind == textmeasure.MarkdownTextPrimitive {
			primitives[primitive.Text] = primitive
		}
	}

	require.Contains(t, primitives, "heading italic")
	assert.Equal(t, textmeasure.MarkdownFontSemibold, primitives["heading italic"].Font)
	assert.True(t, primitives["heading italic"].SyntheticItalic)
	require.Contains(t, primitives, "heading bold italic")
	assert.Equal(t, textmeasure.MarkdownFontBold, primitives["heading bold italic"].Font)
	assert.True(t, primitives["heading bold italic"].SyntheticItalic)
	require.Contains(t, primitives, "both")
	assert.Equal(t, textmeasure.MarkdownFontBold, primitives["both"].Font)
	assert.True(t, primitives["both"].SyntheticItalic)
	require.Contains(t, primitives, "bold code")
	assert.Equal(t, textmeasure.MarkdownFontMonoBold, primitives["bold code"].Font)
	assert.False(t, primitives["bold code"].SyntheticItalic)
	require.Contains(t, primitives, "italic code")
	assert.Equal(t, textmeasure.MarkdownFontMonoItalic, primitives["italic code"].Font)
	assert.False(t, primitives["italic code"].SyntheticItalic)

	fragment := layout.SVG(textmeasure.MarkdownSVGOptions{})
	assert.Contains(t, fragment, `class="md-text text-bold`)
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
			contains: []string{"there is ok, yes, sure", "•", "3"},
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
					assert.LessOrEqual(t, primitive.X, float64(layout.Width))
					assert.GreaterOrEqual(t, primitive.Y, 0.0)
					assert.LessOrEqual(t, primitive.Y, float64(layout.Height))
					if primitive.Kind == textmeasure.MarkdownRectPrimitive {
						assert.LessOrEqual(t, primitive.X+primitive.Width, float64(layout.Width)+0.001)
						assert.LessOrEqual(t, primitive.Y+primitive.Height, float64(layout.Height)+0.001)
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
