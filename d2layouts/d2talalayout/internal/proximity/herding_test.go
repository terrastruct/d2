package proximity_test

import (
	"context"
	"errors"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/internal/testlog"
	"github.com/d2lang/d2/lib/geo"

	"github.com/d2lang/d2/lib/log"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/grouping"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/invariant"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/placement"
	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/proximity"
)

func withTestLogger(ctx context.Context, tb testlog.TB) context.Context {
	tb.Helper()
	return log.With(ctx, testlog.New(tb))
}

// All x's are cousins of container
// e is connected to y, which is an uncle (no children), and should not be counted
// .
// .       x ◄──┐
// .      ▲     │     x
// .      │     │     ▲
// .      │     │     │
// .      │     │     │
// . ┌────┼─────┼─────┼──────────┐
// . │    │     │     │          │
// . │  ┌─┴┐  ┌─┴─┐   │ ┌────┐   │
// . │  │a │  │b  ├───┘ │ c  ├───┼────────┐
// . │  └──┘  └───┘     └────┘   │        ▼
// . │                           │        x
// . │     ┌────┐     ┌────┐     │        ▲
// . │     │ e  │     │    │     │        │
// . │     │    │     │ d  ├─────┼────────┘
// . └─────┴──┬─┴─────┴────┴─────┘
// .          │
// .          │
// .          └─► y
// .
// .
func TestGroupSheep(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	ctx = log.Leveled(ctx, slog.LevelDebug)
	t.Parallel()

	g := layoutgraph.NewGraph()

	container := layoutgraph.NewNode(0, 1000, 1000)

	a := layoutgraph.NewNode(1, 5, 5)
	b := layoutgraph.NewNode(2, 5, 5)
	c := layoutgraph.NewNode(3, 5, 5)
	d := layoutgraph.NewNode(4, 5, 5)
	e := layoutgraph.NewNode(5, 5, 5)

	a.TopLeft = geo.NewPoint(10, 10)
	b.TopLeft = geo.NewPoint(20, 10)
	c.TopLeft = geo.NewPoint(30, 10)
	d.TopLeft = geo.NewPoint(30, 20)
	e.TopLeft = geo.NewPoint(20, 20)

	g.AddNode(container)
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)
	g.AddNode(d)
	g.AddNode(e)

	ab_cousin := layoutgraph.NewNode(6, 5, 5)
	ab_uncle := layoutgraph.NewNode(6, 10, 10)
	ab_cousin.Container = ab_uncle

	b_cousin := layoutgraph.NewNode(6, 5, 5)
	b_uncle := layoutgraph.NewNode(6, 10, 10)
	b_cousin.Container = b_uncle

	cd_cousin := layoutgraph.NewNode(6, 5, 5)
	cd_uncle := layoutgraph.NewNode(6, 10, 10)
	cd_cousin.Container = cd_uncle

	// Uncle with no children have no grouping
	e_uncle := layoutgraph.NewNode(6, 5, 5)

	edgeAbductions := []*layoutgraph.EdgeAbduction{
		{
			OriginallyFrom: a,
			OriginallyTo:   ab_cousin,
			CurrentTo:      ab_uncle,
		},
		{
			OriginallyFrom: b,
			OriginallyTo:   ab_cousin,
			CurrentTo:      ab_uncle,
		},
		{
			OriginallyFrom: b,
			OriginallyTo:   b_cousin,
			CurrentTo:      b_uncle,
		},
		{
			OriginallyFrom: c,
			OriginallyTo:   cd_cousin,
			CurrentTo:      cd_uncle,
		},
		{
			OriginallyFrom: d,
			OriginallyTo:   cd_cousin,
			CurrentTo:      cd_uncle,
		},
		{
			OriginallyFrom: e,
			OriginallyTo:   e_uncle,
			CurrentTo:      e_uncle,
		},
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		container: {a, b, c, d, e},
		ab_uncle:  {ab_cousin},
		b_uncle:   {b_cousin},
		cd_uncle:  {cd_cousin},
	}
	for container := range g.Containers {
		if container != nil {
			container.SetContainer(true)
		}
	}

	got, _, err := proximity.GroupSheep(ctx, g, container, edgeAbductions)
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, got[ab_uncle], a, b)
	assert.Contains(t, got[b_uncle], b)
	assert.Contains(t, got[cd_uncle], c, d)

	assert.Len(t, got[ab_uncle], 2)
	assert.Len(t, got[b_uncle], 1)
	assert.Len(t, got[cd_uncle], 2)
	assert.Len(t, got[e_uncle], 0)
}

func TestGroupNestedSheep(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	ctx = log.Leveled(ctx, slog.LevelDebug)
	t.Parallel()

	g := layoutgraph.NewGraph()

	container := layoutgraph.NewNode(0, 1000, 1000)

	child := layoutgraph.NewNode(1, 100, 100)
	grandChild := layoutgraph.NewNode(2, 5, 5)

	child.TopLeft = geo.NewPoint(10, 10)
	grandChild.TopLeft = geo.NewPoint(20, 10)
	grandChild.Container = child
	child.Container = container

	g.AddNode(container)
	g.AddNode(child)
	g.AddNode(grandChild)

	cousin := layoutgraph.NewNode(6, 5, 5)
	uncle := layoutgraph.NewNode(6, 10, 10)
	cousin.Container = uncle

	edgeAbductions := []*layoutgraph.EdgeAbduction{
		{
			OriginallyFrom: grandChild,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		container: {child},
		child:     {grandChild},
		uncle:     {cousin},
	}
	for container := range g.Containers {
		if container != nil {
			container.SetContainer(true)
		}
	}

	got, _, err := proximity.GroupSheep(ctx, g, container, edgeAbductions)
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, got[uncle], child)
	assert.Len(t, got[uncle], 1)
}

func TestGroupClusterSheep(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	ctx = log.Leveled(ctx, slog.LevelDebug)
	t.Parallel()

	g := layoutgraph.NewGraph()

	container := layoutgraph.NewNode(0, 1000, 1000)

	normalChild := layoutgraph.NewNode(1, 5, 5)
	clusterNodeA := layoutgraph.NewNode(1, 5, 5)
	clusterNodeB := layoutgraph.NewNode(1, 5, 5)
	vessel := layoutgraph.NewNode(1, 5, 5)

	grouping.AddCluster(g, &layoutgraph.Cluster{
		Vessel: vessel,
		Nodes:  []*layoutgraph.Node{clusterNodeA, clusterNodeB},
	})

	normalChild.TopLeft = geo.NewPoint(10, 10)
	vessel.TopLeft = geo.NewPoint(20, 10)

	normalChild.Container = container
	vessel.Container = container

	g.AddNode(container)
	g.AddNode(normalChild)
	g.AddNode(vessel)

	cousin := layoutgraph.NewNode(6, 5, 5)
	uncle := layoutgraph.NewNode(6, 10, 10)
	cousin.Container = uncle

	edgeAbductions := []*layoutgraph.EdgeAbduction{
		{
			OriginallyFrom: clusterNodeA,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
		{
			OriginallyFrom: normalChild,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		container: {normalChild, vessel},
		uncle:     {cousin},
	}
	for container := range g.Containers {
		if container != nil {
			container.SetContainer(true)
		}
	}

	got, _, err := proximity.GroupSheep(ctx, g, container, edgeAbductions)
	if err != nil {
		t.Fatal(err)
	}

	assert.Contains(t, got[uncle], vessel, normalChild)
	assert.Len(t, got[uncle], 2)
}

// Two sheep (x, y) are connected to cousins which are placed
// Those two sheep are also connected to cousins, along with other sheep, that are not placed
// Those other sheep get the same placement as the two
func TestAssignHerdVirality(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	ctx = log.Leveled(ctx, slog.LevelDebug)
	t.Parallel()

	g := layoutgraph.NewGraph()

	container := layoutgraph.NewNode(0, 1000, 1000)

	// These sheep have initial assignments
	x := layoutgraph.NewNode(1, 5, 5)
	y := layoutgraph.NewNode(1, 5, 5)

	// These sheep inherit the above sheep's assignments
	a := layoutgraph.NewNode(1, 5, 5)
	b := layoutgraph.NewNode(1, 5, 5)
	c := layoutgraph.NewNode(1, 5, 5)

	x.Container = container
	y.Container = container
	a.Container = container
	b.Container = container
	c.Container = container

	g.AddNode(container)
	g.AddNode(x)
	g.AddNode(y)
	g.AddNode(a)
	g.AddNode(b)
	g.AddNode(c)

	// Placed
	placedCousin := layoutgraph.NewNode(6, 5, 5)
	placedCousin.TopLeft = geo.NewPoint(8, 8)
	placedUncle := layoutgraph.NewNode(6, 10, 10)
	placedCousin.Container = placedUncle

	placedCousin.HerdAssignment = layoutgraph.NewHerdAssignment()
	placedCousin.HerdAssignment.Orientation = geo.Right

	// Not placed
	cousin := layoutgraph.NewNode(6, 5, 5)
	uncle := layoutgraph.NewNode(6, 10, 10)
	cousin.Container = uncle

	edgeAbductions := []*layoutgraph.EdgeAbduction{
		{
			OriginallyFrom: x,
			OriginallyTo:   placedCousin,
			CurrentTo:      placedUncle,
		},
		{
			OriginallyFrom: y,
			OriginallyTo:   placedCousin,
			CurrentTo:      placedUncle,
		},
		{
			OriginallyFrom: x,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
		{
			OriginallyFrom: y,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
		{
			OriginallyFrom: a,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
		{
			OriginallyFrom: b,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
		{
			OriginallyFrom: c,
			OriginallyTo:   cousin,
			CurrentTo:      uncle,
		},
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		container:   {x, y, a, b, c},
		placedUncle: {placedCousin},
		uncle:       {cousin},
	}
	for container := range g.Containers {
		if container != nil {
			container.SetContainer(true)
		}
	}

	if err := proximity.AssignHerds(ctx, g, container, edgeAbductions); err != nil {
		t.Fatal(err)
	}

	assert.ElementsMatch(t,
		[]geo.Orientation{
			x.HerdAssignment.Orientation,
			y.HerdAssignment.Orientation,
			a.HerdAssignment.Orientation,
			b.HerdAssignment.Orientation,
			c.HerdAssignment.Orientation,
		},
		[]geo.Orientation{
			x.HerdAssignment.Orientation,
			x.HerdAssignment.Orientation,
			x.HerdAssignment.Orientation,
			x.HerdAssignment.Orientation,
			x.HerdAssignment.Orientation,
		},
	)
}

// Tests pairing on different side when a cousin which is already paired with another container on one side
func TestUseBothSides(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	ctx = log.Leveled(ctx, slog.LevelDebug)
	t.Parallel()

	g := layoutgraph.NewGraph()

	container := layoutgraph.NewNode(0, 1000, 1000)

	x := layoutgraph.NewNode(1, 5, 5)
	y := layoutgraph.NewNode(1, 5, 5)

	x.Container = container
	y.Container = container

	g.AddNode(container)
	g.AddNode(x)
	g.AddNode(y)

	placedCousin := layoutgraph.NewNode(6, 5, 5)
	placedCousin.TopLeft = geo.NewPoint(8, 8)
	// Tall, so both sides usable
	placedUncle := layoutgraph.NewNode(6, 10, 30)
	placedCousin.Container = placedUncle

	assert.True(t, proximity.CanUseBothSides(placedUncle, geo.Right))

	placedCousin.HerdAssignment = layoutgraph.NewHerdAssignment()
	placedCousin.HerdAssignment.Orientation = geo.Right
	placedCousin.HerdAssignment.PairOppositeSide(layoutgraph.NewNode(1, 1, 1))

	edgeAbductions := []*layoutgraph.EdgeAbduction{
		{
			OriginallyFrom: x,
			OriginallyTo:   placedCousin,
			CurrentTo:      placedUncle,
		},
		{
			OriginallyFrom: y,
			OriginallyTo:   placedCousin,
			CurrentTo:      placedUncle,
		},
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		container:   {x, y},
		placedUncle: {placedCousin},
	}
	for container := range g.Containers {
		if container != nil {
			container.SetContainer(true)
		}
	}

	if err := proximity.AssignHerds(ctx, g, container, edgeAbductions); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, placedCousin.HerdAssignment.SameSidePairCount())

	// Reset with no pairings, we should get opposite side paired
	placedCousin.HerdAssignment = layoutgraph.NewHerdAssignment()
	placedCousin.HerdAssignment.Orientation = geo.Right

	if err := proximity.AssignHerds(ctx, g, container, edgeAbductions); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, placedCousin.HerdAssignment.OppositeSidePairCount())
	assert.Equal(t, 0, placedCousin.HerdAssignment.SameSidePairCount())
}

// Three pairs of 2 sheep are all connected to different unplaced uncles
// They are also all connected to a common unplaced uncle
// Despite their random assignments, they should all get the same side
func TestRandomHerdVirality(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	ctx = log.Leveled(ctx, slog.LevelDebug)
	t.Parallel()

	g := layoutgraph.NewGraph()

	container := layoutgraph.NewNode(0, 1000, 1000)

	a_1 := layoutgraph.NewNode(1, 5, 5)
	a_2 := layoutgraph.NewNode(1, 5, 5)

	b_1 := layoutgraph.NewNode(1, 5, 5)
	b_2 := layoutgraph.NewNode(1, 5, 5)

	c_1 := layoutgraph.NewNode(1, 5, 5)
	c_2 := layoutgraph.NewNode(1, 5, 5)

	a_1.Container = container
	a_2.Container = container

	b_1.Container = container
	b_2.Container = container

	c_1.Container = container
	c_2.Container = container

	g.AddNode(container)
	g.AddNode(a_1)
	g.AddNode(a_2)
	g.AddNode(b_1)
	g.AddNode(b_2)
	g.AddNode(c_1)
	g.AddNode(c_2)

	aCousin := layoutgraph.NewNode(6, 5, 5)
	aUncle := layoutgraph.NewNode(6, 10, 10)
	aCousin.Container = aUncle

	bCousin := layoutgraph.NewNode(6, 5, 5)
	bUncle := layoutgraph.NewNode(6, 10, 10)
	bCousin.Container = bUncle

	cCousin := layoutgraph.NewNode(6, 5, 5)
	cUncle := layoutgraph.NewNode(6, 10, 10)
	cCousin.Container = cUncle

	commonCousin := layoutgraph.NewNode(6, 5, 5)
	commonUncle := layoutgraph.NewNode(6, 10, 10)
	commonCousin.Container = commonUncle

	edgeAbductions := []*layoutgraph.EdgeAbduction{
		{
			OriginallyFrom: a_1,
			OriginallyTo:   aCousin,
			CurrentTo:      aUncle,
		},
		{
			OriginallyFrom: a_2,
			OriginallyTo:   aCousin,
			CurrentTo:      aUncle,
		},
		{
			OriginallyFrom: b_1,
			OriginallyTo:   bCousin,
			CurrentTo:      bUncle,
		},
		{
			OriginallyFrom: b_2,
			OriginallyTo:   bCousin,
			CurrentTo:      bUncle,
		},
		{
			OriginallyFrom: c_1,
			OriginallyTo:   cCousin,
			CurrentTo:      cUncle,
		},
		{
			OriginallyFrom: c_2,
			OriginallyTo:   cCousin,
			CurrentTo:      cUncle,
		},
		{
			OriginallyFrom: a_1,
			OriginallyTo:   commonCousin,
			CurrentTo:      commonUncle,
		},
		{
			OriginallyFrom: a_2,
			OriginallyTo:   commonCousin,
			CurrentTo:      commonUncle,
		},
		{
			OriginallyFrom: b_1,
			OriginallyTo:   commonCousin,
			CurrentTo:      commonUncle,
		},
		{
			OriginallyFrom: b_2,
			OriginallyTo:   commonCousin,
			CurrentTo:      commonUncle,
		},
		{
			OriginallyFrom: c_1,
			OriginallyTo:   commonCousin,
			CurrentTo:      commonUncle,
		},
		{
			OriginallyFrom: c_2,
			OriginallyTo:   commonCousin,
			CurrentTo:      commonUncle,
		},
	}

	g.Containers = map[*layoutgraph.Node][]*layoutgraph.Node{
		container:   {a_1, a_2, b_1, b_2, c_1, c_2},
		aUncle:      {aCousin},
		bUncle:      {bCousin},
		cUncle:      {cCousin},
		commonUncle: {commonCousin},
	}
	for container := range g.Containers {
		if container != nil {
			container.SetContainer(true)
		}
	}

	if err := proximity.AssignHerds(ctx, g, container, edgeAbductions); err != nil {
		t.Fatal(err)
	}

	assert.ElementsMatch(t,
		[]geo.Orientation{
			a_1.HerdAssignment.Orientation,
			a_2.HerdAssignment.Orientation,
			b_1.HerdAssignment.Orientation,
			b_2.HerdAssignment.Orientation,
			c_1.HerdAssignment.Orientation,
			c_2.HerdAssignment.Orientation,
		},
		[]geo.Orientation{
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
		},
	)

	for _, n := range g.Nodes {
		n.HerdAssignment = nil
	}

	// Reorder
	aUncle.ID = -2
	bUncle.ID = -3
	commonUncle.ID = -1

	if err := proximity.AssignHerds(ctx, g, container, edgeAbductions); err != nil {
		t.Fatal(err)
	}

	assert.ElementsMatch(t,
		[]geo.Orientation{
			a_1.HerdAssignment.Orientation,
			a_2.HerdAssignment.Orientation,
			b_1.HerdAssignment.Orientation,
			b_2.HerdAssignment.Orientation,
			c_1.HerdAssignment.Orientation,
			c_2.HerdAssignment.Orientation,
		},
		[]geo.Orientation{
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
			a_1.HerdAssignment.Orientation,
		},
	)
}

func TestPlaceChildrenOrder(t *testing.T) {
	t.Parallel()

	a := layoutgraph.NewNode(1, 10, 10)
	b := layoutgraph.NewNode(2, 10, 10)
	c := layoutgraph.NewNode(3, 10, 10)
	d := layoutgraph.NewNode(4, 10, 10)
	external := layoutgraph.NewNode(5, 10, 10)

	got, err := placement.PlaceChildrenOrder(context.Background(), []*layoutgraph.Node{a, b, c, d}, []*layoutgraph.EdgeAbduction{
		{CurrentFrom: b, CurrentTo: c},
		{CurrentFrom: c, CurrentTo: d},
		{CurrentFrom: d, CurrentTo: external},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, []*layoutgraph.Node{a, b, c, d}, got)
}

func TestPlaceChildrenOrderRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	node := layoutgraph.NewNode(1, 10, 10)
	tests := []struct {
		name           string
		nodes          []*layoutgraph.Node
		edgeAbductions []*layoutgraph.EdgeAbduction
	}{
		{name: "nil child", nodes: []*layoutgraph.Node{nil}},
		{name: "duplicate child", nodes: []*layoutgraph.Node{node, node}},
		{name: "nil edge abduction", nodes: []*layoutgraph.Node{node}, edgeAbductions: []*layoutgraph.EdgeAbduction{nil}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := placement.PlaceChildrenOrder(context.Background(), test.nodes, test.edgeAbductions)
			if !errors.Is(err, invariant.ErrViolation) {
				t.Fatalf("placement.PlaceChildrenOrder() error = %v; want invariant.ErrViolation", err)
			}
		})
	}
}

func TestPlaceChildrenOrderPreservesCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := placement.PlaceChildrenOrder(ctx, []*layoutgraph.Node{layoutgraph.NewNode(1, 10, 10)}, nil)
	if got != nil {
		t.Fatalf("placement.PlaceChildrenOrder() = %v; want nil after cancellation", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("placement.PlaceChildrenOrder() error = %v; want context.Canceled", err)
	}

	ctx = &cancelAfterErrChecks{Context: context.Background(), remaining: 2}
	_, err = placement.PlaceChildrenOrder(ctx, []*layoutgraph.Node{
		layoutgraph.NewNode(1, 10, 10),
		layoutgraph.NewNode(2, 10, 10),
		layoutgraph.NewNode(3, 10, 10),
	}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("placement.PlaceChildrenOrder() mid-scan error = %v; want context.Canceled", err)
	}
}

type cancelAfterErrChecks struct {
	context.Context
	remaining int
}

func (ctx *cancelAfterErrChecks) Err() error {
	if ctx.remaining == 0 {
		return context.Canceled
	}
	ctx.remaining--
	return nil
}

func TestApplyVirallyRejectsConflictingOrientations(t *testing.T) {
	t.Parallel()

	uncle := layoutgraph.NewNode(1, 10, 10)
	left := layoutgraph.NewNode(2, 10, 10)
	right := layoutgraph.NewNode(3, 10, 10)
	left.HerdAssignment = layoutgraph.NewHerdAssignment()
	left.HerdAssignment.Orientation = geo.Left
	right.HerdAssignment = layoutgraph.NewHerdAssignment()
	right.HerdAssignment.Orientation = geo.Right

	err := proximity.ApplyVirally(context.Background(), []*layoutgraph.Node{uncle}, map[*layoutgraph.Node][]*layoutgraph.Node{
		uncle: {left, right},
	})
	if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("proximity.ApplyVirally() error = %v; want invariant.ErrViolation", err)
	}
}

func TestAssignHerdsRejectsUnindexedUncleChildren(t *testing.T) {
	t.Parallel()

	g := layoutgraph.NewGraph()
	root := layoutgraph.NewNode(1, 100, 100)
	a := layoutgraph.NewNode(2, 10, 10)
	b := layoutgraph.NewNode(3, 10, 10)
	uncle := layoutgraph.NewNode(4, 100, 100)
	uncle.SetContainer(true)
	cousinA := layoutgraph.NewNode(5, 10, 10)
	cousinB := layoutgraph.NewNode(6, 10, 10)
	cousinA.Container = uncle
	cousinB.Container = uncle
	g.Containers[root] = []*layoutgraph.Node{a, b}

	err := proximity.AssignHerds(context.Background(), g, root, []*layoutgraph.EdgeAbduction{
		{OriginallyFrom: a, OriginallyTo: cousinA, CurrentTo: uncle},
		{OriginallyFrom: b, OriginallyTo: cousinB, CurrentTo: uncle},
	})
	if !errors.Is(err, invariant.ErrViolation) {
		t.Fatalf("assignHerds() error = %v; want invariant.ErrViolation", err)
	}
}

func TestAssignHerdsReconcilesOverlappingGroups(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name            string
		firstWide       bool
		lastWide        bool
		lastOrientation geo.Orientation
		want            geo.Orientation
	}{
		{name: "opposing preferences with a shared side", firstWide: true, lastWide: true, lastOrientation: geo.Bottom, want: geo.Top},
		{name: "later constraint selects the shared side", firstWide: true, lastOrientation: geo.Bottom, want: geo.Top},
		{name: "later constraint overrides the first preference", firstWide: true, lastOrientation: geo.Top, want: geo.Bottom},
		{name: "incompatible sides leave the connected herd free", lastOrientation: geo.Bottom, want: geo.NONE},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := layoutgraph.NewGraph()
			root := layoutgraph.NewNode(0, 100, 100)
			a := layoutgraph.NewNode(1, 10, 10)
			b := layoutgraph.NewNode(2, 10, 10)
			c := layoutgraph.NewNode(3, 10, 10)
			root.SetContainer(true)
			g.Containers[root] = []*layoutgraph.Node{a, b, c}
			for _, node := range g.Containers[root] {
				node.Container = root
			}
			first := layoutgraph.NewNode(4, 100, 100)
			last := layoutgraph.NewNode(5, 100, 100)
			if test.firstWide {
				first.Width = 300
			}
			if test.lastWide {
				last.Width = 300
			}
			first.SetContainer(true)
			last.SetContainer(true)
			firstCousin := layoutgraph.NewNode(6, 10, 10)
			lastCousin := layoutgraph.NewNode(7, 10, 10)
			firstCousin.Container = first
			lastCousin.Container = last
			g.Containers[first] = []*layoutgraph.Node{firstCousin}
			g.Containers[last] = []*layoutgraph.Node{lastCousin}
			for _, cousin := range []*layoutgraph.Node{firstCousin, lastCousin} {
				cousin.TopLeft = geo.NewPoint(0, 0)
				cousin.HerdAssignment = layoutgraph.NewHerdAssignment()
				// Both flexible uncles prefer the same side as their cousin.
				cousin.HerdAssignment.PairOppositeSide(layoutgraph.NewNode(8, 10, 10))
			}
			firstCousin.HerdAssignment.Orientation = geo.Top
			lastCousin.HerdAssignment.Orientation = test.lastOrientation

			// The shared child joins {a, b} and {b, c}. Previously b's side
			// could change while handling last, leaving it different from a.
			err := proximity.AssignHerds(context.Background(), g, root, []*layoutgraph.EdgeAbduction{
				{OriginallyFrom: a, OriginallyTo: firstCousin, CurrentTo: first},
				{OriginallyFrom: b, OriginallyTo: firstCousin, CurrentTo: first},
				{OriginallyFrom: b, OriginallyTo: lastCousin, CurrentTo: last},
				{OriginallyFrom: c, OriginallyTo: lastCousin, CurrentTo: last},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range g.Containers[root] {
				if test.want == geo.NONE {
					assert.Nil(t, node.HerdAssignment)
				} else if assert.NotNil(t, node.HerdAssignment) {
					assert.Equal(t, test.want, node.HerdAssignment.Orientation)
				}
			}
			// Provisional choices must not count as pairs when the complete
			// herd cannot use them, or bias unrelated herds placed later.
			wantFirstSame, wantLastSame := 0, 0
			if test.want == geo.Top {
				wantFirstSame = 1
			}
			if test.want == test.lastOrientation {
				wantLastSame = 1
			}
			assert.Equal(t, wantFirstSame, firstCousin.HerdAssignment.SameSidePairCount())
			assert.Equal(t, wantLastSame, lastCousin.HerdAssignment.SameSidePairCount())
		})
	}
}
