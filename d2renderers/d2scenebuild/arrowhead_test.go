package d2scenebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
)

var allArrowheads = []d2target.Arrowhead{
	d2target.NoArrowhead,
	d2target.ArrowArrowhead,
	d2target.UnfilledTriangleArrowhead,
	d2target.TriangleArrowhead,
	d2target.LineArrowhead,
	d2target.DiamondArrowhead,
	d2target.FilledDiamondArrowhead,
	d2target.CircleArrowhead,
	d2target.FilledCircleArrowhead,
	d2target.CrossArrowhead,
	d2target.BoxArrowhead,
	d2target.FilledBoxArrowhead,
	d2target.CfOne,
	d2target.CfMany,
	d2target.CfOneRequired,
	d2target.CfManyRequired,
}

func TestBuildSupportsEveryArrowheadAtBothEndpoints(t *testing.T) {
	for _, arrowhead := range allArrowheads {
		t.Run(string(arrowhead), func(t *testing.T) {
			diagram := validDiagram()
			diagram.Connections[0].SrcArrow = arrowhead
			diagram.Connections[0].DstArrow = arrowhead
			before, err := json.Marshal(diagram)
			if err != nil {
				t.Fatalf("marshal target before Build: %v", err)
			}

			pad := int64(0)
			document, err := Build(context.Background(), diagram, Options{Pad: &pad})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			after, err := json.Marshal(diagram)
			if err != nil {
				t.Fatalf("marshal target after Build: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("Build mutated %q arrowhead target", arrowhead)
			}

			connection := findSceneNode(t, document.Root, "a-b")
			if connection.Children[0].ID != "a-b:path" {
				t.Errorf("connection path ID = %q, want %q", connection.Children[0].ID, "a-b:path")
			}
			if arrowhead == d2target.NoArrowhead {
				if len(connection.Children) != 1 {
					t.Fatalf("no-arrow connection children = %d, want route only", len(connection.Children))
				}
				return
			}
			if len(connection.Children) != 3 {
				t.Fatalf("connection children = %d, want route + src/dst arrowheads", len(connection.Children))
			}

			route := connection.Children[0].Primitive.(d2scene.Path)
			strokePaint := route.Stroke.Paint
			background := document.Root.Children[0].Primitive.(d2scene.Rect).Fill
			width, height := arrowhead.Dimensions(float64(diagram.Connections[0].StrokeWidth))
			for _, endpoint := range []struct {
				name     string
				index    int
				x        float64
				target   bool
				wantID   string
				wantRefX float64
			}{
				{name: "src", index: 1, x: 24, wantID: "a-b:src-arrowhead", wantRefX: sourceRefX(arrowhead, width, 2)},
				{name: "dst", index: 2, x: 76, target: true, wantID: "a-b:dst-arrowhead", wantRefX: targetRefX(arrowhead, width, 2)},
			} {
				node := connection.Children[endpoint.index]
				if node.ID != endpoint.wantID {
					t.Errorf("%s arrowhead ID = %q, want %q", endpoint.name, node.ID, endpoint.wantID)
				}
				wantTransform := d2scene.Translate(endpoint.x, 10).Mul(d2scene.Translate(-endpoint.wantRefX, -height/2))
				if node.Transform != wantTransform {
					t.Errorf("%s transform = %+v, want %+v", endpoint.name, node.Transform, wantTransform)
				}
				assertArrowheadPaintRoles(t, arrowhead, node, strokePaint, background)
				for _, child := range node.Children {
					if len(child.ID) <= len(node.ID) || child.ID[:len(node.ID)] != node.ID {
						t.Errorf("%s nested child ID = %q, want prefix %q", endpoint.name, child.ID, node.ID)
					}
				}
			}
		})
	}
}

func TestBuildLineArrowheadCommands(t *testing.T) {
	connection := buildArrowheadConnection(t, d2target.LineArrowhead)
	width, height := d2target.LineArrowhead.Dimensions(2)
	inset := 1.0
	wants := [][]d2scene.PathCommand{
		pathCommands(
			d2scene.Point{X: width - inset, Y: inset},
			d2scene.Point{X: inset, Y: height / 2},
			d2scene.Point{X: width - inset, Y: height - inset},
		),
		pathCommands(
			d2scene.Point{X: inset, Y: inset},
			d2scene.Point{X: width - inset, Y: height / 2},
			d2scene.Point{X: inset, Y: height - inset},
		),
	}
	for i, want := range wants {
		path := connection.Children[i+1].Primitive.(d2scene.Path)
		if !reflect.DeepEqual(path.Commands, want) {
			t.Errorf("endpoint %d line commands = %+v, want %+v", i, path.Commands, want)
		}
	}
}

func TestBuildCrossArrowheadCommands(t *testing.T) {
	connection := buildArrowheadConnection(t, d2target.CrossArrowhead)
	width, height := d2target.CrossArrowhead.Dimensions(2)
	inset := 2.0 / 8
	wantCross := closedPathCommands(
		d2scene.Point{Y: height/2 + inset},
		d2scene.Point{X: width/2 - inset, Y: height/2 + inset},
		d2scene.Point{X: width/2 - inset, Y: height},
		d2scene.Point{X: width/2 + inset, Y: height},
		d2scene.Point{X: width/2 + inset, Y: height/2 + inset},
		d2scene.Point{X: width, Y: height/2 + inset},
		d2scene.Point{X: width, Y: height/2 - inset},
		d2scene.Point{X: width/2 + inset, Y: height/2 - inset},
		d2scene.Point{X: width/2 + inset},
		d2scene.Point{X: width/2 - inset},
		d2scene.Point{X: width/2 - inset, Y: height/2 - inset},
		d2scene.Point{Y: height/2 - inset},
	)
	origin := d2scene.Point{X: width / 2, Y: height / 2}
	rotatedOrigin := d2scene.Rotate(math.Pi / 4).Point(origin)
	wantCrossTransform := d2scene.Translate(-rotatedOrigin.X+width/2, -rotatedOrigin.Y+height/2).Mul(d2scene.Rotate(math.Pi / 4))

	for i, stemEndX := range []float64{0, width} {
		node := connection.Children[i+1]
		if len(node.Children) != 2 {
			t.Fatalf("endpoint %d cross children = %d, want two", i, len(node.Children))
		}
		cross := node.Children[0]
		path := cross.Primitive.(d2scene.Path)
		if !reflect.DeepEqual(path.Commands, wantCross) {
			t.Errorf("endpoint %d cross commands = %+v, want %+v", i, path.Commands, wantCross)
		}
		if cross.Transform != wantCrossTransform {
			t.Errorf("endpoint %d cross transform = %+v, want %+v", i, cross.Transform, wantCrossTransform)
		}
		stem := node.Children[1].Primitive.(d2scene.Path)
		wantStem := pathCommands(
			d2scene.Point{X: width / 2, Y: height / 2},
			d2scene.Point{X: stemEndX, Y: height / 2},
		)
		if !reflect.DeepEqual(stem.Commands, wantStem) {
			t.Errorf("endpoint %d stem commands = %+v, want %+v", i, stem.Commands, wantStem)
		}
	}
}

func TestBuildCrowFootArrowheadCommands(t *testing.T) {
	for _, arrowhead := range []d2target.Arrowhead{
		d2target.CfOne, d2target.CfMany, d2target.CfOneRequired, d2target.CfManyRequired,
	} {
		t.Run(string(arrowhead), func(t *testing.T) {
			connection := buildArrowheadConnection(t, arrowhead)
			width, height := arrowhead.Dimensions(2)
			offset := 3.0 + 2*1.8
			wantSourceTransform := d2scene.Scale(-1, -1).Mul(d2scene.Translate(-width, -height))

			for i, wantTransform := range []d2scene.Matrix{wantSourceTransform, d2scene.Identity()} {
				node := connection.Children[i+1]
				if len(node.Children) != 2 {
					t.Fatalf("endpoint %d children = %d, want modifier + marks", i, len(node.Children))
				}
				modifier := node.Children[0]
				marks := node.Children[1]
				if modifier.Transform != wantTransform || marks.Transform != wantTransform {
					t.Errorf("endpoint %d internal transforms = %+v/%+v, want %+v", i, modifier.Transform, marks.Transform, wantTransform)
				}

				if arrowhead == d2target.CfOneRequired || arrowhead == d2target.CfManyRequired {
					modifierPath, ok := modifier.Primitive.(d2scene.Path)
					if !ok {
						t.Fatalf("required modifier = %T, want Path", modifier.Primitive)
					}
					want := pathCommands(d2scene.Point{X: offset}, d2scene.Point{X: offset, Y: height})
					if !reflect.DeepEqual(modifierPath.Commands, want) {
						t.Errorf("required modifier commands = %+v, want %+v", modifierPath.Commands, want)
					}
				} else {
					modifierEllipse, ok := modifier.Primitive.(d2scene.Ellipse)
					if !ok {
						t.Fatalf("optional modifier = %T, want Ellipse", modifier.Primitive)
					}
					if modifierEllipse.Center != (d2scene.Point{X: offset/2 + 2, Y: height / 2}) || modifierEllipse.RadiusX != offset/2 || modifierEllipse.RadiusY != offset/2 {
						t.Errorf("optional modifier geometry = %+v, want circle formula", modifierEllipse)
					}
				}

				marksPath := marks.Primitive.(d2scene.Path)
				var wantMarks []d2scene.PathCommand
				if arrowhead == d2target.CfMany || arrowhead == d2target.CfManyRequired {
					wantMarks = []d2scene.PathCommand{
						d2scene.MoveTo(width-3, height/2), d2scene.LineTo(width+offset, height/2),
						d2scene.MoveTo(offset+3, height/2), d2scene.LineTo(width+offset, 0),
						d2scene.MoveTo(offset+3, height/2), d2scene.LineTo(width+offset, height),
					}
				} else {
					wantMarks = []d2scene.PathCommand{
						d2scene.MoveTo(width-3, height/2), d2scene.LineTo(width+offset, height/2),
						d2scene.MoveTo(offset*2, 0), d2scene.LineTo(offset*2, height),
					}
				}
				if !reflect.DeepEqual(marksPath.Commands, wantMarks) {
					t.Errorf("marks commands = %+v, want %+v", marksPath.Commands, wantMarks)
				}
			}
		})
	}
}

func buildArrowheadConnection(t *testing.T, arrowhead d2target.Arrowhead) *d2scene.Node {
	t.Helper()
	diagram := validDiagram()
	diagram.Connections[0].SrcArrow = arrowhead
	diagram.Connections[0].DstArrow = arrowhead
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build(%q) error = %v", arrowhead, err)
	}
	return findSceneNode(t, document.Root, "a-b")
}

func findSceneNode(t *testing.T, root *d2scene.Node, id string) *d2scene.Node {
	t.Helper()
	if root == nil {
		t.Fatalf("scene root is nil while finding %q", id)
	}
	if root.ID == id {
		return root
	}
	for _, child := range root.Children {
		if node := findSceneNodeOrNil(child, id); node != nil {
			return node
		}
	}
	t.Fatalf("scene node %q not found", id)
	return nil
}

func findSceneNodeOrNil(root *d2scene.Node, id string) *d2scene.Node {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	for _, child := range root.Children {
		if node := findSceneNodeOrNil(child, id); node != nil {
			return node
		}
	}
	return nil
}

func sourceRefX(arrowhead d2target.Arrowhead, width, strokeWidth float64) float64 {
	if arrowhead == d2target.DiamondArrowhead {
		return width/8 + .6*strokeWidth
	}
	return 1.5 * strokeWidth
}

func targetRefX(arrowhead d2target.Arrowhead, width, strokeWidth float64) float64 {
	if arrowhead == d2target.DiamondArrowhead {
		return width - .6*strokeWidth
	}
	return width - 1.5*strokeWidth
}

func assertArrowheadPaintRoles(t *testing.T, arrowhead d2target.Arrowhead, node *d2scene.Node, strokePaint, background d2scene.Paint) {
	t.Helper()
	switch arrowhead {
	case d2target.ArrowArrowhead, d2target.TriangleArrowhead, d2target.FilledDiamondArrowhead, d2target.FilledBoxArrowhead:
		path, ok := node.Primitive.(d2scene.Path)
		if !ok {
			t.Fatalf("%q primitive = %T, want Path", arrowhead, node.Primitive)
		}
		assertPaintRole(t, arrowhead, path.Fill, path.Stroke, strokePaint, nil)
	case d2target.UnfilledTriangleArrowhead, d2target.DiamondArrowhead, d2target.BoxArrowhead:
		path, ok := node.Primitive.(d2scene.Path)
		if !ok {
			t.Fatalf("%q primitive = %T, want Path", arrowhead, node.Primitive)
		}
		assertPaintRole(t, arrowhead, path.Fill, path.Stroke, background, strokePaint)
	case d2target.LineArrowhead:
		path, ok := node.Primitive.(d2scene.Path)
		if !ok {
			t.Fatalf("%q primitive = %T, want Path", arrowhead, node.Primitive)
		}
		assertPaintRole(t, arrowhead, path.Fill, path.Stroke, nil, strokePaint)
	case d2target.FilledCircleArrowhead:
		ellipse, ok := node.Primitive.(d2scene.Ellipse)
		if !ok {
			t.Fatalf("%q primitive = %T, want Ellipse", arrowhead, node.Primitive)
		}
		assertPaintRole(t, arrowhead, ellipse.Fill, ellipse.Stroke, strokePaint, nil)
	case d2target.CircleArrowhead:
		ellipse, ok := node.Primitive.(d2scene.Ellipse)
		if !ok {
			t.Fatalf("%q primitive = %T, want Ellipse", arrowhead, node.Primitive)
		}
		assertPaintRole(t, arrowhead, ellipse.Fill, ellipse.Stroke, background, strokePaint)
	case d2target.CrossArrowhead, d2target.CfOne, d2target.CfMany, d2target.CfOneRequired, d2target.CfManyRequired:
		if node.Primitive != nil || len(node.Children) != 2 {
			t.Fatalf("%q node = primitive %T, %d children; want two-child group", arrowhead, node.Primitive, len(node.Children))
		}
		for _, child := range node.Children {
			switch primitive := child.Primitive.(type) {
			case d2scene.Path:
				assertPaintRole(t, arrowhead, primitive.Fill, primitive.Stroke, background, strokePaint)
			case d2scene.Ellipse:
				assertPaintRole(t, arrowhead, primitive.Fill, primitive.Stroke, background, strokePaint)
			default:
				t.Fatalf("%q child primitive = %T, want Path/Ellipse", arrowhead, child.Primitive)
			}
		}
	default:
		t.Fatalf("missing paint-role assertion for %q", arrowhead)
	}
}

func assertPaintRole(t *testing.T, arrowhead d2target.Arrowhead, fill d2scene.Paint, stroke *d2scene.Stroke, wantFill, wantStroke d2scene.Paint) {
	t.Helper()
	if !reflect.DeepEqual(fill, wantFill) {
		t.Errorf("%q fill = %#v, want %#v", arrowhead, fill, wantFill)
	}
	if wantStroke == nil {
		if stroke != nil {
			t.Errorf("%q stroke = %#v, want nil", arrowhead, stroke)
		}
		return
	}
	if stroke == nil || !reflect.DeepEqual(stroke.Paint, wantStroke) || stroke.Width != 2 {
		t.Errorf("%q stroke = %#v, want width 2 with paint %#v", arrowhead, stroke, wantStroke)
	}
}

func pathCommands(points ...d2scene.Point) []d2scene.PathCommand {
	commands := []d2scene.PathCommand{d2scene.MoveTo(points[0].X, points[0].Y)}
	for _, point := range points[1:] {
		commands = append(commands, d2scene.LineTo(point.X, point.Y))
	}
	return commands
}

func closedPathCommands(points ...d2scene.Point) []d2scene.PathCommand {
	return append(pathCommands(points...), d2scene.ClosePath())
}
