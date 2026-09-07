package d2isometricimg

import (
	"context"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
)

func TestSVGVisibilityLargeSources(t *testing.T) {
	if testing.Short() {
		t.Skip("large native SVG visibility regression")
	}
	for _, name := range []string{"stable/nesting_power", "stable/all_shapes_link", "stable/large_arch", "stable/us_map", "stable/chaos2", "stable/arrowhead_scaling", "regression/grid_oom"} {
		t.Run(name, func(t *testing.T) {
			diagram := sourcePanelFixture(t, name+"/dagre/board.exp.json")
			_, err := Render(context.Background(), diagram, &Options{Format: SVG, Width: 1200, Height: 1200, FitContent: true, Render: d2isometric.RenderOpts{}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
