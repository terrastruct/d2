package d2elklayout_test

import (
	"context"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2layouts/d2elklayout"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/log"
)

func TestAncestorDescendantEdgeMatchesELKJS(t *testing.T) {
	t.Parallel()

	g, _, err := d2compiler.Compile("", strings.NewReader(`direction: down
a: {
  width: 80
  height: 60
  b: {
    width: 80
    height: 60
    c: { width: 80; height: 60 }
  }
}
a -> a.b.c
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, obj := range g.Objects {
		obj.Box = geo.NewBox(nil, 80, 60)
	}
	ctx := log.WithTB(context.Background(), t)
	if err := d2elklayout.DefaultLayout(ctx, g); err != nil {
		t.Fatal(err)
	}
	wantObjects := map[string][4]float64{
		"a":     {12, 12, 320, 305},
		"a.b":   {62, 107, 180, 160},
		"a.b.c": {112, 157, 80, 60},
	}
	if len(g.Objects) != len(wantObjects) {
		t.Fatalf("objects = %d, want %d", len(g.Objects), len(wantObjects))
	}
	for _, obj := range g.Objects {
		want, ok := wantObjects[obj.AbsID()]
		if !ok {
			t.Fatalf("unexpected object %q", obj.AbsID())
		}
		assertNear(t, obj.AbsID()+" x", obj.TopLeft.X, want[0], 1e-9)
		assertNear(t, obj.AbsID()+" y", obj.TopLeft.Y, want[1], 1e-9)
		assertNear(t, obj.AbsID()+" width", obj.Width, want[2], 1e-9)
		assertNear(t, obj.AbsID()+" height", obj.Height, want[3], 1e-9)
	}

	if len(g.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(g.Edges))
	}
	wantRoute := [][2]float64{{282, 317}, {282, 62}, {152, 62}, {152, 157}}
	if len(g.Edges[0].Route) != len(wantRoute) {
		t.Fatalf("route points = %d, want %d", len(g.Edges[0].Route), len(wantRoute))
	}
	for i, point := range g.Edges[0].Route {
		assertNear(t, "route point x", point.X, wantRoute[i][0], 1e-9)
		assertNear(t, "route point y", point.Y, wantRoute[i][1], 1e-9)
	}
}
