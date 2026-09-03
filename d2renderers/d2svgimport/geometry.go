package d2svgimport

import (
	"errors"
	"fmt"
	"strings"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

type svgGeometryKind uint8

const (
	geometryNone svgGeometryKind = iota
	geometryRect
	geometryEllipse
	geometryPath
	geometryImage
)

type svgGeometry struct {
	kind        svgGeometryKind
	box         d2scene.Box
	center      d2scene.Point
	radiusX     float64
	radiusY     float64
	commands    []d2scene.PathCommand
	forceNoFill bool
	asset       d2scene.AssetID
	aspect      d2scene.AspectRatio
}

func (i *svgImporter) compileGeometry(element *svgElement) (svgGeometry, error) {
	switch element.name {
	case "svg", "g", "defs", "style", "use", "clipPath":
		return svgGeometry{}, nil
	case "path":
		commands, err := i.pathCommands(element, element.attrs["d"])
		if err != nil {
			return svgGeometry{}, err
		}
		// A path consisting only of moveto commands has no fill or stroke
		// geometry. In particular, round and square linecaps must not turn it
		// into a painted point (SVG 1.1, Paths, Path Data General Information).
		if pathHasOnlyMoves(commands) {
			return svgGeometry{}, nil
		}
		return svgGeometry{kind: geometryPath, commands: commands}, nil
	case "text":
		commands, err := i.frozenMathJaxTextCommands(element)
		if err != nil {
			return svgGeometry{}, err
		}
		return svgGeometry{kind: geometryPath, commands: commands}, nil
	case "image":
		return i.compileEmbeddedImage(element)
	case "rect":
		x, err := i.lengthAttribute(element, "x", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		y, err := i.lengthAttribute(element, "y", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		width, err := i.lengthAttribute(element, "width", 0, true)
		if err != nil {
			return svgGeometry{}, err
		}
		height, err := i.lengthAttribute(element, "height", 0, true)
		if err != nil {
			return svgGeometry{}, err
		}
		rx, haveRX, err := i.radiusAttribute(element, "rx")
		if err != nil {
			return svgGeometry{}, err
		}
		ry, haveRY, err := i.radiusAttribute(element, "ry")
		if err != nil {
			return svgGeometry{}, err
		}
		if haveRX && !haveRY {
			ry = rx
		} else if haveRY && !haveRX {
			rx = ry
		}
		if rx > width/2 {
			rx = width / 2
		}
		if ry > height/2 {
			ry = height / 2
		}
		return svgGeometry{kind: geometryRect, box: d2scene.Box{X: x, Y: y, Width: width, Height: height}, radiusX: rx, radiusY: ry}, nil
	case "circle":
		cx, err := i.lengthAttribute(element, "cx", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		cy, err := i.lengthAttribute(element, "cy", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		radius, err := i.lengthAttribute(element, "r", 0, true)
		if err != nil {
			return svgGeometry{}, err
		}
		return svgGeometry{kind: geometryEllipse, center: d2scene.Point{X: cx, Y: cy}, radiusX: radius, radiusY: radius}, nil
	case "ellipse":
		cx, err := i.lengthAttribute(element, "cx", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		cy, err := i.lengthAttribute(element, "cy", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		rx, haveRX, err := i.radiusAttribute(element, "rx")
		if err != nil {
			return svgGeometry{}, err
		}
		ry, haveRY, err := i.radiusAttribute(element, "ry")
		if err != nil {
			return svgGeometry{}, err
		}
		if haveRX && !haveRY {
			ry = rx
		} else if haveRY && !haveRX {
			rx = ry
		}
		return svgGeometry{kind: geometryEllipse, center: d2scene.Point{X: cx, Y: cy}, radiusX: rx, radiusY: ry}, nil
	case "line":
		x1, err := i.lengthAttribute(element, "x1", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		y1, err := i.lengthAttribute(element, "y1", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		x2, err := i.lengthAttribute(element, "x2", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		y2, err := i.lengthAttribute(element, "y2", 0, false)
		if err != nil {
			return svgGeometry{}, err
		}
		commands, err := i.addCommands(element, []d2scene.PathCommand{d2scene.MoveTo(x1, y1), d2scene.LineTo(x2, y2)})
		if err != nil {
			return svgGeometry{}, err
		}
		return svgGeometry{kind: geometryPath, commands: commands, forceNoFill: true}, nil
	case "polyline", "polygon":
		points := element.attrs["points"]
		commands, err := i.pointCommands(element, points, element.name == "polygon")
		if err != nil {
			if contextErr := i.ctx.Err(); contextErr != nil {
				return svgGeometry{}, contextErr
			}
			return svgGeometry{}, i.errorf("element <%s> has invalid points: %v", element.name, err)
		}
		return svgGeometry{kind: geometryPath, commands: commands}, nil
	default:
		return svgGeometry{}, i.errorf("internal unsupported geometry <%s>", element.name)
	}
}

func pathHasOnlyMoves(commands []d2scene.PathCommand) bool {
	if len(commands) == 0 {
		return false
	}
	for _, command := range commands {
		if command.Kind != d2scene.MoveCommand {
			return false
		}
	}
	return true
}

func (i *svgImporter) radiusAttribute(element *svgElement, name string) (float64, bool, error) {
	raw, ok := element.attrs[name]
	if !ok || (len(raw) <= 16 && strings.EqualFold(strings.TrimSpace(raw), "auto")) {
		return 0, false, nil
	}
	value, err := parseSVGLength(raw, true)
	if err != nil || value < 0 {
		if err == nil {
			err = fmt.Errorf("must not be negative")
		}
		return 0, false, i.propertyError(element, name, err)
	}
	return value, true, nil
}

func (i *svgImporter) pointCommands(element *svgElement, points string, closePath bool) ([]d2scene.PathCommand, error) {
	remaining := i.limits.MaxPathCommands - i.parsedPathCommand
	availablePairs := remaining
	if closePath && availablePairs > 0 {
		availablePairs--
	}
	if availablePairs < 0 {
		availablePairs = 0
	}
	maxFields := len(points)
	if availablePairs <= len(points)/2 {
		maxFields = availablePairs * 2
	}
	fields, err := splitSVGNumberList(i.ctx, points, maxFields)
	if err != nil {
		if errors.Is(err, errSVGNumberListLimit) {
			return nil, fmt.Errorf("path command count exceeds limit %d", i.limits.MaxPathCommands)
		}
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("expected complete coordinate pairs")
	}
	commandCount := len(fields) / 2
	if closePath {
		commandCount++
	}
	if commandCount > i.limits.MaxPathCommands-i.parsedPathCommand {
		return nil, fmt.Errorf("path command count exceeds limit %d", i.limits.MaxPathCommands)
	}
	commands := make([]d2scene.PathCommand, 0, commandCount)
	for index := 0; index < len(fields); index += 2 {
		if err := i.ctx.Err(); err != nil {
			return nil, err
		}
		x, err := parseSVGNumber(fields[index])
		if err != nil {
			return nil, fmt.Errorf("expected finite unitless coordinates")
		}
		y, err := parseSVGNumber(fields[index+1])
		if err != nil {
			return nil, fmt.Errorf("expected finite unitless coordinates")
		}
		if len(commands) == 0 {
			commands = append(commands, d2scene.MoveTo(x, y))
		} else {
			commands = append(commands, d2scene.LineTo(x, y))
		}
	}
	if closePath {
		commands = append(commands, d2scene.ClosePath())
	}
	i.parsedPathCommand += len(commands)
	return commands, nil
}

func (i *svgImporter) lengthAttribute(element *svgElement, name string, fallback float64, nonnegative bool) (float64, error) {
	raw, ok := element.attrs[name]
	if !ok {
		return fallback, nil
	}
	value, err := parseSVGLength(raw, true)
	if err != nil || (nonnegative && value < 0) {
		if err == nil {
			err = fmt.Errorf("must not be negative")
		}
		return 0, i.propertyError(element, name, err)
	}
	return value, nil
}

func (i *svgImporter) pathCommands(element *svgElement, data string) ([]d2scene.PathCommand, error) {
	trimmed, err := trimSVGSpace(i.ctx, data)
	if err != nil {
		return nil, err
	}
	if trimmed == "" {
		return nil, nil
	}
	remaining := i.limits.MaxPathCommands - i.parsedPathCommand
	if remaining <= 0 {
		return nil, i.errorf("path command count exceeds limit %d", i.limits.MaxPathCommands)
	}
	commands, parsed, err := parsePathWithCount(i.ctx, i.source, data, PathLimits{MaxBytes: i.limits.MaxBytes, MaxCommands: remaining})
	if err != nil {
		return nil, err
	}
	i.parsedPathCommand += parsed
	return append([]d2scene.PathCommand(nil), commands...), nil
}

func (i *svgImporter) addCommands(element *svgElement, commands []d2scene.PathCommand) ([]d2scene.PathCommand, error) {
	if len(commands) > i.limits.MaxPathCommands-i.parsedPathCommand {
		return nil, i.errorf("element <%s> makes path command count exceed limit %d", element.name, i.limits.MaxPathCommands)
	}
	i.parsedPathCommand += len(commands)
	return append([]d2scene.PathCommand(nil), commands...), nil
}
