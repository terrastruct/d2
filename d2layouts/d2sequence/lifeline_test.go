package d2sequence_test

import (
	"testing"

	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2sequence"
	"github.com/d2lang/d2/lib/geo"
)

func TestLifelineEndIDArchitectureStable(t *testing.T) {
	const actorID = "a"
	const want = "a-lifeline-end-2251863791"

	if got := d2sequence.LifelineEndID(actorID); got != want {
		t.Fatalf("LifelineEndID(%q) = %q, want %q", actorID, got, want)
	}

	legacySignedID := "a-lifeline-end--2043103505"
	if d2sequence.IsLifelineEnd(&d2graph.Object{ID: legacySignedID}) {
		t.Fatalf("IsLifelineEnd accepted architecture-dependent ID %q", legacySignedID)
	}
}

func TestIsLifelineEnd(t *testing.T) {
	actorWithMarker := "actor-lifeline-end-archive"
	validID := d2sequence.LifelineEndID(actorWithMarker)

	tests := []struct {
		name string
		obj  *d2graph.Object
		want bool
	}{
		{name: "nil", obj: nil},
		{name: "valid", obj: &d2graph.Object{ID: validID}, want: true},
		{name: "wrong hash", obj: &d2graph.Object{ID: actorWithMarker + "-lifeline-end-0"}},
		{name: "ordinary object", obj: &d2graph.Object{ID: actorWithMarker}},
		{name: "graph member", obj: &d2graph.Object{ID: validID, Graph: &d2graph.Graph{}}},
		{name: "child", obj: &d2graph.Object{ID: validID, Parent: &d2graph.Object{}}},
		{name: "positioned", obj: &d2graph.Object{ID: validID, Box: geo.NewBox(nil, 1, 1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := d2sequence.IsLifelineEnd(test.obj); got != test.want {
				t.Fatalf("IsLifelineEnd(%#v) = %t, want %t", test.obj, got, test.want)
			}
		})
	}
}
