package svg_fuzzing

import (
	"context"
	"testing"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/textmeasure"
	"oss.terrastruct.com/util-go/go2"
)

func FuzzSVG(f *testing.F) {
	f.Fuzz(func(t *testing.T,
		diagramSpec string,
		renderOptPad int64,
		renderOptSketch bool,
		renderOptCenter bool,
		renderOptThemeID int64,
		renderOptDarkThemeID int64,
		// TODO: Theme*, Font
		renderOptScale float64,
		renderOptMasterID string) {
		ruler, err := textmeasure.NewRuler()
		if err != nil {
			return
		}
		layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
			return d2dagrelayout.DefaultLayout, nil
		}
		renderOpts := &d2svg.RenderOpts{
			Pad:      go2.Pointer(renderOptPad),
			Sketch:   go2.Pointer(renderOptSketch),
			Center:   go2.Pointer(renderOptCenter),
			ThemeID:  &d2themescatalog.GrapeSoda.ID,
			Scale:    go2.Pointer(renderOptScale),
			MasterID: renderOptMasterID,
		}
		compileOpts := &d2lib.CompileOptions{
			LayoutResolver: layoutResolver,
			Ruler:          ruler,
		}
		diagram, _, err := d2lib.Compile(context.Background(), diagramSpec, compileOpts, renderOpts)

		if err != nil {
			return
		}
		_, err = d2svg.Render(diagram, renderOpts)
		if err != nil {
			return
		}
	})
}
