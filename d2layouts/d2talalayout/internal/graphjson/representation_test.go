package graphjson

import (
	"reflect"
	"testing"
)

func TestSerializedStyleJSONSchemaMatchesRuntimeStyle(t *testing.T) {
	expected := []struct {
		name string
		tag  string
	}{
		{name: "Opacity", tag: "opacity,omitempty"},
		{name: "Stroke", tag: "stroke,omitempty"},
		{name: "Fill", tag: "fill,omitempty"},
		{name: "FillPattern", tag: "fillPattern,omitempty"},
		{name: "StrokeWidth", tag: "strokeWidth,omitempty"},
		{name: "StrokeDash", tag: "strokeDash,omitempty"},
		{name: "BorderRadius", tag: "borderRadius,omitempty"},
		{name: "Shadow", tag: "shadow,omitempty"},
		{name: "ThreeDee", tag: "3d,omitempty"},
		{name: "Multiple", tag: "multiple,omitempty"},
		{name: "Font", tag: "font,omitempty"},
		{name: "FontSize", tag: "fontSize,omitempty"},
		{name: "FontColor", tag: "fontColor,omitempty"},
		{name: "Animated", tag: "animated,omitempty"},
		{name: "Bold", tag: "bold,omitempty"},
		{name: "Italic", tag: "italic,omitempty"},
		{name: "Underline", tag: "underline,omitempty"},
		{name: "Filled", tag: "filled,omitempty"},
		{name: "DoubleBorder", tag: "doubleBorder,omitempty"},
		{name: "TextTransform", tag: "textTransform,omitempty"},
	}

	styleType := reflect.TypeFor[SerializedStyle]()
	scalarType := reflect.TypeFor[*SerializedScalar]()
	if styleType.NumField() != len(expected) {
		t.Fatalf("SerializedStyle has %d fields, want %d", styleType.NumField(), len(expected))
	}
	for i, want := range expected {
		field := styleType.Field(i)
		if field.Name != want.name || field.Type != scalarType || field.Tag.Get("json") != want.tag {
			t.Fatalf("field %d = %s %v %q, want %s %v %q", i, field.Name, field.Type, field.Tag.Get("json"), want.name, scalarType, want.tag)
		}
	}

	scalar := reflect.TypeFor[SerializedScalar]()
	if scalar.NumField() != 1 || scalar.Field(0).Name != "Value" || scalar.Field(0).Type.Kind() != reflect.String || scalar.Field(0).Tag.Get("json") != "value" {
		t.Fatalf("SerializedScalar schema changed: %v", scalar)
	}
}
