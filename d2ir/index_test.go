package d2ir

import (
	"strings"
	"sync"
	"testing"

	"github.com/d2lang/util-go/mapfs"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2parser"
)

func TestMapFieldIndexPreservesLookupSemantics(t *testing.T) {
	unquoted := &Field{Name: d2ast.FlatUnquotedString("style")}
	quoted := &Field{Name: d2ast.FlatDoubleQuotedString("style")}
	kelvin := &Field{Name: d2ast.FlatUnquotedString("\u212Aelvin")}
	m := &Map{Fields: []*Field{unquoted, quoted, kelvin}}

	if got := m.getFieldIndexed(d2ast.FlatUnquotedString("STYLE")); got != unquoted {
		t.Fatalf("unquoted reserved lookup = %p, want %p", got, unquoted)
	}
	if got := m.getFieldIndexed(d2ast.FlatDoubleQuotedString("STYLE")); got != quoted {
		t.Fatalf("quoted reserved lookup = %p, want %p", got, quoted)
	}
	if got := m.getFieldIndexed(d2ast.FlatUnquotedString("kelvin")); got != kelvin {
		t.Fatalf("Unicode case-folded lookup = %p, want %p", got, kelvin)
	}

	// Fields remains public for consumers that construct IR directly. A length
	// change must invalidate a previously built derived index automatically.
	added := &Field{Name: d2ast.FlatUnquotedString("added")}
	m.Fields = append(m.Fields, added)
	if got := m.getFieldIndexed(d2ast.FlatUnquotedString("ADDED")); got != added {
		t.Fatalf("lookup after direct append = %p, want %p", got, added)
	}
}

func TestPublicMapLookupsRemainSafeAfterDirectMutation(t *testing.T) {
	child := &Field{Name: d2ast.FlatUnquotedString("child")}
	containerMap := &Map{Fields: []*Field{child}}
	container := &Field{Name: d2ast.FlatUnquotedString("same"), Composite: containerMap}
	containerMap.parent = container
	empty := &Field{Name: d2ast.FlatUnquotedString("same")}
	m := &Map{Fields: []*Field{empty, container}}
	empty.parent, container.parent, child.parent = m, m, containerMap

	// Prime the compiler-only index, then mutate the public slice without an
	// internal invalidation hook. Exported lookups must still scan live state.
	_ = m.getFieldIndexed(d2ast.FlatUnquotedString("same"))
	if got := m.GetField(d2ast.FlatUnquotedString("same"), d2ast.FlatUnquotedString("child")); got != child {
		t.Fatalf("duplicate nested lookup = %p, want %p", got, child)
	}
	replacement := &Field{parent: m, Name: d2ast.FlatUnquotedString("replacement")}
	m.Fields[0] = replacement
	if got := m.GetField(d2ast.FlatUnquotedString("replacement")); got != replacement {
		t.Fatalf("lookup after same-length replacement = %p, want %p", got, replacement)
	}
	replacement.Name = d2ast.FlatUnquotedString("renamed")
	if got := m.GetField(d2ast.FlatUnquotedString("renamed")); got != replacement {
		t.Fatalf("lookup after direct rename = %p, want %p", got, replacement)
	}
	renamedKey, err := d2parser.ParseKey("renamed")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := m.EnsureField(renamedKey, nil, true, nil); err != nil || len(got) != 1 || got[0] != replacement {
		t.Fatalf("EnsureField after direct rename = %#v, %v; want replacement", got, err)
	}

	edge := &Edge{parent: m, ID: &EdgeID{
		SrcPath: []d2ast.String{d2ast.FlatUnquotedString("a")},
		DstPath: []d2ast.String{d2ast.FlatUnquotedString("b")},
	}}
	m.Edges = []*Edge{edge}
	_ = m.getEdgesIndexed(edge.ID)
	edge.ID.SrcPath[0] = d2ast.FlatUnquotedString("changed")
	query := edge.ID.Copy()
	if got := m.GetEdges(query, nil, nil); len(got) != 1 || got[0] != edge {
		t.Fatalf("edge lookup after direct ID mutation = %#v, want edge", got)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = m.GetField(d2ast.FlatUnquotedString("renamed"))
				_, _ = m.EnsureField(renamedKey, nil, false, nil)
				_ = m.GetEdges(query, nil, nil)
			}
		}()
	}
	wg.Wait()
}

func compileIndexTestSource(t *testing.T, source string) *Map {
	t.Helper()
	ast, err := d2parser.Parse("index-test.d2", strings.NewReader(source), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := Compile(ast, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestOverlayMapUsesLivePublicFieldAndEdgeState(t *testing.T) {
	base := compileIndexTestSource(t, "a: base field\nb\na -> b: base edge\n")
	overlay := compileIndexTestSource(t, "c: overlay field\nb\nc -> b: overlay edge\n")

	field := base.GetField(d2ast.FlatUnquotedString("a"))
	if field == nil || len(base.Edges) != 1 {
		t.Fatal("base fixture did not compile as expected")
	}
	edge := base.Edges[0]
	_ = base.getFieldIndexed(d2ast.FlatUnquotedString("a"))
	_ = base.getEdgesIndexed(edge.ID)
	field.Name = d2ast.FlatUnquotedString("c")
	edge.ID.SrcPath[0] = d2ast.FlatUnquotedString("c")

	OverlayMap(base, overlay)
	if got := len(base.Fields); got != 2 {
		t.Fatalf("field count after overlay = %d, want 2", got)
	}
	if got := field.Primary(); got == nil || got.Value.ScalarString() != "overlay field" {
		t.Fatalf("renamed field primary after overlay = %#v, want overlay field", got)
	}
	if got := len(base.Edges); got != 1 {
		t.Fatalf("edge count after overlay = %d, want 1", got)
	}
	if got := edge.Primary(); got == nil || got.Value.ScalarString() != "overlay edge" {
		t.Fatalf("mutated edge primary after overlay = %#v, want overlay edge", got)
	}
}

func TestExpandSubstitutionUsesLivePublicFieldState(t *testing.T) {
	base := compileIndexTestSource(t, "placeholder\nold: base\n")
	resolved := compileIndexTestSource(t, "new: resolved\n")
	placeholder := base.GetField(d2ast.FlatUnquotedString("placeholder"))
	field := base.GetField(d2ast.FlatUnquotedString("old"))
	if placeholder == nil || field == nil {
		t.Fatal("substitution fixture did not compile as expected")
	}
	_ = base.getFieldIndexed(d2ast.FlatUnquotedString("old"))
	field.Name = d2ast.FlatUnquotedString("new")

	ExpandSubstitution(base, resolved, placeholder)
	if got := len(base.Fields); got != 2 {
		t.Fatalf("field count after substitution expansion = %d, want 2", got)
	}
	if got := field.Primary(); got == nil || got.Value.ScalarString() != "resolved" {
		t.Fatalf("renamed field primary after substitution = %#v, want resolved", got)
	}
}

func TestCreateEdgeUsesLivePublicFieldAndEdgeState(t *testing.T) {
	root := compileIndexTestSource(t, "a\nb\na -> b\n")
	field := root.GetField(d2ast.FlatUnquotedString("a"))
	if field == nil || len(root.Edges) != 1 {
		t.Fatal("edge fixture did not compile as expected")
	}
	existing := root.Edges[0]
	_ = root.getFieldIndexed(d2ast.FlatUnquotedString("a"))
	_ = root.getEdgesIndexed(existing.ID)
	field.Name = d2ast.FlatUnquotedString("c")
	existing.ID.SrcPath[0] = d2ast.FlatUnquotedString("c")

	key, err := d2parser.ParseMapKey("c -> b")
	if err != nil {
		t.Fatal(err)
	}
	refctx := &RefContext{Key: key, Edge: key.Edges[0], ScopeMap: root}
	c := &compiler{globContextStack: [][]*globContext{{}}}
	created, err := root.CreateEdge(NewEdgeIDs(key)[0], refctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(root.Fields); got != 2 {
		t.Fatalf("field count after CreateEdge = %d, want 2", got)
	}
	if len(created) != 1 || created[0].ID.Index == nil || *created[0].ID.Index != 1 {
		t.Fatalf("created edge = %#v, want one edge with index 1", created)
	}
	if got := len(root.Edges); got != 2 {
		t.Fatalf("edge count after CreateEdge = %d, want 2", got)
	}
}

func TestNestedCompileKeyRetainsOuterLazyPostTargets(t *testing.T) {
	root := compileIndexTestSource(t, `*: {
  &foo: yes
  style.fill: red
}
x: {
  foo
  foo: yes
}
`)
	fill := root.GetField(
		d2ast.FlatUnquotedString("x"),
		d2ast.FlatUnquotedString("style"),
		d2ast.FlatUnquotedString("fill"),
	)
	if fill == nil || fill.Primary() == nil || fill.Primary().Value.ScalarString() != "red" {
		t.Fatalf("x.style.fill = %#v, want red", fill)
	}
}

func TestCloneImportMapDoesNotShareEdgeIDs(t *testing.T) {
	index := 1
	template := &Map{}
	template.initRoot()
	edge := &Edge{parent: template, ID: &EdgeID{
		SrcPath: []d2ast.String{d2ast.FlatUnquotedString("a")},
		DstPath: []d2ast.String{d2ast.FlatUnquotedString("b")},
		Index:   &index,
	}}
	template.Edges = []*Edge{edge}

	first := cloneImportMap(template)
	second := cloneImportMap(template)
	first.Edges[0].ID.SrcPath[0] = d2ast.FlatUnquotedString("changed")
	*first.Edges[0].ID.Index = 9
	if got := template.Edges[0].ID.SrcPath[0].ScalarString(); got != "a" {
		t.Fatalf("template source = %q, want a", got)
	}
	if got := second.Edges[0].ID.SrcPath[0].ScalarString(); got != "a" {
		t.Fatalf("second clone source = %q, want a", got)
	}
	if got := *second.Edges[0].ID.Index; got != 1 {
		t.Fatalf("second clone index = %d, want 1", got)
	}
}

func TestLeadingGlobReplaysAfterSelectedFieldImport(t *testing.T) {
	filesystem, err := mapfs.New(map[string]string{
		"index.d2":  "**: default\nx\nx: @nested\n",
		"nested.d2": "a: { b }\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer filesystem.Close()
	ast, err := d2parser.Parse("index.d2", strings.NewReader("**: default\nx\nx: @nested\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := Compile(ast, &CompileOptions{FS: filesystem})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range [][]d2ast.String{
		{d2ast.FlatUnquotedString("x")},
		{d2ast.FlatUnquotedString("x"), d2ast.FlatUnquotedString("a")},
		{d2ast.FlatUnquotedString("x"), d2ast.FlatUnquotedString("a"), d2ast.FlatUnquotedString("b")},
	} {
		field := root.GetField(path...)
		if field == nil || field.Primary() == nil || field.Primary().Value.ScalarString() != "default" {
			t.Fatalf("field %v did not receive leading glob: %#v", d2ast.MakeKeyPathString(path), field)
		}
	}
}

func TestLazyGlobReferencesAreCanonical(t *testing.T) {
	ast, err := d2parser.Parse("canonical.d2", strings.NewReader("**.style.fill: red\nx\ny\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := Compile(ast, nil)
	if err != nil {
		t.Fatal(err)
	}

	var checkMap func(*Map)
	checkMap = func(m *Map) {
		for _, field := range m.Fields {
			for i, ref := range field.References {
				if !ref.DueToLazyGlob_ {
					continue
				}
				for _, previous := range field.References[:i] {
					if previous.DueToLazyGlob_ && sameReferenceContext(previous.Context_, ref.Context_) &&
						previous.KeyPath == ref.KeyPath && previous.String == ref.String &&
						previous.DueToGlob_ == ref.DueToGlob_ {
						t.Fatalf("field %q contains a duplicate lazy-glob reference", field.Name.ScalarString())
					}
				}
			}
			if field.Map() != nil {
				checkMap(field.Map())
			}
		}
	}
	checkMap(root)
}

func TestMapEdgeIndexPreservesWildcardIndexSemantics(t *testing.T) {
	index0, index1 := 0, 1
	makeID := func(index *int, src string) *EdgeID {
		return &EdgeID{
			SrcPath: []d2ast.String{d2ast.FlatUnquotedString(src)},
			DstPath: []d2ast.String{d2ast.FlatUnquotedString("B")},
			Index:   index,
		}
	}
	e0 := &Edge{ID: makeID(&index0, "A")}
	e1 := &Edge{ID: makeID(&index1, "a")}
	m := &Map{Edges: []*Edge{e0, e1}}

	all := m.GetEdges(makeID(nil, "A"), nil, nil)
	if len(all) != 2 || all[0] != e0 || all[1] != e1 {
		t.Fatalf("wildcard index lookup = %#v, want both edges in source order", all)
	}
	one := m.GetEdges(makeID(&index1, "A"), nil, nil)
	if len(one) != 1 || one[0] != e1 {
		t.Fatalf("indexed lookup = %#v, want second edge", one)
	}
}

func TestMapStructureVersionIncludesDescendants(t *testing.T) {
	root := &Map{}
	root.initRoot()
	container := &Field{parent: root, Name: d2ast.FlatUnquotedString("container")}
	root.appendField(container)
	child := &Map{parent: container}
	container.Composite = child

	before := root.structureVersion
	child.appendField(&Field{parent: child, Name: d2ast.FlatUnquotedString("nested")})
	if root.structureVersion != before+1 {
		t.Fatalf("root structure version = %d, want %d", root.structureVersion, before+1)
	}
}

func TestCopyBasePreservesLegacyBoardSelectionAndOrder(t *testing.T) {
	root := &Map{}
	root.initRoot()
	quotedLayers := &Field{parent: root, Name: d2ast.FlatDoubleQuotedString("layers")}
	ordinary := &Field{parent: root, Name: d2ast.FlatUnquotedString("ordinary")}
	scenarios := &Field{parent: root, Name: d2ast.FlatUnquotedString("scenarios")}
	steps := &Field{parent: root, Name: d2ast.FlatUnquotedString("steps")}
	root.Fields = []*Field{steps, quotedLayers, ordinary, scenarios}

	base := root.CopyBase(nil)
	if len(base.Fields) != 1 || base.Fields[0].Name.ScalarString() != "ordinary" {
		t.Fatalf("base fields = %#v, want only ordinary", base.Fields)
	}
	want := []*Field{ordinary, quotedLayers, scenarios, steps}
	for i, field := range want {
		if root.Fields[i] != field {
			t.Fatalf("root field %d = %p, want %p", i, root.Fields[i], field)
		}
	}
}

func TestCopyBasePreservesLegacyQuotedBoardEdgeCleanup(t *testing.T) {
	const source = `"layers" -> x
scenarios: { s: {} }
`
	ast, err := d2parser.Parse("quoted-board.d2", strings.NewReader(source), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := Compile(ast, nil)
	if err != nil {
		t.Fatal(err)
	}
	scenarios := root.GetField(d2ast.FlatUnquotedString("scenarios"))
	if scenarios == nil || scenarios.Map() == nil {
		t.Fatal("scenarios map not found")
	}
	scenario := scenarios.Map().GetField(d2ast.FlatUnquotedString("s"))
	if scenario == nil || scenario.Map() == nil {
		t.Fatal("scenario s not found")
	}
	if got := scenario.Map().GetField(d2ast.FlatDoubleQuotedString("layers")); got != nil {
		t.Fatalf("quoted layers leaked into scenario base: %#v", got)
	}
	if got := len(scenario.Map().Edges); got != 0 {
		t.Fatalf("scenario edge count = %d, want 0", got)
	}
	if got := scenario.Map().GetField(d2ast.FlatUnquotedString("x")); got == nil {
		t.Fatal("ordinary endpoint x was removed from scenario base")
	}
}
