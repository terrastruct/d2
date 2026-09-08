package d2compiler_test

import (
	"strings"
	"testing"

	"github.com/d2lang/d2/d2compiler"
)

func TestQuotedKeywordDefaultLabels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		label  string
	}{
		{"double quoted", `"LEFT"`, "LEFT"},
		{"single quoted", `'ShApE'`, "ShApE"},
		{"null", `"NULL"`, "NULL"},
		{"explicit label", `"LEFT": shown`, "shown"},
		{"explicit reserved property", `"LEFT": {label: shown}`, "shown"},
		{"repeated case variants", "\"LEFT\"\n\"left\"\n'LeFt'", "LEFT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _, err := d2compiler.Compile("quoted-keyword.d2", strings.NewReader(tc.script), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(g.Objects) != 1 || g.Objects[0].Label.Value != tc.label {
				t.Fatalf("objects = %#v, want one object labelled %q", g.Objects, tc.label)
			}
		})
	}
}

func TestQuotedKeywordReferences(t *testing.T) {
	g, _, err := d2compiler.Compile("quoted-reference.d2", strings.NewReader(`direction: right
"LEFT"
"left" -> x
'LeFt'.style.fill: red
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Objects) != 2 || len(g.Edges) != 1 {
		t.Fatalf("objects/edges = %d/%d, want 2/1", len(g.Objects), len(g.Edges))
	}
	left := g.Objects[0]
	if left.Label.Value != "LEFT" || g.Edges[0].Src != left || left.Style.Fill.Value != "red" {
		t.Fatalf("quoted references did not retain their label and target: %#v", left)
	}
	if g.Root.Direction.Value != "right" {
		t.Fatalf("unquoted property was not interpreted: %q", g.Root.Direction.Value)
	}
}

func TestQuotedKeywordDefaultTypes(t *testing.T) {
	g, _, err := d2compiler.Compile("quoted-types.d2", strings.NewReader(`t: {
  shape: sql_table
  "LEFT"
  "RIGHT": int
  "TOP": TOP
}
c: {
  shape: class
  "LEFT"
  "RIGHT": int
  "TOP": TOP
  f()
}
`), nil)
	if err != nil {
		t.Fatal(err)
	}
	columns := g.Objects[0].SQLTable.Columns
	if columns[0].Type.Label != "" || columns[1].Type.Label != "int" || columns[2].Type.Label != "TOP" {
		t.Fatalf("default labels were treated as column types: %#v", columns)
	}
	class := g.Objects[1].Class
	if class.Fields[0].Type != "" || class.Fields[1].Type != "int" || class.Fields[2].Type != "TOP" || class.Methods[0].Return != "void" {
		t.Fatalf("default labels were treated as class types: %#v", class)
	}
}
