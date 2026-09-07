package d2isometricimg

import (
	"context"
	"testing"
)

func TestSVGFragmentIndexBoundsLiveReferences(t *testing.T) {
	limits := svgDefaultVisibilityLimits
	limits.gridReferences = 2
	budget := &svgVisibilityBudget{ctx: context.Background(), limits: limits}
	s, err := svgNewVisibleFragmentSet(svgVisibilityRectangle(0, 0, 100, 100, 0, 0, 0), budget, 100, 400)
	if err != nil {
		t.Fatal(err)
	}
	small := svgVisibilityRectangle(1, 1, 2, 2, 0, 0, 0)
	if err := s.add(small); err != nil {
		t.Fatal(err)
	}
	if err := s.add(small); err == nil {
		t.Fatal("fragment index exceeded its reference budget")
	}
	if s.references != 2 || len(s.fragments) != 2 || s.vertices != 8 {
		t.Fatal("rejected fragment changed the active index")
	}
	id := s.next
	if err := s.changeCells(id, s.fragments[id].box, false); err != nil {
		t.Fatal(err)
	}
	delete(s.fragments, id)
	s.vertices -= len(small)
	if err := s.add(small); err != nil || s.references != 2 {
		t.Fatalf("removed fragments did not release their references: %v", err)
	}
}
