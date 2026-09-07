package d2isometricimg

import (
	"fmt"
	"math"
)

// A cover paints one continuous material silhouette. Each mesh facet keeps its
// own vector lighting inside the pattern, but antialiasing at internal facet
// edges cannot reveal the page behind it. The exact outer silhouette is filled
// once, so neither source opacity nor the authored outline gets thicker.
type svgPaintCover struct {
	batches  []*svgPaintBatch
	polygons [][]svgPoint
	first    int
	written  bool
}

func svgPreparePaintCovers(batches []*svgPaintBatch) map[*svgPaintBatch]*svgPaintCover {
	type key struct {
		material *Material
		decal    bool
		texture  bool
	}
	byMaterial := make(map[key]*svgPaintCover)
	for _, batch := range batches {
		material := batch.triangle.Material
		if material == nil || len(batch.polygons) == 0 {
			continue
		}
		// Only a full, filled physical surface permits texture overdraw. Decals
		// and artwork with transparent margins must retain their separate clips.
		if batch.texture && (!material.svgSolidTexture || batch.triangle.NoDepthWrite || material.Multiply) {
			continue
		}
		k := key{material, batch.triangle.NoDepthWrite, batch.texture}
		cover := byMaterial[k]
		if cover == nil {
			cover = &svgPaintCover{first: batch.first}
			byMaterial[k] = cover
		}
		cover.batches = append(cover.batches, batch)
		cover.polygons = append(cover.polygons, batch.polygons...)
	}
	result := make(map[*svgPaintBatch]*svgPaintCover)
	for _, cover := range byMaterial {
		if len(cover.batches) < 2 {
			continue
		}
		for _, batch := range cover.batches {
			result[batch] = cover
		}
	}
	return result
}

func writeSVGPaintCover(w *nativeSVGWriter, cover *svgPaintCover, camera rasterCamera) {
	if cover.written || w.err != nil {
		return
	}
	cover.written = true
	var points []svgPoint
	for _, polygon := range cover.polygons {
		points = append(points, polygon...)
	}
	box := svgPolygonBox(points)
	// Keep the tile boundary away from the exact outer silhouette. Otherwise
	// the tile's own antialiasing would be multiplied by the final path edge.
	box.minX, box.minY, box.maxX, box.maxY = box.minX-1, box.minY-1, box.maxX+1, box.maxY+1
	name := fmt.Sprintf("paint-cover-%d", cover.first)
	fills := make([]string, len(cover.batches))
	for i, batch := range cover.batches {
		if batch.texture {
			continue
		}
		fills[i] = batch.color
		if fills[i] == "" {
			// The emitter numbers its regular batches positively. A separate
			// namespace prevents collision with a cover's original mesh indices.
			fills[i] = writeSVGFaceGradient(w, batch, -batch.first-1, camera)
		}
		if w.err != nil {
			return
		}
	}
	w.write(`<defs><pattern id="%s" patternUnits="userSpaceOnUse" x="%s" y="%s" width="%s" height="%s" viewBox="%s %s %s %s" preserveAspectRatio="none">`, name, svgNumber(box.minX), svgNumber(box.minY), svgNumber(box.maxX-box.minX), svgNumber(box.maxY-box.minY), svgNumber(box.minX), svgNumber(box.minY), svgNumber(box.maxX-box.minX), svgNumber(box.maxY-box.minY))
	for i, batch := range cover.batches {
		if batch.texture {
			// Keep the original UV transform and lighting, extending only its
			// internal clip. The final compound path applies silhouette and alpha
			// exactly once, so overlapping fragments cannot create darker seams.
			part := *batch
			material := *part.triangle.Material
			material.Color.A, material.Multiply = 255, false
			part.triangle.Material, part.cover = &material, nil
			part.polygons = make([][]svgPoint, len(batch.polygons))
			for j, polygon := range batch.polygons {
				part.polygons[j] = svgExpandCoverPolygon(polygon, .4)
			}
			writeSVGBatch(w, &part, -batch.first-1, camera)
			continue
		}
		// This is only substrate overdraw inside a single paint field. It is
		// clipped by the original compound silhouette below, never used as ink.
		w.write(`<path d="%s" fill="%s" stroke="%s" stroke-width="0.8" stroke-linejoin="round"/>`, svgPolygonPath(batch.polygons), fills[i], fills[i])
	}
	w.write(`</pattern></defs>`)
	material := cover.batches[0].triangle.Material
	blend := ""
	if material.Multiply {
		blend = ` style="mix-blend-mode:multiply"`
	}
	w.write(`<path d="%s" fill="url(#%s)" fill-opacity="%s"%s/>`, svgPolygonPath(cover.polygons), name, svgNumber(float64(material.Color.A)/255), blend)
}

// Offset a convex visible fragment's supporting edges by a fraction of a
// pixel. Bounding the result to the expanded box avoids long miters at acute
// projected corners. These points are only paint-field clips; they never
// replace the exact visible silhouette or any source geometry.
func svgExpandCoverPolygon(polygon []svgPoint, width float64) []svgPoint {
	if len(polygon) < 3 || width <= 0 {
		return polygon
	}
	box := svgPolygonBox(polygon)
	points := []svgPoint{
		{box.minX - width, box.minY - width, 0},
		{box.maxX + width, box.minY - width, 0},
		{box.maxX + width, box.maxY + width, 0},
		{box.minX - width, box.maxY + width, 0},
	}
	sign := 1.
	if svgPolygonArea(polygon) < 0 {
		sign = -1
	}
	for i, a := range polygon {
		b := polygon[(i+1)%len(polygon)]
		dx, dy := b.x-a.x, b.y-a.y
		length := math.Hypot(dx, dy)
		if length < 1e-12 {
			continue
		}
		points, _ = svgSplitPolygon(points, func(p svgPoint) float64 {
			return sign*(dx*(p.y-a.y)-dy*(p.x-a.x))/length + width
		})
	}
	return points
}
