// Package scanline rasterizes D2's closed vector paths with non-zero winding.
//
// The implementation is deliberately specialized for D2's renderer: paths are
// consumed in device coordinates and painted as a uniform color, written to an
// Alpha image, or exposed row-wise for direct paint compositing. Each directed
// edge contributes its exact signed area to the pixels it crosses; a row-sized
// difference buffer carries winding across interior spans. This avoids both
// supersampling and a canvas-sized accumulation image.
package scanline

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"unsafe"
)

const (
	// Curves are subdivided until their control points are within three-eighths
	// of a device pixel from the chord. At that scale, analytic edge coverage
	// absorbs the remaining geometric error without visible faceting.
	curveTolerance = 0.375
	maxCurveDepth  = 18
	// Long inner loops check cancellation at this power-of-two interval. Row
	// loops also check periodically, so small paths do not pay a per-pixel
	// context lookup.
	contextCheckInterval = 4_096
)

var (
	// ErrEdgeLimit reports that path flattening would exceed the caller's
	// retained edge-storage budget.
	ErrEdgeLimit = errors.New("scanline edge limit exceeded")
	// ErrCurveLimit reports that a curve remains unresolved at the safety depth.
	// Returning an error avoids silently replacing extreme geometry with an
	// inaccurate chord.
	ErrCurveLimit = errors.New("scanline curve subdivision limit exceeded")
	// ErrWorkLimit reports that scan conversion exhausted its explicit operation
	// budget. The budget is shared across rasterizer calls for one frame.
	ErrWorkLimit = errors.New("scanline work limit exceeded")
)

// WorkBudget accounts scan-conversion operations across rasterizer calls. It
// is intentionally not concurrency-safe; one renderer owns it for one frame.
type WorkBudget struct {
	remaining int64
}

// NewWorkBudget returns a budget with limit available operation units.
func NewWorkBudget(limit int64) WorkBudget {
	if limit < 0 {
		limit = 0
	}
	return WorkBudget{remaining: limit}
}

// Remaining reports the unconsumed operation units.
func (b *WorkBudget) Remaining() int64 {
	if b == nil {
		return 0
	}
	return b.remaining
}

func (b *WorkBudget) consume(work int64) error {
	if b == nil || work < 0 || work > b.remaining {
		return ErrWorkLimit
	}
	b.remaining -= work
	return nil
}

type point struct {
	x float64
	y float64
}

type edge struct {
	from point
	to   point
}

type scanEdge struct {
	end          int
	next         int
	x            float64
	y            float64
	bottom       float64
	slope        float64
	inverseSlope float64
	winding      int8
}

// Rasterizer retains reusable path and scanline storage between Reset calls.
// It is not safe for concurrent use.
type Rasterizer struct {
	width  int
	height int

	edges      []edge
	scanEdges  []scanEdge
	active     []int
	rowHeads   []int
	partial    []float32
	difference []float32

	first   point
	current point
	hasPath bool

	touchedMin int
	touchedMax int
	touched    bool

	countOnly bool
	edgeCount int
	edgeLimit int
	pathErr   error

	workVisibleEdges       int64
	workActiveRows         int64
	workHorizontalCrossing int64
	workFirstRow           int
	workLastRow            int
	workTouchedMin         int
	workTouchedMax         int
	workTouched            bool
}

// NewRasterizer returns an empty rasterizer for a width by height target.
func NewRasterizer(width, height int) *Rasterizer {
	r := &Rasterizer{edgeLimit: -1}
	r.Reset(width, height)
	return r
}

// NewCounter returns a rasterizer that performs the same path flattening as a
// drawing rasterizer without allocating edge or scanline storage. At most
// edgeLimit edges are counted before ErrEdgeLimit is reported.
func NewCounter(width, height, edgeLimit int) *Rasterizer {
	r := &Rasterizer{countOnly: true, edgeLimit: max(edgeLimit, 0)}
	r.Reset(width, height)
	return r
}

// Reset clears the current paths while retaining bounded scratch capacity.
func (r *Rasterizer) Reset(width, height int) {
	// Successful scanlines clear their touched range immediately. If drawing was
	// interrupted, clear that same range while the old dimensions still describe
	// the retained buffers; untouched storage is already zero.
	if !r.countOnly {
		r.clearTouched()
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	r.width = width
	r.height = height
	r.edges = r.edges[:0]
	r.scanEdges = r.scanEdges[:0]
	r.active = r.active[:0]
	r.hasPath = false
	r.edgeCount = 0
	r.pathErr = nil
	r.workVisibleEdges = 0
	r.workActiveRows = 0
	r.workHorizontalCrossing = 0
	r.workFirstRow = height
	r.workLastRow = 0
	r.workTouchedMin = width
	r.workTouchedMax = 0
	r.workTouched = false
	if !r.countOnly {
		r.resizeRows(width)
	}
	r.resetTouched()
}

// ReserveEdges bounds path construction and reserves the corresponding
// reusable edge, scan-edge, and active-edge capacities.
func (r *Rasterizer) ReserveEdges(limit int) {
	if limit < 0 {
		limit = 0
	}
	r.edgeLimit = limit
	if r.countOnly {
		return
	}
	if cap(r.edges) < limit {
		r.edges = make([]edge, 0, limit)
	}
	if cap(r.scanEdges) < limit {
		r.scanEdges = make([]scanEdge, 0, limit)
	}
	if cap(r.active) < limit {
		r.active = make([]int, 0, limit)
	}
}

// EdgeCount reports the number of non-horizontal finite edges in the current
// path, capped at the configured limit.
func (r *Rasterizer) EdgeCount() int {
	return r.edgeCount
}

// Err reports any edge-budget or curve-complexity error from path construction.
func (r *Rasterizer) Err() error {
	return r.pathErr
}

func (r *Rasterizer) resizeRows(width int) {
	if cap(r.partial) < width {
		r.partial = make([]float32, width)
	} else {
		r.partial = r.partial[:width]
	}
	if cap(r.difference) < width+1 {
		r.difference = make([]float32, width+1)
	} else {
		r.difference = r.difference[:width+1]
	}
}

// MoveTo starts a new subpath at x, y.
func (r *Rasterizer) MoveTo(x, y float32) {
	p := point{x: float64(x), y: float64(y)}
	r.first = p
	r.current = p
	r.hasPath = true
}

// LineTo appends a straight edge to the current subpath.
func (r *Rasterizer) LineTo(x, y float32) {
	next := point{x: float64(x), y: float64(y)}
	if !r.hasPath {
		r.first = next
		r.current = next
		r.hasPath = true
		return
	}
	r.addEdge(r.current, next)
	r.current = next
}

// CubeTo appends a cubic Bezier curve. Curves are flattened in device space so
// the tolerance is independent of the source transform.
func (r *Rasterizer) CubeTo(cx0, cy0, cx1, cy1, x, y float32) {
	end := point{x: float64(x), y: float64(y)}
	if !r.hasPath {
		r.first = end
		r.current = end
		r.hasPath = true
		return
	}
	c0 := point{x: float64(cx0), y: float64(cy0)}
	c1 := point{x: float64(cx1), y: float64(cy1)}
	r.flattenCube(r.current, c0, c1, end, 0)
	r.current = end
}

// ClosePath closes the current subpath with a straight edge.
func (r *Rasterizer) ClosePath() {
	if !r.hasPath {
		return
	}
	r.addEdge(r.current, r.first)
	r.current = r.first
	r.hasPath = false
}

func (r *Rasterizer) addEdge(from, to point) {
	if r.pathErr != nil {
		return
	}
	if from == to || from.y == to.y {
		return
	}
	if !finitePoint(from) || !finitePoint(to) {
		return
	}
	if r.edgeLimit >= 0 && r.edgeCount >= r.edgeLimit {
		r.pathErr = ErrEdgeLimit
		return
	}
	r.edgeCount++
	if !r.recordEdgeWork(from, to) {
		r.pathErr = ErrWorkLimit
		return
	}
	if r.countOnly {
		return
	}
	r.edges = append(r.edges, edge{from: from, to: to})
}

// recordEdgeWork retains only aggregate geometry metrics. This gives preflight
// and drawing the same conservative work bound without storing attacker-sized
// edge slices in count-only mode.
func (r *Rasterizer) recordEdgeWork(from, to point) bool {
	if from.y > to.y {
		from, to = to, from
	}
	if to.y <= 0 || from.y >= float64(r.height) {
		return true
	}
	start := int(max(from.y, 0))
	end := int(math.Ceil(min(to.y, float64(r.height))))
	if start >= end {
		return true
	}
	rowSpan := int64(end - start)
	visibleEdges, ok := checkedAddInt64(r.workVisibleEdges, 1)
	if !ok {
		return false
	}
	activeRows, ok := checkedAddInt64(r.workActiveRows, rowSpan)
	if !ok {
		return false
	}

	// The complete raw edge contains its vertically clipped portion, so using
	// its endpoints avoids solving the line a second time and remains a safe
	// (occasionally looser) bound for both horizontal travel and touched X.
	clippedFrom := min(max(from.x, 0), float64(r.width))
	clippedTo := min(max(to.x, 0), float64(r.width))
	// Splitting an edge at scanline boundaries can add at most one partial
	// segment per active row beyond the monotone horizontal travel.
	deltaX := clippedTo - clippedFrom
	if deltaX < 0 {
		deltaX = -deltaX
	}
	// Truncation plus one also covers an exact-integer span and is cheaper than
	// a second floating-point ceil in this path-submission hot loop.
	horizontal := int64(deltaX) + 1
	horizontal, ok = checkedAddInt64(horizontal, rowSpan)
	if !ok {
		return false
	}
	horizontal, ok = checkedAddInt64(r.workHorizontalCrossing, horizontal)
	if !ok {
		return false
	}

	edgeMin, edgeMax := 0, 0
	minX, maxX := min(from.x, to.x), max(from.x, to.x)
	switch {
	case maxX <= 0:
		edgeMin, edgeMax = 0, 0
	case minX >= float64(r.width):
		edgeMin, edgeMax = r.width, r.width
	default:
		edgeMin = int(max(minX, 0))
		if maxX >= float64(r.width) {
			edgeMax = r.width
		} else {
			edgeMax = int(maxX) + 1
		}
	}
	if !r.workTouched {
		r.workTouchedMin, r.workTouchedMax = edgeMin, edgeMax
		r.workTouched = true
	} else {
		r.workTouchedMin = min(r.workTouchedMin, edgeMin)
		r.workTouchedMax = max(r.workTouchedMax, edgeMax)
	}
	r.workVisibleEdges = visibleEdges
	r.workActiveRows = activeRows
	r.workHorizontalCrossing = horizontal
	r.workFirstRow = min(r.workFirstRow, start)
	r.workLastRow = max(r.workLastRow, end)
	return true
}

func finitePoint(p point) bool {
	return !math.IsNaN(p.x) && !math.IsNaN(p.y) && !math.IsInf(p.x, 0) && !math.IsInf(p.y, 0)
}

func (r *Rasterizer) flattenCube(p0, p1, p2, p3 point, depth int) {
	if r.pathErr != nil {
		return
	}
	// A cubic stays inside the convex hull of its controls. Curves wholly above
	// or below the target cannot affect coverage and need no subdivision.
	if max(p0.y, p1.y, p2.y, p3.y) <= 0 || min(p0.y, p1.y, p2.y, p3.y) >= float64(r.height) {
		return
	}
	if cubeFlatEnough(p0, p1, p2, p3) {
		r.addEdge(p0, p3)
		return
	}
	if depth >= maxCurveDepth {
		r.pathErr = ErrCurveLimit
		return
	}
	p01 := midpoint(p0, p1)
	p12 := midpoint(p1, p2)
	p23 := midpoint(p2, p3)
	p012 := midpoint(p01, p12)
	p123 := midpoint(p12, p23)
	middle := midpoint(p012, p123)
	r.flattenCube(p0, p01, p012, middle, depth+1)
	r.flattenCube(middle, p123, p23, p3, depth+1)
}

func midpoint(a, b point) point {
	return point{x: a.x + (b.x-a.x)*0.5, y: a.y + (b.y-a.y)*0.5}
}

func cubeFlatEnough(p0, p1, p2, p3 point) bool {
	dx := p3.x - p0.x
	dy := p3.y - p0.y
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		d1x, d1y := p1.x-p0.x, p1.y-p0.y
		d2x, d2y := p2.x-p0.x, p2.y-p0.y
		return math.Max(d1x*d1x+d1y*d1y, d2x*d2x+d2y*d2y) <= curveTolerance*curveTolerance
	}
	limit := curveTolerance * curveTolerance * lengthSquared
	projectionTolerance := curveTolerance * math.Sqrt(lengthSquared)
	previousProjection := 0.0
	for _, control := range [...]point{p1, p2} {
		controlX := control.x - p0.x
		controlY := control.y - p0.y
		projection := controlX*dx + controlY*dy
		cross := dx*controlY - dy*controlX
		excess := 0.0
		if projection < 0 {
			excess = -projection
		} else if projection > lengthSquared {
			excess = projection - lengthSquared
		}
		if cross*cross+excess*excess > limit || projection < previousProjection-projectionTolerance {
			return false
		}
		previousProjection = projection
	}
	return true
}

// DrawRGBA paints the current paths over dst using a uniform, unpremultiplied
// color. Rasterizer coordinates start at dst.Bounds().Min. Work is charged to
// budget and cancellation is checked within long scan loops.
func (r *Rasterizer) DrawRGBA(ctx context.Context, budget *WorkBudget, dst *image.RGBA, paint color.NRGBA) error {
	if err := r.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dst == nil || r.width == 0 || r.height == 0 || len(r.edges) == 0 || paint.A == 0 {
		return nil
	}
	// Sparse paths spend most of their painted rows in event-free interiors,
	// where filling an opaque span is cheaper than per-pixel compositing. Dense
	// compound paths have short spans and favor the general loop.
	if paint.A == 0xff && len(r.edges) <= max(r.height, 8) {
		sr, sg, sb, _ := paint.RGBA()
		packedPaint := uint32(paint.R) | uint32(paint.G)<<8 | uint32(paint.B)<<16 | uint32(0xff)<<24
		return r.rasterize(ctx, budget, func(y, minX, maxX int) error {
			row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
			var winding float32
			for x := minX; x < maxX; {
				end := min(x+contextCheckInterval, maxX)
				if x != minX {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				for x < end {
					winding += r.difference[x]
					pixelCoverage := coverage(r.partial[x] + winding)
					if pixelCoverage != 0 {
						pixel := dst.Pix[row : row+4 : row+4]
						if pixelCoverage == 0xff {
							binary.LittleEndian.PutUint32(pixel, packedPaint)
						} else {
							maskAlpha := uint32(pixelCoverage)
							maskAlpha |= maskAlpha << 8
							inverse := (uint32(0xffff) - maskAlpha) * 0x101
							pixel[0] = uint8((uint32(pixel[0])*inverse + sr*maskAlpha) / 0xffff >> 8)
							pixel[1] = uint8((uint32(pixel[1])*inverse + sg*maskAlpha) / 0xffff >> 8)
							pixel[2] = uint8((uint32(pixel[2])*inverse + sb*maskAlpha) / 0xffff >> 8)
							pixel[3] = uint8((uint32(pixel[3])*inverse + uint32(0xffff)*maskAlpha) / 0xffff >> 8)
						}
					}
					x++
					row += 4

					// Interior spans have no partial area or winding event. Consume
					// each fully opaque or transparent span once instead of repeating
					// coverage work for every pixel.
					if x < end && zeroCoverageValues(r.partial[x], r.difference[x]) {
						spanCoverage := coverage(winding)
						if spanCoverage == 0 || spanCoverage == 0xff {
							runEnd := x + 1
							if end-runEnd >= 8 {
								runEnd = zeroCoverageRunEnd(r.partial, r.difference, runEnd, end)
							} else {
								for runEnd < end && zeroCoverageValues(r.partial[runEnd], r.difference[runEnd]) {
									runEnd++
								}
							}
							byteEnd := row + (runEnd-x)*4
							if spanCoverage == 0xff {
								if runEnd-x >= 8 {
									fillPackedRGBA(dst.Pix[row:byteEnd], packedPaint)
								} else {
									for fill := row; fill < byteEnd; fill += 4 {
										binary.LittleEndian.PutUint32(dst.Pix[fill:fill+4], packedPaint)
									}
								}
							}
							row = byteEnd
							x = runEnd
						}
					}
				}
			}
			return nil
		})
	}
	// Dense opaque paths do not benefit from searching for interior spans, but
	// they still avoid the general source-alpha multiply and replace fully
	// covered pixels directly.
	if paint.A == 0xff {
		sr, sg, sb, _ := paint.RGBA()
		packedPaint := uint32(paint.R) | uint32(paint.G)<<8 | uint32(paint.B)<<16 | uint32(0xff)<<24
		return r.rasterize(ctx, budget, func(y, minX, maxX int) error {
			row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
			var winding float32
			for x := minX; x < maxX; {
				end := min(x+contextCheckInterval, maxX)
				if x != minX {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				for ; x < end; x++ {
					winding += r.difference[x]
					pixelCoverage := coverage(r.partial[x] + winding)
					if pixelCoverage == 0 {
						row += 4
						continue
					}
					pixel := dst.Pix[row : row+4 : row+4]
					if pixelCoverage == 0xff {
						binary.LittleEndian.PutUint32(pixel, packedPaint)
					} else {
						maskAlpha := uint32(pixelCoverage)
						maskAlpha |= maskAlpha << 8
						inverse := (uint32(0xffff) - maskAlpha) * 0x101
						pixel[0] = uint8((uint32(pixel[0])*inverse + sr*maskAlpha) / 0xffff >> 8)
						pixel[1] = uint8((uint32(pixel[1])*inverse + sg*maskAlpha) / 0xffff >> 8)
						pixel[2] = uint8((uint32(pixel[2])*inverse + sb*maskAlpha) / 0xffff >> 8)
						pixel[3] = uint8((uint32(pixel[3])*inverse + uint32(0xffff)*maskAlpha) / 0xffff >> 8)
					}
					row += 4
				}
			}
			return nil
		})
	}
	sr, sg, sb, sa := paint.RGBA()
	if len(r.edges) <= max(r.height, 8) {
		return r.rasterize(ctx, budget, func(y, minX, maxX int) error {
			row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
			var winding float32
			for x := minX; x < maxX; {
				end := min(x+contextCheckInterval, maxX)
				if x != minX {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				for x < end {
					winding += r.difference[x]
					pixelCoverage := coverage(r.partial[x] + winding)
					if pixelCoverage != 0 {
						maskAlpha := uint32(pixelCoverage)
						maskAlpha |= maskAlpha << 8
						inverse := (uint32(0xffff) - sa*maskAlpha/0xffff) * 0x101
						pixel := dst.Pix[row : row+4 : row+4]
						pixel[0] = uint8((uint32(pixel[0])*inverse + sr*maskAlpha) / 0xffff >> 8)
						pixel[1] = uint8((uint32(pixel[1])*inverse + sg*maskAlpha) / 0xffff >> 8)
						pixel[2] = uint8((uint32(pixel[2])*inverse + sb*maskAlpha) / 0xffff >> 8)
						pixel[3] = uint8((uint32(pixel[3])*inverse + sa*maskAlpha) / 0xffff >> 8)
					}
					x++
					row += 4

					// Between winding events, coverage is constant. Skip transparent
					// interiors in chunks; for painted spans, reuse the exact source-over
					// coefficients while checking for the next event.
					if x < end && zeroCoverageValues(r.partial[x], r.difference[x]) {
						spanCoverage := coverage(winding)
						if spanCoverage == 0 {
							runEnd := x + 1
							if end-runEnd >= 8 {
								runEnd = zeroCoverageRunEnd(r.partial, r.difference, runEnd, end)
							} else {
								for runEnd < end && zeroCoverageValues(r.partial[runEnd], r.difference[runEnd]) {
									runEnd++
								}
							}
							row += (runEnd - x) * 4
							x = runEnd
							continue
						}
						maskAlpha := uint32(spanCoverage)
						maskAlpha |= maskAlpha << 8
						inverse := (uint32(0xffff) - sa*maskAlpha/0xffff) * 0x101
						red := sr * maskAlpha
						green := sg * maskAlpha
						blue := sb * maskAlpha
						alpha := sa * maskAlpha
						for x < end && zeroCoverageValues(r.partial[x], r.difference[x]) {
							pixel := dst.Pix[row : row+4 : row+4]
							pixel[0] = uint8((uint32(pixel[0])*inverse + red) / 0xffff >> 8)
							pixel[1] = uint8((uint32(pixel[1])*inverse + green) / 0xffff >> 8)
							pixel[2] = uint8((uint32(pixel[2])*inverse + blue) / 0xffff >> 8)
							pixel[3] = uint8((uint32(pixel[3])*inverse + alpha) / 0xffff >> 8)
							x++
							row += 4
						}
					}
				}
			}
			return nil
		})
	}
	return r.rasterize(ctx, budget, func(y, minX, maxX int) error {
		row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
		var winding float32
		for x := minX; x < maxX; {
			end := min(x+contextCheckInterval, maxX)
			if x != minX {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			for ; x < end; x++ {
				winding += r.difference[x]
				coverage := coverage(r.partial[x] + winding)
				if coverage == 0 {
					row += 4
					continue
				}
				maskAlpha := uint32(coverage)
				maskAlpha |= maskAlpha << 8
				inverse := (uint32(0xffff) - sa*maskAlpha/0xffff) * 0x101
				pixel := dst.Pix[row : row+4 : row+4]
				pixel[0] = uint8((uint32(pixel[0])*inverse + sr*maskAlpha) / 0xffff >> 8)
				pixel[1] = uint8((uint32(pixel[1])*inverse + sg*maskAlpha) / 0xffff >> 8)
				pixel[2] = uint8((uint32(pixel[2])*inverse + sb*maskAlpha) / 0xffff >> 8)
				pixel[3] = uint8((uint32(pixel[3])*inverse + sa*maskAlpha) / 0xffff >> 8)
				row += 4
			}
		}
		return nil
	})
}

func fillPackedRGBA(pixels []byte, packed uint32) {
	if len(pixels) == 0 {
		return
	}
	binary.LittleEndian.PutUint32(pixels[:4], packed)
	for filled := 4; filled < len(pixels); {
		filled += copy(pixels[filled:], pixels[:filled])
	}
}

// WriteAlpha writes opaque path coverage into a freshly zeroed dst. Rasterizer
// coordinates start at dst.Bounds().Min. Work is charged to budget and
// cancellation is checked within long scan loops.
func (r *Rasterizer) WriteAlpha(ctx context.Context, budget *WorkBudget, dst *image.Alpha) error {
	if err := r.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dst == nil || r.width == 0 || r.height == 0 || len(r.edges) == 0 {
		return nil
	}
	// Wide, sparse masks contain enough event-free interior to amortize span
	// discovery. Short rows and dense compound paths favor the scalar loop.
	if r.width >= 256 && len(r.edges) <= max(r.height, 8) {
		return r.rasterize(ctx, budget, func(y, minX, maxX int) error {
			row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
			var winding float32
			for x := minX; x < maxX; {
				end := min(x+contextCheckInterval, maxX)
				if x != minX {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				for x < end {
					winding += r.difference[x]
					dst.Pix[row] = coverage(r.partial[x] + winding)
					x++
					row++
					if x < end && zeroCoverageValues(r.partial[x], r.difference[x]) {
						spanCoverage := coverage(winding)
						if spanCoverage == 0 || spanCoverage == 0xff {
							runEnd := x + 1
							if end-runEnd >= 8 {
								runEnd = zeroCoverageRunEnd(r.partial, r.difference, runEnd, end)
							} else {
								for runEnd < end && zeroCoverageValues(r.partial[runEnd], r.difference[runEnd]) {
									runEnd++
								}
							}
							if spanCoverage == 0xff {
								if runEnd-x >= 8 {
									fillBytes(dst.Pix[row:row+runEnd-x], 0xff)
								} else {
									for offset := row; offset < row+runEnd-x; offset++ {
										dst.Pix[offset] = 0xff
									}
								}
							}
							row += runEnd - x
							x = runEnd
						}
					}
				}
			}
			return nil
		})
	}
	return r.rasterize(ctx, budget, func(y, minX, maxX int) error {
		row := dst.PixOffset(dst.Rect.Min.X+minX, dst.Rect.Min.Y+y)
		var winding float32
		for x := minX; x < maxX; {
			end := min(x+contextCheckInterval, maxX)
			if x != minX {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			for ; x < end; x++ {
				winding += r.difference[x]
				dst.Pix[row] = coverage(r.partial[x] + winding)
				row++
			}
		}
		return nil
	})
}

// WalkCoverage rasterizes the current paths and visits each row segment that
// can contain non-zero coverage. partial and difference are transient views
// into the Rasterizer's reusable accumulation storage, aligned so index zero
// corresponds to minX. The callback must consume them before returning and
// must not retain either slice.
//
// Callers reconstruct each pixel's coverage in increasing index order with:
//
//	winding += difference[index]
//	alpha := QuantizeCoverage(partial[index] + winding)
//
// Rasterizer coordinates start at the caller's target origin. Work is charged
// to budget and cancellation is checked during scan conversion; callbacks that
// perform substantial per-pixel work should also check their context.
func (r *Rasterizer) WalkCoverage(ctx context.Context, budget *WorkBudget, writeRow func(y, minX int, partial, difference []float32) error) error {
	if err := r.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.width == 0 || r.height == 0 || len(r.edges) == 0 {
		return nil
	}
	return r.rasterize(ctx, budget, func(y, minX, maxX int) error {
		return writeRow(y, minX, r.partial[minX:maxX], r.difference[minX:maxX])
	})
}

// QuantizeCoverage converts a signed analytic pixel area into the same 8-bit
// absolute coverage used by DrawRGBA and WriteAlpha.
func QuantizeCoverage(area float32) uint8 {
	return coverage(area)
}

func fillBytes(pixels []byte, value byte) {
	if len(pixels) == 0 {
		return
	}
	pixels[0] = value
	for filled := 1; filled < len(pixels); {
		filled += copy(pixels[filled:], pixels[:filled])
	}
}

var retainedBytesPerEdge = uint64(unsafe.Sizeof(edge{})) + uint64(unsafe.Sizeof(scanEdge{})) + uint64(unsafe.Sizeof(int(0)))

// RetainedBytes returns the backing-storage bytes retained after visiting
// targets up to maxWidth and maxHeight and paths up to maxEdges. Width and
// height are independent maxima because Reset retains both capacities.
func RetainedBytes(maxWidth, maxHeight, maxEdges int) (int64, bool) {
	if maxWidth < 0 || maxHeight < 0 || maxEdges < 0 {
		return 0, false
	}
	if maxWidth == 0 && maxHeight == 0 && maxEdges == 0 {
		return 0, true
	}
	columns, ok := checkedMultiplyUint64(uint64(maxWidth), 2*uint64(unsafe.Sizeof(float32(0))))
	if !ok {
		return 0, false
	}
	columns, ok = checkedAddUint64(columns, uint64(unsafe.Sizeof(float32(0))))
	if !ok {
		return 0, false
	}
	rows, ok := checkedMultiplyUint64(uint64(maxHeight), uint64(unsafe.Sizeof(int(0))))
	if !ok {
		return 0, false
	}
	edges, ok := checkedMultiplyUint64(uint64(maxEdges), retainedBytesPerEdge)
	if !ok {
		return 0, false
	}
	total, ok := checkedAddUint64(columns, rows)
	if ok {
		total, ok = checkedAddUint64(total, edges)
	}
	if !ok || total > math.MaxInt64 {
		return 0, false
	}
	return int64(total), true
}

func checkedMultiplyUint64(left, right uint64) (uint64, bool) {
	if left != 0 && right > ^uint64(0)/left {
		return 0, false
	}
	return left * right, true
}

func checkedAddUint64(left, right uint64) (uint64, bool) {
	if right > ^uint64(0)-left {
		return 0, false
	}
	return left + right, true
}

func checkedMultiplyInt64(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || left != 0 && right > math.MaxInt64/left {
		return 0, false
	}
	return left * right, true
}

func checkedAddInt64(left, right int64) (int64, bool) {
	if left < 0 || right < 0 || right > math.MaxInt64-left {
		return 0, false
	}
	return left + right, true
}

// MaxEdgesForBytes returns the largest edge count whose retained edge arrays
// fit in bytes. It is used to stop count-only flattening before preflight can
// itself exceed the configured offscreen budget.
func MaxEdgesForBytes(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	count := uint64(bytes) / retainedBytesPerEdge
	maxInt := uint64(^uint(0) >> 1)
	if count > maxInt {
		return int(maxInt)
	}
	return int(count)
}

// WorkBound returns the worst-case operation units for rasterizing edges into
// a width by height target. Callers that submitted geometry should prefer the
// Rasterizer method, whose O(1) aggregate metrics preserve the same safety
// proof without multiplying every small path by the complete target bounds.
func WorkBound(width, height, edges int) (int64, bool) {
	if width < 0 || height < 0 || edges < 0 {
		return 0, false
	}
	if width == 0 || height == 0 || edges == 0 {
		return 0, true
	}
	w, h, e := uint64(width), uint64(height), uint64(edges)
	terms := [...]struct {
		coefficient uint64
		left        uint64
		right       uint64
	}{
		{3, h, 1}, // row-head initialization, row visits, difference tails
		{3, e, 1}, // edge preparation, scan-edge creation, and insertion
		{3, e, h}, // removal, accumulation, and per-row edge fragments
		{1, e, w}, // monotone horizontal travel
		{3, w, h}, // paint, partial clear, and difference clear spans
	}
	var total uint64
	for _, term := range terms {
		value, ok := checkedMultiplyUint64(term.coefficient, term.left)
		if ok {
			value, ok = checkedMultiplyUint64(value, term.right)
		}
		if !ok {
			return 0, false
		}
		total, ok = checkedAddUint64(total, value)
		if !ok {
			return 0, false
		}
	}
	if total > math.MaxInt64 {
		return 0, false
	}
	return int64(total), true
}

// WorkBound returns a conservative bound derived from aggregate edge metrics.
// The rasterizer records only counts, clipped row spans, monotone horizontal
// travel, and a touched rectangle, so count-only preflight remains O(1) space.
func (r *Rasterizer) WorkBound() (int64, bool) {
	if r == nil || r.edgeCount == 0 {
		return 0, true
	}
	rows := int64(0)
	if r.workFirstRow < r.workLastRow {
		rows = int64(r.workLastRow - r.workFirstRow)
	}
	touchedWidth := int64(0)
	if r.workTouchedMin < r.workTouchedMax {
		touchedWidth = int64(r.workTouchedMax - r.workTouchedMin)
	}
	painted, ok := checkedMultiplyInt64(touchedWidth, rows)
	if ok {
		painted, ok = checkedMultiplyInt64(painted, 3)
	}
	if !ok {
		return 0, false
	}
	visibleWork, ok := checkedMultiplyInt64(r.workVisibleEdges, 2)
	if !ok {
		return 0, false
	}
	activeWork, ok := checkedMultiplyInt64(r.workActiveRows, 2)
	if !ok {
		return 0, false
	}
	rowWork, ok := checkedMultiplyInt64(rows, 2)
	if !ok {
		return 0, false
	}
	terms := [...]int64{
		int64(r.height),          // row-head initialization
		int64(r.edgeCount),       // raw-edge preparation
		visibleWork,              // scan-edge creation and row insertion
		activeWork,               // expiration and accumulation visits
		r.workHorizontalCrossing, // horizontal travel plus row fragments
		rowWork,                  // row iteration and difference tail
		painted,                  // painted and cleared touched spans
	}
	total := int64(0)
	for _, term := range terms {
		if term < 0 {
			return 0, false
		}
		total, ok = checkedAddInt64(total, term)
		if !ok {
			return 0, false
		}
	}
	return total, true
}

func (r *Rasterizer) rasterize(ctx context.Context, budget *WorkBudget, writeRow func(y, minX, maxX int) error) error {
	work, ok := r.WorkBound()
	if !ok {
		return ErrWorkLimit
	}
	if err := budget.consume(work); err != nil {
		return err
	}
	firstRow, lastRow, err := r.prepareScanEdges(ctx)
	if err != nil {
		return err
	}
	if firstRow >= lastRow {
		return ctx.Err()
	}
	// Consume row heads as the scan advances. This preserves an all-zero idle
	// buffer without clearing the full target height for every small path.
	// Clear any unvisited suffix on cancellation or a destination error.
	nextHeadRow := firstRow
	defer func() {
		r.clearRowHeads(nextHeadRow, lastRow)
	}()

	r.active = r.active[:0]
	for y := firstRow; y < lastRow; y++ {
		if offset := y - firstRow; offset&(31) == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		r.removeExpired(y)
		added := 0
		head := r.rowHeads[y]
		r.rowHeads[y] = 0
		nextHeadRow = y + 1
		for head != 0 {
			index := head - 1
			if added != 0 && added&(contextCheckInterval-1) == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			r.active = append(r.active, index)
			added++
			head = r.scanEdges[index].next
		}
		for first := 0; first < len(r.active); first += contextCheckInterval {
			if first != 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			end := min(first+contextCheckInterval, len(r.active))
			for index := first; index < end; index++ {
				if err := r.accumulateEdgeRow(ctx, &r.scanEdges[r.active[index]], y); err != nil {
					return err
				}
			}
		}
		if r.touchedMin < r.touchedMax {
			if err := writeRow(y, r.touchedMin, r.touchedMax); err != nil {
				return err
			}
		}
		r.clearTouched()
	}
	return ctx.Err()
}

func (r *Rasterizer) prepareScanEdges(ctx context.Context) (int, int, error) {
	r.scanEdges = r.scanEdges[:0]
	if cap(r.rowHeads) < r.height {
		r.rowHeads = make([]int, r.height)
	} else {
		r.rowHeads = r.rowHeads[:r.height]
	}
	firstRow := r.height
	lastRow := 0
	for rawIndex, raw := range r.edges {
		if rawIndex != 0 && rawIndex&(contextCheckInterval-1) == 0 {
			if err := ctx.Err(); err != nil {
				r.clearRowHeads(firstRow, lastRow)
				return 0, 0, err
			}
		}
		from, to := raw.from, raw.to
		winding := int8(1)
		if from.y > to.y {
			from, to = to, from
			winding = -1
		}
		if to.y <= 0 || from.y >= float64(r.height) {
			continue
		}
		start := int(math.Floor(max(from.y, 0)))
		end := int(math.Ceil(min(to.y, float64(r.height))))
		if start >= end {
			continue
		}
		slope := (to.x - from.x) / (to.y - from.y)
		top := max(from.y, float64(start))
		x := math.FMA(top-from.y, slope, from.x)
		inverseSlope := 0.0
		if slope != 0 {
			inverseSlope = 1 / slope
		}
		index := len(r.scanEdges)
		r.scanEdges = append(r.scanEdges, scanEdge{
			end: end, next: r.rowHeads[start],
			x: x, y: top, bottom: to.y, slope: slope,
			inverseSlope: inverseSlope, winding: winding,
		})
		// Zero is the empty-head sentinel; stored indices are one-based.
		r.rowHeads[start] = index + 1
		if start < firstRow {
			firstRow = start
		}
		if end > lastRow {
			lastRow = end
		}
	}
	return firstRow, lastRow, nil
}

func (r *Rasterizer) clearRowHeads(firstRow, lastRow int) {
	if firstRow >= lastRow {
		return
	}
	clear(r.rowHeads[firstRow:lastRow])
}

func (r *Rasterizer) removeExpired(row int) {
	for index := 0; index < len(r.active); {
		if r.scanEdges[r.active[index]].end > row {
			index++
			continue
		}
		last := len(r.active) - 1
		r.active[index] = r.active[last]
		r.active = r.active[:last]
	}
}

func (r *Rasterizer) accumulateEdgeRow(ctx context.Context, active *scanEdge, row int) error {
	bottom := min(active.bottom, float64(row+1))
	height := bottom - active.y
	if height <= 0 {
		return nil
	}
	xTop := active.x
	xBottom := math.FMA(height, active.slope, xTop)
	top := active.y
	active.x = xBottom
	active.y = bottom
	sign := float64(active.winding)
	deltaX := xBottom - xTop
	if deltaX == 0 {
		if xTop < 0 {
			r.addDifferenceEvent(0, sign*height)
		} else if xTop >= float64(r.width) {
			r.addDifferenceEvent(r.width, sign*height)
		} else {
			pixel := int(math.Floor(xTop))
			r.accumulateVisibleEdgePortion(pixel, sign, xTop, xBottom, height)
			r.touch(pixel, pixel+1)
		}
		return nil
	}

	dyPerX := active.inverseSlope
	currentX, currentY := xTop, top
	if deltaX > 0 {
		if currentX < 0 {
			if xBottom <= 0 {
				r.addDifferenceEvent(0, sign*height)
				return nil
			}
			crossY := math.FMA(-currentX, dyPerX, currentY)
			r.addDifferenceEvent(0, sign*(crossY-currentY))
			currentX, currentY = 0, crossY
		}
		if currentX >= float64(r.width) {
			r.addDifferenceEvent(r.width, sign*(bottom-currentY))
			return nil
		}
		visibleEndX := min(xBottom, float64(r.width))
		visibleEndY := math.FMA(visibleEndX-currentX, dyPerX, currentY)
		r.touch(int(math.Floor(currentX)), min(r.width, int(math.Floor(visibleEndX))+1))
		boundary := math.Floor(currentX) + 1
		if boundary < visibleEndX {
			nextY := math.FMA(boundary-currentX, dyPerX, currentY)
			r.accumulateVisibleEdgePortion(int(boundary)-1, sign, currentX, boundary, nextY-currentY)
			currentX, currentY = boundary, nextY
			boundary++
			partial := float32(sign * dyPerX * 0.5)
			difference := float32(sign * dyPerX)
			processed := 0
			for boundary < visibleEndX {
				if processed != 0 && processed&(contextCheckInterval-1) == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				pixel := int(boundary) - 1
				r.partial[pixel] += partial
				r.difference[pixel+1] += difference
				currentX = boundary
				currentY += dyPerX
				boundary++
				processed++
			}
		}
		r.accumulateVisibleEdgePortion(int(math.Floor((currentX+visibleEndX)*0.5)), sign, currentX, visibleEndX, visibleEndY-currentY)
		if visibleEndY < bottom {
			r.addDifferenceEvent(r.width, sign*(bottom-visibleEndY))
		}
		return nil
	}

	if currentX > float64(r.width) {
		if xBottom >= float64(r.width) {
			r.addDifferenceEvent(r.width, sign*height)
			return nil
		}
		crossY := math.FMA(float64(r.width)-currentX, dyPerX, currentY)
		r.addDifferenceEvent(r.width, sign*(crossY-currentY))
		currentX, currentY = float64(r.width), crossY
	}
	if currentX <= 0 {
		r.addDifferenceEvent(0, sign*(bottom-currentY))
		return nil
	}
	visibleEndX := max(xBottom, 0)
	visibleEndY := math.FMA(visibleEndX-currentX, dyPerX, currentY)
	r.touch(int(math.Floor(visibleEndX)), min(r.width, int(math.Floor(currentX))+1))
	boundary := math.Ceil(currentX) - 1
	if boundary > visibleEndX {
		nextY := math.FMA(boundary-currentX, dyPerX, currentY)
		r.accumulateVisibleEdgePortion(int(boundary), sign, currentX, boundary, nextY-currentY)
		currentX, currentY = boundary, nextY
		boundary--
		fullHeight := -dyPerX
		partial := float32(sign * fullHeight * 0.5)
		difference := float32(sign * fullHeight)
		processed := 0
		for boundary > visibleEndX {
			if processed != 0 && processed&(contextCheckInterval-1) == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			pixel := int(boundary)
			r.partial[pixel] += partial
			r.difference[pixel+1] += difference
			currentX = boundary
			currentY += fullHeight
			boundary--
			processed++
		}
	}
	r.accumulateVisibleEdgePortion(int(math.Floor((currentX+visibleEndX)*0.5)), sign, currentX, visibleEndX, visibleEndY-currentY)
	if visibleEndY < bottom {
		r.addDifferenceEvent(0, sign*(bottom-visibleEndY))
	}
	return nil
}

func (r *Rasterizer) accumulateVisibleEdgePortion(pixel int, sign, fromX, toX, height float64) {
	if height <= 0 {
		return
	}
	middleX := (fromX + toX) * 0.5
	r.partial[pixel] += float32(sign * height * (float64(pixel+1) - middleX))
	r.difference[pixel+1] += float32(sign * height)
}

func (r *Rasterizer) addDifferenceEvent(x int, amount float64) {
	if amount == 0 {
		return
	}
	r.difference[x] += float32(amount)
	r.touchEvent(x)
}

func coverage(area float32) uint8 {
	if area < 0 {
		area = -area
	}
	if area <= 0 {
		return 0
	}
	if area >= 1 {
		return 255
	}
	return uint8(area*255 + 0.5)
}

func zeroCoverageValues(partial, difference float32) bool {
	// Ignore the sign bit so negative zero retains ordinary floating-point == 0
	// semantics. Combining the bits avoids two floating-point comparisons in
	// the sparse-span discovery loop.
	return (math.Float32bits(partial)|math.Float32bits(difference))<<1 == 0
}

func zeroCoverageRunEnd(partial, difference []float32, start, end int) int {
	index := start
	for index+4 <= end {
		bits := math.Float32bits(partial[index]) | math.Float32bits(difference[index]) |
			math.Float32bits(partial[index+1]) | math.Float32bits(difference[index+1]) |
			math.Float32bits(partial[index+2]) | math.Float32bits(difference[index+2]) |
			math.Float32bits(partial[index+3]) | math.Float32bits(difference[index+3])
		if bits<<1 != 0 {
			break
		}
		index += 4
	}
	for index < end && zeroCoverageValues(partial[index], difference[index]) {
		index++
	}
	return index
}

func (r *Rasterizer) resetTouched() {
	r.touchedMin = r.width
	r.touchedMax = 0
	r.touched = false
}

func (r *Rasterizer) touch(minX, maxX int) {
	r.touched = true
	if minX < r.touchedMin {
		r.touchedMin = minX
	}
	if maxX > r.touchedMax {
		r.touchedMax = maxX
	}
}

func (r *Rasterizer) touchEvent(x int) {
	r.touched = true
	if x < r.touchedMin {
		r.touchedMin = x
	}
	if x > r.touchedMax {
		r.touchedMax = x
	}
}

func (r *Rasterizer) clearTouched() {
	if !r.touched {
		return
	}
	partialMax := min(r.touchedMax, r.width)
	if r.touchedMin < partialMax {
		clear(r.partial[r.touchedMin:partialMax])
	}
	clear(r.difference[r.touchedMin : min(r.touchedMax, r.width)+1])
	r.resetTouched()
}
