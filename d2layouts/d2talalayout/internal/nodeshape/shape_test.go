package nodeshape

import (
	"testing"

	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/shape"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		kind     Kind
	}{
		{name: "default", typeName: "", kind: Square},
		{name: "square", typeName: shape.SQUARE_TYPE, kind: Square},
		{name: "real square", typeName: shape.REAL_SQUARE_TYPE, kind: RealSquare},
		{name: "parallelogram", typeName: shape.PARALLELOGRAM_TYPE, kind: Parallelogram},
		{name: "document", typeName: shape.DOCUMENT_TYPE, kind: Document},
		{name: "cylinder", typeName: shape.CYLINDER_TYPE, kind: Cylinder},
		{name: "queue", typeName: shape.QUEUE_TYPE, kind: Queue},
		{name: "page", typeName: shape.PAGE_TYPE, kind: Page},
		{name: "package", typeName: shape.PACKAGE_TYPE, kind: Package},
		{name: "step", typeName: shape.STEP_TYPE, kind: Step},
		{name: "callout", typeName: shape.CALLOUT_TYPE, kind: Callout},
		{name: "stored data", typeName: shape.STORED_DATA_TYPE, kind: StoredData},
		{name: "person", typeName: shape.PERSON_TYPE, kind: Person},
		{name: "c4 person", typeName: shape.C4_PERSON_TYPE, kind: C4Person},
		{name: "diamond", typeName: shape.DIAMOND_TYPE, kind: Diamond},
		{name: "oval", typeName: shape.OVAL_TYPE, kind: Oval},
		{name: "circle", typeName: shape.CIRCLE_TYPE, kind: Circle},
		{name: "hexagon", typeName: shape.HEXAGON_TYPE, kind: Hexagon},
		{name: "cloud", typeName: shape.CLOUD_TYPE, kind: Cloud},
		{name: "table", typeName: shape.TABLE_TYPE, kind: Table},
		{name: "class", typeName: shape.CLASS_TYPE, kind: Class},
		{name: "text", typeName: shape.TEXT_TYPE, kind: Text},
		{name: "code", typeName: shape.CODE_TYPE, kind: Code},
		{name: "image", typeName: shape.IMAGE_TYPE, kind: Image},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			box := geo.Box{TopLeft: geo.NewPoint(10, 20), Width: 100, Height: 50}
			got, kind, ok := New(test.typeName, &box)
			if !ok {
				t.Fatalf("New(%q) rejected a supported shape", test.typeName)
			}
			if kind != test.kind {
				t.Fatalf("New(%q) kind = %v, want %v", test.typeName, kind, test.kind)
			}
			if got.GetType() != test.typeName {
				t.Fatalf("New(%q) type = %q", test.typeName, got.GetType())
			}
			if got.GetBox() != &box {
				t.Fatalf("New(%q) did not retain the supplied box", test.typeName)
			}
			wantKindString := test.typeName
			if wantKindString == "" {
				wantKindString = shape.SQUARE_TYPE
			}
			if kind.String() != wantKindString {
				t.Fatalf("New(%q) kind string = %q, want %q", test.typeName, kind.String(), wantKindString)
			}
		})
	}
}

func TestNewRejectsUnsupportedShape(t *testing.T) {
	box := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 10, Height: 10}
	got, _, ok := New("unsupported", &box)
	if ok || got != nil {
		t.Fatalf("New accepted an unsupported shape: %#v", got)
	}
}

func TestUnknownKindString(t *testing.T) {
	if got := Kind(255).String(); got != "" {
		t.Fatalf("unknown kind string = %q, want empty", got)
	}
}

func TestTableShapeColumns(t *testing.T) {
	box := geo.Box{TopLeft: geo.NewPoint(0, 0), Width: 100, Height: 100}
	got, _, ok := New(shape.TABLE_TYPE, &box)
	if !ok {
		t.Fatal("New rejected table")
	}
	if NumColumns(got) != 0 {
		t.Fatalf("initial column count = %d, want 0", NumColumns(got))
	}
	SetNumColumns(got, 4)
	if NumColumns(got) != 4 {
		t.Fatalf("column count = %d, want 4", NumColumns(got))
	}
}
