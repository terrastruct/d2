package layoutgraph

import (
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
)

func TestIsTargetedToSourceArrowEndpoint(t *testing.T) {
	from := NewNode(1, 10, 10)
	to := NewNode(2, 10, 10)
	other := NewNode(3, 10, 10)
	edge := NewEdge(from, to)
	edge.SourceArrowhead = TriangleArrowhead
	edge.TargetArrowhead = NoArrowhead

	if !edge.isTargetedTo(from) {
		t.Fatal("source-arrow-only edge should target its From endpoint")
	}
	if edge.isTargetedTo(to) {
		t.Fatal("source-arrow-only edge should not target its To endpoint")
	}
	if edge.isTargetedTo(other) {
		t.Fatal("edge should not target an unrelated node")
	}
}

func TestEdgeGetBoundingBox(t *testing.T) {
	e := NewEdge(NewNode(1, 100, 100), NewNode(2, 100, 100))

	tl, br := e.bounds()
	assert.Equal(t, math.Inf(1), tl.X)
	assert.Equal(t, math.Inf(1), tl.Y)
	assert.Equal(t, math.Inf(-1), br.X)
	assert.Equal(t, math.Inf(-1), br.Y)

	e.Points = []*geo.Point{
		geo.NewPoint(1., 5.),
		geo.NewPoint(10., 5.),
		geo.NewPoint(10., 50.),
		geo.NewPoint(1., 50.),
	}
	tl, br = e.bounds()
	assert.Equal(t, 1., tl.X)
	assert.Equal(t, 5., tl.Y)
	assert.Equal(t, 10., br.X)
	assert.Equal(t, 50., br.Y)

	e.Label = &Label{
		Position: label.OutsideBottomRight,
		Width:    100,
		Height:   100,
	}

	labelTL := e.LabelTopLeft(e.Label.Position, e.Label.Width, e.Label.Height)
	tl, br = e.bounds()
	assert.Equal(t, math.Min(1., math.Round(labelTL.X)), tl.X)
	assert.Equal(t, math.Min(5., math.Round(labelTL.Y)), tl.Y)
	assert.Equal(t, 10., br.X)
	assert.Equal(t, math.Round(labelTL.Y+e.Label.Height), br.Y)
}

func TestEdgeBoundingBoxIncludesRendererCompatibleArrowheadLabels(t *testing.T) {
	edge := NewEdge(NewNode(1, 10, 10), NewNode(2, 10, 10))
	edge.Points = geo.Route{geo.NewPoint(0, 0), geo.NewPoint(100, 0)}
	edge.SourceArrowhead = TriangleArrowhead
	edge.TargetArrowhead = TriangleArrowhead
	edge.SourceArrowheadLabel = &Label{Text: "source", Width: 30, Height: 12}
	edge.TargetArrowheadLabel = &Label{Text: "target", Width: 40, Height: 14}

	topLeft, bottomRight := edge.bounds()
	if !topLeft.Equals(geo.NewPoint(0, -22)) || !bottomRight.Equals(geo.NewPoint(100, 0)) {
		t.Fatalf("bounding box = %v to %v, want (0, -22) to (100, 0)", topLeft, bottomRight)
	}
}

func TestEquivalentStylesClassifiesEveryOwnedStyleField(t *testing.T) {
	affecting := map[string]struct{}{
		"Opacity": {}, "Stroke": {}, "StrokeWidth": {}, "StrokeDash": {}, "Animated": {},
	}
	ignored := map[string]struct{}{
		"Fill": {}, "FillPattern": {}, "BorderRadius": {}, "Shadow": {}, "ThreeDee": {},
		"Multiple": {}, "Font": {}, "FontSize": {}, "FontColor": {}, "Bold": {},
		"Italic": {}, "Underline": {}, "Filled": {}, "DoubleBorder": {}, "TextTransform": {},
	}

	styleType := reflect.TypeFor[EdgeStyle]()
	scalarType := reflect.TypeFor[*StyleScalar]()
	if styleType.NumField() != len(affecting)+len(ignored) {
		t.Fatalf("EdgeStyle has %d fields; classified %d", styleType.NumField(), len(affecting)+len(ignored))
	}
	for i := 0; i < styleType.NumField(); i++ {
		field := styleType.Field(i)
		_, affectsRouting := affecting[field.Name]
		_, isIgnored := ignored[field.Name]
		if affectsRouting == isIgnored {
			t.Fatalf("EdgeStyle.%s must be classified exactly once", field.Name)
		}
		if field.Type != scalarType {
			t.Fatalf("EdgeStyle.%s has type %v, want %v", field.Name, field.Type, scalarType)
		}

		leftValue := reflect.New(styleType).Elem()
		rightValue := reflect.New(styleType).Elem()
		leftValue.Field(i).Set(reflect.ValueOf(&StyleScalar{Value: "left"}))
		rightValue.Field(i).Set(reflect.ValueOf(&StyleScalar{Value: "right"}))
		left := &Edge{Style: leftValue.Interface().(EdgeStyle)}
		right := &Edge{Style: rightValue.Interface().(EdgeStyle)}
		if got := left.EquivalentStyles(right); got == affectsRouting {
			t.Fatalf("EquivalentStyles with different %s = %v, want %v", field.Name, got, !affectsRouting)
		}

		rightValue.Field(i).Set(reflect.ValueOf(&StyleScalar{Value: "left"}))
		right.Style = rightValue.Interface().(EdgeStyle)
		if !left.EquivalentStyles(right) {
			t.Fatalf("EquivalentStyles rejected matching %s", field.Name)
		}
	}
}
