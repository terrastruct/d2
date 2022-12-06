package d2graph

import "oss.terrastruct.com/d2/d2target"

func (obj *Object) IsSequenceDiagram() bool {
	return obj != nil && obj.Attributes.Shape.Value == d2target.ShapeSequenceDiagram
}

func (obj *Object) OuterSequenceDiagram() *Object {
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
	if obj.OuterSequenceDiagram() == nil {
		return false
	}
	for _, e := range obj.Graph.Edges {
		if e.Src == obj || e.Dst == obj {
			if obj.AbsID() == "choo" {
				println("\033[1;31m--- DEBUG:", "=======================", "\033[m")
				println("\033[1;31m--- DEBUG:", e.AbsID(), "\033[m")
			}
			return false
		}
	}
	for _, ch := range obj.ChildrenArray {
		// if the child contains a message, it's a span, not a note
		if !ch.ContainsAnyEdge(obj.Graph.Edges) {
			return false
		}
	}
	return obj.ContainsAnyObject(obj.Graph.Objects) || obj.ContainsAnyEdge(obj.Graph.Edges)
}

// notes are descendant of actors with no edges and no children
func (obj *Object) IsSequenceDiagramNote() bool {
	if obj.OuterSequenceDiagram() == nil {
		return false
	}
	return !obj.hasEdgeRef() && !obj.ContainsAnyEdge(obj.Graph.Edges) && len(obj.ChildrenArray) == 0 && !obj.ContainsAnyObject(obj.Graph.Objects)
}

func (obj *Object) hasEdgeRef() bool {
	for _, ref := range obj.References {
		if ref.MapKey != nil && len(ref.MapKey.Edges) > 0 {
			return true
		}
	}

	return false
}

func (obj *Object) ContainsAnyObject(objects []*Object) bool {
	for _, o := range objects {
		if o.ContainedBy(obj) {
			return true
		}
	}
	return false
}

func (o *Object) ContainedBy(obj *Object) bool {
	for _, ref := range o.References {
		curr := ref.UnresolvedScopeObj
		for curr != nil {
			if curr == obj {
				return true
			}
			curr = curr.Parent
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
