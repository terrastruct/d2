package sequencediagram_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2layouts/d2sequence"
	sequencediagram "github.com/d2lang/d2/d2layouts/d2sequence2"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/label"
	"github.com/d2lang/d2/lib/textmeasure"
)

func compile(t *testing.T, script string) *d2graph.Graph {
	t.Helper()
	g, _, err := d2compiler.Compile("index.d2", strings.NewReader("shape: sequence-diagram\n"+script), nil)
	if err != nil {
		t.Fatal(err)
	}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetDimensions(nil, ruler, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := sequencediagram.Layout(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	return g
}
func obj(t *testing.T, g *d2graph.Graph, id string) *d2graph.Object {
	t.Helper()
	for _, o := range g.Objects {
		if o.AbsID() == id {
			return o
		}
	}
	t.Fatalf("object %s not found", id)
	return nil
}
func messages(g *d2graph.Graph) []*d2graph.Edge {
	var es []*d2graph.Edge
	for _, e := range g.Edges {
		if !d2sequence.IsLifelineEnd(e.Dst) {
			es = append(es, e)
		}
	}
	return es
}
func finiteBounds(t *testing.T, g *d2graph.Graph) {
	t.Helper()
	for _, o := range g.Objects {
		for _, v := range []float64{o.TopLeft.X, o.TopLeft.Y, o.Width, o.Height} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("nonfinite geometry for %s: %+v", o.AbsID(), o.Box)
			}
		}
		if o.Width <= 0 || o.Height <= 0 || o.TopLeft.X < 0 || o.TopLeft.Y < 0 || o.TopLeft.X+o.Width > g.Root.Width+.01 || o.TopLeft.Y+o.Height > g.Root.Height+.01 {
			t.Fatalf("%s outside diagram: %v root %v", o.AbsID(), o.Box, g.Root.Box)
		}
	}
	for _, e := range g.Edges {
		for _, p := range e.Route {
			if !g.Root.Box.Contains(p) {
				t.Fatalf("route outside diagram: %s %v", e.AbsID(), p)
			}
		}
	}
}
func TestChronologyAndAfterGap(t *testing.T) {
	g := compile(t, `a -> b: first {vertical-gap: 0}; b -> c: simultaneous; a.n: Note {shape: page}; c -> a: last`)
	es := messages(g)
	if len(es) != 3 {
		t.Fatalf("messages %d", len(es))
	}
	if es[0].Route[0].Y != es[1].Route[0].Y {
		t.Fatal("vertical-gap zero must place the next message at the same time")
	}
	note := obj(t, g, "a.n")
	if note.TopLeft.Y <= es[1].Route[0].Y || note.TopLeft.Y+note.Height >= es[2].Route[0].Y {
		t.Fatal("semicolon-separated note is not between messages")
	}
	finiteBounds(t, g)
}
func TestExplicitGapOverridesAutomaticOverlap(t *testing.T) {
	g := compile(t, `a -> b: one {vertical-gap: 13}; b -> a: two`)
	es := messages(g)
	if got := es[1].Route[0].Y - es[0].Route[0].Y; got != 13 {
		t.Fatalf("after gap = %v, want 13", got)
	}
}
func TestSelfMessagesNestedSpansAndNotes(t *testing.T) {
	g := compile(t, `a -> b.work: request
 b.work: Processing {label.near: top-right}
 b.work.inner -> b.work.inner: self
 b.note: A rather wide explanatory note {shape: page}
 b.work.inner -> a: reply
 unused: {span}
 `)
	es := messages(g)
	self := es[1]
	if len(self.Route) != 4 || self.Route[0].Y >= self.Route[3].Y {
		t.Fatal("self message must have a vertical return")
	}
	outer, inner := obj(t, g, "b.work"), obj(t, g, "b.work.inner")
	if outer.Label.Value != "Processing" || inner.Label.Value != "" {
		t.Fatal("span labels lost or default labels visible")
	}
	if outer.TopLeft.Y >= inner.TopLeft.Y || outer.TopLeft.Y+outer.Height <= inner.TopLeft.Y+inner.Height {
		t.Fatal("parent span does not contain nested interval")
	}
	if self.Route[0].X != inner.TopLeft.X+inner.Width {
		t.Fatal("self message must leave activation boundary")
	}
	finiteBounds(t, g)
}
func TestGroupsMirrorsAndIdentity(t *testing.T) {
	g := compile(t, `vars: {mirror: true}
 participants: {shape: actor-group; a; b}
 exchange: {shape: edge-group; participants.a -> participants.b: start; nested: {shape: edge-group; participants.b -> participants.a: done}}
 participants.a.again: {shape: actor}
 empty: {shape: edge-group}
 `)
	finiteBounds(t, g)
	group := obj(t, g, "exchange")
	nested := obj(t, g, "exchange.nested")
	if !group.Box.Contains(nested.TopLeft) || !group.Box.Contains(nested.Center()) {
		t.Fatal("nested frame escapes outer frame")
	}
	a := obj(t, g, "participants.a")
	mirror := obj(t, g, "participants.a.mirror")
	if mirror.Parent != a || mirror.Label.Value != a.Label.Value {
		t.Fatal("mirror loses actor parent or label")
	}
	for _, o := range g.Objects {
		if o.Parent != nil && o.Parent.Children[strings.ToLower(o.ID)] != o {
			t.Fatalf("inconsistent parent child map: %s", o.AbsID())
		}
	}
	actors := make(map[*d2graph.Object]bool)
	ids := make(map[string]bool)
	for _, e := range g.Edges {
		if d2sequence.IsLifelineEnd(e.Dst) {
			actors[e.Src] = true
			if ids[e.AbsID()] {
				t.Fatal("duplicate lifeline identity")
			}
			ids[e.AbsID()] = true
			if e.Dst.ID != d2sequence.LifelineEndID(e.Src.ID) {
				t.Fatal("unstable lifeline ID")
			}
		}
	}
	if len(actors) != 2 {
		t.Fatalf("actors with lifelines %d want 2", len(actors))
	}
}
func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sequencediagram.Layout(ctx, d2graph.NewGraph()); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}

func TestWideParticipantGroupsDoNotOverlap(t *testing.T) {
	g := compile(t, `producers: A very wide participant group name {shape: actor-group; inner: {shape: actor-group; a}}
 consumers: Another wide participant group name {shape: actor-group; inner: {shape: actor-group; b}}
 producers.inner.a -> consumers.inner.b: message`)
	a, b := obj(t, g, "producers"), obj(t, g, "consumers")
	if a.TopLeft.X+a.Width >= b.TopLeft.X {
		t.Fatalf("participant groups overlap: %v and %v", a.Box, b.Box)
	}
	finiteBounds(t, g)
}

func TestRepeatedStructuredActor(t *testing.T) {
	g := d2graph.NewGraph()
	g.Root.Shape.Value = d2target.ShapeSequenceDiagramV2
	a := &d2graph.Object{Graph: g, Parent: g.Root, ID: "a", IDVal: "a", Class: &d2target.Class{}, Box: geo.NewBox(geo.NewPoint(0, 0), 100, 60), Attributes: d2graph.Attributes{Shape: d2graph.Scalar{Value: d2target.ShapeClass}}, Children: make(map[string]*d2graph.Object)}
	repeat := &d2graph.Object{Graph: g, Parent: a, ID: "again", IDVal: "again", Box: geo.NewBox(geo.NewPoint(0, 0), 60, 60), Attributes: d2graph.Attributes{Shape: d2graph.Scalar{Value: d2target.ShapeSequenceDiagramActor}}}
	a.Children["again"] = repeat
	a.ChildrenArray = []*d2graph.Object{repeat}
	g.Root.Children["a"] = a
	g.Root.ChildrenArray = []*d2graph.Object{a}
	g.Objects = []*d2graph.Object{a, repeat}
	if err := sequencediagram.Layout(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	if repeat.Class == nil || repeat.Shape.Value != d2target.ShapeSequenceDiagramActor {
		t.Fatal("class actor repeat lost structured appearance or semantic role")
	}
	finiteBounds(t, g)
}

func TestQuotedMirrorIdentity(t *testing.T) {
	g := compile(t, `vars: {mirror: true}
 "a.b" -> "with spaces": message`)
	if obj(t, g, `"a.b".mirror`).IDVal != "mirror" {
		t.Fatal("quoted actor mirror ID is not escaped")
	}
	finiteBounds(t, g)
}

func TestNestedSpanLabelsStayBelowHeaders(t *testing.T) {
	g := compile(t, `a; b
 a.outer: Outer activation {label.near: top-right}
 a.outer.inner: Inner activation {label.near: top-right}
 b -> a.outer.inner: Start`)
	a, s := obj(t, g, "a"), obj(t, g, "a.outer")
	if s.TopLeft.Y < a.TopLeft.Y+a.Height {
		t.Fatal("nested activation label overlaps actor header")
	}
	finiteBounds(t, g)
}

func TestEmptyGroupOccupiesItsTextualTime(t *testing.T) {
	g := compile(t, `a -> b: before
 empty: {shape: edge-group}
 a.note: Note {shape: page}
 b -> a: after`)
	group, note := obj(t, g, "empty"), obj(t, g, "a.note")
	es := messages(g)
	if group.TopLeft.Y <= es[0].Route[0].Y || group.TopLeft.Y+group.Height >= note.TopLeft.Y || note.TopLeft.Y+note.Height >= es[1].Route[0].Y {
		t.Fatal("empty message group does not reserve its position in the timeline")
	}
	finiteBounds(t, g)
}

func TestImportedTimelineUsesImportSite(t *testing.T) {
	fs := fstest.MapFS{"part.d2": {Data: []byte(`a.note: Imported note {shape: page}
 b -> c: imported`)}}
	g, _, err := d2compiler.Compile("index.d2", strings.NewReader(`shape: sequence-diagram
 a -> b: before
 ...@part
 c -> a: after`), &d2compiler.CompileOptions{FS: fs})
	if err != nil {
		t.Fatal(err)
	}
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetDimensions(nil, ruler, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := sequencediagram.Layout(context.Background(), g); err != nil {
		t.Fatal(err)
	}
	es := messages(g)
	ys := make(map[string]float64)
	for _, e := range es {
		ys[e.Label.Value] = e.Route[0].Y
	}
	note := obj(t, g, "a.note")
	if !(ys["before"] < note.TopLeft.Y && note.TopLeft.Y+note.Height < ys["imported"] && ys["imported"] < ys["after"]) {
		t.Fatalf("wrong imported chronology: %v, note %v", ys, note.Box)
	}
	finiteBounds(t, g)
}

func TestSelfMessageTravelsOutsideWideActivation(t *testing.T) {
	g := compile(t, `a.work: A deliberately wide activation label {label.near: top-right}
 a.work -> a.work: Recurse`)
	e := messages(g)[0]
	if e.Route[1].X <= e.Route[0].X || e.Route[2].X <= e.Route[3].X {
		t.Fatal("self message bends inside its activation")
	}
	finiteBounds(t, g)
}

func TestUndeclaredActorsFollowEndpointOrderWithinGroup(t *testing.T) {
	g := compile(t, `request: {
 shape: edge-group
 client -> server: Request
 processing: {shape: edge-group; server.work -> database.query: Read}
 }`)
	client, server, database := obj(t, g, "client"), obj(t, g, "server"), obj(t, g, "database")
	if !(client.Center().X < server.Center().X && server.Center().X < database.Center().X) {
		t.Fatalf("actors not in first endpoint order: client=%v server=%v database=%v", client.Center().X, server.Center().X, database.Center().X)
	}
	finiteBounds(t, g)
}

func TestDefaultNestedSpanLabelIsVisibleAboveEvents(t *testing.T) {
	g := compile(t, `client -> server.work: Start
 server.work: Processing {label.near: top-right}
 server.work.inner: Transaction
 server.work.inner -> server.work.inner: Validate
 server.status: Waiting {shape: page}
 server.checkpoint: Checkpoint {shape: diamond}
 server.work.inner -> client: Finished`)
	span := obj(t, g, "server.work.inner")
	if span.LabelPosition == nil || *span.LabelPosition != label.InsideTopLeft.String() {
		t.Fatal("explicit span label should default to the top of its activation")
	}
	p := label.InsideTopLeft.GetPointOnBox(span.Box, label.PADDING, float64(span.LabelDimensions.Width), float64(span.LabelDimensions.Height))
	text := geo.NewBox(p, float64(span.LabelDimensions.Width), float64(span.LabelDimensions.Height))
	if text.TopLeft.X < span.TopLeft.X || text.TopLeft.X+text.Width > span.TopLeft.X+span.Width {
		t.Fatal("activation does not contain its label")
	}
	for _, id := range []string{"server.status", "server.checkpoint"} {
		if text.Overlaps(*obj(t, g, id).Box) {
			t.Fatalf("span label obscured by %s", id)
		}
	}
	self := messages(g)[1]
	if text.TopLeft.Y+text.Height >= self.Route[0].Y {
		t.Fatal("activation label overlaps self message")
	}
	if self.Route[1].X <= span.TopLeft.X+span.Width {
		t.Fatal("self loop bends inside labeled activation")
	}
	finiteBounds(t, g)
}

func TestMirroredSQLTableRecreatesChildMap(t *testing.T) {
	g := compile(t, `vars: {mirror: true}
 a: {shape: sql_table; id: int}
 a -> b`)
	a, mirror := obj(t, g, "a"), obj(t, g, "a.mirror")
	if a.SQLTable == nil || mirror.SQLTable != a.SQLTable {
		t.Fatal("mirror lost SQL table appearance")
	}
	if a.Children["mirror"] != mirror || mirror.Parent != a {
		t.Fatal("SQL table mirror is absent from its parent child map")
	}
	finiteBounds(t, g)
}
