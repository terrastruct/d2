package d2raster

import (
	"context"
	"fmt"
	"image"
	"math"

	"github.com/d2lang/d2/d2renderers/d2raster/internal/scanline"
	"github.com/d2lang/d2/d2renderers/d2scene"
)

const (
	maxCurveErrorPixels = 0.25
	maxCurveDepth       = 24
	geometryEpsilon     = 1e-12
)

type subpath struct {
	points []d2scene.Point
	closed bool
}

type strokeRun struct {
	points []d2scene.Point
	closed bool
}

func flattenTolerance(transform d2scene.Matrix) float64 {
	maxCoefficient := math.Max(
		math.Max(math.Abs(transform.A), math.Abs(transform.B)),
		math.Max(math.Abs(transform.C), math.Abs(transform.D)),
	)
	if !finite(maxCoefficient) {
		return math.SmallestNonzeroFloat64
	}
	if maxCoefficient == 0 {
		return math.MaxFloat64
	}
	// Normalize before calculating the exact largest singular value. Divide in
	// two stages so both enormous and subnormal finite scales retain a
	// meaningful reciprocal tolerance without changing ordinary-scale output.
	factor := (d2scene.Matrix{
		A: transform.A / maxCoefficient,
		B: transform.B / maxCoefficient,
		C: transform.C / maxCoefficient,
		D: transform.D / maxCoefficient,
	}).MaxScale()
	normalizedTolerance := maxCurveErrorPixels / factor
	if maxCoefficient < normalizedTolerance/math.MaxFloat64 {
		return math.MaxFloat64
	}
	tolerance := normalizedTolerance / maxCoefficient
	if tolerance == 0 {
		return math.SmallestNonzeroFloat64
	}
	return tolerance
}

func flattenScenePath(ctx context.Context, path d2scene.Path, transform d2scene.Matrix, count func() error) ([]subpath, error) {
	tolerance := flattenTolerance(transform)
	var result []subpath
	var current subpath
	var cursor, start d2scene.Point
	haveCursor := false

	flush := func(closed bool) {
		if len(current.points) != 0 {
			current.closed = closed
			result = append(result, current)
		}
		current = subpath{}
	}
	startAt := func(point d2scene.Point) error {
		if !finitePoint(point) {
			return fmt.Errorf("non-finite point")
		}
		if err := count(); err != nil {
			return err
		}
		current.points = append(current.points, point)
		cursor, start, haveCursor = point, point, true
		return nil
	}
	appendPoint := func(point d2scene.Point) error {
		if !finitePoint(point) {
			return fmt.Errorf("non-finite point")
		}
		if err := count(); err != nil {
			return err
		}
		if len(current.points) == 0 || !samePoint(current.points[len(current.points)-1], point) {
			current.points = append(current.points, point)
		}
		return nil
	}
	ensureCurrent := func() {
		if len(current.points) == 0 {
			current.points = append(current.points, cursor)
		}
	}

	for i, command := range path.Commands {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch command.Kind {
		case d2scene.MoveCommand:
			flush(false)
			if err := startAt(command.P1); err != nil {
				return nil, fmt.Errorf("command %d: %w", i, err)
			}
		case d2scene.LineCommand:
			if !haveCursor {
				return nil, fmt.Errorf("command %d: line before move", i)
			}
			ensureCurrent()
			if err := appendPoint(command.P1); err != nil {
				return nil, fmt.Errorf("command %d: %w", i, err)
			}
			cursor = command.P1
		case d2scene.QuadraticCommand:
			if !haveCursor {
				return nil, fmt.Errorf("command %d: quadratic before move", i)
			}
			if !finitePoint(command.P1) || !finitePoint(command.P2) {
				return nil, fmt.Errorf("command %d: non-finite point", i)
			}
			ensureCurrent()
			if err := flattenQuadratic(ctx, cursor, command.P1, command.P2, tolerance, 0, appendPoint); err != nil {
				return nil, fmt.Errorf("command %d: %w", i, err)
			}
			cursor = command.P2
		case d2scene.CubicCommand:
			if !haveCursor {
				return nil, fmt.Errorf("command %d: cubic before move", i)
			}
			if !finitePoint(command.P1) || !finitePoint(command.P2) || !finitePoint(command.P3) {
				return nil, fmt.Errorf("command %d: non-finite point", i)
			}
			ensureCurrent()
			if err := flattenCubic(ctx, cursor, command.P1, command.P2, command.P3, tolerance, 0, appendPoint); err != nil {
				return nil, fmt.Errorf("command %d: %w", i, err)
			}
			cursor = command.P3
		case d2scene.ArcCommand:
			if !haveCursor {
				return nil, fmt.Errorf("command %d: arc before move", i)
			}
			if !finitePoint(command.P1) || !finite(command.RadiusX) || !finite(command.RadiusY) || !finite(command.Rotation) {
				return nil, fmt.Errorf("command %d: non-finite arc", i)
			}
			ensureCurrent()
			if err := flattenArc(ctx, cursor, command, transform, maxCurveErrorPixels, appendPoint); err != nil {
				return nil, fmt.Errorf("command %d: %w", i, err)
			}
			cursor = command.P1
		case d2scene.CloseCommand:
			if !haveCursor {
				return nil, fmt.Errorf("command %d: close before move", i)
			}
			if err := count(); err != nil {
				return nil, fmt.Errorf("command %d: %w", i, err)
			}
			flush(true)
			cursor = start
			haveCursor = true
		default:
			return nil, fmt.Errorf("command %d: unknown kind %d", i, command.Kind)
		}
	}
	flush(false)
	return result, nil
}

func roundedRectSubpaths(ctx context.Context, rect d2scene.Rect, tolerance float64, count func() error) ([]subpath, error) {
	b := rect.Box
	if b.Width == 0 || b.Height == 0 {
		return nil, nil
	}
	rx := math.Min(rect.RadiusX, b.Width/2)
	ry := math.Min(rect.RadiusY, b.Height/2)
	if rx == 0 || ry == 0 {
		points := []d2scene.Point{
			{X: b.X, Y: b.Y},
			{X: b.X + b.Width, Y: b.Y},
			{X: b.X + b.Width, Y: b.Y + b.Height},
			{X: b.X, Y: b.Y + b.Height},
		}
		for range points {
			if err := count(); err != nil {
				return nil, err
			}
		}
		return []subpath{{points: points, closed: true}}, nil
	}

	var points []d2scene.Point
	appendPoint := func(point d2scene.Point) error {
		if err := count(); err != nil {
			return err
		}
		if len(points) == 0 || !samePoint(points[len(points)-1], point) {
			points = append(points, point)
		}
		return nil
	}
	if err := appendPoint(d2scene.Point{X: b.X + rx, Y: b.Y}); err != nil {
		return nil, err
	}
	if err := appendPoint(d2scene.Point{X: b.X + b.Width - rx, Y: b.Y}); err != nil {
		return nil, err
	}
	k := 0.5522847498307936
	curves := [][4]d2scene.Point{
		{{X: b.X + b.Width - rx, Y: b.Y}, {X: b.X + b.Width - rx + k*rx, Y: b.Y}, {X: b.X + b.Width, Y: b.Y + ry - k*ry}, {X: b.X + b.Width, Y: b.Y + ry}},
		{{X: b.X + b.Width, Y: b.Y + b.Height - ry}, {X: b.X + b.Width, Y: b.Y + b.Height - ry + k*ry}, {X: b.X + b.Width - rx + k*rx, Y: b.Y + b.Height}, {X: b.X + b.Width - rx, Y: b.Y + b.Height}},
		{{X: b.X + rx, Y: b.Y + b.Height}, {X: b.X + rx - k*rx, Y: b.Y + b.Height}, {X: b.X, Y: b.Y + b.Height - ry + k*ry}, {X: b.X, Y: b.Y + b.Height - ry}},
		{{X: b.X, Y: b.Y + ry}, {X: b.X, Y: b.Y + ry - k*ry}, {X: b.X + rx - k*rx, Y: b.Y}, {X: b.X + rx, Y: b.Y}},
	}
	lines := []d2scene.Point{
		{X: b.X + b.Width, Y: b.Y + b.Height - ry},
		{X: b.X + rx, Y: b.Y + b.Height},
		{X: b.X, Y: b.Y + ry},
	}
	for i, curve := range curves {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if i != 0 {
			if err := appendPoint(lines[i-1]); err != nil {
				return nil, err
			}
		}
		if err := flattenCubic(ctx, curve[0], curve[1], curve[2], curve[3], tolerance, 0, appendPoint); err != nil {
			return nil, err
		}
	}
	return []subpath{{points: points, closed: true}}, nil
}

func ellipseSubpaths(ctx context.Context, center d2scene.Point, rx, ry, tolerance float64, count func() error) ([]subpath, error) {
	if rx == 0 || ry == 0 {
		return nil, nil
	}
	k := 0.5522847498307936
	curves := [][4]d2scene.Point{
		{{X: center.X + rx, Y: center.Y}, {X: center.X + rx, Y: center.Y + k*ry}, {X: center.X + k*rx, Y: center.Y + ry}, {X: center.X, Y: center.Y + ry}},
		{{X: center.X, Y: center.Y + ry}, {X: center.X - k*rx, Y: center.Y + ry}, {X: center.X - rx, Y: center.Y + k*ry}, {X: center.X - rx, Y: center.Y}},
		{{X: center.X - rx, Y: center.Y}, {X: center.X - rx, Y: center.Y - k*ry}, {X: center.X - k*rx, Y: center.Y - ry}, {X: center.X, Y: center.Y - ry}},
		{{X: center.X, Y: center.Y - ry}, {X: center.X + k*rx, Y: center.Y - ry}, {X: center.X + rx, Y: center.Y - k*ry}, {X: center.X + rx, Y: center.Y}},
	}
	points := make([]d2scene.Point, 0, 32)
	appendPoint := func(point d2scene.Point) error {
		if err := count(); err != nil {
			return err
		}
		if len(points) == 0 || !samePoint(points[len(points)-1], point) {
			points = append(points, point)
		}
		return nil
	}
	if err := appendPoint(curves[0][0]); err != nil {
		return nil, err
	}
	for _, curve := range curves {
		if err := flattenCubic(ctx, curve[0], curve[1], curve[2], curve[3], tolerance, 0, appendPoint); err != nil {
			return nil, err
		}
	}
	return []subpath{{points: points, closed: true}}, nil
}

func flattenQuadratic(ctx context.Context, p0, p1, p2 d2scene.Point, tolerance float64, depth int, appendPoint func(d2scene.Point) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if pointLineDistance(p1, p0, p2) <= tolerance && controlsProgress(p0, p2, tolerance, p1) {
		return appendPoint(p2)
	}
	if depth >= maxCurveDepth {
		return fmt.Errorf("quadratic cannot be flattened within tolerance")
	}
	p01 := midpoint(p0, p1)
	p12 := midpoint(p1, p2)
	p012 := midpoint(p01, p12)
	if err := flattenQuadratic(ctx, p0, p01, p012, tolerance, depth+1, appendPoint); err != nil {
		return err
	}
	return flattenQuadratic(ctx, p012, p12, p2, tolerance, depth+1, appendPoint)
}

func flattenCubic(ctx context.Context, p0, p1, p2, p3 d2scene.Point, tolerance float64, depth int, appendPoint func(d2scene.Point) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if math.Max(pointLineDistance(p1, p0, p3), pointLineDistance(p2, p0, p3)) <= tolerance &&
		controlsProgress(p0, p3, tolerance, p1, p2) {
		return appendPoint(p3)
	}
	if depth >= maxCurveDepth {
		return fmt.Errorf("cubic cannot be flattened within tolerance")
	}
	p01 := midpoint(p0, p1)
	p12 := midpoint(p1, p2)
	p23 := midpoint(p2, p3)
	p012 := midpoint(p01, p12)
	p123 := midpoint(p12, p23)
	p0123 := midpoint(p012, p123)
	if err := flattenCubic(ctx, p0, p01, p012, p0123, tolerance, depth+1, appendPoint); err != nil {
		return err
	}
	return flattenCubic(ctx, p0123, p123, p23, p3, tolerance, depth+1, appendPoint)
}

func flattenArc(ctx context.Context, start d2scene.Point, command d2scene.PathCommand, transform d2scene.Matrix, tolerance float64, appendPoint func(d2scene.Point) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !finite(tolerance) || tolerance <= 0 {
		return fmt.Errorf("invalid arc flattening tolerance")
	}
	end := command.P1
	// SVG endpoint arcs with identical endpoints are omitted. This is distinct
	// from a zero-radius arc, which is a straight line to the endpoint. Still
	// invoke appendPoint: its callback charges the input command before exact
	// duplicate removal, preventing no-op arcs from bypassing path limits.
	if start == end {
		return appendPoint(end)
	}
	if command.RadiusX == 0 || command.RadiusY == 0 {
		return appendPoint(end)
	}

	arc, ok, err := d2scene.EndpointArcToCenter(start, command)
	if err != nil {
		return err
	}
	if !ok {
		return appendPoint(end)
	}
	radiusBound := arc.TransformedRadiusBound(transform)
	if !finite(radiusBound) {
		return fmt.Errorf("transformed arc radius is outside the numeric domain")
	}
	return flattenArcInterval(ctx, arc, arc.StartAngle(), arc.StartAngle()+arc.DeltaAngle(), end, radiusBound, tolerance, 0, appendPoint)
}

// flattenArcInterval uses the linear-interpolation remainder bound
// M*h^2/8. For an affine ellipse the second derivative M is bounded by
// hypot(|u|, |v|), so every emitted chord stays within tolerance without
// depending on a fixed angular sample count.
func flattenArcInterval(ctx context.Context, arc d2scene.CenterArc, theta0, theta1 float64, end d2scene.Point, radiusBound, tolerance float64, depth int, appendPoint func(d2scene.Point) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	span := math.Abs(theta1 - theta0)
	deviationFactor := span * span / 8
	if deviationFactor == 0 || radiusBound <= tolerance/deviationFactor {
		return appendPoint(end)
	}
	if depth >= maxCurveDepth {
		return fmt.Errorf("arc cannot be flattened within tolerance")
	}
	middleTheta := theta0 + (theta1-theta0)/2
	middle := arc.PointAt(middleTheta)
	if !finitePoint(middle) {
		return fmt.Errorf("arc is outside the numeric domain")
	}
	if err := flattenArcInterval(ctx, arc, theta0, middleTheta, middle, radiusBound, tolerance, depth+1, appendPoint); err != nil {
		return err
	}
	return flattenArcInterval(ctx, arc, middleTheta, theta1, end, radiusBound, tolerance, depth+1, appendPoint)
}

func pointLineDistance(point, start, end d2scene.Point) float64 {
	dx, dy := end.X-start.X, end.Y-start.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		return math.Hypot(point.X-start.X, point.Y-start.Y)
	}
	return math.Abs(dx*(start.Y-point.Y)-(start.X-point.X)*dy) / length
}

func controlsProgress(start, end d2scene.Point, tolerance float64, controls ...d2scene.Point) bool {
	dx, dy := end.X-start.X, end.Y-start.Y
	length := math.Hypot(dx, dy)
	if length == 0 {
		for _, control := range controls {
			if math.Hypot(control.X-start.X, control.Y-start.Y) > tolerance {
				return false
			}
		}
		return true
	}
	unitX, unitY := dx/length, dy/length
	previous := 0.0
	for _, control := range controls {
		controlX, controlY := control.X-start.X, control.Y-start.Y
		projection := controlX*unitX + controlY*unitY
		perpendicular := math.Abs(controlX*unitY - controlY*unitX)
		excess := 0.0
		if projection < 0 {
			excess = -projection
		} else if projection > length {
			excess = projection - length
		}
		// A small projection reversal is harmless only when every control
		// remains inside the tolerance-radius tube around the finite chord.
		// That tube is convex, so it also bounds the complete Bezier. Larger
		// reversals still subdivide to preserve dashes and retraced strokes.
		if !finite(projection) || !finite(perpendicular) || math.Hypot(perpendicular, excess) > tolerance || projection < previous-tolerance {
			return false
		}
		previous = projection
	}
	return true
}

func midpoint(a, b d2scene.Point) d2scene.Point {
	return d2scene.Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
}

func samePoint(a, b d2scene.Point) bool {
	return a == b
}

func interpolate(a, b d2scene.Point, t float64) d2scene.Point {
	return d2scene.Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t}
}

func drawStroke(ctx context.Context, dst *image.RGBA, runs []strokeRun, transform d2scene.Matrix, stroke *preparedStroke, scratch *rasterScratch) error {
	if stroke.paint.kind == preparedSolidPaint {
		rasterizer := scratch.reset(dst.Bounds())
		shifted := d2scene.Translate(-float64(dst.Bounds().Min.X), -float64(dst.Bounds().Min.Y)).Mul(transform)
		for _, run := range runs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := addStrokeRun(ctx, rasterizer, run, shifted, stroke); err != nil {
				return err
			}
		}
		if err := rasterizer.DrawRGBA(ctx, scratch.workBudget(), dst, stroke.paint.solid); err != nil {
			return fmt.Errorf("d2raster: solid stroke: %w", err)
		}
		return ctx.Err()
	}

	bounds := paintedStrokePixelBounds(runs, transform, stroke, dst.Bounds())
	return drawGradientMask(ctx, dst, bounds, stroke.paint, scratch, func(mask *image.Alpha) error {
		rasterizer := scratch.reset(mask.Bounds())
		shifted := d2scene.Translate(-float64(bounds.Min.X), -float64(bounds.Min.Y)).Mul(transform)
		for _, run := range runs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := addStrokeRun(ctx, rasterizer, run, shifted, stroke); err != nil {
				return err
			}
		}
		if err := rasterizer.WriteAlpha(ctx, scratch.workBudget(), mask); err != nil {
			return fmt.Errorf("d2raster: gradient stroke: %w", err)
		}
		return ctx.Err()
	})
}

func paintedStrokePixelBounds(runs []strokeRun, transform d2scene.Matrix, stroke *preparedStroke, canvas image.Rectangle) image.Rectangle {
	expansion := stroke.width * transform.MaxScale() / 2
	factor := 1.0
	if stroke.cap == d2scene.CapSquare {
		factor = math.Sqrt2
	}
	if stroke.join == d2scene.JoinMiter && stroke.miterLimit > factor {
		factor = stroke.miterLimit
	}
	return strokeRunPixelBounds(runs, transform, expansion*factor, canvas)
}

func strokeRunPixelBounds(runs []strokeRun, transform d2scene.Matrix, expansion float64, canvas image.Rectangle) image.Rectangle {
	paths := make([]subpath, len(runs))
	for index, run := range runs {
		paths[index] = subpath{points: run.points, closed: run.closed}
	}
	return subpathPixelBounds(paths, transform, expansion, canvas)
}

func makeStrokeRuns(ctx context.Context, path subpath, dashes []float64, offset float64, count func() error) ([]strokeRun, error) {
	points := cleanPoints(path.points, path.closed)
	if len(points) < 2 {
		return nil, nil
	}
	if len(dashes) == 0 {
		return []strokeRun{{points: points, closed: path.closed}}, nil
	}

	pattern := append([]float64(nil), dashes...)
	if len(pattern)%2 != 0 {
		pattern = append(pattern, pattern...)
	}
	total := 0.0
	for _, length := range pattern {
		total += length
	}
	phase := math.Mod(offset, total)
	if phase < 0 {
		phase += total
	}
	patternIndex := 0
	for phase >= pattern[patternIndex] {
		nextPhase := phase - pattern[patternIndex]
		if nextPhase == phase {
			return nil, fmt.Errorf("dash offset is too large relative to the dash pattern")
		}
		phase = nextPhase
		patternIndex = (patternIndex + 1) % len(pattern)
	}
	remaining := pattern[patternIndex] - phase
	on := patternIndex%2 == 0
	initialOn := on
	var runs []strokeRun
	var current []d2scene.Point

	edgeCount := len(points) - 1
	if path.closed {
		edgeCount = len(points)
	}
	for edge := 0; edge < edgeCount; edge++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		start := points[edge]
		end := points[(edge+1)%len(points)]
		dx, dy := end.X-start.X, end.Y-start.Y
		length := math.Hypot(dx, dy)
		if length == 0 {
			continue
		}
		position := 0.0
		for position < length {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			available := length - position
			hitBoundary := remaining <= available
			step := available
			if hitBoundary {
				step = remaining
			}
			nextPosition := position + step
			if nextPosition <= position {
				return nil, fmt.Errorf("dash pattern is below the path's floating-point resolution")
			}
			a := interpolate(start, end, position/length)
			b := interpolate(start, end, nextPosition/length)
			if on {
				if len(current) == 0 {
					if err := count(); err != nil {
						return nil, err
					}
					current = append(current, a)
				} else if !samePoint(current[len(current)-1], a) {
					if err := count(); err != nil {
						return nil, err
					}
					current = append(current, a)
				}
				if !samePoint(current[len(current)-1], b) {
					if err := count(); err != nil {
						return nil, err
					}
					current = append(current, b)
				}
			}
			position = nextPosition
			if hitBoundary {
				if on && len(current) >= 2 {
					runs = append(runs, strokeRun{points: current})
					current = nil
				}
				patternIndex = (patternIndex + 1) % len(pattern)
				on = patternIndex%2 == 0
				remaining = pattern[patternIndex]
			} else {
				remaining -= step
			}
		}
	}
	if on && len(current) >= 2 {
		runs = append(runs, strokeRun{points: current})
	}
	if !path.closed || len(runs) == 0 {
		return runs, nil
	}

	origin := points[0]
	firstStartsAtOrigin := samePoint(runs[0].points[0], origin)
	lastRun := &runs[len(runs)-1]
	lastEndsAtOrigin := samePoint(lastRun.points[len(lastRun.points)-1], origin)
	if len(runs) == 1 && firstStartsAtOrigin && lastEndsAtOrigin {
		runs[0].points = runs[0].points[:len(runs[0].points)-1]
		runs[0].closed = true
		return runs, nil
	}
	if initialOn && firstStartsAtOrigin && lastEndsAtOrigin && len(runs) > 1 {
		last := runs[len(runs)-1].points
		first := runs[0].points
		merged := make([]d2scene.Point, 0, len(last)+len(first)-1)
		merged = append(merged, last...)
		merged = append(merged, first[1:]...)
		runs[0] = strokeRun{points: merged}
		runs = runs[:len(runs)-1]
	}
	return runs, nil
}

func addStrokeRun(ctx context.Context, rasterizer *scanline.Rasterizer, run strokeRun, transform d2scene.Matrix, stroke *preparedStroke) error {
	// makeStrokeRuns canonicalizes every run during preflight, so rendering can
	// use the stored points without allocating another cleaned copy per dash.
	points := run.points
	halfWidth := stroke.width / 2
	if len(points) < 2 || halfWidth <= 0 {
		return nil
	}
	edgeCount := len(points) - 1
	if run.closed {
		edgeCount = len(points)
	}
	for edge := 0; edge < edgeCount; edge++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		start := points[edge]
		end := points[(edge+1)%len(points)]
		dx, dy := end.X-start.X, end.Y-start.Y
		length := math.Hypot(dx, dy)
		if length == 0 {
			continue
		}
		unitX, unitY := dx/length, dy/length
		if !run.closed && stroke.cap == d2scene.CapSquare {
			if edge == 0 {
				start.X -= unitX * halfWidth
				start.Y -= unitY * halfWidth
			}
			if edge == edgeCount-1 {
				end.X += unitX * halfWidth
				end.Y += unitY * halfWidth
			}
		}
		normal := d2scene.Point{X: -unitY * halfWidth, Y: unitX * halfWidth}
		addPolygon(rasterizer, transform, []d2scene.Point{
			{X: start.X + normal.X, Y: start.Y + normal.Y},
			{X: end.X + normal.X, Y: end.Y + normal.Y},
			{X: end.X - normal.X, Y: end.Y - normal.Y},
			{X: start.X - normal.X, Y: start.Y - normal.Y},
		})
	}

	if err := forEachStrokeJoin(ctx, points, run.closed, func(previous, vertex, next d2scene.Point) error {
		switch stroke.join {
		case d2scene.JoinRound:
			// The union of segment rectangles and a disk at each vertex is
			// the exact round-join outline (the centerline's Minkowski sum).
			addCircle(rasterizer, transform, vertex, halfWidth)
		case d2scene.JoinMiter, d2scene.JoinBevel:
			if polygon, ok := strokeJoinPolygon(previous, vertex, next, halfWidth, stroke.join, stroke.miterLimit); ok {
				addPolygon(rasterizer, transform, polygon)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if !run.closed && stroke.cap == d2scene.CapRound {
		addCircle(rasterizer, transform, points[0], halfWidth)
		addCircle(rasterizer, transform, points[len(points)-1], halfWidth)
	}
	return nil
}

func forEachStrokeJoin(ctx context.Context, points []d2scene.Point, closed bool, visit func(previous, vertex, next d2scene.Point) error) error {
	start, end := 1, len(points)-1
	if closed {
		if len(points) < 2 {
			return nil
		}
		start, end = 0, len(points)
	} else if len(points) < 3 {
		return nil
	}
	for index := start; index < end; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		previous := points[(index-1+len(points))%len(points)]
		vertex := points[index]
		next := points[(index+1)%len(points)]
		if err := visit(previous, vertex, next); err != nil {
			return err
		}
	}
	return nil
}

// strokeJoinPolygon returns the outer wedge missing from the union of the two
// adjacent segment rectangles. A miter over its limit degrades to the same
// three-point bevel wedge. Geometry is built in local coordinates so an affine
// transform, including a nonuniform one, affects the complete stroke outline.
func strokeJoinPolygon(previous, vertex, next d2scene.Point, halfWidth float64, join d2scene.LineJoin, miterLimit float64) ([]d2scene.Point, bool) {
	incoming, incomingOK := unitVector(previous, vertex)
	outgoing, outgoingOK := unitVector(vertex, next)
	if !incomingOK || !outgoingOK {
		return nil, false
	}
	turn := crossProduct(incoming, outgoing)
	if math.Abs(turn) <= geometryEpsilon {
		// A straight continuation needs no join. A reversal has no stable
		// outer side; the two butt-ended segment rectangles are the safe
		// finite bevel result.
		return nil, false
	}
	side := 1.0
	if turn > 0 {
		side = -1
	}
	incomingNormal := d2scene.Point{X: -incoming.Y * halfWidth * side, Y: incoming.X * halfWidth * side}
	outgoingNormal := d2scene.Point{X: -outgoing.Y * halfWidth * side, Y: outgoing.X * halfWidth * side}
	outerIncoming := d2scene.Point{X: vertex.X + incomingNormal.X, Y: vertex.Y + incomingNormal.Y}
	outerOutgoing := d2scene.Point{X: vertex.X + outgoingNormal.X, Y: vertex.Y + outgoingNormal.Y}
	bevel := []d2scene.Point{outerIncoming, vertex, outerOutgoing}
	if join != d2scene.JoinMiter {
		return bevel, true
	}

	denominator := crossProduct(incoming, outgoing)
	delta := d2scene.Point{X: outerOutgoing.X - outerIncoming.X, Y: outerOutgoing.Y - outerIncoming.Y}
	distanceAlongIncoming := crossProduct(delta, outgoing) / denominator
	miter := d2scene.Point{
		X: outerIncoming.X + distanceAlongIncoming*incoming.X,
		Y: outerIncoming.Y + distanceAlongIncoming*incoming.Y,
	}
	miterDistance := math.Hypot(miter.X-vertex.X, miter.Y-vertex.Y)
	ratio := miterDistance / halfWidth
	if !finitePoint(miter) || !finite(ratio) || ratio > miterLimit {
		return bevel, true
	}
	return []d2scene.Point{outerIncoming, miter, outerOutgoing, vertex}, true
}

func unitVector(start, end d2scene.Point) (d2scene.Point, bool) {
	dx, dy := end.X-start.X, end.Y-start.Y
	length := math.Hypot(dx, dy)
	if !finite(length) || length == 0 {
		return d2scene.Point{}, false
	}
	return d2scene.Point{X: dx / length, Y: dy / length}, true
}

func crossProduct(a, b d2scene.Point) float64 {
	return a.X*b.Y - a.Y*b.X
}

func addPolygon(rasterizer *scanline.Rasterizer, transform d2scene.Matrix, points []d2scene.Point) {
	if len(points) < 3 {
		return
	}
	area := transformedPolygonSignedArea2(points, transform)
	if area == 0 || !finite(area) {
		return
	}
	// Segment rectangles and round disks have negative local winding. Preserve
	// that sign through reflections so every union component accumulates rather
	// than cancelling its neighbors under non-zero rasterization.
	wantNegative := transform.Determinant() >= 0
	reverse := (area < 0) != wantNegative
	firstIndex := 0
	if reverse {
		firstIndex = len(points) - 1
	}
	first := transform.Point(points[firstIndex])
	rasterizer.MoveTo(float32(first.X), float32(first.Y))
	if reverse {
		for index := len(points) - 2; index >= 0; index-- {
			point := transform.Point(points[index])
			rasterizer.LineTo(float32(point.X), float32(point.Y))
		}
	} else {
		for _, localPoint := range points[1:] {
			point := transform.Point(localPoint)
			rasterizer.LineTo(float32(point.X), float32(point.Y))
		}
	}
	rasterizer.ClosePath()
}

func transformedPolygonSignedArea2(points []d2scene.Point, transform d2scene.Matrix) float64 {
	if len(points) < 3 {
		return 0
	}
	origin := transform.Point(points[0])
	area := 0.0
	for index := 1; index+1 < len(points); index++ {
		pointA := transform.Point(points[index])
		pointB := transform.Point(points[index+1])
		a := d2scene.Point{X: pointA.X - origin.X, Y: pointA.Y - origin.Y}
		b := d2scene.Point{X: pointB.X - origin.X, Y: pointB.Y - origin.Y}
		area += crossProduct(a, b)
	}
	return area
}

func addCircle(rasterizer *scanline.Rasterizer, transform d2scene.Matrix, center d2scene.Point, radius float64) {
	// The circle winding matches addPolygon so overlapping stroke components
	// form a non-zero union instead of cancelling one another.
	k := 0.5522847498307936 * radius
	start := transform.Point(d2scene.Point{X: center.X + radius, Y: center.Y})
	rasterizer.MoveTo(float32(start.X), float32(start.Y))
	curveTo := func(c1, c2, end d2scene.Point) {
		c1, c2, end = transform.Point(c1), transform.Point(c2), transform.Point(end)
		rasterizer.CubeTo(float32(c1.X), float32(c1.Y), float32(c2.X), float32(c2.Y), float32(end.X), float32(end.Y))
	}
	curveTo(
		d2scene.Point{X: center.X + radius, Y: center.Y - k},
		d2scene.Point{X: center.X + k, Y: center.Y - radius},
		d2scene.Point{X: center.X, Y: center.Y - radius},
	)
	curveTo(
		d2scene.Point{X: center.X - k, Y: center.Y - radius},
		d2scene.Point{X: center.X - radius, Y: center.Y - k},
		d2scene.Point{X: center.X - radius, Y: center.Y},
	)
	curveTo(
		d2scene.Point{X: center.X - radius, Y: center.Y + k},
		d2scene.Point{X: center.X - k, Y: center.Y + radius},
		d2scene.Point{X: center.X, Y: center.Y + radius},
	)
	curveTo(
		d2scene.Point{X: center.X + k, Y: center.Y + radius},
		d2scene.Point{X: center.X + radius, Y: center.Y + k},
		d2scene.Point{X: center.X + radius, Y: center.Y},
	)
	rasterizer.ClosePath()
}

func cleanPoints(points []d2scene.Point, closed bool) []d2scene.Point {
	cleaned := make([]d2scene.Point, 0, len(points))
	for _, point := range points {
		if len(cleaned) == 0 || !samePoint(cleaned[len(cleaned)-1], point) {
			cleaned = append(cleaned, point)
		}
	}
	if closed && len(cleaned) > 1 && samePoint(cleaned[0], cleaned[len(cleaned)-1]) {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return cleaned
}
