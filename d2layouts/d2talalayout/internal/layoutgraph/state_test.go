package layoutgraph

import (
	"testing"
	"unsafe"
)

func TestEntityIDHasArchitectureIndependentWidth(t *testing.T) {
	var id EntityID
	if got, want := unsafe.Sizeof(id), uintptr(8); got != want {
		t.Fatalf("EntityID width = %d bytes, want %d", got, want)
	}
}

func TestFixedLabelAndIconPositions(t *testing.T) {
	var label Label
	if label.PositionFixed() {
		t.Fatal("zero-value label position is fixed")
	}
	label.FixPosition()
	if !label.PositionFixed() {
		t.Fatal("FixPosition did not fix the label position")
	}

	var icon Icon
	if icon.PositionFixed() {
		t.Fatal("zero-value icon position is fixed")
	}
	icon.FixPosition()
	if !icon.PositionFixed() {
		t.Fatal("FixPosition did not fix the icon position")
	}
}
