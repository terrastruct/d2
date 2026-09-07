package d2isometricimg

import (
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

// Leaf Markdown is a document in its own source allocation. Container labels
// and sequence annotations already have physical surfaces of their own.
func nativeMarkdownCard(n d2isometric.Node) bool {
	language := strings.ToLower(strings.TrimSpace(n.Metadata.Original.Language))
	return !n.Container && n.SequenceRole == "" && n.Type == d2target.ShapeText && (language == "markdown" || language == "md")
}

const markdownCardPaperDepth = .035

func nativePlainMarkdownCard(n d2isometric.Node) bool {
	s := n.Metadata.Original
	return nativeMarkdownCard(n) && nativePaint(n.Fill, "transparent").A == 0 && !n.StrokeExplicit &&
		n.StrokeWidth == 2 && n.StrokeDash == 0 && s.BorderRadius == 0 &&
		s.FillPattern == "" && !s.DoubleBorder && !s.Multiple && !s.ThreeDee
}

func nativeMarkdownCardInset(n d2isometric.Node) float64 {
	return min(.045, n.Size.X/4, n.Size.Z/4)
}

// The default transparent Markdown face becomes paper. Nontransparent source
// paints and authored linework retain their values; only the presentation copy
// changes, leaving the rich document and its compiled placement untouched.
func nativeMarkdownCardPaint(n d2isometric.Node) d2isometric.Node {
	if nativePaint(n.Fill, "transparent").A == 0 {
		n.Fill = "#fffdf6"
	}
	n.FillExplicit = true
	if !n.StrokeExplicit && (n.Metadata.Original.Stroke == "" || nativeToken(n.Metadata.Original.Stroke)) {
		n.Stroke = "#304552"
		if n.StrokeWidth == 2 {
			n.StrokeWidth = 1
		}
		n.StrokeExplicit = true
	}
	return n
}

func (b *meshBuilder) markdownCard(n d2isometric.Node, tint string) {
	if b.err != nil || n.Opacity <= 0 || n.Size.X <= 0 || n.Size.Z <= 0 {
		return
	}
	if n.Opacity < 1 {
		// Compose the recessed support, paper and printing before fading the
		// document once, so its lower support cannot bleed through its face.
		group := &nativeOpacityGroup{Opacity: n.Opacity}
		first := len(b.triangles)
		n.Opacity = 1
		defer func() {
			for i := first; i < len(b.triangles); i++ {
				b.triangles[i].OpacityGroup = group
			}
		}()
	}

	paint := nativeMarkdownCardPaint(n)
	// Decorative borders and modifiers use the canonical source face rather
	// than dropping a pattern, rounding, second outline or multiple offset.
	if !nativePlainMarkdownCard(n) {
		first := len(b.triangles)
		b.canonicalNode(paint, tint)
		b.classicInkEdges(paint, b.triangles[first:])
		return
	}

	x0, x1 := n.Position.X-n.Size.X/2, n.Position.X+n.Size.X/2
	z0, z1 := n.Position.Z-n.Size.Z/2, n.Position.Z+n.Size.Z/2
	floor := n.Position.Y - n.Size.Y/2
	top := floor + nativeCanonicalHeight(n, b.scale)
	// The recessed support remains inside even a one-pixel source node.
	inset := nativeMarkdownCardInset(n)
	b.markdownCardSlab(paint, x0+inset, z0+inset, x1-inset, z1-inset, floor+b.nodeSupportDrop, top-markdownCardPaperDepth, "#d7d8d3", "#b6c0c0", "#7b8a90")
	b.markdownCardSlab(paint, x0, z0, x1, z1, top-markdownCardPaperDepth, top, paint.Fill, "#e4e6e1", paint.Stroke)
	// Reuse the complete rich document, icon and link paths at the raised
	// writing plane. X/Z coordinates, text dimensions and fonts do not change.
	b.canonicalNodeContent(n, nativeFaceSource(n, paint.Fill), paint.Fill, top)
}

func (b *meshBuilder) markdownCardSlab(n d2isometric.Node, x0, z0, x1, z1, bottom, top float64, fill, side, stroke string) {
	first := len(b.triangles)
	profile := []Vec{nv(x0, top, z0), nv(x1, top, z0), nv(x1, top, z1), nv(x0, top, z1)}
	b.extrudedProfile(profile, bottom, nativeMaterial(fill, .80, 0, 1), nativeMaterial(side, .78, 0, 1))
	ink := n
	ink.Type, ink.Fill, ink.Stroke, ink.StrokeWidth = d2target.ShapeRectangle, fill, stroke, 1
	ink.StrokeExplicit = true
	ink.Metadata.Original.Type, ink.Metadata.Original.Fill, ink.Metadata.Original.Stroke = ink.Type, fill, stroke
	ink.Metadata.Original.StrokeWidth, ink.Metadata.Original.Label = 1, ""
	b.classicInkEdges(ink, b.triangles[first:])
}
