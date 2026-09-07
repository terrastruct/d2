package d2isometricimg

import (
	"context"
	"math"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

func TestNativePageFoldIsPhysicalAndKeepsSourceFootprint(t *testing.T) {
	for _, pattern := range []string{"", "dots"} {
		n := fidelityNode(d2target.ShapePage)
		n.Metadata.Original.FillPattern = pattern
		b := &meshBuilder{ctx: context.Background(), scale: .01}
		b.canonicalNode(n, "")
		if b.err != nil {
			t.Fatal(b.err)
		}
		top := n.Position.Y - n.Size.Y/2 + math.Max(.10, n.Size.Y*1.15)
		sloped := 0
		for _, tri := range b.triangles {
			if tri.Material.Unlit || !tri.CastShadow {
				continue
			}
			above := false
			for _, v := range tri.V {
				above = above || v.Position.Y > top+.01
			}
			if !above {
				continue
			}
			for _, v := range tri.V {
				if math.Abs(v.Position.X-n.Position.X) > n.Size.X/2+1e-9 || math.Abs(v.Position.Z-n.Position.Z) > n.Size.Z/2+1e-9 {
					t.Fatal("page fold changes source footprint")
				}
			}
			if normal := tri.V[0].Normal; normal.Y > .1 && normal.Y < .98 {
				sloped++
			}
		}
		if sloped == 0 {
			t.Fatal("page fold is still a flat printed glyph")
		}
	}
}

func TestNativePhysicalPageReplacesFlatGlyphAndRetainsSourcePerimeter(t *testing.T) {
	n := fidelityNode(d2target.ShapePage)
	n.StrokeDash = 3
	n.Metadata.Original.FillPattern = "lines"
	d := d2target.NewDiagram()
	d.Shapes = []d2target.Shape{nativeFaceSource(n, n.Fill)}
	doc, err := d2scenebuild.Build(context.Background(), d, d2scenebuild.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var paths []*d2scene.Node
	var visit func(*d2scene.Node)
	visit = func(n *d2scene.Node) {
		if p, ok := n.Primitive.(d2scene.Path); ok && p.Stroke != nil {
			paths = append(paths, n)
		}
		for _, child := range n.Children {
			visit(child)
		}
	}
	visit(doc.Root)
	if len(paths) != 2 {
		t.Fatalf("expected real Page outer and inner paths, got %d", len(paths))
	}
	outer := paths[0].Primitive.(d2scene.Path).Stroke
	if outer == nil || paths[1].Primitive.(d2scene.Path).Stroke == nil {
		t.Fatal("fixture lacks source Page strokes")
	}
	nativeRemovePageGlyph(doc.Root)
	if paths[0].Primitive.(d2scene.Path).Stroke != outer || paths[1].Primitive.(d2scene.Path).Stroke != nil {
		t.Fatal("physical Page retained the old flat fold or removed the authored perimeter")
	}
}

func TestClassicPalettePreservesAuthoredAndThemeOverrideInk(t *testing.T) {
	n := fidelityNode(d2target.ShapeRectangle)
	n.Stroke, n.Metadata.Original.Stroke = "#1560ff", "B1"
	if got := nativeClassicNode(n).Stroke; got != "#263c4e" {
		t.Fatalf("default classic ink = %s", got)
	}
	n.StrokeExplicit = true
	if got := nativeClassicNode(n).Stroke; got != n.Stroke {
		t.Fatal("classic palette replaced theme override")
	}
	n.StrokeExplicit = false
	n.Metadata.Original.Stroke = n.Stroke
	if got := nativeClassicNode(n).Stroke; got != n.Stroke {
		t.Fatal("classic palette replaced authored stroke")
	}
}

func TestClassicPalettePreservesStructuredDocumentBackgrounds(t *testing.T) {
	for _, kind := range []string{d2target.ShapeClass, d2target.ShapeSQLTable} {
		n := fidelityNode(kind)
		n.Stroke, n.Metadata.Original.Stroke = "#1269bc", "B1"
		if got := nativeClassicNode(n).Stroke; got != n.Stroke {
			t.Fatal("structured document header color changed")
		}
	}
}
