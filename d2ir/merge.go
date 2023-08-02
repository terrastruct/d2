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
		bea := base.GetEdges(oe.ID, nil)
		if len(bea) == 0 {
			base.Edges = append(base.Edges, oe.Copy(base).(*Edge))
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
			bf.Composite = of.Composite.Copy(bf).(*Map)
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
