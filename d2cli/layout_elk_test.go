//go:build !noelk

package d2cli

import (
	"context"
	"testing"

	"github.com/d2lang/util-go/go2"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
)

func TestELKLayoutOptions(t *testing.T) {
	wantOpts := d2elklayout.ConfigurableOpts{
		Algorithm:       "layered",
		NodeSpacing:     123,
		Padding:         "[top=11,left=22,bottom=33,right=44]",
		EdgeNodeSpacing: 67,
		EdgeEdgeSpacing: go2.Pointer(78),
		SelfLoopSpacing: 89,
	}
	want := layoutGraphJSON(t, func(ctx context.Context, g *d2graph.Graph) error {
		return d2elklayout.Layout(ctx, g, &wantOpts)
	})
	defaults := layoutGraphJSON(t, d2elklayout.DefaultLayout)
	if defaults == want {
		t.Fatal("test graph does not exercise configured ELK spacing")
	}
	state := layoutOptionsState(t,
		"--elk-algorithm=layered", "--elk-nodeNodeBetweenLayers=123",
		"--elk-padding=[top=11,left=22,bottom=33,right=44]",
		"--elk-edgeNodeBetweenLayers=67", "--elk-edgeEdgeBetweenLayers=78",
		"--elk-nodeSelfLoop=89")
	resolve := LayoutResolver(t.Context(), state)
	if err := state.Opts.Flags.Set("elk-nodeNodeBetweenLayers", "234"); err != nil {
		t.Fatal(err)
	}
	layout, err := resolve("elk")
	if err != nil {
		t.Fatal(err)
	}
	if got := layoutGraphJSON(t, layout); got != want {
		t.Fatal("ELK command-line options differ from native options")
	}
	cached, err := resolve("ELK")
	if err != nil {
		t.Fatal(err)
	}
	if got := layoutGraphJSON(t, cached); got != want {
		t.Fatal("changing CLI flags changed an already-resolved ELK layout")
	}
	other, err := LayoutResolver(t.Context(), state)("elk")
	if err != nil {
		t.Fatal(err)
	}
	wantOpts.NodeSpacing = 234
	otherWant := layoutGraphJSON(t, func(ctx context.Context, g *d2graph.Graph) error {
		return d2elklayout.Layout(ctx, g, &wantOpts)
	})
	if got := layoutGraphJSON(t, other); got != otherWant {
		t.Fatal("new ELK resolver did not capture changed options")
	}
	if got := layoutGraphJSON(t, layout); got != want {
		t.Fatal("creating another ELK resolver changed the first")
	}
	defaultLayout, err := LayoutResolver(t.Context(), nil)("elk")
	if err != nil {
		t.Fatal(err)
	}
	if got := layoutGraphJSON(t, defaultLayout); got != defaults {
		t.Fatal("nil CLI state did not select ELK defaults")
	}
}
