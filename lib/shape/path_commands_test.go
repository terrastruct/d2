package shape_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
	"github.com/d2lang/d2/lib/svg"
)

func TestCustomShapesProvideTypedPathCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		shapeType string
		pathCount int
	}{
		{shape.CALLOUT_TYPE, 1},
		{shape.CLOUD_TYPE, 1},
		{shape.CYLINDER_TYPE, 2},
		{shape.DIAMOND_TYPE, 1},
		{shape.DOCUMENT_TYPE, 1},
		{shape.HEXAGON_TYPE, 1},
		{shape.PACKAGE_TYPE, 1},
		{shape.PAGE_TYPE, 2},
		{shape.PARALLELOGRAM_TYPE, 1},
		{shape.PERSON_TYPE, 1},
		{shape.C4_PERSON_TYPE, 2},
		{shape.QUEUE_TYPE, 2},
		{shape.STEP_TYPE, 1},
		{shape.STORED_DATA_TYPE, 1},
	}

	for _, tt := range tests {
		t.Run(tt.shapeType, func(t *testing.T) {
			t.Parallel()

			s := shape.NewShape(tt.shapeType, geo.NewBox(geo.NewPoint(7.5, -4.25), 120, 90))
			provider, ok := s.(shape.PathCommandProvider)
			if !ok {
				t.Fatalf("%T does not implement shape.PathCommandProvider", s)
			}

			paths := provider.GetPathCommands()
			if len(paths) != tt.pathCount {
				t.Fatalf("GetPathCommands() returned %d paths, want %d", len(paths), tt.pathCount)
			}
			if got := shape.GetPathCommands(s); !reflect.DeepEqual(got, paths) {
				t.Fatalf("shape.GetPathCommands() = %#v, provider returned %#v", got, paths)
			}
			if data := s.GetSVGPathData(); len(data) != len(paths) {
				t.Fatalf("GetSVGPathData() returned %d paths, typed provider returned %d", len(data), len(paths))
			}

			for pathIndex, commands := range paths {
				if len(commands) == 0 {
					t.Fatalf("path %d has no commands", pathIndex)
				}
				if commands[0].Kind != svg.PathCommandMove {
					t.Fatalf("path %d starts with command kind %d, want move", pathIndex, commands[0].Kind)
				}
				for commandIndex, command := range commands {
					if command.Kind > svg.PathCommandClose {
						t.Fatalf("path %d command %d has unknown kind %d", pathIndex, commandIndex, command.Kind)
					}
					for pointName, point := range map[string]geo.Point{
						"control1": command.Control1,
						"control2": command.Control2,
						"end":      command.End,
					} {
						if math.IsNaN(point.X) || math.IsInf(point.X, 0) || math.IsNaN(point.Y) || math.IsInf(point.Y, 0) {
							t.Fatalf("path %d command %d %s is not finite: %#v", pathIndex, commandIndex, pointName, point)
						}
					}
				}
			}

			first := paths[0][0]
			paths[0][0].End.X++
			paths[0] = nil
			fresh := provider.GetPathCommands()
			if len(fresh) == 0 || len(fresh[0]) == 0 || fresh[0][0] != first {
				t.Fatalf("GetPathCommands() did not return a defensive copy: %#v", fresh)
			}
		})
	}
}

func TestPrimitiveShapeHasNoCustomPathCommands(t *testing.T) {
	t.Parallel()

	rectangle := shape.NewShape(shape.SQUARE_TYPE, geo.NewBox(geo.NewPoint(0, 0), 10, 20))
	if commands := shape.GetPathCommands(rectangle); commands != nil {
		t.Fatalf("shape.GetPathCommands(rectangle) = %#v, want nil", commands)
	}
}
