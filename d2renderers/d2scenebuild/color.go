package d2scenebuild

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/mazznoer/csscolorparser"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2themes"
	libcolor "github.com/d2lang/d2/lib/color"
)

func (b *builder) paint(raw, description string) (d2scene.Paint, error) {
	if raw == "" || strings.EqualFold(raw, libcolor.None) {
		return nil, nil
	}
	if libcolor.IsGradient(raw) {
		paint, err := gradientPaint(raw)
		if err != nil {
			return nil, fmt.Errorf("scene: parse %s gradient %q: %w", description, raw, err)
		}
		return paint, nil
	}
	resolved := d2themes.ResolveThemeColor(b.theme, raw)
	if resolved == "" {
		return nil, fmt.Errorf("scene: %s has unknown theme color %q", description, raw)
	}
	parsed, err := csscolorparser.Parse(resolved)
	if err != nil {
		return nil, fmt.Errorf("scene: parse %s color %q: %w", description, resolved, err)
	}
	r, g, blue, a := parsed.RGBA255()
	return d2scene.SolidPaint{Color: color.NRGBA{R: r, G: g, B: blue, A: a}}, nil
}

// gradientPaint translates the object-bounding-box gradient geometry emitted
// by lib/color so the rasterizer does not need to parse CSS strings.
func gradientPaint(raw string) (d2scene.Paint, error) {
	gradient, err := libcolor.ParseGradient(raw)
	if err != nil {
		return nil, err
	}
	if len(gradient.ColorStops) == 0 {
		return nil, fmt.Errorf("gradient has no valid color stops")
	}
	stops := make([]d2scene.GradientStop, len(gradient.ColorStops))
	previous := 0.0
	for i, stop := range gradient.ColorStops {
		parsed, err := csscolorparser.Parse(stop.Color)
		if err != nil {
			return nil, fmt.Errorf("stop %d color %q: %w", i, stop.Color, err)
		}
		offset, err := gradientStopOffset(stop.Position, i, len(gradient.ColorStops))
		if err != nil {
			return nil, fmt.Errorf("stop %d position %q: %w", i, stop.Position, err)
		}
		// SVG clamps stop offsets into [0,1] and prevents later stops from
		// moving behind an earlier one.
		offset = math.Max(previous, math.Max(0, math.Min(1, offset)))
		previous = offset
		r, g, blue, a := parsed.RGBA255()
		stops[i] = d2scene.GradientStop{Offset: offset, Color: color.NRGBA{R: r, G: g, B: blue, A: a}}
	}

	switch gradient.Type {
	case "linear":
		x1, y1, x2, y2, err := linearGradientVector(gradient.Direction)
		if err != nil {
			return nil, err
		}
		return d2scene.LinearGradient{
			Start: d2scene.Point{X: x1, Y: y1}, End: d2scene.Point{X: x2, Y: y2},
			Stops: stops, Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
		}, nil
	case "radial":
		return d2scene.RadialGradient{
			Center: d2scene.Point{X: .5, Y: .5}, Radius: .5,
			Focal: d2scene.Point{X: .5, Y: .5}, Stops: stops,
			Units: d2scene.ObjectBoundingBox, Transform: d2scene.Identity(), Spread: d2scene.SpreadPad,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported gradient type %q", gradient.Type)
	}
}

func gradientStopOffset(position string, index, total int) (float64, error) {
	if position == "" {
		if total == 1 {
			return 0, nil
		}
		return float64(index) / float64(total-1), nil
	}
	position = strings.TrimSpace(position)
	divisor := 1.0
	if strings.HasSuffix(position, "%") {
		position = strings.TrimSpace(strings.TrimSuffix(position, "%"))
		divisor = 100
	}
	value, err := strconv.ParseFloat(position, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("must be a finite number or percentage")
	}
	return value / divisor, nil
}

func linearGradientVector(direction string) (x1, y1, x2, y2 float64, err error) {
	x1, y1, x2, y2 = 0, 0, 0, 1
	direction = strings.TrimSpace(direction)
	if strings.HasPrefix(direction, "to ") {
		x1, y1, x2, y2 = .5, .5, .5, .5
		xSet, ySet := false, false
		for _, part := range strings.Fields(strings.TrimSpace(strings.TrimPrefix(direction, "to "))) {
			switch part {
			case "left":
				x1, x2, xSet = 1, 0, true
			case "right":
				x1, x2, xSet = 0, 1, true
			case "top":
				y1, y2, ySet = 1, 0, true
			case "bottom":
				y1, y2, ySet = 0, 1, true
			default:
				return 0, 0, 0, 0, fmt.Errorf("unsupported linear-gradient direction %q", direction)
			}
		}
		if !xSet {
			x1, x2 = .5, .5
		}
		if !ySet {
			y1, y2 = .5, .5
		}
		return x1, y1, x2, y2, nil
	}
	if strings.HasSuffix(direction, "deg") {
		degrees, parseErr := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(direction, "deg")), 64)
		if parseErr != nil || math.IsNaN(degrees) || math.IsInf(degrees, 0) {
			return 0, 0, 0, 0, fmt.Errorf("invalid linear-gradient angle %q", direction)
		}
		radians := (90 - degrees) * math.Pi / 180
		return .5, .5, .5 + .5*math.Cos(radians), .5 + .5*math.Sin(radians), nil
	}
	if direction != "" {
		return 0, 0, 0, 0, fmt.Errorf("unsupported linear-gradient direction %q", direction)
	}
	return x1, y1, x2, y2, nil
}
