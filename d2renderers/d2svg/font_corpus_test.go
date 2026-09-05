package d2svg

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/textmeasure"
)

func TestCollectFontCorpora(t *testing.T) {
	tests := []struct {
		name, source string
		want         fontCorpora
	}{
		{"empty", `<g><path d="M0 0L1 1"/></g>`, fontCorpora{}},
		{"entities", `<text class="text">À &amp; &#x3a9; &#160; &#x1f600; e&#769; &lt;&gt;&quot;&apos;</text>`, fontCorpora{"text": "À & Ω \u00a0 😀 e\u0301 <>\"'"}},
		{"inheritance", `<g class="text-mono-bold"><text>A<tspan class="text">B<tspan>C</tspan></tspan>D</text><text>E</text></g>`, fontCorpora{"text": "BC", "text-mono-bold": "ADE"}},
		{"same_element_cascade", `<text class="text-italic text-semibold text-bold text">A</text><text class="text-mono-italic text-mono-bold text-mono">B</text>`, fontCorpora{"text-italic": "A", "text-mono-italic": "B"}},
		{"exact_tokens", `<text class="fill-N1 text-mono-bold other-text-mono">A</text>`, fontCorpora{"text-mono-bold": "A"}},
		{"class_whitespace", "<text class=\"fill-N1\ttext-bold\ntext\">A</text>", fontCorpora{"text-bold": "A"}},
		{"unicode_not_class_separator", `<text class="text-mono&#160;other text">A</text><text class="text-mono&#8195;other text">B</text><text class="text">C</text>`, fontCorpora{"text": "ABC"}},
		{"synthetic_axes", `<text class="text-italic" font-weight="bold" style="font-style:italic;font-size:16px">A</text>`, fontCorpora{"text-italic": "A"}},
		{"metadata", `<g class="text-bold"><title>not glyphs</title><desc>not glyphs</desc><text>A<title>not glyphs</title>B</text></g>`, fontCorpora{"text-bold": "AB"}},
		{"links_and_hidden_runs", `<g class="text"><text><a xlink:href="https://example.com">A</a>B</text><text style="display:none">C</text><text opacity="0">D</text></g>`, fontCorpora{"text": "ABCD"}},
		{"path_latex", `<svg xmlns="http://www.w3.org/2000/svg" style="vertical-align: -0.18ex;"><g data-mml-node="math"><path d="M0 0L1 1"/></g></svg><text class="text">A</text>`, fontCorpora{"text": "A"}},
		{"cdata", `<text class="text"><![CDATA[A<&]]>B</text>`, fontCorpora{"text": "A<&B"}},
		{"preserved_spaces", `<text class="text" xml:space="preserve"> A <tspan> </tspan> B </text>`, fontCorpora{"text": " A   B "}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := collectFontCorpora(tt.source)
			if !ok || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("collectFontCorpora = %#v, %v; want %#v, true", got, ok, tt.want)
			}
		})
	}
}

func TestCollectFontCorporaAndClasses(t *testing.T) {
	source := `<!-- <g class="fill-N2"/> --><g class="fill-N1 stroke&#45;B3">` +
		`<text class="text color-AB4">A<tspan class="text-bold">B</tspan></text>` +
		`<defs><path class="sketch-overlay-AB5 background-color-AA2"/></defs></g>`
	fonts, classes, ok := collectFontCorporaAndClasses(source)
	wantFonts := fontCorpora{"text": "A", "text-bold": "B"}
	wantClasses := map[string]bool{"fill-N1": true, "stroke-B3": true, "color-AB4": true, "sketch-overlay-AB5": true, "background-color-AA2": true}
	if !ok || !reflect.DeepEqual(fonts, wantFonts) || !reflect.DeepEqual(classes, wantClasses) {
		t.Fatalf("got %#v, %#v, %v; want %#v, %#v, true", fonts, classes, ok, wantFonts, wantClasses)
	}
}

func TestCollectedClassesLegacyThemeParity(t *testing.T) {
	dark := int64(200)
	full, err := ThemeCSS("d2-test", nil, &dark, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`<g class="fill-N1 stroke-B3 color-AA2"/>`,
		`<g class="fill-N1&#9;stroke-B3"/><g class="sketch-overlay-N7"/>`,
		`<g class="fill-N1&#160;stroke-B3"/>`,
		`<g class="fill-N1&#8195;stroke-B3"/>`,
		`<g class="fill-N1"/><g`,
		`<g class="fill-N1"/><style>.custom{fill:red}</style>`,
		`<use href="#local"/>`,
		`<SCRIPT>changeClasses()</SCRIPT>`,
		`<g ONLOAD="changeClasses()"/>`,
		`<g attributeName="class" to="fill-N2"/>`,
		`<USE HREF="external.svg#shape"/>`,
		`<a xlink:href=" JavaScript:changeClasses()"/>`,
		`<g xmlns:c="urn:custom" c:class="fill-N1"/>`,
	} {
		for _, background := range []string{`<rect class="fill-N7"/>`, `<rect`, `<rect class="stroke-B1"/>`} {
			source := `<text class="text">A</text>` + fragment
			_, classes, _ := collectFontCorporaAndClasses(source)
			var got string
			if classes == nil {
				got = pruneThemeCSS(full, source, background)
			} else {
				got = pruneThemeCSSWithClasses(full, classes, background)
			}
			if want := legacyPruneThemeCSS(full, source, background); got != want {
				t.Fatalf("fragment %q, background %q changed theme CSS", fragment, background)
			}
		}
	}
}

func TestCollectedClassesFallbackDoesNotChangeFontSelection(t *testing.T) {
	for _, fragment := range []string{
		`<SCRIPT/>`, `<g ONLOAD="changeClasses()"/>`,
		`<g attributeName="class" to="fill-N1"/>`,
		`<USE href="external.svg#shape"/>`,
		`<a href=" JavaScript:changeClasses()"/>`,
		`<g class="fill-N1&#160;stroke-B3"/>`,
	} {
		fonts, classes, ok := collectFontCorporaAndClasses(`<text class="text">A</text>` + fragment)
		if !ok || !reflect.DeepEqual(fonts, fontCorpora{"text": "A"}) || classes != nil {
			t.Fatalf("theme-only fallback changed font behavior for %q: %#v, %#v, %v", fragment, fonts, classes, ok)
		}
	}
	for _, source := range []string{`<g class="fill-N1"/><g`, `<g class="fill-N1"/><text>unknown font</text>`} {
		if _, classes, ok := collectFontCorporaAndClasses(source); ok || classes != nil {
			t.Fatalf("incomplete scan exposed partial classes for %q", source)
		}
	}
}

func TestCollectFontCorporaFallback(t *testing.T) {
	for _, source := range []string{
		`<text class="text">unterminated`,
		`<text class="text">&unknown;</text>`,
		`<text>unclassified</text>`,
		`<text class="prefix-text-bold">unknown class</text>`,
		`<text class="other text">legacy regular face absent</text>`,
		`<text class='text'>legacy regular face absent</text>`,
		`<text class="text" class="text-bold">ambiguous duplicate</text>`,
		`<text class="text" font-family="custom">override</text>`,
		`<g style="font-family: custom"><text class="text">override</text></g>`,
		`<text class="text" style="FONT: 12px custom">override</text>`,
		`<text class="text" style="font\2d family: custom">override</text>`,
		`<text class="text" style="font/**/-family: custom">override</text>`,
		`<text class="text" style="text-transform: uppercase">ß</text>`,
		`<text class="text" style="font-variant: small-caps">abc</text>`,
		`<text class="text" font-variant="small-caps">abc</text>`,
		`<text class="text" text-transform="uppercase">abc</text>`,
		`<text class="text" direction="rtl">(abc)</text>`,
		`<text class="text" unicode-bidi="bidi-override">(abc)</text>`,
		`<text class="text" style="direction: rtl">(abc)</text>`,
		`<text class="text" style="unicode-bidi: bidi-override">(abc)</text>`,
		`<text class="text" xmlns:custom="urn:custom" custom:class="text-bold">abc</text>`,
		`<text class="text" style="all: initial">override</text>`,
		`<text class="text" style="malformed declaration">A</text>`,
		`<text class="text" onclick="this.textContent='B'">A</text>`,
		`<style>.text { font-family: custom }</style><text class="text">A</text>`,
		`<script>void 0</script>`,
		`<foreignObject><p>HTML</p></foreignObject>`,
		`<text xmlns="urn:unknown" class="text">A</text>`,
		`<text class="text"><textPath href="#path">A</textPath></text>`,
		`<text class="text"><tref href="#text"/></text>`,
		`<use href="#text"/>`,
		`<text class="text">A<set attributeName="class" to="text-bold"/></text>`,
		`<animate attributeName="class" values="text;text-bold"/>`,
		`<?xml-stylesheet href="custom.css"?>`,
		`<!DOCTYPE svg>`,
	} {
		t.Run(source, func(t *testing.T) {
			if got, ok := collectFontCorpora(source); ok || got != nil {
				t.Fatalf("expected legacy fallback; got %#v, %v", got, ok)
			}
		})
	}
}

func TestFontCorporaCustomBaseStylesheetFallback(t *testing.T) {
	original := BaseStylesheet
	t.Cleanup(func() { BaseStylesheet = original })
	BaseStylesheet += `.diagram .text {font-family: "diagram-font-bold" !important}`
	if got, ok := collectFontCorpora(`<text class="text">A</text><text class="text-bold">B</text>`); ok || got != nil {
		t.Fatalf("custom stylesheet must retain whole-corpus embedding; got %#v, %v", got, ok)
	}
}

func TestFontCorporaGeneratedCode(t *testing.T) {
	style := styleAttr(map[chroma.TokenType]string{
		chroma.Keyword: `font-weight="bold" font-style="italic" `,
	}, chroma.Keyword)
	source := `<text class="text-mono">` + svgEscaper.Replace("\t<& ") +
		`<tspan ` + style + `>` + svgEscaper.Replace("é") + `</tspan>` + svgEscaper.Replace(" Ω") + `</text>`
	got, ok := collectFontCorpora(source)
	want := fontCorpora{"text-mono": "\u00a0\u00a0\u00a0\u00a0<&\u00a0\u00a0Ω", "text-mono-italic": "é"}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("generated code corpora = %#v, %v; want %#v", got, ok, want)
	}
}

func TestFontCorporaNativeMarkdownPrimitives(t *testing.T) {
	for _, baseFont := range []string{"", "mono"} {
		renderer := newMarkdownRenderer(nil, nil, nil)
		layout, err := renderer.layout("# Heading Ω\n\n- plain &amp; **bold** *italic* ***both***\n- `code` **`strong code`**\n\n| A | B |\n| - | - |\n| é | Ж |", baseFont, 16)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := collectFontCorpora(renderer.render(layout, 0, 0, layout.Width, layout.Height, "#000", "", false, false))
		if !ok {
			t.Fatal("generated native Markdown requested fallback")
		}
		want := fontCorpora{}
		for _, primitive := range layout.Primitives {
			if primitive.Kind != textmeasure.MarkdownTextPrimitive || primitive.Text == "" {
				continue
			}
			class := "text-" + string(primitive.Font)
			if primitive.Font == textmeasure.MarkdownFontRegular {
				class = "text"
			}
			want[class] += primitive.Text
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%q Markdown corpus differs from painted primitives: got %#v; want %#v", baseFont, got, want)
		}
	}
}

func TestAppendFontOnTrigger(t *testing.T) {
	for _, tt := range []struct {
		name, source, class string
		corpora             fontCorpora
		want                string
	}{
		{"legacy_trigger", "prefix-text-mono-bold", "text-mono", nil, "whole corpus"},
		{"legacy_no_trigger", "nothing", "text-mono", nil, ""},
		{"rendered", "ignored", "text-mono", fontCorpora{"text-mono": "only code"}, "only code"},
		{"unused", "text-mono-bold", "text-mono", fontCorpora{"text-mono-bold": "only bold"}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			appendFontOnTrigger(&out, tt.source, []string{"text-mono"}, tt.corpora, tt.class, "whole corpus", func(corpus string) string {
				calls++
				return corpus
			})
			wantCalls := 0
			if tt.want != "" {
				wantCalls = 1
			}
			if out.String() != tt.want || calls != wantCalls {
				t.Fatalf("got %q, %d callback calls; want %q, %d", out.String(), calls, tt.want, wantCalls)
			}
		})
	}
}

func TestRestrictFontCorporaPreservesMissingGlyphFallback(t *testing.T) {
	corpora := fontCorpora{"text-mono": "A\u00a0B", "text-mono-bold": "\u00a0", "text": "A", "text-bold": ""}
	got := restrictFontCorpora(corpora, "A B")
	want := fontCorpora{"text-mono": "A B", "text-mono-bold": "A B", "text": "A", "text-bold": ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v; want %#v", got, want)
	}
	if got := restrictFontCorpora(nil, "legacy corpus"); got != nil {
		t.Fatal("nil legacy fallback became a nonnil collected map")
	}
	var out bytes.Buffer
	appendFontOnTrigger(&out, "", nil, got, "text-bold", "", func(corpus string) string {
		if corpus != "" {
			t.Fatal("new glyph introduced into previously empty subset")
		}
		return "font-family declaration"
	})
	if out.String() == "" {
		t.Fatal("filtered empty font lost its family declaration")
	}
}

func TestFontCorporaAbsentNBSPUsesLegacyFace(t *testing.T) {
	family, mono := d2fonts.SourceSansPro, d2fonts.SourceCodePro
	source := `<text class="text-mono">A&#160;B</text>`
	corpora, ok := collectFontCorpora(source)
	if !ok {
		t.Fatal("unexpected fallback")
	}
	var got, want bytes.Buffer
	embedFonts(&got, "diagram", source, &family, &mono, "A B other legacy glyphs", corpora)
	legacyEmbedFonts(&want, "diagram", source, &family, &mono, "A B other legacy glyphs")
	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Fatal("NBSP fallback lost the legacy SPACE glyph or other fallback dependencies")
	}
}

func TestEmbedFontsUsesRenderedCorpora(t *testing.T) {
	family, mono := d2fonts.SourceSansPro, d2fonts.SourceCodePro
	source := `<text class="text">A&amp;B</text><text class="text-mono-bold">X</text>`
	corpora, ok := collectFontCorpora(source)
	if !ok {
		t.Fatal("unexpected fallback")
	}
	var out bytes.Buffer
	embedFonts(&out, "diagram", source, &family, &mono, "unused label markup A&BX", corpora)
	for _, want := range []string{
		family.Font(0, d2fonts.FONT_STYLE_REGULAR).GetEncodedSubset("A&B"),
		mono.Font(0, d2fonts.FONT_STYLE_BOLD).GetEncodedSubset("X"),
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatal("rendered character subset missing")
		}
	}
	if strings.Contains(out.String(), `font-family: diagram-font-mono;`) || strings.Count(out.String(), "@font-face") != 2 {
		t.Fatal("unused monospace face embedded by substring trigger")
	}
}

func TestFontCorporaUnicodeUsesLegacyFaces(t *testing.T) {
	for _, family := range []d2fonts.FontFamily{d2fonts.SourceSansPro, d2fonts.HandDrawn} {
		for _, text := range []string{"e\u0301", "abc אבג (1)", "가 가"} {
			mono := d2fonts.SourceCodePro
			source := `<text class="text">` + text + `</text><text class="text-bold">é</text><text class="text-mono-bold">Code</text>`
			corpora, ok := collectFontCorpora(source)
			if !ok {
				t.Fatal("unexpected source parsing fallback")
			}
			corpus := text + "é Code"
			var got, want bytes.Buffer
			embedFonts(&got, "diagram", source, &family, &mono, corpus, corpora)
			legacyEmbedFonts(&want, "diagram", source, &family, &mono, corpus)
			if !bytes.Equal(got.Bytes(), want.Bytes()) {
				t.Fatalf("%s / %q changed normalization, bidi, or face-presence dependencies", family, text)
			}
		}
	}
}

func TestRestrictFontCorporaConservativeAlphabet(t *testing.T) {
	for _, corpus := range []string{"Aé", "A\tB", "A\rB", "A\x00B", "A\x1fB", "A\x7fB"} {
		for _, corpora := range []fontCorpora{{"text": "A"}, {}} {
			if got := restrictFontCorpora(corpora, corpus); got != nil {
				t.Fatalf("%q did not preserve all legacy faces: %#v", corpus, got)
			}
		}
	}
	if got := restrictFontCorpora(fontCorpora{"text": "A"}, "A\nB"); !reflect.DeepEqual(got, fontCorpora{"text": "A"}) {
		t.Fatalf("ordinary multiline ASCII input lost specialization: %#v", got)
	}
}

func TestRenderAppendixFontCorporaLegacyParity(t *testing.T) {
	for _, appendixText := range [][2]string{{"**footer XYZ & <raw>**", "Pretty URL"}, {"**éЖ Ω & <raw>**", "Pretty Δ"}} {
		for _, family := range []d2fonts.FontFamily{d2fonts.SourceSansPro, d2fonts.HandDrawn} {
			for _, labels := range [][2]string{{"1", "ABC"}, {"", "ABC"}, {"1", ""}, {"", ""}} {
				diagram := d2target.NewDiagram()
				mono := d2fonts.SourceCodePro
				diagram.FontFamily, diagram.MonoFontFamily = &family, &mono
				for i, label := range labels {
					diagram.Shapes = append(diagram.Shapes, d2target.Shape{
						ID: label, Type: d2target.ShapeRectangle,
						Pos: d2target.Point{X: i * 150, Y: 0}, Width: 100, Height: 80,
						Opacity: 1, StrokeWidth: 2, Fill: "#ffffff", Stroke: "#000000",
						Text: d2target.Text{Label: label, FontSize: 16, LabelWidth: 40, LabelHeight: 20, Bold: i == 1},
					})
				}
				diagram.Shapes[0].Tooltip = appendixText[0]
				diagram.Shapes[1].Link = "https://example.com/appendix"
				diagram.Shapes[1].PrettyLink = appendixText[1]
				out, err := Render(diagram, nil)
				if err != nil {
					t.Fatal(err)
				}
				hash, err := diagram.HashID(nil)
				if err != nil {
					t.Fatal(err)
				}
				var want bytes.Buffer
				legacyEmbedFonts(&want, hash, string(out), &family, &mono, diagram.GetCorpus())
				start := bytes.Index(out, []byte(`<style type="text/css">`))
				end := bytes.Index(out[start:], []byte(`</style>`)) + start + len(`</style>`)
				if !bytes.Equal(out[start:end], want.Bytes()) {
					t.Fatalf("%s, labels %q: fonts differ from legacy before appendix adds new characters", family, labels)
				}
			}
		}
	}
}
