package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2exporter"
	"github.com/d2lang/d2/d2layouts/d2dagrelayout"
	"github.com/d2lang/d2/d2renderers/d2svg"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
	"github.com/d2lang/d2/lib/log"
	"github.com/d2lang/d2/lib/textmeasure"
)

// Remember to add if err != nil checks in production.
func main() {
	graph, config, _ := d2compiler.Compile("", strings.NewReader("x -> y"), nil)
	graph.ApplyTheme(d2themescatalog.NeutralDefault.ID)
	ruler, _ := textmeasure.NewRuler()
	_ = graph.SetDimensions(nil, ruler, nil, nil)
	ctx := log.WithDefault(context.Background())
	_ = d2dagrelayout.Layout(ctx, graph, nil)
	diagram, _ := d2exporter.Export(ctx, graph, nil, nil)
	diagram.Config = config
	out, _ := d2svg.Render(diagram, &d2svg.RenderOpts{
		ThemeID: &d2themescatalog.NeutralDefault.ID,
	})
	_ = os.WriteFile(filepath.Join("out.svg"), out, 0600)
}
