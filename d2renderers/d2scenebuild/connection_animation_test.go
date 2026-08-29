package d2scenebuild

import (
	"context"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/svg"
)

func TestBuildAnimatedConnectionDashTimingAndDirection(t *testing.T) {
	for _, test := range []struct {
		name       string
		srcArrow   d2target.Arrowhead
		dstArrow   d2target.Arrowhead
		wantIDs    []string
		wantStartX float64
		wantEndX   float64
		baseSign   float64
	}{
		{
			name:       "neither arrow splits in two directions",
			srcArrow:   d2target.NoArrowhead,
			dstArrow:   d2target.NoArrowhead,
			wantIDs:    []string{"a-b:path:reverse", "a-b:path:forward"},
			wantStartX: 22, wantEndX: 78, baseSign: -1,
		},
		{
			name:     "both arrows split in two directions",
			srcArrow: d2target.TriangleArrowhead, dstArrow: d2target.TriangleArrowhead,
			wantIDs:    []string{"a-b:path:reverse", "a-b:src-arrowhead", "a-b:path:forward", "a-b:dst-arrowhead"},
			wantStartX: 24, wantEndX: 76, baseSign: -1,
		},
		{
			name:       "source arrow reverses the single path offset",
			srcArrow:   d2target.TriangleArrowhead,
			dstArrow:   d2target.NoArrowhead,
			wantIDs:    []string{"a-b:path", "a-b:src-arrowhead"},
			wantStartX: 24, wantEndX: 78, baseSign: 1,
		},
		{
			name:       "destination arrow keeps the single path offset forward",
			srcArrow:   d2target.NoArrowhead,
			dstArrow:   d2target.TriangleArrowhead,
			wantIDs:    []string{"a-b:path", "a-b:dst-arrowhead"},
			wantStartX: 22, wantEndX: 76, baseSign: -1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagram := validDiagram()
			connection := &diagram.Connections[0]
			connection.Animated = true
			connection.SrcArrow = test.srcArrow
			connection.DstArrow = test.dstArrow
			document := buildAnimatedConnectionDocument(t, diagram)
			group := findSceneNode(t, document.Root, connection.ID)
			assertChildIDs(t, group, test.wantIDs)

			dashSize, gapSize := svg.GetStrokeDashAttributes(float64(connection.StrokeWidth), 5)
			baseOffset := test.baseSign * 10 * (dashSize + gapSize)
			duration := time.Duration(gapSize * .5 * float64(time.Second))
			split := (connection.SrcArrow == d2target.NoArrowhead) == (connection.DstArrow == d2target.NoArrowhead)
			var pathNodes []*d2scene.Node
			if split {
				pathNodes = []*d2scene.Node{
					findSceneNode(t, group, connection.ID+":path:reverse"),
					findSceneNode(t, group, connection.ID+":path:forward"),
				}
			} else {
				pathNodes = []*d2scene.Node{findSceneNode(t, group, connection.ID+":path")}
			}
			for i, pathNode := range pathNodes {
				path := pathNode.Primitive.(d2scene.Path)
				if path.Stroke == nil || !reflect.DeepEqual(path.Stroke.Dashes, []float64{dashSize, gapSize}) {
					t.Fatalf("path %d dashes = %+v, want [%v %v]", i, path.Stroke, dashSize, gapSize)
				}
				if path.Stroke.DashOffset != baseOffset {
					t.Errorf("path %d base offset = %v, want %v", i, path.Stroke.DashOffset, baseOffset)
				}
				if len(pathNode.Animations) != 1 {
					t.Fatalf("path %d animations = %d, want one", i, len(pathNode.Animations))
				}
				track := pathNode.Animations[0]
				if track.Property != d2scene.AnimateStrokeDashOffset || track.Delay != 0 || track.Duration != duration || !track.Repeat {
					t.Errorf("path %d track = %+v, want repeating dash offset with duration %s", i, track, duration)
				}
			}

			if split {
				reverse := pathNodes[0].Primitive.(d2scene.Path)
				forward := pathNodes[1].Primitive.(d2scene.Path)
				wantReverse := []d2scene.PathCommand{
					d2scene.MoveTo(test.wantStartX, 10), d2scene.LineTo(50, 10),
				}
				wantForward := []d2scene.PathCommand{
					d2scene.MoveTo(50, 10), d2scene.LineTo(test.wantEndX, 10),
				}
				if !reflect.DeepEqual(reverse.Commands, wantReverse) || !reflect.DeepEqual(forward.Commands, wantForward) {
					t.Fatalf("split line geometry = %+v / %+v, want %+v / %+v", reverse.Commands, forward.Commands, wantReverse, wantForward)
				}
				assertAnimatedConnectionSamples(t, pathNodes[0].Animations[0], baseOffset, 0)
				assertAnimatedConnectionSamples(t, pathNodes[1].Animations[0], 0, baseOffset)
			} else {
				path := pathNodes[0].Primitive.(d2scene.Path)
				want := []d2scene.PathCommand{
					d2scene.MoveTo(test.wantStartX, 10), d2scene.LineTo(test.wantEndX, 10),
				}
				if !reflect.DeepEqual(path.Commands, want) {
					t.Fatalf("single animated line geometry = %+v, want %+v", path.Commands, want)
				}
				assertAnimatedConnectionSamples(t, pathNodes[0].Animations[0], 0, baseOffset)
			}
		})
	}
}

func TestBuildAnimatedConnectionPreservesExplicitDash(t *testing.T) {
	diagram := validDiagram()
	connection := &diagram.Connections[0]
	connection.Animated = true
	connection.SrcArrow = d2target.TriangleArrowhead
	connection.StrokeDash = 3.25
	document := buildAnimatedConnectionDocument(t, diagram)
	pathNode := findSceneNode(t, document.Root, "a-b:path")
	path := pathNode.Primitive.(d2scene.Path)
	dashSize, gapSize := svg.GetStrokeDashAttributes(2, 3.25)
	if !reflect.DeepEqual(path.Stroke.Dashes, []float64{dashSize, gapSize}) {
		t.Fatalf("explicit animated dashes = %v, want [%v %v]", path.Stroke.Dashes, dashSize, gapSize)
	}
	wantOffset := 10 * (dashSize + gapSize)
	if path.Stroke.DashOffset != wantOffset || pathNode.Animations[0].Keyframes[1].Value.Number != wantOffset {
		t.Fatalf("explicit animated offset = %v / %+v, want %v", path.Stroke.DashOffset, pathNode.Animations[0], wantOffset)
	}
}

func TestBuildAnimatedConnectionCubicSplit(t *testing.T) {
	diagram := validDiagram()
	for i := range diagram.Shapes {
		diagram.Shapes[i].StrokeWidth = 0
	}
	connection := &diagram.Connections[0]
	connection.Animated = true
	connection.IsCurve = true
	connection.Route = []*geo.Point{
		{X: 0, Y: -1}, {X: 0, Y: 100}, {X: 100, Y: 100}, {X: 100, Y: -1},
	}
	document := buildAnimatedConnectionDocument(t, diagram)
	group := findSceneNode(t, document.Root, connection.ID)

	svgFirst, svgSecond, err := svg.SplitPath("M 0.000000 0.000000 C 0.000000 100.000000 100.000000 100.000000 100.000000 0.000000", .5)
	if err != nil {
		t.Fatal(err)
	}
	if svgFirst != "M 0.000000 0.000000 C 0.000000 50.000000 25.000000 75.000000 50.000000 75.000000 " ||
		svgSecond != "M 50.000000 75.000000 C 75.000000 75.000000 100.000000 50.000000 100.000000 0.000000 " {
		t.Fatalf("cubic SplitPath result = %q / %q", svgFirst, svgSecond)
	}
	wantReverse := []d2scene.PathCommand{
		d2scene.MoveTo(0, 0), d2scene.CubicTo(0, 50, 25, 75, 50, 75),
	}
	wantForward := []d2scene.PathCommand{
		d2scene.MoveTo(50, 75), d2scene.CubicTo(75, 75, 100, 50, 100, 0),
	}
	assertAnimatedConnectionGeometry(t, group, wantReverse, wantForward)
}

func TestBuildAnimatedConnectionSmoothSplit(t *testing.T) {
	diagram := validDiagram()
	for i := range diagram.Shapes {
		diagram.Shapes[i].StrokeWidth = 0
	}
	connection := &diagram.Connections[0]
	connection.Animated = true
	connection.Route = []*geo.Point{{X: -1, Y: 0}, {X: 100, Y: 0}, {X: 100, Y: 101}}
	document := buildAnimatedConnectionDocument(t, diagram)
	group := findSceneNode(t, document.Root, connection.ID)

	svgFirst, svgSecond, err := svg.SplitPath("M 0.000000 0.000000 L 90.000000 0.000000 S 100.000000 0.000000 100.000000 10.000000 L 100.000000 100.000000", .5)
	if err != nil {
		t.Fatal(err)
	}
	if svgFirst != "M 0.000000 0.000000 L 90.000000 0.000000 S 100.000000 0.000000 100.000000 10.000000 " ||
		svgSecond != "M 100.000000 10.000000 L 100.000000 100.000000 " {
		t.Fatalf("smooth SplitPath result = %q / %q", svgFirst, svgSecond)
	}
	// The smooth-curve transition remains wholly in the reverse path. Treating
	// the typed cubic as a C command would incorrectly bisect it.
	wantReverse := []d2scene.PathCommand{
		d2scene.MoveTo(0, 0),
		d2scene.LineTo(90, 0),
		d2scene.CubicTo(90, 0, 100, 0, 100, 10),
	}
	wantForward := []d2scene.PathCommand{
		d2scene.MoveTo(100, 10), d2scene.LineTo(100, 100),
	}
	assertAnimatedConnectionGeometry(t, group, wantReverse, wantForward)
}

func TestBuildAnimatedConnectionPreservesMaskMarkersAndLabelOrder(t *testing.T) {
	diagram := validDiagram()
	connection := &diagram.Connections[0]
	connection.Animated = true
	connection.SrcArrow = d2target.TriangleArrowhead
	connection.DstArrow = d2target.TriangleArrowhead
	connection.Text = d2target.Text{
		Label: "edge", FontSize: 16, FontFamily: "default", LabelWidth: 32, LabelHeight: 18,
	}
	connection.Fill = "#eeeeee"
	connection.LabelPosition = "INSIDE_MIDDLE_CENTER"
	document := buildAnimatedConnectionDocument(t, diagram)
	group := findSceneNode(t, document.Root, connection.ID)
	if len(group.Children) < 2 || group.Children[0].ID != "a-b:geometry" || group.Children[1].ID != "a-b:label-fill" {
		t.Fatalf("animated connection group order = %+v", group.Children)
	}
	geometry := group.Children[0]
	if geometry.Mask == nil || geometry.Mask.Type != d2scene.MaskLuminance {
		t.Fatalf("animated connection geometry lost label mask: %+v", geometry.Mask)
	}
	assertChildIDs(t, geometry, []string{
		"a-b:path:reverse", "a-b:src-arrowhead", "a-b:path:forward", "a-b:dst-arrowhead",
	})
}

func TestBuildAnimatedConnectionRejectsUnrepresentableWork(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*d2target.Connection)
		want   string
	}{
		{
			name:   "implicit dash is non-finite for stroke width",
			mutate: func(connection *d2target.Connection) { connection.StrokeWidth = 18 },
			want:   "strokeDash",
		},
		{
			name:   "duration rounds below one nanosecond",
			mutate: func(connection *d2target.Connection) { connection.StrokeDash = 1e-20 },
			want:   "animation duration",
		},
		{
			name: "symmetric route has zero painted length",
			mutate: func(connection *d2target.Connection) {
				connection.Route[1] = connection.Route[0].Copy()
			},
			want: "zero painted length",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			diagram := validDiagram()
			connection := &diagram.Connections[0]
			connection.Animated = true
			test.mutate(connection)
			_, err := Build(context.Background(), diagram, Options{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build() error = %v, want %q", err, test.want)
			}
		})
	}
}

func buildAnimatedConnectionDocument(t *testing.T, diagram *d2target.Diagram) *d2scene.Document {
	t.Helper()
	pad := int64(0)
	document, err := Build(context.Background(), diagram, Options{Pad: &pad})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return document
}

func assertAnimatedConnectionSamples(t *testing.T, track d2scene.Track, start, end float64) {
	t.Helper()
	if track.Property != d2scene.AnimateStrokeDashOffset || track.Delay != 0 || track.Duration <= 0 || !track.Repeat {
		t.Fatalf("animated connection track = %+v", track)
	}
	if len(track.Keyframes) != 2 || track.Keyframes[0].Offset != 0 || track.Keyframes[1].Offset != 1 ||
		track.Keyframes[0].Easing.Kind != d2scene.EaseLinear || track.Keyframes[1].Easing.Kind != d2scene.EaseLinear ||
		track.Keyframes[0].Value.Kind != d2scene.NumberAnimationValue || track.Keyframes[1].Value.Kind != d2scene.NumberAnimationValue ||
		math.Abs(track.Keyframes[0].Value.Number-start) > 1e-12 || math.Abs(track.Keyframes[1].Value.Number-end) > 1e-12 {
		t.Fatalf("animated connection keyframes = %+v, want %v to %v", track.Keyframes, start, end)
	}
}

func assertAnimatedConnectionGeometry(t *testing.T, group *d2scene.Node, wantReverse, wantForward []d2scene.PathCommand) {
	t.Helper()
	if len(group.Children) < 2 {
		t.Fatalf("animated connection children = %d, want at least two paths", len(group.Children))
	}
	reverse := group.Children[0].Primitive.(d2scene.Path)
	forward := group.Children[1].Primitive.(d2scene.Path)
	if !reflect.DeepEqual(reverse.Commands, wantReverse) || !reflect.DeepEqual(forward.Commands, wantForward) {
		t.Fatalf("animated split geometry = %+v / %+v, want %+v / %+v", reverse.Commands, forward.Commands, wantReverse, wantForward)
	}
}
