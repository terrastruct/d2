package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"math"
	"net/url"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
	"github.com/d2lang/d2/d2target"
)

func nativeEmbeddedAssetsOnly(diagram *d2target.Diagram) error {
	check := func(icon *url.URL) error {
		if icon != nil && !strings.EqualFold(icon.Scheme, "data") {
			return fmt.Errorf("native surface external icon requires an explicit asset resolver")
		}
		return nil
	}
	if err := check(diagram.Root.Icon); err != nil {
		return err
	}
	for _, shape := range diagram.Shapes {
		if err := check(shape.Icon); err != nil {
			return err
		}
	}
	for _, edge := range diagram.Connections {
		if err := check(edge.Icon); err != nil {
			return err
		}
	}
	if diagram.Legend != nil {
		for _, shape := range diagram.Legend.Shapes {
			if err := check(shape.Icon); err != nil {
				return err
			}
		}
		for _, edge := range diagram.Legend.Connections {
			if err := check(edge.Icon); err != nil {
				return err
			}
		}
	}
	return nil
}

func nativeSurfaceFonts(supplied *d2scenebuild.FontFallbackOptions) (*d2scenebuild.FontFallbackOptions, error) {
	options := d2scenebuild.FontFallbackOptions{MaxAssets: 96, MaxBytes: 64 << 20, MaxRunesPerText: maxLabelRunes, MaxTotalRunes: maxTextSourceBytes, MaxCoverageChecks: 8000000, MaxShapedGlyphs: 1000000, MaxShapingRuns: 200000, MaxFontFacesPerText: 8}
	if supplied != nil {
		options.Resolver = supplied.Resolver
		options.MaxAssets = iconLimit(options.MaxAssets, supplied.MaxAssets)
		if supplied.MaxBytes > 0 {
			options.MaxBytes = min(options.MaxBytes, supplied.MaxBytes)
		}
		options.MaxRunesPerText = iconLimit(options.MaxRunesPerText, supplied.MaxRunesPerText)
		options.MaxTotalRunes = iconLimit(options.MaxTotalRunes, supplied.MaxTotalRunes)
		if supplied.MaxCoverageChecks > 0 {
			options.MaxCoverageChecks = min(options.MaxCoverageChecks, supplied.MaxCoverageChecks)
		}
		options.MaxShapedGlyphs = iconLimit(options.MaxShapedGlyphs, supplied.MaxShapedGlyphs)
		options.MaxShapingRuns = iconLimit(options.MaxShapingRuns, supplied.MaxShapingRuns)
		options.MaxFontFacesPerText = iconLimit(options.MaxFontFacesPerText, supplied.MaxFontFacesPerText)
	}
	if options.Resolver == nil {
		resolver, err := d2fonts.NewBundledFallbackResolver(nil, d2fonts.BundledFallbackLimits{MaxRequestedRunes: options.MaxTotalRunes, MaxResolvedBytes: options.MaxBytes})
		if err != nil {
			return nil, err
		}
		options.Resolver = resolver
	}
	return &options, nil
}

// Reject a missing source glyph explicitly. The normal 2D renderer may draw
// a placeholder; a static 3D export must not silently present that as fidelity.
func nativeDocumentGlyphCoverage(ctx context.Context, doc *d2scene.Document) error {
	faces := map[d2scene.AssetID]*fontface.ParsedFace{}
	type lookup struct {
		id    d2scene.AssetID
		value rune
	}
	cache := map[lookup]bool{}
	checks := 0
	supports := func(id d2scene.AssetID, r rune) (bool, error) {
		key := lookup{id, r}
		if v, ok := cache[key]; ok {
			return v, nil
		}
		checks++
		if checks > 8000000 {
			return false, fmt.Errorf("native surface glyph coverage exceeds budget")
		}
		face := faces[id]
		if face == nil {
			asset, ok := doc.Assets[id].(d2scene.FontAsset)
			if !ok {
				return false, fmt.Errorf("native surface font asset %q is unavailable", id)
			}
			source, bundled, err := fontface.RegisteredBundledFace(asset.Data, asset.FaceIndex)
			if err != nil {
				return false, err
			}
			if !bundled {
				source, bundled, err = fontface.RegisteredBundledNotoColorEmoji(asset.Data, asset.FaceIndex)
				if err != nil {
					return false, err
				}
			}
			if bundled {
				face, err = source.CloneReadOnly()
			} else {
				face, err = fontface.ParseFace(asset.Data, asset.FaceIndex)
			}
			if err != nil {
				return false, err
			}
			faces[id] = face
		}
		v, err := face.SupportsRenderableRune(r)
		cache[key] = v
		return v, err
	}
	var walk func(*d2scene.Node) error
	walk = func(node *d2scene.Node) error {
		if node == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if run, ok := node.Primitive.(d2scene.TextRun); ok {
			ids := append([]d2scene.AssetID{run.Font.Asset}, run.Fallbacks...)
			for _, r := range run.Text {
				if r == '\n' || r == '\r' || r == '\t' || fontface.IsDefaultIgnorableRune(r) {
					continue
				}
				found := false
				for _, id := range ids {
					ok, err := supports(id, r)
					if err != nil {
						return err
					}
					if ok {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("native surface label %q has no available font for U+%04X; configure a font or fallback resolver covering this character", node.ID, r)
				}
			}
		}
		for _, child := range node.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(doc.Root); err != nil {
		return err
	}
	for _, asset := range doc.Assets {
		if vector, ok := asset.(d2scene.VectorAsset); ok {
			if err := walk(vector.Root); err != nil {
				return err
			}
		}
	}
	return nil
}

func nativeTextNeedsFallback(ctx context.Context, value string, face *fontface.ParsedFace) (bool, error) {
	seen := map[rune]bool{}
	for _, r := range value {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if r == '\n' || r == '\r' || r == '\t' || fontface.IsDefaultIgnorableRune(r) || seen[r] {
			continue
		}
		seen[r] = true
		// The complete native text pipeline also handles color fonts, marks,
		// complex scripts and grapheme-cluster fallback selection.
		if r > 255 || face.IsBundledNotoColorEmoji() {
			return true, nil
		}
		ok, err := face.SupportsRenderableRune(r)
		if err != nil {
			return false, err
		}
		if !ok {
			return true, nil
		}
	}
	return false, nil
}

func nativeFallbackTextTexture(ctx context.Context, value string, style labelTextStyle, width, height int, primary, mono d2fonts.FontFamily, fonts *d2scenebuild.FontFallbackOptions) (*image.RGBA, textLayout, error) {
	primary, mono = nativeFontFamilies(&primary, &mono)
	lines := strings.Split(normalizeLabel(value), "\n")
	fontSize := style.FontSize
	if fontSize <= 0 {
		fontSize = 23.5
	}
	if fontSize > 100000 {
		return nil, textLayout{}, fmt.Errorf("native fallback text font size exceeds limit")
	}
	w := max(1, int(math.Min(100000, math.Ceil(style.Width/style.PixelScale))))
	h := max(1, int(math.Min(100000, math.Ceil(float64(len(lines))*fontSize*1.2))))
	s := d2target.Shape{ID: "surface-text", Type: d2target.ShapeText, Width: w, Height: h, Fill: "transparent", Stroke: "none", Opacity: 1, LabelPosition: "INSIDE_MIDDLE_CENTER", Text: d2target.Text{Label: normalizeLabel(value), FontSize: max(1, int(math.Round(fontSize))), LabelWidth: w, LabelHeight: h, FontFamily: "default", Color: richColor(style.Color), Bold: style.Bold, Italic: style.Italic, Underline: style.Underline}}
	d := d2target.NewDiagram()
	d.Root.Fill, d.Root.Stroke = "transparent", "none"
	d.FontFamily, d.MonoFontFamily = &primary, &mono
	d.Shapes = []d2target.Shape{s}
	doc, err := nativeSurfaceDocument(ctx, d, nil, fonts)
	if err != nil {
		return nil, textLayout{}, err
	}
	maxW, minY, maxY := 0., math.Inf(1), math.Inf(-1)
	var walk func(*d2scene.Node)
	walk = func(n *d2scene.Node) {
		if n == nil {
			return
		}
		if run, ok := n.Primitive.(d2scene.TextRun); ok {
			lo, hi := 0., 0.
			if run.Ink.Valid {
				lo, hi = run.Ink.Min.X, run.Ink.Max.X
				minY = min(minY, run.Origin.Y+run.Ink.Min.Y)
				maxY = max(maxY, run.Origin.Y+run.Ink.Max.Y)
			}
			for _, g := range run.Glyphs {
				hi = max(hi, g.Position.X+g.Advance)
			}
			if style.Bold && style.Italic {
				n.Transform = n.Transform.Mul(d2scene.Matrix{A: 1, C: -.20, D: 1, E: .20 * run.Origin.Y})
				lo -= fontSize * .20
				hi += fontSize * .20
			}
			maxW = max(maxW, hi-lo)
			run.Origin.X = -(lo + hi) / 2
			run.Anchor = d2scene.AnchorStart
			if style.Align == "left" {
				run.Origin.X = 1 - lo
			}
			n.Primitive = run
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(doc.Root)
	if !captionFinite(minY, maxY) {
		minY, maxY = 0, float64(h)
	}
	box := d2scene.Box{X: -(maxW + 2) / 2, Y: min(0, minY-1), Width: max(1, maxW+2), Height: max(float64(h), maxY+1) - min(0, minY-1)}
	if style.Align == "left" {
		box.X = 0
	}
	doc.ViewBox = box
	fit := min(1, min(style.Width/style.PixelScale/box.Width, style.Depth/style.PixelScale/box.Height))
	if err := fitRichViewport(doc, style); err != nil {
		return nil, textLayout{}, err
	}
	frame, err := rasterNativeSurfaceDocument(ctx, doc, width, height)
	if err != nil {
		return nil, textLayout{}, err
	}
	tex := image.NewRGBA(frame.Bounds())
	if style.Background != nil {
		draw.Draw(tex, tex.Bounds(), image.NewUniform(*style.Background), image.Point{}, draw.Src)
	}
	draw.Draw(tex, tex.Bounds(), frame, image.Point{}, draw.Over)
	for y := 0; y < height; y++ {
		if err := ctx.Err(); err != nil {
			return nil, textLayout{}, err
		}
		for x := 0; x < width*4; x++ {
			i := y*tex.Stride + x
			tex.Pix[i] = uint8(math.Round(float64(tex.Pix[i]) * style.Opacity))
		}
	}
	view := *doc
	view.LogicalWidth, view.LogicalHeight = float64(width), float64(height)
	view.ViewportFit, view.ViewportAlign = d2scene.ViewportMeet, d2scene.ViewportAlignXMidYMid
	if err := retainStyledNativeVectorSurface(ctx, tex, &view, style.Background, style.Opacity); err != nil {
		return nil, textLayout{}, err
	}
	return tex, textLayout{Lines: lines, FontSize: fontSize * style.PixelScale * fit, LineHeight: fontSize * style.PixelScale * fit * 1.2}, nil
}
