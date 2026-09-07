package d2isometricimg

import (
	"context"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestContainerBackgroundPreservesSourceAndThemePaint(t *testing.T) {
	for _, tc := range []struct {
		name, kind, fill, override, want string
		theme                            int64
	}{
		{name: "cylinder default theme", kind: d2target.ShapeCylinder, fill: "AA4", want: "#edf0fd"},
		{name: "rectangle default theme", kind: d2target.ShapeRectangle, fill: "AA4", want: "#edf0fd"},
		{name: "dark theme", kind: d2target.ShapeCylinder, fill: "AA4", theme: 201, want: "#4918b1"},
		{name: "theme override", kind: d2target.ShapeCylinder, fill: "AA4", override: "#f3e5c7", want: "#f3e5c7"},
		{name: "literal fill", kind: d2target.ShapeCylinder, fill: "#102030", want: "#102030"},
		{name: "transparent fill", kind: d2target.ShapeCylinder, fill: "transparent", want: "transparent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const parentID = "container title is hidden"
			d := sourcePanelFixture(t, "regression/cylinder_grid_label/elk/board.exp.json")
			d.Config.ThemeID = &tc.theme
			if tc.override != "" {
				d.Config.ThemeOverrides = &d2target.ThemeOverrides{AA4: &tc.override}
			}
			for i := range d.Shapes {
				if d.Shapes[i].ID == parentID {
					d.Shapes[i].Type, d.Shapes[i].Fill = tc.kind, tc.fill
				}
			}
			scene, err := d2isometric.BuildScene(d, &d2isometric.RenderOpts{})
			if err != nil {
				t.Fatal(err)
			}
			ctx := nativeVectorContext(context.Background())
			native, err := newNativeSceneWithOptions(ctx, scene, 600, 600, nil, nil, nativeSceneOptions{deferRaster: true, vector: true})
			if err != nil {
				t.Fatal(err)
			}
			// Locate the actual composed parent surface by its source board
			// elevation and footprint, independently of tint-selection helpers.
			var cap *Material
			for _, board := range scene.Boards {
				if board.SourceID != parentID {
					continue
				}
				y := hierarchySurfaceY(board)
				for _, tri := range native.triangles {
					if tri.Material == nil || tri.Material.Texture == nil {
						continue
					}
					a, b, c := tri.V[0].Position, tri.V[1].Position, tri.V[2].Position
					if math.Abs(a.Y-y) > 1e-8 || math.Abs(b.Y-y) > 1e-8 || math.Abs(c.Y-y) > 1e-8 {
						continue
					}
					area := math.Abs((b.X-a.X)*(c.Z-a.Z)-(b.Z-a.Z)*(c.X-a.X)) / 2
					if area > board.Size.X*board.Size.Z/4 {
						cap = tri.Material
						break
					}
				}
			}
			if cap == nil {
				t.Fatal("source container surface missing")
			}
			bounds := cap.Texture.Bounds()
			got := color.NRGBAModel.Convert(cap.Texture.At(bounds.Min.X+bounds.Dx()/2, bounds.Min.Y+bounds.Dy()/2)).(color.NRGBA)
			want := nativePaint(tc.want, "transparent")
			if got != want {
				t.Fatalf("composed container fill is %v, want resolved source %v", got, want)
			}
			fragment, err := nativeSurfaceSVG(ctx, cap.Vector, "container")
			if err != nil {
				t.Fatal(err)
			}
			if want.A > 0 && !strings.Contains(fragment, `fill="`+nativeSVGColor(want)+`"`) {
				t.Fatal("SVG surface lost the resolved container paint")
			}
		})
	}
}
