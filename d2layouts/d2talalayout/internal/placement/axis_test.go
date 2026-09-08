package placement

import "testing"

func TestInvalidAxisAndDirectionRemainInvalidWhenReversed(t *testing.T) {
	if got := invalidAxis.opposite(); got != invalidAxis {
		t.Fatalf("invalid axis opposite = %v, want invalid", got)
	}
	if got := invalidDirection.opposite(); got != invalidDirection {
		t.Fatalf("invalid direction opposite = %v, want invalid", got)
	}
}
