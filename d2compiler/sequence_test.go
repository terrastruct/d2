package d2compiler_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/d2lang/d2/d2compiler"
	"github.com/d2lang/d2/d2graph"
)

func compileSequence(t *testing.T, source string) *d2graph.Graph {
	t.Helper()
	g, _, err := d2compiler.Compile("sequence.d2", strings.NewReader(source), nil)
	require.NoError(t, err)
	return g
}

func sequenceObject(t *testing.T, g *d2graph.Graph, path string) *d2graph.Object {
	t.Helper()
	obj, ok := g.Root.HasChild(strings.Split(path, "."))
	require.True(t, ok, "missing %s", path)
	return obj
}

func TestSequenceV2Groups(t *testing.T) {
	t.Parallel()
	g := compileSequence(t, `shape: sequence-diagram
vars: {mirror: true; numbered: true}
team: {shape: actor-group; a}
first: {
  shape: edge-group
  a.work -> b: begin
  nested: {
    shape: edge-group
    b -> a.work: finish {vertical-gap: 0}
    a: {note: hello {shape: page}}
    team.a -> b
  }
}
`)
	require.Len(t, g.Objects, 8)
	require.True(t, g.Root.Sequence.Mirror)
	require.True(t, g.Root.Sequence.Numbered)
	a := sequenceObject(t, g, "a")
	b := sequenceObject(t, g, "b")
	span := sequenceObject(t, g, "a.work")
	group := sequenceObject(t, g, "first")
	nested := sequenceObject(t, g, "first.nested")
	note := sequenceObject(t, g, "a.note")
	require.True(t, a.IsSequenceDiagramActor())
	require.True(t, b.IsSequenceDiagramActor())
	require.True(t, span.IsSequenceDiagramSpan())
	require.Empty(t, span.Label.Value)
	require.True(t, note.IsSequenceDiagramNote())
	require.True(t, note.ContainedBy(nested))
	require.True(t, sequenceObject(t, g, "team.a").IsSequenceDiagramActor())
	require.Len(t, group.ChildrenArray, 1)
	require.Empty(t, nested.ChildrenArray)
	require.Len(t, g.Edges, 3)
	require.Same(t, span, g.Edges[0].Src)
	require.Same(t, b, g.Edges[0].Dst)
	require.True(t, g.Edges[0].ContainedBy(group))
	require.Equal(t, "1. begin", g.Edges[0].Label.Value)
	require.Equal(t, "2. finish", g.Edges[1].Label.Value)
	require.Equal(t, "0", g.Edges[1].VerticalGap.Value)
	require.Equal(t, "3", g.Edges[2].Label.Value)
	require.Same(t, sequenceObject(t, g, "team.a"), g.Edges[2].Src)
}

func TestSequenceV2ActorDescendants(t *testing.T) {
	t.Parallel()
	g := compileSequence(t, `shape: sequence-diagram
a: {
  a
  named: Work {label.near: top-left}
  event: Retry {shape: diamond}
  again: {shape: actor}
  note: hello {shape: page}
}
a.a -> a.named
`)
	require.True(t, sequenceObject(t, g, "a.a").IsSequenceDiagramSpan())
	require.Empty(t, sequenceObject(t, g, "a.a").Label.Value)
	require.Equal(t, "Work", sequenceObject(t, g, "a.named").Label.Value)
	require.True(t, sequenceObject(t, g, "a.event").IsSequenceDiagramEvent())
	require.True(t, sequenceObject(t, g, "a.again").IsSequenceDiagramActorRepeat())
	require.True(t, sequenceObject(t, g, "a.note").IsSequenceDiagramNote())
}

func TestSequenceV2Invalid(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ source, want string }{
		{`shape: sequence-diagram; vars: {mirror: maybe}`, "vars.mirror must be true or false"},
		{`shape: sequence-diagram; vars: {numbered: {yes}}`, "vars.numbered must be true or false"},
		{`shape: sequence-diagram; a.note.shape: page; a.note -> b`, "can only connect actors or spans"},
		{`shape: sequence-diagram; a.event.shape: diamond; a.event -> b`, "can only connect actors or spans"},
		{`shape: sequence-diagram; a.again.shape: actor; a.again -> b`, "can only connect actors or spans"},
		{`shape: sequence-diagram; group.shape: edge-group; group -> b`, "can only connect actors or spans"},
		{`shape: sequence-diagram; group.shape: actor-group; group -> b`, "can only connect actors or spans"},
		{`shape: sequence-diagram; a.note: {shape: page; child}`, "cannot have children"},
		{`a.shape: edge-group`, "edge-group must be inside"},
		{`shape: actor-group`, "actor-group must be inside"},
		{`a.shape: actor`, "shape actor must be a descendant"},
		{`seq: {shape: sequence-diagram; a}; seq.a -> outside`, "same diagram"},
	} {
		t.Run(tt.source, func(t *testing.T) {
			_, _, err := d2compiler.Compile("sequence.d2", strings.NewReader(tt.source), nil)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestSequenceV2ScopesAndBoards(t *testing.T) {
	t.Parallel()
	g, _, err := d2compiler.Compile("root.d2", strings.NewReader(`...@actors
ordinary: {a -> b}
seq: {
  shape: sequence-diagram
  group: {shape: edge-group; a: {work -> _.b}; a -> b}
}
layers: {example: {shape: sequence-diagram; group: {shape: edge-group; x -> y}}}
`), &d2compiler.CompileOptions{FS: fstest.MapFS{
		"actors.d2": &fstest.MapFile{Data: []byte(`imported: {shape: sequence-diagram; group: {shape: edge-group; x -> y}}`)},
	}})
	require.NoError(t, err)
	require.NotNil(t, sequenceObject(t, g, "ordinary.a"))
	require.NotNil(t, sequenceObject(t, g, "ordinary.b"))
	require.NotNil(t, sequenceObject(t, g, "seq.a.work"))
	require.NotNil(t, sequenceObject(t, g, "seq.b"))
	require.NotNil(t, sequenceObject(t, g, "imported.x"))
	require.Len(t, g.Layers, 1)
	require.NotNil(t, sequenceObject(t, g.Layers[0], "x"))
	require.Empty(t, sequenceObject(t, g.Layers[0], "group").ChildrenArray)
}

func TestSequenceLegacyCompatibility(t *testing.T) {
	t.Parallel()
	g := compileSequence(t, `shape: sequence_diagram
a; b
group: {a -> b}
a.note: hello
`)
	require.Nil(t, g.Root.Sequence)
	require.False(t, g.Root.IsSequenceDiagramV2())
	require.Same(t, sequenceObject(t, g, "a"), g.Edges[0].Src)
	require.Same(t, sequenceObject(t, g, "b"), g.Edges[0].Dst)
	require.True(t, sequenceObject(t, g, "group").IsSequenceDiagramGroup())
	require.True(t, sequenceObject(t, g, "a.note").IsSequenceDiagramNote())
}

func TestSequenceV2HoistedAttributeOrder(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`first: {shape: edge-group; a: Early {style.fill: red}; a -> b}
 a: Later {style.fill: blue}`,
		`a: Early {style.fill: red}
 first: {shape: edge-group; a: Later {style.fill: blue}; a -> b}`,
		`first: {shape: edge-group; a: Early {style.fill: red}; a -> b}
 a: Middle {style.fill: green}
 second: {shape: edge-group; a: Later {style.fill: blue}; a -> b}`,
	} {
		g := compileSequence(t, "shape: sequence-diagram\n"+source)
		a := sequenceObject(t, g, "a")
		require.Equal(t, "Later", a.Label.Value)
		require.Equal(t, "blue", a.Style.Fill.Value)
	}
}

func TestSequenceV2ClassShapes(t *testing.T) {
	t.Parallel()
	g := compileSequence(t, `classes: {
 diagram: {shape: sequence-diagram}
 messages: {shape: edge-group}
 participants: {shape: actor-group}
}
class: diagram
team: {class: participants; a}
handshake: {class: messages; team.a -> b}
`)
	require.True(t, g.Root.IsSequenceDiagramV2())
	require.True(t, sequenceObject(t, g, "handshake").IsSequenceDiagramGroup())
	require.True(t, sequenceObject(t, g, "team.a").IsSequenceDiagramActor())
	require.Same(t, sequenceObject(t, g, "team.a"), g.Edges[0].Src)
	require.Same(t, sequenceObject(t, g, "b"), g.Edges[0].Dst)
}

func TestSequenceV2HoistedActorMessages(t *testing.T) {
	t.Parallel()
	g := compileSequence(t, `shape: sequence-diagram
first: {shape: edge-group; a: {work -> _.b: first}}
second: {shape: edge-group; a: {work -> _.b: second}}
`)
	require.Len(t, g.Edges, 2)
	require.Equal(t, "first", g.Edges[0].Label.Value)
	require.Equal(t, "second", g.Edges[1].Label.Value)
	require.Equal(t, 0, g.Edges[0].Index)
	require.Equal(t, 1, g.Edges[1].Index)
	require.True(t, g.Edges[0].ContainedBy(sequenceObject(t, g, "first")))
	require.True(t, g.Edges[1].ContainedBy(sequenceObject(t, g, "second")))
}

func TestSequenceV2ImportOrder(t *testing.T) {
	t.Parallel()
	g, _, err := d2compiler.Compile("root.d2", strings.NewReader(`shape: sequence-diagram
vars: {numbered: true}
a -> b: Before
...@messages
c -> a: After
`), &d2compiler.CompileOptions{FS: fstest.MapFS{
		"messages.d2": &fstest.MapFile{Data: []byte(`b -> c: Imported
a.note: At import {shape: page}`)},
	}})
	require.NoError(t, err)
	labels := make(map[string]*d2graph.Edge)
	for _, edge := range g.Edges {
		labels[edge.Label.Value] = edge
	}
	require.Contains(t, labels, "1. Before")
	require.Contains(t, labels, "2. Imported")
	require.Contains(t, labels, "3. After")
	require.Equal(t, 3, labels["2. Imported"].SequenceImportPosition.Line)
	require.Equal(t, 3, sequenceObject(t, g, "a.note").SequenceImportPosition.Line)
}

func TestSequenceV2ImportedAttributeOrder(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"shape: sequence-diagram\n...@early\na: Later {style.fill: blue}",
		"shape: sequence-diagram\na: Early {style.fill: red}\n...@late",
		"shape: sequence-diagram\n...@early\na: Middle {style.fill: green}\n...@late",
	} {
		g, _, err := d2compiler.Compile("root.d2", strings.NewReader(source), &d2compiler.CompileOptions{FS: fstest.MapFS{
			"early.d2": &fstest.MapFile{Data: []byte(`g: {shape: edge-group; a: Early {style.fill: red}; a -> b}`)},
			"late.d2":  &fstest.MapFile{Data: []byte(`h: {shape: edge-group; a: Later {style.fill: blue}; a -> b}`)},
		}})
		require.NoError(t, err)
		require.Equal(t, "Later", sequenceObject(t, g, "a").Label.Value)
		require.Equal(t, "blue", sequenceObject(t, g, "a").Style.Fill.Value)
	}
}

func TestSequenceV2SubstitutedAttributeOrder(t *testing.T) {
	t.Parallel()
	g := compileSequence(t, `shape: sequence-diagram
vars: {later: Later}
a: Early
first: {shape: edge-group; a: ${later}; a -> b}
a: {style.fill: blue}
`)
	require.Equal(t, "Later", sequenceObject(t, g, "a").Label.Value)
	require.Equal(t, "blue", sequenceObject(t, g, "a").Style.Fill.Value)
}

func TestSequenceV2GroupNameEndpoint(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`shape: sequence-diagram; g: {shape: edge-group; g -> b}`,
		`shape: sequence-diagram; g: {shape: edge-group; _.g -> b}`,
		`shape: sequence-diagram; g: {shape: edge-group; b -> g}`,
		`shape: sequence-diagram; g: {shape: edge-group; nested: {shape: edge-group; g -> b}}`,
	} {
		t.Run(source, func(t *testing.T) {
			_, _, err := d2compiler.Compile("sequence.d2", strings.NewReader(source), nil)
			require.ErrorContains(t, err, "can only connect actors or spans")
		})
	}
	_, _, err := d2compiler.Compile("sequence.d2", strings.NewReader(`shape: sequence-diagram
 g: {shape: edge-group; g: {shape: circle}; g -> b}`), nil)
	require.ErrorContains(t, err, "conflicts with an edge-group")
	g := compileSequence(t, `shape: sequence-diagram
messages: {shape: edge-group; a: {work -> _.a}}
`)
	require.Len(t, g.Edges, 1)
	require.Same(t, sequenceObject(t, g, "a.work"), g.Edges[0].Src)
	require.Same(t, sequenceObject(t, g, "a"), g.Edges[0].Dst)
}

func TestSequenceV2StructuredActorTimeline(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ shape, members string }{
		{"sql_table", "id: int"},
		{"class", "+name: string; +method()"},
	} {
		t.Run(tt.shape, func(t *testing.T) {
			g := compileSequence(t, `shape: sequence-diagram
 a: Actor {shape: `+tt.shape+`; `+tt.members+`}
 a -> b
 a.repeat: {shape: actor}
 a.note: Hello {shape: page}
 a.event: Retry {shape: diamond}
`)
			a := sequenceObject(t, g, "a")
			require.Len(t, a.ChildrenArray, 3)
			require.True(t, sequenceObject(t, g, "a.repeat").IsSequenceDiagramActorRepeat())
			require.True(t, sequenceObject(t, g, "a.note").IsSequenceDiagramNote())
			require.True(t, sequenceObject(t, g, "a.event").IsSequenceDiagramEvent())
			if tt.shape == "sql_table" {
				require.Len(t, a.SQLTable.Columns, 1)
				require.Equal(t, "id", a.SQLTable.Columns[0].Name.Label)
			} else {
				require.Len(t, a.Class.Fields, 1)
				require.Len(t, a.Class.Methods, 1)
			}
		})
	}
}
