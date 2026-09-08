package d2elklayout

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/log"
)

func TestConfigurableOptsSerializeELK012Keys(t *testing.T) {
	opts := ConfigurableOpts{
		Algorithm:       "layered",
		NodeSpacing:     123,
		Padding:         "[top=11,left=22,bottom=33,right=44]",
		EdgeNodeSpacing: 67,
		EdgeEdgeSpacing: 78,
		SelfLoopSpacing: 89,
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"elk.algorithm":                 "layered",
		"spacing.nodeNodeBetweenLayers": float64(123),
		"elk.padding":                   "[top=11,left=22,bottom=33,right=44]",
		"spacing.edgeNodeBetweenLayers": float64(67),
		"spacing.edgeEdgeBetweenLayers": float64(78),
		"elk.spacing.nodeSelfLoop":      float64(89),
	}
	if len(got) != len(want) {
		t.Fatalf("serialized options = %s", raw)
	}
	for key, wantValue := range want {
		if gotValue := got[key]; gotValue != wantValue {
			t.Errorf("%s = %#v, want %#v (serialized %s)", key, gotValue, wantValue, raw)
		}
	}
}

func TestNativeELKMigrationProfile(t *testing.T) {
	root := newRootLayoutOptions(&DefaultOpts)
	if root.ConsiderModelOrder != "NODES_AND_EDGES" || root.CycleBreakingStrategy != "GREEDY_MODEL_ORDER" || root.ForceNodeModelOrder {
		t.Fatalf("root migration profile = consider %q, cycle %q, force model order %t; want NODES_AND_EDGES/GREEDY_MODEL_ORDER/false",
			root.ConsiderModelOrder, root.CycleBreakingStrategy, root.ForceNodeModelOrder)
	}

	container := newContainerLayoutOptions(&DefaultOpts)
	if container.ConsiderModelOrder != "NONE" || container.CycleBreakingStrategy != "GREEDY" || container.ForceNodeModelOrder {
		t.Fatalf("container migration profile = consider %q, cycle %q, force model order %t; want NONE/GREEDY/false",
			container.ConsiderModelOrder, container.CycleBreakingStrategy, container.ForceNodeModelOrder)
	}
}

func TestConfigurableOptsAffectLayout(t *testing.T) {
	t.Run("node spacing", func(t *testing.T) {
		compact := layoutOptionFixture(t, "a -> b", func(opts *ConfigurableOpts) {
			opts.NodeSpacing = 20
		})
		spacious := layoutOptionFixture(t, "a -> b", func(opts *ConfigurableOpts) {
			opts.NodeSpacing = 160
		})
		compactGap := verticalGap(objectByID(t, compact, "a"), objectByID(t, compact, "b"))
		spaciousGap := verticalGap(objectByID(t, spacious, "a"), objectByID(t, spacious, "b"))
		if spaciousGap-compactGap < 100 {
			t.Fatalf("node gap changed from %g to %g, want a material increase", compactGap, spaciousGap)
		}
	})

	t.Run("padding", func(t *testing.T) {
		compact := layoutOptionFixture(t, "container: {\n  a\n}", func(opts *ConfigurableOpts) {
			opts.Padding = "[top=10,left=10,bottom=10,right=10]"
		})
		spacious := layoutOptionFixture(t, "container: {\n  a\n}", func(opts *ConfigurableOpts) {
			opts.Padding = "[top=100,left=100,bottom=100,right=100]"
		})
		compactContainer := objectByID(t, compact, "container")
		spaciousContainer := objectByID(t, spacious, "container")
		if spaciousContainer.Width-compactContainer.Width < 150 || spaciousContainer.Height-compactContainer.Height < 150 {
			t.Fatalf("container size changed from %gx%g to %gx%g, want padding to materially increase both dimensions",
				compactContainer.Width, compactContainer.Height, spaciousContainer.Width, spaciousContainer.Height)
		}
	})

	t.Run("edge-node spacing", func(t *testing.T) {
		compact := layoutOptionFixture(t, "a -> c\nb -> c", func(opts *ConfigurableOpts) {
			opts.EdgeNodeSpacing = 5
		})
		spacious := layoutOptionFixture(t, "a -> c\nb -> c", func(opts *ConfigurableOpts) {
			opts.EdgeNodeSpacing = 200
		})
		compactGap := verticalGap(objectByID(t, compact, "a"), objectByID(t, compact, "c"))
		spaciousGap := verticalGap(objectByID(t, spacious, "a"), objectByID(t, spacious, "c"))
		if spaciousGap-compactGap < 300 {
			t.Fatalf("edge-node layer gap changed from %g to %g, want a material increase", compactGap, spaciousGap)
		}
	})

	t.Run("edge-edge spacing between layers", func(t *testing.T) {
		const src = "a -> x\na -> y\nb -> x\nb -> y\nc -> x\nc -> y"
		compact := layoutOptionFixture(t, src, func(opts *ConfigurableOpts) {
			opts.EdgeEdgeSpacing = 5
		})
		spacious := layoutOptionFixture(t, src, func(opts *ConfigurableOpts) {
			opts.EdgeEdgeSpacing = 200
		})
		compactGap := verticalGap(objectByID(t, compact, "a"), objectByID(t, compact, "x"))
		spaciousGap := verticalGap(objectByID(t, spacious, "a"), objectByID(t, spacious, "x"))
		if spaciousGap-compactGap < 300 {
			t.Fatalf("edge-edge layer gap changed from %g to %g, want a material increase", compactGap, spaciousGap)
		}
	})

	t.Run("self-loop spacing", func(t *testing.T) {
		compact := layoutOptionFixture(t, "a -> a", func(opts *ConfigurableOpts) {
			opts.SelfLoopSpacing = 20
		})
		spacious := layoutOptionFixture(t, "a -> a", func(opts *ConfigurableOpts) {
			opts.SelfLoopSpacing = 140
		})
		compactExtent := routeExtent(t, compact.Edges[0])
		spaciousExtent := routeExtent(t, spacious.Edges[0])
		if spaciousExtent-compactExtent < 80 {
			t.Fatalf("self-loop route extent changed from %g to %g, want a material increase", compactExtent, spaciousExtent)
		}
	})
}

func layoutOptionFixture(t *testing.T, input string, configure func(*ConfigurableOpts)) *d2graph.Graph {
	t.Helper()
	g, _, err := d2compiler.Compile("", strings.NewReader(input), nil)
	if err != nil {
		t.Fatalf("compile fixture: %v", err)
	}
	for i, obj := range g.Objects {
		obj.Box = geo.NewBox(geo.NewPoint(float64(i*100), float64(i*50)), 60, 40)
	}
	opts := DefaultOpts
	configure(&opts)
	if err := Layout(log.WithTB(context.Background(), t), g, &opts); err != nil {
		t.Fatalf("layout fixture: %v", err)
	}
	return g
}

func objectByID(t *testing.T, g *d2graph.Graph, id string) *d2graph.Object {
	t.Helper()
	for _, obj := range g.Objects {
		if obj.AbsID() == id {
			return obj
		}
	}
	t.Fatalf("object %q not found", id)
	return nil
}

func verticalGap(a, b *d2graph.Object) float64 {
	if a.TopLeft.Y > b.TopLeft.Y {
		a, b = b, a
	}
	return b.TopLeft.Y - a.TopLeft.Y - a.Height
}

func routeExtent(t *testing.T, edge *d2graph.Edge) float64 {
	t.Helper()
	if len(edge.Route) < 2 {
		t.Fatalf("edge %q route has %d points, want at least 2", edge.AbsID(), len(edge.Route))
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, point := range edge.Route {
		minX = math.Min(minX, point.X)
		minY = math.Min(minY, point.Y)
		maxX = math.Max(maxX, point.X)
		maxY = math.Max(maxY, point.Y)
	}
	return math.Max(maxX-minX, maxY-minY)
}
