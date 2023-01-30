package main

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// Remember to add if err != nil checks in production.
func main() {
	ruler, _ := textmeasure.NewRuler()
	defaultLayout := func(ctx context.Context, g *d2graph.Graph) error {
		return d2dagrelayout.Layout(ctx, g, nil)
	}
	diagram, _, _ := d2lib.Compile(context.Background(), "x -> y", &d2lib.CompileOptions{
		Layout: defaultLayout,
		Ruler:  ruler,
	})
	out, _ := d2svg.Render(diagram, &d2svg.RenderOpts{
		Pad:         d2svg.DEFAULT_PADDING,
		ThemeID:     d2themescatalog.GrapeSoda.ID,
		DarkThemeID: -1,
	})
	_ = ioutil.WriteFile(filepath.Join("out.svg"), out, 0600)
}
