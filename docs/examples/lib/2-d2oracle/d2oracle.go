package main

import (
	"context"
	"fmt"

	"oss.terrastruct.com/d2/d2format"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2dagrelayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2oracle"
	"oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

// Remember to add if err != nil checks in production.
func main() {
	// From one.go
	ruler, _ := textmeasure.NewRuler()
	layoutResolver := func(engine string) (d2graph.LayoutGraph, error) {
		return d2dagrelayout.DefaultLayout, nil
	}
	compileOpts := &d2lib.CompileOptions{
		LayoutResolver: layoutResolver,
		Ruler:          ruler,
	}
	ctx := log.WithDefault(context.Background())
	_, graph, _ := d2lib.Compile(ctx, "x -> y", compileOpts, nil)

	// Create a shape with the ID, "meow"
	graph, _, _ = d2oracle.Create(graph, nil, "meow")
	// Style the shape green
	color := "green"
	graph, _ = d2oracle.Set(graph, nil, "meow.style.fill", nil, &color)
	// Create a shape with the ID, "cat"
	graph, _, _ = d2oracle.Create(graph, nil, "cat")
	// Move the shape "meow" inside the container "cat"
	graph, _ = d2oracle.Move(graph, nil, "meow", "cat.meow", false)
	// Prints formatted D2 script
	fmt.Print(d2format.Format(graph.AST))
}
