package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// Remember to add if err != nil checks in production.
func main() {
	graph, config, _ := d2compiler.Compile("", strings.NewReader("x -> y"), nil)
	graph.ApplyTheme(d2themescatalog.NeutralDefault.ID)
	ruler, _ := textmeasure.NewRuler()
	_ = graph.SetDimensions(nil, ruler, nil)
	ctx := log.WithDefault(context.Background())
	_ = d2dagrelayout.Layout(ctx, graph, nil)
	diagram, _ := d2exporter.Export(ctx, graph, nil)
	diagram.Config = config
	out, _ := d2svg.Render(diagram, &d2svg.RenderOpts{
		ThemeID: &d2themescatalog.NeutralDefault.ID,
	})
	_ = os.WriteFile(filepath.Join("out.svg"), out, 0600)
}
