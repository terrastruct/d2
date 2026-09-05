package d2raster

import (
	"context"
	"fmt"
	"testing"

	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

var benchmarkPositionedGlyphs []positionedGlyph

func BenchmarkPositionGlyphsRepeatedLabels(b *testing.B) {
	fontData, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: d2fonts.SourceSansPro, Style: d2fonts.FONT_STYLE_REGULAR})
	if !ok {
		b.Fatal("Source Sans Pro is not bundled")
	}
	prepared, err := parsePreparedFont(fontData, 0)
	if err != nil {
		b.Fatal(err)
	}
	run := d2scene.TextRun{
		Text: "repeat repeated ASCII label",
		Font: d2scene.Font{Asset: "regular", Size: 16}, Fill: black,
	}
	options := testOptions()
	options.MaxFontFacesPerText = 8
	options.MaxTextCoverageChecks = 100_000
	options.MaxTextShapingRuns = 10_000
	for _, count := range []int{1, 100} {
		b.Run(fmt.Sprintf("%dLabels", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				preflight := &preflight{
					ctx: context.Background(), options: options,
					fonts: map[d2scene.AssetID]*preparedFont{"regular": prepared},
				}
				for range count {
					glyphs, _, err := preflight.positionGlyphs(run, fixed.I(16))
					if err != nil {
						b.Fatal(err)
					}
					benchmarkPositionedGlyphs = glyphs
				}
			}
		})
	}
}
