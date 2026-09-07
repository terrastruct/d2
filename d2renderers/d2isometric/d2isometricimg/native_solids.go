package d2isometricimg

import (
	"image"
	"math"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2target"
)

// Surface decoration must not select a different physical shape. In
// particular, a patterned cylinder is still a barrel, not an extruded drawing
// of the cylinder icon. Source X/Z dimensions remain the solid's footprint.
func nativeSolidNode(n d2isometric.Node) bool {
	if n.Container {
		return false
	}
	switch n.Type {
	case d2target.ShapeCylinder, d2target.ShapeQueue, d2target.ShapeCircle, d2target.ShapeOval, d2target.ShapeHexagon:
		return true
	}
	return false
}

func nativeSolidHeight(n d2isometric.Node) float64 {
	h := max(.04, n.Size.Y)
	if n.Metadata.Original.ThreeDee {
		scale := .01
		if n.Metadata.Original.Width > 0 {
			scale = n.Size.X / float64(n.Metadata.Original.Width)
		}
		h += d2target.THREE_DEE_OFFSET * scale
	}
	return h
}

type nativeSolidPaint struct {
	cap, ink, wall  *Material
	inkFit          float64
	shadowFillAlpha *uint8
}

func (b *meshBuilder) solidPaint(n d2isometric.Node, kind, fill string) nativeSolidPaint {
	s := nativeFaceSource(n, fill)
	s.Type = kind
	// A cap and its unwrapped patterned wall share the node's original
	// allocation. Decorating a volume cannot double the face texture budget.
	budget := b.faceMaxPixels
	patterned := s.FillPattern != "" && s.FillPattern != "none"
	if patterned && budget > 0 {
		b.faceMaxPixels = max(1, budget/2)
	}
	defer func() { b.faceMaxPixels = budget }()
	var tex, stroke *image.RGBA
	if nativeClassicRim(n) {
		// Full physical rim ink is emitted by classicInkEdges. Do not carve
		// opacity out of the lower fill for a texture layer that is omitted.
		tex, _, _ = b.nativeFaceLayers(s)
	} else {
		tex, stroke, _ = b.nativeFaceLayers(s, n.Opacity)
	}
	cap := nativeMaterial("white", .68, 0, n.Opacity)
	cap.Texture, cap.Vector = tex, nativeVectorForTexture(b.ctx, tex)
	if tex == nil {
		cap.Color.A = 0
	}
	ink := nativeMaterial("white", .68, 0, n.Opacity)
	ink.Texture, ink.Vector, ink.Unlit = stroke, nativeVectorForTexture(b.ctx, stroke), true
	if stroke == nil {
		ink.Color.A = 0
	}
	// A barrel's crown points upward. Let its actual normals and lighting
	// establish tone, using the same source material as the upright end cap.
	wall := nativeMaterial(fill, .68, 0, n.Opacity)
	if patterned && b.err == nil {
		wallSource := s
		wallSource.Type = d2target.ShapeRectangle
		wallSource.Text = d2target.Text{}
		wallSource.LabelPosition = ""
		wallSource.Stroke, wallSource.StrokeWidth, wallSource.StrokeDash = "transparent", 0, 0
		wallSource.DoubleBorder, wallSource.BorderRadius = false, 0
		wallTex, _ := b.nativeFace(wallSource)
		wall = nativeMaterial("white", .68, 0, n.Opacity)
		wall.Texture, wall.Vector = wallTex, nativeVectorForTexture(b.ctx, wallTex)
		if wallTex == nil {
			wall.Color.A = 0
		}
	}
	var shadowFillAlpha *uint8
	if alpha := nativeMaterial(fill, .68, 0, n.Opacity).Color.A; alpha > 0 {
		// This texture paints an existing filled cap. Its rasterized perimeter
		// is not a physical hole that can weaken the wall's cast silhouette.
		shadowFillAlpha = &alpha
	}
	if nativePaint(fill, "transparent").A == 255 {
		cap.svgSolidTexture = true
		wall.svgSolidTexture = true
		cap.Vector = nativeVectorSolidCap(cap.Vector, nativePaint(fill, "transparent"))
	}
	return nativeSolidPaint{cap: cap, ink: ink, wall: wall, inkFit: b.solidInkFit(stroke, kind == d2target.ShapeHexagon), shadowFillAlpha: shadowFillAlpha}
}

// A centered SVG stroke is an offset curve, not another exact ellipse. Its
// antialiasing fringe can also extend a fraction of a pixel beyond that curve.
// Fit the complete painted extent inside the actual convex cap polygon, with
// no geometry expansion, stroke thickening, or unsupported decal corners.
// Only row endpoints need testing: every cap is convex. Work is bounded by the
// already-budgeted texture pixels and O(height) contour tests.
func (b *meshBuilder) solidInkFit(tex *image.RGBA, hex bool) float64 {
	if tex == nil {
		return 1
	}
	bounds := tex.Bounds()
	radius := 1.
	include := func(x, y float64) {
		if !hex {
			radius = max(radius, math.Hypot(x, y)/math.Cos(math.Pi/nativeUprightSegments))
			return
		}
		for i := 0; i < 6; i++ {
			ax, ay := nativeUprightPoint(i, true)
			bx, by := nativeUprightPoint(i+1, true)
			nx, ny := by-ay, ax-bx
			radius = max(radius, (nx*x+ny*y)/(nx*ax+ny*ay))
		}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if (y-bounds.Min.Y)%64 == 0 {
			if err := b.ctx.Err(); err != nil {
				b.err = err
				return 1
			}
		}
		x0, x1 := bounds.Max.X, bounds.Min.X
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if tex.Pix[tex.PixOffset(x, y)+3] != 0 {
				x0, x1 = min(x0, x), max(x1, x+1)
			}
		}
		if x1 <= x0 {
			continue
		}
		for _, x := range []int{x0, x1} {
			for _, py := range []int{y, y + 1} {
				include(2*float64(x-bounds.Min.X)/float64(bounds.Dx())-1, 2*float64(py-bounds.Min.Y)/float64(bounds.Dy())-1)
			}
		}
	}
	return 1 / radius
}

// The same source paint is applied to a convex cap and a rounded sidewall.
// Curved normals, sharp rims and directional light establish volume;
// there is no 2D cylinder outline or synthetic hardware base underneath it.
func (b *meshBuilder) solidNode(n d2isometric.Node) {
	fill := n.Fill
	if nativeToken(n.Metadata.Original.Fill) && !n.FillExplicit {
		fill = "#dce5eb"
	}
	kind := n.Type
	if kind == d2target.ShapeCylinder || kind == d2target.ShapeQueue || kind == d2target.ShapeCircle {
		kind = d2target.ShapeOval
	}
	paint := b.solidPaint(n, kind, fill)
	if b.err != nil {
		return
	}
	w, d := n.Size.X, n.Size.Z
	h := nativeSolidHeight(n)
	floor := n.Position.Y - n.Size.Y/2
	scale := b.scale
	if scale <= 0 {
		scale = .01
	}
	for copy := 1; copy >= 0; copy-- {
		if copy == 1 && !n.Metadata.Original.Multiple {
			continue
		}
		center := nv(n.Position.X+float64(copy)*d2target.MULTIPLE_OFFSET*scale,
			floor+b.nodeSupportDrop-float64(copy)*min(.08, h*.18), n.Position.Z-float64(copy)*d2target.MULTIPLE_OFFSET*scale)
		if n.Type == d2target.ShapeQueue {
			b.solidBarrel(center, w, d, h-b.nodeSupportDrop, paint, nativeQueueCrown(n))
		} else {
			b.solidUpright(center, w, d, h-b.nodeSupportDrop, n.Type == d2target.ShapeHexagon, paint)
		}
	}
	// Plan support from the final painted content, after icon/text partitioning.
	// Keep the original cylinder/queue inner box and compiled label metrics.
	s := nativeFaceSource(n, fill)
	first := len(b.triangles)
	b.canonicalNodeContent(n, s, fill, floor+h)
	if n.Type != d2target.ShapeQueue {
		b.solidContentSupport(n, fill, floor, h, first)
	}
}

func (b *meshBuilder) solidContentSupport(n d2isometric.Node, fill string, floor, height float64, first int) {
	if b.err != nil || first == len(b.triangles) {
		return
	}
	left, right := n.Position.X-n.Size.X/2, n.Position.X+n.Size.X/2
	front, back := n.Position.Z-n.Size.Z/2, n.Position.Z+n.Size.Z/2
	x0, z0, x1, z1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	materials := make(map[*Material][]Triangle)
	order := []*Material{}
	for _, tri := range b.triangles[first:] {
		if !tri.NoDepthWrite || tri.Material == nil || tri.Material.Texture == nil {
			continue
		}
		if _, ok := materials[tri.Material]; !ok {
			order = append(order, tri.Material)
		}
		materials[tri.Material] = append(materials[tri.Material], tri)
	}
	for _, mat := range order {
		loX, loZ, hiX, hiZ := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, tri := range materials[mat] {
			for _, v := range tri.V {
				loX, loZ, hiX, hiZ = min(loX, v.Position.X), min(loZ, v.Position.Z), max(hiX, v.Position.X), max(hiZ, v.Position.Z)
			}
		}
		ink := solidTextureInk(mat.Texture)
		if ink.Empty() {
			continue
		}
		tb := mat.Texture.Bounds()
		a, c := loX+(hiX-loX)*float64(ink.Min.X-tb.Min.X)/float64(tb.Dx()), loZ+(hiZ-loZ)*float64(ink.Min.Y-tb.Min.Y)/float64(tb.Dy())
		b, d := loX+(hiX-loX)*float64(ink.Max.X-tb.Min.X)/float64(tb.Dx()), loZ+(hiZ-loZ)*float64(ink.Max.Y-tb.Min.Y)/float64(tb.Dy())
		a, c, b, d = max(left, a), max(front, c), min(right, b), min(back, d)
		if b <= a || d <= c {
			continue
		} // authored outside captions stay outside
		x0, z0, x1, z1 = min(x0, a), min(z0, c), max(x1, b), max(z1, d)
	}
	if x1 <= x0 || z1 <= z0 {
		return
	}
	needsPlate := false
	if !needsPlate && n.Type != d2target.ShapeHexagon {
		for _, x := range []float64{x0, x1} {
			for _, z := range []float64{z0, z1} {
				qx, qz := (x-n.Position.X)/(n.Size.X/2), (z-n.Position.Z)/(n.Size.Z/2)
				inradius := math.Cos(math.Pi / nativeUprightSegments)
				needsPlate = needsPlate || qx*qx+qz*qz > inradius*inradius
			}
		}
	}
	if !needsPlate {
		return
	}
	x0, z0, x1, z1 = max(left, x0-.025), max(front, z0-.025), min(right, x1+.025), min(back, z1+.025)
	depth := min(.02, height*.15)
	content := append([]Triangle(nil), b.triangles[first:]...)
	b.triangles = b.triangles[:first]
	b.box(nv((x0+x1)/2, floor+height-depth/2, (z0+z1)/2), nv(x1-x0, depth, z1-z0), nativeMaterial(fill, .68, 0, n.Opacity), .008)
	// Keep the label last for outside-caption clearance and its link regions.
	b.triangles = append(b.triangles, content...)
}

func solidTextureInk(tex image.Image) image.Rectangle {
	bounds := tex.Bounds()
	x0, y0, x1, y1 := bounds.Max.X, bounds.Max.Y, bounds.Min.X, bounds.Min.Y
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			visible := false
			if rgba, ok := tex.(*image.RGBA); ok {
				visible = rgba.Pix[rgba.PixOffset(x, y)+3] > 3
			} else {
				_, _, _, a := tex.At(x, y).RGBA()
				visible = a > 3*257
			}
			if visible {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x+1), max(y1, y+1)
			}
		}
	}
	if x1 <= x0 || y1 <= y0 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

func (p nativeSolidPaint) capVertex(at, normal Vec, u, v float64) Vertex {
	// The native viewport includes the complete centered outline. Map that
	// viewport to the cap, so the fan cannot slice the outer half off its ink.
	fit := p.inkFit
	if fit <= 0 {
		fit = 1
	}
	return Vertex{Position: at, Normal: normal, U: .5 + (u-.5)/fit, V: .5 + (v-.5)/fit}
}

func (b *meshBuilder) solidCapTriangle(a, c, d Vertex, paint nativeSolidPaint) {
	first := len(b.triangles)
	b.triangle(a, c, d, paint.cap, true)
	for i := first; i < len(b.triangles); i++ {
		b.triangles[i].ShadowFillAlpha = paint.shadowFillAlpha
	}
	first = len(b.triangles)
	b.triangle(a, c, d, paint.ink, false)
	for i := first; i < len(b.triangles); i++ {
		// A raster depth bias separates coincident printed ink without raising
		// it off the cap or changing the physical footprint/bounds.
		b.triangles[i].DepthBias = .0005
	}
}

func (b *meshBuilder) solidUpright(center Vec, w, d, h float64, hex bool, paint nativeSolidPaint) {
	count := nativeUprightSegments
	if hex {
		count = 6
	}
	point := func(i int) (float64, float64) { return nativeUprightPoint(i, hex) }
	for i := 0; i < count; i++ {
		vertex := func(index int, height float64) Vertex {
			x, z := point(index)
			nx, nz := x/(w/2), z/(d/2)
			if hex {
				ax, az := point(i)
				bx, bz := point(i + 1)
				nx, nz = (bz-az)*d/2, (ax-bx)*w/2
			}
			return Vertex{Position: nadd(center, nv(x*w/2, height, z*d/2)), Normal: nunit(nv(nx, 0, nz)), U: float64(index) / float64(count), V: 1 - height/h}
		}
		a, c, e, f := vertex(i, 0), vertex(i+1, 0), vertex(i+1, h), vertex(i, h)
		b.triangle(a, f, e, paint.wall, true)
		b.triangle(a, e, c, paint.wall, true)
		cap := func(index int) Vertex {
			x, z := point(index)
			return paint.capVertex(nadd(center, nv(x*w/2, h, z*d/2)), nv(0, 1, 0), (x+1)/2, (z+1)/2)
		}
		mid := paint.capVertex(nadd(center, nv(0, h, 0)), nv(0, 1, 0), .5, .5)
		b.solidCapTriangle(mid, cap(i+1), cap(i), paint)
	}
}

// Queue is a horizontal barrel with real end caps and an integrated flat
// print crown. Cap and wall meet at one sharp rim; the shared edge pass inks it.
func (b *meshBuilder) solidBarrel(center Vec, w, d, h float64, paint nativeSolidPaint, crown ...float64) {
	cut := 1.
	if len(crown) > 0 {
		cut = max(0, min(1, crown[0]))
	}
	for i := 0; i < nativeBarrelSegments; i++ {
		y0, _ := nativeBarrelPoint(i, d, h, cut)
		y1, _ := nativeBarrelPoint(i+1, d, h, cut)
		flat := math.Abs(y0-h) < 1e-12 && math.Abs(y1-h) < 1e-12
		point := func(index int, x float64) Vertex {
			angle := float64(index) * 2 * math.Pi / nativeBarrelSegments
			y, z := nativeBarrelPoint(index, d, h, cut)
			normal := nunit(nv(0, math.Cos(angle)/(h/(1+cut)), math.Sin(angle)/(d/2)))
			if flat {
				normal = nv(0, 1, 0)
			}
			return Vertex{Position: nadd(center, nv(x, y, z)), Normal: normal, U: x/w + .5, V: float64(index) / nativeBarrelSegments}
		}
		a, c, e, f := point(i, -w/2), point(i+1, -w/2), point(i+1, w/2), point(i, w/2)
		b.triangle(a, e, f, paint.wall, true)
		b.triangle(a, c, e, paint.wall, true)
		for _, side := range []float64{-1, 1} {
			cap := func(index int) Vertex {
				angle := float64(index) * 2 * math.Pi / nativeBarrelSegments
				y, z := nativeBarrelPoint(index, d, h, cut)
				return paint.capVertex(nadd(center, nv(side*w/2, y, z)), nv(side, 0, 0), (math.Sin(angle)+1)/2, (1-math.Cos(angle))/2)
			}
			p, q := cap(i), cap(i+1)
			mid := paint.capVertex(nadd(center, nv(side*w/2, h/2, 0)), nv(side, 0, 0), .5, .5)
			if side > 0 {
				b.solidCapTriangle(mid, p, q, paint)
			} else {
				b.solidCapTriangle(mid, q, p, paint)
			}
		}
	}
}
