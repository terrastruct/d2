package invariant

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	err := New("broken ownership")
	if got, want := err.Error(), "layout invariant violated: broken ownership"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if !errors.Is(err, ErrViolation) {
		t.Fatalf("errors.Is(%v, ErrViolation) = false", err)
	}
}

func TestErrorf(t *testing.T) {
	err := Errorf("node %d has no owner", 42)
	if got, want := err.Error(), "layout invariant violated: node 42 has no owner"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if !errors.Is(err, ErrViolation) {
		t.Fatalf("errors.Is(%v, ErrViolation) = false", err)
	}
}
