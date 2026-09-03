// Package d2svgimport imports the bounded, network-free SVG subset used by D2
// assets and MathJax into d2scene.
package d2svgimport

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

// PathLimits are caller-selected hard limits for one SVG path data string.
// MaxCommands counts parsed source command groups, including groups such as an
// identical-endpoint arc that SVG defines to emit no segment. Both values must
// be positive.
type PathLimits struct {
	MaxBytes    int
	MaxCommands int
}

// ParsePath converts the complete common SVG path grammar to absolute typed
// d2scene commands. source must already be safe to include in an error (asset
// resolvers should pass their redacted display name, never a credentialed URL).
func ParsePath(ctx context.Context, source, data string, limits PathLimits) ([]d2scene.PathCommand, error) {
	commands, _, err := parsePathWithCount(ctx, source, data, limits)
	return commands, err
}

// parsePathWithCount also returns the number of parsed source command groups.
// ImportNode uses that count for its document-wide source limit; it must not
// infer source work from the smaller emitted command slice.
func parsePathWithCount(ctx context.Context, source, data string, limits PathLimits) ([]d2scene.PathCommand, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if limits.MaxBytes <= 0 || limits.MaxCommands <= 0 {
		return nil, 0, fmt.Errorf("d2svgimport: path limits must be positive")
	}
	if len(data) > limits.MaxBytes {
		return nil, 0, fmt.Errorf("d2svgimport: %s path data is %d bytes, exceeding limit %d", displaySource(source), len(data), limits.MaxBytes)
	}
	parser := pathParser{
		ctx: ctx, source: displaySource(source), data: data,
		maxCommands: limits.MaxCommands,
	}
	commands, err := parser.parse()
	if err != nil {
		return nil, 0, err
	}
	return commands, parser.parsedCommands, nil
}

type pathParser struct {
	ctx         context.Context
	source      string
	data        string
	offset      int
	maxCommands int
	commands    []d2scene.PathCommand
	// parsedCommands counts source command groups rather than emitted scene
	// commands. Degenerate arcs may intentionally emit nothing.
	parsedCommands int
	// afterExplicitCommand makes the first numeric argument reject a leading
	// comma. Commas remain valid between arguments and implicit groups.
	afterExplicitCommand bool

	current      d2scene.Point
	subpathStart d2scene.Point
	haveCurrent  bool
	previous     byte
	lastCubic    d2scene.Point
	lastQuad     d2scene.Point
}

func (p *pathParser) parse() ([]d2scene.PathCommand, error) {
	var command byte
	for {
		if err := p.skipWhitespace(); err != nil {
			return nil, err
		}
		if p.offset == len(p.data) {
			return append([]d2scene.PathCommand(nil), p.commands...), nil
		}
		explicitCommand := false
		if isCommand(p.data[p.offset]) {
			command = p.data[p.offset]
			p.offset++
			explicitCommand = true
			if command == 'Z' || command == 'z' {
				if !p.haveCurrent {
					return nil, p.errorf("close command appears before a move")
				}
				if err := p.chargeCommand(); err != nil {
					return nil, err
				}
				if err := p.append(d2scene.ClosePath()); err != nil {
					return nil, err
				}
				p.current = p.subpathStart
				p.previous = command
				command = 0
				continue
			}
		} else if command == 0 {
			return nil, p.errorf("expected a path command")
		}

		firstMove := command == 'M' || command == 'm'
		if err := p.chargeCommand(); err != nil {
			return nil, err
		}
		p.afterExplicitCommand = explicitCommand
		if err := p.parseGroup(command); err != nil {
			return nil, err
		}
		if firstMove {
			if command == 'M' {
				command = 'L'
			} else {
				command = 'l'
			}
		}
	}
}

func (p *pathParser) parseGroup(command byte) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	relative := command >= 'a' && command <= 'z'
	upper := command
	if relative {
		upper -= 'a' - 'A'
	}
	if upper != 'M' && !p.haveCurrent {
		return p.errorf("%c command appears before a move", command)
	}

	point := func(x, y float64) d2scene.Point {
		if relative {
			x += p.current.X
			y += p.current.Y
		}
		return d2scene.Point{X: x, Y: y}
	}
	resetControls := func() {
		p.lastCubic = d2scene.Point{}
		p.lastQuad = d2scene.Point{}
	}

	switch upper {
	case 'M':
		x, y, err := p.numberPair()
		if err != nil {
			return err
		}
		end := point(x, y)
		if err := p.append(d2scene.MoveTo(end.X, end.Y)); err != nil {
			return err
		}
		p.current, p.subpathStart, p.haveCurrent = end, end, true
		resetControls()
	case 'L':
		x, y, err := p.numberPair()
		if err != nil {
			return err
		}
		end := point(x, y)
		if err := p.append(d2scene.LineTo(end.X, end.Y)); err != nil {
			return err
		}
		p.current = end
		resetControls()
	case 'H':
		x, err := p.number()
		if err != nil {
			return err
		}
		if relative {
			x += p.current.X
		}
		if err := p.append(d2scene.LineTo(x, p.current.Y)); err != nil {
			return err
		}
		p.current.X = x
		resetControls()
	case 'V':
		y, err := p.number()
		if err != nil {
			return err
		}
		if relative {
			y += p.current.Y
		}
		if err := p.append(d2scene.LineTo(p.current.X, y)); err != nil {
			return err
		}
		p.current.Y = y
		resetControls()
	case 'C':
		values, err := p.numbers(6)
		if err != nil {
			return err
		}
		control1 := point(values[0], values[1])
		control2 := point(values[2], values[3])
		end := point(values[4], values[5])
		if err := p.append(d2scene.CubicTo(control1.X, control1.Y, control2.X, control2.Y, end.X, end.Y)); err != nil {
			return err
		}
		p.current, p.lastCubic, p.lastQuad = end, control2, d2scene.Point{}
	case 'S':
		values, err := p.numbers(4)
		if err != nil {
			return err
		}
		control1 := p.current
		if previousUpper(p.previous) == 'C' || previousUpper(p.previous) == 'S' {
			control1 = reflectPoint(p.lastCubic, p.current)
		}
		control2 := point(values[0], values[1])
		end := point(values[2], values[3])
		if err := p.append(d2scene.CubicTo(control1.X, control1.Y, control2.X, control2.Y, end.X, end.Y)); err != nil {
			return err
		}
		p.current, p.lastCubic, p.lastQuad = end, control2, d2scene.Point{}
	case 'Q':
		values, err := p.numbers(4)
		if err != nil {
			return err
		}
		control := point(values[0], values[1])
		end := point(values[2], values[3])
		if err := p.append(d2scene.QuadraticTo(control.X, control.Y, end.X, end.Y)); err != nil {
			return err
		}
		p.current, p.lastQuad, p.lastCubic = end, control, d2scene.Point{}
	case 'T':
		x, y, err := p.numberPair()
		if err != nil {
			return err
		}
		control := p.current
		if previousUpper(p.previous) == 'Q' || previousUpper(p.previous) == 'T' {
			control = reflectPoint(p.lastQuad, p.current)
		}
		end := point(x, y)
		if err := p.append(d2scene.QuadraticTo(control.X, control.Y, end.X, end.Y)); err != nil {
			return err
		}
		p.current, p.lastQuad, p.lastCubic = end, control, d2scene.Point{}
	case 'A':
		rx, err := p.number()
		if err != nil {
			return err
		}
		ry, err := p.number()
		if err != nil {
			return err
		}
		rotation, err := p.number()
		if err != nil {
			return err
		}
		largeArc, err := p.flag()
		if err != nil {
			return err
		}
		sweep, err := p.flag()
		if err != nil {
			return err
		}
		x, y, err := p.numberPair()
		if err != nil {
			return err
		}
		if rx < 0 || ry < 0 {
			return p.errorf("arc radii must be non-negative")
		}
		end := point(x, y)
		if end != p.current {
			if rx == 0 || ry == 0 {
				if err := p.append(d2scene.LineTo(end.X, end.Y)); err != nil {
					return err
				}
			} else if err := p.append(d2scene.ArcTo(rx, ry, math.Remainder(rotation, 360)/180*math.Pi, largeArc, sweep, end.X, end.Y)); err != nil {
				return err
			}
		}
		p.current = end
		resetControls()
	default:
		return p.errorf("unsupported path command %q", command)
	}
	p.previous = command
	return nil
}

func (p *pathParser) numberPair() (float64, float64, error) {
	values, err := p.numbers(2)
	if err != nil {
		return 0, 0, err
	}
	return values[0], values[1], nil
}

func (p *pathParser) numbers(count int) ([]float64, error) {
	values := make([]float64, count)
	for index := range values {
		value, err := p.number()
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	return values, nil
}

func (p *pathParser) number() (float64, error) {
	if p.afterExplicitCommand {
		p.afterExplicitCommand = false
		if err := p.skipWhitespace(); err != nil {
			return 0, err
		}
		if p.offset < len(p.data) && p.data[p.offset] == ',' {
			return 0, p.errorf("comma is not allowed before a command's first argument")
		}
	} else {
		if err := p.separator(); err != nil {
			return 0, err
		}
	}
	start := p.offset
	if p.offset < len(p.data) && (p.data[p.offset] == '+' || p.data[p.offset] == '-') {
		p.offset++
	}
	digits := 0
	for p.offset < len(p.data) && isDigit(p.data[p.offset]) {
		p.offset++
		digits++
		if p.offset&4095 == 0 {
			if err := p.ctx.Err(); err != nil {
				return 0, err
			}
		}
	}
	if p.offset < len(p.data) && p.data[p.offset] == '.' {
		p.offset++
		for p.offset < len(p.data) && isDigit(p.data[p.offset]) {
			p.offset++
			digits++
			if p.offset&4095 == 0 {
				if err := p.ctx.Err(); err != nil {
					return 0, err
				}
			}
		}
	}
	if digits == 0 {
		return 0, p.errorAt(start, "expected a finite number")
	}
	if p.offset < len(p.data) && (p.data[p.offset] == 'e' || p.data[p.offset] == 'E') {
		exponent := p.offset
		p.offset++
		if p.offset < len(p.data) && (p.data[p.offset] == '+' || p.data[p.offset] == '-') {
			p.offset++
		}
		exponentDigits := 0
		for p.offset < len(p.data) && isDigit(p.data[p.offset]) {
			p.offset++
			exponentDigits++
			if p.offset&4095 == 0 {
				if err := p.ctx.Err(); err != nil {
					return 0, err
				}
			}
		}
		if exponentDigits == 0 {
			return 0, p.errorAt(exponent, "number exponent has no digits")
		}
	}
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(p.data[start:p.offset], 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, p.errorAt(start, "expected a finite number")
	}
	return value, nil
}

func (p *pathParser) flag() (bool, error) {
	if err := p.separator(); err != nil {
		return false, err
	}
	if p.offset >= len(p.data) || (p.data[p.offset] != '0' && p.data[p.offset] != '1') {
		return false, p.errorf("arc flag must be 0 or 1")
	}
	value := p.data[p.offset] == '1'
	p.offset++
	return value, nil
}

func (p *pathParser) separator() error {
	if err := p.skipWhitespace(); err != nil {
		return err
	}
	if p.offset < len(p.data) && p.data[p.offset] == ',' {
		p.offset++
		if err := p.skipWhitespace(); err != nil {
			return err
		}
		if p.offset < len(p.data) && p.data[p.offset] == ',' {
			return p.errorf("repeated comma in path data")
		}
	}
	return nil
}

func (p *pathParser) skipWhitespace() error {
	for p.offset < len(p.data) && isWhitespace(p.data[p.offset]) {
		p.offset++
		if p.offset&4095 == 0 {
			if err := p.ctx.Err(); err != nil {
				return err
			}
		}
	}
	return p.ctx.Err()
}

func (p *pathParser) append(command d2scene.PathCommand) error {
	finitePoint := func(point d2scene.Point) bool {
		return !math.IsNaN(point.X) && !math.IsInf(point.X, 0) && !math.IsNaN(point.Y) && !math.IsInf(point.Y, 0)
	}
	valid := true
	switch command.Kind {
	case d2scene.MoveCommand, d2scene.LineCommand:
		valid = finitePoint(command.P1)
	case d2scene.QuadraticCommand:
		valid = finitePoint(command.P1) && finitePoint(command.P2)
	case d2scene.CubicCommand:
		valid = finitePoint(command.P1) && finitePoint(command.P2) && finitePoint(command.P3)
	case d2scene.ArcCommand:
		valid = finitePoint(command.P1) && !math.IsNaN(command.RadiusX) && !math.IsInf(command.RadiusX, 0) &&
			!math.IsNaN(command.RadiusY) && !math.IsInf(command.RadiusY, 0) &&
			!math.IsNaN(command.Rotation) && !math.IsInf(command.Rotation, 0)
	case d2scene.CloseCommand:
	default:
		valid = false
	}
	if !valid {
		return p.errorf("path command has a non-finite derived coordinate")
	}
	p.commands = append(p.commands, command)
	return p.ctx.Err()
}

func (p *pathParser) chargeCommand() error {
	if p.parsedCommands >= p.maxCommands {
		return p.errorf("path command count exceeds limit %d", p.maxCommands)
	}
	p.parsedCommands++
	return p.ctx.Err()
}

func (p *pathParser) errorf(format string, args ...any) error {
	return p.errorAt(p.offset, format, args...)
}

func (p *pathParser) errorAt(offset int, format string, args ...any) error {
	return fmt.Errorf("d2svgimport: %s path byte %d: %s", p.source, offset, fmt.Sprintf(format, args...))
}

func displaySource(source string) string {
	if source == "" {
		return "SVG asset"
	}
	return source
}

func reflectPoint(control, around d2scene.Point) d2scene.Point {
	return d2scene.Point{X: 2*around.X - control.X, Y: 2*around.Y - control.Y}
}

func previousUpper(command byte) byte {
	if command >= 'a' && command <= 'z' {
		return command - ('a' - 'A')
	}
	return command
}

func isCommand(value byte) bool {
	switch value {
	case 'M', 'm', 'Z', 'z', 'L', 'l', 'H', 'h', 'V', 'v', 'C', 'c', 'S', 's', 'Q', 'q', 'T', 't', 'A', 'a':
		return true
	default:
		return false
	}
}

func isDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func isWhitespace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	default:
		return false
	}
}
