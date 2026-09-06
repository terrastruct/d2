package d2raster

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

type preparedPattern struct {
	tile            d2scene.Box
	originModX      float64
	originModY      float64
	deviceToPattern d2scene.Matrix
	tileResource    *preparedPatternTile
	directPixels    bool
	pixelOffsetX    int
	pixelOffsetY    int
}

type preparedPatternTile struct {
	width     int
	height    int
	bounds    image.Rectangle
	tileBytes int64
	root      *preparedNode

	image     *image.RGBA
	rendering bool
}

type preparedPatternTileKey struct {
	root        *d2scene.Node
	width       int
	height      int
	scaleX      float64
	scaleY      float64
	importDepth int
}

func (p *preflight) prepareAnimatedPaint(paint d2scene.Paint, animatedColor *color.NRGBA, objectBounds d2scene.Box, objectToDevice d2scene.Matrix, importDepth int) (*preparedPaint, error) {
	return prepareAnimatedPaintWithPattern(paint, animatedColor, objectBounds, objectToDevice, func(pattern d2scene.PatternPaint, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (*preparedPaint, error) {
		return p.preparePatternPaint(pattern, objectBounds, objectToDevice, importDepth)
	})
}

func (p *preflight) preparePatternPaint(pattern d2scene.PatternPaint, objectBounds d2scene.Box, objectToDevice d2scene.Matrix, importDepth int) (*preparedPaint, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateBox(pattern.Tile); err != nil {
		return nil, fmt.Errorf("pattern tile: %w", err)
	}
	if pattern.Tile.Width == 0 || pattern.Tile.Height == 0 {
		return nil, fmt.Errorf("pattern tile has zero width or height")
	}
	if !finite(pattern.Tile.X+pattern.Tile.Width) || !finite(pattern.Tile.Y+pattern.Tile.Height) {
		return nil, fmt.Errorf("pattern tile endpoints are non-finite")
	}
	if pattern.Root == nil {
		return nil, fmt.Errorf("pattern has no root node")
	}

	patternToDevice, deviceToPattern, err := preparePatternTransform(pattern.Units, pattern.Transform, objectBounds, objectToDevice)
	if err != nil {
		return nil, fmt.Errorf("pattern: %w", err)
	}
	width, err := patternTilePixels(patternToDevice.Vector(d2scene.Point{X: pattern.Tile.Width}))
	if err != nil {
		return nil, fmt.Errorf("pattern tile width: %w", err)
	}
	height, err := patternTilePixels(patternToDevice.Vector(d2scene.Point{Y: pattern.Tile.Height}))
	if err != nil {
		return nil, fmt.Errorf("pattern tile height: %w", err)
	}
	bounds := image.Rect(0, 0, width, height)
	tileBytes, err := pixelStorageBytes(bounds, 4)
	if err != nil {
		return nil, fmt.Errorf("pattern tile: %w", err)
	}

	scaleX := float64(width) / pattern.Tile.Width
	scaleY := float64(height) / pattern.Tile.Height
	if !finite(scaleX) || !finite(scaleY) || scaleX <= 0 || scaleY <= 0 {
		return nil, fmt.Errorf("pattern tile cannot be represented in raster coordinates")
	}
	nextImportDepth := importDepth + 1
	key := preparedPatternTileKey{
		root: pattern.Root, width: width, height: height,
		scaleX: scaleX, scaleY: scaleY, importDepth: nextImportDepth,
	}
	if p.patternTiles == nil {
		p.patternTiles = make(map[preparedPatternTileKey]*preparedPatternTile)
	}
	tileResource, found := p.patternTiles[key]
	if found && tileResource == nil {
		return nil, fmt.Errorf("pattern root: cyclic pattern resource")
	}
	if !found {
		// A nil sentinel makes definition cycles actionable while the root is
		// being prepared. Remove it on error so a failed preparation cannot
		// poison subsequent diagnostic work in this preflight.
		p.patternTiles[key] = nil
		previousFrameBounds := p.frameBounds
		p.frameBounds = bounds
		root, rootErr := p.node(pattern.Root, d2scene.Scale(scaleX, scaleY), 1, nextImportDepth)
		p.frameBounds = previousFrameBounds
		if rootErr != nil {
			delete(p.patternTiles, key)
			return nil, fmt.Errorf("pattern root: %w", rootErr)
		}
		tileResource = &preparedPatternTile{
			width: width, height: height, bounds: bounds, tileBytes: tileBytes, root: root,
		}
		p.patternTiles[key] = tileResource
	}
	pixelOffsetX, pixelOffsetY, directPixels := directPatternPixelOffsets(pattern.Tile, deviceToPattern, width, height)
	return &preparedPaint{
		kind: preparedPatternPaint,
		pattern: &preparedPattern{
			tile:            pattern.Tile,
			originModX:      math.Mod(pattern.Tile.X, pattern.Tile.Width),
			originModY:      math.Mod(pattern.Tile.Y, pattern.Tile.Height),
			deviceToPattern: deviceToPattern,
			tileResource:    tileResource,
			directPixels:    directPixels,
			pixelOffsetX:    pixelOffsetX,
			pixelOffsetY:    pixelOffsetY,
		},
	}, nil
}

// directPatternPixelOffsets recognizes the common one-device-pixel to
// one-pattern-pixel mapping. Integer translation and tile origins make every
// pixel-center sample land at n+0.5, so wrapping can be performed exactly in
// integer pixel space without a floating-point remainder per channel.
func directPatternPixelOffsets(tile d2scene.Box, deviceToPattern d2scene.Matrix, width, height int) (int, int, bool) {
	if width <= 0 || height <= 0 ||
		tile.Width != float64(width) || tile.Height != float64(height) ||
		deviceToPattern.A != 1 || deviceToPattern.B != 0 ||
		deviceToPattern.C != 0 || deviceToPattern.D != 1 {
		return 0, 0, false
	}
	const exactIntegerLimit = float64(uint64(1) << 52)
	values := [...]float64{tile.X, tile.Y, deviceToPattern.E, deviceToPattern.F}
	for _, value := range values {
		if !finite(value) || math.Trunc(value) != value || value < -exactIntegerLimit || value > exactIntegerLimit {
			return 0, 0, false
		}
	}
	offset := func(translation, origin float64, period int) int {
		period64 := int64(period)
		value := int64(translation)%period64 - int64(origin)%period64
		value %= period64
		if value < 0 {
			value += period64
		}
		return int(value)
	}
	return offset(deviceToPattern.E, tile.X, width), offset(deviceToPattern.F, tile.Y, height), true
}

func preparePatternTransform(units d2scene.PaintUnits, patternTransform d2scene.Matrix, objectBounds d2scene.Box, objectToDevice d2scene.Matrix) (d2scene.Matrix, d2scene.Matrix, error) {
	if units > d2scene.UserSpaceOnUse {
		return d2scene.Matrix{}, d2scene.Matrix{}, fmt.Errorf("invalid paint units %d", units)
	}
	if !patternTransform.IsFinite() {
		return d2scene.Matrix{}, d2scene.Matrix{}, fmt.Errorf("non-finite pattern transform")
	}
	if !objectToDevice.IsFinite() {
		return d2scene.Matrix{}, d2scene.Matrix{}, fmt.Errorf("non-finite object transform")
	}
	unitsToObject := d2scene.Identity()
	if units == d2scene.ObjectBoundingBox {
		if err := validateBox(objectBounds); err != nil {
			return d2scene.Matrix{}, d2scene.Matrix{}, fmt.Errorf("invalid object bounding box: %w", err)
		}
		if objectBounds.Width == 0 || objectBounds.Height == 0 {
			return d2scene.Matrix{}, d2scene.Matrix{}, fmt.Errorf("object bounding box has zero width or height")
		}
		unitsToObject = d2scene.Translate(objectBounds.X, objectBounds.Y).Mul(d2scene.Scale(objectBounds.Width, objectBounds.Height))
	}
	patternToDevice := objectToDevice.Mul(unitsToObject).Mul(patternTransform)
	if !patternToDevice.IsFinite() {
		return d2scene.Matrix{}, d2scene.Matrix{}, fmt.Errorf("composed pattern transform is non-finite")
	}
	deviceToPattern, invertible, err := finiteAffineInverse(patternToDevice)
	if err != nil || !invertible {
		return d2scene.Matrix{}, d2scene.Matrix{}, fmt.Errorf("singular pattern transform")
	}
	return patternToDevice, deviceToPattern, nil
}

func patternTilePixels(vector d2scene.Point) (int, error) {
	extent := math.Hypot(vector.X, vector.Y)
	if !finite(extent) || extent <= 0 {
		return 0, fmt.Errorf("maps outside the finite pixel domain")
	}
	extent = math.Ceil(extent)
	maxInt := int(^uint(0) >> 1)
	if !finite(extent) || extent <= 0 || extent >= float64(maxInt) {
		return 0, fmt.Errorf("exceeds the platform integer domain")
	}
	return int(extent), nil
}

func (paint *preparedPaint) ensureRendered(ctx context.Context, scratch *rasterScratch) error {
	if paint == nil || paint.kind != preparedPatternPaint {
		return nil
	}
	if paint.pattern == nil {
		return fmt.Errorf("d2raster: internal nil prepared pattern")
	}
	if paint.pattern.tileResource == nil {
		return fmt.Errorf("d2raster: internal nil prepared pattern tile")
	}
	return paint.pattern.tileResource.render(ctx, scratch)
}

func (tile *preparedPatternTile) render(ctx context.Context, scratch *rasterScratch) error {
	if tile.image != nil {
		return ctx.Err()
	}
	if tile.rendering {
		return fmt.Errorf("d2raster: cyclic prepared pattern rendering")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reservation, err := scratch.offscreen.reserveBytes(tile.tileBytes, "pattern tile")
	if err != nil {
		return err
	}
	scratch.patternBytes += reservation
	tile.rendering = true
	defer func() { tile.rendering = false }()
	imageTile := image.NewRGBA(tile.bounds)
	if err := renderNode(ctx, imageTile, tile.root, scratch); err != nil {
		scratch.offscreen.release(reservation)
		scratch.patternBytes -= reservation
		return err
	}
	tile.image = imageTile
	scratch.patternTiles = append(scratch.patternTiles, tile)
	return ctx.Err()
}

func (pattern *preparedPattern) colorAt(x, y float64) (color.NRGBA, bool) {
	if pattern == nil || pattern.tileResource == nil || pattern.tileResource.image == nil {
		return color.NRGBA{}, false
	}
	tile := pattern.tileResource
	point := pattern.deviceToPattern.Point(d2scene.Point{X: x, Y: y})
	localX, ok := wrappedPatternCoordinateFromOriginMod(point.X, pattern.originModX, pattern.tile.Width)
	if !ok {
		return color.NRGBA{}, false
	}
	localY, ok := wrappedPatternCoordinateFromOriginMod(point.Y, pattern.originModY, pattern.tile.Height)
	if !ok {
		return color.NRGBA{}, false
	}
	pixelX := int(math.Floor(localX / pattern.tile.Width * float64(tile.width)))
	pixelY := int(math.Floor(localY / pattern.tile.Height * float64(tile.height)))
	pixelX = max(0, min(tile.width-1, pixelX))
	pixelY = max(0, min(tile.height-1, pixelY))
	return patternTileColor(tile, pixelX, pixelY)
}

func patternTileColor(tile *preparedPatternTile, pixelX, pixelY int) (color.NRGBA, bool) {
	offset := tile.image.PixOffset(pixelX, pixelY)
	pixel := tile.image.Pix[offset : offset+4]
	alpha := pixel[3]
	if alpha == 0 {
		return color.NRGBA{}, false
	}
	if alpha == 0xff {
		return color.NRGBA{R: pixel[0], G: pixel[1], B: pixel[2], A: alpha}, true
	}
	// RGBA stores premultiplied bytes. This is the same 16-bit conversion used
	// by color.NRGBAModel, written directly to avoid two interface allocations
	// for every sampled pattern pixel.
	return color.NRGBA{
		R: uint8((uint32(pixel[0]) * 0xffff / uint32(alpha)) >> 8),
		G: uint8((uint32(pixel[1]) * 0xffff / uint32(alpha)) >> 8),
		B: uint8((uint32(pixel[2]) * 0xffff / uint32(alpha)) >> 8),
		A: alpha,
	}, true
}

func wrappedPatternCoordinateFromOriginMod(value, originMod, period float64) (float64, bool) {
	if !finite(value) || !finite(originMod) || !finite(period) || period <= 0 {
		return 0, false
	}
	wrapped := math.Mod(value, period) - originMod
	if !finite(wrapped) {
		return 0, false
	}
	if wrapped >= period {
		wrapped -= period
	} else if wrapped < 0 {
		if wrapped == -period {
			return math.Copysign(0, -1), true
		}
		wrapped += period
		if wrapped < 0 {
			wrapped += period
		}
	}
	return wrapped, true
}

func (scratch *rasterScratch) releasePatternTiles() {
	if scratch == nil {
		return
	}
	for _, tile := range scratch.patternTiles {
		tile.image = nil
	}
	scratch.patternTiles = nil
	if scratch.patternBytes != 0 {
		scratch.offscreen.release(scratch.patternBytes)
	}
	scratch.patternBytes = 0
}

func collectPreparedPatterns(ctx context.Context, root *preparedNode, dst image.Rectangle) ([]*preparedPatternTile, error) {
	collector := patternCollector{
		ctx:   ctx,
		state: make(map[*preparedPatternTile]uint8),
	}
	if err := collector.node(root, dst); err != nil {
		return nil, err
	}
	return collector.patterns, nil
}

type patternCollector struct {
	ctx      context.Context
	state    map[*preparedPatternTile]uint8
	patterns []*preparedPatternTile
}

func (collector *patternCollector) node(node *preparedNode, dst image.Rectangle) error {
	if node == nil || node.opacity == 0 {
		return nil
	}
	if err := collector.ctx.Err(); err != nil {
		return err
	}
	bounds := node.bounds.Intersect(dst)
	if bounds.Empty() {
		return nil
	}
	paintBounds := dst
	if len(node.filters) != 0 {
		paintBounds = node.contentBounds
	} else if node.opacity < 1 || node.blend != d2scene.BlendNormal || node.clip != nil || node.mask != nil {
		paintBounds = bounds
	}
	if node.primitive != nil && !node.primitive.bounds.Intersect(paintBounds).Empty() {
		if err := collector.primitive(node.primitive, paintBounds); err != nil {
			return err
		}
	}
	for _, child := range node.children {
		if err := collector.node(child, paintBounds); err != nil {
			return err
		}
	}
	if node.mask != nil {
		if err := collector.node(node.mask.root, bounds); err != nil {
			return err
		}
	}
	return nil
}

func (collector *patternCollector) primitive(primitive *preparedPrimitive, dst image.Rectangle) error {
	if primitive.image != nil {
		return nil
	}
	if primitive.vector != nil {
		return collector.node(primitive.vector, dst)
	}
	if primitive.fill != nil && !subpathPixelBounds(primitive.subpaths, primitive.transform, 0, dst).Empty() {
		if err := collector.paint(primitive.fill); err != nil {
			return err
		}
	}
	if primitive.stroke != nil && !paintedStrokePixelBounds(primitive.strokeRuns, primitive.transform, primitive.stroke, dst).Empty() {
		if err := collector.paint(primitive.stroke.paint); err != nil {
			return err
		}
	}
	return nil
}

func (collector *patternCollector) paint(paint *preparedPaint) error {
	if paint == nil || paint.kind != preparedPatternPaint {
		return nil
	}
	pattern := paint.pattern
	if pattern == nil || pattern.tileResource == nil {
		return fmt.Errorf("d2raster: internal nil prepared pattern")
	}
	tile := pattern.tileResource
	switch collector.state[tile] {
	case 1:
		return fmt.Errorf("d2raster: cyclic prepared pattern graph")
	case 2:
		return nil
	}
	collector.state[tile] = 1
	if err := collector.node(tile.root, tile.bounds); err != nil {
		return err
	}
	collector.state[tile] = 2
	collector.patterns = append(collector.patterns, tile)
	return nil
}
