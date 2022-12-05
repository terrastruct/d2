package d2graph

import "oss.terrastruct.com/d2/d2target"

func (obj *Object) IsSequenceDiagram() bool {
	return obj != nil && obj.Attributes.Shape.Value == d2target.ShapeSequenceDiagram
}

func (obj *Object) outerSequenceDiagram() *Object {
	for obj != nil {
		obj = obj.Parent
		if obj.IsSequenceDiagram() {
			return obj
		}
	}
	return nil
}

// groups are objects in sequence diagrams that have no messages connected
// and does not have a note as a child (a note can appear within a group, but it's a child of an actor)
func (obj *Object) IsSequenceDiagramGroup() bool {
	if obj.outerSequenceDiagram() == nil {
		return false
	}
	for _, e := range obj.Graph.Edges {
		if e.Src == obj || e.Dst == obj {
			return false
		}
	}
	for _, ch := range obj.ChildrenArray {
		// if the child contains a message, it's a span, not a note
		if !ch.ContainsAnyEdge(obj.Graph.Edges) {
			return false
		}
	}
	return true
}

// notes are descendant of actors with no edges and no children
func (obj *Object) IsSequenceDiagramNote() bool {
	if obj.outerSequenceDiagram() == nil {
		return false
	}
	return !obj.hasEdgeRef() && !obj.ContainsAnyEdge(obj.Graph.Edges) && len(obj.ChildrenArray) == 0
}

func (obj *Object) hasEdgeRef() bool {
	for _, ref := range obj.References {
		if ref.MapKey != nil && len(ref.MapKey.Edges) > 0 {
			return true
		}
	}

	return false
}

func (obj *Object) ContainsAnyEdge(edges []*Edge) bool {
	for _, e := range edges {
		if e.ContainedBy(obj) {
			return true
		}
	}
	return false
}

func (e *Edge) ContainedBy(obj *Object) bool {
	for _, ref := range e.References {
		curr := ref.ScopeObj
		for curr != nil {
			if curr == obj {
				return true
			}
			curr = curr.Parent
		}
	}
	return false
}
