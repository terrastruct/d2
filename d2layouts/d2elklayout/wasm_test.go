//go:build js && wasm

package d2elklayout

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/lib/geo"
)

func TestConvertGraphPreservesCustomELKProfile(t *testing.T) {
	g, _, err := d2compiler.Compile("", strings.NewReader("container: {\n  a -> b\n}"), nil)
	if err != nil {
		t.Fatalf("compile fixture: %v", err)
	}
	for i, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(float64(i*100), float64(i*50)), 60, 40)
	}
	opts := ConfigurableOpts{
		Algorithm:       "layered",
		NodeSpacing:     123,
		Padding:         "[top=11,left=22,bottom=33,right=44]",
		EdgeNodeSpacing: 67,
		EdgeEdgeSpacing: 78,
		SelfLoopSpacing: 89,
	}

	elkGraph, err := ConvertGraph(context.Background(), g, &opts)
	if err != nil {
		t.Fatalf("convert graph: %v", err)
	}
	rootOpts := elkGraph.LayoutOptions
	if rootOpts.ConsiderModelOrder != "NODES_AND_EDGES" || rootOpts.CycleBreakingStrategy != "GREEDY_MODEL_ORDER" || rootOpts.ForceNodeModelOrder {
		t.Fatalf("root migration profile = consider %q, cycle %q, force model order %t; want NODES_AND_EDGES/GREEDY_MODEL_ORDER/false",
			rootOpts.ConsiderModelOrder, rootOpts.CycleBreakingStrategy, rootOpts.ForceNodeModelOrder)
	}
	root := rootOpts.ConfigurableOpts
	if root.Algorithm != opts.Algorithm || root.NodeSpacing != opts.NodeSpacing ||
		root.EdgeNodeSpacing != opts.EdgeNodeSpacing || root.EdgeEdgeSpacing != opts.EdgeEdgeSpacing ||
		root.SelfLoopSpacing != opts.SelfLoopSpacing {
		t.Fatalf("root profile = %#v, want algorithm/spacing values from %#v", root, opts)
	}
	if len(elkGraph.Children) != 1 || elkGraph.Children[0].ID != "container" {
		t.Fatalf("root children = %#v, want container", elkGraph.Children)
	}
	containerOpts := elkGraph.Children[0].LayoutOptions
	if containerOpts == nil {
		t.Fatal("container layout options are nil")
	}
	if containerOpts.ConsiderModelOrder != "NONE" || containerOpts.CycleBreakingStrategy != "GREEDY" || containerOpts.ForceNodeModelOrder {
		t.Fatalf("container migration profile = consider %q, cycle %q, force model order %t; want NONE/GREEDY/false",
			containerOpts.ConsiderModelOrder, containerOpts.CycleBreakingStrategy, containerOpts.ForceNodeModelOrder)
	}
	if containerOpts.ConfigurableOpts != (ConfigurableOpts{
		NodeSpacing:     opts.NodeSpacing,
		Padding:         opts.Padding,
		EdgeNodeSpacing: opts.EdgeNodeSpacing,
		EdgeEdgeSpacing: opts.EdgeEdgeSpacing,
		SelfLoopSpacing: opts.SelfLoopSpacing,
	}) {
		t.Fatalf("container profile = %#v, want custom spacing and padding", containerOpts.ConfigurableOpts)
	}
}
