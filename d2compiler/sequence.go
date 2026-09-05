package d2compiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/d2lang/d2/d2ast"
	"github.com/d2lang/d2/d2graph"
	"github.com/d2lang/d2/d2ir"
	"github.com/d2lang/d2/d2target"
)

func sequencePathKey(path []d2ast.String) string {
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = strings.ToLower(p.ScalarString())
	}
	return strings.Join(parts, "\x00")
}

func sequenceShape(m *d2ir.Map) string {
	return sequenceClassShape(m, make(map[*d2ir.Map]bool))
}

func sequenceClassShape(m *d2ir.Map, visiting map[*d2ir.Map]bool) string {
	if m == nil || visiting[m] {
		return ""
	}
	visiting[m] = true
	defer delete(visiting, m)
	f := m.GetField(d2ast.FlatUnquotedString("shape"))
	if f != nil && f.Primary() != nil {
		return strings.ToLower(f.Primary().Value.ScalarString())
	}
	class := m.GetField(d2ast.FlatUnquotedString("class"))
	if class == nil {
		return ""
	}
	var names []string
	if class.Primary() != nil {
		names = append(names, class.Primary().String())
	} else if array, ok := class.Composite.(*d2ir.Array); ok {
		for _, value := range array.Values {
			if scalar, ok := value.(*d2ir.Scalar); ok {
				names = append(names, scalar.Value.ScalarString())
			}
		}
	}
	shape := ""
	for _, name := range names {
		if inherited := sequenceClassShape(m.GetClassMap(name), visiting); inherited != "" {
			shape = inherited
		}
	}
	return shape
}

func sequenceObjectField(f *d2ir.Field) bool {
	_, reserved := d2ast.ReservedKeywords[strings.ToLower(f.Name.ScalarString())]
	return !(reserved && f.Name.IsUnquoted())
}

// Normalize v2 message groups only after imports, variables, and boards have
// been resolved by the IR compiler. Legacy sequence_diagram scopes retain their
// original implicit actor resolution.
func (c *compiler) preprocessSeqDiagrams(m *d2ir.Map) {
	if sequenceShape(m) == d2target.ShapeSequenceDiagramV2 {
		c.preprocessSequenceDiagram(m)
		return
	}
	for _, f := range m.Fields {
		if sequenceObjectField(f) && f.Map() != nil {
			c.preprocessSeqDiagrams(f.Map())
		}
	}
}

func (c *compiler) preprocessSequenceDiagram(diagram *d2ir.Map) {
	type scope struct {
		m         *d2ir.Map
		original  []d2ast.String
		canonical []d2ast.String
	}
	var scopes []scope
	invalid := false
	paths := make(map[string][]d2ast.String)
	base := d2ir.BoardIDA(diagram)
	var collect func(*d2ir.Map, []d2ast.String, []d2ast.String)
	collect = func(m *d2ir.Map, canonical, group []d2ast.String) {
		original := d2ir.BoardIDA(m)
		scopes = append(scopes, scope{m, original, canonical})
		paths[sequencePathKey(original)] = canonical
		// Retain the containing message group as the reference scope of moved
		// actors' contents. Their graph parent remains the owning actor.
		referenceScope := canonical
		if group != nil {
			referenceScope = group
		}
		c.sequenceScopes[sequencePathKey(original)] = referenceScope
		isGroup := sequenceShape(m) == d2target.ShapeSequenceDiagramEdgeGroup
		for _, f := range m.Fields {
			if !sequenceObjectField(f) {
				continue
			}
			parent := canonical
			childGroup := group
			if isGroup && sequenceShape(f.Map()) != d2target.ShapeSequenceDiagramEdgeGroup {
				parent = base
				if existing := diagram.GetField(f.Name); existing != nil &&
					sequenceShape(existing.Map()) == d2target.ShapeSequenceDiagramEdgeGroup {
					c.errorf(f.LastRef().AST(), "sequence diagram actor %q conflicts with an edge-group of the same name", f.Name.ScalarString())
					invalid = true
				}
			}
			childPath := append(append([]d2ast.String(nil), parent...), f.Name)
			if sequenceShape(f.Map()) == d2target.ShapeSequenceDiagramEdgeGroup {
				childGroup = childPath
			}
			originalChild := append(append([]d2ast.String(nil), original...), f.Name)
			paths[sequencePathKey(originalChild)] = childPath
			if f.Map() != nil {
				collect(f.Map(), childPath, childGroup)
			}
		}
	}
	collect(diagram, base, nil)
	if invalid {
		return
	}

	// Resolve both endpoints independently in the original hierarchy, then
	// express their canonical paths relative to their eventual graph scope.
	// This also handles parent paths and edges within hoisted actor maps.
	for _, s := range scopes {
		for _, e := range s.m.Edges {
			for _, endpoint := range []*[]d2ast.String{&e.ID.SrcPath, &e.ID.DstPath} {
				absolute := append([]d2ast.String(nil), s.original...)
				path := *endpoint
				for len(path) > 0 && path[0].IsUnquoted() && path[0].ScalarString() == "_" {
					if len(absolute) > 0 {
						absolute = absolute[:len(absolute)-1]
					}
					path = path[1:]
				}
				absolute = append(absolute, path...)
				for i := len(absolute); i >= len(base); i-- {
					if canonical, ok := paths[sequencePathKey(absolute[:i])]; ok {
						absolute = append(append([]d2ast.String(nil), canonical...), absolute[i:]...)
						break
					}
				}
				common := 0
				for common < len(absolute) && common < len(s.canonical) &&
					sequencePathKey(absolute[common:common+1]) == sequencePathKey(s.canonical[common:common+1]) {
					common++
				}
				// An endpoint naming this scope or one of its ancestors must
				// retain a final identifier. Empty paths (or only underscores)
				// are not valid IR edge paths during the subsequent hoist.
				if common == len(absolute) && common > 0 {
					common--
				}
				var relative []d2ast.String
				for i := common; i < len(s.canonical); i++ {
					relative = append(relative, d2ast.FlatUnquotedString("_"))
				}
				*endpoint = append(relative, absolute[common:]...)
			}
		}
	}

	positions := make(map[*d2ir.Field]d2ast.Range)
	// Iterate the snapshot: moving fields must not skip siblings or process
	// newly appended actors again. Copy through OverlayMap to retain IR parents.
	for _, s := range scopes {
		if sequenceShape(s.m) != d2target.ShapeSequenceDiagramEdgeGroup {
			continue
		}
		var retained []*d2ir.Field
		for _, f := range s.m.Fields {
			if !sequenceObjectField(f) || sequenceShape(f.Map()) == d2target.ShapeSequenceDiagramEdgeGroup {
				retained = append(retained, f)
				continue
			}
			sequenceOverlayMap(diagram, &d2ir.Map{Fields: []*d2ir.Field{f}}, positions)
		}
		// This is a move, not a user deletion: DeleteField would also remove
		// messages whose original references named the moved actors.
		s.m.Fields = retained
	}
}

// Declarations of a hoisted actor may occur both at the diagram root and in
// multiple message groups. Merge each attribute in source order rather than
// letting the order of the normalization traversal decide which one wins.
func sequenceOverlayMap(base, overlay *d2ir.Map, positions map[*d2ir.Field]d2ast.Range) {
	for _, field := range overlay.Fields {
		old := base.GetField(field.Name)
		if old == nil {
			d2ir.OverlayMap(base, &d2ir.Map{Fields: []*d2ir.Field{field}})
			continue
		}
		copy := field.Copy(base).(*d2ir.Field)
		if copy.Primary() != nil {
			after := sequencePrimaryRange(field)
			before, known := positions[old]
			if !known && old.Primary() != nil {
				before = sequencePrimaryRange(old)
			}
			if old.Primary() != nil && before.Path == after.Path && after.Before(before) {
				copy.Primary_ = nil
			} else {
				positions[old] = after
			}
		}
		if old.Map() != nil && field.Map() != nil {
			sequenceOverlayMap(old.Map(), field.Map(), positions)
			copy.Composite = nil
		}
		d2ir.OverlayMap(base, &d2ir.Map{Fields: []*d2ir.Field{copy}})
	}
	// Distinct messages in separate groups remain distinct even if both are
	// declared within maps for the same actor.
	for _, edge := range overlay.Edges {
		base.Edges = append(base.Edges, edge.Copy(base).(*d2ir.Edge))
	}
}

func sequencePrimaryRange(field *d2ir.Field) d2ast.Range {
	range_ := field.Primary().Value.GetRange()
	for i := len(field.References) - 1; i >= 0; i-- {
		ref := field.References[i]
		key := ref.Context_.Key
		if ref.Primary() && (key.Primary.Unbox() != nil || key.Value.ScalarBox().Unbox() != nil) {
			range_ = key.Range
			break
		}
	}
	if imported := field.ImportAST(); imported != nil && imported.GetRange().Path != range_.Path {
		return imported.GetRange()
	}
	return range_
}

func (c *compiler) sequenceScope(m *d2ir.Map) []d2ast.String {
	path := d2ir.BoardIDA(m)
	if canonical, ok := c.sequenceScopes[sequencePathKey(path)]; ok {
		return canonical
	}
	return path
}

func (c *compiler) compileSequenceOptions(obj *d2graph.Object, m *d2ir.Map) {
	obj.Sequence = &d2graph.SequenceOptions{}
	for _, option := range []struct {
		name  string
		value *bool
	}{
		{"mirror", &obj.Sequence.Mirror},
		{"numbered", &obj.Sequence.Numbered},
	} {
		f := m.GetField(d2ast.FlatUnquotedString("vars"), d2ast.FlatUnquotedString(option.name))
		if f == nil {
			continue
		}
		if f.Primary() == nil || f.Composite != nil ||
			(f.Primary().Value.ScalarString() != "true" && f.Primary().Value.ScalarString() != "false") {
			c.errorf(f.LastRef().AST(), "sequence diagram vars.%s must be true or false", option.name)
			continue
		}
		*option.value = f.Primary().Value.ScalarString() == "true"
	}
}

func (c *compiler) compileSequenceDiagrams(g *d2graph.Graph) {
	for _, obj := range append([]*d2graph.Object{g.Root}, g.Objects...) {
		switch obj.Shape.Value {
		case d2target.ShapeSequenceDiagramEdgeGroup:
			if !obj.OuterSequenceDiagram().IsSequenceDiagramV2() ||
				(!obj.Parent.IsSequenceDiagramV2() && !obj.Parent.IsSequenceDiagramGroup()) {
				c.errorf(obj.Shape.MapKey, "edge-group must be inside a sequence-diagram or another edge-group")
			}
		case d2target.ShapeSequenceDiagramActorGroup:
			if !obj.Parent.IsSequenceDiagramV2() && !obj.Parent.IsSequenceDiagramActorGroup() {
				c.errorf(obj.Shape.MapKey, "actor-group must be inside a sequence-diagram or another actor-group")
			}
		case d2target.ShapeSequenceDiagramActor:
			if !obj.IsSequenceDiagramActorRepeat() {
				c.errorf(obj.Shape.MapKey, "shape actor must be a descendant of a sequence-diagram actor")
			}
		}
		if obj.IsSequenceDiagramSpan() && !c.explicitLabels[&obj.Attributes] {
			obj.Label.Value = ""
		}
		if (obj.IsSequenceDiagramNote() || obj.IsSequenceDiagramEvent() || obj.IsSequenceDiagramActorRepeat()) &&
			obj.OuterSequenceDiagram().IsSequenceDiagramV2() && len(obj.ChildrenArray) > 0 {
			c.errorf(obj.Label.MapKey, "sequence diagram notes, events, and repeated actors cannot have children")
		}
	}

	edges := append([]*d2graph.Edge(nil), g.Edges...)
	sort.SliceStable(edges, func(i, j int) bool {
		if len(edges[i].References) == 0 || len(edges[j].References) == 0 {
			return false
		}
		left, right := edges[i].References[0].Edge.Range.Start, edges[j].References[0].Edge.Range.Start
		if edges[i].SequenceImportPosition != nil {
			left = *edges[i].SequenceImportPosition
		}
		if edges[j].SequenceImportPosition != nil {
			right = *edges[j].SequenceImportPosition
		}
		if left.Line != right.Line || left.Column != right.Column {
			return left.Before(right)
		}
		return edges[i].References[0].Edge.Range.Before(edges[j].References[0].Edge.Range)
	})
	numbers := make(map[*d2graph.Object]int)
	for _, e := range edges {
		srcDiagram, dstDiagram := e.Src.OuterSequenceDiagram(), e.Dst.OuterSequenceDiagram()
		if !srcDiagram.IsSequenceDiagramV2() && !dstDiagram.IsSequenceDiagramV2() {
			continue
		}
		if srcDiagram != dstDiagram {
			c.errorf(e.GetAstEdge(), "sequence diagram messages must connect actors or spans in the same diagram")
			continue
		}
		if (!e.Src.IsSequenceDiagramActor() && !e.Src.IsSequenceDiagramSpan()) ||
			(!e.Dst.IsSequenceDiagramActor() && !e.Dst.IsSequenceDiagramSpan()) {
			c.errorf(e.GetAstEdge(), "sequence diagram messages can only connect actors or spans")
			continue
		}
		if srcDiagram.Sequence != nil && srcDiagram.Sequence.Numbered {
			numbers[srcDiagram]++
			prefix := fmt.Sprintf("%d", numbers[srcDiagram])
			if e.Label.Value != "" {
				prefix += ". " + e.Label.Value
			}
			e.Label.Value = prefix
		}
	}
}
