package d2isometricimg

import (
	"context"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

func TestSequenceBackgroundPreservesSourceAndThemePaint(t *testing.T) {
	for _, tc := range []struct{ name, fill, override, want string }{
		{"default paper", "N7", "", "#ffffff"},
		{"authored dark", "#102030", "", "#102030"},
		{"theme paper", "N7", "#f3e5c7", "#f3e5c7"},
		{"transparent paper", "transparent", "", "transparent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := sourcePanelFixture(t, "stable/sequence_diagram_real/elk/board.exp.json")
			for i := range d.Shapes {
				if d.Shapes[i].Type == d2target.ShapeSequenceDiagram {
					d.Shapes[i].Fill = tc.fill
				}
			}
			if tc.override != "" {
				d.Config.ThemeOverrides = &d2target.ThemeOverrides{N7: &tc.override}
			}
			scene, err := d2isometric.BuildScene(d, nil)
			if err != nil {
				t.Fatal(err)
			}
			native, err := newNativeSceneWithOptions(context.Background(), scene, 600, 600, nil, nil, nativeSceneOptions{deferRaster: true})
			if err != nil {
				t.Fatal(err)
			}
			// Inspect the emitted background surface, after scene composition
			// and tint selection, rather than testing a color-selection helper.
			var cap image.Image
			for _, board := range scene.Boards {
				if board.SourceID != "How this is rendered" {
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
						cap = tri.Material.Texture
						break
					}
				}
			}
			if cap == nil {
				if nativePaint(tc.want, "transparent").A == 0 {
					return // A transparent sequence needs no filled surface.
				}
				t.Fatal("sequence background surface missing")
			}
			counts := map[color.NRGBA]int{}
			var dominant color.NRGBA
			for y := cap.Bounds().Min.Y; y < cap.Bounds().Max.Y; y += 7 {
				for x := cap.Bounds().Min.X; x < cap.Bounds().Max.X; x += 7 {
					paint := color.NRGBAModel.Convert(cap.At(x, y)).(color.NRGBA)
					counts[paint]++
					if counts[paint] > counts[dominant] {
						dominant = paint
					}
				}
			}
			if want := nativePaint(tc.want, "transparent"); dominant != want {
				t.Fatalf("sequence paper got %v, want source/theme fill %v", dominant, want)
			}
		})
	}
}
