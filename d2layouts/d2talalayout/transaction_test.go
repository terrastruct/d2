package d2talalayout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2sequence"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"

	"github.com/d2lang/d2/d2layouts/d2talalayout/internal/layoutgraph"
)

type d2GraphSnapshot struct {
	json       []byte
	serialized []byte
	root       *d2graph.Object
	objects    []*d2graph.Object
	edges      []*d2graph.Edge
	object     []d2ObjectSnapshot
	edge       []d2EdgeSnapshot
}

type d2ObjectSnapshot struct {
	object        *d2graph.Object
	graph         *d2graph.Graph
	parent        *d2graph.Object
	box           *geo.Box
	topLeft       *geo.Point
	children      map[string]*d2graph.Object
	childrenArray []*d2graph.Object
	labelPosition *string
	iconPosition  *string
	style         []*d2graph.Scalar
}

type d2EdgeSnapshot struct {
	edge            *d2graph.Edge
	src             *d2graph.Object
	dst             *d2graph.Object
	route           []*geo.Point
	labelPosition   *string
	labelPercentage *float64
	style           []*d2graph.Scalar
}

func stylePointers(style d2graph.Style) []*d2graph.Scalar {
	return []*d2graph.Scalar{
		style.Opacity,
		style.Stroke,
		style.Fill,
		style.FillPattern,
		style.StrokeWidth,
		style.StrokeDash,
		style.BorderRadius,
		style.Shadow,
		style.ThreeDee,
		style.Multiple,
		style.Font,
		style.FontSize,
		style.FontColor,
		style.Animated,
		style.Bold,
		style.Italic,
		style.Underline,
		style.Filled,
		style.DoubleBorder,
		style.TextTransform,
	}
}

func snapshotD2Graph(t *testing.T, g *d2graph.Graph) d2GraphSnapshot {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	serialized, err := d2graph.SerializeGraph(g)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := d2GraphSnapshot{
		json:       raw,
		serialized: serialized,
		root:       g.Root,
		objects:    slices.Clone(g.Objects),
		edges:      slices.Clone(g.Edges),
	}
	objects := append([]*d2graph.Object{g.Root}, g.Objects...)
	for _, object := range objects {
		var topLeft *geo.Point
		if object.Box != nil {
			topLeft = object.TopLeft
		}
		children := make(map[string]*d2graph.Object, len(object.Children))
		maps.Copy(children, object.Children)
		snapshot.object = append(snapshot.object, d2ObjectSnapshot{
			object:        object,
			graph:         object.Graph,
			parent:        object.Parent,
			box:           object.Box,
			topLeft:       topLeft,
			children:      children,
			childrenArray: slices.Clone(object.ChildrenArray),
			labelPosition: object.LabelPosition,
			iconPosition:  object.IconPosition,
			style:         stylePointers(object.Style),
		})
	}
	for _, edge := range g.Edges {
		snapshot.edge = append(snapshot.edge, d2EdgeSnapshot{
			edge:            edge,
			src:             edge.Src,
			dst:             edge.Dst,
			route:           slices.Clone(edge.Route),
			labelPosition:   edge.LabelPosition,
			labelPercentage: edge.LabelPercentage,
			style:           stylePointers(edge.Style),
		})
	}
	return snapshot
}

func (snapshot d2GraphSnapshot) assertUnchanged(t *testing.T, g *d2graph.Graph) {
	t.Helper()
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot.json, raw) {
		t.Fatal("D2 graph JSON changed")
	}
	serialized, err := d2graph.SerializeGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot.serialized, serialized) {
		t.Fatal("serialized D2 graph changed")
	}
	if g.Root != snapshot.root || !slices.Equal(g.Objects, snapshot.objects) || !slices.Equal(g.Edges, snapshot.edges) {
		t.Fatal("graph, object, or edge identity changed")
	}
	for _, before := range snapshot.object {
		object := before.object
		var topLeft *geo.Point
		if object.Box != nil {
			topLeft = object.TopLeft
		}
		if object.Graph != before.graph || object.Parent != before.parent || object.Box != before.box || topLeft != before.topLeft {
			t.Fatalf("object %q topology or geometry pointers changed", object.ID)
		}
		if !slices.Equal(object.ChildrenArray, before.childrenArray) || len(object.Children) != len(before.children) {
			t.Fatalf("object %q children changed", object.ID)
		}
		for key, child := range before.children {
			if object.Children[key] != child {
				t.Fatalf("object %q child %q changed", object.ID, key)
			}
		}
		if object.LabelPosition != before.labelPosition || object.IconPosition != before.iconPosition || !slices.Equal(stylePointers(object.Style), before.style) {
			t.Fatalf("object %q adapter-owned pointers changed", object.ID)
		}
	}
	for _, before := range snapshot.edge {
		edge := before.edge
		if edge.Src != before.src || edge.Dst != before.dst || !slices.Equal(edge.Route, before.route) {
			t.Fatalf("edge %q topology or route pointers changed", edge.AbsID())
		}
		if edge.LabelPosition != before.labelPosition || edge.LabelPercentage != before.labelPercentage || !slices.Equal(stylePointers(edge.Style), before.style) {
			t.Fatalf("edge %q label pointers changed", edge.AbsID())
		}
	}
}

func newD2TransactionGraph(withLifeline bool) (*d2graph.Graph, *d2graph.Edge) {
	g := d2graph.NewGraph()
	addObject := func(id string, x float64) *d2graph.Object {
		object := &d2graph.Object{
			Graph:      g,
			Parent:     g.Root,
			ID:         id,
			IDVal:      id,
			Box:        geo.NewBox(geo.NewPoint(x, 20), 100, 60),
			Children:   make(map[string]*d2graph.Object),
			Attributes: d2graph.Attributes{},
		}
		g.Root.Children[id] = object
		g.Root.ChildrenArray = append(g.Root.ChildrenArray, object)
		g.Objects = append(g.Objects, object)
		return object
	}
	a := addObject("a", 10)
	b := addObject("b", 250)
	edge := &d2graph.Edge{
		Src:      a,
		Dst:      b,
		DstArrow: true,
		Route: []*geo.Point{
			geo.NewPoint(110, 50),
			geo.NewPoint(250, 50),
		},
		Attributes: d2graph.Attributes{},
	}
	g.Edges = append(g.Edges, edge)
	if withLifeline {
		endID := d2sequence.LifelineEndID(a.ID)
		g.Edges = append(g.Edges, &d2graph.Edge{
			Src:   a,
			Dst:   &d2graph.Object{ID: endID},
			Route: []*geo.Point{geo.NewPoint(60, 80), geo.NewPoint(60, 180)},
		})
	}
	return g, edge
}

func TestTranslateGraphDoesNotMutateLifelineEndpoint(t *testing.T) {
	g, _ := newD2TransactionGraph(true)
	before := snapshotD2Graph(t, g)

	if _, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), true); err != nil {
		t.Fatal(err)
	}

	before.assertUnchanged(t, g)
}

func TestTranslateGraphAcceptsOnlyCanonicalLifelineID(t *testing.T) {
	g, _ := newD2TransactionGraph(true)
	lifeline := g.Edges[len(g.Edges)-1]
	bindings, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), true)
	if err != nil {
		t.Fatal(err)
	}
	if bindings.edgeDestinations[lifeline] != lifeline.Src {
		t.Fatal("lifeline was not translated through its canonical self-loop identity")
	}

	g, _ = newD2TransactionGraph(true)
	g.Edges[len(g.Edges)-1].Dst.ID = "a-lifeline-end--2043103505"
	if _, err := translateGraph(t.Context(), g, layoutgraph.NewGraph(), true); err == nil {
		t.Fatal("signed 32-bit lifeline ID was accepted")
	}
}

func TestTranslateGraphClonesStyles(t *testing.T) {
	g, _ := newD2TransactionGraph(false)
	source := &d2graph.Scalar{Value: "blue", MapKey: &d2ast.Key{}}
	g.Edges[0].Style.Stroke = source
	talaGraph := layoutgraph.NewGraph()

	if _, err := translateGraph(t.Context(), g, talaGraph, true); err != nil {
		t.Fatal(err)
	}
	translated := talaGraph.Edges[0].Style.Stroke
	if translated == nil || translated.Value != source.Value {
		t.Fatalf("translated stroke = %#v, want an independent copy of %#v", translated, source)
	}
	translated.Value = "red"
	if source.Value != "blue" {
		t.Fatal("mutating the translated style changed the D2 style")
	}
}

func TestCloneStyleCoversEveryField(t *testing.T) {
	hostType := reflect.TypeFor[d2graph.Style]()
	hostScalarType := reflect.TypeFor[*d2graph.Scalar]()
	ownedType := reflect.TypeFor[layoutgraph.EdgeStyle]()
	ownedScalarType := reflect.TypeFor[*layoutgraph.StyleScalar]()
	if hostType.NumField() != ownedType.NumField() {
		t.Fatalf("style field count: d2graph=%d TALA=%d", hostType.NumField(), ownedType.NumField())
	}
	sourceValue := reflect.New(hostType).Elem()
	for i := 0; i < hostType.NumField(); i++ {
		field := hostType.Field(i)
		if field.Type != hostScalarType {
			t.Fatalf("d2graph.Style.%s has type %v; update cloneStyle for the new field", field.Name, field.Type)
		}
		ownedField, ok := ownedType.FieldByName(field.Name)
		if !ok || ownedField.Type != ownedScalarType {
			t.Fatalf("TALA EdgeStyle.%s is missing or has type %v; update cloneStyle", field.Name, ownedField.Type)
		}
		sourceValue.Field(i).Set(reflect.ValueOf(&d2graph.Scalar{
			Value:  field.Name,
			MapKey: &d2ast.Key{},
		}))
	}
	source := sourceValue.Interface().(d2graph.Style)
	clone := cloneStyle(source)
	sourceValue = reflect.ValueOf(source)
	cloneValue := reflect.ValueOf(clone)
	for i := 0; i < hostType.NumField(); i++ {
		original := sourceValue.Field(i).Interface().(*d2graph.Scalar)
		copied := cloneValue.FieldByName(hostType.Field(i).Name).Interface().(*layoutgraph.StyleScalar)
		if copied == nil || copied.Value != original.Value {
			t.Fatalf("cloneStyle did not copy %s", hostType.Field(i).Name)
		}
	}
	if _, ok := reflect.TypeFor[layoutgraph.StyleScalar]().FieldByName("MapKey"); ok {
		t.Fatal("TALA StyleScalar retained D2 parser provenance")
	}
}

func TestTranslateGraphOwnsD2ShapeConversion(t *testing.T) {
	g, _ := newD2TransactionGraph(false)
	g.Objects[0].Shape.Value = "HIERARCHY"
	g.Objects[1].Shape.Value = d2target.ShapeHexagon

	talaGraph := layoutgraph.NewGraph()
	bindings, err := translateGraph(t.Context(), g, talaGraph, true)
	if err != nil {
		t.Fatal(err)
	}
	findNode := func(object *d2graph.Object) *layoutgraph.Node {
		t.Helper()
		id := bindings.objectIDs[object]
		for _, node := range talaGraph.Nodes {
			if node.ID == id {
				return node
			}
		}
		t.Fatalf("translated node %d not found", id)
		return nil
	}

	hierarchy := findNode(g.Objects[0])
	if !hierarchy.ForceHierarchy || hierarchy.ShapeType() != d2target.DSL_SHAPE_TO_SHAPE_TYPE[d2target.ShapeHierarchy] {
		t.Fatalf("hierarchy translation = force:%v shape:%q", hierarchy.ForceHierarchy, hierarchy.ShapeType())
	}
	hexagon := findNode(g.Objects[1])
	if hexagon.ForceHierarchy || hexagon.ShapeType() != d2target.DSL_SHAPE_TO_SHAPE_TYPE[d2target.ShapeHexagon] {
		t.Fatalf("hexagon translation = force:%v shape:%q", hexagon.ForceHierarchy, hexagon.ShapeType())
	}
}

func TestLayoutAppliesGraphsWithObjectHashCollisions(t *testing.T) {
	// These strings have the same 32-bit FNV-1a hash.
	if d2FNV32("costarring") != d2FNV32("liquid") {
		t.Fatal("test strings no longer collide")
	}
	g, _ := newD2TransactionGraph(false)
	delete(g.Root.Children, g.Objects[0].ID)
	delete(g.Root.Children, g.Objects[1].ID)
	g.Objects[0].ID, g.Objects[0].IDVal = "costarring", "costarring"
	g.Objects[1].ID, g.Objects[1].IDVal = "liquid", "liquid"
	g.Root.Children[g.Objects[0].ID] = g.Objects[0]
	g.Root.Children[g.Objects[1].ID] = g.Objects[1]
	talaGraph := layoutgraph.NewGraph()
	bindings, err := translateGraph(t.Context(), g, talaGraph, false)
	if err != nil {
		t.Fatal(err)
	}
	firstID := bindings.objectIDs[g.Objects[0]]
	secondID := bindings.objectIDs[g.Objects[1]]
	if firstID != firstD2SpillEntityID || secondID != firstD2SpillEntityID+1 {
		t.Fatalf("collision IDs = (%d, %d), want deterministic spill IDs", firstID, secondID)
	}

	input, err := newSeedInput(t.Context(), g)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := runSeed(t.Context(), input, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := evaluateSeedResult(t.Context(), input, attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := applySeedResult(t.Context(), g, result); err != nil {
		t.Fatal(err)
	}
}

func TestRouteEdgesAppliesGraphsWithEdgeHashCollisions(t *testing.T) {
	g, first := newD2TransactionGraph(false)
	delete(g.Root.Children, g.Objects[1].ID)
	g.Objects[1].ID, g.Objects[1].IDVal = "d1n2koawf4", "d1n2koawf4"
	g.Root.Children[g.Objects[1].ID] = g.Objects[1]
	third := &d2graph.Object{
		Graph:      g,
		Parent:     g.Root,
		ID:         "z085rpzlv4",
		IDVal:      "z085rpzlv4",
		Box:        geo.NewBox(geo.NewPoint(500, 20), 100, 60),
		Children:   make(map[string]*d2graph.Object),
		Attributes: d2graph.Attributes{},
	}
	g.Root.Children[third.ID] = third
	g.Root.ChildrenArray = append(g.Root.ChildrenArray, third)
	g.Objects = append(g.Objects, third)
	second := &d2graph.Edge{
		Src:      g.Objects[0],
		Dst:      third,
		DstArrow: true,
		Route: []*geo.Point{
			geo.NewPoint(110, 50),
			geo.NewPoint(500, 50),
		},
		Attributes: d2graph.Attributes{},
	}
	g.Edges = append(g.Edges, second)
	if d2FNV32(first.AbsID()) != d2FNV32(second.AbsID()) {
		t.Fatalf("test edges %q and %q no longer collide", first.AbsID(), second.AbsID())
	}
	talaGraph := layoutgraph.NewGraph()
	bindings, err := translateGraph(t.Context(), g, talaGraph, true)
	if err != nil {
		t.Fatal(err)
	}
	if bindings.edgeIDs[first] != firstD2SpillEntityID || bindings.edgeIDs[second] != firstD2SpillEntityID+1 {
		t.Fatalf("edge collision IDs = (%d, %d), want deterministic spill IDs", bindings.edgeIDs[first], bindings.edgeIDs[second])
	}
	objects := slices.Clone(g.Objects)
	edges := slices.Clone(g.Edges)
	if err := RouteEdges(t.Context(), g, g.Edges); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(g.Objects, objects) || !slices.Equal(g.Edges, edges) {
		t.Fatal("successful routing changed D2 object or edge identities")
	}
}

func TestLayoutFailuresLeaveD2GraphUnchanged(t *testing.T) {
	g, _ := newD2TransactionGraph(true)
	g.Edges[0].Style.Stroke = &d2graph.Scalar{Value: "blue"}
	g.Data = map[string]any{"tala-seeds": []int64{}}
	before := snapshotD2Graph(t, g)
	if err := Layout(t.Context(), g, &Options{}); err == nil {
		t.Fatal("expected graph-level empty seed policy to fail")
	}
	before.assertUnchanged(t, g)
	if err := applySeedResult(t.Context(), g, seedResult{}); err == nil {
		t.Fatal("expected empty evaluated seed to fail")
	}
	before.assertUnchanged(t, g)
}

func TestCanceledLayoutLeavesD2GraphUnchanged(t *testing.T) {
	g, _ := newD2TransactionGraph(true)
	before := snapshotD2Graph(t, g)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := Layout(ctx, g, &Options{Seeds: []int64{1}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("layout error = %v, want context.Canceled", err)
	}
	before.assertUnchanged(t, g)
}

func TestPatchCommitCancellationPolicy(t *testing.T) {
	tests := []struct {
		name       string
		context    func() (context.Context, context.CancelFunc)
		commit     func(context.Context, d2Patch) error
		wantErr    error
		wantCommit bool
	}{
		{
			name: "active layout",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(t.Context())
			},
			commit:     commitLayoutPatch,
			wantCommit: true,
		},
		{
			name: "canceled layout",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx, func() {}
			},
			commit:  commitLayoutPatch,
			wantErr: context.Canceled,
		},
		{
			name: "expired layout",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			},
			commit:  commitLayoutPatch,
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "canceled routing",
			context: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx, func() {}
			},
			commit:  commitRoutePatch,
			wantErr: context.Canceled,
		},
		{
			name: "expired routing",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			},
			commit:  commitRoutePatch,
			wantErr: context.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newD2TransactionGraph(false)
			object := g.Objects[0]
			before := object.TopLeft.Copy()
			want := geo.NewPoint(900, 700)
			patch := d2Patch{objects: []d2ObjectPatch{{
				object:  object,
				box:     object.Box,
				topLeft: want,
				width:   object.Width,
				height:  object.Height,
			}}}
			ctx, cancel := tt.context()
			defer cancel()

			err := tt.commit(ctx, patch)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("commit error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if tt.wantCommit {
				if object.TopLeft != want {
					t.Fatalf("committed top-left = %v, want pointer %p", object.TopLeft, want)
				}
			} else if !object.TopLeft.Equals(before) {
				t.Fatalf("rejected patch changed top-left from %v to %v", before, object.TopLeft)
			}
		})
	}
}

func TestPreferContextError(t *testing.T) {
	sentinel := errors.New("algorithm failed")
	if got := preferContextError(t.Context(), "test", sentinel); got != sentinel {
		t.Fatalf("active context error = %v, want sentinel", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := preferContextError(ctx, "test", sentinel); !errors.Is(got, context.Canceled) {
		t.Fatalf("canceled context error = %v, want context.Canceled", got)
	}
}

func TestLayoutPatchFailureLeavesD2GraphUnchanged(t *testing.T) {
	g, _ := newD2TransactionGraph(false)
	before := snapshotD2Graph(t, g)
	talaGraph := layoutgraph.NewGraph()
	first := layoutgraph.NewNode(layoutgraph.EntityID(d2FNV32(g.Objects[0].AbsID())), 120, 80)
	first.TopLeft = geo.NewPoint(400, 400)
	second := layoutgraph.NewNode(layoutgraph.EntityID(d2FNV32(g.Objects[1].AbsID()))+1, 120, 80)
	second.TopLeft = geo.NewPoint(700, 400)
	talaGraph.Nodes = []*layoutgraph.Node{first, second}
	bindings := translation{
		objectIDs: map[*d2graph.Object]layoutgraph.EntityID{
			g.Objects[0]: first.ID,
			g.Objects[1]: layoutgraph.EntityID(d2FNV32(g.Objects[1].AbsID())),
		},
		edgeIDs: map[*d2graph.Edge]layoutgraph.EntityID{
			g.Edges[0]: layoutgraph.EntityID(d2FNV32(g.Edges[0].AbsID())),
		},
	}

	_, err := buildLayoutPatch(t.Context(), g, talaGraph, bindings, nil)
	if err == nil {
		t.Fatal("expected reassociation failure")
	}
	if got := err.Error(); !bytes.Contains([]byte(got), []byte("reassociate graph node")) {
		t.Fatalf("expected late reassociation failure, got %v", err)
	}
	before.assertUnchanged(t, g)
}

func TestLayoutPatchPreservesNilEdgeSlice(t *testing.T) {
	g := d2graph.NewGraph()
	talaGraph := layoutgraph.NewGraph()
	bindings, err := translateGraph(t.Context(), g, talaGraph, false)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := buildLayoutPatch(t.Context(), g, talaGraph, bindings, nil)
	if err != nil {
		t.Fatal(err)
	}
	patch.apply()
	if g.Edges != nil {
		t.Fatalf("empty edge slice = %#v, want nil", g.Edges)
	}
}

func TestLayoutPatchRemovesOnlyConsumedSequenceEdges(t *testing.T) {
	g, first := newD2TransactionGraph(false)
	second := &d2graph.Edge{
		Src:        first.Src,
		Dst:        first.Dst,
		Index:      1,
		Route:      []*geo.Point{geo.NewPoint(110, 60), geo.NewPoint(250, 60)},
		Attributes: d2graph.Attributes{},
	}
	third := &d2graph.Edge{
		Src:        first.Src,
		Dst:        first.Dst,
		Index:      2,
		Route:      []*geo.Point{geo.NewPoint(110, 70), geo.NewPoint(250, 70)},
		Attributes: d2graph.Attributes{},
	}
	g.Edges = append(g.Edges, second, third)
	talaGraph := layoutgraph.NewGraph()
	bindings, err := translateGraph(t.Context(), g, talaGraph, true)
	if err != nil {
		t.Fatal(err)
	}

	patch, err := buildLayoutPatch(t.Context(), g, talaGraph, bindings, map[layoutgraph.EntityID]struct{}{
		bindings.edgeIDs[second]: {},
	})
	if err != nil {
		t.Fatal(err)
	}
	patch.apply()
	if !slices.Equal(g.Edges, []*d2graph.Edge{first, third}) {
		t.Fatalf("remaining edges = %p, want first and third", g.Edges)
	}
}

func TestLayoutPatchRejectsUnpositionedExtraNode(t *testing.T) {
	g := d2graph.NewGraph()
	talaGraph := layoutgraph.NewGraph()
	bindings, err := translateGraph(t.Context(), g, talaGraph, false)
	if err != nil {
		t.Fatal(err)
	}
	talaGraph.Nodes = append(talaGraph.Nodes, layoutgraph.NewNode(42, 10, 10))

	_, err = buildLayoutPatch(t.Context(), g, talaGraph, bindings, nil)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("no position")) {
		t.Fatalf("layout patch error = %v, want an unpositioned-node error", err)
	}
}

func TestRoutePatchNormalizesEmptyEdgeSlice(t *testing.T) {
	g := d2graph.NewGraph()
	g.Edges = make([]*d2graph.Edge, 0)
	patch, err := buildRoutePatch(t.Context(), g, translation{edgeIDs: make(map[*d2graph.Edge]layoutgraph.EntityID)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	patch.apply()
	if g.Edges != nil {
		t.Fatalf("empty edge slice = %#v, want nil", g.Edges)
	}
}

func TestRouteEdgesFailureLeavesD2GraphUnchanged(t *testing.T) {
	g, _ := newD2TransactionGraph(true)
	before := snapshotD2Graph(t, g)
	_, foreign := newD2TransactionGraph(false)
	if foreign.AbsID() != g.Edges[0].AbsID() {
		t.Fatal("foreign edge must have the same ID as a graph edge")
	}

	err := RouteEdges(t.Context(), g, []*d2graph.Edge{foreign})
	if err == nil {
		t.Fatal("expected foreign edge selection to fail")
	}
	before.assertUnchanged(t, g)
}

func TestSuccessfulLayoutPreservesD2Identity(t *testing.T) {
	g, _ := newD2TransactionGraph(true)
	g.Edges[0].Style.Stroke = &d2graph.Scalar{Value: "blue"}
	stroke := g.Edges[0].Style.Stroke
	objects := slices.Clone(g.Objects)
	edges := slices.Clone(g.Edges)
	lifeline := g.Edges[len(g.Edges)-1]
	originalDestination := lifeline.Dst

	if err := Layout(t.Context(), g, &Options{Seeds: []int64{1}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(g.Objects, objects) || !slices.Equal(g.Edges, edges) {
		t.Fatal("successful layout replaced D2 objects or edges")
	}
	if lifeline.Dst != lifeline.Src || lifeline.Dst == originalDestination {
		t.Fatal("successful layout did not apply the synthetic lifeline endpoint rewrite")
	}
	if g.Edges[0].Style.Stroke != stroke || stroke.Value != "blue" {
		t.Fatal("successful layout leaked a translated style mutation into the D2 graph")
	}
}

func TestObjectPatchPreservesFontSizeScalar(t *testing.T) {
	tests := []struct {
		name     string
		existing *d2graph.Scalar
	}{
		{name: "existing scalar", existing: &d2graph.Scalar{Value: "12"}},
		{name: "missing scalar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, _ := newD2TransactionGraph(false)
			object := g.Objects[0]
			object.Style.FontSize = tt.existing
			node := layoutgraph.NewNode(1, 120, 80)
			node.TopLeft = geo.NewPoint(300, 200)
			fontSize := 18
			node.FontSize = &fontSize

			objectPatch, err := buildObjectPatch(t.Context(), object, node)
			if err != nil {
				t.Fatal(err)
			}
			d2Patch{objects: []d2ObjectPatch{objectPatch}}.apply()

			if object.Style.FontSize == nil || object.Style.FontSize.Value != "18" {
				t.Fatalf("font size = %#v, want 18", object.Style.FontSize)
			}
			if tt.existing != nil && object.Style.FontSize != tt.existing {
				t.Fatal("font-size update replaced the existing scalar")
			}
		})
	}
}

func TestEdgePatchPreservesReverseArrowSemantics(t *testing.T) {
	g, edge := newD2TransactionGraph(false)
	_ = g
	edge.SrcArrow = true
	edge.DstArrow = false
	result := &layoutgraph.Edge{
		Points: []*geo.Point{
			geo.NewPoint(10, 20),
			geo.NewPoint(30, 40),
		},
		IsCurve: true,
		Label: &layoutgraph.Label{
			Position: label.OutsideTopLeft,
			Width:    80,
			Height:   20,
		},
		LabelPercentage: 0.25,
	}

	edgePatch, err := buildEdgePatch(t.Context(), edge, result, true)
	if err != nil {
		t.Fatal(err)
	}
	d2Patch{edges: []d2EdgePatch{edgePatch}}.apply()

	if got, want := *edge.LabelPosition, label.OutsideTopLeft.Mirrored().String(); got != want {
		t.Fatalf("label position = %q, want %q", got, want)
	}
	if got, want := *edge.LabelPercentage, 0.75; got != want {
		t.Fatalf("label percentage = %v, want %v", got, want)
	}
	if edge.LabelDimensions.Width != 80 || edge.LabelDimensions.Height != 20 {
		t.Fatalf("label dimensions = %#v, want 80x20", edge.LabelDimensions)
	}
	if !edge.IsCurve {
		t.Fatal("curve flag was not applied")
	}
	if !edge.Route[0].Equals(geo.NewPoint(30, 40)) || !edge.Route[1].Equals(geo.NewPoint(10, 20)) {
		t.Fatalf("route was not reversed: %v", edge.Route)
	}
	if edge.Route[0] == result.Points[1] || edge.Route[1] == result.Points[0] {
		t.Fatal("route points alias the TALA result")
	}
}

func TestRoutePatchUpdatesOnlyRoutedEdges(t *testing.T) {
	g, routedEdge := newD2TransactionGraph(false)
	routedEdge.SrcArrow = true
	routedEdge.DstArrow = false
	routedEdge.LabelDimensions = d2target.TextDimensions{Width: 11, Height: 12}
	relabeledEdge := &d2graph.Edge{
		Src:   routedEdge.Src,
		Dst:   routedEdge.Dst,
		Index: 1,
		Route: []*geo.Point{
			geo.NewPoint(12, 34),
			geo.NewPoint(56, 78),
		},
		IsCurve: true,
		Attributes: d2graph.Attributes{
			Label:           d2graph.Scalar{Value: "unselected"},
			LabelDimensions: d2target.TextDimensions{Width: 21, Height: 22},
		},
	}
	relabeledPosition := label.OutsideBottomRight.String()
	relabeledPercentage := 0.6
	relabeledEdge.LabelPosition = &relabeledPosition
	relabeledEdge.LabelPercentage = &relabeledPercentage
	relabeledRoute := append([]*geo.Point(nil), relabeledEdge.Route...)
	relabeledPositionPointer := relabeledEdge.LabelPosition
	relabeledPercentagePointer := relabeledEdge.LabelPercentage
	g.Edges = append(g.Edges, relabeledEdge)
	edges := slices.Clone(g.Edges)
	bindings := translation{edgeIDs: map[*d2graph.Edge]layoutgraph.EntityID{
		routedEdge:    101,
		relabeledEdge: 202,
	}}
	routed := &layoutgraph.Edge{
		ID:              101,
		Points:          []*geo.Point{geo.NewPoint(10, 20), geo.NewPoint(30, 40)},
		IsCurve:         true,
		Label:           &layoutgraph.Label{Position: label.OutsideTopLeft, Width: 80, Height: 20},
		LabelPercentage: 0.25,
	}

	patch, err := buildRoutePatch(t.Context(), g, bindings, []*layoutgraph.Edge{routed})
	if err != nil {
		t.Fatal(err)
	}
	patch.apply()

	if !slices.Equal(g.Edges, edges) {
		t.Fatal("route patch replaced edge objects or changed their order")
	}
	if got, want := *routedEdge.LabelPosition, label.OutsideTopLeft.Mirrored().String(); got != want {
		t.Fatalf("routed label position = %q, want %q", got, want)
	}
	if got, want := *routedEdge.LabelPercentage, 0.75; got != want {
		t.Fatalf("routed label percentage = %v, want %v", got, want)
	}
	if !routedEdge.IsCurve || !routedEdge.Route[0].Equals(geo.NewPoint(30, 40)) {
		t.Fatalf("routed edge was not reversed and curved: %#v", routedEdge)
	}
	if routedEdge.LabelDimensions != (d2target.TextDimensions{Width: 11, Height: 12}) ||
		relabeledEdge.LabelDimensions != (d2target.TextDimensions{Width: 21, Height: 22}) {
		t.Fatal("route-only patch changed label dimensions")
	}
	if !relabeledEdge.IsCurve || !slices.Equal(relabeledEdge.Route, relabeledRoute) ||
		relabeledEdge.LabelPosition != relabeledPositionPointer ||
		relabeledEdge.LabelPercentage != relabeledPercentagePointer {
		t.Fatal("route patch changed an unselected edge")
	}
	if routedEdge.Route[0] == routed.Points[1] {
		t.Fatal("route patch retained TALA point pointers")
	}
}
