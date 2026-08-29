package d2raster

import (
	"fmt"
	"math"
	"math/big"

	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// retainedVectorValidator validates the supported scene structure without
// selecting any device transform. Each retained vector definition is visited
// exactly once; visible Image instances are prepared separately by preflight.
// Cached summaries retain both the longest vector-import chain and composed
// node depth, preserving both limits when a dependency was memoized from an
// earlier sorted root.
type retainedVectorValidator struct {
	preflight           *preflight
	validated           map[d2scene.AssetID]retainedVectorSummary
	active              map[d2scene.AssetID]bool
	nodes               map[*d2scene.Node]bool
	ancestorsInvertible bool
}

// retainedVectorSummary describes work depth independently of any placement.
// importHeight counts vector definitions beginning with the summarized asset;
// nodeHeight counts nodes beginning with its root and continues across normal,
// mask, and vector-image edges.
type retainedVectorSummary struct {
	importHeight int
	nodeHeight   int
}

func (p *preflight) validateRetainedVectorAssets(ids []string) error {
	validator := &retainedVectorValidator{
		preflight:           p,
		validated:           make(map[d2scene.AssetID]retainedVectorSummary, len(p.vectors)),
		active:              make(map[d2scene.AssetID]bool),
		nodes:               make(map[*d2scene.Node]bool),
		ancestorsInvertible: true,
	}
	for _, rawID := range ids {
		if err := p.ctx.Err(); err != nil {
			return err
		}
		id := d2scene.AssetID(rawID)
		if _, ok := p.vectors[id]; !ok {
			continue
		}
		// A VectorAsset cannot be a Document root directly. Its shallowest
		// possible placement has a host Image at depth one and the asset root
		// at depth two, so strict retained validation rejects definitions that
		// cannot fit in any visible placement.
		if _, err := validator.asset(id, 1, 2); err != nil {
			return err
		}
	}
	return nil
}

func (v *retainedVectorValidator) asset(id d2scene.AssetID, importDepth, nodeDepth int) (retainedVectorSummary, error) {
	p := v.preflight
	if err := p.ctx.Err(); err != nil {
		return retainedVectorSummary{}, err
	}
	if importDepth > p.options.MaxImportDepth {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: vector asset %q import depth %d exceeds limit %d", id, importDepth, p.options.MaxImportDepth)
	}
	if v.active[id] {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: cyclic vector asset reference at %q", id)
	}
	if summary, ok := v.validated[id]; ok {
		if err := v.checkSummaryDepths(id, summary, importDepth, nodeDepth); err != nil {
			return retainedVectorSummary{}, err
		}
		return summary, nil
	}
	asset, ok := p.vectors[id]
	if !ok {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: missing vector asset %q", id)
	}
	v.active[id] = true
	defer delete(v.active, id)

	previousInvertibility := v.ancestorsInvertible
	v.ancestorsInvertible = true
	nested, err := v.node(asset.Root, 1, importDepth)
	v.ancestorsInvertible = previousInvertibility
	if err != nil {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: vector asset %q retained validation: %w", id, err)
	}
	summary := retainedVectorSummary{
		importHeight: 1,
		nodeHeight:   nested.nodeHeight,
	}
	if nested.importHeight != 0 {
		summary.importHeight += nested.importHeight
	}
	v.validated[id] = summary
	if err := v.checkSummaryDepths(id, summary, importDepth, nodeDepth); err != nil {
		return retainedVectorSummary{}, err
	}
	return summary, nil
}

func (v *retainedVectorValidator) checkSummaryDepths(id d2scene.AssetID, summary retainedVectorSummary, importDepth, nodeDepth int) error {
	p := v.preflight
	deepestImport := importDepth + summary.importHeight - 1
	if deepestImport > p.options.MaxImportDepth {
		return fmt.Errorf("d2raster: vector asset %q import depth %d exceeds limit %d", id, deepestImport, p.options.MaxImportDepth)
	}
	deepestNode := nodeDepth + summary.nodeHeight - 1
	if deepestNode > p.options.MaxDepth {
		return fmt.Errorf("d2raster: vector asset %q composed node depth %d exceeds limit %d", id, deepestNode, p.options.MaxDepth)
	}
	return nil
}

// node returns relative import and composed node heights for this subtree.
// Definitions are charged only on their first structural visit; cached
// summaries propagate depth across vector-image edges without retraversal.
func (v *retainedVectorValidator) node(node *d2scene.Node, depth, importDepth int) (retainedVectorSummary, error) {
	p := v.preflight
	if err := p.ctx.Err(); err != nil {
		return retainedVectorSummary{}, err
	}
	if node == nil {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: nil retained vector node")
	}
	if depth > p.options.MaxDepth {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: node depth %d exceeds limit %d", depth, p.options.MaxDepth)
	}
	if v.nodes[node] {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: node cycle at %q", node.ID)
	}
	p.nodes++
	if p.nodes > p.options.MaxNodes {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: node count exceeds limit %d", p.options.MaxNodes)
	}
	if !node.Transform.IsFinite() || !finite(node.Opacity) || node.Opacity < 0 || node.Opacity > 1 {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q has invalid transform or opacity", node.ID)
	}
	if !supportedBlendMode(node.Blend) {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q uses invalid or unsupported blend mode %d", node.ID, node.Blend)
	}
	if err := p.chargeFilterWork(node.ID, node.Filters); err != nil {
		return retainedVectorSummary{}, err
	}
	overrides, nodeTransform, err := v.animations(node)
	if err != nil {
		return retainedVectorSummary{}, err
	}
	if _, err := normalizeNodeFilters(p.ctx, node.ID, node.Filters, overrides.dropShadows); err != nil {
		return retainedVectorSummary{}, err
	}
	previousInvertibility := v.ancestorsInvertible
	v.ancestorsInvertible = previousInvertibility && finiteLinearTransformInvertible(nodeTransform)
	defer func() { v.ancestorsInvertible = previousInvertibility }()
	v.nodes[node] = true
	defer delete(v.nodes, node)

	primitiveSummary, primitive, err := v.primitive(node.ID, node.Primitive, overrides, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, err
	}
	summary := retainedVectorSummary{importHeight: primitiveSummary.importHeight, nodeHeight: 1}
	if primitiveSummary.nodeHeight != 0 {
		summary.nodeHeight = 1 + primitiveSummary.nodeHeight
	}
	if !primitive && (overrides.fillColor != nil || overrides.strokeColor != nil || overrides.dashOffset != nil) {
		return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q has a paint or stroke animation but no primitive", node.ID)
	}
	if node.Clip != nil {
		if !node.Clip.Transform.IsFinite() {
			return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q has invalid clip transform", node.ID)
		}
		if node.Clip.Path.FillRule > d2scene.EvenOdd {
			return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q clip has invalid fill rule %d", node.ID, node.Clip.Path.FillRule)
		}
		if err := v.path(node.Clip.Path); err != nil {
			return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q clip: %w", node.ID, err)
		}
	}
	if node.Mask != nil {
		if !node.Mask.Transform.IsFinite() || node.Mask.Type > d2scene.MaskLuminance || node.Mask.Root == nil {
			return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q has invalid mask", node.ID)
		}
		beforeMask := v.ancestorsInvertible
		v.ancestorsInvertible = beforeMask && finiteLinearTransformInvertible(node.Mask.Transform)
		childSummary, err := v.node(node.Mask.Root, depth+1, importDepth)
		v.ancestorsInvertible = beforeMask
		if err != nil {
			return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q mask: %w", node.ID, err)
		}
		summary = mergeRetainedNodeSummary(summary, childSummary)
	}
	for childIndex, child := range node.Children {
		if err := p.ctx.Err(); err != nil {
			return retainedVectorSummary{}, err
		}
		if child == nil {
			return retainedVectorSummary{}, fmt.Errorf("d2raster: node %q child %d is nil", node.ID, childIndex)
		}
		childSummary, err := v.node(child, depth+1, importDepth)
		if err != nil {
			return retainedVectorSummary{}, err
		}
		summary = mergeRetainedNodeSummary(summary, childSummary)
	}
	return summary, nil
}

func mergeRetainedNodeSummary(parent, child retainedVectorSummary) retainedVectorSummary {
	parent.importHeight = maxRetainedHeight(parent.importHeight, child.importHeight)
	parent.nodeHeight = maxRetainedHeight(parent.nodeHeight, 1+child.nodeHeight)
	return parent
}

func mergeRetainedPrimitiveSummary(left, right retainedVectorSummary) retainedVectorSummary {
	return retainedVectorSummary{
		importHeight: maxRetainedHeight(left.importHeight, right.importHeight),
		nodeHeight:   maxRetainedHeight(left.nodeHeight, right.nodeHeight),
	}
}

func (v *retainedVectorValidator) animations(node *d2scene.Node) (animationOverrides, d2scene.Matrix, error) {
	p := v.preflight
	if err := p.chargeAnimationWork(node.ID, node.Animations); err != nil {
		return animationOverrides{}, d2scene.Matrix{}, err
	}
	var overrides animationOverrides
	nodeTransform := node.Transform
	seen := make(map[[2]int]bool, len(node.Animations))
	for trackIndex, track := range node.Animations {
		if err := p.ctx.Err(); err != nil {
			return animationOverrides{}, d2scene.Matrix{}, err
		}
		key := [2]int{int(track.Property), track.TargetIndex}
		if seen[key] {
			return animationOverrides{}, d2scene.Matrix{}, fmt.Errorf("d2raster: node %q has duplicate animation target property %d index %d", node.ID, track.Property, track.TargetIndex)
		}
		seen[key] = true
		value, err := p.animationValueAt(track)
		if err != nil {
			return animationOverrides{}, d2scene.Matrix{}, fmt.Errorf("d2raster: node %q animation %d: %w", node.ID, trackIndex, err)
		}
		if track.Property != d2scene.AnimateDropShadow && track.TargetIndex != 0 {
			return animationOverrides{}, d2scene.Matrix{}, fmt.Errorf("d2raster: node %q animation %d uses non-zero target index for scalar property %d", node.ID, trackIndex, track.Property)
		}
		switch track.Property {
		case d2scene.AnimateOpacity:
			if value.Number < 0 || value.Number > 1 {
				return animationOverrides{}, d2scene.Matrix{}, fmt.Errorf("d2raster: node %q animation %d resolves opacity outside [0,1]", node.ID, trackIndex)
			}
		case d2scene.AnimateTransform:
			nodeTransform = value.Transform
		case d2scene.AnimateStrokeDashOffset:
			offset := value.Number
			overrides.dashOffset = &offset
		case d2scene.AnimateFillColor:
			animatedColor := value.Color
			overrides.fillColor = &animatedColor
		case d2scene.AnimateStrokeColor:
			animatedColor := value.Color
			overrides.strokeColor = &animatedColor
		case d2scene.AnimateDropShadow:
			if overrides.dropShadows == nil {
				overrides.dropShadows = make(map[int]d2scene.DropShadow)
			}
			overrides.dropShadows[track.TargetIndex] = value.Shadow
		default:
			return animationOverrides{}, d2scene.Matrix{}, fmt.Errorf("d2raster: node %q animation %d uses unknown property %d", node.ID, trackIndex, track.Property)
		}
	}
	return overrides, nodeTransform, nil
}

func (v *retainedVectorValidator) primitive(nodeID string, primitive d2scene.Primitive, animation animationOverrides, depth, importDepth int) (retainedVectorSummary, bool, error) {
	switch primitive := primitive.(type) {
	case nil:
		return retainedVectorSummary{}, false, nil
	case d2scene.Rect:
		return v.rect(nodeID, primitive, animation, depth, importDepth)
	case *d2scene.Rect:
		if primitive == nil {
			return retainedVectorSummary{}, false, nil
		}
		return v.rect(nodeID, *primitive, animation, depth, importDepth)
	case d2scene.Ellipse:
		return v.ellipse(nodeID, primitive, animation, depth, importDepth)
	case *d2scene.Ellipse:
		if primitive == nil {
			return retainedVectorSummary{}, false, nil
		}
		return v.ellipse(nodeID, *primitive, animation, depth, importDepth)
	case d2scene.Path:
		return v.scenePath(nodeID, primitive, animation, depth, importDepth)
	case *d2scene.Path:
		if primitive == nil {
			return retainedVectorSummary{}, false, nil
		}
		return v.scenePath(nodeID, *primitive, animation, depth, importDepth)
	case d2scene.TextRun:
		return v.text(nodeID, primitive, animation, depth, importDepth)
	case *d2scene.TextRun:
		if primitive == nil {
			return retainedVectorSummary{}, false, nil
		}
		return v.text(nodeID, *primitive, animation, depth, importDepth)
	case d2scene.Image:
		return v.image(nodeID, primitive, animation, depth, importDepth)
	case *d2scene.Image:
		if primitive == nil {
			return retainedVectorSummary{}, false, nil
		}
		return v.image(nodeID, *primitive, animation, depth, importDepth)
	default:
		return retainedVectorSummary{}, false, fmt.Errorf("d2raster: node %q uses unsupported primitive %T", nodeID, primitive)
	}
}

func (v *retainedVectorValidator) rect(nodeID string, rect d2scene.Rect, animation animationOverrides, depth, importDepth int) (retainedVectorSummary, bool, error) {
	if err := validateBox(rect.Box); err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q rectangle: %w", nodeID, err)
	}
	if !finite(rect.RadiusX) || !finite(rect.RadiusY) || rect.RadiusX < 0 || rect.RadiusY < 0 {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q rectangle has invalid corner radius", nodeID)
	}
	objectBounds := rect.Box
	fillSummary, _, err := v.paint(rect.Fill, animation.fillColor != nil, &objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q rectangle fill: %w", nodeID, err)
	}
	strokeSummary, err := v.stroke(rect.Stroke, animation.strokeColor != nil, animation.dashOffset != nil, &objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q rectangle stroke: %w", nodeID, err)
	}
	return mergeRetainedPrimitiveSummary(fillSummary, strokeSummary), true, nil
}

func (v *retainedVectorValidator) ellipse(nodeID string, ellipse d2scene.Ellipse, animation animationOverrides, depth, importDepth int) (retainedVectorSummary, bool, error) {
	if !finitePoint(ellipse.Center) || !finite(ellipse.RadiusX) || !finite(ellipse.RadiusY) || ellipse.RadiusX < 0 || ellipse.RadiusY < 0 {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q ellipse has invalid geometry", nodeID)
	}
	objectBounds := d2scene.Box{
		X: ellipse.Center.X - ellipse.RadiusX, Y: ellipse.Center.Y - ellipse.RadiusY,
		Width: 2 * ellipse.RadiusX, Height: 2 * ellipse.RadiusY,
	}
	fillSummary, _, err := v.paint(ellipse.Fill, animation.fillColor != nil, &objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q ellipse fill: %w", nodeID, err)
	}
	strokeSummary, err := v.stroke(ellipse.Stroke, animation.strokeColor != nil, animation.dashOffset != nil, &objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q ellipse stroke: %w", nodeID, err)
	}
	return mergeRetainedPrimitiveSummary(fillSummary, strokeSummary), true, nil
}

func (v *retainedVectorValidator) scenePath(nodeID string, path d2scene.Path, animation animationOverrides, depth, importDepth int) (retainedVectorSummary, bool, error) {
	if path.FillRule > d2scene.EvenOdd {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q path has invalid fill rule %d", nodeID, path.FillRule)
	}
	if err := v.path(path); err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q path: %w", nodeID, err)
	}
	var objectBounds *d2scene.Box
	if paintUsesObjectBoundingBox(path.Fill) || strokeUsesObjectBoundingBox(path.Stroke) {
		bounds, err := path.GeometryBounds()
		if err != nil {
			return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q path bounds: %w", nodeID, err)
		}
		box := bounds.Box()
		objectBounds = &box
	}
	fillSummary, _, err := v.paint(path.Fill, animation.fillColor != nil, objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q path fill: %w", nodeID, err)
	}
	strokeSummary, err := v.stroke(path.Stroke, animation.strokeColor != nil, animation.dashOffset != nil, objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q path stroke: %w", nodeID, err)
	}
	return mergeRetainedPrimitiveSummary(fillSummary, strokeSummary), true, nil
}

func (v *retainedVectorValidator) text(nodeID string, text d2scene.TextRun, animation animationOverrides, depth, importDepth int) (retainedVectorSummary, bool, error) {
	p := v.preflight
	if !finitePoint(text.Origin) || !finite(text.Font.Size) || text.Font.Size <= 0 || text.Font.Size > float64(math.MaxInt32)/64 {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text has invalid origin or font size", nodeID)
	}
	if text.Anchor > d2scene.AnchorEnd {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text has invalid anchor %d", nodeID, text.Anchor)
	}
	if !text.Ink.IsFinite() {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text has invalid ink bounds", nodeID)
	}
	if text.Font.Asset == "" {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text has empty font asset ID", nodeID)
	}
	_, ok := p.fonts[text.Font.Asset]
	if !ok {
		if _, exists := p.document.Assets[text.Font.Asset]; exists {
			return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text asset %q is not a usable font", nodeID, text.Font.Asset)
		}
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text references missing font asset %q", nodeID, text.Font.Asset)
	}
	ppem := fixed.Int26_6(math.Round(text.Font.Size * 64))
	glyphs, advance, err := p.positionGlyphs(text, ppem)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text: %w", nodeID, err)
	}
	var objectBounds *d2scene.Box
	if text.Ink.Valid {
		box := text.Ink.Box()
		objectBounds = &box
	} else if paintUsesObjectBoundingBox(text.Fill) || strokeUsesObjectBoundingBox(text.Stroke) {
		box, err := v.textObjectBounds(text, ppem, glyphs, advance)
		if err != nil {
			return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text bounds: %w", nodeID, err)
		}
		objectBounds = &box
	}
	fillSummary, _, err := v.paint(text.Fill, animation.fillColor != nil, objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text fill: %w", nodeID, err)
	}
	strokeSummary, err := v.stroke(text.Stroke, animation.strokeColor != nil, animation.dashOffset != nil, objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q text stroke: %w", nodeID, err)
	}
	return mergeRetainedPrimitiveSummary(fillSummary, strokeSummary), true, nil
}

func (v *retainedVectorValidator) textObjectBounds(text d2scene.TextRun, ppem fixed.Int26_6, glyphs []positionedGlyph, advance float64) (d2scene.Box, error) {
	anchorOffset := 0.0
	switch text.Anchor {
	case d2scene.AnchorStart:
	case d2scene.AnchorMiddle:
		anchorOffset = -advance / 2
	case d2scene.AnchorEnd:
		anchorOffset = -advance
	}
	var bounds d2scene.Bounds
	for index, glyph := range glyphs {
		if err := v.preflight.ctx.Err(); err != nil {
			return d2scene.Box{}, err
		}
		if glyph.empty {
			continue
		}
		glyphBounds, _, err := glyph.font.parsedFace().GlyphRenderBounds(uint32(glyph.id), ppem)
		if err != nil {
			return d2scene.Box{}, fmt.Errorf("load glyph %d (ID %d) render bounds from font asset %q: %w", index, glyph.id, glyph.asset, err)
		}
		originX := text.Origin.X + anchorOffset + glyph.position.X
		originY := text.Origin.Y + glyph.position.Y
		bounds = bounds.Union(d2scene.NewBounds(
			originX+fixedToFloat(glyphBounds.Min.X),
			originY+fixedToFloat(glyphBounds.Min.Y),
			originX+fixedToFloat(glyphBounds.Max.X),
			originY+fixedToFloat(glyphBounds.Max.Y),
		))
	}
	if (text.Underline || text.Strike) && advance > 0 {
		// Only zero-area detection is needed here. Visible preparation derives
		// exact decoration metrics under the real placement transform.
		left := text.Origin.X + anchorOffset
		bounds = bounds.Union(d2scene.NewBounds(left, text.Origin.Y, left+advance, text.Origin.Y+text.Font.Size/16))
	}
	return bounds.Box(), nil
}

func (v *retainedVectorValidator) image(nodeID string, image d2scene.Image, animation animationOverrides, depth, importDepth int) (retainedVectorSummary, bool, error) {
	p := v.preflight
	if animation.fillColor != nil || animation.strokeColor != nil || animation.dashOffset != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q image cannot be targeted by paint or stroke animation", nodeID)
	}
	if image.Asset == "" {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q image has an empty asset ID", nodeID)
	}
	if err := validateBox(image.Box); err != nil {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q image: %w", nodeID, err)
	}
	if image.Aspect.Align > d2scene.AlignXMaxYMax || image.Aspect.Fit > d2scene.AspectSlice {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q image has invalid aspect-ratio policy align=%d fit=%d", nodeID, image.Aspect.Align, image.Aspect.Fit)
	}
	if _, ok := p.rasters[image.Asset]; ok {
		return retainedVectorSummary{}, true, nil
	}
	if asset, ok := p.vectors[image.Asset]; ok {
		// AspectRatioMatrix is independent of outer placement and can itself
		// overflow for incompatible finite source/destination dimensions. Mirror
		// visible preparation for painting instances; zero-area or singular-CTM
		// images deliberately use canonical accounting and paint nothing.
		if image.Box.Width != 0 && image.Box.Height != 0 && v.ancestorsInvertible {
			mapping, err := d2scene.AspectRatioMatrix(asset.ViewBox, image.Box, image.Aspect)
			if err != nil {
				return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q vector asset %q aspect ratio: %w", nodeID, image.Asset, err)
			}
			if !finiteLinearTransformInvertible(mapping) {
				return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q vector asset %q aspect ratio: mapping is singular in the finite numeric domain", nodeID, image.Asset)
			}
		}
		summary, err := v.asset(image.Asset, importDepth+1, depth+1)
		return summary, true, err
	}
	if raw, exists := p.document.Assets[image.Asset]; exists {
		return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q image asset %q is not a raster or vector asset (got %T)", nodeID, image.Asset, raw)
	}
	return retainedVectorSummary{}, true, fmt.Errorf("d2raster: node %q image references missing asset %q", nodeID, image.Asset)
}

func (v *retainedVectorValidator) path(path d2scene.Path) error {
	p := v.preflight
	if len(path.Commands) > p.options.MaxPathCommands-p.pathSegments {
		return fmt.Errorf("path command count exceeds limit %d", p.options.MaxPathCommands)
	}
	p.pathSegments += len(path.Commands)
	haveCurrent := false
	for commandIndex, command := range path.Commands {
		if commandIndex&255 == 0 {
			if err := p.ctx.Err(); err != nil {
				return err
			}
		}
		point := func(value d2scene.Point) bool { return finitePoint(value) }
		switch command.Kind {
		case d2scene.MoveCommand:
			if !point(command.P1) {
				return fmt.Errorf("command %d: non-finite point", commandIndex)
			}
			haveCurrent = true
		case d2scene.LineCommand:
			if !haveCurrent {
				return fmt.Errorf("command %d: line before move", commandIndex)
			}
			if !point(command.P1) {
				return fmt.Errorf("command %d: non-finite point", commandIndex)
			}
		case d2scene.QuadraticCommand:
			if !haveCurrent {
				return fmt.Errorf("command %d: quadratic before move", commandIndex)
			}
			if !point(command.P1) || !point(command.P2) {
				return fmt.Errorf("command %d: non-finite point", commandIndex)
			}
		case d2scene.CubicCommand:
			if !haveCurrent {
				return fmt.Errorf("command %d: cubic before move", commandIndex)
			}
			if !point(command.P1) || !point(command.P2) || !point(command.P3) {
				return fmt.Errorf("command %d: non-finite point", commandIndex)
			}
		case d2scene.ArcCommand:
			if !haveCurrent {
				return fmt.Errorf("command %d: arc before move", commandIndex)
			}
			if !point(command.P1) || !finite(command.RadiusX) || !finite(command.RadiusY) || !finite(command.Rotation) {
				return fmt.Errorf("command %d: non-finite arc", commandIndex)
			}
		case d2scene.CloseCommand:
			if !haveCurrent {
				return fmt.Errorf("command %d: close before move", commandIndex)
			}
		default:
			return fmt.Errorf("command %d: unknown kind %d", commandIndex, command.Kind)
		}
	}
	return nil
}

// paint validates intrinsic paint structure only. Device invertibility and
// objectBoundingBox geometry are checked later for each visible placement.
func (v *retainedVectorValidator) paint(paint d2scene.Paint, animatedColor bool, objectBounds *d2scene.Box, depth, importDepth int) (retainedVectorSummary, bool, error) {
	switch paint := paint.(type) {
	case nil:
		if animatedColor {
			return retainedVectorSummary{}, false, fmt.Errorf("color animation targets missing paint")
		}
		return retainedVectorSummary{}, false, nil
	case d2scene.SolidPaint:
		return retainedVectorSummary{}, true, nil
	case *d2scene.SolidPaint:
		if paint == nil {
			if animatedColor {
				return retainedVectorSummary{}, false, fmt.Errorf("color animation targets missing paint")
			}
			return retainedVectorSummary{}, false, nil
		}
		return retainedVectorSummary{}, true, nil
	case d2scene.LinearGradient:
		if animatedColor {
			return retainedVectorSummary{}, false, fmt.Errorf("color animation targets non-solid paint")
		}
		return retainedVectorSummary{}, true, v.linearGradient(paint, objectBounds)
	case *d2scene.LinearGradient:
		if paint == nil {
			return retainedVectorSummary{}, false, fmt.Errorf("nil linear gradient")
		}
		return v.paint(*paint, animatedColor, objectBounds, depth, importDepth)
	case d2scene.RadialGradient:
		if animatedColor {
			return retainedVectorSummary{}, false, fmt.Errorf("color animation targets non-solid paint")
		}
		return retainedVectorSummary{}, true, v.radialGradient(paint, objectBounds)
	case *d2scene.RadialGradient:
		if paint == nil {
			return retainedVectorSummary{}, false, fmt.Errorf("nil radial gradient")
		}
		return v.paint(*paint, animatedColor, objectBounds, depth, importDepth)
	case d2scene.PatternPaint:
		if animatedColor {
			return retainedVectorSummary{}, false, fmt.Errorf("color animation targets pattern paint")
		}
		summary, err := v.pattern(paint, objectBounds, importDepth)
		return summary, true, err
	case *d2scene.PatternPaint:
		if paint == nil {
			return retainedVectorSummary{}, false, fmt.Errorf("nil pattern paint")
		}
		return v.paint(*paint, animatedColor, objectBounds, depth, importDepth)
	default:
		return retainedVectorSummary{}, false, fmt.Errorf("unsupported paint %T", paint)
	}
}

func (v *retainedVectorValidator) pattern(pattern d2scene.PatternPaint, objectBounds *d2scene.Box, importDepth int) (retainedVectorSummary, error) {
	if err := validateBox(pattern.Tile); err != nil {
		return retainedVectorSummary{}, fmt.Errorf("pattern tile: %w", err)
	}
	if pattern.Tile.Width == 0 || pattern.Tile.Height == 0 {
		return retainedVectorSummary{}, fmt.Errorf("pattern tile has zero width or height")
	}
	if !finite(pattern.Tile.X+pattern.Tile.Width) || !finite(pattern.Tile.Y+pattern.Tile.Height) {
		return retainedVectorSummary{}, fmt.Errorf("pattern tile endpoints are non-finite")
	}
	if pattern.Units > d2scene.UserSpaceOnUse {
		return retainedVectorSummary{}, fmt.Errorf("invalid paint units %d", pattern.Units)
	}
	if !pattern.Transform.IsFinite() {
		return retainedVectorSummary{}, fmt.Errorf("non-finite pattern transform")
	}
	if !finiteLinearTransformInvertible(pattern.Transform) || !v.ancestorsInvertible {
		return retainedVectorSummary{}, fmt.Errorf("singular pattern transform")
	}
	if pattern.Units == d2scene.ObjectBoundingBox && objectBounds != nil {
		if err := validateBox(*objectBounds); err != nil {
			return retainedVectorSummary{}, fmt.Errorf("invalid object bounding box: %w", err)
		}
		if objectBounds.Width == 0 || objectBounds.Height == 0 {
			return retainedVectorSummary{}, fmt.Errorf("object bounding box has zero width or height")
		}
	}
	if pattern.Root == nil {
		return retainedVectorSummary{}, fmt.Errorf("pattern has no root node")
	}
	nextImportDepth := importDepth + 1
	previousInvertibility := v.ancestorsInvertible
	v.ancestorsInvertible = true
	summary, err := v.node(pattern.Root, 1, nextImportDepth)
	v.ancestorsInvertible = previousInvertibility
	if err != nil {
		return retainedVectorSummary{}, fmt.Errorf("pattern root: %w", err)
	}
	// A pattern root starts an independent node-depth domain, but its import
	// depth is inherited by any vector asset reached below it. Pattern edges
	// without a downstream vector do not themselves exercise MaxImportDepth.
	summary.nodeHeight = 0
	if summary.importHeight != 0 {
		summary.importHeight++
	}
	return summary, nil
}

func (v *retainedVectorValidator) linearGradient(gradient d2scene.LinearGradient, objectBounds *d2scene.Box) error {
	if !finitePoint(gradient.Start) || !finitePoint(gradient.End) {
		return fmt.Errorf("linear gradient has non-finite geometry")
	}
	if err := v.gradientCommon(gradient.Units, gradient.Transform, gradient.Stops, gradient.Spread, objectBounds); err != nil {
		return fmt.Errorf("linear gradient: %w", err)
	}
	dx, dy := gradient.End.X-gradient.Start.X, gradient.End.Y-gradient.Start.Y
	if denominator := dx*dx + dy*dy; !finite(denominator) {
		return fmt.Errorf("linear gradient vector is outside the finite numeric domain")
	}
	return nil
}

func (v *retainedVectorValidator) radialGradient(gradient d2scene.RadialGradient, objectBounds *d2scene.Box) error {
	if !finitePoint(gradient.Center) || !finitePoint(gradient.Focal) || !finite(gradient.Radius) || !finite(gradient.FocalRadius) {
		return fmt.Errorf("radial gradient has non-finite geometry")
	}
	if gradient.Radius < 0 || gradient.FocalRadius < 0 {
		return fmt.Errorf("radial gradient has negative radius")
	}
	if err := v.gradientCommon(gradient.Units, gradient.Transform, gradient.Stops, gradient.Spread, objectBounds); err != nil {
		return fmt.Errorf("radial gradient: %w", err)
	}
	if len(gradient.Stops) == 1 {
		return nil
	}
	dx, dy := gradient.Center.X-gradient.Focal.X, gradient.Center.Y-gradient.Focal.Y
	dr := gradient.Radius - gradient.FocalRadius
	if a := dx*dx + dy*dy - dr*dr; !finite(a) || !finite(dr) {
		return fmt.Errorf("radial gradient cone is outside the finite numeric domain")
	}
	return nil
}

func (v *retainedVectorValidator) gradientCommon(units d2scene.PaintUnits, transform d2scene.Matrix, stops []d2scene.GradientStop, spread d2scene.SpreadMethod, objectBounds *d2scene.Box) error {
	if units > d2scene.UserSpaceOnUse {
		return fmt.Errorf("invalid paint units %d", units)
	}
	if !transform.IsFinite() {
		return fmt.Errorf("non-finite gradient transform")
	}
	if !finiteLinearTransformInvertible(transform) {
		return fmt.Errorf("singular gradient transform")
	}
	if !v.ancestorsInvertible {
		return fmt.Errorf("singular gradient transform")
	}
	if units == d2scene.ObjectBoundingBox && objectBounds != nil {
		if err := validateBox(*objectBounds); err != nil {
			return fmt.Errorf("invalid object bounding box: %w", err)
		}
		if objectBounds.Width == 0 || objectBounds.Height == 0 {
			return fmt.Errorf("object bounding box has zero width or height")
		}
	}
	if len(stops) == 0 {
		return fmt.Errorf("gradient has no stops")
	}
	for stopIndex, stop := range stops {
		if stopIndex&255 == 0 {
			if err := v.preflight.ctx.Err(); err != nil {
				return err
			}
		}
		if !finite(stop.Offset) {
			return fmt.Errorf("gradient stop %d has non-finite offset", stopIndex)
		}
	}
	if spread > d2scene.SpreadRepeat {
		return fmt.Errorf("invalid spread method %d", spread)
	}
	return nil
}

func (v *retainedVectorValidator) stroke(stroke *d2scene.Stroke, animatedColor, animatedDashOffset bool, objectBounds *d2scene.Box, depth, importDepth int) (retainedVectorSummary, error) {
	if stroke == nil {
		if animatedColor || animatedDashOffset {
			return retainedVectorSummary{}, fmt.Errorf("stroke animation targets missing stroke")
		}
		return retainedVectorSummary{}, nil
	}
	if !finite(stroke.Width) || stroke.Width < 0 || !finite(stroke.MiterLimit) || !finite(stroke.DashOffset) {
		return retainedVectorSummary{}, fmt.Errorf("invalid stroke")
	}
	paintSummary, paint, err := v.paint(stroke.Paint, animatedColor, objectBounds, depth, importDepth)
	if err != nil {
		return retainedVectorSummary{}, err
	}
	dashTotal := 0.0
	for dashIndex, dash := range stroke.Dashes {
		if dashIndex&255 == 0 {
			if err := v.preflight.ctx.Err(); err != nil {
				return retainedVectorSummary{}, err
			}
		}
		if !finite(dash) || dash <= 0 {
			return retainedVectorSummary{}, fmt.Errorf("invalid stroke dash")
		}
		dashTotal += dash
		if !finite(dashTotal) {
			return retainedVectorSummary{}, fmt.Errorf("invalid stroke dash total")
		}
	}
	if stroke.Width == 0 || !paint {
		return paintSummary, nil
	}
	switch stroke.Cap {
	case d2scene.CapButt, d2scene.CapRound, d2scene.CapSquare:
	default:
		return retainedVectorSummary{}, fmt.Errorf("unsupported line cap %d", stroke.Cap)
	}
	switch stroke.Join {
	case d2scene.JoinMiter, d2scene.JoinRound, d2scene.JoinBevel:
	default:
		return retainedVectorSummary{}, fmt.Errorf("unsupported line join %d", stroke.Join)
	}
	if stroke.Join == d2scene.JoinMiter && stroke.MiterLimit != 0 && stroke.MiterLimit < 1 {
		return retainedVectorSummary{}, fmt.Errorf("invalid miter limit %g", stroke.MiterLimit)
	}
	return paintSummary, nil
}

func paintUsesObjectBoundingBox(paint d2scene.Paint) bool {
	switch paint := paint.(type) {
	case d2scene.LinearGradient:
		return paint.Units == d2scene.ObjectBoundingBox
	case *d2scene.LinearGradient:
		return paint != nil && paint.Units == d2scene.ObjectBoundingBox
	case d2scene.RadialGradient:
		return paint.Units == d2scene.ObjectBoundingBox
	case *d2scene.RadialGradient:
		return paint != nil && paint.Units == d2scene.ObjectBoundingBox
	case d2scene.PatternPaint:
		return paint.Units == d2scene.ObjectBoundingBox
	case *d2scene.PatternPaint:
		return paint != nil && paint.Units == d2scene.ObjectBoundingBox
	default:
		return false
	}
}

func strokeUsesObjectBoundingBox(stroke *d2scene.Stroke) bool {
	return stroke != nil && paintUsesObjectBoundingBox(stroke.Paint)
}

// finiteLinearTransformInvertible tests rank without constructing the inverse.
// An exact rational determinant preserves the full float64 exponent range and
// cannot misclassify a singular dyadic matrix through normalization rounding.
func finiteLinearTransformInvertible(transform d2scene.Matrix) bool {
	if !transform.IsFinite() {
		return false
	}
	if (transform.A == 0 && transform.C == 0) || (transform.B == 0 && transform.D == 0) {
		return false
	}
	leftFloat := transform.A * transform.D
	rightFloat := transform.B * transform.C
	// If both correctly-rounded products are finite and differ, their exact
	// products cannot have been equal. Ambiguous equal/overflow/underflow cases
	// fall through to the exact determinant.
	if finite(leftFloat) && finite(rightFloat) && leftFloat != rightFloat {
		return true
	}
	var exactA, exactB, exactC, exactD big.Rat
	exactA.SetFloat64(transform.A)
	exactB.SetFloat64(transform.B)
	exactC.SetFloat64(transform.C)
	exactD.SetFloat64(transform.D)
	var left, right big.Rat
	left.Mul(&exactA, &exactD)
	right.Mul(&exactB, &exactC)
	return left.Cmp(&right) != 0
}

// finiteAffineInverse computes a full inverse without allowing determinant
// overflow or underflow to misclassify an invertible float64 matrix. The fast
// path covers ordinary transforms; exact rationals handle extreme exponents.
func finiteAffineInverse(transform d2scene.Matrix) (d2scene.Matrix, bool, error) {
	if !transform.IsFinite() {
		return d2scene.Matrix{}, false, fmt.Errorf("non-finite transform")
	}
	if inverse, err := transform.Inverse(); err == nil && inverse.IsFinite() {
		return inverse, true, nil
	}

	var a, b, c, d, e, f big.Rat
	a.SetFloat64(transform.A)
	b.SetFloat64(transform.B)
	c.SetFloat64(transform.C)
	d.SetFloat64(transform.D)
	e.SetFloat64(transform.E)
	f.SetFloat64(transform.F)
	var left, right, determinant big.Rat
	left.Mul(&a, &d)
	right.Mul(&b, &c)
	determinant.Sub(&left, &right)
	if determinant.Sign() == 0 {
		return d2scene.Matrix{}, false, nil
	}
	quotient := func(numerator *big.Rat) (float64, error) {
		var value big.Rat
		value.Quo(numerator, &determinant)
		result, _ := value.Float64()
		if !finite(result) {
			return 0, fmt.Errorf("inverse is outside the finite numeric domain")
		}
		return result, nil
	}
	negated := func(value *big.Rat) *big.Rat {
		result := new(big.Rat)
		return result.Neg(value)
	}
	inverseA, err := quotient(&d)
	if err != nil {
		return d2scene.Matrix{}, false, err
	}
	inverseB, err := quotient(negated(&b))
	if err != nil {
		return d2scene.Matrix{}, false, err
	}
	inverseC, err := quotient(negated(&c))
	if err != nil {
		return d2scene.Matrix{}, false, err
	}
	inverseD, err := quotient(&a)
	if err != nil {
		return d2scene.Matrix{}, false, err
	}
	var cf, de, inverseENumerator big.Rat
	cf.Mul(&c, &f)
	de.Mul(&d, &e)
	inverseENumerator.Sub(&cf, &de)
	inverseE, err := quotient(&inverseENumerator)
	if err != nil {
		return d2scene.Matrix{}, false, err
	}
	var be, af, inverseFNumerator big.Rat
	be.Mul(&b, &e)
	af.Mul(&a, &f)
	inverseFNumerator.Sub(&be, &af)
	inverseF, err := quotient(&inverseFNumerator)
	if err != nil {
		return d2scene.Matrix{}, false, err
	}
	return d2scene.Matrix{
		A: inverseA, B: inverseB, C: inverseC, D: inverseD, E: inverseE, F: inverseF,
	}, true, nil
}

func maxRetainedHeight(left, right int) int {
	if left > right {
		return left
	}
	return right
}
