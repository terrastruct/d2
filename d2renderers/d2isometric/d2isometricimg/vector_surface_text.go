package d2isometricimg

import (
	"fmt"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

func (w *nativeSurfaceSVGWriter) font(id d2scene.AssetID) (*fontface.ParsedFace, error) {
	if f := w.fonts[id]; f != nil {
		return f, nil
	}
	asset, ok := w.doc.Assets[id].(d2scene.FontAsset)
	if !ok {
		return nil, fmt.Errorf("native SVG missing font asset %q", id)
	}
	if len(w.fonts) >= 96 || len(asset.Data) > 64<<20 {
		return nil, fmt.Errorf("native SVG font assets exceed limit")
	}
	source, bundled, err := fontface.RegisteredBundledFace(asset.Data, asset.FaceIndex)
	if err != nil {
		return nil, err
	}
	if !bundled {
		source, bundled, err = fontface.RegisteredBundledNotoColorEmoji(asset.Data, asset.FaceIndex)
		if err != nil {
			return nil, err
		}
	}
	var face *fontface.ParsedFace
	if bundled {
		face, err = source.CloneReadOnly()
	} else {
		face, err = fontface.ParseFace(asset.Data, asset.FaceIndex)
	}
	if err == nil {
		w.fonts[id] = face
	}
	return face, err
}

func (w *nativeSurfaceSVGWriter) text(run d2scene.TextRun, depth int) string {
	if !w.admit(depth) {
		return ""
	}
	primary, err := w.font(run.Font.Asset)
	if err != nil {
		w.err = err
		return ""
	}
	ppem := fixed.Int26_6(math.Round(run.Font.Size * 64))
	if ppem <= 0 {
		w.err = fmt.Errorf("native SVG has invalid text size")
		return ""
	}
	glyphs := run.Glyphs
	advance := 0.
	if len(glyphs) == 0 && run.Text != "" {
		ids := append([]d2scene.AssetID{run.Font.Asset}, run.Fallbacks...)
		var faces []fontface.ShapeFace
		seen := map[d2scene.AssetID]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			f, err := w.font(id)
			if err != nil {
				w.err = err
				return ""
			}
			faces = append(faces, fontface.ShapeFace{ID: string(id), Face: f})
		}
		var shaper fontface.ShapingWorkspace
		shaped, err := shaper.ShapeTextTransient(w.ctx, run.Text, ppem, faces, fontface.ShapeLimits{Runes: maxLabelRunes, Faces: 8, CoverageChecks: 8000000, Runs: 200000, Glyphs: 1000000})
		if err != nil {
			w.err = err
			return ""
		}
		advance = shaped.Advance
		for _, g := range shaped.Glyphs {
			glyphs = append(glyphs, d2scene.Glyph{ID: g.ID, Empty: g.Empty, Asset: d2scene.AssetID(faces[g.Face].ID), Position: d2scene.Point{X: g.PositionX, Y: g.PositionY}, Advance: g.Advance})
		}
	} else {
		for _, g := range glyphs {
			advance = max(advance, g.Position.X+g.Advance)
		}
	}
	if len(glyphs) > maxLabelRunes {
		w.err = fmt.Errorf("native SVG exceeds glyph count")
		return ""
	}
	anchor := 0.
	if run.Anchor == d2scene.AnchorMiddle {
		anchor = -advance / 2
	}
	if run.Anchor == d2scene.AnchorEnd {
		anchor = -advance
	}
	var body strings.Builder
	for _, g := range glyphs {
		if g.Empty {
			continue
		}
		if err := w.ctx.Err(); err != nil {
			w.err = err
			return ""
		}
		id := g.Asset
		if id == "" {
			id = run.Font.Asset
		}
		face, err := w.font(id)
		if err != nil {
			w.err = err
			return ""
		}
		origin := d2scene.Point{X: run.Origin.X + anchor + g.Position.X, Y: run.Origin.Y + g.Position.Y}
		if face.IsBundledNotoColorEmoji() {
			plan, found, err := face.CompileBundledNotoColorEmojiCOLRv1Plan(g.ID)
			if err != nil {
				w.err = err
				return ""
			}
			if found {
				w.append(&body, w.colorGlyph(face, plan, origin, run.Font.Size, depth+1))
				continue
			}
		}
		layers, colored, err := face.COLR0GlyphLayers(g.ID)
		if err != nil {
			w.err = err
			return ""
		}
		if colored {
			for _, layer := range layers {
				fill := run.Fill
				if !layer.Foreground {
					fill = d2scene.SolidPaint{Color: layer.Color}
				}
				w.append(&body, w.glyph(face, layer.GlyphID, ppem, origin, fill, nil, depth+1))
			}
			continue
		}
		w.append(&body, w.glyph(face, g.ID, ppem, origin, run.Fill, run.Stroke, depth+1))
	}
	if (run.Underline || run.Strike) && advance > 0 {
		var buffer sfnt.Buffer
		metrics, err := primary.Outline.Metrics(&buffer, ppem, font.HintingNone)
		if err != nil {
			w.err = err
			return ""
		}
		scale := w.transform.MaxScale()
		thickness := max(run.Font.Size/16, 1/max(1e-12, scale))
		add := func(top float64) {
			w.append(&body, w.primitive(d2scene.Rect{Box: d2scene.Box{X: run.Origin.X + anchor, Y: top, Width: advance, Height: thickness}, Fill: run.Fill}, depth+1))
		}
		if run.Underline {
			add(run.Origin.Y + max(run.Font.Size/12, float64(metrics.Descent)/64*.25))
		}
		if run.Strike {
			xHeight := float64(metrics.XHeight) / 64
			if xHeight <= 0 {
				xHeight = run.Font.Size * .5
			}
			add(run.Origin.Y - xHeight/2 - thickness/2)
		}
	}
	return body.String()
}

func nativeSVGGlyphPath(face *fontface.ParsedFace, id uint32, ppem fixed.Int26_6, origin d2scene.Point) (d2scene.Path, error) {
	if id == 0 || id > math.MaxUint16 || int(id) >= face.Outline.NumGlyphs() {
		return d2scene.Path{}, fmt.Errorf("native SVG has invalid glyph ID %d", id)
	}
	var buffer sfnt.Buffer
	segments, err := face.Outline.LoadGlyph(&buffer, sfnt.GlyphIndex(id), ppem, nil)
	if err != nil {
		return d2scene.Path{}, fmt.Errorf("native SVG glyph %d outline: %w", id, err)
	}
	path := d2scene.Path{}
	point := func(p fixed.Point26_6) d2scene.Point {
		return d2scene.Point{X: origin.X + float64(p.X)/64, Y: origin.Y + float64(p.Y)/64}
	}
	open := false
	for _, s := range segments {
		a, b, c := point(s.Args[0]), point(s.Args[1]), point(s.Args[2])
		switch s.Op {
		case sfnt.SegmentOpMoveTo:
			if open {
				path.Commands = append(path.Commands, d2scene.ClosePath())
			}
			path.Commands = append(path.Commands, d2scene.MoveTo(a.X, a.Y))
			open = true
		case sfnt.SegmentOpLineTo:
			path.Commands = append(path.Commands, d2scene.LineTo(a.X, a.Y))
		case sfnt.SegmentOpQuadTo:
			path.Commands = append(path.Commands, d2scene.QuadraticTo(a.X, a.Y, b.X, b.Y))
		case sfnt.SegmentOpCubeTo:
			path.Commands = append(path.Commands, d2scene.CubicTo(a.X, a.Y, b.X, b.Y, c.X, c.Y))
		}
	}
	if open {
		path.Commands = append(path.Commands, d2scene.ClosePath())
	}
	return path, nil
}

type nativeSVGFontGlyph struct {
	face *fontface.ParsedFace
	id   uint32
	ppem fixed.Int26_6
}

// Reuse exact font curves across rich-text runs. Position and source paint
// belong to the use, while each font/glyph/size outline is serialized once.
func (w *nativeSurfaceSVGWriter) glyphDefinition(face *fontface.ParsedFace, id uint32, ppem fixed.Int26_6) string {
	if w.glyphs == nil {
		w.glyphs = make(map[nativeSVGFontGlyph]string)
	}
	key := nativeSVGFontGlyph{face: face, id: id, ppem: ppem}
	if name := w.glyphs[key]; name != "" {
		return name
	}
	path, err := nativeSVGGlyphPath(face, id, ppem, d2scene.Point{})
	if err != nil {
		w.err = err
		return ""
	}
	name := w.id("glyph")
	w.def("<path" + nativeSVGAttr("id", name) + nativeSVGAttr("d", w.path(path)) + "/>")
	w.glyphs[key] = name
	return name
}

func (w *nativeSurfaceSVGWriter) glyph(face *fontface.ParsedFace, id uint32, ppem fixed.Int26_6, origin d2scene.Point, fill d2scene.Paint, stroke *d2scene.Stroke, depth int) string {
	if !w.admit(depth) {
		return ""
	}
	solid := func(paint d2scene.Paint) bool {
		switch paint.(type) {
		case nil, d2scene.SolidPaint, *d2scene.SolidPaint:
			return true
		}
		return false
	}
	if !solid(fill) || stroke != nil && !solid(stroke.Paint) {
		// Absolute paint coordinates must retain their relationship to a glyph.
		path, err := nativeSVGGlyphPath(face, id, ppem, origin)
		if err != nil {
			w.err = err
			return ""
		}
		path.Fill, path.Stroke = fill, stroke
		return w.primitive(path, depth+1)
	}
	name := w.glyphDefinition(face, id, ppem)
	if w.err != nil {
		return ""
	}
	return "<use" + nativeSVGAttr("href", "#"+name) + nativeSVGAttr("transform", "translate("+nativeSVGNumber(origin.X)+" "+nativeSVGNumber(origin.Y)+")") + w.style(fill, stroke, depth+1) + "/>"
}
