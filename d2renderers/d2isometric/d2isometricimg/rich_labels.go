package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
	"github.com/d2lang/d2/d2themes/d2themescatalog"
)

const (
	maxRichLabelBytes  = 64 * 1024
	maxRichSourceBytes = 1024 * 1024
	maxRichRows        = 512
	maxRichTotalRows   = 4096
	maxRichPixels      = 8 * 1024 * 1024
)

// Rich labels reuse D2's typed native Markdown, Chroma and structured-table
// layout. Only their paint is rasterized; no compiler layout or source shape
// dimensions are changed and no browser or external asset resolver is used.
type richLabelPainter struct {
	themeID                                *int64
	themeOverrides                         *d2target.ThemeOverrides
	fallbackFonts                          *d2scenebuild.FontFallbackOptions
	ctx                                    context.Context
	count, used, pixels, sourceBytes, rows int
	primary, mono                          d2fonts.FontFamily
	outputDensity                          float64
	lastDocument                           *d2scene.Document
}

func newRichLabelPainter(ctx context.Context, count int) (*richLabelPainter, error) {
	if ctx == nil {
		return nil, fmt.Errorf("isometric rich labels require a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if count < 0 || count > maxTextLabels {
		return nil, fmt.Errorf("isometric rich labels exceed %d labels", maxTextLabels)
	}
	return &richLabelPainter{ctx: ctx, count: count}, nil
}

func (p *richLabelPainter) configureFontFamilies(primary, mono *d2fonts.FontFamily) {
	p.primary, p.mono = nativeFontFamilies(primary, mono)
}

func (p *richLabelPainter) configureFallbackFonts(fonts *d2scenebuild.FontFallbackOptions) {
	p.fallbackFonts = fonts
}

func (p *richLabelPainter) configureOutputDensity(pixelsPerWorld float64) {
	p.outputDensity = pixelsPerWorld
}

func (p *richLabelPainter) configureTheme(id int64, overrides *d2target.ThemeOverrides) {
	p.themeID, p.themeOverrides = &id, overrides
}

func isRichLabel(s d2target.Shape) bool {
	language := strings.ToLower(strings.TrimSpace(s.Language))
	return s.Type == d2target.ShapeSQLTable || s.Type == d2target.ShapeClass ||
		s.Type == d2target.ShapeCode || language != ""
}

func (p *richLabelPainter) texture(original d2target.Shape, style labelTextStyle) (*image.RGBA, error) {
	p.lastDocument = nil
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	if p.used >= p.count {
		return nil, fmt.Errorf("isometric rich label allocation exceeds its declared count")
	}
	if !isRichLabel(original) {
		return nil, fmt.Errorf("isometric rich label has no supported rich content")
	}
	if !captionFinite(style.Width, style.Depth, style.PixelScale, style.FontSize, style.Opacity) ||
		style.Width <= 0 || style.Depth <= 0 || style.PixelScale <= 0 || style.FontSize < 0 || style.Opacity < 0 || style.Opacity > 1 {
		return nil, fmt.Errorf("isometric rich label has invalid surface dimensions or style")
	}
	bytes, rows, err := richSourceSize(original)
	if err != nil {
		return nil, err
	}
	if bytes > maxRichSourceBytes-p.sourceBytes || rows > maxRichTotalRows-p.rows {
		return nil, fmt.Errorf("isometric rich labels exceed aggregate source or row budget")
	}
	p.sourceBytes += bytes
	p.rows += rows
	p.used++
	document, err := richLabelDocumentWithResources(p.ctx, original, style, p.fallbackFonts, p.themeID, p.themeOverrides, p.primary, p.mono)
	if err != nil {
		return nil, fmt.Errorf("isometric rich label %q: %w", original.ID, err)
	}
	structured := original.Type == d2target.ShapeSQLTable || original.Type == d2target.ShapeClass
	if !structured {
		if err := fitRichViewport(document, style); err != nil {
			return nil, err
		}
	}
	// Budget each tile by its actual physical aspect ratio, not a fixed wide
	// text atlas patch. Tall Markdown and SQL rows therefore retain detail.
	budget := min(4*1024*1024, maxRichPixels/max(1, p.count))
	w, h := surfaceTextureDimensionsAtDensity(style.Width, style.Depth, 4096, budget, p.outputDensity)
	if w*h > maxRichPixels-p.pixels {
		return nil, fmt.Errorf("isometric rich labels exceed texture pixel budget")
	}
	p.pixels += w * h
	document.LogicalWidth, document.LogicalHeight = float64(w), float64(h)
	document.ViewportFit = d2scene.ViewportMeet
	if structured {
		document.ViewportFit = d2scene.ViewportStretch
	}
	document.ViewportAlign = d2scene.ViewportAlignXMidYMid
	frame, err := d2raster.Render(p.ctx, document, richRasterOptions())
	if err != nil {
		return nil, err
	}
	texture := image.NewRGBA(frame.Bounds())
	if style.Background != nil {
		draw.Draw(texture, texture.Bounds(), image.NewUniform(*style.Background), image.Point{}, draw.Src)
	}
	draw.Draw(texture, texture.Bounds(), frame, image.Point{}, draw.Over)
	// Text and any inline backgrounds receive object opacity once. Conversion
	// through draw.Draw keeps bilinear sampling in premultiplied RGBA space.
	if style.Opacity < 1 {
		for y := 0; y < texture.Rect.Dy(); y++ {
			if y&31 == 0 {
				if err := p.ctx.Err(); err != nil {
					return nil, err
				}
			}
			for x := 0; x < texture.Rect.Dx()*4; x++ {
				i := y*texture.Stride + x
				texture.Pix[i] = uint8(math.Round(float64(texture.Pix[i]) * style.Opacity))
			}
		}
	}
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	p.lastDocument = document
	if err := retainStyledNativeVectorSurface(p.ctx, texture, document, style.Background, style.Opacity); err != nil {
		return nil, err
	}
	return texture, nil
}

func richSourceSize(s d2target.Shape) (int, int, error) {
	rows := len(s.Fields) + len(s.Methods) + len(s.Columns)
	if rows > maxRichRows {
		return 0, 0, fmt.Errorf("isometric rich label exceeds %d structured rows", maxRichRows)
	}
	size := 0
	add := func(value string) error {
		if len(value) > maxRichLabelBytes-size || !utf8.ValidString(value) {
			return fmt.Errorf("isometric rich label exceeds source byte budget or contains invalid UTF-8")
		}
		size += len(value)
		return nil
	}
	if err := add(s.Label); err != nil {
		return 0, 0, err
	}
	for _, f := range s.Fields {
		for _, value := range []string{f.Name, f.Type, f.Visibility} {
			if err := add(value); err != nil {
				return 0, 0, err
			}
		}
	}
	for _, f := range s.Methods {
		for _, value := range []string{f.Name, f.Return, f.Visibility} {
			if err := add(value); err != nil {
				return 0, 0, err
			}
		}
	}
	for _, c := range s.Columns {
		if len(c.Constraint) > maxRichRows {
			return 0, 0, fmt.Errorf("isometric rich label exceeds constraint limit")
		}
		if err := add(c.Name.Label); err != nil {
			return 0, 0, err
		}
		if err := add(c.Type.Label); err != nil {
			return 0, 0, err
		}
		for _, v := range c.Constraint {
			if err := add(v); err != nil {
				return 0, 0, err
			}
		}
	}
	return size, rows, nil
}

func richLabelDocument(ctx context.Context, original d2target.Shape, style labelTextStyle, families ...d2fonts.FontFamily) (*d2scene.Document, error) {
	return richLabelDocumentWithFonts(ctx, original, style, nil, families...)
}

func richLabelDocumentWithFonts(ctx context.Context, original d2target.Shape, style labelTextStyle, fonts *d2scenebuild.FontFallbackOptions, families ...d2fonts.FontFamily) (*d2scene.Document, error) {
	return richLabelDocumentWithResources(ctx, original, style, fonts, nil, nil, families...)
}

func richLabelDocumentWithResources(ctx context.Context, original d2target.Shape, style labelTextStyle, fonts *d2scenebuild.FontFallbackOptions, themeID *int64, overrides *d2target.ThemeOverrides, families ...d2fonts.FontFamily) (*d2scene.Document, error) {
	// Construct a minimal owned target. Icons, links, shadows and other object
	// effects belong to the 3D object or its own asset phase, never this texture.
	s := d2target.Shape{ID: "surface", Type: original.Type, Opacity: 1, Stroke: "none", Fill: "transparent", Text: original.Text,
		Class: original.Class, SQLTable: original.SQLTable, LabelPosition: "INSIDE_MIDDLE_CENTER"}
	s.Language = strings.ToLower(strings.TrimSpace(s.Language))
	if s.Language == "md" {
		s.Language = "markdown"
	}
	if s.FontSize <= 0 {
		s.FontSize = max(1, int(math.Round(style.FontSize)))
	}
	if s.FontSize <= 0 {
		s.FontSize = 16
	}
	if s.FontSize > 10000 {
		return nil, fmt.Errorf("rich font size exceeds limit")
	}
	primary, mono := nativeFontFamilies(nil, nil)
	if len(families) > 0 && families[0] != "" {
		primary = families[0]
	}
	if len(families) > 1 && families[1] != "" {
		mono = families[1]
	}
	family, err := nativeFontFamily(original.FontFamily, primary, mono)
	if err != nil {
		return nil, err
	}
	primary = family
	s.FontFamily = "DEFAULT"
	ink := richColor(style.Color)
	s.Color = ink
	if s.Language == "latex" {
		s.Stroke = ink
	}
	structured := s.Type == d2target.ShapeSQLTable || s.Type == d2target.ShapeClass
	if structured {
		s.Width, s.Height = original.Width, original.Height
		// Structured D2 shapes use Fill for the header and row ink, and
		// Stroke for their body background. Keep this native contract and
		// theme tokens; never recolor every row with a generic caption ink.
		s.Fill, s.Stroke, s.Color = original.Fill, original.Stroke, original.Color
		if s.Fill == "" {
			s.Fill = ink
		}
		if s.Stroke == "" {
			s.Stroke = "transparent"
		}
		if s.Color == "" {
			s.Color = readableSurfaceInk(s.Fill, 1)
		}
		s.BorderRadius = original.BorderRadius
		s.PrimaryAccentColor = original.PrimaryAccentColor
		s.SecondaryAccentColor = original.SecondaryAccentColor
		s.NeutralAccentColor = original.NeutralAccentColor
		if s.PrimaryAccentColor == "" {
			s.PrimaryAccentColor = ink
		}
		if s.SecondaryAccentColor == "" {
			s.SecondaryAccentColor = ink
		}
		if s.NeutralAccentColor == "" {
			s.NeutralAccentColor = ink
		}
	} else {
		s.LabelWidth = original.LabelWidth
		s.LabelHeight = original.LabelHeight
		if s.LabelWidth <= 0 {
			s.LabelWidth = max(1, int(math.Min(100000, style.Width/style.PixelScale)))
		}
		if s.LabelHeight <= 0 {
			s.LabelHeight = max(1, int(math.Min(100000, style.Depth/style.PixelScale)))
		}
		s.Width, s.Height = s.LabelWidth, s.LabelHeight
		if s.Language == "markdown" || s.Language == "latex" {
			s.Type = d2target.ShapeText
		} else {
			s.Type = d2target.ShapeCode
			if s.Language == "" {
				s.Language = "text"
			}
			s.Width += s.FontSize
			s.Height += s.FontSize
		}
	}
	if s.Width <= 0 || s.Height <= 0 || s.Width > 100000 || s.Height > 100000 || s.LabelWidth < 0 || s.LabelHeight < 0 || s.LabelWidth > 100000 || s.LabelHeight > 100000 {
		return nil, fmt.Errorf("rich label dimensions exceed limits")
	}
	diagram := d2target.NewDiagram()
	diagram.Root.Fill, diagram.Root.Stroke = "transparent", "none"
	diagram.FontFamily, diagram.MonoFontFamily = &primary, &mono
	diagram.Shapes = []d2target.Shape{s}
	pad := int64(0)
	theme := d2themescatalog.NeutralDefault.ID
	// Root chooses light default ink for a dark physical face; use matching
	// Markdown inline palettes and code syntax colors on that face.
	if int(style.Color.R)*299+int(style.Color.G)*587+int(style.Color.B)*114 > 180000 {
		theme = d2themescatalog.DarkMauve.ID
	}
	assets, err := nativeSurfaceAssets(ctx, nil)
	if err != nil {
		return nil, err
	}
	if themeID != nil {
		theme = *themeID
	}
	fontOptions, err := nativeSurfaceFonts(fonts)
	if err != nil {
		return nil, err
	}
	doc, err := d2scenebuild.Build(ctx, diagram, d2scenebuild.Options{Assets: assets, Pad: &pad, ThemeID: &theme, ThemeOverrides: overrides, MaxNodes: 20000, MaxPathCommands: 250000,
		LinkBudget: d2scenebuild.LinkBudget{MaxRegions: 2048, MaxStringBytes: maxRichLabelBytes}, Fonts: fontOptions})
	if err != nil {
		return nil, err
	}
	if err := nativeDocumentGlyphCoverage(ctx, doc); err != nil {
		return nil, err
	}
	doc.ViewBox = d2scene.Box{Width: float64(s.Width), Height: float64(s.Height)}
	if structured {
		stripRichBodies(doc.Root)
	}
	// Retain inline link geometry for exports that support annotations. Links
	// remain metadata: generating the surface never resolves or fetches them.
	return doc, nil
}

// Preserve the original D2 font size when a component has spare physical
// space. Smaller print surfaces uniformly shrink the rich area to fit.
func fitRichViewport(document *d2scene.Document, style labelTextStyle) error {
	w, h := style.Width/style.PixelScale, style.Depth/style.PixelScale
	if !captionFinite(w, h) || w <= 0 || h <= 0 {
		return fmt.Errorf("isometric rich label has invalid physical pixel dimensions")
	}
	source := document.ViewBox
	fit := math.Min(1, math.Min(w/source.Width, h/source.Height))
	w, h = w/fit, h/fit
	document.ViewBox = d2scene.Box{X: source.X + (source.Width-w)/2, Y: source.Y + (source.Height-h)/2, Width: w, Height: h}
	return nil
}

func richColor(c color.NRGBA) string { return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A) }
func richAccent(value, fallback string) string {
	if value == "" || nativeToken(value) {
		return fallback
	}
	return value
}
func stripRichBodies(n *d2scene.Node) {
	if n == nil {
		return
	}
	if n.ID == "surface:outline" {
		n.Primitive = nil
	}
	for _, child := range n.Children {
		stripRichBodies(child)
	}
}

func richRasterOptions() d2raster.FrameOptions {
	return d2raster.FrameOptions{Scale: 1, MaxWidth: 4096, MaxHeight: 4096, MaxPixels: 8 * 1024 * 1024,
		MaxNodes: 100000, MaxDepth: 128, MaxPathCommands: 500000, MaxTextRunesPerRun: maxRichLabelBytes,
		MaxTextCoverageChecks: 2_000_000, MaxTextShapingRuns: 20000, MaxFontFacesPerText: 4,
		MaxAnimationTracks: 1, MaxAnimationKeyframes: 1, MaxAssets: 4096, MaxAssetBytes: 32 * 1024 * 1024,
		MaxDecodedAssetBytes: 1, MaxImportDepth: 8, MaxOffscreenBytes: 32 * 1024 * 1024,
		MaxEvenOddClipWork: 20_000_000, MaxScanlineWork: 100_000_000}
}
