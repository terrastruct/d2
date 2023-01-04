package d2graph

func (s *Scalar) Copy() *Scalar {
	if s == nil {
		return nil
	}
	tmp := *s
	return &tmp
}

func (s Style) Copy() Style {
	return Style{
		Opacity:      s.Opacity.Copy(),
		Stroke:       s.Stroke.Copy(),
		Fill:         s.Fill.Copy(),
		StrokeWidth:  s.StrokeWidth.Copy(),
		StrokeDash:   s.StrokeDash.Copy(),
		BorderRadius: s.BorderRadius.Copy(),
		Shadow:       s.Shadow.Copy(),
		ThreeDee:     s.ThreeDee.Copy(),
		Multiple:     s.Multiple.Copy(),
		Font:         s.Font.Copy(),
		FontSize:     s.FontSize.Copy(),
		FontColor:    s.FontColor.Copy(),
		Animated:     s.Animated.Copy(),
		Bold:         s.Bold.Copy(),
		Italic:       s.Italic.Copy(),
		Underline:    s.Underline.Copy(),
		Filled:       s.Filled.Copy(),
	}
}

func (attrs *Attributes) Copy() *Attributes {
	if attrs == nil {
		return nil
	}
	return &Attributes{
		Label:   attrs.Label,
		Style:   attrs.Style.Copy(),
		Icon:    attrs.Icon,
		Tooltip: attrs.Tooltip,
		Link:    attrs.Link,

		Width:  attrs.Width.Copy(),
		Height: attrs.Height.Copy(),

		NearKey:  attrs.NearKey,
		Language: attrs.Language,
		Shape:    attrs.Shape,

		Direction: attrs.Shape,
	}
}

func (o *Object) Copy() *Object {
	return &Object{
		Graph:  o.Graph,
		Parent: o.Parent,

		ID:              o.ID,
		IDVal:           o.IDVal,
		Map:             o.Map,
		LabelDimensions: o.LabelDimensions,
		References:      append([]Reference(nil), o.References...),

		Box:           o.Box.Copy(),
		LabelPosition: o.LabelPosition,
		LabelWidth:    o.LabelHeight,
		LabelHeight:   o.LabelHeight,
		IconPosition:  o.IconPosition,

		Class:    o.Class.Copy(),
		SQLTable: o.SQLTable.Copy(),

		Children:      copyChildrenMap(o.Children),
		ChildrenArray: append([]*Object(nil), o.ChildrenArray...),

		Attributes: o.Attributes.Copy(),

		ZIndex: o.ZIndex,
	}
}

func copyChildrenMap(children map[string]*Object) map[string]*Object {
	children2 := make(map[string]*Object, len(children))
	for id, ch := range children {
		children2[id] = ch
	}
	return children2
}

func (e *Edge) Copy() *Edge {
	return &Edge{
		Index: e.Index,

		MinWidth:  e.MinWidth,
		MinHeight: e.MinHeight,

		SrcTableColumnIndex: e.SrcTableColumnIndex,
		DstTableColumnIndex: e.DstTableColumnIndex,

		LabelDimensions: e.LabelDimensions,
		LabelPosition:   e.LabelPosition,
		LabelPercentage: e.LabelPercentage,

		IsCurve: e.IsCurve,
		Route:   e.Route,

		Src:          e.Src,
		SrcArrow:     e.SrcArrow,
		SrcArrowhead: e.SrcArrowhead.Copy(),
		Dst:          e.Dst,
		DstArrow:     e.DstArrow,
		DstArrowhead: e.DstArrowhead.Copy(),

		References: append([]EdgeReference(nil), e.References...),
		Attributes: e.Attributes.Copy(),

		ZIndex: e.ZIndex,
	}
}

// Copy copies for use as the base of a step or scenario.
func (g *Graph) Copy() *Graph {
	g2 := &Graph{
		AST: g.AST,

		Root: g.Root.Copy(),
	}

	absIDMap := make(map[string]*Object, len(g.Objects))
	for _, o := range g.Objects {
		o2 := o.Copy()
		g2.Objects = append(g2.Objects, o2)
		absIDMap[o.AbsID()] = o2
	}

	updateObjectPointers := func(o2 *Object) {
		o2.Graph = g2
		if o2.Parent != nil {
			if o2.Parent.Parent == nil {
				o2.Parent = g2.Root
			} else {
				o2.Parent = absIDMap[o2.Parent.AbsID()]
			}
		}

		for i, ref := range o2.References {
			o2.References[i].ScopeObj = absIDMap[ref.ScopeObj.AbsID()]
		}
		for id, ch := range o2.Children {
			o2.Children[id] = absIDMap[ch.AbsID()]
		}
		for i, ch := range o2.ChildrenArray {
			o2.ChildrenArray[i] = absIDMap[ch.AbsID()]
		}
	}
	updateObjectPointers(g2.Root)
	for _, o2 := range g2.Objects {
		updateObjectPointers(o2)
	}

	for _, e := range g.Edges {
		g2.Edges = append(g2.Edges, e.Copy())
	}
	for _, e2 := range g2.Edges {
		e2.Src = absIDMap[e2.Src.AbsID()]
		e2.Dst = absIDMap[e2.Dst.AbsID()]

		for i, ref := range e2.References {
			e2.References[i].ScopeObj = absIDMap[ref.ScopeObj.AbsID()]
		}
	}

	return g2
}
