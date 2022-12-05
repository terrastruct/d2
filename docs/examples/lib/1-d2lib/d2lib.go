package main

import (
	"context"
	"io/ioutil"
	"path/filepath"

	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2renderers/textmeasure"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
)

// Remember to add if err != nil checks in production.
func main() {
	ruler, _ := textmeasure.NewRuler()
	diagram, _, _ := d2lib.Compile(context.Background(), "x -> y", &d2lib.CompileOptions{
		Layout:  d2dagrelayout.Layout,
		Ruler:   ruler,
		ThemeID: d2themescatalog.GrapeSoda.ID,
	})
	out, _ := d2svg.Render(diagram)
	_ = ioutil.WriteFile(filepath.Join("out.svg"), out, 0600)
}
