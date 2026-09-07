package d2talalayout_test

import (
	"context"
	"fmt"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2talalayout"
	"github.com/d2lang/d2/lib/geo"
)

func ExampleLayout() {
	ctx := context.Background()
	graph := d2graph.NewGraph()
	object := &d2graph.Object{
		Graph:      graph,
		Parent:     graph.Root,
		ID:         "a",
		IDVal:      "a",
		Box:        geo.NewBox(geo.NewPoint(0, 0), 100, 60),
		Children:   make(map[string]*d2graph.Object),
		Attributes: d2graph.Attributes{},
	}
	graph.Root.Children[object.ID] = object
	graph.Root.ChildrenArray = append(graph.Root.ChildrenArray, object)
	graph.Objects = append(graph.Objects, object)

	opts := d2talalayout.DefaultOptions()
	opts.Seeds = []int64{1}
	if err := d2talalayout.Layout(ctx, graph, &opts); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%.0f %.0f\n", object.Box.TopLeft.X, object.Box.TopLeft.Y)

	// Output: 0 0
}
