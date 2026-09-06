package d2svg_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2lib"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/textmeasure"
	"github.com/d2lang/util-go/assert"
)

func TestMarkdownRendersAsNativeSVG(t *testing.T) {
	t.Parallel()

	script := `
md: ||md
  # Native Markdown

  - **bold**, *italic*, ***both***, ~~struck~~, and [linked](https://example.com/?a=1&b=2 "help <&>")
  - ` + "`inline code`" + ` and **` + "`bold code`" + `**

  > quoted

  | A | B |
  | - | - |
  | 1 | 2 |
||
md.tooltip: "**tooltip**" {
  near: top-right
}
mono_md: |md
  # mono heading
|
mono_md.style.font: mono
outer_link: |md
  [inner](https://inner.example)
|
outer_link.link: https://outer.example
underlined: |md
  Kubernetes Integrations
| {
  style.underline: true
}
wide: |md
  # Centered
|
wide.width: 400
wide.height: 200
a -> b: |md
  **edge label**
|
`

	ruler, err := textmeasure.NewRuler()
	assert.Success(t, err)
	layoutResolver := func(string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}
	fontFamily := d2fonts.HandDrawn
	monoFontFamily := d2fonts.SourceCodePro
	themeID := int64(200)
	renderOpts := &d2svg.RenderOpts{ThemeID: &themeID}
	diagram, _, err := d2lib.Compile(log.WithTB(context.Background(), t), script, &d2lib.CompileOptions{
		Ruler:          ruler,
		LayoutResolver: layoutResolver,
		FontFamily:     &fontFamily,
		MonoFontFamily: &monoFontFamily,
	}, renderOpts)
	assert.Success(t, err)

	out, err := d2svg.Render(diagram, renderOpts)
	assert.Success(t, err)
	var parsed any
	assert.Success(t, xml.Unmarshal(out, &parsed))

	svg := string(out)
	if strings.Contains(svg, "<foreignObject") {
		t.Fatal("Markdown output must use native SVG, not foreignObject")
	}
	if !strings.Contains(svg, `class="md md-native"`) {
		t.Fatal("Markdown output is missing its native SVG group")
	}
	if !strings.Contains(svg, `<text`) {
		t.Fatal("Markdown output is missing native SVG text")
	}
	if !strings.Contains(svg, `href="https://example.com/?a=1&amp;b=2"`) {
		t.Fatal("Markdown output did not preserve its sanitized link")
	}
	if !strings.Contains(svg, `<title>help &lt;&amp;&gt;</title>`) {
		t.Fatal("Markdown output did not preserve and escape its link title")
	}
	if !strings.Contains(svg, `font-style="italic"`) {
		t.Fatal("Markdown output did not compose bold and italic styles")
	}
	if !strings.Contains(svg, `font-weight="bold"`) {
		t.Fatal("Markdown output did not preserve bold emphasis inside inline code")
	}
	if !strings.Contains(svg, `class="positioned-tooltip"`) {
		t.Fatal("positioned tooltip was not rendered")
	}
	if strings.Contains(svg, "https://inner.example") || !strings.Contains(svg, `href="https://outer.example"`) {
		t.Fatal("outer D2 link did not take precedence over nested Markdown link")
	}
	if !strings.Contains(svg, `text-decoration="underline"`) {
		t.Fatal("Markdown label did not preserve style.underline")
	}
	var wide *d2target.Shape
	for i := range diagram.Shapes {
		if diagram.Shapes[i].ID == "wide" {
			wide = &diagram.Shapes[i]
			break
		}
	}
	if wide == nil {
		t.Fatal("wide Markdown shape was not compiled")
	}
	wantCenteredViewport := fmt.Sprintf(
		`<svg x="%g" y="%g" width="%d" height="%d"`,
		float64(wide.Pos.X)+(float64(wide.Width)-float64(wide.LabelWidth))/2,
		float64(wide.Pos.Y)+(float64(wide.Height)-float64(wide.LabelHeight))/2,
		wide.LabelWidth,
		wide.LabelHeight,
	)
	if !strings.Contains(svg, wantCenteredViewport) {
		t.Fatalf("unset Markdown label position was not centered: want %s", wantCenteredViewport)
	}
	tooltipLayout, err := textmeasure.LayoutMarkdown("**tooltip**", ruler, &fontFamily, &monoFontFamily, d2fonts.FONT_SIZE_M)
	assert.Success(t, err)
	tooltipViewport := fmt.Sprintf(`width="%d" height="%d" viewBox="0 0 %d %d"`, tooltipLayout.Width, tooltipLayout.Height, tooltipLayout.Width, tooltipLayout.Height)
	tooltipStart := strings.Index(svg, `class="positioned-tooltip"`)
	if !strings.Contains(svg[tooltipStart:], tooltipViewport) {
		t.Fatalf("tooltip viewport was not measured with the diagram font: want %s", tooltipViewport)
	}
	// Stop at the nested Markdown viewport, so shorter coordinates cannot
	// bring the following appendix icon into the tooltip paint assertion.
	tooltipEnd := tooltipStart + strings.Index(svg[tooltipStart:], "<svg ")
	tooltipPrefix := svg[tooltipStart:tooltipEnd]
	if strings.Contains(tooltipPrefix, `fill="white" stroke="#DEE1EB"`) || !strings.Contains(tooltipPrefix, "fill-N7") {
		t.Fatal("positioned tooltip did not use theme-aware background colors")
	}
	for _, fontClass := range []string{"text-semibold", "text-bold", "text-italic", "text-mono", "text-mono-semibold"} {
		if !strings.Contains(svg, "."+fontClass+" {") {
			t.Fatalf("native Markdown did not embed the %s font face", fontClass)
		}
	}
}
