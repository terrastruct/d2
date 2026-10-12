package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"math"
	"strings"
	"time"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2raster"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

func nativeFontFamilies(primary, mono *d2fonts.FontFamily) (d2fonts.FontFamily, d2fonts.FontFamily) {
	p, m := d2fonts.SourceSansPro, d2fonts.SourceCodePro
	if primary != nil && *primary != "" {
		p = *primary
	}
	if mono != nil && *mono != "" {
		m = *mono
	}
	return p, m
}

func nativeFontFamily(name string, primary, mono d2fonts.FontFamily) (d2fonts.FontFamily, error) {
	primary, mono = nativeFontFamilies(&primary, &mono)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "default":
		return primary, nil
	case "mono":
		return mono, nil
	case "sourcesanspro":
		return d2fonts.SourceSansPro, nil
	case "sourcecodepro":
		return d2fonts.SourceCodePro, nil
	case "handdrawn", "fuzzybubbles":
		return d2fonts.HandDrawn, nil
	}
	family := d2fonts.FontFamily(name)
	if _, ok := d2fonts.FontFaces.Lookup(d2fonts.Font{Family: family, Style: d2fonts.FONT_STYLE_REGULAR}); ok {
		return family, nil
	}
	return "", fmt.Errorf("isometric text font family %q is not registered", name)
}

func nativeSurfaceAssets(ctx context.Context, supplied *d2scenebuild.AssetOptions) (*d2scenebuild.AssetOptions, error) {
	p, err := newSurfaceIconPainter(ctx, 1, supplied)
	if err != nil {
		return nil, err
	}
	assets := &d2scenebuild.AssetOptions{SVGImportLimits: p.limits, SVGImportBudget: p.remaining}
	// Only explicitly supplied resolvers may perform asset I/O. LaTeX itself
	// needs just the import limits and can render without any resolver.
	assets.Resolver = p.resolver
	return assets, nil
}

// nativeSurfaceDocument retains the standard D2 scene for content that must
// preserve its complete 2D grammar before being projected onto a 3D surface.
func nativeSurfaceDocument(ctx context.Context, diagram *d2target.Diagram, assets *d2scenebuild.AssetOptions, fonts ...*d2scenebuild.FontFallbackOptions) (*d2scene.Document, error) {
	if ctx == nil || diagram == nil {
		return nil, fmt.Errorf("native surface requires context and diagram")
	}
	if assets == nil || assets.Resolver == nil {
		if err := nativeEmbeddedAssetsOnly(diagram); err != nil {
			return nil, err
		}
	}
	boundedAssets, err := nativeSurfaceAssets(ctx, assets)
	if err != nil {
		return nil, err
	}
	pad := int64(0)
	options := d2scenebuild.Options{
		Pad: &pad, MaxNodes: 200000, MaxPathCommands: 2000000,
		Assets:     boundedAssets,
		LinkBudget: d2scenebuild.LinkBudget{MaxRegions: 20000, MaxStringBytes: maxTextSourceBytes},
		Fonts:      &d2scenebuild.FontFallbackOptions{MaxAssets: 96, MaxBytes: 64 << 20, MaxRunesPerText: maxLabelRunes, MaxTotalRunes: maxTextSourceBytes, MaxCoverageChecks: 8000000, MaxShapedGlyphs: 1000000, MaxShapingRuns: 200000, MaxFontFacesPerText: 8},
	}
	if diagram.Config != nil {
		options.ThemeID, options.ThemeOverrides = diagram.Config.ThemeID, diagram.Config.ThemeOverrides
	}
	var configured *d2scenebuild.FontFallbackOptions
	if len(fonts) > 0 {
		configured = fonts[0]
	}
	options.Fonts, err = nativeSurfaceFonts(configured)
	if err != nil {
		return nil, err
	}
	doc, err := d2scenebuild.Build(ctx, diagram, options)
	if err != nil {
		return nil, err
	}
	if err = nativeDocumentGlyphCoverage(ctx, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// rasterNativeSurfaceDocument never mutates the document. seconds is optional
// so static labels and deterministic animated panels share the same pipeline.
func rasterNativeSurfaceDocument(ctx context.Context, document *d2scene.Document, width, height int, seconds ...float64) (*image.RGBA, error) {
	if ctx == nil || document == nil || width <= 0 || height <= 0 || width > 8192 || height > 8192 || int64(width)*int64(height) > 8<<20 {
		return nil, fmt.Errorf("native surface has invalid document or texture dimensions")
	}
	at := 0.
	if len(seconds) > 0 {
		at = seconds[0]
	}
	if !captionFinite(at) || at < 0 || at > float64(math.MaxInt64)/float64(time.Second) {
		return nil, fmt.Errorf("native surface has invalid frame time")
	}
	view := *document
	view.LogicalWidth, view.LogicalHeight = float64(width), float64(height)
	view.ViewportFit, view.ViewportAlign = d2scene.ViewportMeet, d2scene.ViewportAlignXMidYMid
	frame, err := d2raster.Render(ctx, &view, d2raster.FrameOptions{
		Scale: 1, Time: time.Duration(at * float64(time.Second)), MaxWidth: width, MaxHeight: height, MaxPixels: int64(width) * int64(height),
		MaxNodes: 200000, MaxDepth: 256, MaxPathCommands: 2000000,
		MaxTextRunesPerRun: maxLabelRunes, MaxTextCoverageChecks: 8000000, MaxTextShapingRuns: 200000, MaxFontFacesPerText: 8,
		MaxAnimationTracks: 20000, MaxAnimationKeyframes: 200000,
		MaxAssets: 20000, MaxAssetBytes: 128 << 20, MaxDecodedAssetBytes: 128 << 20, MaxImportDepth: 32,
		MaxOffscreenBytes: 256 << 20, MaxEvenOddClipWork: 64000000, MaxScanlineWork: 500000000,
	})
	if err != nil {
		return nil, err
	}
	texture := image.NewRGBA(frame.Bounds())
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		draw.Draw(texture, image.Rect(0, y, width, y+1), frame, image.Pt(0, y), draw.Src)
	}
	if err := retainNativeVectorSurface(ctx, texture, &view); err != nil {
		return nil, err
	}
	return texture, nil
}

// Allocate by the actual surface aspect ratio. A long label can use a wide
// texture without reserving a mostly empty square or exceeding its share of
// the render's aggregate pixel budget.
func surfaceTextureDimensions(width, depth float64, maximum, budget int) (int, int) {
	return surfaceTextureDimensionsAtDensity(width, depth, maximum, budget, 0)
}

// surfaceTextureDimensionsAtDensity matches a surface's projected output size.
// A small sampling margin avoids magnifying text at oblique angles. The same
// per-surface share and aggregate ceilings still apply at every output scale.
// Zero density preserves the original standalone painter allocation policy.
func surfaceTextureDimensionsAtDensity(width, depth float64, maximum, budget int, pixelsPerWorld float64) (int, int) {
	x, z := width/max(width, depth), depth/max(width, depth)
	long := min(float64(maximum), math.Sqrt(float64(budget)/(x*z)))
	if pixelsPerWorld > 0 && captionFinite(pixelsPerWorld) {
		long = min(long, max(width, depth)*pixelsPerWorld*1.5)
	}
	w, h := max(1, int(math.Floor(long*x))), max(1, int(math.Floor(long*z)))
	if w*h > budget {
		if w >= h {
			w = max(1, budget/h)
		} else {
			h = max(1, budget/w)
		}
	}
	return w, h
}
