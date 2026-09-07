package d2isometricimg

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func svgOrderTestUnit(first int, polygons ...[]svgPoint) *svgPaintUnit {
	unit := &svgPaintUnit{first: first, opacity: 1}
	for i, points := range polygons {
		unit.batches = append(unit.batches, &svgPaintBatch{first: first + i, polygons: [][]svgPoint{points}})
		for _, p := range points {
			unit.depth += p.z
			unit.count++
		}
	}
	unit.depth /= float64(unit.count)
	return unit
}

func TestSVGPaintOrderLargePlanesCannotCoverNearLabels(t *testing.T) {
	// The page's mean depth is 50; the title's mean depth is only 7.5.
	// On their actual overlap, every title point is one unit above the page.
	page := svgOrderTestUnit(5, svgVisibilityRectangle(0, 0, 100, 100, 0, 0, 1))
	title := svgOrderTestUnit(0, svgVisibilityRectangle(5, 5, 15, 8, 6, 0, 1))
	bottom := svgOrderTestUnit(10, svgVisibilityRectangle(5, 85, 15, 88, 86, 0, 1))
	for _, input := range [][]*svgPaintUnit{{title, page, bottom}, {bottom, page, title}, {page, bottom, title}} {
		stats, err := svgOrderPaintUnitsWithLimits(context.Background(), input, svgDefaultVisibilityLimits)
		if err != nil {
			t.Fatal(err)
		}
		if input[0] != page || stats.cyclicGroups != 0 || stats.relations != 2 {
			t.Fatalf("page overwrites a label: %+v / %+v", input, stats)
		}
	}
}

func TestSVGPaintOrderIndexesFragmentedSurfacesWithinBudget(t *testing.T) {
	far, near := &svgPaintUnit{first: 0, opacity: 1}, &svgPaintUnit{first: 1, opacity: 1}
	for i := 0; i < 1200; i++ {
		x, y := float64(i%60)*4, float64(i/60)*4
		for depth, unit := range []*svgPaintUnit{far, near} {
			p := svgVisibilityRectangle(x, y, x+2, y+2, float64(depth), 0, 0)
			unit.batches = append(unit.batches, &svgPaintBatch{polygons: [][]svgPoint{p}})
		}
	}
	// Checking all fragment pairs needs more than 1.4 million visits. The
	// spatial index must preserve the same depth relation with bounded work.
	limits := svgDefaultVisibilityLimits
	limits.work = 400000
	units := []*svgPaintUnit{near, far}
	stats, err := svgOrderPaintUnitsWithLimits(context.Background(), units, limits)
	if err != nil {
		t.Fatal(err)
	}
	if units[0] != far || stats.relations != 1 || stats.cyclicGroups != 0 {
		t.Fatalf("fragment indexing changed depth order: %+v", stats)
	}
}

func TestSVGPaintOrderUsesPolygonOverlapAndCoplanarSourceOrder(t *testing.T) {
	a := svgOrderTestUnit(2, []svgPoint{{0, 0, 10}, {2, 0, 10}, {0, 2, 10}})
	b := svgOrderTestUnit(3, []svgPoint{{2, 2, 0}, {1, 2, 0}, {2, 1, 0}})
	units := []*svgPaintUnit{a, b}
	stats, err := svgOrderPaintUnitsWithLimits(context.Background(), units, svgDefaultVisibilityLimits)
	if err != nil {
		t.Fatal(err)
	}
	if stats.relations != 0 {
		t.Fatal("overlapping boxes with disjoint triangles acquired a depth constraint")
	}
	front := svgOrderTestUnit(20, svgVisibilityRectangle(0, 0, 10, 10, 0, 0, 0))
	back := svgOrderTestUnit(10, svgVisibilityRectangle(2, 2, 8, 8, 0, 0, 0))
	units = []*svgPaintUnit{front, back}
	if err := svgOrderPaintUnits(context.Background(), units); err != nil {
		t.Fatal(err)
	}
	if units[0] != back {
		t.Fatal("coplanar paint lost source order")
	}
	biased := svgOrderTestUnit(0, svgVisibilityRectangle(2, 2, 8, 8, .00001, 0, 0))
	units = []*svgPaintUnit{biased, front}
	if err := svgOrderPaintUnits(context.Background(), units); err != nil {
		t.Fatal(err)
	}
	if units[1] != biased {
		t.Fatal("source order overrode real depth bias")
	}
}

func TestSVGPaintOrderPreservesAtomicGroupsAndDecalOrder(t *testing.T) {
	substrate := svgVisibilityRectangle(1, 1, 9, 9, 1, 0, .5)
	decal := svgVisibilityRectangle(3, 2, 6, 3, 1.6, 0, .5)
	object := svgOrderTestUnit(5, substrate, decal)
	object.opacity = .35
	page := svgOrderTestUnit(0, svgVisibilityRectangle(0, 0, 100, 100, 0, 0, .5))
	batches := append([]*svgPaintBatch(nil), object.batches...)
	units := []*svgPaintUnit{object, page}
	stats, err := svgOrderPaintUnitsWithLimits(context.Background(), units, svgDefaultVisibilityLimits)
	if err != nil {
		t.Fatal(err)
	}
	if units[1] != object || object.opacity != .35 || !reflect.DeepEqual(batches, object.batches) || stats.cyclicGroups != 0 {
		t.Fatal("ordering split a source opacity group or reordered its decals")
	}
}

func TestSVGPaintOrderCyclesRetainExternalDepthConstraints(t *testing.T) {
	rect := func(x, z float64) []svgPoint { return svgVisibilityRectangle(x, 0, x+5, 5, z, 0, 0) }
	a := svgOrderTestUnit(1, rect(0, 0), rect(20, 1))
	b := svgOrderTestUnit(2, rect(0, 1), rect(10, 0))
	c := svgOrderTestUnit(3, rect(10, 1), rect(20, 0))
	background := svgOrderTestUnit(100, svgVisibilityRectangle(-1, -1, 30, 8, -1, 0, 0))
	foreground := svgOrderTestUnit(0, svgVisibilityRectangle(-1, -1, 30, 8, 2, 0, 0))
	for _, input := range [][]*svgPaintUnit{{a, b, c, foreground, background}, {background, c, b, a, foreground}} {
		stats, err := svgOrderPaintUnitsWithLimits(context.Background(), input, svgDefaultVisibilityLimits)
		if err != nil {
			t.Fatal(err)
		}
		if stats.cyclicGroups != 1 || stats.conflictingPairs != 0 {
			t.Fatalf("three-way cycle was not retained as one component: %+v", stats)
		}
		if !reflect.DeepEqual(input, []*svgPaintUnit{background, a, b, c, foreground}) {
			t.Fatal("cycle fallback violated external depth or stable native source order")
		}
	}
}

func TestSVGPaintOrderCancellationAndWorkBudget(t *testing.T) {
	units := []*svgPaintUnit{svgOrderTestUnit(0, svgVisibilityRectangle(0, 0, 1, 1, 0, 0, 0))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svgOrderPaintUnits(ctx, units); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	limits := svgDefaultVisibilityLimits
	limits.work = 1
	if _, err := svgOrderPaintUnitsWithLimits(context.Background(), units, limits); err == nil {
		t.Fatal("paint ordering ignored work limit")
	}
}
