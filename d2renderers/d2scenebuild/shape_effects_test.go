package d2scenebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	imagecolor "image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

func TestBuildThreeDeeRectangleGeometryAndPaintOrder(t *testing.T) {
	t.Parallel()

	shape := effectTestShape("rect", d2target.ShapeRectangle, 10, 20, 100, 60)
	shape.ThreeDee = true
	shape.BorderRadius = 999 // 3D rectangles ignore border radius.
	document := buildEffectsDocument(t, shape)
	node := findSceneNode(t, document.Root, shape.ID)

	wantIDs := []string{"rect:3d-main", "rect:3d-sides", "rect:3d-border"}
	assertChildIDs(t, node, wantIDs)
	main, ok := node.Children[0].Primitive.(d2scene.Rect)
	if !ok {
		t.Fatalf("main primitive = %T, want Rect", node.Children[0].Primitive)
	}
	if main.Box != (d2scene.Box{X: 10, Y: 20, Width: 100, Height: 60}) || main.RadiusX != 0 || main.RadiusY != 0 || main.Stroke != nil {
		t.Fatalf("main rectangle = %+v, want square-corner fill-only geometry", main)
	}

	sides, ok := node.Children[1].Primitive.(d2scene.Path)
	if !ok {
		t.Fatalf("side primitive = %T, want Path", node.Children[1].Primitive)
	}
	wantSides := []d2scene.PathCommand{
		d2scene.MoveTo(10, 20),
		d2scene.LineTo(25, 5),
		d2scene.LineTo(125, 5),
		d2scene.LineTo(125, 65),
		d2scene.LineTo(110, 80),
		d2scene.LineTo(110, 20),
		d2scene.ClosePath(),
	}
	if !reflect.DeepEqual(sides.Commands, wantSides) || sides.Fill == nil || sides.Stroke != nil || reflect.DeepEqual(sides.Fill, main.Fill) {
		t.Fatalf("3d sides = %+v, want exact closed darkened fill-only polygon", sides)
	}

	border, ok := node.Children[2].Primitive.(d2scene.Path)
	if !ok {
		t.Fatalf("border primitive = %T, want Path", node.Children[2].Primitive)
	}
	wantBorder := []d2scene.PathCommand{
		d2scene.MoveTo(10, 20),
		d2scene.LineTo(25, 5),
		d2scene.LineTo(125, 5),
		d2scene.LineTo(125, 65),
		d2scene.LineTo(110, 80),
		d2scene.LineTo(10, 80),
		d2scene.LineTo(10, 20),
		d2scene.LineTo(110, 20),
		d2scene.LineTo(110, 80),
		d2scene.MoveTo(110, 20),
		d2scene.LineTo(125, 5),
	}
	if !reflect.DeepEqual(border.Commands, wantBorder) || border.Fill != nil || border.Stroke == nil {
		t.Fatalf("3d border = %+v, want exact unfilled stroked path", border)
	}
}

func TestBuildThreeDeeHexagonIntegerGeometry(t *testing.T) {
	t.Parallel()

	shape := effectTestShape("hex", d2target.ShapeHexagon, 10, 20, 101, 87)
	shape.ThreeDee = true
	document := buildEffectsDocument(t, shape)
	node := findSceneNode(t, document.Root, shape.ID)
	assertChildIDs(t, node, []string{"hex:3d-main", "hex:3d-sides", "hex:3d-border"})

	main := node.Children[0].Primitive.(d2scene.Path)
	wantMain := []d2scene.PathCommand{
		d2scene.MoveTo(35, 20),
		d2scene.LineTo(85, 20),
		d2scene.LineTo(111, 63),
		d2scene.LineTo(85, 107),
		d2scene.LineTo(35, 107),
		d2scene.LineTo(10, 63),
		d2scene.ClosePath(),
	}
	if !reflect.DeepEqual(main.Commands, wantMain) || main.Fill == nil || main.Stroke != nil {
		t.Fatalf("3d hex main = %+v, want exact integer-scaled polygon", main)
	}

	sides := node.Children[1].Primitive.(d2scene.Path)
	wantSides := []d2scene.PathCommand{
		d2scene.MoveTo(50, 13),
		d2scene.LineTo(100, 13),
		d2scene.LineTo(126, 56),
		d2scene.LineTo(100, 100),
		d2scene.LineTo(85, 107),
		d2scene.LineTo(111, 63),
		d2scene.LineTo(85, 20),
		d2scene.LineTo(35, 20),
		d2scene.ClosePath(),
	}
	if !reflect.DeepEqual(sides.Commands, wantSides) || sides.Fill == nil || sides.Stroke != nil {
		t.Fatalf("3d hex sides = %+v, want exact side polygon", sides)
	}

	border := node.Children[2].Primitive.(d2scene.Path)
	wantBorder := []d2scene.PathCommand{
		d2scene.MoveTo(35, 20),
		d2scene.LineTo(50, 13),
		d2scene.LineTo(100, 13),
		d2scene.LineTo(126, 56),
		d2scene.LineTo(100, 100),
		d2scene.LineTo(85, 107),
		d2scene.LineTo(35, 107),
		d2scene.LineTo(10, 63),
		d2scene.LineTo(35, 20),
		d2scene.LineTo(85, 20),
		d2scene.LineTo(111, 63),
		d2scene.LineTo(85, 107),
		d2scene.MoveTo(85, 20),
		d2scene.LineTo(100, 13),
		d2scene.MoveTo(111, 63),
		d2scene.LineTo(126, 56),
		d2scene.MoveTo(85, 107),
		d2scene.LineTo(100, 100),
	}
	if !reflect.DeepEqual(border.Commands, wantBorder) || border.Fill != nil || border.Stroke == nil {
		t.Fatalf("3d hex border = %+v, want exact border commands", border)
	}
}

func TestBuildMultipleTypedShapePaintsDuplicateBeforeMain(t *testing.T) {
	t.Parallel()

	shape := effectTestShape("diamond", d2target.ShapeDiamond, 10, 20, 80, 60)
	shape.Multiple = true
	document := buildEffectsDocument(t, shape)
	node := findSceneNode(t, document.Root, shape.ID)
	assertChildIDs(t, node, []string{"diamond:multiple", "diamond:main"})

	duplicate, ok := node.Children[0].Primitive.(d2scene.Path)
	if !ok {
		t.Fatalf("duplicate primitive = %T, want Path", node.Children[0].Primitive)
	}
	main := node.Children[1].Primitive.(d2scene.Path)
	if main.Commands[0] != d2scene.MoveTo(50, 80) {
		t.Fatalf("main first point = %+v, want original typed diamond geometry", main.Commands[0])
	}
	if duplicate.Commands[0] != d2scene.MoveTo(main.Commands[0].P1.X+10, main.Commands[0].P1.Y-10) {
		t.Fatalf("duplicate first point = %+v, want +10,-10 from %+v", duplicate.Commands[0], main.Commands[0])
	}
	if !reflect.DeepEqual(duplicate.Fill, main.Fill) || !reflect.DeepEqual(duplicate.Stroke, main.Stroke) {
		t.Fatal("multiple duplicate did not preserve the main fill and stroke roles")
	}
}

func TestBuildOutsideLabelUsesEffectExpandedBox(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		shapeType  string
		threeDee   bool
		multiple   bool
		wantOrigin d2scene.Point
	}{
		{
			name: "multiple rectangle", shapeType: d2target.ShapeRectangle, multiple: true,
			wantOrigin: d2scene.Point{X: 65, Y: 2},
		},
		{
			name: "3d rectangle", shapeType: d2target.ShapeRectangle, threeDee: true,
			wantOrigin: d2scene.Point{X: 67.5, Y: -3},
		},
		{
			name: "3d hexagon", shapeType: d2target.ShapeHexagon, threeDee: true,
			wantOrigin: d2scene.Point{X: 67.5, Y: 5},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			shape := effectTestShape("label", test.shapeType, 10, 20, 100, 60)
			shape.ThreeDee = test.threeDee
			shape.Multiple = test.multiple
			shape.Label = "label"
			shape.LabelPosition = "OUTSIDE_TOP_CENTER"
			shape.FontSize = 16
			shape.FontFamily = "DEFAULT"
			shape.LabelWidth = 40
			shape.LabelHeight = 19
			document := buildEffectsDocument(t, shape)
			node := findSceneNode(t, document.Root, shape.ID)
			text, ok := node.Children[len(node.Children)-1].Primitive.(d2scene.TextRun)
			if !ok {
				t.Fatalf("last child = %T, want effect-positioned TextRun", node.Children[len(node.Children)-1].Primitive)
			}
			if text.Origin != test.wantOrigin {
				t.Fatalf("label origin = %+v, want %+v from effect box", text.Origin, test.wantOrigin)
			}
		})
	}
}

func TestBuildDoubleBorderGeometryAndPaintOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		shapeType string
		multiple  bool
		wantIDs   []string
	}{
		{
			name: "rectangle", shapeType: d2target.ShapeRectangle,
			wantIDs: []string{"effect:double-border:outer", "effect:double-border:inner"},
		},
		{
			name: "oval", shapeType: d2target.ShapeOval,
			wantIDs: []string{"effect:double-border:outer", "effect:double-border:inner"},
		},
		{
			name: "multiple rectangle", shapeType: d2target.ShapeRectangle, multiple: true,
			wantIDs: []string{"effect:multiple:outer", "effect:multiple:inner", "effect:double-border:outer", "effect:double-border:inner"},
		},
		{
			name: "multiple oval", shapeType: d2target.ShapeOval, multiple: true,
			wantIDs: []string{"effect:multiple:outer", "effect:multiple:inner", "effect:double-border:outer", "effect:double-border:inner"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			shape := effectTestShape("effect", test.shapeType, 10, 20, 100, 60)
			shape.DoubleBorder = true
			shape.Multiple = test.multiple
			shape.BorderRadius = 999
			document := buildEffectsDocument(t, shape)
			node := findSceneNode(t, document.Root, shape.ID)
			assertChildIDs(t, node, test.wantIDs)

			mainStart := 0
			if test.multiple {
				mainStart = 2
				assertEffectBoxes(t, test.shapeType, node.Children[0], node.Children[1],
					d2scene.Box{X: 20, Y: 10, Width: 100, Height: 60},
					d2scene.Box{X: 25, Y: 15, Width: 90, Height: 50},
				)
				if !reflect.DeepEqual(effectFill(node.Children[0]), effectFill(node.Children[1])) {
					t.Fatal("multiple double-border inner must repeat the filled duplicate")
				}
			}
			assertEffectBoxes(t, test.shapeType, node.Children[mainStart], node.Children[mainStart+1],
				d2scene.Box{X: 10, Y: 20, Width: 100, Height: 60},
				d2scene.Box{X: 15, Y: 25, Width: 90, Height: 50},
			)
			outerFill := effectFill(node.Children[mainStart])
			innerFill := effectFill(node.Children[mainStart+1])
			if test.shapeType == d2target.ShapeOval {
				if !reflect.DeepEqual(innerFill, outerFill) {
					t.Fatal("double oval inner must repeat the outer fill")
				}
			} else if alphaOf(t, innerFill) != 0 {
				t.Fatalf("double rectangle inner alpha = %d, want transparent", alphaOf(t, innerFill))
			}
		})
	}
}

func TestBuildRootDoubleBorderExpansionAndOrder(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.Fill = "#ddeeff"
	diagram.Root.Stroke = "#112233"
	diagram.Root.StrokeWidth = 2
	diagram.Root.BorderRadius = 100
	diagram.Root.DoubleBorder = true
	diagram.Shapes = []d2target.Shape{effectTestShape("shape", d2target.ShapeRectangle, 0, 0, 100, 20)}
	diagram.Shapes[0].Stroke = "none"
	diagram.Shapes[0].StrokeWidth = 0
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if document.ViewBox != (d2scene.Box{X: -9, Y: -9, Width: 118, Height: 38}) {
		t.Fatalf("ViewBox = %+v, want double-root expansion", document.ViewBox)
	}
	assertChildIDs(t, document.Root, []string{"root:double-border:outer", "root:background", "shape"})
	outer := document.Root.Children[0].Primitive.(d2scene.Rect)
	inner := document.Root.Children[1].Primitive.(d2scene.Rect)
	if outer.Box != (d2scene.Box{X: -8, Y: -8, Width: 116, Height: 36}) || outer.RadiusX != 18 || outer.RadiusY != 18 {
		t.Fatalf("outer root border = %+v, want expanded filled background", outer)
	}
	if inner.Box != (d2scene.Box{X: -1, Y: -1, Width: 102, Height: 22}) || inner.RadiusX != 11 || inner.RadiusY != 11 {
		t.Fatalf("inner root border = %+v, want original background geometry", inner)
	}
	if !reflect.DeepEqual(outer.Stroke, inner.Stroke) || outer.Fill == nil || alphaOf(t, inner.Fill) != 0 {
		t.Fatal("root double border must paint filled outer first and transparent stroked inner second")
	}
}

func TestBuildRejectsInvalidShapeEffectCombinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		make func() *d2target.Diagram
		want string
	}{
		{name: "root 3d", make: func() *d2target.Diagram {
			d := d2target.NewDiagram()
			d.Root.ThreeDee = true
			return d
		}, want: "unsupported 3d effect"},
		{name: "root multiple", make: func() *d2target.Diagram {
			d := d2target.NewDiagram()
			d.Root.Multiple = true
			return d
		}, want: "unsupported multiple effect"},
		{name: "3d oval", make: effectDiagramMutator(func(s *d2target.Shape) {
			s.Type, s.ThreeDee = d2target.ShapeOval, true
		}), want: "3d for shape type oval"},
		{name: "double diamond", make: effectDiagramMutator(func(s *d2target.Shape) {
			s.Type, s.DoubleBorder = d2target.ShapeDiamond, true
		}), want: "double border for shape type diamond"},
		{name: "multiple text", make: effectDiagramMutator(func(s *d2target.Shape) {
			s.Type, s.Multiple = d2target.ShapeText, true
		}), want: "multiple for shape type text"},
		{name: "double border narrow", make: effectDiagramMutator(func(s *d2target.Shape) {
			s.DoubleBorder, s.Width = true, 9
		}), want: `field "width"`},
		{name: "double border short", make: effectDiagramMutator(func(s *d2target.Shape) {
			s.DoubleBorder, s.Height = true, 9
		}), want: `field "height"`},
		{name: "multiple coordinate overflow", make: effectDiagramMutator(func(s *d2target.Shape) {
			s.Multiple = true
			s.StrokeWidth = 0
			s.Pos.X = int(^uint(0)>>1) - s.Width
		}), want: `field "pos.x"`},
		{name: "shadow coordinate overflow", make: effectDiagramMutator(func(s *d2target.Shape) {
			s.Shadow = true
			s.StrokeWidth = 0
			s.Pos.X = int(^uint(0)>>1) - s.Width - d2target.SHADOW_SIZE_X + 1
		}), want: `field "pos.x"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(context.Background(), test.make(), Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildThreeDeeTakesPrecedenceOverOtherEffects(t *testing.T) {
	t.Parallel()

	for _, sketch := range []bool{false, true} {
		t.Run(fmt.Sprintf("sketch=%t", sketch), func(t *testing.T) {
			diagram := effectDiagramMutator(func(shape *d2target.Shape) {
				shape.ThreeDee = true
				shape.Multiple = true
				shape.DoubleBorder = true
				// This would be too small for an active double border; 3D still takes
				// precedence.
				shape.Width = 9
				shape.Height = 9
			})()
			document, err := Build(context.Background(), diagram, Options{
				Sketch: sketch,
				SketchBudget: SketchBudget{
					MaxOperationSets: 1_000,
					MaxOperations:    10_000,
					MaxPathCommands:  10_000,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			shape := findSceneNode(t, document.Root, "bad")
			for _, child := range shape.Children {
				if strings.Contains(child.ID, "multiple") || strings.Contains(child.ID, "double-border") {
					t.Fatalf("3D precedence retained secondary effect node %q", child.ID)
				}
			}
		})
	}
}

func TestBuildShapeEffectsDoNotMutateTarget(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.DoubleBorder = true
	rect := effectTestShape("3d", d2target.ShapeRectangle, 0, 20, 100, 60)
	rect.ThreeDee = true
	rect.BorderRadius = 999
	diamond := effectTestShape("multiple", d2target.ShapeDiamond, 160, 20, 80, 60)
	diamond.Multiple = true
	oval := effectTestShape("double", d2target.ShapeOval, 300, 20, 100, 60)
	oval.Multiple = true
	oval.DoubleBorder = true
	diagram.Shapes = []d2target.Shape{rect, diamond, oval}
	before, err := json.Marshal(diagram)
	if err != nil {
		t.Fatalf("marshal before Build: %v", err)
	}
	if _, err := Build(context.Background(), diagram, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	after, err := json.Marshal(diagram)
	if err != nil {
		t.Fatalf("marshal after Build: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("Build mutated effect target\nbefore: %s\n after: %s", before, after)
	}
}

func TestBuildShapeEffectsProduceRenderableScene(t *testing.T) {
	t.Parallel()

	diagram := d2target.NewDiagram()
	diagram.Root.DoubleBorder = true
	rect := effectTestShape("3d-rect", d2target.ShapeRectangle, 20, 40, 100, 60)
	rect.ThreeDee = true
	hex := effectTestShape("3d-hex", d2target.ShapeHexagon, 170, 40, 100, 60)
	hex.ThreeDee = true
	diamond := effectTestShape("multiple", d2target.ShapeDiamond, 320, 40, 100, 60)
	diamond.Multiple = true
	oval := effectTestShape("double", d2target.ShapeOval, 470, 40, 100, 60)
	oval.Multiple = true
	oval.DoubleBorder = true
	diagram.Shapes = []d2target.Shape{rect, hex, diamond, oval}
	pad := int64(10)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	frame, err := d2raster.Render(context.Background(), document, d2raster.FrameOptions{
		Scale: 1, Background: imagecolor.White,
		MaxWidth: 4096, MaxHeight: 4096, MaxPixels: 16_000_000,
		MaxNodes: 1_000, MaxDepth: 32, MaxPathCommands: 10_000,
		MaxAnimationTracks: 1_000, MaxAnimationKeyframes: 10_000,
		MaxAssets: 100, MaxAssetBytes: 64 * 1024 * 1024,
		MaxDecodedAssetBytes: 64 * 1024 * 1024, MaxImportDepth: 32,
		MaxOffscreenBytes: 64 * 1024 * 1024, MaxEvenOddClipWork: 1_000_000_000,
	})
	if err != nil {
		t.Fatalf("d2raster.Render() error = %v", err)
	}
	if frame.Bounds().Dx() == 0 || frame.Bounds().Dy() == 0 {
		t.Fatalf("rendered empty frame: %v", frame.Bounds())
	}
}

func effectTestShape(id, shapeType string, x, y, width, height int) d2target.Shape {
	return d2target.Shape{
		ID: id, Type: shapeType,
		Pos: d2target.Point{X: x, Y: y}, Width: width, Height: height,
		Fill: "#6699cc", Stroke: "#112233", StrokeWidth: 2, Opacity: 1,
	}
}

func buildEffectsDocument(t *testing.T, shape d2target.Shape) *d2scene.Document {
	t.Helper()
	diagram := d2target.NewDiagram()
	diagram.Shapes = []d2target.Shape{shape}
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return document
}

func assertChildIDs(t *testing.T, node *d2scene.Node, want []string) {
	t.Helper()
	if len(node.Children) != len(want) {
		t.Fatalf("node %q children = %d, want %d", node.ID, len(node.Children), len(want))
	}
	for i, id := range want {
		if node.Children[i].ID != id {
			t.Errorf("node %q child %d ID = %q, want %q", node.ID, i, node.Children[i].ID, id)
		}
	}
}

func assertEffectBoxes(t *testing.T, shapeType string, outerNode, innerNode *d2scene.Node, wantOuter, wantInner d2scene.Box) {
	t.Helper()
	if shapeType == d2target.ShapeOval {
		outer := outerNode.Primitive.(d2scene.Ellipse)
		inner := innerNode.Primitive.(d2scene.Ellipse)
		gotOuter := d2scene.Box{X: outer.Center.X - outer.RadiusX, Y: outer.Center.Y - outer.RadiusY, Width: 2 * outer.RadiusX, Height: 2 * outer.RadiusY}
		gotInner := d2scene.Box{X: inner.Center.X - inner.RadiusX, Y: inner.Center.Y - inner.RadiusY, Width: 2 * inner.RadiusX, Height: 2 * inner.RadiusY}
		if gotOuter != wantOuter || gotInner != wantInner {
			t.Fatalf("ellipse boxes = %+v/%+v, want %+v/%+v", gotOuter, gotInner, wantOuter, wantInner)
		}
		return
	}
	outer := outerNode.Primitive.(d2scene.Rect)
	inner := innerNode.Primitive.(d2scene.Rect)
	if outer.Box != wantOuter || inner.Box != wantInner {
		t.Fatalf("rectangle boxes = %+v/%+v, want %+v/%+v", outer.Box, inner.Box, wantOuter, wantInner)
	}
	if outer.RadiusX != 30 || outer.RadiusY != 30 || inner.RadiusX != 25 || inner.RadiusY != 25 {
		t.Fatalf("per-box clamped radii = (%v,%v)/(%v,%v), want 30/25", outer.RadiusX, outer.RadiusY, inner.RadiusX, inner.RadiusY)
	}
}

func effectFill(node *d2scene.Node) d2scene.Paint {
	switch primitive := node.Primitive.(type) {
	case d2scene.Rect:
		return primitive.Fill
	case d2scene.Ellipse:
		return primitive.Fill
	case d2scene.Path:
		return primitive.Fill
	default:
		return nil
	}
}

func alphaOf(t *testing.T, paint d2scene.Paint) uint8 {
	t.Helper()
	solid, ok := paint.(d2scene.SolidPaint)
	if !ok {
		t.Fatalf("paint = %T, want SolidPaint", paint)
	}
	return solid.Color.A
}

func effectDiagramMutator(mutate func(*d2target.Shape)) func() *d2target.Diagram {
	return func() *d2target.Diagram {
		diagram := d2target.NewDiagram()
		shape := effectTestShape("bad", d2target.ShapeRectangle, 0, 0, 100, 60)
		mutate(&shape)
		diagram.Shapes = []d2target.Shape{shape}
		return diagram
	}
}
