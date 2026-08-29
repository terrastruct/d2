package d2raster

import (
	"context"
	"errors"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

func TestArcFlatteningFlagsRotationAndRadiiCorrection(t *testing.T) {
	t.Parallel()
	for _, largeArc := range []bool{false, true} {
		for _, sweep := range []bool{false, true} {
			largeArc, sweep := largeArc, sweep
			t.Run(flagName(largeArc, sweep), func(t *testing.T) {
				t.Parallel()
				start := d2scene.Point{X: 3.25, Y: -4.5}
				command := d2scene.ArcTo(80, 50, 0.47, largeArc, sweep, 17.75, 8.25)
				arc, ok, err := d2scene.EndpointArcToCenter(start, command)
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					t.Fatal("non-degenerate arc was not converted")
				}
				var points []d2scene.Point
				err = flattenArc(context.Background(), start, command, d2scene.Identity(), 0.01, func(point d2scene.Point) error {
					points = append(points, point)
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(points) < 4 {
					t.Fatalf("flattened point count = %d, want adaptive curve", len(points))
				}
				if got := points[len(points)-1]; got != command.P1 {
					t.Fatalf("last point = %#v, want exact endpoint %#v", got, command.P1)
				}
				if sweep && arc.DeltaAngle() <= 0 || !sweep && arc.DeltaAngle() >= 0 {
					t.Fatalf("delta = %g for sweep=%t", arc.DeltaAngle(), sweep)
				}
				if largeArc && math.Abs(arc.DeltaAngle()) <= math.Pi || !largeArc && math.Abs(arc.DeltaAngle()) >= math.Pi {
					t.Fatalf("delta = %g for largeArc=%t", arc.DeltaAngle(), largeArc)
				}
			})
		}
	}
}

func TestArcFlatteningHasConservativeErrorAndTransformTolerance(t *testing.T) {
	t.Parallel()
	start := d2scene.Point{X: 5, Y: 12}
	command := d2scene.ArcTo(31, 7, 0.61, true, true, 72, 19)
	arc, ok, err := d2scene.EndpointArcToCenter(start, command)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("non-degenerate arc was not converted")
	}
	const tolerance = 0.025
	points := []d2scene.Point{start}
	if err := flattenArc(context.Background(), start, command, d2scene.Identity(), tolerance, func(point d2scene.Point) error {
		points = append(points, point)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Dense analytic samples must remain within the promised chord error.
	for sample := 0; sample <= 20_000; sample++ {
		theta := arc.StartAngle() + arc.DeltaAngle()*float64(sample)/20_000
		point := arc.PointAt(theta)
		best := math.Inf(1)
		for i := 1; i < len(points); i++ {
			best = math.Min(best, pointSegmentDistance(point, points[i-1], points[i]))
		}
		if best > tolerance*(1+1e-9) {
			t.Fatalf("sample %d chord error = %.12g, want <= %.12g", sample, best, tolerance)
		}
	}
	for name, transform := range map[string]d2scene.Matrix{
		"shear":      {A: 2.5, B: -1.2, C: 1.7, D: 0.4},
		"reflection": d2scene.Scale(-4, 0.75).Mul(d2scene.Rotate(0.31)),
	} {
		t.Run(name, func(t *testing.T) {
			devicePoints := []d2scene.Point{transform.Point(start)}
			if err := flattenArc(context.Background(), start, command, transform, tolerance, func(point d2scene.Point) error {
				devicePoints = append(devicePoints, transform.Point(point))
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			for sample := 0; sample <= 5_000; sample++ {
				theta := arc.StartAngle() + arc.DeltaAngle()*float64(sample)/5_000
				point := transform.Point(arc.PointAt(theta))
				best := math.Inf(1)
				for i := 1; i < len(devicePoints); i++ {
					best = math.Min(best, pointSegmentDistance(point, devicePoints[i-1], devicePoints[i]))
				}
				if best > tolerance*(1+1e-9) {
					t.Fatalf("sample %d device chord error = %.12g, want <= %.12g", sample, best, tolerance)
				}
			}
		})
	}

	path := d2scene.Path{Commands: []d2scene.PathCommand{d2scene.MoveTo(start.X, start.Y), command}}
	identity, err := flattenScenePath(context.Background(), path, d2scene.Identity(), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	scaled, err := flattenScenePath(context.Background(), path, d2scene.Scale(100, 100), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(scaled[0].points) <= len(identity[0].points) {
		t.Fatalf("scaled points = %d, identity points = %d; want finer device-space flattening", len(scaled[0].points), len(identity[0].points))
	}
}

func TestDiameterArcSignedZeroSelectsSweep(t *testing.T) {
	t.Parallel()
	start := d2scene.Point{X: -1, Y: math.Copysign(0, -1)}
	for _, sweep := range []bool{false, true} {
		arc, ok, err := d2scene.EndpointArcToCenter(start, d2scene.ArcTo(1, 1, math.Copysign(0, -1), false, sweep, 1, 0))
		if err != nil || !ok {
			t.Fatalf("sweep=%t conversion = %#v, %t, %v", sweep, arc, ok, err)
		}
		if math.Abs(math.Abs(arc.DeltaAngle())-math.Pi) > 1e-15 {
			t.Fatalf("sweep=%t delta magnitude = %g", sweep, arc.DeltaAngle())
		}
		if sweep && arc.DeltaAngle() <= 0 || !sweep && arc.DeltaAngle() >= 0 {
			t.Fatalf("sweep=%t delta sign = %g", sweep, arc.DeltaAngle())
		}
	}
}

func TestArcDegeneratesLimitsAndCancellation(t *testing.T) {
	t.Parallel()
	start := d2scene.Point{X: 1, Y: 2}
	var points []d2scene.Point
	if err := flattenArc(context.Background(), start, d2scene.ArcTo(0, 4, 0, false, true, 9, 7), d2scene.Identity(), 0.1, func(point d2scene.Point) error {
		points = append(points, point)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0] != (d2scene.Point{X: 9, Y: 7}) {
		t.Fatalf("zero-radius arc = %#v, want line endpoint", points)
	}
	points = nil
	if err := flattenArc(context.Background(), start, d2scene.ArcTo(4, 4, 0, true, true, start.X, start.Y), d2scene.Identity(), 0.1, func(point d2scene.Point) error {
		points = append(points, point)
		return nil
	}); err != nil || len(points) != 1 || points[0] != start {
		t.Fatalf("same-endpoint arc callback = %#v, %v; want one charged duplicate", points, err)
	}
	positive, negative := make([]d2scene.Point, 0), make([]d2scene.Point, 0)
	for _, test := range []struct {
		command d2scene.PathCommand
		points  *[]d2scene.Point
	}{
		{command: d2scene.ArcTo(8, 3, 0.2, false, true, 12, 6), points: &positive},
		{command: d2scene.ArcTo(-8, -3, 0.2, false, true, 12, 6), points: &negative},
	} {
		if err := flattenArc(context.Background(), start, test.command, d2scene.Identity(), 0.05, func(point d2scene.Point) error {
			*test.points = append(*test.points, point)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(positive) != len(negative) {
		t.Fatalf("negative radii generated %d points, positive radii generated %d", len(negative), len(positive))
	}
	for index := range positive {
		if positive[index] != negative[index] {
			t.Fatalf("negative radius point %d = %#v, want %#v", index, negative[index], positive[index])
		}
	}

	limitError := errors.New("test point limit")
	count := 0
	err := flattenArc(context.Background(), start, d2scene.ArcTo(1e6, 1e6, 0, true, true, 4, 5), d2scene.Identity(), 1e-4, func(d2scene.Point) error {
		count++
		if count > 7 {
			return limitError
		}
		return nil
	})
	if !errors.Is(err, limitError) || count != 8 {
		t.Fatalf("bounded callback error = %v after %d points", err, count)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = flattenArc(ctx, start, d2scene.ArcTo(10, 10, 0, false, true, 8, 9), d2scene.Identity(), 0.1, func(d2scene.Point) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled arc error = %v, want context.Canceled", err)
	}

	err = flattenArc(context.Background(), start, d2scene.ArcTo(math.MaxFloat64, math.MaxFloat64, 0, false, true, 8, 9), d2scene.Identity(), 0.1, func(d2scene.Point) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "numeric domain") {
		t.Fatalf("extreme arc error = %v, want numeric-domain rejection", err)
	}
}

func TestArcRendersAsPrimitiveAndClip(t *testing.T) {
	t.Parallel()
	paint := d2scene.SolidPaint{Color: color.NRGBA{R: 220, G: 40, B: 30, A: 255}}
	arcPath := d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(5, 25),
			d2scene.ArcTo(20, 20, 0, false, true, 45, 25),
			d2scene.ArcTo(20, 20, 0, false, true, 5, 25),
			d2scene.ClosePath(),
		},
		Fill: paint,
	}
	root := d2scene.NewNode(arcPath)
	root.Clip = &d2scene.Clip{Transform: d2scene.Identity(), Path: d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(5, 25),
			d2scene.ArcTo(20, 20, 0, false, true, 45, 25),
			d2scene.ArcTo(20, 20, 0, false, true, 5, 25),
			d2scene.ClosePath(),
		},
	}}
	document := d2scene.NewDocument(d2scene.Box{Width: 50, Height: 50}, root)
	image, err := Render(context.Background(), document, testOptions())
	if err != nil {
		t.Fatal(err)
	}
	if image.NRGBAAt(25, 25).A == 0 {
		t.Fatal("arc circle center is transparent")
	}
	if image.NRGBAAt(25, 4).A != 0 || image.NRGBAAt(25, 46).A != 0 {
		t.Fatal("arc clip leaked outside its circle")
	}
}

func TestArcGeometrySurvivesInverseScaleCoordinates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		radius float64
		scale  float64
	}{
		{name: "tiny local expanded", radius: 1e-80, scale: 1e80},
		{name: "huge local contracted", radius: 1e20, scale: 1e-20},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			path := circleArcPath(test.radius, black)
			transform := d2scene.Translate(25, 25).Mul(d2scene.Scale(test.scale, test.scale))

			primitive := d2scene.NewNode(path)
			primitive.Transform = transform
			primitiveImage, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 50, Height: 50}, primitive), testOptions())
			if err != nil {
				t.Fatal(err)
			}
			if primitiveImage.NRGBAAt(25, 25).A == 0 || primitiveImage.NRGBAAt(2, 2).A != 0 {
				t.Fatalf("scaled primitive center/corner alpha = %d/%d", primitiveImage.NRGBAAt(25, 25).A, primitiveImage.NRGBAAt(2, 2).A)
			}

			clipped := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 50, Height: 50}, Fill: black})
			clipped.Clip = &d2scene.Clip{Path: circleArcPath(test.radius, nil), Transform: transform}
			clipImage, err := Render(context.Background(), d2scene.NewDocument(d2scene.Box{Width: 50, Height: 50}, clipped), testOptions())
			if err != nil {
				t.Fatal(err)
			}
			if clipImage.NRGBAAt(25, 25).A == 0 || clipImage.NRGBAAt(2, 2).A != 0 {
				t.Fatalf("scaled clip center/corner alpha = %d/%d", clipImage.NRGBAAt(25, 25).A, clipImage.NRGBAAt(2, 2).A)
			}
		})
	}
}

func TestIdenticalEndpointArcsConsumePathBudget(t *testing.T) {
	t.Parallel()
	const arcCount = 8
	commands := []d2scene.PathCommand{d2scene.MoveTo(5, 5)}
	for range arcCount {
		commands = append(commands, d2scene.ArcTo(3, 2, 0.2, true, true, 5, 5))
	}
	path := d2scene.Path{Commands: commands, Fill: black}
	for _, test := range []struct {
		name       string
		clip       bool
		extraLimit int
	}{
		{name: "primitive exact", extraLimit: 0},
		{name: "primitive plus one", extraLimit: -1},
		{name: "clip exact", clip: true, extraLimit: 0},
		{name: "clip plus one", clip: true, extraLimit: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document *d2scene.Document
			baseCommands := 1 + arcCount
			if test.clip {
				node := d2scene.NewNode(d2scene.Rect{Box: d2scene.Box{Width: 10, Height: 10}, Fill: black})
				node.Clip = &d2scene.Clip{Path: path, Transform: d2scene.Identity()}
				document = d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, node)
				baseCommands += 4 // the rectangle's flattened corners
			} else {
				document = d2scene.NewDocument(d2scene.Box{Width: 10, Height: 10}, d2scene.NewNode(path))
			}
			options := testOptions()
			options.MaxPathCommands = baseCommands + test.extraLimit
			_, err := Render(context.Background(), document, options)
			if test.extraLimit == 0 && err != nil {
				t.Fatalf("exact path budget failed: %v", err)
			}
			if test.extraLimit < 0 && (err == nil || !strings.Contains(err.Error(), "path command")) {
				t.Fatalf("limit+1 error = %v, want path command rejection", err)
			}
		})
	}
}

func circleArcPath(radius float64, fill d2scene.Paint) d2scene.Path {
	return d2scene.Path{
		Commands: []d2scene.PathCommand{
			d2scene.MoveTo(-radius, 0),
			d2scene.ArcTo(radius, radius, 0, false, true, radius, 0),
			d2scene.ArcTo(radius, radius, 0, false, true, -radius, 0),
			d2scene.ClosePath(),
		},
		Fill: fill,
	}
}

func pointSegmentDistance(point, start, end d2scene.Point) float64 {
	dx, dy := end.X-start.X, end.Y-start.Y
	lengthSquared := dx*dx + dy*dy
	if lengthSquared == 0 {
		return math.Hypot(point.X-start.X, point.Y-start.Y)
	}
	t := ((point.X-start.X)*dx + (point.Y-start.Y)*dy) / lengthSquared
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(point.X-(start.X+t*dx), point.Y-(start.Y+t*dy))
}

func flagName(largeArc, sweep bool) string {
	return "large=" + boolName(largeArc) + "/sweep=" + boolName(sweep)
}

func boolName(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
