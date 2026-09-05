package d2svg

import (
	"bytes"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2fonts"
)

func TestEmbedFontsMatchesLegacy(t *testing.T) {
	for _, source := range []string{
		`<svg/>`,
		`<text class="text">Hello</text>`,
		`<text class="text another-class">Hello</text>`,
		`<text class="text-mono-bold">Code</text>`,
		`<text class="text text-semibold text-bold text-italic text-mono text-mono-semibold text-mono-bold text-mono-italic">All styles</text>`,
		`<text class="md md-text">&#x2022; A &amp; B</text><b>bold</b><strong>strong</strong><em>italic</em><dfn>definition</dfn><pre>pre</pre><code>code</code><kbd>key</kbd><samp>sample</samp>`,
		`text-underline text-link text-strikethrough text-italic text-bold text-mono sketch-overlay-bright sketch-overlay-normal sketch-overlay-dark text-animated`,
	} {
		for _, family := range []d2fonts.FontFamily{d2fonts.SourceSansPro, d2fonts.HandDrawn} {
			mono := d2fonts.SourceCodePro
			var want, got bytes.Buffer
			legacyEmbedFonts(&want, "diagram-hash", source, &family, &mono, "Hello Àéß Ω Ж 中 😀 e\u0301")
			EmbedFonts(&got, "diagram-hash", source, &family, &mono, "Hello Àéß Ω Ж 中 😀 e\u0301")
			if !bytes.Equal(got.Bytes(), want.Bytes()) {
				t.Fatalf("font CSS differs for %s and source %q", family, source)
			}
		}
	}
}

func TestAppendOnTriggerLazy(t *testing.T) {
	for _, source := range []string{"no matching class", "first", "second first", "prefix-first-suffix"} {
		var want, got bytes.Buffer
		triggers := []string{"first", "second"}
		appendOnTrigger(&want, source, triggers, "content")
		calls := 0
		appendOnTriggerLazy(&got, source, triggers, func() string {
			calls++
			return "content"
		})
		if got.String() != want.String() {
			t.Fatalf("trigger output differs for %q", source)
		}
		wantCalls := 0
		if strings.Contains(source, "first") || strings.Contains(source, "second") {
			wantCalls = 1
		}
		if calls != wantCalls {
			t.Fatalf("content built %d times, want %d", calls, wantCalls)
		}
	}
}

func BenchmarkEmbedFonts(b *testing.B) {
	for _, source := range []struct{ name, text string }{
		{"no_text", `<svg/>`},
		{"regular", `<text class="text">Hello</text>`},
		{"all_styles", `class="text" text-semibold text-bold text-italic text-mono text-mono-semibold text-mono-bold text-mono-italic`},
	} {
		b.Run(source.name, func(b *testing.B) {
			for _, legacy := range []bool{true, false} {
				name := "lazy"
				if legacy {
					name = "legacy"
				}
				b.Run(name, func(b *testing.B) {
					family, mono := d2fonts.SourceSansPro, d2fonts.SourceCodePro
					b.ReportAllocs()
					for range b.N {
						var out bytes.Buffer
						if legacy {
							legacyEmbedFonts(&out, "diagram", source.text, &family, &mono, "Hello D2 diagrams 0123 -> []")
						} else {
							EmbedFonts(&out, "diagram", source.text, &family, &mono, "Hello D2 diagrams 0123 -> []")
						}
					}
				})
			}
		})
	}
}
