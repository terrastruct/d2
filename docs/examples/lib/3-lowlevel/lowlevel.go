package main

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"

	"oss.terrastruct.com/d2/d2compiler"
	"oss.terrastruct.com/d2/d2exporter"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
)

// Remember to add if err != nil checks in production.
func main() {
	graph, _ := d2compiler.Compile("", strings.NewReader("x -> y"), nil)
	ruler, _ := textmeasure.NewRuler()
	_ = graph.SetDimensions(nil, ruler)
	_ = d2dagrelayout.Layout(context.Background(), graph)
	diagram, _ := d2exporter.Export(context.Background(), graph, d2themescatalog.NeutralDefault.ID)
	out, _ := d2svg.Render(diagram)
	_ = ioutil.WriteFile(filepath.Join("out.svg"), out, 0600)
}
