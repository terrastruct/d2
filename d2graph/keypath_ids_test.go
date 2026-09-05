package d2graph_test

import (
	"strings"
	"testing"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2format"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/d2/d2target"
)

func TestEnsureChildCanonicalIDs(t *testing.T) {
	for _, value := range []string{"", "a", "A0", "a_b", "shape", "SHAPE", "NULL", "a.b", "a b", " a ", "a--b", "a\\b", "a\nb", `a"b`, "${a}", "**", "世界", "x\x00y", string([]byte{0xff})} {
		t.Run(value, func(t *testing.T) {
			// Use the old one-element AST formatting expression as the oracle.
			wantID := d2format.Format(&d2ast.KeyPath{Path: []*d2ast.StringBox{d2ast.MakeValueBox(d2ast.RawString(value, true)).StringBox()}})
			wantValue := wantID
			if key, _ := d2parser.ParseKey(wantID); key != nil && len(key.Path) > 0 {
				wantValue = key.Path[0].Unbox().ScalarString()
			}
			g := d2graph.NewGraph()
			quoted := d2ast.FlatDoubleQuotedString(value)
			child := g.Root.EnsureChild([]d2ast.String{quoted})
			if child == g.Root || child.ID != wantID || child.IDVal != wantValue || child.Label.Value != wantValue {
				t.Fatalf("ID/value/label = %q/%q/%q, want %q/%q/%q", child.ID, child.IDVal, child.Label.Value, wantID, wantValue, wantValue)
			}
			if child.Parent != g.Root || child.Graph != g || len(g.Objects) != 1 || g.Objects[0] != child || g.Root.Children[strings.ToLower(wantID)] != child {
				t.Fatal("created child is not registered consistently")
			}
			// Quoting syntax does not distinguish canonical graph IDs.
			repeated := g.Root.EnsureChild([]d2ast.String{&d2ast.SingleQuotedString{Value: value}})
			if repeated != child || len(g.Objects) != 1 || len(g.Root.ChildrenArray) != 1 {
				t.Fatal("canonical lookup created a duplicate object")
			}
		})
	}
}

func TestEnsureChildReservedAndSequenceIDs(t *testing.T) {
	g := d2graph.NewGraph()
	shape := d2ast.FlatUnquotedString("shape")
	if g.Root.EnsureChild([]d2ast.String{shape}) != g.Root {
		t.Fatal("unquoted reserved field created an object")
	}
	quotedShape := g.Root.EnsureChild([]d2ast.String{d2ast.FlatDoubleQuotedString("shape")})
	if quotedShape == g.Root || quotedShape.ID != "shape" {
		t.Fatal("quoted reserved name did not create a child")
	}
	actor := g.Root.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("Alice")})
	if g.Root.EnsureChild([]d2ast.String{d2ast.FlatDoubleQuotedString("alice")}) != actor {
		t.Fatal("case-insensitive lookup created a duplicate object")
	}

	seq := g.Root.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("sequence")})
	seq.Shape.Value = d2target.ShapeSequenceDiagram
	alice := seq.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("alice")})
	group := seq.EnsureChild([]d2ast.String{d2ast.FlatDoubleQuotedString("group.with.dot")})
	if group.EnsureChild([]d2ast.String{d2ast.FlatDoubleQuotedString("alice")}) != alice {
		t.Fatal("group reference did not resolve the sequence actor")
	}
	nested := alice.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("alice")})
	if nested == alice || nested.Parent != alice || nested.ID != "alice" {
		t.Fatal("same-name nested actor resolved to the sequence root")
	}
	peer := group.EnsureChild([]d2ast.String{d2ast.FlatUnquotedString("_"), d2ast.FlatUnquotedString("bob")})
	if peer.Parent != seq || peer.ID != "bob" {
		t.Fatal("parent reference did not retain its scope")
	}
}
