package routing

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
	"github.com/d2lang/d2/lib/geo"
)

// evenlyDistribute returns a list of positions of length num, where the values are evenly distributed between floor and ceiling
func evenlyDistribute(floor, ceil float64, num int) []float64 {
	out := make([]float64, 0)
	if num <= 0 || floor >= ceil {
		return out
	}
	increment := math.Floor((ceil - floor) / float64(num+1))
	if increment <= 0 {
		return out
	}
	out = make([]float64, 0, num)
	for i := 1; i <= num; i++ {
		out = append(out, floor+float64(i)*increment)
	}
	return out
}

func TestEvenlyDistribute(t *testing.T) {
	type testCase struct {
		floor float64
		ceil  float64
		num   int
	}
	cases := map[testCase][]float64{
		{0, 10, 1}:  {5},
		{0, 10, 2}:  {3, 6},
		{0, 5, 3}:   {1, 2, 3},
		{20, 40, 5}: {23, 26, 29, 32, 35},
		{0, 10, 0}:  {},
		{0, 10, -1}: {},
		// shouldn't infinite loop
		{-235, -234, 1}: {},
	}
	for tc, expectedVals := range cases {
		assert.Equal(t, expectedVals, evenlyDistribute(tc.floor, tc.ceil, tc.num))
	}
}

func TestFixClusterEdgeBranchingChoosesLowestCostPoint(t *testing.T) {
	g := layoutgraph.NewGraph()
	external := layoutgraph.NewNode(1, 10, 10)
	left := layoutgraph.NewNode(2, 10, 10)
	right := layoutgraph.NewNode(3, 10, 10)
	external.TopLeft = geo.NewPoint(10_000, 10_000)
	left.TopLeft = geo.NewPoint(11_000, 11_000)
	right.TopLeft = geo.NewPoint(12_000, 12_000)
	for _, n := range []*layoutgraph.Node{external, left, right} {
		g.AddNewNodeToContainer(nil, n)
	}

	cluster := &layoutgraph.Cluster{
		Vessel:             layoutgraph.NewNode(4, 10, 10),
		Nodes:              []*layoutgraph.Node{left, right},
		Arrangement:        layoutgraph.Row,
		DesiredArrangement: layoutgraph.Row,
		Graph:              g,
	}
	left.Cluster = cluster
	right.Cluster = cluster

	leftEdge := g.Connect(external, left)
	leftEdge.Points = []*geo.Point{
		geo.NewPoint(0, 0),
		geo.NewPoint(0, -10),
		geo.NewPoint(-100, -10),
		geo.NewPoint(-100, 100),
	}
	rightEdge := g.Connect(external, right)
	rightEdge.Points = []*geo.Point{
		geo.NewPoint(0, 0),
		geo.NewPoint(0, 10),
		geo.NewPoint(100, 10),
		geo.NewPoint(100, -5),
	}

	if err := FixClusterEdgeBranching(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if leftEdge.Points[1].Y != 10 || rightEdge.Points[1].Y != 10 {
		t.Fatalf("branch point y coordinates = (%v, %v), want (10, 10)", leftEdge.Points[1].Y, rightEdge.Points[1].Y)
	}
}

func TestBounds(t *testing.T) {
	type testCase struct {
		line       *geo.Segment
		otherLines []*geo.Segment
		buffer     float64

		expectedFloor float64
		expectedCeil  float64
	}
	cases := []testCase{
		// Test basic horizontal direction
		{
			line: &geo.Segment{Start: geo.NewPoint(10, 0), End: geo.NewPoint(10, 10)},
			otherLines: []*geo.Segment{
				{Start: geo.NewPoint(0, 0), End: geo.NewPoint(0, 10)},
				{Start: geo.NewPoint(20, 0), End: geo.NewPoint(20, 10)},
			},
			buffer: 2,

			expectedFloor: 0,
			expectedCeil:  20,
		},
		// Test basic vertical direction
		{
			line: &geo.Segment{Start: geo.NewPoint(0, 10), End: geo.NewPoint(10, 10)},
			otherLines: []*geo.Segment{
				{Start: geo.NewPoint(0, 0), End: geo.NewPoint(10, 0)},
				{Start: geo.NewPoint(0, 20), End: geo.NewPoint(10, 20)},
			},
			buffer: 2,

			expectedFloor: 0,
			expectedCeil:  20,
		},
		// Test non-overlaps not considered
		{
			line: &geo.Segment{Start: geo.NewPoint(10, 0), End: geo.NewPoint(10, 10)},
			otherLines: []*geo.Segment{
				{Start: geo.NewPoint(0, 0), End: geo.NewPoint(0, 10)},
				{Start: geo.NewPoint(20, 0), End: geo.NewPoint(20, 10)},
				// Non overlaps
				{Start: geo.NewPoint(8, 100), End: geo.NewPoint(8, 110)},
				{Start: geo.NewPoint(18, -100), End: geo.NewPoint(18, -110)},
				{Start: geo.NewPoint(15, -2), End: geo.NewPoint(15, -12)},
			},
			buffer: 2,

			expectedFloor: 0,
			expectedCeil:  20,
		},
		// Test no lower bound
		{
			line: &geo.Segment{Start: geo.NewPoint(10, 0), End: geo.NewPoint(10, 10)},
			otherLines: []*geo.Segment{
				{Start: geo.NewPoint(21, 0), End: geo.NewPoint(21, 10)},
				{Start: geo.NewPoint(20, 0), End: geo.NewPoint(20, 10)},
			},
			buffer: 2,

			expectedFloor: math.Inf(-1),
			expectedCeil:  20,
		},
		// Test point (start = end)
		{
			line: &geo.Segment{Start: geo.NewPoint(10, 0), End: geo.NewPoint(10, 0)},
			otherLines: []*geo.Segment{
				{Start: geo.NewPoint(21, 0), End: geo.NewPoint(21, 10)},
				{Start: geo.NewPoint(20, 0), End: geo.NewPoint(20, 10)},
			},
			buffer: 2,

			expectedFloor: math.Inf(-1),
			expectedCeil:  math.Inf(0),
		},
	}
	for _, tc := range cases {
		actualFloor, actualCeil := tc.line.GetBounds(tc.otherLines, tc.buffer)
		assert.Equal(t, tc.expectedFloor, actualFloor)
		assert.Equal(t, tc.expectedCeil, actualCeil)
	}
}

// . ┌─────────────┐
// . │             │
// . │             │
// . │             │
// . └─┬───────────┘
// .   │
// .   │
// .   │
// .   │
// .   │
// .   ▼
// . ┌─────────────┐
// . │             │
// . │             │
// . │             │
// . └─────────────┘
func TestBalanceEdgeSegmentsSimpleSingle(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	e1 := g.Connect(a, b)
	e1.Points = []*geo.Point{
		{X: 1, Y: 1000},
		{X: 1, Y: 2000},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, e1.Points[0].X, 500.0)
	assert.Equal(t, e1.Points[1].X, 500.0)
}

// . ┌─────────────┐
// . │             │
// . │             │
// . │             │
// . └─┬─┬─────────┘
// .   │ │
// .   │ │
// .   │ │
// .   │ │
// .   │ │
// .   ▼ ▼
// . ┌─────────────┐
// . │             │
// . │             │
// . │             │
// . └─────────────┘
func TestBalanceEdgeSegmentsSimpleDouble(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	e1 := g.Connect(a, b)
	e1.Points = []*geo.Point{
		{X: 1, Y: 1000},
		{X: 1, Y: 2000},
	}

	e2 := g.Connect(a, b)
	e2.Points = []*geo.Point{
		{X: 3, Y: 1000},
		{X: 3, Y: 2000},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, math.Floor(1000/3.0), e1.Points[0].X)
	assert.Equal(t, math.Floor(1000/3.0), e1.Points[1].X)
	assert.Equal(t, math.Floor(1000*2/3.0), e2.Points[0].X)
	assert.Equal(t, math.Floor(1000*2/3.0), e2.Points[1].X)
}

// . ┌─────────────┐
// . │             │
// . │             │
// . │             │
// . └─┬───────────┘
// .   │ ▲
// .   │ │
// .   │ │
// .   │ │
// .   │ │
// .   ▼ │
// . ┌───┴─────────┐
// . │             │
// . │             │
// . │             │
// . └─────────────┘
func TestBalanceEdgeSegmentsSimpleDoubleOppositeDirections(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	e1 := g.Connect(a, b)
	e1.Points = []*geo.Point{
		{X: 1, Y: 1000},
		{X: 1, Y: 2000},
	}

	e2 := g.Connect(b, a)
	e2.Points = []*geo.Point{
		{X: 3, Y: 2000},
		{X: 3, Y: 1000},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, math.Floor(1000/3.0), e1.Points[0].X)
	assert.Equal(t, math.Floor(1000/3.0), e1.Points[1].X)
	assert.Equal(t, math.Floor(1000*2/3.0), e2.Points[0].X)
	assert.Equal(t, math.Floor(1000*2/3.0), e2.Points[1].X)
}

// . ┌────────┐         ┌─────────┐
// . │        │         │         │
// . │        │         │         │
// . │        │         │         │
// . │        │         │         │
// . └┬───────┘         └┬────────┘
// .  │                  │
// .  │                  │
// .  │                  │
// .  │                  │
// .  ▼                  ▼
// . ┌─────────────────────────────┐
// . │                             │
// . │                             │
// . │                             │
// . │                             │
// . └─────────────────────────────┘
func TestBalanceEdgeSegmentsMultipleNodes(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 200, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 200, 1000)
	b.TopLeft = geo.NewPoint(500, 0)
	c := layoutgraph.NewNode(0, 1000, 1000)
	c.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)
	g.AddNodeUnchecked(c)

	e1 := g.Connect(a, c)
	e1.Points = []*geo.Point{
		{X: 1, Y: 1000},
		{X: 1, Y: 2000},
	}

	e2 := g.Connect(b, c)
	e2.Points = []*geo.Point{
		{X: 501, Y: 1000},
		{X: 501, Y: 2000},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 100.0, e1.Points[0].X)
	assert.Equal(t, 100.0, e1.Points[1].X)
	assert.Equal(t, 600.0, e2.Points[0].X)
	assert.Equal(t, 600.0, e2.Points[1].X)
}

// . ┌───────┐
// . │       │
// . │       │
// . │       │
// . └─┬─────┘
// .   │
// .   │
// .   │
// .   └──┐
// .      │
// .      ▼
// . ┌────────┐
// . │        │
// . │        │
// . │        │
// . └────────┘
func TestBalanceEdgeSegmentsSingleBendy(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	e := g.Connect(a, b)
	e.Points = []*geo.Point{
		{X: 1, Y: 1000},
		{X: 1, Y: 1300},
		{X: 300, Y: 1300},
		{X: 300, Y: 2000},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 500.0, e.Points[0].X)
	assert.Equal(t, 500.0, e.Points[1].X)
	assert.Equal(t, 500.0, e.Points[2].X)
}

// . ┌──────┐              ┌──────┐
// . │      ├──────┐┌──────┤      │
// . │  a   │      ││      │   c  │
// . └──────┘      ││      └──────┘
// .               ││
// .               ││
// .               ││
// .               ││
// . ┌──────┐      ││      ┌──────┐
// . │      │      ││      │      │
// . │  b   │◄─────┘└─────►│   d  │
// . └──────┘              └──────┘
func TestBalanceEdgeSegmentsBackToBack(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	e1 := g.Connect(a, b)
	e1.Points = []*geo.Point{
		{X: 1000, Y: 500},
		{X: 1450, Y: 500},
		{X: 1450, Y: 2400},
		{X: 1000, Y: 2400},
	}

	c := layoutgraph.NewNode(0, 1000, 1000)
	c.TopLeft = geo.NewPoint(2000, 0)
	d := layoutgraph.NewNode(0, 1000, 1000)
	d.TopLeft = geo.NewPoint(2000, 2000)
	g.AddNodeUnchecked(c)
	g.AddNodeUnchecked(d)

	e2 := g.Connect(c, d)
	e2.Points = []*geo.Point{
		{X: 2000, Y: 500},
		{X: 1500, Y: 500},
		{X: 1500, Y: 2500},
		{X: 2000, Y: 2500},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, *geo.NewPoint(1000, 500), *e1.Points[0])
	assert.Equal(t, *geo.NewPoint(1000+math.Floor(1000/3.0), 500), *e1.Points[1])
	assert.Equal(t, *geo.NewPoint(1000+math.Floor(1000/3.0), 2500), *e1.Points[2])
	assert.Equal(t, *geo.NewPoint(1000, 2500), *e1.Points[3])

	assert.Equal(t, *geo.NewPoint(2000, 500), *e2.Points[0])
	assert.Equal(t, *geo.NewPoint(1000+2*math.Floor(1000/3.0), 500), *e2.Points[1])
	assert.Equal(t, *geo.NewPoint(1000+2*math.Floor(1000/3.0), 2500), *e2.Points[2])
	assert.Equal(t, *geo.NewPoint(2000, 2500), *e2.Points[3])
}

// . ┌───────┐
// . │       │
// . │       │◄─────────────────┐
// . │  a    │                  │
// . │       │                  │
// . └───────┘                  │
// .                            │        ┌────────────────────────┐
// .                            │        │                        │
// .                            │        │                        │
// .                            │        │                        │
// .                            │        │                        │
// .  ┌──────┐                  │        │                        │
// .  │      │                  │        │                        │
// .  │ c    │◄─────────────────┼──┐     │                        │
// .  │      │                  │  │     │                        │
// .  └──────┘                  │  │     │                        │
// .                            │  │     │           f            │
// .                            │  │     │                        │
// .  ┌──────┐                  │  │     │                        │
// .  │      ├──────────────────┘  │     │                        │
// .  │ b    │                     │     │                        │
// .  └──────┘     ┌──────┐        │     │                        │
// .               │      │        │     │                        │
// .               │   e  │        │     │                        │
// .               │      │        │     │                        │
// .               └──────┘        │     │                        │
// .                               │     │                        │
// .                               │     │                        │
// .                               │     └────────────────────────┘
// .  ┌───────┐                    │
// .  │       │                    │
// .  │ d     ├────────────────────┘
// .  │       │
// .  └───────┘

func TestBalanceEdgeSegmentsMultipleRanges(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(0, 6000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	e1 := g.Connect(a, b)
	e1.Points = []*geo.Point{
		{X: 1000, Y: 500},
		{X: 1600, Y: 500},
		{X: 1600, Y: 6200},
		{X: 1000, Y: 6200},
	}

	c := layoutgraph.NewNode(0, 1000, 1000)
	c.TopLeft = geo.NewPoint(0, 3000)
	d := layoutgraph.NewNode(0, 1000, 1000)
	d.TopLeft = geo.NewPoint(0, 10000)
	g.AddNodeUnchecked(c)
	g.AddNodeUnchecked(d)

	e2 := g.Connect(c, d)
	e2.Points = []*geo.Point{
		{X: 1000, Y: 3500},
		{X: 2640, Y: 3500},
		{X: 2640, Y: 10500},
		{X: 1000, Y: 10500},
	}

	e := layoutgraph.NewNode(0, 1000, 1000)
	e.TopLeft = geo.NewPoint(1000, 7500)
	g.AddNodeUnchecked(e)

	f := layoutgraph.NewNode(0, 1000, 10000)
	f.TopLeft = geo.NewPoint(5000, 0)
	g.AddNodeUnchecked(f)

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, *geo.NewPoint(c.TopLeft.X+c.Width, math.Floor(c.TopLeft.Y+c.Height/2.0)), *e2.Points[0])
	assert.Equal(t, *geo.NewPoint(math.Floor((e.TopLeft.X+e.Width+f.TopLeft.X)/2.0), math.Floor(c.TopLeft.Y+c.Height/2.0)), *e2.Points[1])
	assert.Equal(t, *geo.NewPoint(math.Floor((e.TopLeft.X+e.Width+f.TopLeft.X)/2.0), math.Floor(d.TopLeft.Y+d.Height/2.0)), *e2.Points[2])
	assert.Equal(t, *geo.NewPoint(d.TopLeft.X+d.Width, math.Floor(d.TopLeft.Y+d.Height/2.0)), *e2.Points[3])

	assert.Equal(t, *geo.NewPoint(a.TopLeft.X+c.Width, math.Floor(a.TopLeft.Y+a.Height/2.0)), *e1.Points[0])
	assert.Equal(t, *geo.NewPoint(math.Floor((a.TopLeft.X+a.Width+e2.Points[1].X)/2.0), math.Floor(a.TopLeft.Y+a.Height/2.0)), *e1.Points[1])
	assert.Equal(t, *geo.NewPoint(math.Floor((a.TopLeft.X+a.Width+e2.Points[1].X)/2.0), math.Floor(b.TopLeft.Y+b.Height/2.0)), *e1.Points[2])
	assert.Equal(t, *geo.NewPoint(b.TopLeft.X+d.Width, math.Floor(b.TopLeft.Y+b.Height/2.0)), *e1.Points[3])
}

// 2 going down, 1 going up
// . ┌─────────────┐
// . │             │
// . │             │
// . │             │
// . └─┬───────────┘
// .   │ ▲
// .   │ │
// .   │ │
// .   │ │
// .   │ │
// .   ▼ │
// . ┌───┴─────────┐
// . │             │
// . │             │
// . │             │
// . └─────────────┘
func TestBalanceEdgeSegmentsSharedRoutes(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 1000, 1000)
	a.TopLeft = geo.NewPoint(0, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)

	e1 := g.Connect(a, b)
	e1.Points = []*geo.Point{
		{X: 1, Y: 1000},
		{X: 1, Y: 2000},
	}
	e2 := g.Connect(a, b)
	e2.Points = []*geo.Point{
		{X: 1, Y: 1000},
		{X: 1, Y: 2000},
	}

	e3 := g.Connect(b, a)
	e3.Points = []*geo.Point{
		{X: 3, Y: 2000},
		{X: 3, Y: 1000},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, math.Floor(1000/3.0), e1.Points[0].X)
	assert.Equal(t, math.Floor(1000/3.0), e1.Points[1].X)
	assert.Equal(t, math.Floor(1000/3.0), e2.Points[0].X)
	assert.Equal(t, math.Floor(1000/3.0), e2.Points[1].X)

	assert.Equal(t, math.Floor(1000*2/3.0), e3.Points[0].X)
	assert.Equal(t, math.Floor(1000*2/3.0), e3.Points[1].X)
}

// a-b has smaller range of motion, but when it moves, it should also move its shared route, a-c
// and then when a-c rebalances, it shouldn't treat a-b as a boundary that it needs to move away from
// .           ┌───────────────┐
// .           │               │
// .           │       a       │
// .           │               │
// .           └───────┬───────┘
// .                   │
// . ┌────┐            │
// . │    │            │
// . │ c  │◄───────────┤
// . └────┘            │
// .                   ▼
// .                 ┌──────┐
// .                 │      │
// .                 │  b   │
// .                 │      │
// .                 └──────┘
func TestBalanceEdgeSegmentsSharedRoutesDifferentRanges(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)
	g := layoutgraph.NewGraph()

	a := layoutgraph.NewNode(0, 3000, 1000)
	a.TopLeft = geo.NewPoint(4000, 0)
	b := layoutgraph.NewNode(0, 1000, 1000)
	b.TopLeft = geo.NewPoint(4500, 4000)
	c := layoutgraph.NewNode(0, 1000, 1000)
	c.TopLeft = geo.NewPoint(0, 2000)
	g.AddNodeUnchecked(a)
	g.AddNodeUnchecked(b)
	g.AddNodeUnchecked(c)

	ab := g.Connect(a, b)
	ab.Points = []*geo.Point{
		{X: b.TopLeft.X + 10, Y: a.TopLeft.Y + a.Height},
		{X: b.TopLeft.X + 10, Y: b.TopLeft.Y},
	}

	ac := g.Connect(a, c)
	ac.Points = []*geo.Point{
		{X: b.TopLeft.X + 10, Y: a.TopLeft.Y + a.Height},
		{X: b.TopLeft.X + 10, Y: c.TopLeft.Y + math.Round(c.Height/2)},
		{X: c.TopLeft.X + c.Width, Y: c.TopLeft.Y + math.Round(c.Height/2)},
	}

	err := BalanceEdgeSegments(ctx, g)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, *geo.NewPoint(b.TopLeft.X+math.Round(b.Width/2), a.TopLeft.Y+a.Height), *ab.Points[0])
	assert.Equal(t, *geo.NewPoint(b.TopLeft.X+math.Round(b.Width/2), b.TopLeft.Y), *ab.Points[1])

	assert.Equal(t, *geo.NewPoint(b.TopLeft.X+math.Round(b.Width/2), a.TopLeft.Y+a.Height), *ac.Points[0])
	assert.Equal(t, *geo.NewPoint(b.TopLeft.X+math.Round(b.Width/2), c.TopLeft.Y+math.Round(c.Height/2)), *ac.Points[1])
	assert.Equal(t, *geo.NewPoint(c.TopLeft.X+c.Width, c.TopLeft.Y+math.Round(c.Height/2)), *ac.Points[2])
}

func TestClusterBalancing(t *testing.T) {
	ctx := withTestLogger(context.Background(), t)

	a := layoutgraph.NewNode(1, 215, 126)
	a.TopLeft = geo.NewPoint(1349, 1232)

	b := layoutgraph.NewNode(2, 162, 126)
	b.TopLeft = geo.NewPoint(1376, 1421)

	c := layoutgraph.NewNode(3, 139, 126)
	c.TopLeft = geo.NewPoint(1387, 1610)

	d := layoutgraph.NewNode(4, 160, 126)
	d.TopLeft = geo.NewPoint(1000, 1275)

	e := layoutgraph.NewNode(5, 160, 126)
	e.TopLeft = geo.NewPoint(1000, 1421)

	f := layoutgraph.NewNode(6, 160, 126)
	f.TopLeft = geo.NewPoint(1000, 1567)

	graph := layoutgraph.NewGraph()
	for _, n := range []*layoutgraph.Node{a, b, c, d, e, f} {
		graph.AddNewNodeToContainer(nil, n)
	}

	ab := graph.Connect(a, b)
	ab.Points = []*geo.Point{geo.NewPoint(1457, 1358), geo.NewPoint(1457, 1421)}
	ab.TargetArrowhead = layoutgraph.TriangleArrowhead

	cb := graph.Connect(c, b)
	cb.Points = []*geo.Point{geo.NewPoint(1457, 1610), geo.NewPoint(1457, 1547)}
	cb.SourceArrowhead = layoutgraph.TriangleArrowhead

	bd := graph.Connect(b, d)
	bd.Points = []*geo.Point{geo.NewPoint(1376, 1484), geo.NewPoint(1268, 1484), geo.NewPoint(1268, 1338), geo.NewPoint(1160, 1338)}
	bd.SourceArrowhead = layoutgraph.TriangleArrowhead
	bd.TargetArrowhead = layoutgraph.TriangleArrowhead

	be := graph.Connect(b, e)
	be.Points = []*geo.Point{geo.NewPoint(1376, 1484), geo.NewPoint(1160, 1484)}
	be.SourceArrowhead = layoutgraph.TriangleArrowhead
	be.TargetArrowhead = layoutgraph.TriangleArrowhead

	bf := graph.Connect(b, f)
	bf.Points = []*geo.Point{geo.NewPoint(1376, 1484), geo.NewPoint(1268, 1484), geo.NewPoint(1268, 1630), geo.NewPoint(1160, 1630)}
	bf.SourceArrowhead = layoutgraph.TriangleArrowhead
	bf.TargetArrowhead = layoutgraph.TriangleArrowhead

	graph.CellSize = 215

	vessel := layoutgraph.NewNode(8, 160, 418)
	vessel.TopLeft = geo.NewPoint(0, 275)

	cluster := &layoutgraph.Cluster{
		Vessel:             vessel,
		Nodes:              []*layoutgraph.Node{d, e, f},
		Arrangement:        layoutgraph.Column,
		DesiredArrangement: layoutgraph.Column,
		EdgeAbductions: []*layoutgraph.EdgeAbduction{
			{Edge: bd, CurrentFrom: b, OriginallyTo: d, CurrentTo: vessel},
			{Edge: be, CurrentFrom: b, OriginallyTo: e, CurrentTo: vessel},
			{Edge: bf, CurrentFrom: b, OriginallyTo: f, CurrentTo: vessel},
		},
		Container: nil,
		FixedSize: false,
		Padding:   20,
	}
	graph.Clusters = map[*layoutgraph.Node]*layoutgraph.Cluster{
		vessel: cluster,
	}
	for _, n := range cluster.Nodes {
		n.Cluster = cluster
	}

	err := BalanceEdgeSegments(ctx, graph)
	if err != nil {
		t.Fatal(err)
	}

	distributionVals := evenlyDistribute(f.TopLeft.X+f.Width, a.TopLeft.X, 1)
	assert.Equal(t, len(distributionVals), 1)
	assert.Equal(t, distributionVals[0], bf.Points[1].X)
	assert.Equal(t, distributionVals[0], bf.Points[2].X)
}
