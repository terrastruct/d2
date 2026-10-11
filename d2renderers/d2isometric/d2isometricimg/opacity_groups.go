package d2isometricimg

import (
	"image"
	"math"
	"sort"
)

// Identity is shared by all triangles of one object and immutable after mesh
// construction. Materials retain their authored paint alpha; object opacity is
// applied once to the composed object, including its individual text decals.
type nativeOpacityGroup struct {
	Opacity float64
}

// A paint owner is ordering metadata, not another opacity effect. The bool
// also gives each allocation a distinct address for stable shared identity.
type nativePaintOwner struct {
	Opaque bool
}

func (b *meshBuilder) markPaintOwner(first int) {
	if b.err != nil || first >= len(b.triangles) {
		return
	}
	var owner *nativePaintOwner
	opaque, partial := false, false
	for _, t := range b.triangles[first:] {
		if t.OpacityGroup != nil {
			return // This complete object already owns its opacity composition.
		}
		if t.PaintOwner != nil {
			owner = t.PaintOwner
		}
		// Physical sidewalls retain source fill alpha independently of cap
		// texture margins or printed text. Transparent symbols without a solid
		// body remain in the depth-ordered transparent pass.
		if t.Material != nil && !t.Material.Unlit && t.Material.Texture == nil && t.Material.Color.A > 0 {
			opaque = opaque || t.Material.Color.A == 255
			partial = partial || t.Material.Color.A < 255
		}
	}
	if owner == nil {
		owner = &nativePaintOwner{Opaque: opaque && !partial}
	}
	for i := first; i < len(b.triangles); i++ {
		b.triangles[i].PaintOwner = owner
	}
}

type rasterOpacityPlan struct {
	plan       *rasterDrawPlan
	bounds     image.Rectangle
	opacity    float64
	depth      float64
	opaque     bool
	writeDepth bool
}

func (r *Raster) rasterPrepareOpacityGroups(ts []Triangle) rasterDrawPlan {
	plan := rasterDrawPlan{triangles: ts, paints: make(map[*Material]rasterPaint), groups: make(map[int]rasterOpacityPlan)}
	type ownerKey struct {
		fade  *nativeOpacityGroup
		paint *nativePaintOwner
	}
	first := make(map[ownerKey]int)
	members := make(map[int][]Triangle)
	for i, t := range ts {
		if _, ok := plan.paints[t.Material]; !ok {
			plan.paints[t.Material] = rasterMaterial(t.Material)
		}
		if t.OpacityGroup == nil && t.PaintOwner == nil {
			plan.order = append(plan.order, i)
			continue
		}
		key := ownerKey{fade: t.OpacityGroup}
		if key.fade == nil {
			key.paint = t.PaintOwner
		}
		index, ok := first[key]
		if !ok {
			index = i
			first[key] = i
			plan.order = append(plan.order, i)
		}
		group := plan.groups[index]
		if t.OpacityGroup != nil {
			group.opacity = t.OpacityGroup.Opacity
		} else {
			group.opacity, group.opaque, group.writeDepth = 1, t.PaintOwner.Opaque, true
		}
		var points [3]rasterPoint
		for j, v := range t.V {
			points[j] = r.camera.project(v.Position)
			group.depth += rasterDot(v.Position, r.camera.direction)
		}
		x0, y0, x1, y1 := rasterBounds(points, r.camera.width, r.camera.height)
		group.bounds = group.bounds.Union(image.Rect(x0, y0, x1, y1))
		plan.groups[index] = group
		t.OpacityGroup, t.PaintOwner = nil, nil
		members[index] = append(members[index], t)
	}
	for index, triangles := range members {
		group := plan.groups[index]
		inside := r.rasterPrepare(triangles)
		group.plan = &inside
		// Match the ordinary comparator's sum of three vertex depths.
		group.depth /= float64(len(triangles))
		plan.groups[index] = group
	}
	transparent := func(index int) bool {
		if group, grouped := plan.groups[index]; grouped {
			return !group.opaque
		}
		t := ts[index]
		paint := plan.paints[t.Material]
		return t.NoDepthWrite || paint.multiply || paint.alpha < .999 || paint.texture != nil
	}
	depth := func(index int) float64 {
		if group, grouped := plan.groups[index]; grouped {
			return group.depth
		}
		z := 0.
		for _, vertex := range ts[index].V {
			z += rasterDot(vertex.Position, r.camera.direction)
		}
		return z
	}
	sort.SliceStable(plan.order, func(i, j int) bool {
		a, b := plan.order[i], plan.order[j]
		// Native paint owners keep their substrate and decals as one unit.
		// Opaque units resolve depth first; remaining units and unowned alpha
		// primitives share one back-to-front order without pairwise exceptions.
		if transparent(a) != transparent(b) {
			return !transparent(a)
		}
		return transparent(a) && depth(a) < depth(b)
	})
	return plan
}

// Composite into a copy of the existing backdrop, then interpolate the result
// once. This is algebraically equivalent to an isolated premultiplied group,
// and reuses the existing physical shading and depth rules for its inner mesh.
// Scratch is shared across objects and never exceeds one supersampled strip.
func (r *Raster) drawOpacityGroup(w *rasterWork, dst *image.RGBA, depth []float32, group rasterOpacityPlan, dirty []bool) error {
	if group.opacity <= 0 || group.plan == nil {
		return nil
	}
	box := group.bounds.Intersect(dst.Rect)
	if box.Empty() {
		return nil
	}
	for y0 := box.Min.Y; y0 < box.Max.Y; y0 += rasterStripRows * r.aa {
		y1 := min(box.Max.Y, y0+rasterStripRows*r.aa)
		bounds := image.Rect(box.Min.X, y0, box.Max.X, y1)
		samples := bounds.Dx() * bounds.Dy()
		copies := int64(3)
		if group.writeDepth {
			copies++
		}
		if err := w.charge(int64(samples) * copies); err != nil {
			return err
		}
		if cap(w.groupPixels) < samples*4 {
			w.groupPixels = make([]uint8, samples*4)
		}
		if cap(w.groupDepth) < samples {
			w.groupDepth = make([]float32, samples)
		}
		pixels := &image.RGBA{Pix: w.groupPixels[:samples*4], Stride: bounds.Dx() * 4, Rect: bounds}
		localDepth := w.groupDepth[:samples]
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			at, start := pixels.PixOffset(bounds.Min.X, y), dst.PixOffset(bounds.Min.X, y)
			copy(pixels.Pix[at:at+bounds.Dx()*4], dst.Pix[start:start+bounds.Dx()*4])
			local := (y - bounds.Min.Y) * bounds.Dx()
			global := (y-dst.Rect.Min.Y)*dst.Rect.Dx() + bounds.Min.X - dst.Rect.Min.X
			copy(localDepth[local:local+bounds.Dx()], depth[global:global+bounds.Dx()])
		}
		if err := r.drawPrepared(w, pixels, localDepth, *group.plan, nil); err != nil {
			return err
		}
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				from, to := pixels.PixOffset(x, y), dst.PixOffset(x, y)
				changed := false
				for channel := 0; channel < 4; channel++ {
					before := dst.Pix[to+channel]
					after := uint8(math.Round(float64(pixels.Pix[from+channel])*group.opacity + float64(before)*(1-group.opacity)))
					dst.Pix[to+channel] = after
					changed = changed || before != after
				}
				if dirty != nil && changed {
					dirty[(y/r.aa)*r.width+x/r.aa] = true
				}
			}
			if group.writeDepth {
				local := (y - bounds.Min.Y) * bounds.Dx()
				global := (y-dst.Rect.Min.Y)*dst.Rect.Dx() + bounds.Min.X - dst.Rect.Min.X
				copy(depth[global:global+bounds.Dx()], localDepth[local:local+bounds.Dx()])
			}
		}
	}
	return nil
}
