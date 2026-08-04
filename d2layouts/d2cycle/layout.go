package d2cycle

import (
	"context"
	"math"

	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/geo"
	"oss.terrastruct.com/d2/lib/label"
	"oss.terrastruct.com/util-go/go2"
)

const (
	CONTAINER_PADDING = 60
	DEFAULT_GAP       = 40
)

func Layout(ctx context.Context, g *d2graph.Graph) error {
	obj := g.Root

	cd := layoutCycle(obj)

	if obj.HasLabel() && obj.LabelPosition == nil {
		obj.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
	}
	if obj.Icon != nil && obj.IconPosition == nil {
		obj.IconPosition = go2.Pointer(label.InsideTopLeft.String())
	}

	if obj.Box != nil {
		sizeCycleContainer(obj, cd)
	}

	for _, e := range g.Edges {
		if !e.Src.Parent.IsDescendantOf(obj) && !e.Dst.Parent.IsDescendantOf(obj) {
			continue
		}

		cd.edges = append(cd.edges, e)

		if e.Src.Parent != obj || e.Dst.Parent != obj {
			continue
		}

		e.Route = []*geo.Point{e.Src.Center(), e.Dst.Center()}
		e.TraceToShape(e.Route, 0, 1)
		if e.Label.Value != "" {
			e.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
		}
	}

	if g.Root.IsCycleDiagram() && len(g.Root.ChildrenArray) != 0 {
		g.Root.TopLeft = geo.NewPoint(0, 0)
	}

	if g.RootLevel > 0 {
		cd.shift(
			obj.TopLeft.X+CONTAINER_PADDING,
			obj.TopLeft.Y+CONTAINER_PADDING,
		)
	}
	return nil
}

func layoutCycle(root *d2graph.Object) *cycleDiagram {
	cd := newCycleDiagram(root)

	for _, o := range cd.objects {
		positionedLabel := false
		if o.Icon != nil && o.IconPosition == nil {
			if len(o.ChildrenArray) > 0 {
				o.IconPosition = go2.Pointer(label.OutsideTopLeft.String())
				if o.LabelPosition == nil {
					o.LabelPosition = go2.Pointer(label.OutsideTopRight.String())
					positionedLabel = true
				}
			} else {
				o.IconPosition = go2.Pointer(label.InsideMiddleCenter.String())
			}
		}
		if !positionedLabel && o.HasLabel() && o.LabelPosition == nil {
			if len(o.ChildrenArray) > 0 {
				o.LabelPosition = go2.Pointer(label.OutsideTopCenter.String())
			} else if o.HasOutsideBottomLabel() {
				o.LabelPosition = go2.Pointer(label.OutsideBottomCenter.String())
			} else if o.Icon != nil {
				o.LabelPosition = go2.Pointer(label.InsideTopCenter.String())
			} else {
				o.LabelPosition = go2.Pointer(label.InsideMiddleCenter.String())
			}
		}
	}

	revertAdjustments := cd.sizeForOutsideLabels()
	cd.layout()
	revertAdjustments()

	return cd
}

func (cd *cycleDiagram) layout() {
	count := len(cd.objects)
	if count == 0 {
		return
	}

	if count == 1 {
		obj := cd.objects[0]
		obj.MoveWithDescendantsTo(0, 0)
		cd.width = obj.Width
		cd.height = obj.Height
		return
	}

	var maxWidth, maxHeight float64
	for _, obj := range cd.objects {
		maxWidth = math.Max(maxWidth, obj.Width)
		maxHeight = math.Max(maxHeight, obj.Height)
	}

	maxDimension := math.Max(maxWidth, maxHeight)
	radius := (maxDimension + DEFAULT_GAP) / (2 * math.Sin(math.Pi/float64(count)))
	radius = math.Max(radius, maxDimension)
	center := geo.NewPoint(radius+maxWidth/2, radius+maxHeight/2)

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)

	for i, obj := range cd.objects {
		angle := -math.Pi/2 + 2*math.Pi*float64(i)/float64(count)
		x := center.X + radius*math.Cos(angle) - obj.Width/2
		y := center.Y + radius*math.Sin(angle) - obj.Height/2

		obj.MoveWithDescendantsTo(x, y)

		minX = math.Min(minX, obj.TopLeft.X)
		minY = math.Min(minY, obj.TopLeft.Y)
		maxX = math.Max(maxX, obj.TopLeft.X+obj.Width)
		maxY = math.Max(maxY, obj.TopLeft.Y+obj.Height)
	}

	if minX != 0 || minY != 0 {
		cd.shift(-minX, -minY)
		maxX -= minX
		maxY -= minY
	}

	cd.width = maxX
	cd.height = maxY
}

func sizeCycleContainer(obj *d2graph.Object, cd *cycleDiagram) {
	contentWidth, contentHeight := cd.width, cd.height

	var labelPosition, iconPosition label.Position
	if obj.LabelPosition != nil {
		labelPosition = label.FromString(*obj.LabelPosition)
	}
	if obj.IconPosition != nil {
		iconPosition = label.FromString(*obj.IconPosition)
	}

	_, padding := obj.Spacing()

	var labelWidth, labelHeight float64
	if obj.LabelDimensions.Width > 0 {
		labelWidth = float64(obj.LabelDimensions.Width) + 2*label.PADDING
	}
	if obj.LabelDimensions.Height > 0 {
		labelHeight = float64(obj.LabelDimensions.Height) + 2*label.PADDING
	}

	if labelWidth > 0 {
		switch labelPosition {
		case label.OutsideTopLeft, label.OutsideTopCenter, label.OutsideTopRight,
			label.InsideTopLeft, label.InsideTopCenter, label.InsideTopRight,
			label.InsideBottomLeft, label.InsideBottomCenter, label.InsideBottomRight,
			label.OutsideBottomLeft, label.OutsideBottomCenter, label.OutsideBottomRight:
			overflow := labelWidth - contentWidth
			if overflow > 0 {
				padding.Left += overflow / 2
				padding.Right += overflow / 2
			}
		}
	}
	if labelHeight > 0 {
		switch labelPosition {
		case label.OutsideLeftTop, label.OutsideLeftMiddle, label.OutsideLeftBottom,
			label.InsideMiddleLeft, label.InsideMiddleCenter, label.InsideMiddleRight,
			label.OutsideRightTop, label.OutsideRightMiddle, label.OutsideRightBottom:
			overflow := labelHeight - contentHeight
			if overflow > 0 {
				padding.Top += overflow / 2
				padding.Bottom += overflow / 2
			}
		}
	}

	if iconPosition == label.InsideTopLeft && labelPosition == label.InsideTopCenter {
		iconSize := float64(d2target.MAX_ICON_SIZE) + 2*label.PADDING
		padding.Left = math.Max(padding.Left, iconSize)
		padding.Right = math.Max(padding.Right, iconSize)
		minWidth := 2*iconSize + float64(obj.LabelDimensions.Width) + 2*label.PADDING
		overflow := minWidth - contentWidth
		if overflow > 0 {
			padding.Left = math.Max(padding.Left, overflow/2)
			padding.Right = math.Max(padding.Right, overflow/2)
		}
	}

	padding.Top = math.Max(padding.Top, CONTAINER_PADDING)
	padding.Bottom = math.Max(padding.Bottom, CONTAINER_PADDING)
	padding.Left = math.Max(padding.Left, CONTAINER_PADDING)
	padding.Right = math.Max(padding.Right, CONTAINER_PADDING)

	totalWidth := padding.Left + contentWidth + padding.Right
	totalHeight := padding.Top + contentHeight + padding.Bottom
	obj.SizeToContent(totalWidth, totalHeight, 0, 0)

	s := obj.ToShape()
	innerTL := s.GetInsidePlacement(totalWidth, totalHeight, 0, 0)
	innerBox := s.GetInnerBox()
	var resizeDx, resizeDy float64
	if innerBox.Width > totalWidth {
		resizeDx = (innerBox.Width - totalWidth) / 2
	}
	if innerBox.Height > totalHeight {
		resizeDy = (innerBox.Height - totalHeight) / 2
	}

	dx := -CONTAINER_PADDING + innerTL.X + padding.Left + resizeDx
	dy := -CONTAINER_PADDING + innerTL.Y + padding.Top + resizeDy
	if dx != 0 || dy != 0 {
		cd.shift(dx, dy)
	}
}

func (cd *cycleDiagram) sizeForOutsideLabels() (revert func()) {
	margins := make(map[*d2graph.Object]geo.Spacing)

	for _, o := range cd.objects {
		margin := o.GetMargin()
		margins[o] = margin

		o.Height += margin.Top + margin.Bottom
		o.Width += margin.Left + margin.Right
	}

	return func() {
		for _, o := range cd.objects {
			m, has := margins[o]
			if !has {
				continue
			}
			dy := m.Top + m.Bottom
			dx := m.Left + m.Right
			o.Height -= dy
			o.Width -= dx

			margin := o.GetMargin()
			marginX := margin.Left + margin.Right
			marginY := margin.Top + margin.Bottom
			if marginX < dx {
				o.Width += dx - marginX
			}
			if marginY < dy {
				o.Height += dy - marginY
			}

			if margin.Left > 0 || margin.Top > 0 {
				o.MoveWithDescendants(margin.Left, margin.Top)
			}
		}
	}
}
