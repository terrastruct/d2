package d2isometricimg

import (
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"math"
	"net/url"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/d2svgimport"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/lib/imageasset"
	"github.com/d2lang/d2/lib/label"
)

const (
	maxSurfaceIcons         = 7000
	maxSurfaceIconPixels    = 8 << 20
	maxSurfaceIconDimension = 512
	maxSurfaceIconBytes     = 16 << 20
	maxSurfaceIconDecoded   = 64 << 20
)

type surfaceIconKey struct {
	source        [32]byte
	width, height int
	radius        uint64
}

type surfaceIconAsset struct {
	asset  d2scene.Asset
	assets map[d2scene.AssetID]d2scene.Asset
}

// surfaceIconPainter owns one render's immutable icon assets and textures. It
// is used serially while constructing meshes, before any animation frames.
// A supplied resolver is the only authority for external asset I/O. The
// default resolver is guarded to accept already embedded data URIs only.
type surfaceIconPainter struct {
	ctx                   context.Context
	resolver              *imageasset.Resolver
	dataOnly              bool
	count, dimension      int
	outputDensity         float64
	pixels, bytes, decode int64
	limits                d2svgimport.Limits
	remaining             d2scenebuild.SVGImportBudget
	sources               map[[32]byte]*surfaceIconAsset
	content               map[[32]byte]*surfaceIconAsset
	textures              map[surfaceIconKey]*image.RGBA
}

func newSurfaceIconPainter(ctx context.Context, count int, assets *d2scenebuild.AssetOptions) (*surfaceIconPainter, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if count < 0 || count > maxSurfaceIcons {
		return nil, fmt.Errorf("isometric surface icon count exceeds %d", maxSurfaceIcons)
	}
	p := &surfaceIconPainter{
		ctx: ctx, count: max(1, count), dataOnly: true,
		limits:    d2svgimport.Limits{MaxBytes: 2 << 20, MaxDepth: 64, MaxElements: 20000, MaxAttributes: 80000, MaxAttributeBytes: 2 << 20, MaxPathCommands: 100000, MaxTransformFunctions: 40000, MaxUseDepth: 32, MaxResources: 4096},
		remaining: d2scenebuild.SVGImportBudget{MaxSourceBytes: maxSurfaceIconBytes, MaxElements: 100000, MaxAttributes: 400000, MaxAttributeBytes: 8 << 20, MaxPathCommands: 500000, MaxTransformFunctions: 200000, MaxDeclaredResources: 16000, MaxExpandedUseInstances: 16000},
		sources:   make(map[[32]byte]*surfaceIconAsset), content: make(map[[32]byte]*surfaceIconAsset), textures: make(map[surfaceIconKey]*image.RGBA),
	}
	p.dimension = min(maxSurfaceIconDimension, int(math.Sqrt(float64(maxSurfaceIconPixels/p.count))))
	if assets != nil {
		p.resolver = assets.Resolver
		p.dataOnly = assets.Resolver == nil
		// Honor stricter caller limits while retaining native render ceilings.
		p.limitTo(assets.SVGImportLimits, assets.SVGImportBudget)
	}
	if p.resolver == nil {
		var err error
		p.resolver, err = imageasset.New(imageasset.Options{Limits: imageasset.Limits{
			MaxFetchedBytes: 12 << 20, MaxEncodedBytes: 8 << 20, MaxDecompressedBytes: 8 << 20, MaxSVGBytes: int64(p.limits.MaxBytes),
			MaxDecodedWidth: 8192, MaxDecodedHeight: 8192, MaxDecodedPixels: 8 << 20,
			MaxAssets: maxSurfaceIcons, MaxCumulativeEncodedBytes: maxSurfaceIconBytes, MaxCumulativeDecodedBytes: maxSurfaceIconDecoded,
		}})
		if err != nil {
			return nil, err
		}
	}
	return p, nil
}

func iconLimit(value, supplied int) int {
	if supplied > 0 {
		return min(value, supplied)
	}
	return value
}

func (p *surfaceIconPainter) configureOutputDensity(pixelsPerWorld float64) {
	p.outputDensity = pixelsPerWorld
}

func (p *surfaceIconPainter) limitTo(l d2svgimport.Limits, b d2scenebuild.SVGImportBudget) {
	p.limits.MaxBytes = iconLimit(p.limits.MaxBytes, l.MaxBytes)
	p.limits.MaxDepth = iconLimit(p.limits.MaxDepth, l.MaxDepth)
	p.limits.MaxElements = iconLimit(p.limits.MaxElements, l.MaxElements)
	p.limits.MaxAttributes = iconLimit(p.limits.MaxAttributes, l.MaxAttributes)
	p.limits.MaxAttributeBytes = iconLimit(p.limits.MaxAttributeBytes, l.MaxAttributeBytes)
	p.limits.MaxPathCommands = iconLimit(p.limits.MaxPathCommands, l.MaxPathCommands)
	p.limits.MaxTransformFunctions = iconLimit(p.limits.MaxTransformFunctions, l.MaxTransformFunctions)
	p.limits.MaxUseDepth = iconLimit(p.limits.MaxUseDepth, l.MaxUseDepth)
	p.limits.MaxResources = iconLimit(p.limits.MaxResources, l.MaxResources)
	p.remaining.MaxSourceBytes = iconLimit(p.remaining.MaxSourceBytes, b.MaxSourceBytes)
	p.remaining.MaxElements = iconLimit(p.remaining.MaxElements, b.MaxElements)
	p.remaining.MaxAttributes = iconLimit(p.remaining.MaxAttributes, b.MaxAttributes)
	p.remaining.MaxAttributeBytes = iconLimit(p.remaining.MaxAttributeBytes, b.MaxAttributeBytes)
	p.remaining.MaxPathCommands = iconLimit(p.remaining.MaxPathCommands, b.MaxPathCommands)
	p.remaining.MaxTransformFunctions = iconLimit(p.remaining.MaxTransformFunctions, b.MaxTransformFunctions)
	p.remaining.MaxDeclaredResources = iconLimit(p.remaining.MaxDeclaredResources, b.MaxDeclaredResources)
	p.remaining.MaxExpandedUseInstances = iconLimit(p.remaining.MaxExpandedUseInstances, b.MaxExpandedUseInstances)
}

// texture returns top-left-origin, premultiplied RGBA suitable for the native
// triangle rasterizer. Aspect fitting and transparent letterboxing happen in
// d2raster, including EXIF orientation and deterministic first animated frames.
func (p *surfaceIconPainter) texture(source *url.URL, width, depth float64, borderRadiusWorld ...float64) (*image.RGBA, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if source == nil || !captionFinite(width, depth) || width <= 0 || depth <= 0 {
		return nil, fmt.Errorf("isometric surface icon requires a source and positive finite dimensions")
	}
	raw := source.String()
	if len(raw) > 12<<20 {
		return nil, fmt.Errorf("isometric surface icon source exceeds byte limit")
	}
	if p.dataOnly && !strings.EqualFold(source.Scheme, "data") {
		return nil, fmt.Errorf("isometric surface icon requires an explicit asset resolver for external sources")
	}
	sourceID := sha256.Sum256([]byte(raw))
	w, h := surfaceIconDimensions(width, depth, p.dimension)
	if p.outputDensity > 0 && captionFinite(p.outputDensity) {
		w, h = surfaceTextureDimensionsAtDensity(width, depth, 4096, maxSurfaceIconPixels/p.count, p.outputDensity)
	}
	radius := 0.
	if len(borderRadiusWorld) > 0 {
		radius = borderRadiusWorld[0]
	}
	if !captionFinite(radius) || radius < 0 {
		return nil, fmt.Errorf("isometric surface icon has invalid border radius")
	}
	radius = min(radius, min(width, depth)/2) * min(float64(w)/width, float64(h)/depth)
	key := surfaceIconKey{source: sourceID, width: w, height: h, radius: math.Float64bits(radius)}
	if tex := p.textures[key]; tex != nil {
		return tex, nil
	}
	pixels := int64(w) * int64(h)
	if len(p.textures) >= p.count || p.pixels+pixels > maxSurfaceIconPixels {
		return nil, fmt.Errorf("isometric surface icon textures exceed the render budget")
	}
	asset, err := p.resolve(sourceID, raw)
	if err != nil {
		return nil, err
	}
	node := d2scene.NewNode(d2scene.Image{Asset: "surface-icon", Box: d2scene.Box{Width: float64(w), Height: float64(h)}, Aspect: d2scene.AspectRatio{Align: d2scene.AlignXMidYMid, Fit: d2scene.AspectMeet}})
	if radius > 0 {
		node.Clip = roundedSurfaceIconClip(float64(w), float64(h), radius)
	}
	doc := d2scene.NewDocument(d2scene.Box{Width: float64(w), Height: float64(h)}, node)
	for id, resource := range asset.assets {
		doc.Assets[id] = resource
	}
	doc.Assets["surface-icon"] = asset.asset
	frame, err := d2raster.Render(p.ctx, doc, d2raster.FrameOptions{
		Scale: 1, MaxWidth: w, MaxHeight: h, MaxPixels: pixels,
		MaxNodes: 100000, MaxDepth: 128, MaxPathCommands: 500000,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1,
		MaxAssets: 4097, MaxAssetBytes: maxSurfaceIconBytes, MaxDecodedAssetBytes: maxSurfaceIconDecoded, MaxImportDepth: 4,
		MaxOffscreenBytes: 32 << 20, MaxEvenOddClipWork: 32 << 20, MaxScanlineWork: 100_000_000,
	})
	if err != nil {
		return nil, fmt.Errorf("isometric surface icon rasterization: %w", err)
	}
	tex := image.NewRGBA(frame.Bounds())
	// Copy by rows so cancellation also bounds the alpha conversion pass.
	for y := 0; y < h; y++ {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		draw.Draw(tex, image.Rect(0, y, w, y+1), frame, image.Pt(0, y), draw.Src)
	}
	p.pixels += pixels
	p.textures[key] = tex
	if err := retainNativeVectorSurface(p.ctx, tex, doc); err != nil {
		return nil, err
	}
	return tex, nil
}

func surfaceIconDimensions(width, depth float64, maximum int) (int, int) {
	// Normalize before multiplication, keeping even extreme accepted scene
	// aspect ratios finite. At least one sample remains on a very narrow face.
	ratio := width / max(width, depth)
	zratio := depth / max(width, depth)
	long := min(float64(maximum), max(1, max(width, depth)*160))
	return max(1, int(math.Round(long*ratio))), max(1, int(math.Round(long*zratio)))
}

func (p *surfaceIconPainter) resolve(id [32]byte, source string) (*surfaceIconAsset, error) {
	if asset := p.sources[id]; asset != nil {
		return asset, nil
	}
	if len(p.sources) >= p.count {
		return nil, fmt.Errorf("isometric surface icon sources exceed the render budget")
	}
	resource, err := p.resolver.Resolve(p.ctx, source)
	if err != nil {
		return nil, fmt.Errorf("isometric surface icon: %w", err)
	}
	if resource == nil {
		return nil, fmt.Errorf("isometric surface icon resolver returned no resource")
	}
	if resource.EncodedBytes() > 8<<20 || resource.DecodedBytes() > 32<<20 || p.bytes+resource.EncodedBytes() > maxSurfaceIconBytes || p.decode+resource.DecodedBytes() > maxSurfaceIconDecoded {
		return nil, fmt.Errorf("isometric surface icon assets exceed byte budget")
	}
	data, err := resource.BytesContext(p.ctx)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	if asset := p.content[digest]; asset != nil {
		p.sources[id] = asset
		return asset, nil
	}
	asset := &surfaceIconAsset{}
	extraBytes, extraDecoded := int64(0), int64(0)
	switch resource.Kind() {
	case imageasset.KindRaster:
		asset.asset = d2scene.RasterAsset{MIMEType: resource.MIMEType(), Data: data, PixelWidth: resource.PixelWidth(), PixelHeight: resource.PixelHeight(), DecodedBytes: resource.DecodedBytes()}
	case imageasset.KindSVG:
		limits, err := p.importLimits()
		if err != nil {
			return nil, err
		}
		// The importer never resolves files or URLs referenced within an SVG.
		result, err := d2svgimport.ImportNode(p.ctx, "surface icon", data, limits)
		if err != nil {
			return nil, fmt.Errorf("isometric surface icon SVG: %w", err)
		}
		extraBytes, extraDecoded = int64(result.Metrics.EmbeddedRasterBytes), result.Metrics.DecodedRasterBytes
		if p.bytes+resource.EncodedBytes()+extraBytes > maxSurfaceIconBytes || p.decode+resource.DecodedBytes()+extraDecoded > maxSurfaceIconDecoded {
			return nil, fmt.Errorf("isometric surface icon embedded assets exceed byte budget")
		}
		p.reserveImport(result.Metrics)
		content := d2scene.NewNode(nil)
		content.Transform = result.ViewportTransform
		content.Children = []*d2scene.Node{result.Root}
		root := d2scene.NewNode(nil)
		root.Clip = &d2scene.Clip{Transform: d2scene.Identity(), Path: d2scene.Path{Commands: []d2scene.PathCommand{d2scene.MoveTo(0, 0), d2scene.LineTo(result.Width, 0), d2scene.LineTo(result.Width, result.Height), d2scene.LineTo(0, result.Height), d2scene.ClosePath()}}}
		root.Children = []*d2scene.Node{content}
		asset.asset = d2scene.VectorAsset{ViewBox: d2scene.Box{Width: result.Width, Height: result.Height}, Root: root}
		asset.assets = result.Assets
	default:
		return nil, fmt.Errorf("isometric surface icon has unsupported resource kind")
	}
	p.bytes += resource.EncodedBytes() + extraBytes
	p.decode += resource.DecodedBytes() + extraDecoded
	p.sources[id], p.content[digest] = asset, asset
	return asset, nil
}

func (p *surfaceIconPainter) importLimits() (d2svgimport.Limits, error) {
	l, b := p.limits, p.remaining
	l.MaxBytes = min(l.MaxBytes, b.MaxSourceBytes)
	l.MaxElements = min(l.MaxElements, b.MaxElements)
	l.MaxAttributes = min(l.MaxAttributes, b.MaxAttributes)
	l.MaxAttributeBytes = min(l.MaxAttributeBytes, b.MaxAttributeBytes)
	l.MaxPathCommands = min(l.MaxPathCommands, b.MaxPathCommands)
	l.MaxTransformFunctions = min(l.MaxTransformFunctions, b.MaxTransformFunctions)
	l.MaxResources = min(l.MaxResources, min(b.MaxDeclaredResources, b.MaxExpandedUseInstances))
	if l.MaxBytes <= 0 || l.MaxElements <= 0 || l.MaxAttributes <= 0 || l.MaxAttributeBytes <= 0 || l.MaxPathCommands <= 0 || l.MaxTransformFunctions <= 0 || l.MaxResources <= 0 {
		return l, fmt.Errorf("isometric surface icons exceed the SVG import budget")
	}
	return l, nil
}

func (p *surfaceIconPainter) reserveImport(m d2svgimport.Metrics) {
	p.remaining.MaxSourceBytes -= m.SourceBytes
	p.remaining.MaxElements -= max(m.ParsedElements, m.EmittedElements)
	p.remaining.MaxAttributes -= m.ParsedAttributes
	p.remaining.MaxAttributeBytes -= m.ParsedAttributeBytes
	p.remaining.MaxPathCommands -= max(m.ParsedPathCommands, m.EmittedPathCommands)
	p.remaining.MaxTransformFunctions -= m.ParsedTransformFuncs
	p.remaining.MaxDeclaredResources -= m.DeclaredResources
	p.remaining.MaxExpandedUseInstances -= m.ExpandedUseInstances
}

// surfaceIconLayout partitions an existing printable face without changing
// the physical shape footprint. Outside icon positions are projected onto the
// corresponding face edge: icon artwork never floats beyond its parent mesh.
// The returned rectangles are disjoint and inherit the real surface angle.
func surfaceIconLayout(s labelSurface, original d2target.Shape, scale float64, kind string) (icon, text labelSurface) {
	text = s
	if original.Icon == nil || !captionFinite(s.width, s.depth, s.angle, s.center.X, s.center.Y, s.center.Z) || s.width <= 0 || s.depth <= 0 {
		return labelSurface{}, text
	}
	if scale <= 0 || !captionFinite(scale) {
		scale = .01
	}
	structured := original.Type == d2target.ShapeSQLTable || original.Type == d2target.ShapeClass || len(original.Columns)+len(original.Fields)+len(original.Methods) > 0
	if original.Label == "" && !structured {
		return s, labelSurface{}
	}
	if structured {
		// A table's row coordinates are the attachment contract. Its icon uses
		// the existing header; the row texture keeps the complete source face.
		header := s.depth / float64(1+len(original.Columns))
		if original.Type == d2target.ShapeClass {
			header = max(2*s.depth/float64(2+len(original.Fields)+len(original.Methods)), float64(original.LabelHeight)*scale+2*label.PADDING*scale)
		}
		pad := min(label.PADDING*scale, min(s.width, header)*.1)
		size := min(float64(d2target.DEFAULT_ICON_SIZE)*scale, min(s.width-2*pad, header-2*pad))
		size = max(0, size)
		icon = s
		icon.width, icon.depth = size, size
		x := (s.width-size)/2 - pad
		if strings.Contains(original.IconPosition, "LEFT") {
			x = -x
		}
		z := -s.depth/2 + header/2
		icon.center = nadd(s.center, nv(math.Cos(s.angle)*x-math.Sin(s.angle)*z, 0, math.Sin(s.angle)*x+math.Cos(s.angle)*z))
		return icon, s
	}
	position := original.IconPosition
	horizontal := strings.Contains(position, "LEFT") || strings.Contains(position, "RIGHT") || kind == "board" || kind == "edge"
	if position == "" || strings.Contains(position, "MIDDLE_CENTER") {
		// Prefer the layout's already allocated whitespace over a fixed chip
		// subdivision; short, wide modules usually reserve a left icon lane.
		freeX := s.width - float64(original.LabelWidth)*scale
		freeZ := s.depth - float64(original.LabelHeight)*scale
		horizontal = freeX >= freeZ || kind == "board" || kind == "edge"
	}
	if original.Type == d2target.ShapeImage {
		horizontal = false
		position = "INSIDE_TOP_CENTER"
	}
	if structured {
		// Structured rows need the full face width, including when their
		// authored header is empty. Reserve only a compact top icon strip.
		horizontal = false
		position = "INSIDE_TOP_CENTER"
	}
	icon, text = s, s
	gap := min(.08, min(s.width, s.depth)*.08)
	// The compiled label metrics take priority over a decorative icon minimum.
	// A little print padding covers native glyph overhang and texture insets.
	requiredWidth := float64(original.LabelWidth)*scale*1.06 + min(.04, s.width*.04)
	requiredDepth := float64(original.LabelHeight)*scale*1.06 + min(.04, s.depth*.04)
	availableWidth := s.width - requiredWidth - gap
	if horizontal && availableWidth < min(s.depth*.22, s.width*.12) {
		horizontal = false
	}
	offset := func(x, z float64) Vec {
		return nadd(s.center, nv(math.Cos(s.angle)*x-math.Sin(s.angle)*z, 0, math.Sin(s.angle)*x+math.Cos(s.angle)*z))
	}
	if horizontal {
		size := min(s.depth, min(s.width*.48, availableWidth))
		if kind == "board" || kind == "edge" {
			size = min(size, s.width*.28)
		}
		icon.width, icon.depth = size, min(size, s.depth)
		text.width = s.width - size - gap
		direction := 1.
		if strings.Contains(position, "RIGHT") {
			direction = -1
		}
		icon.center = offset(-direction*(s.width-size)/2, 0)
		if strings.Contains(position, "TOP") {
			icon.center = offset(-direction*(s.width-size)/2, -(s.depth-icon.depth)/2)
		} else if strings.Contains(position, "BOTTOM") {
			icon.center = offset(-direction*(s.width-size)/2, (s.depth-icon.depth)/2)
		}
		text.center = offset(direction*(size+gap)/2, 0)
	} else {
		size := min(s.width, max(s.depth*.14, s.depth-requiredDepth-gap))
		size = min(size, s.depth*.72)
		if structured {
			size = min(size, min(s.depth*.2, float64(d2target.DEFAULT_ICON_SIZE)*scale))
		}
		icon.width, icon.depth = min(size, s.width), size
		if original.Type == d2target.ShapeImage {
			// A wide image gets the whole image panel, not a square badge.
			icon.width = s.width
		}
		text.depth = s.depth - size - gap
		direction := 1.
		if strings.Contains(position, "BOTTOM") {
			direction = -1
		}
		icon.center = offset(0, -direction*(s.depth-size)/2)
		text.center = offset(0, direction*(size+gap)/2)
	}
	return icon, text
}

// Use vector clipping before rasterization so transparent and antialiased
// image corners retain correct premultiplied alpha on the physical surface.
func roundedSurfaceIconClip(w, h, r float64) *d2scene.Clip {
	r = min(r, min(w, h)/2)
	k := r * .5522847498307936
	return &d2scene.Clip{Transform: d2scene.Identity(), Path: d2scene.Path{Commands: []d2scene.PathCommand{
		d2scene.MoveTo(r, 0), d2scene.LineTo(w-r, 0), d2scene.CubicTo(w-r+k, 0, w, r-k, w, r),
		d2scene.LineTo(w, h-r), d2scene.CubicTo(w, h-r+k, w-r+k, h, w-r, h),
		d2scene.LineTo(r, h), d2scene.CubicTo(r-k, h, 0, h-r+k, 0, h-r),
		d2scene.LineTo(0, r), d2scene.CubicTo(0, r-k, r-k, 0, r, 0), d2scene.ClosePath(),
	}}}
}
