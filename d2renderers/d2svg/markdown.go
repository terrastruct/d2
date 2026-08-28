package d2svg

import (
	"fmt"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2themes"
	"github.com/d2lang/d2/lib/color"
	"github.com/d2lang/d2/lib/textmeasure"
)

// markdownRenderer owns the shared measurement state used while rendering a
// diagram. LayoutMarkdown returns both the dimensions used by graph layout and
// the primitives painted here, so native SVG output cannot drift onto a
// separate browser layout path.
type markdownRenderer struct {
	ruler          *textmeasure.Ruler
	rulerErr       error
	fontFamily     *d2fonts.FontFamily
	monoFontFamily *d2fonts.FontFamily
	inlineTheme    *d2themes.Theme
	corpus         strings.Builder
}

func newMarkdownRenderer(fontFamily, monoFontFamily *d2fonts.FontFamily, inlineTheme *d2themes.Theme) *markdownRenderer {
	if fontFamily == nil {
		family := d2fonts.SourceSansPro
		fontFamily = &family
	}
	if monoFontFamily == nil {
		family := d2fonts.SourceCodePro
		monoFontFamily = &family
	}
	return &markdownRenderer{
		fontFamily:     fontFamily,
		monoFontFamily: monoFontFamily,
		inlineTheme:    inlineTheme,
	}
}

func (r *markdownRenderer) layout(markdown, fontName string, fontSize int) (*textmeasure.MarkdownLayout, error) {
	fontFamily := r.fontFamily
	if strings.EqualFold(fontName, "mono") {
		fontFamily = r.monoFontFamily
	} else if fontName != "" && !strings.EqualFold(fontName, "default") {
		if family := fontToFamily(strings.ToLower(fontName)); family != nil {
			fontFamily = family
		}
	}

	return r.layoutWithFont(markdown, fontFamily, r.monoFontFamily, fontSize)
}

func (r *markdownRenderer) layoutWithFont(markdown string, fontFamily, monoFontFamily *d2fonts.FontFamily, fontSize int) (*textmeasure.MarkdownLayout, error) {
	if r.ruler == nil && r.rulerErr == nil {
		r.ruler, r.rulerErr = textmeasure.NewRuler()
	}
	if r.rulerErr != nil {
		return nil, r.rulerErr
	}
	layout, err := textmeasure.LayoutMarkdown(markdown, r.ruler, fontFamily, monoFontFamily, fontSize)
	if err != nil {
		return nil, err
	}
	r.corpus.WriteString(layout.Corpus)
	return layout, nil
}

func (r *markdownRenderer) render(
	layout *textmeasure.MarkdownLayout,
	x, y float64,
	width, height int,
	foreground, background string,
	disableLinks bool,
	underline bool,
) string {
	var out strings.Builder
	fmt.Fprintf(&out, `<svg x="%f" y="%f" width="%d" height="%d" viewBox="0 0 %d %d" overflow="hidden">`,
		x, y, width, height, width, height,
	)

	if background != "" && background != color.None && background != "transparent" {
		backgroundEl := d2themes.NewThemableElement("rect", r.inlineTheme)
		backgroundEl.Width = float64(width)
		backgroundEl.Height = float64(height)
		backgroundEl.Fill = background
		out.WriteString(backgroundEl.Render())
	}

	rolePaint := map[textmeasure.MarkdownColorRole]textmeasure.MarkdownSVGPaint{
		textmeasure.MarkdownColorForeground:   r.fillPaint(foreground),
		textmeasure.MarkdownColorMuted:        r.fillPaint(color.N2),
		textmeasure.MarkdownColorAccent:       r.fillPaint(color.B2),
		textmeasure.MarkdownColorBorder:       r.strokePaint(color.B1),
		textmeasure.MarkdownColorBorderMuted:  r.strokePaint(color.B2),
		textmeasure.MarkdownColorCanvas:       r.fillPaint(color.N7),
		textmeasure.MarkdownColorCanvasSubtle: r.fillPaint(color.N6),
		textmeasure.MarkdownColorNeutralMuted: r.fillPaint(color.N6),
	}
	out.WriteString(layout.SVG(textmeasure.MarkdownSVGOptions{
		Class:        "md md-native",
		RolePaint:    rolePaint,
		DisableLinks: disableLinks,
		Underline:    underline,
	}))
	out.WriteString(`</svg>`)
	return out.String()
}

func (r *markdownRenderer) fillPaint(value string) textmeasure.MarkdownSVGPaint {
	return r.paint(value, "fill")
}

func (r *markdownRenderer) strokePaint(value string) textmeasure.MarkdownSVGPaint {
	return r.paint(value, "stroke")
}

func (r *markdownRenderer) paint(value, property string) textmeasure.MarkdownSVGPaint {
	if value == "" {
		value = color.N1
	}
	if color.IsThemeColor(value) {
		paint := textmeasure.MarkdownSVGPaint{Class: property + "-" + value}
		if r.inlineTheme != nil {
			paint.Color = d2themes.ResolveThemeColor(*r.inlineTheme, value)
		}
		return paint
	}
	if color.IsGradient(value) {
		value = fmt.Sprintf("url('#%s')", color.UniqueGradientID(value))
	}
	return textmeasure.MarkdownSVGPaint{Color: value}
}

func (r *markdownRenderer) generatedCorpus() string {
	return r.corpus.String()
}
