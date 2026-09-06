package d2scenebuild

import (
	"fmt"
	"image/color"
	"time"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2target"
	libcolor "github.com/d2lang/d2/lib/color"
	"github.com/d2lang/d2/lib/shape"
)

const (
	animatedShapeDuration           = time.Second
	animatedShapeFilterExpansion    = 12.6 + 3*25.2
	animatedShapeMaxFilterExpansion = (12.6 + 3*25.2) + (7.56 + 3*15.12) + 2
)

var (
	svgStaticShapeShadow = d2scene.DropShadow{
		OffsetX: 3,
		OffsetY: 5,
		Color:   color.NRGBA{R: 0x3d, G: 0x45, B: 0x74, A: 102},
	}
	// Filter Effects Module Level 1 defines drop-shadow()'s optional third
	// length as the Gaussian standard deviation, unlike box-shadow's blur
	// radius. The CSS lengths therefore map directly to scene Sigma values.
	animatedShapeShadowMidpoints = [...]d2scene.DropShadow{
		{
			OffsetY: 12.6,
			SigmaX:  25.2,
			SigmaY:  25.2,
			Color:   color.NRGBA{R: 50, G: 50, B: 93, A: 64},
		},
		{
			OffsetY: 7.56,
			SigmaX:  15.12,
			SigmaY:  15.12,
			Color:   color.NRGBA{A: 26},
		},
	}
	// CSS shadow colors interpolate in premultiplied space. The scene IR's
	// NRGBA interpolation is component-wise, so retain each midpoint RGB at
	// alpha zero to produce the same fade without changing shared IR behavior.
	animatedShapeShadowEndpoints = [...]d2scene.DropShadow{
		{Color: color.NRGBA{R: 50, G: 50, B: 93}},
		{},
	}
	// Scene validation accounts declared filter geometry independently from
	// animation tracks. Keep the maximum animated deviations/offsets in the
	// transparent declarations so MaxFilterExpansion covers midpoint work;
	// AnimateDropShadow overrides them with the exact endpoint at render time.
	animatedShapeFilterDeclarations = [...]d2scene.DropShadow{
		{
			OffsetY: 12.6,
			SigmaX:  25.2,
			SigmaY:  25.2,
			Color:   animatedShapeShadowEndpoints[0].Color,
		},
		{
			OffsetY: 7.56,
			SigmaX:  15.12,
			SigmaY:  15.12,
			Color:   animatedShapeShadowEndpoints[1].Color,
		},
	}
)

// appendShapeGeometry nests shape geometry so static shadow and blend do not
// affect sibling icons or ordinary labels. Structured shapes remain wholly
// inside because their text and geometry form one unit.
func (b *builder) appendShapeGeometry(group *d2scene.Node, targetShape d2target.Shape, children []*d2scene.Node) {
	shadow := targetShape.Shadow && shapeSupportsShadow(targetShape.Type)
	if !shadow && !targetShape.Blend {
		group.Children = append(group.Children, children...)
		return
	}
	inner := d2scene.NewNode(nil)
	inner.ID = targetShape.ID + ":shape"
	inner.Children = append(inner.Children, children...)
	if shadow {
		// The visible static shadow is a sharp offset/flood composite.
		inner.Filters = []d2scene.Filter{svgStaticShapeShadow}
	}
	if targetShape.Blend {
		inner.Blend = d2scene.BlendMultiply
		inner.Opacity = .5
	}
	group.Children = append(group.Children, inner)
}

func shapeSupportsShadow(shapeType string) bool {
	switch shapeType {
	case d2target.ShapeText, d2target.ShapeCode, d2target.ShapeClass, d2target.ShapeSQLTable:
		return false
	default:
		return true
	}
}

func configureAnimatedShape(group *d2scene.Node) {
	group.Filters = []d2scene.Filter{animatedShapeFilterDeclarations[0], animatedShapeFilterDeclarations[1]}
	group.Animations = []d2scene.Track{
		shapeEffectTrack(
			d2scene.AnimateTransform,
			0,
			d2scene.TransformValue(d2scene.Identity()),
			d2scene.TransformValue(d2scene.Translate(0, -4)),
			d2scene.TransformValue(d2scene.Identity()),
		),
		shapeEffectTrack(
			d2scene.AnimateDropShadow,
			0,
			d2scene.ShadowValue(animatedShapeShadowEndpoints[0]),
			d2scene.ShadowValue(animatedShapeShadowMidpoints[0]),
			d2scene.ShadowValue(animatedShapeShadowEndpoints[0]),
		),
		shapeEffectTrack(
			d2scene.AnimateDropShadow,
			1,
			d2scene.ShadowValue(animatedShapeShadowEndpoints[1]),
			d2scene.ShadowValue(animatedShapeShadowMidpoints[1]),
			d2scene.ShadowValue(animatedShapeShadowEndpoints[1]),
		),
	}
}

func (b *builder) finishShape(group *d2scene.Node, targetShape d2target.Shape) (*d2scene.Node, error) {
	if !targetShape.Animated || len(group.Children) == 0 {
		return group, nil
	}
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	visualTarget := targetShape
	visualTarget.Link = ""
	visualTarget.PrettyLink = ""
	visualTarget.Tooltip = ""
	visualTarget.TooltipPosition = ""
	topLeft, bottomRight := (d2target.Diagram{Shapes: []d2target.Shape{visualTarget}}).BoundingBox()
	if bottomRight.X <= topLeft.X || bottomRight.Y <= topLeft.Y {
		return group, nil
	}
	// A CSS filter creates an isolated group even when its current value is a
	// transparent identity. d2raster correctly prunes transparent shadows, so
	// retain that stacking context with a no-op clip applied after the filter
	// chain. Its finite expansion covers both sequential shadows' complete
	// 3-sigma support plus one pixel of kernel rounding per filter.
	bounds := d2scene.NewBounds(float64(topLeft.X), float64(topLeft.Y), float64(bottomRight.X), float64(bottomRight.Y))
	group.Clip = boxClip(bounds.Expand(animatedShapeMaxFilterExpansion, animatedShapeMaxFilterExpansion).Box())
	return group, b.ctx.Err()
}

func shapeEffectTrack(property d2scene.AnimationProperty, targetIndex int, start, midpoint, end d2scene.AnimationValue) d2scene.Track {
	return d2scene.Track{
		Property:    property,
		TargetIndex: targetIndex,
		Duration:    animatedShapeDuration,
		Repeat:      true,
		Keyframes: []d2scene.Keyframe{
			{Offset: 0, Value: start},
			{Offset: .5, Value: midpoint},
			{Offset: 1, Value: end},
		},
	}
}

func validateShapeEffects(object string, targetShape d2target.Shape) error {
	if targetShape.ThreeDee {
		switch targetShape.Type {
		case "", d2target.ShapeRectangle, d2target.ShapeSquare, d2target.ShapeHexagon:
		default:
			return unsupported(object, "3d for shape type "+targetShape.Type)
		}
	}
	// 3D geometry takes precedence over multiple and double-border effects.
	if !targetShape.ThreeDee && targetShape.DoubleBorder {
		switch targetShape.Type {
		case "", d2target.ShapeRectangle, d2target.ShapeSquare, d2target.ShapeOval, d2target.ShapeCircle:
		default:
			return unsupported(object, "double border for shape type "+targetShape.Type)
		}
		minimum := 2 * d2target.INNER_BORDER_OFFSET
		if targetShape.Width < minimum {
			return invalidField(object, "width", targetShape.Width, fmt.Sprintf("must be at least %d for double border", minimum))
		}
		if targetShape.Height < minimum {
			return invalidField(object, "height", targetShape.Height, fmt.Sprintf("must be at least %d for double border", minimum))
		}
	}
	if !targetShape.ThreeDee && targetShape.Multiple {
		switch targetShape.Type {
		case d2target.ShapeText, d2target.ShapeCode, d2target.ShapeClass, d2target.ShapeSQLTable, d2target.ShapeImage:
			return unsupported(object, "multiple for shape type "+targetShape.Type)
		}
	}

	return validateShapeEffectIntegerBounds(object, targetShape)
}

func validateShapeEffectIntegerBounds(object string, targetShape d2target.Shape) error {
	if !targetShape.ThreeDee && !targetShape.Multiple && !targetShape.Shadow {
		return nil
	}
	maxInt := int64(^uint(0) >> 1)
	minInt := -maxInt - 1
	check := func(field string, initial int, deltas ...int) error {
		value := int64(initial)
		for _, delta := range deltas {
			var ok bool
			value, ok = checkedAdd(value, int64(delta), minInt, maxInt)
			if !ok {
				return invalidField(object, field, initial, "must fit raster effect bounds arithmetic")
			}
		}
		return nil
	}
	if targetShape.ThreeDee {
		offsetY := d2target.THREE_DEE_OFFSET
		if targetShape.Type == d2target.ShapeHexagon {
			offsetY /= 2
		}
		if err := check("pos.x", targetShape.Pos.X, targetShape.Width, d2target.THREE_DEE_OFFSET, targetShape.StrokeWidth); err != nil {
			return err
		}
		if err := check("pos.y", targetShape.Pos.Y, -offsetY, -targetShape.StrokeWidth); err != nil {
			return err
		}
	}
	if !targetShape.ThreeDee && targetShape.Multiple {
		if err := check("pos.x", targetShape.Pos.X, targetShape.Width, d2target.MULTIPLE_OFFSET, targetShape.StrokeWidth); err != nil {
			return err
		}
		if err := check("pos.y", targetShape.Pos.Y, -d2target.MULTIPLE_OFFSET, -targetShape.StrokeWidth); err != nil {
			return err
		}
	}
	if targetShape.Shadow {
		halfStroke := targetShape.StrokeWidth/2 + targetShape.StrokeWidth%2
		if err := check("pos.x", targetShape.Pos.X, targetShape.Width, halfStroke, d2target.SHADOW_SIZE_X); err != nil {
			return err
		}
		if err := check("pos.y", targetShape.Pos.Y, targetShape.Height, halfStroke, d2target.SHADOW_SIZE_Y); err != nil {
			return err
		}
	}
	return nil
}

func (b *builder) buildShapeEffects(targetShape d2target.Shape, fill d2scene.Paint, stroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	if targetShape.ThreeDee {
		if targetShape.Type == d2target.ShapeHexagon {
			return b.buildThreeDeeHexagon(targetShape, fill, stroke)
		}
		return b.buildThreeDeeRectangle(targetShape, fill, stroke)
	}
	if targetShape.DoubleBorder {
		return b.buildDoubleBorderShape(targetShape, fill, stroke)
	}
	if targetShape.Multiple {
		multiple := targetShape
		multiple.Pos.X += d2target.MULTIPLE_OFFSET
		multiple.Pos.Y -= d2target.MULTIPLE_OFFSET
		duplicate, err := b.buildOrdinaryShapeOutline(multiple, fill, stroke, targetShape.ID+":multiple")
		if err != nil {
			return nil, err
		}
		main, err := b.buildOrdinaryShapeOutline(targetShape, fill, stroke, targetShape.ID+":main")
		if err != nil {
			return nil, err
		}
		return append(duplicate, main...), nil
	}
	return nil, fmt.Errorf("scene: shape %q has no shape effect", targetShape.ID)
}

func (b *builder) buildOrdinaryShapeOutline(targetShape d2target.Shape, fill d2scene.Paint, stroke *d2scene.Stroke, idPrefix string) ([]*d2scene.Node, error) {
	box := d2scene.Box{
		X:      float64(targetShape.Pos.X),
		Y:      float64(targetShape.Pos.Y),
		Width:  float64(targetShape.Width),
		Height: float64(targetShape.Height),
	}
	var nodes []*d2scene.Node
	switch targetShape.Type {
	case d2target.ShapeText:
		// Text-only shapes deliberately have no outline primitive.
		return nil, nil
	case d2target.ShapeOval, d2target.ShapeCircle:
		nodes = append(nodes, d2scene.NewNode(d2scene.Ellipse{
			Center:  d2scene.Point{X: box.X + box.Width/2, Y: box.Y + box.Height/2},
			RadiusX: box.Width / 2,
			RadiusY: box.Height / 2,
			Fill:    fill,
			Stroke:  stroke,
		}))
	case d2target.ShapeRectangle, d2target.ShapeSquare, d2target.ShapeSequenceDiagram, d2target.ShapeHierarchy, "":
		radius := clampBorderRadius(float64(targetShape.BorderRadius), box.Width, box.Height)
		nodes = append(nodes, d2scene.NewNode(d2scene.Rect{
			Box:     box,
			RadiusX: radius,
			RadiusY: radius,
			Fill:    fill,
			Stroke:  stroke,
		}))
	default:
		geometry := targetGeometry(targetShape)
		paths := shape.GetPathCommands(geometry)
		if len(paths) == 0 {
			return nil, unsupported(fmt.Sprintf("shape %q", targetShape.ID), "typed geometry for "+targetShape.Type)
		}
		for pathIndex, commands := range paths {
			path, err := scenePath(commands, fill, stroke)
			if err != nil {
				return nil, fmt.Errorf("scene: shape %q path %d: %w", targetShape.ID, pathIndex, err)
			}
			node := d2scene.NewNode(path)
			if idPrefix != "" {
				if len(paths) == 1 {
					node.ID = idPrefix
				} else {
					node.ID = fmt.Sprintf("%s:path:%d", idPrefix, pathIndex)
				}
			}
			nodes = append(nodes, node)
		}
	}
	if idPrefix != "" && len(nodes) == 1 && nodes[0].ID == "" {
		nodes[0].ID = idPrefix
	}
	return nodes, nil
}

func (b *builder) buildDoubleBorderShape(targetShape d2target.Shape, fill d2scene.Paint, stroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	var nodes []*d2scene.Node
	if targetShape.Multiple {
		multiple := targetShape
		multiple.Pos.X += d2target.MULTIPLE_OFFSET
		multiple.Pos.Y -= d2target.MULTIPLE_OFFSET
		nodes = append(nodes, doubleBorderPair(multiple, fill, fill, stroke, targetShape.ID+":multiple")...)
	}
	transparent, err := b.paint("transparent", fmt.Sprintf("shape %q double-border inner fill", targetShape.ID))
	if err != nil {
		return nil, err
	}
	innerFill := transparent
	if targetShape.Type == d2target.ShapeOval || targetShape.Type == d2target.ShapeCircle {
		innerFill = fill
	}
	nodes = append(nodes, doubleBorderPair(targetShape, fill, innerFill, stroke, targetShape.ID+":double-border")...)
	return nodes, nil
}

func doubleBorderPair(targetShape d2target.Shape, outerFill, innerFill d2scene.Paint, stroke *d2scene.Stroke, idPrefix string) []*d2scene.Node {
	outerBox := d2scene.Box{
		X:      float64(targetShape.Pos.X),
		Y:      float64(targetShape.Pos.Y),
		Width:  float64(targetShape.Width),
		Height: float64(targetShape.Height),
	}
	offset := float64(d2target.INNER_BORDER_OFFSET)
	innerBox := d2scene.Box{
		X:      outerBox.X + offset,
		Y:      outerBox.Y + offset,
		Width:  outerBox.Width - 2*offset,
		Height: outerBox.Height - 2*offset,
	}
	var outer, inner *d2scene.Node
	if targetShape.Type == d2target.ShapeOval || targetShape.Type == d2target.ShapeCircle {
		outer = d2scene.NewNode(d2scene.Ellipse{
			Center:  d2scene.Point{X: outerBox.X + outerBox.Width/2, Y: outerBox.Y + outerBox.Height/2},
			RadiusX: outerBox.Width / 2,
			RadiusY: outerBox.Height / 2,
			Fill:    outerFill,
			Stroke:  stroke,
		})
		inner = d2scene.NewNode(d2scene.Ellipse{
			Center:  d2scene.Point{X: innerBox.X + innerBox.Width/2, Y: innerBox.Y + innerBox.Height/2},
			RadiusX: innerBox.Width / 2,
			RadiusY: innerBox.Height / 2,
			Fill:    innerFill,
			Stroke:  stroke,
		})
	} else {
		outerRadius := clampBorderRadius(float64(targetShape.BorderRadius), outerBox.Width, outerBox.Height)
		innerRadius := clampBorderRadius(float64(targetShape.BorderRadius), innerBox.Width, innerBox.Height)
		outer = d2scene.NewNode(d2scene.Rect{
			Box: outerBox, RadiusX: outerRadius, RadiusY: outerRadius, Fill: outerFill, Stroke: stroke,
		})
		inner = d2scene.NewNode(d2scene.Rect{
			Box: innerBox, RadiusX: innerRadius, RadiusY: innerRadius, Fill: innerFill, Stroke: stroke,
		})
	}
	outer.ID = idPrefix + ":outer"
	inner.ID = idPrefix + ":inner"
	return []*d2scene.Node{outer, inner}
}

func (b *builder) buildThreeDeeRectangle(targetShape d2target.Shape, fill d2scene.Paint, stroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	x := float64(targetShape.Pos.X)
	y := float64(targetShape.Pos.Y)
	w := float64(targetShape.Width)
	h := float64(targetShape.Height)
	offset := float64(d2target.THREE_DEE_OFFSET)

	main := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{X: x, Y: y, Width: w, Height: h},
		Fill: fill,
	})
	main.ID = targetShape.ID + ":3d-main"

	dark, err := b.threeDeeSidePaint(targetShape)
	if err != nil {
		return nil, err
	}
	sides := d2scene.NewNode(polygonPath([]d2scene.Point{
		{X: x, Y: y},
		{X: x + offset, Y: y - offset},
		{X: x + w + offset, Y: y - offset},
		{X: x + w + offset, Y: y + h - offset},
		{X: x + w, Y: y + h},
		{X: x + w, Y: y},
	}, dark, nil))
	sides.ID = targetShape.ID + ":3d-sides"

	border := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(x, y),
			d2scene.LineTo(x+offset, y-offset),
			d2scene.LineTo(x+w+offset, y-offset),
			d2scene.LineTo(x+w+offset, y+h-offset),
			d2scene.LineTo(x+w, y+h),
			d2scene.LineTo(x, y+h),
			d2scene.LineTo(x, y),
			d2scene.LineTo(x+w, y),
			d2scene.LineTo(x+w, y+h),
			d2scene.MoveTo(x+w, y),
			d2scene.LineTo(x+w+offset, y-offset),
		},
		Stroke: stroke,
	})
	border.ID = targetShape.ID + ":3d-border"
	return []*d2scene.Node{main, sides, border}, nil
}

func (b *builder) buildThreeDeeHexagon(targetShape d2target.Shape, fill d2scene.Paint, stroke *d2scene.Stroke) ([]*d2scene.Node, error) {
	x := targetShape.Pos.X
	y := targetShape.Pos.Y
	w := targetShape.Width
	h := targetShape.Height
	scale := func(n int, factor float64) int { return int(float64(n) * factor) }
	quarterX := scale(w, 0.25)
	threeQuarterX := scale(w, 0.75)
	middleY := scale(h, 43.6/87.3)
	yOffset := d2target.THREE_DEE_OFFSET / 2
	absolute := func(px, py int) d2scene.Point {
		return d2scene.Point{X: float64(x + px), Y: float64(y + py)}
	}

	mainPoints := []d2scene.Point{
		absolute(quarterX, 0),
		absolute(threeQuarterX, 0),
		absolute(w, middleY),
		absolute(threeQuarterX, h),
		absolute(quarterX, h),
		absolute(0, middleY),
	}
	main := d2scene.NewNode(polygonPath(mainPoints, fill, nil))
	main.ID = targetShape.ID + ":3d-main"

	dark, err := b.threeDeeSidePaint(targetShape)
	if err != nil {
		return nil, err
	}
	sidePoints := []d2scene.Point{
		absolute(quarterX+d2target.THREE_DEE_OFFSET, -yOffset),
		absolute(threeQuarterX+d2target.THREE_DEE_OFFSET, -yOffset),
		absolute(w+d2target.THREE_DEE_OFFSET, middleY-yOffset),
		absolute(threeQuarterX+d2target.THREE_DEE_OFFSET, h-yOffset),
		absolute(threeQuarterX, h),
		absolute(w, middleY),
		absolute(threeQuarterX, 0),
		absolute(quarterX, 0),
	}
	sides := d2scene.NewNode(polygonPath(sidePoints, dark, nil))
	sides.ID = targetShape.ID + ":3d-sides"

	borderCommands := []d2scene.PathCommand{d2scene.MoveTo(mainPoints[0].X, mainPoints[0].Y)}
	for _, point := range []d2scene.Point{
		sidePoints[0], sidePoints[1], sidePoints[2], sidePoints[3],
		mainPoints[3], mainPoints[4], mainPoints[5], mainPoints[0], mainPoints[1], mainPoints[2], mainPoints[3],
	} {
		borderCommands = append(borderCommands, d2scene.LineTo(point.X, point.Y))
	}
	for _, point := range []d2scene.Point{mainPoints[1], mainPoints[2], mainPoints[3]} {
		borderCommands = append(borderCommands,
			d2scene.MoveTo(point.X, point.Y),
			d2scene.LineTo(point.X+float64(d2target.THREE_DEE_OFFSET), point.Y-float64(yOffset)),
		)
	}
	border := d2scene.NewNode(d2scene.Path{Commands: borderCommands, Stroke: stroke})
	border.ID = targetShape.ID + ":3d-border"
	return []*d2scene.Node{main, sides, border}, nil
}

func (b *builder) threeDeeSidePaint(targetShape d2target.Shape) (d2scene.Paint, error) {
	darker, err := libcolor.Darken(targetShape.Fill)
	if err != nil {
		darker = targetShape.Fill
	}
	return b.paint(darker, fmt.Sprintf("shape %q 3d side fill", targetShape.ID))
}

func polygonPath(points []d2scene.Point, fill d2scene.Paint, stroke *d2scene.Stroke) d2scene.Path {
	path := d2scene.Path{Fill: fill, Stroke: stroke}
	if len(points) == 0 {
		return path
	}
	path.Commands = append(path.Commands, d2scene.MoveTo(points[0].X, points[0].Y))
	for _, point := range points[1:] {
		path.Commands = append(path.Commands, d2scene.LineTo(point.X, point.Y))
	}
	path.Commands = append(path.Commands, d2scene.ClosePath())
	return path
}
