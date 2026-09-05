package d2graph

import (
	"github.com/d2lang/d2/d2target"
)

func (obj *Object) IsSequenceDiagram() bool {
	return obj != nil && (obj.Shape.Value == d2target.ShapeSequenceDiagram || obj.IsSequenceDiagramV2())
}

func (obj *Object) IsSequenceDiagramV2() bool {
	return obj != nil && obj.Shape.Value == d2target.ShapeSequenceDiagramV2
}

// Actors are the direct children of a v2 diagram or an actor group. Descendants
// of an actor have timeline semantics rather than introducing another lifeline.
func (obj *Object) IsSequenceDiagramActor() bool {
	return obj != nil && (obj.Parent.IsSequenceDiagramV2() || obj.Parent.IsSequenceDiagramActorGroup()) &&
		obj.Shape.Value != d2target.ShapeSequenceDiagramEdgeGroup && obj.Shape.Value != d2target.ShapeSequenceDiagramActorGroup
}

func (obj *Object) IsSequenceDiagramActorGroup() bool {
	return obj != nil && obj.OuterSequenceDiagram().IsSequenceDiagramV2() && obj.Shape.Value == d2target.ShapeSequenceDiagramActorGroup
}

// SequenceDiagramActor returns the lifeline owning this actor or timeline item.
func (obj *Object) SequenceDiagramActor() *Object {
	for obj != nil && !obj.IsSequenceDiagram() {
		if obj.IsSequenceDiagramActor() {
			return obj
		}
		obj = obj.Parent
	}
	return nil
}

func (obj *Object) IsSequenceDiagramSpan() bool {
	return obj != nil && obj.OuterSequenceDiagram().IsSequenceDiagramV2() && !obj.IsSequenceDiagramActor() &&
		obj.SequenceDiagramActor() != nil && obj.Shape.Value == ""
}

func (obj *Object) IsSequenceDiagramActorRepeat() bool {
	return obj != nil && obj.OuterSequenceDiagram().IsSequenceDiagramV2() && !obj.IsSequenceDiagramActor() &&
		obj.SequenceDiagramActor() != nil && obj.Shape.Value == d2target.ShapeSequenceDiagramActor
}

func (obj *Object) IsSequenceDiagramEvent() bool {
	return obj != nil && obj.OuterSequenceDiagram().IsSequenceDiagramV2() && !obj.IsSequenceDiagramActor() &&
		obj.SequenceDiagramActor() != nil && obj.Shape.Value != "" && obj.Shape.Value != d2target.ShapePage &&
		obj.Shape.Value != d2target.ShapeSequenceDiagramActor && obj.Shape.Value != d2target.ShapeSequenceDiagramEdgeGroup &&
		obj.Shape.Value != d2target.ShapeSequenceDiagramActorGroup
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
	if obj.OuterSequenceDiagram().IsSequenceDiagramV2() {
		return obj.Shape.Value == d2target.ShapeSequenceDiagramEdgeGroup
	}
	if obj.OuterSequenceDiagram() == nil {
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
	return obj.ContainsAnyObject(obj.Graph.Objects) || obj.ContainsAnyEdge(obj.Graph.Edges)
}

// notes are descendant of actors with no edges and no children
func (obj *Object) IsSequenceDiagramNote() bool {
	if obj.OuterSequenceDiagram().IsSequenceDiagramV2() {
		return !obj.IsSequenceDiagramActor() && obj.SequenceDiagramActor() != nil && obj.Shape.Value == d2target.ShapePage
	}
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

func (e *Edge) GetGroup() *Object {
	for _, ref := range e.References {
		if ref.ScopeObj.IsSequenceDiagramGroup() {
			return ref.ScopeObj
		}
	}
	return nil
}
