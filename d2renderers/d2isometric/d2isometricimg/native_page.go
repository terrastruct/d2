package d2isometricimg

import (
	"image"
	"math"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

// The source page reserves its upper-right corner for a fold. Give that
// corner a real inclined surface, within the same source footprint, so light,
// occlusion and the object outline describe the fold instead of a printed glyph.
func (b *meshBuilder) nativePageFold(n d2isometric.Node, face labelSurface, texture *image.RGBA, material *Material, dx, dz, top float64, profile []Vec) {
	x, z := n.Position.X+n.Size.X/2+dx, n.Position.Z-n.Size.Z/2+dz
	var hinge []Vec
	start := -1
	// The source contour traverses the top edge, rounded fold, then right
	// edge. Use those actual vertices, including source coordinate rounding.
	// Clamping the corner to half the page height truncates short pages.
	for i, p := range profile {
		if math.Abs(p.Z-z) < 1e-8 {
			start = i
		}
		if start >= 0 && i > start && math.Abs(p.X-x) < 1e-8 {
			hinge = profile[start : i+1]
			break
		}
	}
	if len(hinge) < 2 {
		return
	}
	// The fold attaches directly to the source's rounded diagonal. A
	// separate slab above this edge creates two competing rims and leaves
	// an acute ink spike where its top edge meets the page perimeter.
	cw, cd := x-hinge[0].X, hinge[len(hinge)-1].Z-z
	tip := nv(x-cw*.90, top+math.Min(cw, cd)*.62, z+cd*.90)
	mat := *material
	if texture != nil {
		mat = *nativeMaterial("white", .68, 0, n.Opacity)
		mat.Texture, mat.Vector = texture, nativeVectorForTexture(b.ctx, texture)
	}
	// Paper thickness is below the outline width. One inclined sheet gives
	// the fold its own lighting and shadow without a second free-edge rim.
	for i := 1; i < len(hinge); i++ {
		a, c := hinge[i-1], hinge[i]
		normal := nunit(ncross(nsub(tip, a), nsub(c, a)))
		vertex := func(at Vec) Vertex {
			return Vertex{Position: at, Normal: normal,
				U: (at.X-face.center.X)/face.width + .5,
				V: (at.Z-face.center.Z)/face.depth + .5}
		}
		b.triangle(vertex(a), vertex(tip), vertex(c), &mat, true)
	}
}

// The second source Page path draws the flat fold and repeats an inset
// perimeter. The physical fold replaces that glyph; the outer source path
// continues to carry authored dash, width and border-label aperture.
func nativeRemovePageGlyph(root *d2scene.Node) {
	pathIndex := 0
	var visit func(*d2scene.Node)
	visit = func(n *d2scene.Node) {
		if n == nil {
			return
		}
		if p, ok := n.Primitive.(d2scene.Path); ok && p.Stroke != nil {
			if pathIndex == 1 {
				p.Stroke = nil
				n.Primitive = p
			}
			pathIndex++
		}
		for _, child := range n.Children {
			visit(child)
		}
	}
	// nativeFaceSource creates exactly one undecorated Page shape. Its
	// ordinary paths have no IDs; their stroked order is outer, inner glyph.
	// Pattern overlays between them carry fill only.
	visit(root)
}
