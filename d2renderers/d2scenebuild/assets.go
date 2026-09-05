package d2scenebuild

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image/color"
	"sort"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/geo"
	"github.com/d2lang/d2/lib/imageasset"
	"github.com/d2lang/d2/lib/label"
)

var defaultImageAspect = d2scene.AspectRatio{
	Align: d2scene.AlignXMidYMid,
	Fit:   d2scene.AspectMeet,
}

const unavailableImageAssetID d2scene.AssetID = "image:unavailable"

func (b *builder) preflightAssets() error {
	for _, targetShape := range b.diagram.Shapes {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if !shapeAssetIsEmitted(targetShape) {
			continue
		}
		if _, err := b.resolveImageAsset(fmt.Sprintf("shape %q", targetShape.ID), targetShape.Icon.String()); err != nil {
			return err
		}
	}
	for _, targetShape := range b.diagram.Shapes {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if targetShape.Opacity == 0 || targetShape.Label == "" || targetShape.Language != "latex" {
			continue
		}
		if _, err := b.resolveLatexAsset(fmt.Sprintf("shape %q", targetShape.ID), targetShape.Label, targetShape.Stroke); err != nil {
			return err
		}
	}
	for _, connection := range b.diagram.Connections {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if connection.Icon == nil {
			continue
		}
		if _, err := b.resolveImageAsset(fmt.Sprintf("connection %q", connection.ID), connection.Icon.String()); err != nil {
			return err
		}
	}
	for _, connection := range b.diagram.Connections {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		if connection.Opacity == 0 || connection.Label == "" || connection.Language != "latex" {
			continue
		}
		if _, err := b.resolveLatexAsset(fmt.Sprintf("connection %q", connection.ID), connection.Label, connection.Color); err != nil {
			return err
		}
	}
	if b.diagram.Legend != nil {
		for index, targetShape := range b.diagram.Legend.Shapes {
			if err := b.ctx.Err(); err != nil {
				return err
			}
			if targetShape.Label == "" {
				continue
			}
			icon := legendShapeIconTarget(index, targetShape)
			if !shapeAssetIsEmitted(icon) {
				continue
			}
			if _, err := b.resolveImageAsset(legendShapeObject(index, targetShape.ID), icon.Icon.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

// shapeAssetIsEmitted reports whether a shape contributes an image asset.
// Image, class, and table shapes retain their image when opacity is zero;
// ordinary and code icons do not.
func shapeAssetIsEmitted(targetShape d2target.Shape) bool {
	if targetShape.Icon == nil {
		return false
	}
	return targetShape.Opacity != 0 || targetShape.Type == d2target.ShapeImage ||
		targetShape.Type == d2target.ShapeClass || targetShape.Type == d2target.ShapeSQLTable
}

func (b *builder) resolveImageAsset(object, source string) (d2scene.AssetID, error) {
	if id := b.sourceAssetIDs[source]; id != "" {
		return id, nil
	}
	if b.options.Assets == nil || b.options.Assets.Resolver == nil {
		return "", unsupported(object, "image/icon asset without a configured resolver")
	}
	resource, err := b.options.Assets.Resolver.Resolve(b.ctx, source)
	if err != nil {
		if ctxErr := b.ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if errors.Is(err, imageasset.ErrUnavailable) {
			return b.resolveUnavailableImageAsset(source), nil
		}
		return "", fmt.Errorf("scene: %s image asset: %w", object, err)
	}
	if resource == nil {
		return "", fmt.Errorf("scene: %s image asset resolver returned nil", object)
	}
	data, err := resource.BytesContext(b.ctx)
	if err != nil {
		return "", fmt.Errorf("scene: %s image asset copy: %w", object, err)
	}
	digest, err := hashImageAsset(b.ctx, resource.Kind(), resource.MIMEType(), data)
	if err != nil {
		return "", fmt.Errorf("scene: %s image asset hash: %w", object, err)
	}
	if id := b.assetIDs[digest]; id != "" {
		b.sourceAssetIDs[source] = id
		return id, nil
	}

	id := d2scene.AssetID(fmt.Sprintf("image:%x", digest[:]))
	var asset d2scene.Asset
	switch resource.Kind() {
	case imageasset.KindRaster:
		asset = d2scene.RasterAsset{
			MIMEType:     resource.MIMEType(),
			Data:         data,
			PixelWidth:   resource.PixelWidth(),
			PixelHeight:  resource.PixelHeight(),
			DecodedBytes: resource.DecodedBytes(),
		}
	case imageasset.KindSVG:
		limits, err := b.remainingSVGImportLimits(object)
		if err != nil {
			return "", err
		}
		result, err := d2svgimport.ImportNode(b.ctx, source, data, limits)
		if err != nil {
			return "", fmt.Errorf("scene: %s SVG asset: %w", object, err)
		}
		if err := b.reserveSVGImport(object, result.Metrics); err != nil {
			return "", err
		}
		if err := b.mergeSVGImportAssets(object, result.Assets); err != nil {
			return "", err
		}
		content := d2scene.NewNode(nil)
		content.ID = string(id) + ":content"
		content.Transform = result.ViewportTransform
		content.Children = []*d2scene.Node{result.Root}

		viewport := d2scene.Box{Width: result.Width, Height: result.Height}
		root := d2scene.NewNode(nil)
		root.ID = string(id) + ":viewport"
		root.Clip = boxClip(viewport)
		root.Children = []*d2scene.Node{content}
		asset = d2scene.VectorAsset{ViewBox: viewport, Root: root}
	default:
		return "", fmt.Errorf("scene: %s image asset has unsupported resolved kind %s", object, resource.Kind())
	}
	b.assets[id] = asset
	b.assetIDs[digest] = id
	b.sourceAssetIDs[source] = id
	return id, nil
}

func (b *builder) resolveUnavailableImageAsset(source string) d2scene.AssetID {
	if _, ok := b.assets[unavailableImageAssetID]; !ok {
		b.assets[unavailableImageAssetID] = newUnavailableImageAsset()
	}
	b.sourceAssetIDs[source] = unavailableImageAssetID
	return unavailableImageAssetID
}

func newUnavailableImageAsset() d2scene.VectorAsset {
	pageFill := d2scene.SolidPaint{Color: color.NRGBA{R: 0xf7, G: 0xf8, B: 0xfa, A: 0xff}}
	outlinePaint := d2scene.SolidPaint{Color: color.NRGBA{R: 0x7b, G: 0x83, B: 0x8c, A: 0xff}}
	outline := func(width float64) *d2scene.Stroke {
		return &d2scene.Stroke{Paint: outlinePaint, Width: width, Cap: d2scene.CapRound, Join: d2scene.JoinRound}
	}

	page := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(7, 3), d2scene.LineTo(42, 3), d2scene.LineTo(57, 18),
			d2scene.LineTo(57, 61), d2scene.LineTo(7, 61), d2scene.ClosePath(),
		},
		Fill: pageFill, Stroke: outline(2),
	})
	page.ID = "image-unavailable:page"

	sky := d2scene.NewNode(d2scene.Rect{
		Box:  d2scene.Box{X: 13, Y: 23, Width: 38, Height: 30},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 0xb8, G: 0xd7, B: 0xf2, A: 0xff}},
	})
	sky.ID = "image-unavailable:sky"

	sun := d2scene.NewNode(d2scene.Ellipse{
		Center: d2scene.Point{X: 23, Y: 31}, RadiusX: 4, RadiusY: 4,
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}},
	})
	sun.ID = "image-unavailable:sun"

	hills := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(13, 49), d2scene.LineTo(24, 37), d2scene.LineTo(32, 45),
			d2scene.LineTo(39, 39), d2scene.LineTo(51, 49), d2scene.LineTo(51, 53),
			d2scene.LineTo(13, 53), d2scene.ClosePath(),
		},
		Fill: d2scene.SolidPaint{Color: color.NRGBA{R: 0x55, G: 0xb9, B: 0x4b, A: 0xff}},
	})
	hills.ID = "image-unavailable:hills"

	breakMark := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{d2scene.MoveTo(37, 54), d2scene.LineTo(51, 40)},
		Stroke:   &d2scene.Stroke{Paint: pageFill, Width: 5, Cap: d2scene.CapSquare, Join: d2scene.JoinRound},
	})
	breakMark.ID = "image-unavailable:break"

	imageOutline := d2scene.NewNode(d2scene.Rect{
		Box: d2scene.Box{X: 13, Y: 23, Width: 38, Height: 30}, Stroke: outline(1),
	})
	imageOutline.ID = "image-unavailable:image-outline"

	fold := d2scene.NewNode(d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(42, 3), d2scene.LineTo(42, 18), d2scene.LineTo(57, 18),
		},
		Stroke: outline(2),
	})
	fold.ID = "image-unavailable:fold"

	root := d2scene.NewNode(nil)
	root.ID = "image-unavailable"
	root.Children = []*d2scene.Node{page, sky, sun, hills, breakMark, imageOutline, fold}
	return d2scene.VectorAsset{ViewBox: d2scene.Box{Width: 64, Height: 64}, Root: root}
}

// mergeSVGImportAssets transfers the importer's owned, immutable embedded
// raster resources into the document asset table. Sorting makes cancellation
// and collision failures deterministic even though Result.Assets is a map.
// The importer content-addresses these resources, so an existing equal entry
// is a normal cross-SVG deduplication while unequal content is an invariant
// violation rather than something that may be silently overwritten.
func (b *builder) mergeSVGImportAssets(object string, imported map[d2scene.AssetID]d2scene.Asset) error {
	ids := make([]string, 0, len(imported))
	for id := range imported {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	type pendingAsset struct {
		id    d2scene.AssetID
		asset d2scene.RasterAsset
	}
	pending := make([]pendingAsset, 0, len(ids))
	for _, rawID := range ids {
		if err := b.ctx.Err(); err != nil {
			return err
		}
		id := d2scene.AssetID(rawID)
		asset, ok := imported[id].(d2scene.RasterAsset)
		if !ok {
			return fmt.Errorf("scene: %s SVG asset %q has unsupported imported resource type %T", object, id, imported[id])
		}
		if existing, exists := b.assets[id]; exists {
			existingRaster, ok := existing.(d2scene.RasterAsset)
			if !ok || !equalRasterAsset(existingRaster, asset) {
				return fmt.Errorf("scene: %s SVG asset %q collides with a different document resource", object, id)
			}
			continue
		}
		pending = append(pending, pendingAsset{id: id, asset: asset})
	}
	for _, entry := range pending {
		b.assets[entry.id] = entry.asset
	}
	return nil
}

func equalRasterAsset(left, right d2scene.RasterAsset) bool {
	return left.MIMEType == right.MIMEType && left.PixelWidth == right.PixelWidth &&
		left.PixelHeight == right.PixelHeight && left.DecodedBytes == right.DecodedBytes &&
		bytes.Equal(left.Data, right.Data)
}

func hashImageAsset(ctx context.Context, kind imageasset.Kind, mimeType string, data []byte) ([sha256.Size]byte, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte{byte(kind), 0})
	_, _ = hash.Write([]byte(mimeType))
	_, _ = hash.Write([]byte{0})
	const chunkBytes = 32 << 10
	for offset := 0; offset < len(data); offset += chunkBytes {
		if err := ctx.Err(); err != nil {
			return [sha256.Size]byte{}, err
		}
		end := offset + chunkBytes
		if end > len(data) {
			end = len(data)
		}
		_, _ = hash.Write(data[offset:end])
	}
	if err := ctx.Err(); err != nil {
		return [sha256.Size]byte{}, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func (b *builder) remainingSVGImportLimits(object string) (d2svgimport.Limits, error) {
	budget := b.options.Assets.SVGImportBudget
	if budget.MaxSourceBytes <= 0 || budget.MaxElements <= 0 || budget.MaxAttributes <= 0 || budget.MaxAttributeBytes <= 0 ||
		budget.MaxPathCommands <= 0 || budget.MaxTransformFunctions <= 0 ||
		budget.MaxDeclaredResources <= 0 || budget.MaxExpandedUseInstances <= 0 {
		return d2svgimport.Limits{}, fmt.Errorf("scene: %s SVG asset requires positive document-wide import budgets", object)
	}
	type remainingLimit struct {
		name      string
		remaining int
		apply     func(*d2svgimport.Limits, int)
	}
	limits := b.options.Assets.SVGImportLimits
	remaining := []remainingLimit{
		{"source bytes", budget.MaxSourceBytes - b.svgSourceBytes, func(value *d2svgimport.Limits, maximum int) { value.MaxBytes = minInt(value.MaxBytes, maximum) }},
		{"element count", budget.MaxElements - b.svgElements, func(value *d2svgimport.Limits, maximum int) { value.MaxElements = minInt(value.MaxElements, maximum) }},
		{"attribute count", budget.MaxAttributes - b.svgAttributes, func(value *d2svgimport.Limits, maximum int) {
			value.MaxAttributes = minInt(value.MaxAttributes, maximum)
		}},
		{"attribute bytes", budget.MaxAttributeBytes - b.svgAttributeBytes, func(value *d2svgimport.Limits, maximum int) {
			value.MaxAttributeBytes = minInt(value.MaxAttributeBytes, maximum)
		}},
		{"path command count", budget.MaxPathCommands - b.svgCommands, func(value *d2svgimport.Limits, maximum int) {
			value.MaxPathCommands = minInt(value.MaxPathCommands, maximum)
		}},
		{"transform function count", budget.MaxTransformFunctions - b.svgTransforms, func(value *d2svgimport.Limits, maximum int) {
			value.MaxTransformFunctions = minInt(value.MaxTransformFunctions, maximum)
		}},
		{"declared resource count", budget.MaxDeclaredResources - b.svgDeclaredResources, func(value *d2svgimport.Limits, maximum int) { value.MaxResources = minInt(value.MaxResources, maximum) }},
		{"expanded use instance count", budget.MaxExpandedUseInstances - b.svgExpandedUseInstances, func(value *d2svgimport.Limits, maximum int) { value.MaxResources = minInt(value.MaxResources, maximum) }},
	}
	for _, item := range remaining {
		if item.remaining <= 0 {
			return d2svgimport.Limits{}, fmt.Errorf("scene: %s SVG asset has no remaining cumulative %s budget", object, item.name)
		}
		item.apply(&limits, item.remaining)
	}
	return limits, nil
}

func (b *builder) reserveSVGImport(object string, metrics d2svgimport.Metrics) error {
	budget := b.options.Assets.SVGImportBudget
	elements := maxInt(metrics.ParsedElements, metrics.EmittedElements)
	commands := maxInt(metrics.ParsedPathCommands, metrics.EmittedPathCommands)
	type charge struct {
		name   string
		amount int
		used   int
		limit  int
	}
	charges := []charge{
		{"source bytes", metrics.SourceBytes, b.svgSourceBytes, budget.MaxSourceBytes},
		{"element count", elements, b.svgElements, budget.MaxElements},
		{"attribute count", metrics.ParsedAttributes, b.svgAttributes, budget.MaxAttributes},
		{"attribute bytes", metrics.ParsedAttributeBytes, b.svgAttributeBytes, budget.MaxAttributeBytes},
		{"path command count", commands, b.svgCommands, budget.MaxPathCommands},
		{"transform function count", metrics.ParsedTransformFuncs, b.svgTransforms, budget.MaxTransformFunctions},
		{"declared resource count", metrics.DeclaredResources, b.svgDeclaredResources, budget.MaxDeclaredResources},
		{"expanded use instance count", metrics.ExpandedUseInstances, b.svgExpandedUseInstances, budget.MaxExpandedUseInstances},
	}
	for _, item := range charges {
		if item.amount > item.limit-item.used {
			return fmt.Errorf("scene: %s SVG asset causes cumulative %s to exceed limit %d", object, item.name, item.limit)
		}
	}
	b.svgSourceBytes += metrics.SourceBytes
	b.svgElements += elements
	b.svgAttributes += metrics.ParsedAttributes
	b.svgAttributeBytes += metrics.ParsedAttributeBytes
	b.svgCommands += commands
	b.svgTransforms += metrics.ParsedTransformFuncs
	b.svgDeclaredResources += metrics.DeclaredResources
	b.svgExpandedUseInstances += metrics.ExpandedUseInstances
	return nil
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func (b *builder) buildImageNode(object, nodeID, source string, box d2scene.Box, radius float64, svgImageShapeClip bool) (*d2scene.Node, error) {
	assetID, err := b.resolveImageAsset(object, source)
	if err != nil {
		return nil, err
	}
	node := d2scene.NewNode(d2scene.Image{Asset: assetID, Box: box, Aspect: defaultImageAspect})
	node.ID = nodeID
	radius = clampBorderRadius(radius, box.Width, box.Height)
	if radius > 0 {
		if svgImageShapeClip {
			node.Clip = imageShapeClip(box, radius)
		} else {
			node.Clip = roundedBoxClip(box, radius)
		}
	}
	return node, nil
}

func (b *builder) buildShapeImage(targetShape d2target.Shape) (*d2scene.Node, error) {
	object := fmt.Sprintf("shape %q", targetShape.ID)
	if targetShape.Icon == nil {
		return nil, invalidField(object, "icon", nil, "must identify an image source for an image shape")
	}
	return b.buildImageNode(object, targetShape.ID+":image", targetShape.Icon.String(), d2scene.Box{
		X: float64(targetShape.Pos.X), Y: float64(targetShape.Pos.Y),
		Width: float64(targetShape.Width), Height: float64(targetShape.Height),
	}, float64(targetShape.IconBorderRadius), true)
}

func (b *builder) buildShapeIcon(targetShape d2target.Shape, structured bool) (*d2scene.Node, error) {
	if targetShape.Icon == nil || targetShape.Type == d2target.ShapeImage {
		return nil, nil
	}
	object := fmt.Sprintf("shape %q", targetShape.ID)
	position := label.FromString(targetShape.IconPosition)
	if !position.IsShapePosition() {
		return nil, invalidField(object, "iconPosition", targetShape.IconPosition, "must identify a supported icon position")
	}
	var box *geo.Box
	if structured {
		// Class and SQL table rendering uses the raw target box, including for
		// positions that ordinary shapes place in an inner box.
		box = geo.NewBox(
			geo.NewPoint(float64(targetShape.Pos.X), float64(targetShape.Pos.Y)),
			float64(targetShape.Width), float64(targetShape.Height),
		)
	} else {
		geometry := targetGeometry(targetShape)
		if position.IsOutside() {
			box = geometry.GetBox()
		} else {
			box = geometry.GetInnerBox()
		}
	}
	size := d2target.GetIconSize(box, targetShape.IconPosition)
	topLeft := position.GetPointOnBox(box, label.PADDING, float64(size), float64(size))
	if topLeft == nil {
		return nil, invalidField(object, "iconPosition", targetShape.IconPosition, "must identify a supported icon position")
	}
	radius := float64(targetShape.IconBorderRadius)
	if structured {
		// The dedicated class/table SVG paths do not apply IconBorderRadius.
		radius = 0
	}
	return b.buildImageNode(object, targetShape.ID+":icon", targetShape.Icon.String(), d2scene.Box{
		X: topLeft.X, Y: topLeft.Y, Width: float64(size), Height: float64(size),
	}, radius, false)
}

func (b *builder) buildConnectionIcon(connection d2target.Connection) (*d2scene.Node, error) {
	if connection.Icon == nil {
		return nil, nil
	}
	object := fmt.Sprintf("connection %q", connection.ID)
	topLeft := connection.GetIconPosition()
	if topLeft == nil {
		return nil, invalidField(object, "iconPosition", connection.IconPosition, "does not resolve on the route")
	}
	return b.buildImageNode(object, connection.ID+":icon", connection.Icon.String(), d2scene.Box{
		X: topLeft.X, Y: topLeft.Y,
		Width: d2target.DEFAULT_ICON_SIZE, Height: d2target.DEFAULT_ICON_SIZE,
	}, connection.IconBorderRadius, false)
}

func boxClip(box d2scene.Box) *d2scene.Clip {
	return &d2scene.Clip{Path: d2scene.Path{Commands: []d2scene.PathCommand{
		d2scene.MoveTo(box.X, box.Y),
		d2scene.LineTo(box.X+box.Width, box.Y),
		d2scene.LineTo(box.X+box.Width, box.Y+box.Height),
		d2scene.LineTo(box.X, box.Y+box.Height),
		d2scene.ClosePath(),
	}}, Transform: d2scene.Identity()}
}

func roundedBoxClip(box d2scene.Box, radius float64) *d2scene.Clip {
	left, top := box.X, box.Y
	right, bottom := box.X+box.Width, box.Y+box.Height
	return &d2scene.Clip{Path: d2scene.Path{Commands: []d2scene.PathCommand{
		d2scene.MoveTo(left+radius, top),
		d2scene.LineTo(right-radius, top),
		d2scene.ArcTo(radius, radius, 0, false, true, right, top+radius),
		d2scene.LineTo(right, bottom-radius),
		d2scene.ArcTo(radius, radius, 0, false, true, right-radius, bottom),
		d2scene.LineTo(left+radius, bottom),
		d2scene.ArcTo(radius, radius, 0, false, true, left, bottom-radius),
		d2scene.LineTo(left, top+radius),
		d2scene.ArcTo(radius, radius, 0, false, true, left+radius, top),
		d2scene.ClosePath(),
	}}, Transform: d2scene.Identity()}
}

// imageShapeClip uses smooth cubics whose first control is the current endpoint
// after each line, rather than true quarter-circle arcs.
func imageShapeClip(box d2scene.Box, radius float64) *d2scene.Clip {
	left, top := box.X, box.Y
	right, bottom := box.X+box.Width, box.Y+box.Height
	return &d2scene.Clip{Path: d2scene.Path{Commands: []d2scene.PathCommand{
		d2scene.MoveTo(left, top+radius),
		d2scene.LineTo(left, top+radius),
		d2scene.CubicTo(left, top+radius, left, top, left+radius, top),
		d2scene.LineTo(right-radius, top),
		d2scene.LineTo(right-radius, top),
		d2scene.CubicTo(right-radius, top, right, top, right, top+radius),
		d2scene.LineTo(right, bottom-radius),
		d2scene.CubicTo(right, bottom-radius, right, bottom, right-radius, bottom),
		d2scene.LineTo(left+radius, bottom),
		d2scene.CubicTo(left+radius, bottom, left, bottom, left, bottom-radius),
		d2scene.LineTo(left, top+radius),
		d2scene.ClosePath(),
	}}, Transform: d2scene.Identity()}
}
