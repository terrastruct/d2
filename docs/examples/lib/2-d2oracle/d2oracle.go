package main

import (
	"context"
	"fmt"

	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// Remember to add if err != nil checks in production.
func main() {
	// From one.go
	ruler, _ := textmeasure.NewRuler()
	_, graph, _ := d2lib.Compile(context.Background(), "x -> y", &d2lib.CompileOptions{
		Layout:  d2dagrelayout.DefaultLayout,
		Ruler:   ruler,
		ThemeID: d2themescatalog.GrapeSoda.ID,
	})

	// Create a shape with the ID, "meow"
	graph, _, _ = d2oracle.Create(graph, "meow")
	// Style the shape green
	color := "green"
	graph, _ = d2oracle.Set(graph, "meow.style.fill", nil, &color)
	// Create a shape with the ID, "cat"
	graph, _, _ = d2oracle.Create(graph, "cat")
	// Move the shape "meow" inside the container "cat"
	graph, _ = d2oracle.Move(graph, "meow", "cat.meow")
	// Prints formatted D2 script
	fmt.Print(d2format.Format(graph.AST))
}
