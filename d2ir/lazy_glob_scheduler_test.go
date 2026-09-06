package d2ir

import (
	"strings"
	"testing"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2parser"
	"github.com/d2lang/util-go/mapfs"
)

func TestLazyGlobsCanBeginAfterOrdinaryFields(t *testing.T) {
	root := compileIndexTestSource(t, `before
*.style.fill: red
after
`)
	assertLazyGlobScalar(t, root, "red", "before", "style", "fill")
	assertLazyGlobScalar(t, root, "red", "after", "style", "fill")
}

func TestLazyGlobsCanBeginInsideNestedScope(t *testing.T) {
	root := compileIndexTestSource(t, `outside
container: {
  before
  *.style.fill: red
  after
}
sibling
`)
	assertLazyGlobScalar(t, root, "red", "container", "before", "style", "fill")
	assertLazyGlobScalar(t, root, "red", "container", "after", "style", "fill")
	for _, name := range []string{"outside", "container", "sibling"} {
		field := root.GetField(d2ast.FlatUnquotedString(name), d2ast.FlatUnquotedString("style"))
		if field != nil {
			t.Fatalf("nested glob escaped its scope to %s.style", name)
		}
	}
}

func TestLazyGlobsIntroducedByImportPreserveClasses(t *testing.T) {
	const source = `before
...@defs
after: { class: paint }
`
	filesystem, err := mapfs.New(map[string]string{
		"index.d2": source,
		"defs.d2": `***.style.fill: green
imported: { child }
classes: { paint: { style.stroke: blue } }
`,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer filesystem.Close()
	ast, err := d2parser.Parse("index.d2", strings.NewReader(source), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := Compile(ast, &CompileOptions{FS: filesystem})
	if err != nil {
		t.Fatal(err)
	}
	assertLazyGlobScalar(t, root, "green", "before", "style", "fill")
	assertLazyGlobScalar(t, root, "green", "imported", "child", "style", "fill")
	assertLazyGlobScalar(t, root, "green", "after", "style", "fill")
	assertLazyGlobScalar(t, root, "paint", "after", "class")
	assertLazyGlobScalar(t, root, "blue", "classes", "paint", "style", "stroke")
}

func TestEmptyGlobScopePreservesDeferredTargets(t *testing.T) {
	root := &Map{}
	root.initRoot()
	x := &Field{parent: root, Name: d2ast.FlatUnquotedString("x")}
	root.appendField(x)
	c := &compiler{globContextStack: [][]*globContext{{}}}
	c.enqueueLazyPostTargets(x)
	c.applyLazyGlobs([]*Field{x})
	if len(c.lazyPostTargets) != 1 || c.lazyPostTargets[0] != x {
		t.Fatal("settling an empty glob scope discarded its deferred target")
	}
	if _, queued := c.lazyPostQueued[x]; !queued {
		t.Fatal("settling an empty glob scope discarded deferred-target deduplication")
	}
	if version, ok := c.lazySettledVersions[root]; !ok || version != root.structureVersion {
		t.Fatal("empty glob scope did not settle the current structure version")
	}

	// A glob declared before the surrounding key finishes still needs the
	// original post-target, even though its initial scheduling found no globs.
	key, err := d2parser.ParseMapKey("*.style.fill: red")
	if err != nil {
		t.Fatal(err)
	}
	c.ensureGlobContext(&RefContext{Key: key, ScopeMap: root})
	c.applyLazyGlobs(c.takeLazyPostTargets(0))
	assertLazyGlobScalar(t, root, "red", "x", "style", "fill")
}

func TestGlobFieldCreationRespectsNearestShape(t *testing.T) {
	for _, shape := range []string{"class", "sql_table", "rectangle"} {
		t.Run(shape, func(t *testing.T) {
			root := compileIndexTestSource(t, `box: {
  shape: `+shape+`
  explicit: kept
  inner: { shape: rectangle }
}
*.generated: value
*.inner.generated: inner value
*.style.fill: red
`)
			assertLazyGlobScalar(t, root, "kept", "box", "explicit")
			assertLazyGlobScalar(t, root, "inner value", "box", "inner", "generated")
			assertLazyGlobScalar(t, root, "red", "box", "style", "fill")
			if shape == "rectangle" {
				assertLazyGlobScalar(t, root, "value", "box", "generated")
			} else if root.GetField(d2ast.FlatUnquotedString("box"), d2ast.FlatUnquotedString("generated")) != nil {
				t.Fatal("glob created a non-reserved field inside a class or table")
			}
		})
	}
}

func assertLazyGlobScalar(t *testing.T, root *Map, want string, names ...string) {
	t.Helper()
	path := make([]d2ast.String, len(names))
	for i, name := range names {
		path[i] = d2ast.FlatUnquotedString(name)
	}
	field := root.GetField(path...)
	if field == nil || field.Primary() == nil || field.Primary().Value.ScalarString() != want {
		t.Fatalf("%s = %#v, want %q", strings.Join(names, "."), field, want)
	}
}
