package d2isometricimg

import (
	"context"
	"image/color"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestOutputDensityReservesFollowingSourcePanel(t *testing.T) {
	box := d2scene.Box{Width: 800, Height: 600}
	doc := d2scene.NewDocument(box, d2scene.NewNode(d2scene.Rect{Box: box, Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 210, G: 60, B: 40, A: 255}}}))
	var lowPixels int64
	for _, density := range []float64{20, 1e6} {
		b := &meshBuilder{ctx: context.Background(), outputDensity: density}
		// A large legend and its enclosing root frame both need a texture.
		// Previously each requested the full 8M share, failing the second.
		for i, area := range []labelSurface{{width: 8, depth: 6}, {width: 9, depth: 7}} {
			if err := b.sourcePanel(doc, area, 2); err != nil {
				t.Fatalf("panel %d at density %g: %v", i, density, err)
			}
		}
		var pixels int64
		for _, panel := range b.panels {
			pixels += int64(panel.width) * int64(panel.height)
			if panel.width*panel.height > 4<<20 {
				t.Fatal("first source panel consumed the following panel's share")
			}
			tex := b.triangles[panel.first].Material.Texture
			if _, _, _, a := tex.At(panel.width/2, panel.height/2).RGBA(); a == 0 {
				t.Fatal("bounded source panel lost its content")
			}
		}
		if pixels > 8<<20 || len(b.panels) != 2 || doc.ViewBox != box {
			t.Fatal("panel reservation exceeded budget or changed source layout")
		}
		if lowPixels == 0 {
			lowPixels = pixels
		} else if pixels <= lowPixels {
			t.Fatal("source-panel sharpness did not grow with output density")
		}
	}
	// A standalone legend may use the full budget when there is no root frame.
	b := &meshBuilder{ctx: context.Background(), outputDensity: 1e6}
	if err := b.sourcePanel(doc, labelSurface{width: 8, depth: 6}, 1); err != nil {
		t.Fatal(err)
	}
	p := b.panels[0]
	if p.width*p.height <= 4<<20 || p.width*p.height > 8<<20 {
		t.Fatal("single native panel did not retain its full texture budget")
	}
}
