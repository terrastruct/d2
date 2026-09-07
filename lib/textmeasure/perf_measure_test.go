package textmeasure

import (
	"github.com/d2lang/d2/d2renderers/d2fonts"
	"testing"
)

func BenchmarkRulerLabelMetrics(b *testing.B) {
	for _, name := range []string{"legacy", "current"} {
		b.Run(name, func(b *testing.B) {
			r, err := NewRuler()
			if err != nil {
				b.Fatal(err)
			}
			spec := d2fonts.SourceSansPro.Font(16, d2fonts.FONT_STYLE_REGULAR)
			r.addFontSize(spec)
			text := "Service API -> event queue -> worker: processing request"
			b.ReportAllocs()
			for b.Loop() {
				if name == "legacy" {
					r.clear()
					r.buf = append(r.buf, text...)
					legacyDrawBuf(r, spec)
				} else {
					r.MeasurePrecise(spec, text)
				}
			}
		})
	}
}
func BenchmarkMarkdownTableMetrics(b *testing.B) {
	md := "| Header | Second | Third |\n|---|---|---|\n| plain | **bold** | `code` |\n| next row | *italic* | a [link](https://example.com) |"
	r, err := NewRuler()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, err := LayoutMarkdown(md, r, nil, nil, 16)
		if err != nil {
			b.Fatal(err)
		}
	}
}
