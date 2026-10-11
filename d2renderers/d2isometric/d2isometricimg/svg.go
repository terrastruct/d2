package d2isometricimg

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2isometric"
	"github.com/d2lang/d2/d2renderers/d2scenebuild"
	"github.com/d2lang/d2/d2target"
)

// SVG consumes the same physical scene as PNG. Visibility is resolved in
// projected geometry; neither a framebuffer nor a screen-sized image is used.
func renderNativeSVG(ctx context.Context, diagram *d2target.Diagram, o Options) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx = nativeVectorContext(ctx)
	if o.LinkBudget == nil {
		o.LinkBudget = &d2scenebuild.LinkBudget{MaxRegions: 4096, MaxStringBytes: 1 << 20}
	}
	if err := preflightSVGMetadata(ctx, diagram, *o.LinkBudget); err != nil {
		return nil, err
	}
	scene, err := d2isometric.BuildScene(diagram, &o.Render)
	if err != nil {
		return nil, err
	}
	native, err := newNativeSceneWithOptions(ctx, scene, o.Width, o.Height, o.Assets, o.Fonts, nativeSceneOptions{
		deferRaster: true, vector: true, fitContent: o.FitContent, camera: o.camera,
		outputDensity: sceneOutputDensity(scene, o.Width, o.Height, o.camera), links: o.LinkBudget,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize native SVG: %w", err)
	}
	return nativeSceneSVG(ctx, native)
}

func preflightSVGMetadata(ctx context.Context, diagram *d2target.Diagram, budget d2scenebuild.LinkBudget) error {
	if diagram == nil {
		return fmt.Errorf("native SVG requires a diagram")
	}
	bytes, regions := 0, 0
	check := func(link, tooltip string) error {
		if link == "" && tooltip == "" {
			return ctx.Err()
		}
		regions++
		if regions > budget.MaxRegions {
			return fmt.Errorf("native SVG exceeds link region budget")
		}
		for _, value := range []string{link, tooltip} {
			if len(value) > budget.MaxStringBytes-bytes {
				return fmt.Errorf("native SVG exceeds link text budget")
			}
			bytes += len(value)
			if err := validatePageLinkText(ctx, value); err != nil {
				return err
			}
		}
		return validateNativeLink(link)
	}
	for _, s := range diagram.Shapes {
		if err := check(s.Link, s.Tooltip); err != nil {
			return err
		}
	}
	for _, e := range diagram.Connections {
		if err := check(e.Link, e.Tooltip); err != nil {
			return err
		}
	}
	return ctx.Err()
}

type nativeSVGWriter struct {
	ctx      context.Context
	buf      bytes.Buffer
	err      error
	surfaces map[*nativeVectorSurface]string
}

func (w *nativeSVGWriter) write(format string, args ...any) {
	if w.err != nil {
		return
	}
	if w.err = w.ctx.Err(); w.err != nil {
		return
	}
	fmt.Fprintf(&w.buf, format, args...)
	if w.buf.Len() > MaxOutputBytes {
		w.err = fmt.Errorf("native SVG exceeds %d output bytes", MaxOutputBytes)
	}
}

func svgNumber(v float64) string {
	if math.Abs(v) < .0000005 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(v, 'f', 6, 64), "0"), ".")
}

func svgColor(r, g, b float64) string {
	return fmt.Sprintf("#%02x%02x%02x", uint8(math.Round(rasterClamp(r)*255)), uint8(math.Round(rasterClamp(g)*255)), uint8(math.Round(rasterClamp(b)*255)))
}

func svgPolygonPath(polygons [][]svgPoint) string {
	var s strings.Builder
	for _, polygon := range polygons {
		// Serialize a canonical loop. Clipping can retain a different starting
		// vertex at an exactly shared edge, without changing its silhouette.
		// Discard vertices that coincide at SVG precision before choosing it.
		p := make([]svgPoint, 0, len(polygon))
		for _, v := range polygon {
			v = svgPoint{x: math.Round(v.x*1e6) / 1e6, y: math.Round(v.y*1e6) / 1e6}
			if len(p) == 0 || v != p[len(p)-1] {
				p = append(p, v)
			}
		}
		if len(p) > 1 && p[0] == p[len(p)-1] {
			p = p[:len(p)-1]
		}
		p = svgOmitCollinearSplits(p)
		if len(p) < 3 {
			continue
		}
		area := 2 * svgPolygonArea(p)
		if math.Abs(area) < 1e-10 {
			continue
		}
		// A nonzero compound path is a union, including overlapping pieces.
		first := 0
		for i := range p {
			if p[i].x < p[first].x || p[i].x == p[first].x && p[i].y < p[first].y {
				first = i
			}
		}
		for i := range p {
			j := (first + i) % len(p)
			if area < 0 {
				j = (first + len(p) - i) % len(p)
			}
			if i == 0 {
				s.WriteByte('M')
			} else {
				s.WriteByte('L')
			}
			s.WriteString(svgNumber(p[j].x))
			s.WriteByte(' ')
			s.WriteString(svgNumber(p[j].y))
		}
		s.WriteByte('Z')
	}
	return s.String()
}

// Clipping nearly coincident planes can place a redundant intersection at
// different positions along the same straight edge on different CPUs. Drop
// such splits only after projection, at the SVG's six-decimal output precision;
// the visibility mesh and its depth values remain unchanged. Work in place on
// the serializer's private, rounded copy, retaining at least three vertices.
func svgOmitCollinearSplits(points []svgPoint) []svgPoint {
	if len(points) <= 3 {
		return points
	}
	redundant := func(a, b, c svgPoint) bool {
		dx, dy := c.x-a.x, c.y-a.y
		px, py := b.x-a.x, b.y-a.y
		length2 := float64(dx*dx) + float64(dy*dy)
		along := float64(px*dx) + float64(py*dy)
		if length2 == 0 || along < 0 || along > length2 {
			return false
		}
		cross := float64(px*dy) - float64(py*dx)
		return math.Abs(cross) <= 1e-6*math.Sqrt(length2)
	}
	clean := points[:0]
	for i, p := range points {
		for len(clean) > 1 && len(clean)+len(points)-i > 3 && redundant(clean[len(clean)-2], clean[len(clean)-1], p) {
			clean = clean[:len(clean)-1]
		}
		clean = append(clean, p)
	}
	for len(clean) > 3 {
		if redundant(clean[len(clean)-1], clean[0], clean[1]) {
			clean = clean[1:]
		} else if redundant(clean[len(clean)-2], clean[len(clean)-1], clean[0]) {
			clean = clean[:len(clean)-1]
		} else {
			break
		}
	}
	return clean
}

type svgPaintBatch struct {
	cover    *svgPaintCover
	triangle Triangle
	polygons [][]svgPoint
	depth    float64
	first    int
	affine   [6]float64
	texture  bool
	color    string
	gradient [3]Vec
	points   [3]svgPoint
}

type svgPaintUnit struct {
	batches []*svgPaintBatch
	depth   float64
	count   int
	first   int
	opacity float64
	opaque  bool
}

type svgBatchKey struct {
	material *Material
	affine   [6]int64
	color    string
	unique   int
}

func nativeSceneSVG(ctx context.Context, scene *nativeScene) ([]byte, error) {
	if scene == nil {
		return nil, fmt.Errorf("native SVG requires a scene")
	}
	if err := rasterValidate(ctx, scene.triangles); err != nil {
		return nil, err
	}
	camera := scene.camera
	faces := make([]svgVisibilityFace, len(scene.triangles))
	for i, t := range scene.triangles {
		f := &faces[i]
		f.order, f.group = i, t.OpacityGroup
		f.owner = t.PaintOwner
		m := t.Material
		f.contour = m != nil && m.svgContour
		paint := rasterMaterial(m)
		if paint.alpha == 0 {
			continue
		}
		if paint.alpha < .999 || paint.multiply {
			n := nadd(nadd(t.V[0].Normal, t.V[1].Normal), t.V[2].Normal)
			if ndot(n, n) > 1e-20 && ndot(n, camera.direction) <= 0 {
				continue
			}
		}
		f.opaque = !t.NoDepthWrite && paint.alpha == 1 && (paint.texture == nil || m.svgSolidTexture) && !paint.multiply
		for _, v := range t.V {
			p := camera.project(v.Position)
			f.points = append(f.points, svgProjectedPoint(p.x, p.y, p.z+t.DepthBias))
		}
	}
	visible, err := svgVisibleFaces(ctx, faces)
	if err != nil {
		return nil, err
	}
	type unitKey struct {
		group    *nativeOpacityGroup
		owner    *nativePaintOwner
		material *Material
	}
	units := []*svgPaintUnit{}
	unitIndex := make(map[unitKey]int)
	batchIndexes := []map[svgBatchKey]*svgPaintBatch{}
	shade := &Raster{camera: camera}
	for i, t := range scene.triangles {
		if t.svgCoverageOnly || len(visible[i]) == 0 || t.Material != nil && t.Material.Color.A == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		m := t.Material
		if m == nil {
			m = &Material{Color: color.NRGBA{240, 243, 249, 255}, Roughness: .35}
			t.Material = m
		}
		key := unitKey{group: t.OpacityGroup}
		if key.group == nil {
			key.owner = t.PaintOwner
		}
		if key.group == nil && key.owner == nil {
			key.material = m
		}
		unit, exists := unitIndex[key]
		if !exists {
			unit = len(units)
			unitIndex[key] = unit
			u := &svgPaintUnit{first: i, opacity: 1, opaque: faces[i].opaque}
			if key.owner != nil {
				u.opaque = key.owner.Opaque
			}
			if key.group != nil {
				u.opacity, u.opaque = key.group.Opacity, false
			}
			units = append(units, u)
			batchIndexes = append(batchIndexes, make(map[svgBatchKey]*svgPaintBatch))
		}
		u := units[unit]
		depth := 0.
		for _, p := range faces[i].points {
			depth += p.z / 3
		}
		u.depth, u.count = u.depth+depth, u.count+1
		batch := &svgPaintBatch{triangle: t, polygons: visible[i], depth: depth, first: i}
		copy(batch.points[:], faces[i].points)
		bk := svgBatchKey{material: m}
		if m.Texture != nil {
			if m.Vector == nil {
				return nil, fmt.Errorf("native SVG surface %d has no retained vector source", i)
			}
			batch.texture = true
			batch.affine, err = svgTextureAffine(t, batch.points)
			if err != nil {
				return nil, err
			}
			for j, v := range batch.affine {
				bk.affine[j] = int64(math.Round(v * 1e8))
			}
		} else {
			flat := true
			normal := nunit(ncross(nsub(t.V[1].Position, t.V[0].Position), nsub(t.V[2].Position, t.V[0].Position)))
			var firstNormal Vec
			for j, v := range t.V {
				n := v.Normal
				if nlen(n) < 1e-12 {
					n = normal
				}
				r, g, b := shade.shade(rasterMaterial(m), v.Position, n)
				batch.gradient[j] = nv(r, g, b)
				if j == 0 {
					firstNormal = nunit(n)
				} else if !m.Unlit && ndot(firstNormal, nunit(n)) < 1-1e-10 {
					flat = false
				}
				if j > 0 && nlen(nsub(batch.gradient[0], batch.gradient[j])) > 1./510 {
					flat = false
				}
			}
			if flat {
				c := batch.gradient[0]
				batch.color = svgColor(c.X, c.Y, c.Z)
				bk.color = batch.color
			} else {
				bk.unique = i + 1
			}
		}
		if prior := batchIndexes[unit][bk]; prior != nil {
			prior.polygons = append(prior.polygons, batch.polygons...)
		} else {
			u.batches = append(u.batches, batch)
			batchIndexes[unit][bk] = batch
		}
	}
	for _, u := range units {
		u.depth /= float64(u.count)
		sort.SliceStable(u.batches, func(i, j int) bool {
			a, b := u.batches[i], u.batches[j]
			pa, pb := svgIsDecal(a.triangle), svgIsDecal(b.triangle)
			if pa != pb {
				return !pa
			}
			if pa {
				return a.first < b.first
			}
			return a.depth < b.depth
		})
		for batch, cover := range svgPreparePaintCovers(u.batches) {
			batch.cover = cover
		}
	}
	if err := svgOrderPaintUnits(ctx, units); err != nil {
		return nil, err
	}

	w := &nativeSVGWriter{ctx: ctx}
	w.write(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="%d" height="%d" viewBox="0 0 %d %d" role="img"><title>D2 isometric diagram</title>`, scene.width, scene.height, scene.width, scene.height)
	w.write(`<rect width="100%%" height="100%%" fill="%s"/>`, svgColor(float64(scene.background.R)/255, float64(scene.background.G)/255, float64(scene.background.B)/255))
	if scene.backgroundPaint != nil {
		document := nativePaintDocument(scene.backgroundPaint)
		fragment, err := nativeSurfaceSVG(ctx, &nativeVectorSurface{document: document}, "background")
		if err != nil {
			return nil, err
		}
		w.write(`<g transform="scale(%d %d)">%s</g>`, scene.width, scene.height, fragment)
	}
	writeSVGGroundShadow(w, scene)
	id := 0
	for _, u := range units {
		w.write(`<g opacity="%s">`, svgNumber(u.opacity))
		for _, b := range u.batches {
			id++
			writeSVGBatch(w, b, id, camera)
			if w.err != nil {
				return nil, w.err
			}
		}
		w.write(`</g>`)
	}
	writeSVGLinks(w, scene)
	w.write(`</svg>`)
	if w.err != nil {
		return nil, w.err
	}
	return w.buf.Bytes(), nil
}

func svgIsDecal(t Triangle) bool {
	return t.NoDepthWrite || t.Material != nil && (t.Material.svgContour || t.Material.Texture != nil || t.Material.Multiply || t.Material.Color.A < 255)
}

func svgTextureAffine(t Triangle, p [3]svgPoint) ([6]float64, error) {
	u, v := t.V[1].U-t.V[0].U, t.V[1].V-t.V[0].V
	x, y := t.V[2].U-t.V[0].U, t.V[2].V-t.V[0].V
	det := float64(u*y) - float64(x*v)
	if math.Abs(det) < 1e-15 {
		return [6]float64{}, fmt.Errorf("native SVG texture has a degenerate UV mapping")
	}
	dx, dy, ex, ey := p[1].x-p[0].x, p[1].y-p[0].y, p[2].x-p[0].x, p[2].y-p[0].y
	a, b, c, d := (float64(dx*y)-float64(ex*v))/det, (float64(dy*y)-float64(ey*v))/det, (float64(u*ex)-float64(x*dx))/det, (float64(u*ey)-float64(x*dy))/det
	return [6]float64{a, b, c, d, p[0].x - float64(a*t.V[0].U) - float64(c*t.V[0].V), p[0].y - float64(b*t.V[0].U) - float64(d*t.V[0].V)}, nil
}

func writeSVGBatch(w *nativeSVGWriter, b *svgPaintBatch, id int, camera rasterCamera) {
	if b.cover != nil {
		writeSVGPaintCover(w, b.cover, camera)
		return
	}
	path := svgPolygonPath(b.polygons)
	if path == "" {
		return
	}
	m := b.triangle.Material
	alpha := float64(m.Color.A) / 255
	blend := ""
	if m.Multiply {
		blend = ` style="mix-blend-mode:multiply"`
	}
	if b.texture {
		surfaceID := w.surfaceDefinition(m.Vector)
		if w.err != nil {
			return
		}
		w.write(`<defs><clipPath id="face-%d" clipPathUnits="userSpaceOnUse"><path d="%s"/></clipPath></defs>`, id, path)
		filter := ""
		if !m.Unlit {
			slope, intercept := svgTextureLighting(camera, b.triangle)
			w.write(`<defs><filter id="light-%d" color-interpolation-filters="linearRGB"><feComponentTransfer><feFuncR type="linear" slope="%s" intercept="%s"/><feFuncG type="linear" slope="%s" intercept="%s"/><feFuncB type="linear" slope="%s" intercept="%s"/></feComponentTransfer></filter></defs>`, id, svgNumber(slope[0]), svgNumber(intercept[0]), svgNumber(slope[1]), svgNumber(intercept[1]), svgNumber(slope[2]), svgNumber(intercept[2]))
			filter = fmt.Sprintf(` filter="url(#light-%d)"`, id)
		}
		a := b.affine
		w.write(`<g clip-path="url(#face-%d)" opacity="%s"%s><g transform="matrix(%s %s %s %s %s %s)"%s><use xlink:href="#%s"/></g></g>`, id, svgNumber(alpha), blend, svgNumber(a[0]), svgNumber(a[1]), svgNumber(a[2]), svgNumber(a[3]), svgNumber(a[4]), svgNumber(a[5]), filter, surfaceID)
		return
	}
	fill := b.color
	if fill == "" {
		fill = writeSVGFaceGradient(w, b, id, camera)
	}
	w.write(`<path d="%s" fill="%s" fill-opacity="%s"%s/>`, path, fill, svgNumber(alpha), blend)
}

func writeSVGLinks(w *nativeSVGWriter, s *nativeScene) {
	type projectedLink struct {
		link   nativeLink
		points []svgPoint
		area   float64
	}
	links := make([]projectedLink, len(s.links))
	for i, link := range s.links {
		projected := &links[i]
		projected.link = link
		for _, v := range link.points {
			p := s.camera.project(v)
			projected.points = append(projected.points, svgProjectedPoint(p.x, p.y, p.z))
		}
		// Hit regions use the same canonical projection and rounded area
		// arithmetic as physical faces. Tiny FMA residuals must not reorder
		// equal-size links, changing which overlapping target wins a click.
		projected.area = math.Abs(svgPolygonArea(projected.points))
	}
	sort.SliceStable(links, func(i, j int) bool { return links[i].area > links[j].area })
	for _, projected := range links {
		link, points := projected.link, projected.points
		destination := link.region.URL
		if destination == "" && link.region.Target != "" {
			destination = "#" + link.region.Target
		}
		w.write(`<a xlink:href="%s">`, html.EscapeString(destination))
		if link.region.Tooltip != "" {
			w.write(`<title>%s</title>`, html.EscapeString(link.region.Tooltip))
		}
		w.write(`<path d="%s" fill="transparent" pointer-events="all"/></a>`, svgPolygonPath([][]svgPoint{points}))
	}
}
