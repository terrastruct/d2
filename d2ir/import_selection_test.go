package d2ir

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2parser"
)

func TestSelectiveImportMatchesWholeLibraryCopies(t *testing.T) {
	cases := []struct {
		name, library, source string
	}{
		{
			name: "maps and overlay",
			library: `unused: { spare: { label: unused } }
component: Original {
  a: { style.fill: red }
  b
  a -> b: linked
}
`,
			source: `one: @library.component
two: @library.component
one.a.style.fill: green
two.a.style.stroke: blue
three: before { a.style.stroke: purple }
three: @library.component
three: @library.component
`,
		},
		{
			name: "scalars arrays and nested selection",
			library: `group: {
  number: 42
  text: hello
  values: [one; two; [3; true]; { nested: yes }]
}
`,
			source: `a: @library.group.number
b: @library.group.text
c: @library.group.values
d: @library.group.values
e: @library.group
f: @library.group._.group.text
`,
		},
		{
			name: "substitutions retain importer scope",
			library: `vars: { local: library }
group: { first: ${local}; second: ${outside} }
`,
			source: `vars: { outside: main }
a: @library.group
b: @library.group
a.first: override
`,
		},
		{
			name: "quoted names and Unicode folding",
			library: `"style": quoted
style.fill: red
Kelvin: { label: symbol }
`,
			source: "a: @library.\"style\"\nb: @library.style\nc: @library.KELVIN\nd: @library.kelvin\n",
		},
		{
			name: "spread keeps complete source ancestry",
			library: `group: titled { a; b; a -> b }
`,
			source: `a: @library.group
...@library.group
b: @library.group
`,
		},
		{
			name: "missing selected key",
			library: `group: { a }
`,
			source: `a: @library.group
b: @library.missing
`,
		},
		{
			name: "missing nested selected key",
			library: `group: { a }
`,
			source: `a: @library.group
b: @library.group.missing
`,
		},
		{
			name: "imports inside arrays",
			library: `vars: { item: { value: library } }
item: [one; two]
`,
			source: `items: [@library.vars.item; @library.vars.item; @library.item]
`,
		},
		{
			name: "retained globs",
			library: `***.style.fill: red
group: { a; b; a -> b }
`,
			source: `a: @library.group
b: @library.group
b.c
`,
		},
		{
			name: "retained board contexts",
			library: `group: { a }
layers: { x: { b } }
`,
			source: `a: @library.group
b: @library.group
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full, fullErr := compilePrimedImport(t, tc.library, tc.source, false)
			selected, selectedErr := compilePrimedImport(t, tc.library, tc.source, true)
			if fullErr != selectedErr {
				t.Fatalf("diagnostics differ:\nfull: %s\nselected: %s", fullErr, selectedErr)
			}
			if !bytes.Equal(full, selected) {
				t.Fatal("selected import changed IR values, ordering, or source references")
			}
		})
	}
}

// Prime both variants with the same compiled library, then use the old complete
// library clone as an oracle. This compares the entire resulting IR, including
// references and errors, without adding a production switch for the fast path.
func compilePrimedImport(t *testing.T, library, source string, selective bool) ([]byte, string) {
	t.Helper()
	c := primedImportCompiler(t, library)
	if template := c.importTemplates["library.d2"]; template == nil {
		t.Fatal("library template was not cached")
	} else if !selective {
		template.selective = false
		template.selectionChecked = true
	}
	c.globContextStack = nil
	ast, err := d2parser.Parse("index.d2", strings.NewReader(source), nil)
	if err != nil {
		t.Fatal(err)
	}
	root := &Map{}
	root.initRoot()
	root.parent.(*Field).References[0].Context_.Scope = ast
	root.parent.(*Field).References[0].Context_.ScopeAST = ast
	c.compileMap(root, ast, ast)
	c.compileSubstitutions(root, nil)
	c.overlayClasses(root)
	root.removeSuspendedFields()
	encoded, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if !c.err.Empty() {
		// The existing missing-key diagnostic formats AST pointers. Ignore only
		// those allocation addresses when comparing independent compilations.
		return encoded, regexp.MustCompile(`0x[0-9a-f]+`).ReplaceAllString(c.err.Error(), "0xADDR")
	}
	return encoded, ""
}

func primedImportCompiler(t *testing.T, library string) *compiler {
	t.Helper()
	c := &compiler{
		err:              &d2parser.ParseError{},
		fs:               fstest.MapFS{"library.d2": &fstest.MapFile{Data: []byte(library)}},
		importStack:      []string{"index.d2"},
		seenImports:      make(map[string]struct{}),
		parsedImports:    make(map[string]*d2ast.Map),
		importTemplates:  make(map[string]*importTemplate),
		globContextStack: [][]*globContext{{}},
	}
	key, err := d2parser.ParseMapKey("prime: @library")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.__import(key.Value.Import, true, true); !ok || !c.err.Empty() {
		t.Fatalf("prime library: %v", c.err)
	}
	return c
}

func TestCloneImportFieldIsolatesMutableValuesAndReferences(t *testing.T) {
	template := compileIndexTestSource(t, `component: {
  first: { label: original }
  second
  first -> second: original
  values: [1; two; { label: nested }]
}
`)
	original := template.getFieldIndexed(d2ast.FlatUnquotedString("component"))
	before, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	first := cloneImportField(original)
	second := cloneImportField(original)
	nilScopeMap(first)
	first.Map().Fields[0].Map().Fields[0].Primary_.Value = d2ast.FlatUnquotedString("changed")
	first.Map().Edges[0].ID.SrcPath[0] = d2ast.FlatUnquotedString("changed")
	first.Map().Edges[0].Primary_.Value = d2ast.FlatUnquotedString("changed")
	first.References[0].Context_.ScopeAST = nil
	first.Map().Fields[2].Composite.(*Array).Values[0].(*Scalar).Value.(*d2ast.Number).Value.SetInt64(99)
	for name, node := range map[string]Node{"template": original, "second copy": second} {
		if got := node.Map().Edges[0].ID.SrcPath[0].ScalarString(); got != "first" {
			t.Fatalf("%s edge source = %q", name, got)
		}
		if got := node.Map().Fields[2].Composite.(*Array).Values[0].Primary().Value.ScalarString(); got != "1" {
			t.Fatalf("%s array value = %q", name, got)
		}
		if node.(*Field).References[0].Context_.ScopeMap == nil {
			t.Fatalf("%s reference scope was cleared by another import", name)
		}
	}
	after, err := json.Marshal(template)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("mutating a selected import changed the template")
	}
}

func TestSelectedImportStillValidatesUnselectedLibraryKeys(t *testing.T) {
	ast, err := d2parser.Parse("index.d2", strings.NewReader("a: @library.good\nb: @library.good\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Compile(ast, &CompileOptions{FS: fstest.MapFS{
		"library.d2": &fstest.MapFile{Data: []byte("good: okay\nbad: ${missing}\n")},
	}})
	// Unresolved substitutions on unrelated fields are deliberately not imported.
	// Syntax and compile-time key errors in the full library still are validated.
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"good: okay\nbad: {\n", "good: okay\n_.bad: invalid\n"} {
		ast, err = d2parser.Parse("index.d2", strings.NewReader("a: @library.good\nb: @library.good\n"), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = Compile(ast, &CompileOptions{FS: fstest.MapFS{
			"library.d2": &fstest.MapFile{Data: []byte(invalid)},
		}})
		if err == nil || !strings.Contains(err.Error(), "library.d2") {
			t.Fatalf("unselected library error for %q = %v", invalid, err)
		}
	}
}

func TestSelectiveImportFallbackPreservesArrayAncestry(t *testing.T) {
	for _, tc := range []struct {
		name, library, key string
		arrayImporter      bool
	}{
		{"array importer", "vars: { item: { value: original } }", "@library.vars.item", true},
		{"array map value", "vars: { item: [ { value: original } ] }", "@library.vars.item", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := primedImportCompiler(t, tc.library)
			template := c.importTemplates["library.d2"].ir
			key, err := d2parser.ParseMapKey("use: " + tc.key)
			if err != nil {
				t.Fatal(err)
			}
			destination := &Map{}
			destination.initRoot()
			field := &Field{parent: destination, Name: d2ast.FlatUnquotedString("use")}
			var importer Node = field
			if tc.arrayImporter {
				importer = &Array{parent: field}
			}
			var imports []*Field
			for i := 0; i < 2; i++ {
				n, ok := c._import(key.Value.Import, importer)
				if !ok {
					t.Fatal(c.err)
				}
				selected := n.(*Field)
				if selected.Parent() == nil || !IsVar(selected) {
					t.Fatal("import lost its original vars ancestry")
				}
				root := RootMap(ParentMap(selected))
				if root == template {
					t.Fatal("import shares the cached template root")
				}
				if len(imports) > 0 && root == RootMap(ParentMap(imports[0])) {
					t.Fatal("two imports share their source ancestry")
				}
				imports = append(imports, selected)
			}
			if !tc.arrayImporter {
				firstMap := imports[0].Composite.(*Array).Values[0].(*Map)
				secondMap := imports[1].Composite.(*Array).Values[0].(*Map)
				ref := firstMap.Fields[0].References[0].Context_
				if ref.ScopeMap != firstMap || secondMap.Fields[0].References[0].Context_.ScopeMap != secondMap {
					t.Fatal("array map reference scope was not remapped to its private copy")
				}
				if !IsVar(ref.ScopeMap) {
					t.Fatal("retained scope lost vars ancestry")
				}
				ref.ScopeMap.Fields[0].Primary_.Value = d2ast.FlatUnquotedString("changed")
				if got := secondMap.Fields[0].Primary_.Value.ScalarString(); got != "original" {
					t.Fatalf("another import changed through the retained scope: %s", got)
				}
				third, ok := c._import(key.Value.Import, importer)
				if !ok {
					t.Fatal(c.err)
				}
				thirdMap := third.(*Field).Composite.(*Array).Values[0].(*Map)
				if got := thirdMap.Fields[0].Primary_.Value.ScalarString(); got != "original" {
					t.Fatalf("a subsequent import changed through the cached template: %s", got)
				}
			}
		})
	}
}

func TestSelectiveImportEligibilityKeepsScalarArrays(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   bool
	}{
		{"item: { class: [base; component] }", true},
		{"item: [one; [two; 3]]", true},
		{"item: [{ value: nested }]", false},
		{"item: [[{ value: nested }]]", false},
		{"**.style.fill: red\nitem", false},
		{"item\nlayers: { other: { child } }", false},
	} {
		t.Run(tc.source, func(t *testing.T) {
			c := primedImportCompiler(t, tc.source)
			if got := c.importTemplates["library.d2"].canSelect(); got != tc.want {
				t.Fatalf("canSelect = %v, want %v", got, tc.want)
			}
		})
	}
}
