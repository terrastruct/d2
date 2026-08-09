package d2ir

func structureCounts(m *Map) (fields, edges int) {
	if m == nil {
		return 0, 0
	}
	return m.FieldCountRecursive(), m.EdgeCountRecursive()
}

func markStructureCountDelta(parent *Map, beforeFields, beforeEdges int, after *Map) {
	afterFields, afterEdges := structureCounts(after)
	if beforeFields != afterFields || beforeEdges != afterEdges {
		parent.markStructureChanged()
	}
}

func OverlayMap(base, overlay *Map) {
	overlayMap(base, overlay, false)
}

func overlayMapIndexed(base, overlay *Map) {
	overlayMap(base, overlay, true)
}

func overlayMap(base, overlay *Map, indexed bool) {
	for _, of := range overlay.Fields {
		var bf *Field
		if indexed {
			bf = base.getFieldIndexed(of.Name)
		} else {
			bf = base.GetField(of.Name)
		}
		if bf == nil {
			base.appendField(of.Copy(base).(*Field))
			continue
		}
		overlayField(bf, of, indexed)
	}

	for _, oe := range overlay.Edges {
		var bea []*Edge
		if indexed {
			bea = base.getEdgesIndexed(oe.ID)
		} else {
			bea = base.GetEdges(oe.ID, nil, nil)
		}
		if len(bea) == 0 {
			base.appendEdge(oe.Copy(base).(*Edge))
			continue
		}
		be := bea[0]
		overlayEdge(be, oe, indexed)
	}
}

func ExpandSubstitution(m, resolved *Map, placeholder *Field) {
	expandSubstitution(m, resolved, placeholder, false)
}

func expandSubstitutionIndexed(m, resolved *Map, placeholder *Field) {
	expandSubstitution(m, resolved, placeholder, true)
}

func expandSubstitution(m, resolved *Map, placeholder *Field, indexed bool) {
	fi := -1
	for i := 0; i < len(m.Fields); i++ {
		if m.Fields[i] == placeholder {
			fi = i
			break
		}
	}

	for _, of := range resolved.Fields {
		var bf *Field
		if indexed {
			bf = m.getFieldIndexed(of.Name)
		} else {
			bf = m.GetField(of.Name)
		}
		if bf == nil {
			m.insertField(fi, of.Copy(m).(*Field))
			fi++
			continue
		}
		overlayField(bf, of, indexed)
	}

	// NOTE this doesn't expand edges in place, and just appends
	// I suppose to do this, there needs to be an edge placeholder too on top of the field placeholder
	// Will wait to see if a problem
	for _, oe := range resolved.Edges {
		var bea []*Edge
		if indexed {
			bea = m.getEdgesIndexed(oe.ID)
		} else {
			bea = m.GetEdges(oe.ID, nil, nil)
		}
		if len(bea) == 0 {
			m.appendEdge(oe.Copy(m).(*Edge))
			continue
		}
		be := bea[0]
		overlayEdge(be, oe, indexed)
	}
}

func OverlayField(bf, of *Field) {
	overlayField(bf, of, false)
}

func overlayFieldIndexed(bf, of *Field) {
	overlayField(bf, of, true)
}

func overlayField(bf, of *Field, indexed bool) {
	if of.Primary_ != nil {
		bf.Primary_ = of.Primary_.Copy(bf).(*Scalar)
	}

	if of.Composite != nil {
		if bf.Map() != nil && of.Map() != nil {
			overlayMap(bf.Map(), of.Map(), indexed)
		} else {
			beforeFields, beforeEdges := structureCounts(bf.Map())
			bf.Composite = of.Composite.Copy(bf).(Composite)
			markStructureCountDelta(ParentMap(bf), beforeFields, beforeEdges, bf.Map())
		}
	}

	bf.References = append(bf.References, of.References...)
}

func OverlayEdge(be, oe *Edge) {
	overlayEdge(be, oe, false)
}

func overlayEdge(be, oe *Edge, indexed bool) {
	if oe.Primary_ != nil {
		be.Primary_ = oe.Primary_.Copy(be).(*Scalar)
	}
	if oe.Map_ != nil {
		if be.Map_ != nil {
			overlayMap(be.Map(), oe.Map_, indexed)
		} else {
			beforeFields, beforeEdges := structureCounts(be.Map())
			be.Map_ = oe.Map_.Copy(be).(*Map)
			markStructureCountDelta(ParentMap(be), beforeFields, beforeEdges, be.Map())
		}
	}
	be.References = append(be.References, oe.References...)
}
