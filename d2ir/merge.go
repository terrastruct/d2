package d2ir

func OverlayMap(base, overlay *Map) {
	for _, of := range overlay.Fields {
		bf := base.GetField(of.Name)
		if bf == nil {
			base.Fields = append(base.Fields, of.Copy(base).(*Field))
			continue
		}
		OverlayField(bf, of)
	}

	for _, oe := range overlay.Edges {
		bea := base.GetEdges(oe.ID, nil, nil)
		if len(bea) == 0 {
			base.Edges = append(base.Edges, oe.Copy(base).(*Edge))
			continue
		}
		be := bea[0]
		OverlayEdge(be, oe)
	}
}

func ExpandSubstitution(m, resolved *Map, placeholder *Field) {
	fi := -1
	for i := 0; i < len(m.Fields); i++ {
		if m.Fields[i] == placeholder {
			fi = i
			break
		}
	}

	for _, of := range resolved.Fields {
		bf := m.GetField(of.Name)
		if bf == nil {
			m.Fields = append(m.Fields[:fi], append([]*Field{of.Copy(m).(*Field)}, m.Fields[fi:]...)...)
			fi++
			continue
		}
		OverlayField(bf, of)
	}

	// NOTE this doesn't expand edges in place, and just appends
	// I suppose to do this, there needs to be an edge placeholder too on top of the field placeholder
	// Will wait to see if a problem
	for _, oe := range resolved.Edges {
		bea := m.GetEdges(oe.ID, nil, nil)
		if len(bea) == 0 {
			m.Edges = append(m.Edges, oe.Copy(m).(*Edge))
			continue
		}
		be := bea[0]
		OverlayEdge(be, oe)
	}
}

func OverlayField(bf, of *Field) {
	if of.Primary_ != nil {
		bf.Primary_ = of.Primary_.Copy(bf).(*Scalar)
	}

	if of.Composite != nil {
		if bf.Map() != nil && of.Map() != nil {
			OverlayMap(bf.Map(), of.Map())
		} else {
			bf.Composite = of.Composite.Copy(bf).(Composite)
		}
	}

	bf.References = append(bf.References, of.References...)
}

func OverlayEdge(be, oe *Edge) {
	if oe.Primary_ != nil {
		be.Primary_ = oe.Primary_.Copy(be).(*Scalar)
	}
	if oe.Map_ != nil {
		if be.Map_ != nil {
			OverlayMap(be.Map(), oe.Map_)
		} else {
			be.Map_ = oe.Map_.Copy(be).(*Map)
		}
	}
	be.References = append(be.References, oe.References...)
}
