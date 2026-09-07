package d2isometricimg

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/d2lang/d2/d2renderers/d2fonts"
	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

const (
	maxTextPixels       = 8 * 1024 * 1024
	maxTextLabels       = 19000
	maxLabelRunes       = 64 * 1024
	maxTextSourceBytes  = 8 * 1024 * 1024
	maxTextFontBytes    = 32 * 1024 * 1024
	maxTextGlyphWork    = 8_000_000
	maxTextOutlineWork  = 8_000_000
	textReferencePixels = 64
)

// labelTextStyle describes a physical print area, not a camera-facing caption.
// FontSize uses original D2 pixels; Width and Depth use the scene's world units.
// Color is unpremultiplied. The returned texture uses premultiplied image.RGBA.
type labelTextStyle struct {
	Width, Depth, FontSize, PixelScale float64
	FontFamily                         string
	Bold, Italic, Underline            bool
	Color                              color.NRGBA
	Background                         *color.NRGBA
	Opacity                            float64
	Align                              string
	MaxLines                           int
}

type textLayout struct {
	Lines                []string
	FontSize, LineHeight float64 // world units
	Truncated            bool
}

type textPainter struct {
	fallbackFonts                      *d2scenebuild.FontFallbackOptions
	ctx                                context.Context
	count, used, tileWidth, tileHeight int
	pixels, glyphWork, outlineWork     int
	sourceBytes, fontBytes             int
	primary, mono                      d2fonts.FontFamily
	outputDensity                      float64
	faces                              map[d2fonts.Font]*fontface.ParsedFace
	shaper                             fontface.ShapingWorkspace
}

type shapedLabelLine struct {
	text       fontface.ShapedText
	minX, maxX float64
	minY, maxY float64
}

func newTextPainter(ctx context.Context, labelCount int) (*textPainter, error) {
	if ctx == nil {
		return nil, fmt.Errorf("isometric text requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if labelCount < 0 || labelCount > maxTextLabels {
		return nil, fmt.Errorf("isometric text exceeds %d labels", maxTextLabels)
	}
	budget := maxTextPixels / max(1, labelCount)
	w := max(1, int(math.Floor(math.Sqrt(float64(budget)))))
	h := w
	return &textPainter{ctx: ctx, count: labelCount, tileWidth: w, tileHeight: h, faces: make(map[d2fonts.Font]*fontface.ParsedFace)}, nil
}

func (p *textPainter) configureFontFamilies(primary, mono *d2fonts.FontFamily) {
	p.primary, p.mono = nativeFontFamilies(primary, mono)
}

func (p *textPainter) configureFallbackFonts(fonts *d2scenebuild.FontFallbackOptions) {
	p.fallbackFonts = fonts
}

func (p *textPainter) configureOutputDensity(pixelsPerWorld float64) {
	p.outputDensity = pixelsPerWorld
}

func (p *textPainter) face(style labelTextStyle) (*fontface.ParsedFace, bool, error) {
	family, err := nativeFontFamily(style.FontFamily, p.primary, p.mono)
	if err != nil {
		return nil, false, err
	}
	fontStyle := d2fonts.FONT_STYLE_REGULAR
	if style.Bold {
		fontStyle = d2fonts.FONT_STYLE_BOLD
	} else if style.Italic {
		fontStyle = d2fonts.FONT_STYLE_ITALIC
	}
	key := d2fonts.Font{Family: family, Style: fontStyle}
	if face := p.faces[key]; face != nil {
		return face, style.Bold && style.Italic, nil
	}
	data, ok := d2fonts.FontFaces.Lookup(key)
	if !ok {
		return nil, false, fmt.Errorf("isometric text font %s/%s is not loaded", family, fontStyle)
	}
	if len(p.faces) >= 24 || len(data) > maxTextFontBytes-p.fontBytes {
		return nil, false, fmt.Errorf("isometric text exceeds font asset budget")
	}
	// Registered CLI/custom font bytes are explicit inputs. Retain an owned
	// copy for non-bundled faces; never discover fonts on the host filesystem.
	source, bundled, err := fontface.RegisteredBundledFace(data, 0)
	if err != nil {
		return nil, false, fmt.Errorf("isometric text font: %w", err)
	}
	var face *fontface.ParsedFace
	if bundled {
		face, err = source.CloneReadOnly()
	} else {
		face, err = fontface.ParseFace(append([]byte(nil), data...), 0)
	}
	if err != nil {
		return nil, false, fmt.Errorf("isometric text font parse: %w", err)
	}
	p.faces[key] = face
	p.fontBytes += len(data)
	return face, style.Bold && style.Italic, nil
}

func (p *textPainter) texture(value string, style labelTextStyle) (*image.RGBA, textLayout, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, textLayout{}, err
	}
	if p.used >= p.count {
		return nil, textLayout{}, fmt.Errorf("isometric text label allocation exceeds its declared count")
	}
	for _, value := range []float64{style.Width, style.Depth, style.PixelScale} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return nil, textLayout{}, fmt.Errorf("isometric text has an invalid physical print area or pixel scale")
		}
	}
	if style.FontSize < 0 || math.IsNaN(style.FontSize) || math.IsInf(style.FontSize, 0) || math.IsNaN(style.Opacity) || math.IsInf(style.Opacity, 0) || style.Opacity < 0 || style.Opacity > 1 {
		return nil, textLayout{}, fmt.Errorf("isometric text has invalid font size or opacity")
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxLabelRunes || len(value) > maxTextSourceBytes-p.sourceBytes {
		return nil, textLayout{}, fmt.Errorf("isometric text exceeds source budget or contains invalid UTF-8")
	}
	p.sourceBytes += len(value)
	face, shear, err := p.face(style)
	if err != nil {
		return nil, textLayout{}, err
	}
	budget := maxTextPixels / max(1, p.count)
	w, h := surfaceTextureDimensionsAtDensity(style.Width, style.Depth, 4096, budget, p.outputDensity)
	// Keep layout independent of output sampling. In particular, a two-pixel
	// safety inset must not shrink authored text more in a small export.
	layoutW, layoutH := surfaceTextureDimensions(style.Width, style.Depth, 4096, budget)
	if w*h > maxTextPixels-p.pixels {
		return nil, textLayout{}, fmt.Errorf("isometric text exceeds its %d pixel texture budget", maxTextPixels)
	}
	p.used++
	p.pixels += w * h
	fallback, err := nativeTextNeedsFallback(p.ctx, value, face)
	if err != nil {
		return nil, textLayout{}, err
	}
	if fallback {
		family, e := nativeFontFamily(style.FontFamily, p.primary, p.mono)
		if e != nil {
			return nil, textLayout{}, e
		}
		return nativeFallbackTextTexture(p.ctx, value, style, w, h, family, p.mono, p.fallbackFonts)
	}
	texture := image.NewRGBA(image.Rect(0, 0, w, h))
	if style.Background != nil {
		background := *style.Background
		background.A = uint8(math.Round(float64(background.A) * style.Opacity))
		draw.Draw(texture, texture.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	}
	text := normalizeLabel(value)
	cache := make(map[string]shapedLabelLine)
	shape := func(line string) (shapedLabelLine, error) {
		if cached, ok := cache[line]; ok {
			return cached, nil
		}
		remaining := maxTextGlyphWork - p.glyphWork
		if remaining <= 0 {
			return shapedLabelLine{}, fmt.Errorf("isometric text glyph shaping work exceeds %d", maxTextGlyphWork)
		}
		shaped, err := p.shaper.ShapeTextTransient(p.ctx, line, fixed.I(textReferencePixels), []fontface.ShapeFace{{ID: "surface", Face: face}}, fontface.ShapeLimits{
			Runes: maxLabelRunes + 1, Faces: 1, CoverageChecks: int64(remaining), Runs: maxLabelRunes + 1, Glyphs: min(remaining, (maxLabelRunes+1)*4),
		})
		if err != nil {
			return shapedLabelLine{}, fmt.Errorf("isometric text shaping: %w", err)
		}
		p.glyphWork += max(len(shaped.Glyphs), int(shaped.CoverageChecks))
		// The line cache outlives the next shaping call's borrowed glyphs.
		shaped.Glyphs = slices.Clone(shaped.Glyphs)
		result := shapedLabelLine{text: shaped, maxX: shaped.Advance}
		for _, glyph := range shaped.Glyphs {
			if !glyph.HasInk {
				continue
			}
			for _, x := range []float64{float64(glyph.Ink.Min.X) / 64, float64(glyph.Ink.Max.X) / 64} {
				for _, y := range []float64{float64(glyph.Ink.Min.Y) / 64, float64(glyph.Ink.Max.Y) / 64} {
					x, y := x+glyph.PositionX, y+glyph.PositionY
					if shear {
						x -= y * .20
					}
					result.minX = math.Min(result.minX, x)
					result.maxX = math.Max(result.maxX, x)
					result.minY = math.Min(result.minY, y)
					result.maxY = math.Max(result.maxY, y)
				}
			}
		}
		cache[line] = result
		return result, nil
	}
	scale := float64(layoutW) / style.Width
	fontSize := style.FontSize * style.PixelScale * scale
	if style.FontSize <= 0 {
		fontSize = .235 * scale
	}
	inset := math.Min(2, math.Min(float64(layoutW), float64(layoutH))*.05)
	availableW, availableH := float64(layoutW)-2*inset, float64(layoutH)-2*inset
	maxLines := style.MaxLines
	if maxLines <= 0 || maxLines > 64 {
		maxLines = 64
	}
	measure := func(line string, size float64) (float64, error) {
		shaped, err := shape(line)
		return (shaped.maxX - shaped.minX) * size / textReferencePixels, err
	}
	layout, err := fitNativeLabel(text, false, measure, availableW, availableH, fontSize, maxLines)
	if err != nil {
		return nil, textLayout{}, err
	}
	fontPixels, lineHeight := layout.FontSize, layout.LineHeight
	unitScale := fontPixels / textReferencePixels
	minY, maxY := math.Inf(1), math.Inf(-1)
	shapedLines := make([]shapedLabelLine, len(layout.Lines))
	for i, line := range layout.Lines {
		shaped, err := shape(line)
		if err != nil {
			return nil, textLayout{}, err
		}
		shapedLines[i] = shaped
		minY = math.Min(minY, float64(i)*lineHeight+shaped.minY*unitScale)
		maxY = math.Max(maxY, float64(i)*lineHeight+shaped.maxY*unitScale)
	}
	baseline := (float64(layoutH)-(maxY-minY))/2 - minY
	drawScaleX, drawScaleY := float64(w)/float64(layoutW), float64(h)/float64(layoutH)
	raster := vector.NewRasterizer(w, h)
	var outline d2scene.Path
	captureVector := nativeVectorEnabled(p.ctx)
	record := func(command d2scene.PathCommand) {
		if captureVector {
			outline.Commands = append(outline.Commands, command)
		}
	}
	var buffer sfnt.Buffer
	for i, line := range shapedLines {
		if err := p.ctx.Err(); err != nil {
			return nil, textLayout{}, err
		}
		x := (float64(layoutW)-(line.maxX-line.minX)*unitScale)/2 - line.minX*unitScale
		if style.Align == "left" {
			x = inset - line.minX*unitScale
		}
		y := baseline + float64(i)*lineHeight
		for _, glyph := range line.text.Glyphs {
			if glyph.Empty || !glyph.HasInk {
				continue
			}
			segments, err := face.Outline.LoadGlyph(&buffer, sfnt.GlyphIndex(glyph.ID), fixed.I(textReferencePixels), nil)
			if err != nil {
				return nil, textLayout{}, fmt.Errorf("isometric text glyph outline: %w", err)
			}
			p.outlineWork += len(segments)
			if p.outlineWork > maxTextOutlineWork {
				return nil, textLayout{}, fmt.Errorf("isometric text outline work exceeds %d", maxTextOutlineWork)
			}
			transform := func(point fixed.Point26_6) (float32, float32) {
				gx, gy := float64(point.X)/64+glyph.PositionX, float64(point.Y)/64+glyph.PositionY
				if shear {
					gx -= gy * .20
				}
				return float32((x + gx*unitScale) * drawScaleX), float32((y + gy*unitScale) * drawScaleY)
			}
			open := false
			for _, segment := range segments {
				switch segment.Op {
				case sfnt.SegmentOpMoveTo:
					if open {
						raster.ClosePath()
						record(d2scene.ClosePath())
					}
					ax, ay := transform(segment.Args[0])
					raster.MoveTo(ax, ay)
					record(d2scene.MoveTo(float64(ax), float64(ay)))
					open = true
				case sfnt.SegmentOpLineTo:
					ax, ay := transform(segment.Args[0])
					raster.LineTo(ax, ay)
					record(d2scene.LineTo(float64(ax), float64(ay)))
				case sfnt.SegmentOpQuadTo:
					ax, ay := transform(segment.Args[0])
					bx, by := transform(segment.Args[1])
					raster.QuadTo(ax, ay, bx, by)
					record(d2scene.QuadraticTo(float64(ax), float64(ay), float64(bx), float64(by)))
				case sfnt.SegmentOpCubeTo:
					ax, ay := transform(segment.Args[0])
					bx, by := transform(segment.Args[1])
					cx, cy := transform(segment.Args[2])
					raster.CubeTo(ax, ay, bx, by, cx, cy)
					record(d2scene.CubicTo(float64(ax), float64(ay), float64(bx), float64(by), float64(cx), float64(cy)))
				}
			}
			if open {
				raster.ClosePath()
				record(d2scene.ClosePath())
			}
		}
		if style.Underline && line.maxX > line.minX {
			left, right := x+line.minX*unitScale, x+line.maxX*unitScale
			top, bottom := y+fontPixels*.10, y+fontPixels*.15
			left, right = left*drawScaleX, right*drawScaleX
			top, bottom = top*drawScaleY, bottom*drawScaleY
			raster.MoveTo(float32(left), float32(top))
			raster.LineTo(float32(right), float32(top))
			raster.LineTo(float32(right), float32(bottom))
			raster.LineTo(float32(left), float32(bottom))
			raster.ClosePath()
			record(d2scene.MoveTo(left, top))
			record(d2scene.LineTo(right, top))
			record(d2scene.LineTo(right, bottom))
			record(d2scene.LineTo(left, bottom))
			record(d2scene.ClosePath())
		}
	}
	foreground := style.Color
	foreground.A = uint8(math.Round(float64(foreground.A) * style.Opacity))
	raster.Draw(texture, texture.Bounds(), image.NewUniform(foreground), image.Point{})
	if captureVector {
		outline.Fill = d2scene.SolidPaint{Color: foreground}
		root := d2scene.NewNode(nil)
		box := d2scene.Box{Width: float64(w), Height: float64(h)}
		if style.Background != nil {
			background := *style.Background
			background.A = uint8(math.Round(float64(background.A) * style.Opacity))
			root.Children = append(root.Children, d2scene.NewNode(d2scene.Rect{Box: box, Fill: d2scene.SolidPaint{Color: background}}))
		}
		root.Children = append(root.Children, d2scene.NewNode(outline))
		if err := retainNativeVectorSurface(p.ctx, texture, d2scene.NewDocument(box, root)); err != nil {
			return nil, textLayout{}, err
		}
	}
	layout.FontSize /= scale
	layout.LineHeight /= scale
	return texture, layout, nil
}

// D2 has already allocated the label's physical dimensions. Preserve its
// authored lines and whitespace instead of reflowing or silently dropping text.
// A constrained surface scales the complete label uniformly; admission errors
// report resource limits explicitly instead of replacing the tail with dots.
func normalizeLabel(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

func fitNativeLabel(text string, _ bool, measure func(string, float64) (float64, error), width, height, preferred float64, _ int) (textLayout, error) {
	width, height = math.Max(width, 1e-9), math.Max(height, 1e-9)
	lines := strings.Split(normalizeLabel(text), "\n")
	size := math.Min(preferred, height/(float64(len(lines))*1.18))
	for _, line := range lines {
		w, err := measure(line, 1)
		if err != nil {
			return textLayout{}, err
		}
		if w > 0 {
			size = math.Min(size, width/w)
		}
	}
	return textLayout{Lines: lines, FontSize: size, LineHeight: size * 1.18}, nil
}
