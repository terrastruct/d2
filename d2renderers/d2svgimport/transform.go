package d2svgimport

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/d2lang/d2/d2renderers/d2scene"
)

type transformLimits struct {
	MaxBytes     int
	MaxFunctions int
}

func parseTransformWithCount(ctx context.Context, source, value string, limits transformLimits) (d2scene.Matrix, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return d2scene.Matrix{}, 0, err
	}
	if limits.MaxBytes <= 0 || limits.MaxFunctions <= 0 {
		return d2scene.Matrix{}, 0, fmt.Errorf("d2svgimport: transform limits must be positive")
	}
	if len(value) > limits.MaxBytes {
		return d2scene.Matrix{}, 0, fmt.Errorf("d2svgimport: %s transform is %d bytes, exceeding limit %d", displaySource(source), len(value), limits.MaxBytes)
	}
	parser := transformParser{ctx: ctx, source: displaySource(source), value: value, maxFunctions: limits.MaxFunctions}
	matrix, err := parser.parse()
	if err != nil {
		return d2scene.Matrix{}, 0, err
	}
	return matrix, parser.functions, nil
}

type transformParser struct {
	ctx          context.Context
	source       string
	value        string
	offset       int
	maxFunctions int
	functions    int
}

const maxTransformArguments = 6

func (p *transformParser) parse() (d2scene.Matrix, error) {
	result := d2scene.Identity()
	if err := p.skipWhitespace(); err != nil {
		return d2scene.Matrix{}, err
	}
	if p.offset == len(p.value) {
		return result, nil
	}
	for {
		nameStart := p.offset
		for p.offset < len(p.value) && isASCIILetter(p.value[p.offset]) {
			p.offset++
			if p.offset&4095 == 0 {
				if err := p.ctx.Err(); err != nil {
					return d2scene.Matrix{}, err
				}
			}
		}
		if nameStart == p.offset {
			return d2scene.Matrix{}, p.errorf("expected a transform function")
		}
		name := p.value[nameStart:p.offset]
		if err := p.skipWhitespace(); err != nil {
			return d2scene.Matrix{}, err
		}
		if p.offset >= len(p.value) || p.value[p.offset] != '(' {
			return d2scene.Matrix{}, p.errorf("transform function %q is missing '('", name)
		}
		p.offset++
		arguments, err := p.arguments()
		if err != nil {
			return d2scene.Matrix{}, err
		}
		p.functions++
		if p.functions > p.maxFunctions {
			return d2scene.Matrix{}, p.errorf("transform function count exceeds limit %d", p.maxFunctions)
		}
		function, err := transformFunction(name, arguments)
		if err != nil {
			return d2scene.Matrix{}, p.errorf("%v", err)
		}
		result = result.Mul(function)
		if !result.IsFinite() {
			return d2scene.Matrix{}, p.errorf("composed transform is non-finite")
		}
		if err := p.separatorBetweenFunctions(); err != nil {
			return d2scene.Matrix{}, err
		}
		if p.offset == len(p.value) {
			return result, nil
		}
	}
}

func (p *transformParser) arguments() ([]float64, error) {
	arguments := make([]float64, 0, 6)
	for {
		whitespaceStart := p.offset
		if err := p.skipWhitespace(); err != nil {
			return nil, err
		}
		hadWhitespace := p.offset != whitespaceStart
		if p.offset >= len(p.value) {
			return nil, p.errorf("unterminated transform function")
		}
		if p.value[p.offset] == ')' {
			p.offset++
			return arguments, nil
		}
		if len(arguments) != 0 && p.value[p.offset] == ',' {
			p.offset++
			if err := p.skipWhitespace(); err != nil {
				return nil, err
			}
			if p.offset >= len(p.value) || p.value[p.offset] == ',' || p.value[p.offset] == ')' {
				return nil, p.errorf("transform has an empty argument")
			}
		} else if len(arguments) == 0 && p.value[p.offset] == ',' {
			return nil, p.errorf("transform has an empty first argument")
		} else if len(arguments) != 0 && !hadWhitespace {
			return nil, p.errorf("transform arguments must be separated by whitespace or a comma")
		}
		argument, err := p.number()
		if err != nil {
			return nil, err
		}
		if len(arguments) >= maxTransformArguments {
			return nil, p.errorf("transform has more than %d arguments", maxTransformArguments)
		}
		arguments = append(arguments, argument)
	}
}

func (p *transformParser) number() (float64, error) {
	start := p.offset
	if p.offset < len(p.value) && (p.value[p.offset] == '+' || p.value[p.offset] == '-') {
		p.offset++
	}
	digits := 0
	for p.offset < len(p.value) && isDigit(p.value[p.offset]) {
		p.offset++
		digits++
		if p.offset&4095 == 0 {
			if err := p.ctx.Err(); err != nil {
				return 0, err
			}
		}
	}
	if p.offset < len(p.value) && p.value[p.offset] == '.' {
		p.offset++
		for p.offset < len(p.value) && isDigit(p.value[p.offset]) {
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
		return 0, p.errorAt(start, "expected a finite transform number")
	}
	if p.offset < len(p.value) && (p.value[p.offset] == 'e' || p.value[p.offset] == 'E') {
		exponent := p.offset
		p.offset++
		if p.offset < len(p.value) && (p.value[p.offset] == '+' || p.value[p.offset] == '-') {
			p.offset++
		}
		exponentDigits := 0
		for p.offset < len(p.value) && isDigit(p.value[p.offset]) {
			p.offset++
			exponentDigits++
			if p.offset&4095 == 0 {
				if err := p.ctx.Err(); err != nil {
					return 0, err
				}
			}
		}
		if exponentDigits == 0 {
			return 0, p.errorAt(exponent, "transform exponent has no digits")
		}
	}
	if err := p.ctx.Err(); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(p.value[start:p.offset], 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, p.errorAt(start, "expected a finite transform number")
	}
	return value, nil
}

func (p *transformParser) separatorBetweenFunctions() error {
	start := p.offset
	if err := p.skipWhitespace(); err != nil {
		return err
	}
	hadWhitespace := p.offset != start
	if p.offset == len(p.value) {
		return nil
	}
	if p.offset < len(p.value) && p.value[p.offset] == ',' {
		p.offset++
		if err := p.skipWhitespace(); err != nil {
			return err
		}
		if p.offset >= len(p.value) || p.value[p.offset] == ',' {
			return p.errorf("transform has an empty function")
		}
		return nil
	}
	if !hadWhitespace {
		return p.errorf("transform functions must be separated by whitespace or a comma")
	}
	return nil
}

func (p *transformParser) skipWhitespace() error {
	for p.offset < len(p.value) && isWhitespace(p.value[p.offset]) {
		p.offset++
		if p.offset&4095 == 0 {
			if err := p.ctx.Err(); err != nil {
				return err
			}
		}
	}
	return p.ctx.Err()
}

func (p *transformParser) errorf(format string, arguments ...any) error {
	return p.errorAt(p.offset, format, arguments...)
}

func (p *transformParser) errorAt(offset int, format string, arguments ...any) error {
	return fmt.Errorf("d2svgimport: %s transform byte %d: %s", p.source, offset, fmt.Sprintf(format, arguments...))
}

func transformFunction(name string, values []float64) (d2scene.Matrix, error) {
	wrongArity := func(expected string) (d2scene.Matrix, error) {
		return d2scene.Matrix{}, fmt.Errorf("transform function %q has %d arguments; expected %s", name, len(values), expected)
	}
	switch name {
	case "matrix":
		if len(values) != 6 {
			return wrongArity("6")
		}
		return d2scene.Matrix{A: values[0], B: values[1], C: values[2], D: values[3], E: values[4], F: values[5]}, nil
	case "translate":
		if len(values) != 1 && len(values) != 2 {
			return wrongArity("1 or 2")
		}
		y := 0.0
		if len(values) == 2 {
			y = values[1]
		}
		return d2scene.Translate(values[0], y), nil
	case "scale":
		if len(values) != 1 && len(values) != 2 {
			return wrongArity("1 or 2")
		}
		y := values[0]
		if len(values) == 2 {
			y = values[1]
		}
		return d2scene.Scale(values[0], y), nil
	case "rotate":
		if len(values) != 1 && len(values) != 3 {
			return wrongArity("1 or 3")
		}
		radians := math.Remainder(values[0], 360) / 180 * math.Pi
		if len(values) == 3 {
			return d2scene.RotateAround(radians, values[1], values[2]), nil
		}
		return d2scene.Rotate(radians), nil
	case "skewX":
		if len(values) != 1 {
			return wrongArity("1")
		}
		return d2scene.SkewX(math.Remainder(values[0], 360) / 180 * math.Pi), nil
	case "skewY":
		if len(values) != 1 {
			return wrongArity("1")
		}
		return d2scene.SkewY(math.Remainder(values[0], 360) / 180 * math.Pi), nil
	default:
		return d2scene.Matrix{}, fmt.Errorf("unsupported transform function %q", name)
	}
}

func isASCIILetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
