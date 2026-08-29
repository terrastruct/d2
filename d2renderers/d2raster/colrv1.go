package d2raster

import (
	"fmt"
	"math"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

// preparedCOLRv1SoftLight is an internal prepared-tree operation. It never
// enters d2scene, whose public blend-mode set remains unchanged.
const preparedCOLRv1SoftLight d2scene.BlendMode = 255

type colrv1TextCompiler struct {
	preflight    *preflight
	nodeID       string
	font         *preparedFont
	buffer       *sfnt.Buffer
	clipBox      *fontface.COLRv1ClipBox
	fontToDevice d2scene.Matrix
}

func (p *preflight) colrv1TextGlyph(nodeID string, font *preparedFont, plan *fontface.COLRv1Plan, origin d2scene.Point, ppem fixed.Int26_6, transform d2scene.Matrix) (*preparedNode, d2scene.Bounds, error) {
	if plan == nil || plan.Root == nil {
		return nil, d2scene.Bounds{}, fmt.Errorf("d2raster: node %q has an empty COLRv1 plan", nodeID)
	}
	if err := p.addPreparedNodes(plan.Usage.PaintNodes); err != nil {
		return nil, d2scene.Bounds{}, fmt.Errorf("d2raster: node %q COLRv1 glyph %d: %w", nodeID, plan.GlyphID, err)
	}
	unitsPerEm := int(font.outline.UnitsPerEm())
	if unitsPerEm <= 0 {
		return nil, d2scene.Bounds{}, fmt.Errorf("d2raster: node %q COLRv1 font has invalid units per em %d", nodeID, unitsPerEm)
	}
	scale := fixedToFloat(ppem) / float64(unitsPerEm)
	fontToText := d2scene.Translate(origin.X, origin.Y).Mul(d2scene.Scale(scale, -scale))
	compiler := colrv1TextCompiler{
		preflight:    p,
		nodeID:       nodeID,
		font:         font,
		buffer:       &sfnt.Buffer{},
		clipBox:      plan.Clip,
		fontToDevice: transform.Mul(fontToText),
	}
	root, err := compiler.paint(plan.Root, d2scene.Identity())
	if err != nil {
		return nil, d2scene.Bounds{}, fmt.Errorf("d2raster: node %q COLRv1 glyph %d: %w", nodeID, plan.GlyphID, err)
	}
	localBounds := d2scene.Bounds{}
	if plan.Clip != nil {
		clip, err := compiler.clip(plan.Clip, d2scene.Identity())
		if err != nil {
			return nil, d2scene.Bounds{}, fmt.Errorf("d2raster: node %q COLRv1 glyph %d clip: %w", nodeID, plan.GlyphID, err)
		}
		if root != nil {
			root.clip = clip
			root.bounds = root.bounds.Intersect(clip.bounds)
			root.contentBounds = root.bounds
		}
		localBounds = transformedCOLRv1ClipBounds(plan.Clip, fontToText)
	}
	return root, localBounds, nil
}

func (c *colrv1TextCompiler) paint(paint fontface.COLRv1Paint, current d2scene.Matrix) (*preparedNode, error) {
	if err := c.preflight.ctx.Err(); err != nil {
		return nil, err
	}
	switch paint := paint.(type) {
	case fontface.COLRv1Layers:
		root := &preparedNode{opacity: 1, blend: d2scene.BlendNormal}
		for index, childPaint := range paint.Paints {
			child, err := c.paint(childPaint, current)
			if err != nil {
				return nil, fmt.Errorf("layer %d: %w", index, err)
			}
			appendPreparedChild(root, child)
		}
		return nonemptyPreparedNode(root), nil
	case fontface.COLRv1Glyph:
		fill, err := c.glyphFill(paint.Paint, current)
		if err != nil {
			return nil, fmt.Errorf("glyph %d fill: %w", paint.GlyphID, err)
		}
		return c.glyph(paint.GlyphID, fill, current)
	case fontface.COLRv1Transform:
		next := current.Mul(colrv1Affine(paint.Matrix))
		if !next.IsFinite() {
			return nil, fmt.Errorf("non-finite composed transform")
		}
		return c.paint(paint.Paint, next)
	case fontface.COLRv1Composite:
		return c.composite(paint, current)
	case fontface.COLRv1Solid, fontface.COLRv1LinearGradient, fontface.COLRv1RadialGradient:
		return c.unboundedFill(paint, current)
	default:
		return nil, fmt.Errorf("unsupported compiled paint %T", paint)
	}
}

// COLRv1 paints are conceptually infinite until a PaintGlyph or clip box
// bounds them. Noto uses that rule inside flag composites. The authenticated
// font guarantees a static clip for every color root, so materialize an
// otherwise-unbounded fill only over that finite box.
func (c *colrv1TextCompiler) unboundedFill(paint fontface.COLRv1Paint, current d2scene.Matrix) (*preparedNode, error) {
	if c.clipBox == nil {
		return nil, fmt.Errorf("unbounded %T paint has no static root clip", paint)
	}
	fill, err := c.glyphFill(paint, current)
	if err != nil {
		return nil, err
	}
	points := []d2scene.Point{
		{X: c.clipBox.XMin, Y: c.clipBox.YMin},
		{X: c.clipBox.XMax, Y: c.clipBox.YMin},
		{X: c.clipBox.XMax, Y: c.clipBox.YMax},
		{X: c.clipBox.XMin, Y: c.clipBox.YMax},
	}
	for range points {
		if err := c.preflight.addPathSegment(); err != nil {
			return nil, err
		}
	}
	primitive, err := c.preflight.finishPrimitive(c.nodeID, []subpath{{points: points, closed: true}}, c.fontToDevice, fill, nil)
	if err != nil {
		return nil, err
	}
	if primitive == nil || primitive.bounds.Empty() {
		return nil, nil
	}
	return &preparedNode{
		opacity: 1, blend: d2scene.BlendNormal, primitive: primitive,
		bounds: primitive.bounds, contentBounds: primitive.bounds,
	}, nil
}

func (c *colrv1TextCompiler) glyph(glyphID uint32, fill *preparedPaint, current d2scene.Matrix) (*preparedNode, error) {
	if glyphID == 0 || glyphID > math.MaxUint16 || int(glyphID) >= c.font.outline.NumGlyphs() {
		return nil, fmt.Errorf("glyph ID %d is out of range", glyphID)
	}
	unitsPerEm := int(c.font.outline.UnitsPerEm())
	segments, err := c.font.outline.LoadGlyph(c.buffer, sfnt.GlyphIndex(glyphID), fixed.I(unitsPerEm), nil)
	if err != nil {
		return nil, fmt.Errorf("load glyph %d: %w", glyphID, err)
	}
	if len(segments) == 0 {
		return nil, nil
	}
	// sfnt.LoadGlyph exposes y-down outlines. COLRv1 paint coordinates are
	// y-up, so the final reflection converts the outline back into the paint
	// coordinate system before the shared font-to-device mapping is applied.
	geometryToDevice := c.fontToDevice.Mul(current).Mul(d2scene.Scale(1, -1))
	paths, err := flattenGlyph(c.preflight.ctx, segments, d2scene.Point{}, flattenTolerance(geometryToDevice), c.preflight.addPathSegment)
	if err != nil {
		return nil, fmt.Errorf("flatten glyph %d: %w", glyphID, err)
	}
	primitive, err := c.preflight.finishPrimitive(c.nodeID, paths, geometryToDevice, fill, nil)
	if err != nil {
		return nil, err
	}
	if primitive == nil || primitive.bounds.Empty() {
		return nil, nil
	}
	return &preparedNode{
		opacity: 1, blend: d2scene.BlendNormal, primitive: primitive,
		bounds: primitive.bounds, contentBounds: primitive.bounds,
	}, nil
}

func (c *colrv1TextCompiler) glyphFill(paint fontface.COLRv1Paint, current d2scene.Matrix) (*preparedPaint, error) {
	if err := c.preflight.ctx.Err(); err != nil {
		return nil, err
	}
	switch paint := paint.(type) {
	case fontface.COLRv1Solid:
		return &preparedPaint{kind: preparedSolidPaint, solid: paint.Color}, nil
	case fontface.COLRv1LinearGradient:
		return prepareCOLRv1LinearGradient(paint, c.fontToDevice.Mul(current))
	case fontface.COLRv1RadialGradient:
		return prepareCOLRv1RadialGradient(paint, c.fontToDevice.Mul(current))
	case fontface.COLRv1Transform:
		next := current.Mul(colrv1Affine(paint.Matrix))
		if !next.IsFinite() {
			return nil, fmt.Errorf("non-finite composed paint transform")
		}
		return c.glyphFill(paint.Paint, next)
	default:
		return nil, fmt.Errorf("unsupported glyph fill %T", paint)
	}
}

func (c *colrv1TextCompiler) composite(composite fontface.COLRv1Composite, current d2scene.Matrix) (*preparedNode, error) {
	backdrop, err := c.paint(composite.Backdrop, current)
	if err != nil {
		return nil, fmt.Errorf("composite backdrop: %w", err)
	}
	source, err := c.paint(composite.Source, current)
	if err != nil {
		return nil, fmt.Errorf("composite source: %w", err)
	}
	switch composite.Mode {
	case fontface.COLRv1CompositeSrcIn:
		if source == nil || backdrop == nil {
			return nil, nil
		}
		root := &preparedNode{
			opacity: 1, blend: d2scene.BlendNormal,
			children:      []*preparedNode{source},
			mask:          &preparedMask{kind: d2scene.MaskAlpha, root: backdrop},
			contentBounds: source.bounds,
			bounds:        source.bounds.Intersect(backdrop.bounds),
		}
		return nonemptyPreparedNode(root), nil
	case fontface.COLRv1CompositeSoftLight:
		if backdrop == nil {
			return source, nil
		}
		if source == nil {
			return backdrop, nil
		}
		source.blend = preparedCOLRv1SoftLight
		// PaintComposite combines its source and backdrop in isolation, then
		// returns that one result to the enclosing COLRv1 surface using ordinary
		// alpha compositing. Without this layer, SoftLight would also sample any
		// lower PaintColrLayers siblings already present in the parent.
		root := &preparedNode{opacity: 1, blend: d2scene.BlendNormal, isolated: true}
		appendPreparedChild(root, backdrop)
		appendPreparedChild(root, source)
		return root, nil
	default:
		return nil, fmt.Errorf("unsupported compiled composite mode %d", composite.Mode)
	}
}

func (c *colrv1TextCompiler) clip(box *fontface.COLRv1ClipBox, current d2scene.Matrix) (*preparedClip, error) {
	if box == nil {
		return nil, nil
	}
	toDevice := c.fontToDevice.Mul(current)
	points := []d2scene.Point{
		toDevice.Point(d2scene.Point{X: box.XMin, Y: box.YMin}),
		toDevice.Point(d2scene.Point{X: box.XMax, Y: box.YMin}),
		toDevice.Point(d2scene.Point{X: box.XMax, Y: box.YMax}),
		toDevice.Point(d2scene.Point{X: box.XMin, Y: box.YMax}),
	}
	for _, point := range points {
		if err := validateRasterPoint(point); err != nil {
			return nil, err
		}
		if err := c.preflight.addPathSegment(); err != nil {
			return nil, err
		}
	}
	paths := []subpath{{points: points, closed: true}}
	return &preparedClip{
		subpaths: paths,
		fillRule: d2scene.NonZero,
		bounds:   subpathPixelBounds(paths, d2scene.Identity(), 0, c.preflight.preparationBounds()),
		edges:    int64(len(points)),
	}, nil
}

func prepareCOLRv1LinearGradient(gradient fontface.COLRv1LinearGradient, toDevice d2scene.Matrix) (*preparedPaint, error) {
	p0 := d2scene.Point{X: gradient.X0, Y: gradient.Y0}
	p1 := d2scene.Point{X: gradient.X1, Y: gradient.Y1}
	p2 := d2scene.Point{X: gradient.X2, Y: gradient.Y2}
	v := d2scene.Point{X: p1.X - p0.X, Y: p1.Y - p0.Y}
	r := d2scene.Point{X: p2.X - p0.X, Y: p2.Y - p0.Y}
	r2 := r.X*r.X + r.Y*r.Y
	if r2 == 0 || !finite(r2) {
		return nil, fmt.Errorf("ill-formed linear gradient rotation vector")
	}
	projection := (v.X*r.X + v.Y*r.Y) / r2
	w := d2scene.Point{X: v.X - projection*r.X, Y: v.Y - projection*r.Y}
	if w.X*w.X+w.Y*w.Y <= geometryEpsilon*geometryEpsilon {
		return nil, fmt.Errorf("ill-formed linear gradient projection")
	}
	stops := colrv1ColorLine(gradient.ColorLine)
	paint, err := prepareLinearGradient(d2scene.LinearGradient{
		Start:     p0,
		End:       d2scene.Point{X: p0.X + w.X, Y: p0.Y + w.Y},
		Stops:     stops,
		Units:     d2scene.UserSpaceOnUse,
		Transform: d2scene.Identity(),
		Spread:    d2scene.SpreadPad,
	}, d2scene.Box{}, toDevice)
	if err != nil {
		return nil, err
	}
	if paint.kind != preparedSolidPaint {
		paint.gradient.colrv1Interpolation = true
	}
	return paint, nil
}

func prepareCOLRv1RadialGradient(gradient fontface.COLRv1RadialGradient, toDevice d2scene.Matrix) (*preparedPaint, error) {
	stops := colrv1ColorLine(gradient.ColorLine)
	paint, err := prepareRadialGradient(d2scene.RadialGradient{
		Focal:       d2scene.Point{X: gradient.X0, Y: gradient.Y0},
		FocalRadius: gradient.Radius0,
		Center:      d2scene.Point{X: gradient.X1, Y: gradient.Y1},
		Radius:      gradient.Radius1,
		Stops:       stops,
		Units:       d2scene.UserSpaceOnUse,
		Transform:   d2scene.Identity(),
		Spread:      d2scene.SpreadPad,
	}, d2scene.Box{}, toDevice)
	if err != nil {
		return nil, err
	}
	if paint.kind != preparedSolidPaint {
		paint.gradient.colrv1Interpolation = true
		paint.gradient.radialAverage = averageCOLRv1GradientColor(paint.gradient.stops)
	}
	return paint, nil
}

func colrv1ColorLine(line fontface.COLRv1ColorLine) []d2scene.GradientStop {
	stops := make([]d2scene.GradientStop, len(line.Stops))
	for index, stop := range line.Stops {
		stops[index] = d2scene.GradientStop{Offset: stop.Offset, Color: stop.Color}
	}
	return stops
}

func colrv1Affine(matrix fontface.COLRv1Affine) d2scene.Matrix {
	return d2scene.Matrix{
		A: matrix.Xx, B: matrix.Yx,
		C: matrix.Xy, D: matrix.Yy,
		E: matrix.Dx, F: matrix.Dy,
	}
}

func appendPreparedChild(parent, child *preparedNode) {
	if parent == nil || child == nil || child.bounds.Empty() {
		return
	}
	parent.children = append(parent.children, child)
	parent.bounds = unionRect(parent.bounds, child.bounds)
	parent.contentBounds = parent.bounds
}

func nonemptyPreparedNode(node *preparedNode) *preparedNode {
	if node == nil || node.bounds.Empty() {
		return nil
	}
	return node
}

func transformedCOLRv1ClipBounds(box *fontface.COLRv1ClipBox, transform d2scene.Matrix) d2scene.Bounds {
	if box == nil {
		return d2scene.Bounds{}
	}
	return d2scene.BoundsFromPoints(
		transform.Point(d2scene.Point{X: box.XMin, Y: box.YMin}),
		transform.Point(d2scene.Point{X: box.XMax, Y: box.YMin}),
		transform.Point(d2scene.Point{X: box.XMax, Y: box.YMax}),
		transform.Point(d2scene.Point{X: box.XMin, Y: box.YMax}),
	)
}
