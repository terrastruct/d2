package d2raster

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/internal/rasterimage"
)

const imageSamplesPerAxis = 4

type preparedRasterAsset struct {
	image  image.Image
	bounds image.Rectangle
	width  int
	height int
}

type preparedImage struct {
	asset     *preparedRasterAsset
	box       d2scene.Box
	placement d2scene.Box
	inverse   inverseAffine
	bounds    image.Rectangle
}

// inverseAffine retains a normalized inverse of an affine transform's linear
// portion. Normalizing before the determinant avoids turning otherwise valid
// very large or very small transforms into false overflow/underflow failures.
type inverseAffine struct {
	a, b, c, d float64
	e, f       float64
}

type rasterAssetValidation struct {
	format       string
	decodedBytes int64
}

func prepareRasterAsset(ctx context.Context, id d2scene.AssetID, asset d2scene.RasterAsset, availableBytes int64) (*preparedRasterAsset, int64, error) {
	validation, err := validateRasterAsset(ctx, id, asset, availableBytes)
	if err != nil {
		return nil, 0, err
	}
	prepared, err := decodeRasterAsset(ctx, id, asset, validation)
	if err != nil {
		return nil, 0, err
	}
	return prepared, validation.decodedBytes, nil
}

func validateRasterAsset(ctx context.Context, id d2scene.AssetID, asset d2scene.RasterAsset, availableBytes int64) (rasterAssetValidation, error) {
	prefix := fmt.Sprintf("d2raster: raster asset %q", id)
	if err := ctx.Err(); err != nil {
		return rasterAssetValidation{}, err
	}
	if len(asset.Data) == 0 {
		return rasterAssetValidation{}, fmt.Errorf("%s has no data", prefix)
	}
	if asset.PixelWidth <= 0 || asset.PixelHeight <= 0 {
		return rasterAssetValidation{}, fmt.Errorf("%s has invalid declared dimensions %dx%d", prefix, asset.PixelWidth, asset.PixelHeight)
	}
	wantFormat, ok := rasterFormatForMIME(asset.MIMEType)
	if !ok {
		return rasterAssetValidation{}, fmt.Errorf("%s has unsupported MIME type %q; supported raster MIME types are image/png, image/jpeg, image/gif, and image/webp", prefix, asset.MIMEType)
	}
	gotSignature := rasterDataFormat(asset.Data)
	if gotSignature == "" {
		return rasterAssetValidation{}, fmt.Errorf("%s data has no supported raster signature", prefix)
	}
	if gotSignature != wantFormat {
		return rasterAssetValidation{}, fmt.Errorf("%s declares MIME type %q but data has %s signature", prefix, asset.MIMEType, strings.ToUpper(gotSignature))
	}
	config, _, err := rasterimage.Config(ctx, asset.Data, wantFormat)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return rasterAssetValidation{}, contextErr
		}
		return rasterAssetValidation{}, fmt.Errorf("%s has malformed %s data: %w", prefix, strings.ToUpper(wantFormat), err)
	}
	if config.Width != asset.PixelWidth || config.Height != asset.PixelHeight {
		return rasterAssetValidation{}, fmt.Errorf(
			"%s declares dimensions %dx%d but encoded data is %dx%d",
			prefix, asset.PixelWidth, asset.PixelHeight, config.Width, config.Height,
		)
	}
	decodedBytes, err := rasterDecodedBytes(config)
	if err != nil {
		return rasterAssetValidation{}, fmt.Errorf("%s: %w", prefix, err)
	}
	if asset.DecodedBytes != 0 {
		if asset.DecodedBytes < decodedBytes {
			return rasterAssetValidation{}, fmt.Errorf(
				"%s declares a %d-byte decoded footprint below the required %d bytes",
				prefix, asset.DecodedBytes, decodedBytes,
			)
		}
		decodedBytes = asset.DecodedBytes
	}
	if err := validateDecodedAssetBudget(id, decodedBytes, availableBytes); err != nil {
		return rasterAssetValidation{}, err
	}
	if decodedBytes > platformMaxInt() {
		return rasterAssetValidation{}, fmt.Errorf("%s decoded storage exceeds the platform integer domain", prefix)
	}
	if err := ctx.Err(); err != nil {
		return rasterAssetValidation{}, err
	}
	return rasterAssetValidation{format: wantFormat, decodedBytes: decodedBytes}, nil
}

func validateDecodedAssetBudget(id d2scene.AssetID, decodedBytes, availableBytes int64) error {
	if availableBytes < 0 || decodedBytes > availableBytes {
		return fmt.Errorf(
			"d2raster: raster asset %q decoded storage requires %d bytes, exceeding the %d bytes remaining under MaxDecodedAssetBytes",
			id, decodedBytes, maxInt64(availableBytes, 0),
		)
	}
	return nil
}

func decodeRasterAsset(ctx context.Context, id d2scene.AssetID, asset d2scene.RasterAsset, validation rasterAssetValidation) (*preparedRasterAsset, error) {
	prefix := fmt.Sprintf("d2raster: raster asset %q", id)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	decoded, err := rasterimage.DecodeFirst(ctx, asset.Data, validation.format)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("%s has malformed %s pixel data: %w", prefix, strings.ToUpper(validation.format), err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != asset.PixelWidth || bounds.Dy() != asset.PixelHeight {
		return nil, fmt.Errorf(
			"%s decoded pixels are %dx%d, want declared dimensions %dx%d",
			prefix, bounds.Dx(), bounds.Dy(), asset.PixelWidth, asset.PixelHeight,
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &preparedRasterAsset{
		image: decoded, bounds: bounds, width: bounds.Dx(), height: bounds.Dy(),
	}, nil
}

func rasterFormatForMIME(mimeType string) (string, bool) {
	switch mimeType {
	case "image/png":
		return "png", true
	case "image/jpeg":
		return "jpeg", true
	case "image/gif":
		return "gif", true
	case "image/webp":
		return "webp", true
	default:
		return "", false
	}
}

func rasterDataFormat(data []byte) string {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")):
		return "png"
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return "jpeg"
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return "gif"
	case len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "webp"
	default:
		return ""
	}
}

func rasterDecodedBytes(config image.Config) (int64, error) {
	if config.Width <= 0 || config.Height <= 0 {
		return 0, fmt.Errorf("decoded dimensions %dx%d are invalid", config.Width, config.Height)
	}
	if int64(config.Width) > math.MaxInt64/int64(config.Height) {
		return 0, fmt.Errorf("decoded pixel count overflows int64")
	}
	pixels := int64(config.Width) * int64(config.Height)
	bytesPerPixel := int64(4)
	if config.ColorModel != nil {
		sample := config.ColorModel.Convert(color.RGBA64{R: 0x1234, G: 0x5678, B: 0x9abc, A: 0xffff})
		switch sample.(type) {
		case color.RGBA64, color.NRGBA64, color.Gray16, color.Alpha16:
			bytesPerPixel = 8
		}
	}
	if pixels > math.MaxInt64/bytesPerPixel {
		return 0, fmt.Errorf("decoded byte count overflows int64")
	}
	return pixels * bytesPerPixel, nil
}

func (p *preflight) image(nodeID string, primitive d2scene.Image, transform d2scene.Matrix, animation animationOverrides, depth, importDepth int) (*preparedPrimitive, error) {
	if animation.fillColor != nil || animation.strokeColor != nil || animation.dashOffset != nil {
		return nil, fmt.Errorf("d2raster: node %q image cannot be targeted by paint or stroke animation", nodeID)
	}
	if primitive.Asset == "" {
		return nil, fmt.Errorf("d2raster: node %q image has an empty asset ID", nodeID)
	}
	if err := validateBox(primitive.Box); err != nil {
		return nil, fmt.Errorf("d2raster: node %q image: %w", nodeID, err)
	}
	if primitive.Aspect.Align > d2scene.AlignXMaxYMax || primitive.Aspect.Fit > d2scene.AspectSlice {
		return nil, fmt.Errorf("d2raster: node %q image has invalid aspect-ratio policy align=%d fit=%d", nodeID, primitive.Aspect.Align, primitive.Aspect.Fit)
	}
	if asset, ok := p.rasters[primitive.Asset]; ok {
		return p.rasterImage(nodeID, primitive, transform, asset)
	}
	if asset, ok := p.vectors[primitive.Asset]; ok {
		return p.vectorImage(nodeID, primitive, transform, asset, depth, importDepth)
	}
	if raw, exists := p.document.Assets[primitive.Asset]; exists {
		return nil, fmt.Errorf("d2raster: node %q image asset %q is not a raster or vector asset (got %T)", nodeID, primitive.Asset, raw)
	}
	return nil, fmt.Errorf("d2raster: node %q image references missing asset %q", nodeID, primitive.Asset)
}

func (p *preflight) rasterImage(nodeID string, primitive d2scene.Image, transform d2scene.Matrix, asset *preparedRasterAsset) (*preparedPrimitive, error) {
	placement, err := imagePlacement(primitive.Box, asset.width, asset.height, primitive.Aspect)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q image aspect ratio: %w", nodeID, err)
	}
	inverse, invertible, err := makeInverseAffine(transform)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q image transform: %w", nodeID, err)
	}
	bounds, err := transformedImagePixelBounds(primitive.Box, transform, p.preparationBounds())
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q image geometry: %w", nodeID, err)
	}
	if !invertible || primitive.Box.Width == 0 || primitive.Box.Height == 0 {
		bounds = image.Rectangle{}
	}
	preparedImage := &preparedImage{
		asset: asset, box: primitive.Box, placement: placement, inverse: inverse, bounds: bounds,
	}
	return &preparedPrimitive{image: preparedImage, bounds: bounds}, nil
}

func (p *preflight) vectorImage(nodeID string, primitive d2scene.Image, transform d2scene.Matrix, asset d2scene.VectorAsset, depth, importDepth int) (*preparedPrimitive, error) {
	nextImportDepth := importDepth + 1
	if nextImportDepth > p.options.MaxImportDepth {
		return nil, fmt.Errorf(
			"d2raster: node %q vector asset %q import depth %d exceeds limit %d",
			nodeID, primitive.Asset, nextImportDepth, p.options.MaxImportDepth,
		)
	}
	if p.activeAssets[primitive.Asset] {
		return nil, fmt.Errorf("d2raster: node %q has cyclic vector asset reference at %q", nodeID, primitive.Asset)
	}
	emptyViewport := primitive.Box.Width == 0 || primitive.Box.Height == 0 || !finiteLinearTransformInvertible(transform)
	mappingDestination := primitive.Box
	mappingAspect := primitive.Aspect
	// An empty destination paints no pixels, but the referenced scene still
	// needs structural validation and per-instance budget accounting. Prepare
	// it in a canonical viewport so large intrinsic coordinates and the
	// necessarily singular real viewport do not become device-domain errors.
	// The empty outer clip discards all prepared bounds and resource work.
	if emptyViewport {
		// Keep each canonical scale in (0,1]. A fixed 1x1 target would make
		// 1/source overflow for a valid subnormal ViewBox dimension.
		mappingDestination = d2scene.Box{
			Width: math.Min(asset.ViewBox.Width, 1), Height: math.Min(asset.ViewBox.Height, 1),
		}
		mappingAspect = d2scene.AspectRatio{Align: d2scene.AlignNone}
	}
	mapping, err := d2scene.AspectRatioMatrix(asset.ViewBox, mappingDestination, mappingAspect)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q vector asset %q aspect ratio: %w", nodeID, primitive.Asset, err)
	}
	if !emptyViewport && !finiteLinearTransformInvertible(mapping) {
		return nil, fmt.Errorf("d2raster: node %q vector asset %q aspect ratio: mapping is singular in the finite numeric domain", nodeID, primitive.Asset)
	}
	assetTransform := transform.Mul(mapping)
	if emptyViewport {
		// The actual ancestor transform cannot affect an empty viewport. Avoid
		// letting a finite but device-sized ancestor influence accounting-only
		// preparation of the imported definition.
		assetTransform = mapping
	}
	if !assetTransform.IsFinite() {
		return nil, fmt.Errorf("d2raster: node %q vector asset %q has a non-finite composed transform", nodeID, primitive.Asset)
	}

	p.activeAssets[primitive.Asset] = true
	defer delete(p.activeAssets, primitive.Asset)
	root, err := p.node(asset.Root, assetTransform, depth+1, nextImportDepth)
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q vector asset %q: %w", nodeID, primitive.Asset, err)
	}
	clip := &preparedClip{}
	if !emptyViewport {
		clip, err = p.imageBoxClip(nodeID, primitive.Asset, primitive.Box, transform)
		if err != nil {
			return nil, err
		}
	}
	viewport := &preparedNode{
		opacity:  1,
		blend:    d2scene.BlendNormal,
		children: []*preparedNode{root},
		clip:     clip,
	}
	if root != nil {
		viewport.bounds = root.bounds.Intersect(clip.bounds)
	}
	return &preparedPrimitive{vector: viewport, bounds: viewport.bounds}, nil
}

// imageBoxClip constructs the implicit image viewport in device space. It is
// renderer-generated geometry, so its four edges intentionally do not consume
// MaxPathCommands; that limit counts instantiated scene work.
func (p *preflight) imageBoxClip(nodeID string, assetID d2scene.AssetID, box d2scene.Box, transform d2scene.Matrix) (*preparedClip, error) {
	local := [...]d2scene.Point{
		{X: box.X, Y: box.Y},
		{X: box.X + box.Width, Y: box.Y},
		{X: box.X + box.Width, Y: box.Y + box.Height},
		{X: box.X, Y: box.Y + box.Height},
	}
	points := make([]d2scene.Point, len(local))
	for index, point := range local {
		points[index] = transform.Point(point)
		if err := validateRasterPoint(points[index]); err != nil {
			return nil, fmt.Errorf("d2raster: node %q vector asset %q viewport geometry: %w", nodeID, assetID, err)
		}
	}
	bounds, err := transformedImagePixelBounds(box, transform, p.preparationBounds())
	if err != nil {
		return nil, fmt.Errorf("d2raster: node %q vector asset %q viewport geometry: %w", nodeID, assetID, err)
	}
	invertible := finiteLinearTransformInvertible(transform)
	if !invertible || box.Width == 0 || box.Height == 0 {
		bounds = image.Rectangle{}
	}
	return &preparedClip{
		subpaths: []subpath{{points: points, closed: true}},
		fillRule: d2scene.NonZero,
		bounds:   bounds,
		edges:    4,
	}, nil
}

func imagePlacement(box d2scene.Box, sourceWidth, sourceHeight int, aspect d2scene.AspectRatio) (d2scene.Box, error) {
	if aspect.Align == d2scene.AlignNone || box.Width == 0 || box.Height == 0 {
		return box, nil
	}
	sx := box.Width / float64(sourceWidth)
	sy := box.Height / float64(sourceHeight)
	scale := math.Min(sx, sy)
	if aspect.Fit == d2scene.AspectSlice {
		scale = math.Max(sx, sy)
	}
	width := float64(sourceWidth) * scale
	height := float64(sourceHeight) * scale
	if !finite(scale) || !finite(width) || !finite(height) || scale <= 0 {
		return d2scene.Box{}, fmt.Errorf("placement is outside the finite positive domain")
	}
	xFactor, yFactor, ok := aspectAlignmentFactors(aspect.Align)
	if !ok {
		return d2scene.Box{}, fmt.Errorf("unsupported alignment %d", aspect.Align)
	}
	return d2scene.Box{
		X:      box.X + (box.Width-width)*xFactor,
		Y:      box.Y + (box.Height-height)*yFactor,
		Width:  width,
		Height: height,
	}, validateImagePlacement(box, width, height, xFactor, yFactor)
}

func validateImagePlacement(box d2scene.Box, width, height, xFactor, yFactor float64) error {
	x := box.X + (box.Width-width)*xFactor
	y := box.Y + (box.Height-height)*yFactor
	if !finite(x) || !finite(y) {
		return fmt.Errorf("placement origin is outside the finite numeric domain")
	}
	return nil
}

func aspectAlignmentFactors(align d2scene.AspectAlign) (x, y float64, ok bool) {
	switch align {
	case d2scene.AlignXMinYMin:
		return 0, 0, true
	case d2scene.AlignXMidYMin:
		return .5, 0, true
	case d2scene.AlignXMaxYMin:
		return 1, 0, true
	case d2scene.AlignXMinYMid:
		return 0, .5, true
	case d2scene.AlignXMidYMid:
		return .5, .5, true
	case d2scene.AlignXMaxYMid:
		return 1, .5, true
	case d2scene.AlignXMinYMax:
		return 0, 1, true
	case d2scene.AlignXMidYMax:
		return .5, 1, true
	case d2scene.AlignXMaxYMax:
		return 1, 1, true
	default:
		return 0, 0, false
	}
}

func makeInverseAffine(transform d2scene.Matrix) (inverseAffine, bool, error) {
	maximum := math.Max(
		math.Max(math.Abs(transform.A), math.Abs(transform.B)),
		math.Max(math.Abs(transform.C), math.Abs(transform.D)),
	)
	if !finite(maximum) {
		return inverseAffine{}, false, fmt.Errorf("linear coefficients are non-finite")
	}
	if maximum == 0 {
		return inverseAffine{}, false, nil
	}
	a := transform.A / maximum
	b := transform.B / maximum
	c := transform.C / maximum
	d := transform.D / maximum
	determinant := a*d - b*c
	if determinant == 0 {
		return inverseAffine{}, false, nil
	}
	inverse := inverseAffine{
		a: (d / determinant) / maximum,
		b: (-b / determinant) / maximum,
		c: (-c / determinant) / maximum,
		d: (a / determinant) / maximum,
		e: transform.E,
		f: transform.F,
	}
	if !finite(inverse.a) || !finite(inverse.b) || !finite(inverse.c) || !finite(inverse.d) {
		return inverseAffine{}, false, fmt.Errorf("inverse is outside the finite numeric domain")
	}
	return inverse, true, nil
}

func (inverse inverseAffine) point(point d2scene.Point) (d2scene.Point, bool) {
	dx := point.X - inverse.e
	dy := point.Y - inverse.f
	local := d2scene.Point{
		X: math.FMA(inverse.a, dx, inverse.c*dy),
		Y: math.FMA(inverse.b, dx, inverse.d*dy),
	}
	return local, finitePoint(local)
}

func transformedImagePixelBounds(box d2scene.Box, transform d2scene.Matrix, canvas image.Rectangle) (image.Rectangle, error) {
	points := [...]d2scene.Point{
		transform.Point(d2scene.Point{X: box.X, Y: box.Y}),
		transform.Point(d2scene.Point{X: box.X + box.Width, Y: box.Y}),
		transform.Point(d2scene.Point{X: box.X + box.Width, Y: box.Y + box.Height}),
		transform.Point(d2scene.Point{X: box.X, Y: box.Y + box.Height}),
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, point := range points {
		if err := validateRasterPoint(point); err != nil {
			return image.Rectangle{}, err
		}
		minX = math.Min(minX, point.X)
		minY = math.Min(minY, point.Y)
		maxX = math.Max(maxX, point.X)
		maxY = math.Max(maxY, point.Y)
	}
	if maxX <= float64(canvas.Min.X) || maxY <= float64(canvas.Min.Y) || minX >= float64(canvas.Max.X) || minY >= float64(canvas.Max.Y) {
		return image.Rectangle{}, nil
	}
	minX = math.Max(minX, float64(canvas.Min.X))
	minY = math.Max(minY, float64(canvas.Min.Y))
	maxX = math.Min(maxX, float64(canvas.Max.X))
	maxY = math.Min(maxY, float64(canvas.Max.Y))
	return image.Rect(
		int(math.Floor(minX)), int(math.Floor(minY)),
		int(math.Ceil(maxX)), int(math.Ceil(maxY)),
	).Intersect(canvas), nil
}

func drawPreparedImage(ctx context.Context, destination *image.RGBA, prepared *preparedImage) error {
	if prepared == nil || prepared.asset == nil {
		return fmt.Errorf("d2raster: internal image primitive is missing its decoded asset")
	}
	bounds := prepared.bounds.Intersect(destination.Bounds())
	if bounds.Empty() {
		return ctx.Err()
	}
	const sampleCount = imageSamplesPerAxis * imageSamplesPerAxis
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if (x-bounds.Min.X)&255 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			validSamples := 0
			sourceXSum, sourceYSum := 0.0, 0.0
			for sampleY := 0; sampleY < imageSamplesPerAxis; sampleY++ {
				for sampleX := 0; sampleX < imageSamplesPerAxis; sampleX++ {
					device := d2scene.Point{
						X: float64(x) + (float64(sampleX)+.5)/imageSamplesPerAxis,
						Y: float64(y) + (float64(sampleY)+.5)/imageSamplesPerAxis,
					}
					local, ok := prepared.inverse.point(device)
					if !ok || !pointInImageBox(local, prepared.box) {
						continue
					}
					sourceX := (local.X - prepared.placement.X) / prepared.placement.Width * float64(prepared.asset.width)
					sourceY := (local.Y - prepared.placement.Y) / prepared.placement.Height * float64(prepared.asset.height)
					if !finite(sourceX) || !finite(sourceY) || sourceX < 0 || sourceY < 0 || sourceX >= float64(prepared.asset.width) || sourceY >= float64(prepared.asset.height) {
						continue
					}
					validSamples++
					sourceXSum += sourceX
					sourceYSum += sourceY
				}
			}
			if validSamples == 0 {
				continue
			}
			// Affine source coordinates average to the pixel center for fully
			// covered pixels, preserving exact 1:1 texels. At transformed or
			// clipped edges, average only covered samples and apply their
			// coverage after interpolation.
			sample := bilinearPremultiplied(
				prepared.asset,
				sourceXSum/float64(validSamples),
				sourceYSum/float64(validSamples),
			)
			var source [4]uint32
			for channel := range source {
				source[channel] = uint32((uint64(sample[channel])*uint64(validSamples) + sampleCount/2) / sampleCount)
			}
			if source[3] == 0 {
				continue
			}
			offset := destination.PixOffset(x, y)
			compositePremultipliedRGBA64(destination.Pix[offset:offset+4], source)
		}
	}
	return ctx.Err()
}

func pointInImageBox(point d2scene.Point, box d2scene.Box) bool {
	return point.X >= box.X && point.Y >= box.Y &&
		point.X < box.X+box.Width && point.Y < box.Y+box.Height
}

func bilinearPremultiplied(asset *preparedRasterAsset, x, y float64) [4]uint32 {
	centerX := x - .5
	centerY := y - .5
	x0 := int(math.Floor(centerX))
	y0 := int(math.Floor(centerY))
	weightX := centerX - float64(x0)
	weightY := centerY - float64(y0)
	x1, y1 := x0+1, y0+1
	x0 = clampInt(x0, 0, asset.width-1)
	x1 = clampInt(x1, 0, asset.width-1)
	y0 = clampInt(y0, 0, asset.height-1)
	y1 = clampInt(y1, 0, asset.height-1)
	c00 := rgba64(asset.image.At(asset.bounds.Min.X+x0, asset.bounds.Min.Y+y0))
	c10 := rgba64(asset.image.At(asset.bounds.Min.X+x1, asset.bounds.Min.Y+y0))
	c01 := rgba64(asset.image.At(asset.bounds.Min.X+x0, asset.bounds.Min.Y+y1))
	c11 := rgba64(asset.image.At(asset.bounds.Min.X+x1, asset.bounds.Min.Y+y1))
	var result [4]uint32
	for channel := range result {
		top := float64(c00[channel]) + (float64(c10[channel])-float64(c00[channel]))*weightX
		bottom := float64(c01[channel]) + (float64(c11[channel])-float64(c01[channel]))*weightX
		value := top + (bottom-top)*weightY
		result[channel] = uint32(math.Round(math.Max(0, math.Min(65535, value))))
	}
	return result
}

func rgba64(value color.Color) [4]uint32 {
	r, g, b, a := value.RGBA()
	return [4]uint32{r, g, b, a}
}

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func compositePremultipliedRGBA64(destination []byte, source [4]uint32) {
	toByte := func(value uint32) uint32 { return (value + 128) / 257 }
	sourceAlpha := toByte(source[3])
	if sourceAlpha == 0 {
		return
	}
	inverseAlpha := 255 - sourceAlpha
	mul255 := func(left, right uint32) uint32 { return (left*right + 127) / 255 }
	for channel := 0; channel < 3; channel++ {
		sourceChannel := toByte(source[channel])
		if sourceChannel > sourceAlpha {
			sourceChannel = sourceAlpha
		}
		result := sourceChannel + mul255(uint32(destination[channel]), inverseAlpha)
		if result > 255 {
			result = 255
		}
		destination[channel] = uint8(result)
	}
	alpha := sourceAlpha + mul255(uint32(destination[3]), inverseAlpha)
	if alpha > 255 {
		alpha = 255
	}
	destination[3] = uint8(alpha)
}
