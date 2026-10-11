package d2isometricimg

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestSVGSurfaceResourcesAreSharedAcrossPaintPasses(t *testing.T) {
	ctx := nativeVectorContext(context.Background())
	p, _ := newTextPainter(ctx, 1)
	texture, _, err := p.texture("shared face", normalPrintStyle())
	if err != nil {
		t.Fatal(err)
	}
	surface := nativeVectorForTexture(ctx, texture)
	writer := &nativeSVGWriter{ctx: ctx}
	first := writer.surfaceDefinition(surface)
	size := writer.buf.Len()
	for i := 0; i < 1000; i++ {
		if got := writer.surfaceDefinition(surface); got != first || writer.buf.Len() != size {
			t.Fatal("mesh or shadow pass duplicated retained vector source")
		}
	}
	if writer.err != nil {
		t.Fatal(writer.err)
	}
	other := &nativeSVGWriter{ctx: ctx}
	if other.surfaceDefinition(surface) != first || other.buf.Len() != size {
		t.Fatal("surface definitions leaked between exports")
	}
}

func TestNativeSVGLargeRichTextAndPaperStayWithinBudget(t *testing.T) {
	for _, fixture := range []string{"stable/giant_markdown_test/dagre/board.exp.json", "patterns/paper/dagre/board.exp.json"} {
		t.Run(fixture, func(t *testing.T) {
			diagram := sourcePanelFixture(t, fixture)
			svg, err := Render(context.Background(), diagram, &Options{Format: SVG, Width: 1200, Height: 1200, FitContent: true, Render: d2isometric.RenderOpts{}})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %d vector SVG bytes", fixture, len(svg))
			if strings.Contains(string(svg), "data:image") {
				t.Fatal("large vector source was replaced by raster pixels")
			}
			if strings.Contains(fixture, "giant_markdown") {
				uses := strings.Count(string(svg), `href="#surface-`)
				definitions := strings.Count(string(svg), `id="surface-`)
				if uses < definitions*4 {
					t.Fatalf("large rich text did not reuse glyph definitions: uses=%d definitions=%d", uses, definitions)
				}
			}
		})
	}
}
