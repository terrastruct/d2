package d2ir

func Overlay(base, overlay *Map) *Map {
	for _, of := range overlay.Fields {
		bf := base.GetField(of.Name)
		if bf == nil {
			base.Fields = append(base.Fields, of.Copy(base).(*Field))
			continue
		}
		if of.Primary_ != nil {
			bf.Primary_ = of.Primary_.Copy(bf).(*Scalar)
		}
		switch ofc := of.Composite.(type) {
		case *Array:
			bf.Composite = ofc.Copy(bf).(*Map)
		case *Map:
			if bf.Map() != nil {
				bf.Composite = Overlay(bf.Map(), ofc)
			} else {
				bf.Composite = of.Composite.Copy(bf).(*Map)
			}
		}
	}

	for _, oe := range overlay.Edges {
		bea := base.GetEdges(oe.ID)
		if len(bea) == 0 {
			base.Edges = append(base.Edges, oe.Copy(base).(*Edge))
			continue
		}
		be := bea[0]
		if oe.Primary_ != nil {
			be.Primary_ = oe.Primary_.Copy(be).(*Scalar)
		}
		if oe.Map_ != nil {
			if be.Map_ != nil {
				be.Map_ = Overlay(be.Map(), oe.Map_)
			} else {
				be.Map_ = oe.Map_.Copy(be).(*Map)
			}
		}
	}

	return base
}
