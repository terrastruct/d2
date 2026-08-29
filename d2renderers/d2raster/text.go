package d2raster

import (
	"context"
	"fmt"
	"image/color"
	"math"

	gotextfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

type positionedGlyph struct {
	id       sfnt.GlyphIndex
	font     *preparedFont
	asset    d2scene.AssetID
	position d2scene.Point
	empty    bool
	source   rune
	sourceAt int
}

// preparedFont pairs the outline reader used by the raster kernel with a
// render-local shaping face. go-text Face values maintain mutable lookup
// caches, so a RenderSession shares only the immutable underlying Font and
// creates a fresh Face for each preflight.
type preparedFont struct {
	face        *fontface.ParsedFace
	outline     *sfnt.Font
	shaping     *gotextfont.Face
	colrv1Plans map[uint32]preparedCOLRv1Plan
}

type preparedCOLRv1Plan struct {
	plan  *fontface.COLRv1Plan
	found bool
	err   error
}

func (f *preparedFont) parsedFace() *fontface.ParsedFace {
	if f == nil {
		return nil
	}
	return f.face
}

// bundledCOLRv1Plan caches immutable paint plans on the preflight-local font
// wrapper. A RenderSession shares only the underlying parsed font and creates a
// new preparedFont for every preflight, so this map never needs synchronization.
func (f *preparedFont) bundledCOLRv1Plan(glyphID uint32) (*fontface.COLRv1Plan, bool, error) {
	if f == nil {
		return nil, false, fmt.Errorf("d2raster: nil prepared font")
	}
	if f.face == nil || !f.face.IsBundledNotoColorEmoji() {
		return nil, false, nil
	}
	if cached, ok := f.colrv1Plans[glyphID]; ok {
		return cached.plan, cached.found, cached.err
	}
	plan, found, err := f.face.CompileBundledNotoColorEmojiCOLRv1Plan(glyphID)
	if f.colrv1Plans == nil {
		f.colrv1Plans = make(map[uint32]preparedCOLRv1Plan)
	}
	f.colrv1Plans[glyphID] = preparedCOLRv1Plan{plan: plan, found: found, err: err}
	return plan, found, err
}

type preparedTextPart struct {
	paths      []subpath
	vector     *preparedNode
	userPaint  bool
	foreground bool
	color      color.NRGBA
}

func parsePreparedFont(data []byte, faceIndex uint16) (*preparedFont, error) {
	parsed, err := fontface.ParseFace(data, faceIndex)
	if err != nil {
		return nil, err
	}
	return newPreparedFont(parsed), nil
}

func newPreparedFont(parsed *fontface.ParsedFace) *preparedFont {
	if parsed == nil {
		return nil
	}
	return &preparedFont{face: parsed, outline: parsed.Outline, shaping: parsed.Shaping}
}

func (p *preflight) text(nodeID string, text d2scene.TextRun, transform d2scene.Matrix, animation animationOverrides, importDepth int) (*preparedPrimitive, error) {
	if !finitePoint(text.Origin) || !finite(text.Font.Size) || text.Font.Size <= 0 || text.Font.Size > float64(math.MaxInt32)/64 {
		return nil, fmt.Errorf("d2raster: node %q text has invalid origin or font size", nodeID)
	}
	if text.Anchor > d2scene.AnchorEnd {
		return nil, fmt.Errorf("d2raster: node %q text has invalid anchor %d", nodeID, text.Anchor)
	}
	if !text.Ink.IsFinite() {
		return nil, fmt.Errorf("d2raster: node %q text has invalid ink bounds", nodeID)
	}
	if text.Font.Asset == "" {
		return nil, fmt.Errorf("d2raster: node %q text has empty font asset ID", nodeID)
	}
	primary, ok := p.fonts[text.Font.Asset]
	if !ok {
		if _, exists := p.document.Assets[text.Font.Asset]; exists {
			return nil, fmt.Errorf("d2raster: node %q text asset %q is not a usable font", nodeID, text.Font.Asset)
		}
		return nil, fmt.Errorf("d2raster: node %q text references missing font asset %q", nodeID, text.Font.Asset)
	}
	ppem := fixed.Int26_6(math.Round(text.Font.Size * 64))
	glyphs, advance, err := p.positionGlyphs(text, ppem)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q text: %w", nodeID, err)
	}
	anchorOffset := 0.0
	switch text.Anchor {
	case d2scene.AnchorStart:
	case d2scene.AnchorMiddle:
		anchorOffset = -advance / 2
	case d2scene.AnchorEnd:
		anchorOffset = -advance
	}

	tolerance := flattenTolerance(transform)
	var paths []subpath
	var parts []preparedTextPart
	hasColorGlyph := false
	appendPart := func(part preparedTextPart) error {
		if err := p.addPreparedNodes(1); err != nil {
			return fmt.Errorf("d2raster: node %q color text: %w", nodeID, err)
		}
		parts = append(parts, part)
		return nil
	}
	flushUserPaths := func() error {
		if len(paths) == 0 {
			return nil
		}
		if err := appendPart(preparedTextPart{paths: paths, userPaint: true}); err != nil {
			return err
		}
		paths = nil
		return nil
	}
	var allPaths []subpath
	var colorLocalBounds d2scene.Bounds
	buffers := make(map[*sfnt.Font]*sfnt.Buffer)
	for index, glyph := range glyphs {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		if glyph.empty {
			continue
		}
		buffer := buffers[glyph.font.outline]
		if buffer == nil {
			buffer = &sfnt.Buffer{}
			buffers[glyph.font.outline] = buffer
		}
		origin := d2scene.Point{
			X: text.Origin.X + anchorOffset + glyph.position.X,
			Y: text.Origin.Y + glyph.position.Y,
		}
		plan, colorGlyph, err := glyph.font.bundledCOLRv1Plan(uint32(glyph.id))
		if err != nil {
			return nil, unsupportedPositionedGlyphDataError(glyph, err)
		}
		if colorGlyph {
			if !hasColorGlyph {
				if err := p.addPreparedNodes(1); err != nil {
					return nil, fmt.Errorf("d2raster: node %q color text root: %w", nodeID, err)
				}
				hasColorGlyph = true
			}
			if err := flushUserPaths(); err != nil {
				return nil, err
			}
			vector, localBounds, err := p.colrv1TextGlyph(nodeID, glyph.font, plan, origin, ppem, transform)
			if err != nil {
				return nil, unsupportedPositionedGlyphDataError(glyph, err)
			}
			if vector != nil {
				parts = append(parts, preparedTextPart{vector: vector})
			}
			colorLocalBounds = colorLocalBounds.Union(localBounds)
			continue
		}
		layers, colorGlyph, err := glyph.font.parsedFace().COLR0GlyphLayers(uint32(glyph.id))
		if err != nil {
			return nil, unsupportedPositionedGlyphDataError(glyph, err)
		}
		if !colorGlyph {
			segments, err := glyph.font.outline.LoadGlyph(buffer, glyph.id, ppem, nil)
			if err != nil {
				return nil, unsupportedPositionedGlyphDataError(glyph, err)
			}
			glyphPaths, err := flattenGlyph(p.ctx, segments, origin, tolerance, p.addPathSegment)
			if err != nil {
				return nil, fmt.Errorf("flatten glyph %d (ID %d): %w", index, glyph.id, err)
			}
			paths = append(paths, glyphPaths...)
			allPaths = append(allPaths, glyphPaths...)
			continue
		}

		if !hasColorGlyph {
			if err := p.addPreparedNodes(1); err != nil {
				return nil, fmt.Errorf("d2raster: node %q color text root: %w", nodeID, err)
			}
			hasColorGlyph = true
		}
		if err := flushUserPaths(); err != nil {
			return nil, err
		}
		for layerIndex, layer := range layers {
			// Charge every COLRv0 layer before loading it, including empty layers,
			// so a font cannot use them to evade structural or path-work limits.
			if err := p.addPreparedNodes(1); err != nil {
				return nil, fmt.Errorf("d2raster: node %q glyph %d (ID %d) COLRv0 layer %d: %w", nodeID, index, glyph.id, layerIndex, err)
			}
			if err := p.addPathSegment(); err != nil {
				return nil, fmt.Errorf("glyph %d (ID %d) COLRv0 layer %d: %w", index, glyph.id, layerIndex, err)
			}
			segments, err := glyph.font.outline.LoadGlyph(buffer, sfnt.GlyphIndex(layer.GlyphID), ppem, nil)
			if err != nil {
				return nil, unsupportedPositionedGlyphDataError(glyph, fmt.Errorf("load COLRv0 layer glyph %d: %w", layer.GlyphID, err))
			}
			layerPaths, err := flattenGlyph(p.ctx, segments, origin, tolerance, p.addPathSegment)
			if err != nil {
				return nil, fmt.Errorf("flatten glyph %d (ID %d) COLRv0 layer %d: %w", index, glyph.id, layerIndex, err)
			}
			allPaths = append(allPaths, layerPaths...)
			if len(layerPaths) == 0 {
				continue
			}
			parts = append(parts, preparedTextPart{
				paths: layerPaths, foreground: layer.Foreground, color: layer.Color,
			})
		}
	}
	if hasColorGlyph {
		if err := flushUserPaths(); err != nil {
			return nil, err
		}
	}

	var decorations []subpath
	if (text.Underline || text.Strike) && advance > 0 {
		var buffer sfnt.Buffer
		metrics, err := primary.outline.Metrics(&buffer, ppem, font.HintingNone)
		if err != nil {
			return nil, fmt.Errorf("d2raster: node %q text metrics: %w", nodeID, err)
		}
		minThickness := 1.0
		if scale := transform.MaxScale(); finite(scale) && scale > geometryEpsilon {
			minThickness = 1 / scale
		}
		thickness := math.Max(text.Font.Size/16, minThickness)
		left := text.Origin.X + anchorOffset
		if text.Underline {
			descent := fixedToFloat(metrics.Descent)
			top := text.Origin.Y + math.Max(text.Font.Size/12, descent*0.25)
			decoration, err := decorationSubpath(left, top, advance, thickness, p.addPathSegment)
			if err != nil {
				return nil, fmt.Errorf("d2raster: node %q underline: %w", nodeID, err)
			}
			decorations = append(decorations, decoration)
			allPaths = append(allPaths, decoration)
		}
		if text.Strike {
			xHeight := fixedToFloat(metrics.XHeight)
			if xHeight <= 0 {
				xHeight = text.Font.Size * 0.5
			}
			top := text.Origin.Y - xHeight/2 - thickness/2
			decoration, err := decorationSubpath(left, top, advance, thickness, p.addPathSegment)
			if err != nil {
				return nil, fmt.Errorf("d2raster: node %q strike: %w", nodeID, err)
			}
			decorations = append(decorations, decoration)
			allPaths = append(allPaths, decoration)
		}
	}
	if hasColorGlyph && len(decorations) != 0 {
		if err := appendPart(preparedTextPart{paths: decorations, userPaint: true}); err != nil {
			return nil, err
		}
	}
	objectBounds := localObjectBounds(allPaths)
	if colorLocalBounds.Valid {
		if len(allPaths) == 0 {
			objectBounds = colorLocalBounds.Box()
		} else {
			objectBounds = objectBounds.Bounds().Union(colorLocalBounds).Box()
		}
	}
	if text.Ink.Valid {
		// TextRun.Ink is the scene builder's exact measured node-local ink
		// contract. Prefer it to the flattened glyph approximation for
		// objectBoundingBox paint coordinates.
		objectBounds = text.Ink.Box()
	}
	fill, err := p.prepareAnimatedPaint(text.Fill, animation.fillColor, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q text fill: %w", nodeID, err)
	}
	stroke, err := p.prepareAnimatedStroke(text.Stroke, animation.strokeColor, animation.dashOffset, objectBounds, transform, importDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q text stroke: %w", nodeID, err)
	}
	if !hasColorGlyph {
		return p.finishPrimitive(nodeID, allPaths, transform, fill, stroke)
	}
	root := &preparedNode{opacity: 1, blend: d2scene.BlendNormal}
	for _, part := range parts {
		if part.vector != nil {
			appendPreparedChild(root, part.vector)
			continue
		}
		partFill, partStroke := preparedTextPartPaint(part, fill, stroke)
		primitive, err := p.finishPrimitive(nodeID, part.paths, transform, partFill, partStroke)
		if err != nil {
			return nil, err
		}
		child := &preparedNode{
			opacity: 1, blend: d2scene.BlendNormal, primitive: primitive,
			bounds: primitive.bounds, contentBounds: primitive.bounds,
		}
		root.children = append(root.children, child)
		root.bounds = unionRect(root.bounds, child.bounds)
	}
	root.contentBounds = root.bounds
	return &preparedPrimitive{vector: root, bounds: root.bounds}, nil
}

func preparedTextPartPaint(part preparedTextPart, fill *preparedPaint, stroke *preparedStroke) (*preparedPaint, *preparedStroke) {
	if part.userPaint {
		return fill, stroke
	}
	if part.foreground {
		return fill, nil
	}
	return &preparedPaint{kind: preparedSolidPaint, solid: part.color}, nil
}

func (p *preflight) positionGlyphs(text d2scene.TextRun, ppem fixed.Int26_6) ([]positionedGlyph, float64, error) {
	if len(text.Glyphs) != 0 {
		if len(text.Glyphs) > p.options.MaxTextRunesPerRun {
			return nil, 0, fmt.Errorf("explicit glyph count %d exceeds per-run limit %d", len(text.Glyphs), p.options.MaxTextRunesPerRun)
		}
		if len(text.Glyphs) > p.options.MaxPathCommands-p.shapedGlyphs {
			return nil, 0, fmt.Errorf("shaped glyph count exceeds limit %d", p.options.MaxPathCommands)
		}
		glyphs := make([]positionedGlyph, 0, len(text.Glyphs))
		advance := 0.0
		for index, glyph := range text.Glyphs {
			if err := p.ctx.Err(); err != nil {
				return nil, 0, err
			}
			assetID := glyph.Asset
			if assetID == "" {
				assetID = text.Font.Asset
			}
			parsed, ok := p.fonts[assetID]
			if !ok {
				return nil, 0, fmt.Errorf("glyph %d references unusable font asset %q", index, assetID)
			}
			if glyph.Empty {
				if glyph.ID != 0 {
					return nil, 0, fmt.Errorf("empty glyph at index %d has non-zero glyph ID %d", index, glyph.ID)
				}
				if !finitePoint(glyph.Position) || !finite(glyph.Advance) || !glyph.Ink.IsFinite() {
					return nil, 0, fmt.Errorf("glyph %d has invalid geometry", index)
				}
				glyphs = append(glyphs, positionedGlyph{
					font: parsed, asset: assetID, position: glyph.Position, empty: true,
				})
				advance = math.Max(advance, glyph.Position.X+glyph.Advance)
				continue
			}
			if glyph.ID == 0 {
				return nil, 0, fmt.Errorf("missing glyph at index %d: glyph ID 0", index)
			}
			if glyph.ID > math.MaxUint16 || int(glyph.ID) >= parsed.outline.NumGlyphs() {
				return nil, 0, fmt.Errorf("glyph ID %d at index %d is out of range", glyph.ID, index)
			}
			if !finitePoint(glyph.Position) || !finite(glyph.Advance) || !glyph.Ink.IsFinite() {
				return nil, 0, fmt.Errorf("glyph %d has invalid geometry", index)
			}
			glyphs = append(glyphs, positionedGlyph{
				id: sfnt.GlyphIndex(glyph.ID), font: parsed, asset: assetID, position: glyph.Position,
			})
			advance = math.Max(advance, glyph.Position.X+glyph.Advance)
		}
		p.shapedGlyphs += len(glyphs)
		return glyphs, advance, nil
	}

	remainingRunes := p.options.MaxPathCommands - p.textRunes
	if remainingRunes <= 0 {
		return nil, 0, fmt.Errorf("text rune count exceeds aggregate shaping-input limit %d", p.options.MaxPathCommands)
	}
	if len(text.Fallbacks) > p.options.MaxAssets {
		return nil, 0, fmt.Errorf("fallback font reference count %d exceeds asset limit %d", len(text.Fallbacks), p.options.MaxAssets)
	}

	fontIDs := make([]d2scene.AssetID, 0, 1+len(text.Fallbacks))
	fontIDs = append(fontIDs, text.Font.Asset)
	fontIDs = append(fontIDs, text.Fallbacks...)
	shapeFaces := make([]fontface.ShapeFace, 0, len(fontIDs))
	preparedFaces := make([]*preparedFont, 0, len(fontIDs))
	seen := make(map[d2scene.AssetID]bool, len(fontIDs))
	for _, id := range fontIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		parsed, ok := p.fonts[id]
		if !ok {
			return nil, 0, fmt.Errorf("text references unusable font asset %q", id)
		}
		shapeFaces = append(shapeFaces, fontface.ShapeFace{
			ID: string(id), Face: parsed.parsedFace(),
		})
		preparedFaces = append(preparedFaces, parsed)
	}
	remainingCoverage := p.options.MaxTextCoverageChecks - p.textCoverageChecks
	if remainingCoverage <= 0 {
		return nil, 0, fmt.Errorf("text font coverage checks exceed aggregate limit %d", p.options.MaxTextCoverageChecks)
	}
	remainingRuns := p.options.MaxTextShapingRuns - p.textShapingRuns
	if remainingRuns <= 0 {
		return nil, 0, fmt.Errorf("text shaping run count exceeds aggregate limit %d", p.options.MaxTextShapingRuns)
	}
	remainingGlyphs := p.options.MaxPathCommands - p.shapedGlyphs
	if remainingGlyphs <= 0 {
		return nil, 0, fmt.Errorf("shaped glyph count exceeds limit %d", p.options.MaxPathCommands)
	}
	shaped, err := fontface.ShapeText(p.ctx, text.Text, ppem, shapeFaces, fontface.ShapeLimits{
		Runes:          min(p.options.MaxTextRunesPerRun, remainingRunes),
		Faces:          p.options.MaxFontFacesPerText,
		CoverageChecks: remainingCoverage,
		Runs:           remainingRuns,
		Glyphs:         remainingGlyphs,
	})
	if err != nil {
		return nil, 0, err
	}
	p.textRunes += shaped.Runes
	p.textCoverageChecks += shaped.CoverageChecks
	p.textShapingRuns += shaped.Runs
	p.shapedGlyphs += len(shaped.Glyphs)
	glyphs := make([]positionedGlyph, 0, len(shaped.Glyphs))
	for _, glyph := range shaped.Glyphs {
		assetID := d2scene.AssetID(shapeFaces[glyph.Face].ID)
		glyphs = append(glyphs, positionedGlyph{
			id: sfnt.GlyphIndex(glyph.ID), font: preparedFaces[glyph.Face], asset: assetID,
			position: d2scene.Point{X: glyph.PositionX, Y: glyph.PositionY},
			empty:    glyph.Empty, source: glyph.Source, sourceAt: glyph.SourceIndex,
		})
	}
	return glyphs, shaped.Advance, nil
}

func glyphDataKind(data gotextfont.GlyphData) string {
	kind := "non-outline"
	switch data.(type) {
	case gotextfont.GlyphOutline:
		kind = "outline"
	case gotextfont.GlyphColor:
		kind = "color"
	case gotextfont.GlyphBitmap:
		kind = "bitmap"
	case gotextfont.GlyphSVG:
		kind = "SVG"
	case nil:
		kind = "missing"
	}
	return kind

}

func unsupportedPositionedGlyphDataError(glyph positionedGlyph, cause error) error {
	kind := glyphDataKind(glyph.font.shaping.GlyphData(gotextfont.GID(glyph.id)))
	if glyph.source != 0 {
		return fmt.Errorf("%s glyph U+%04X at rune %d in font asset %q cannot be rasterized: %w", kind, glyph.source, glyph.sourceAt, glyph.asset, cause)
	}
	return fmt.Errorf("%s glyph at cluster %d in font asset %q cannot be rasterized: %w", kind, glyph.sourceAt, glyph.asset, cause)
}

func flattenGlyph(ctx context.Context, segments sfnt.Segments, origin d2scene.Point, tolerance float64, count func() error) ([]subpath, error) {
	var paths []subpath
	var current subpath
	var cursor d2scene.Point
	haveCursor := false
	flush := func() {
		if len(current.points) != 0 {
			current.closed = true
			paths = append(paths, current)
		}
		current = subpath{}
	}
	appendPoint := func(point d2scene.Point) error {
		if !finitePoint(point) {
			return fmt.Errorf("non-finite point")
		}
		if err := count(); err != nil {
			return err
		}
		if len(current.points) == 0 || !samePoint(current.points[len(current.points)-1], point) {
			current.points = append(current.points, point)
		}
		return nil
	}
	point := func(value fixed.Point26_6) d2scene.Point {
		return d2scene.Point{X: origin.X + fixedToFloat(value.X), Y: origin.Y + fixedToFloat(value.Y)}
	}

	for index, segment := range segments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			flush()
			cursor = point(segment.Args[0])
			if err := appendPoint(cursor); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
			haveCursor = true
		case sfnt.SegmentOpLineTo:
			if !haveCursor {
				return nil, fmt.Errorf("segment %d: line before move", index)
			}
			cursor = point(segment.Args[0])
			if err := appendPoint(cursor); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
		case sfnt.SegmentOpQuadTo:
			if !haveCursor {
				return nil, fmt.Errorf("segment %d: quadratic before move", index)
			}
			control, end := point(segment.Args[0]), point(segment.Args[1])
			if err := flattenQuadratic(ctx, cursor, control, end, tolerance, 0, appendPoint); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
			cursor = end
		case sfnt.SegmentOpCubeTo:
			if !haveCursor {
				return nil, fmt.Errorf("segment %d: cubic before move", index)
			}
			control1, control2, end := point(segment.Args[0]), point(segment.Args[1]), point(segment.Args[2])
			if err := flattenCubic(ctx, cursor, control1, control2, end, tolerance, 0, appendPoint); err != nil {
				return nil, fmt.Errorf("segment %d: %w", index, err)
			}
			cursor = end
		default:
			return nil, fmt.Errorf("segment %d: unknown operation %d", index, segment.Op)
		}
	}
	flush()
	return paths, nil
}

func decorationSubpath(left, top, width, height float64, count func() error) (subpath, error) {
	points := []d2scene.Point{
		{X: left, Y: top},
		{X: left + width, Y: top},
		{X: left + width, Y: top + height},
		{X: left, Y: top + height},
	}
	for range points {
		if err := count(); err != nil {
			return subpath{}, err
		}
	}
	return subpath{points: points, closed: true}, nil
}

func fixedToFloat(value fixed.Int26_6) float64 {
	return float64(value) / 64
}
