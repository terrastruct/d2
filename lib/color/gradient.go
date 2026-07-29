package color

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Gradient struct {
	Type       string
	Direction  string
	ColorStops []ColorStop
	ID         string
}

type ColorStop struct {
	Color    string
	Position string
}

func ParseGradient(cssGradient string) (Gradient, error) {
	cssGradient = strings.TrimSpace(cssGradient)

	re := regexp.MustCompile(`^(linear-gradient|radial-gradient)\((.*)\)$`)
	matches := re.FindStringSubmatch(cssGradient)
	if matches == nil {
		return Gradient{}, errors.New("invalid gradient syntax")
	}

	gradientType := matches[1]
	params := matches[2]

	gradient := Gradient{
		Type: strings.TrimSuffix(gradientType, "-gradient"),
	}

	paramList := splitParams(params)

	if len(paramList) == 0 {
		return Gradient{}, errors.New("no parameters in gradient")
	}

	firstParam := strings.TrimSpace(paramList[0])

	if gradient.Type == "linear" && (strings.HasSuffix(firstParam, "deg") || strings.HasPrefix(firstParam, "to ")) {
		gradient.Direction = firstParam
		colorStops := paramList[1:]
		if len(colorStops) == 0 {
			return Gradient{}, errors.New("no color stops in gradient")
		}
		gradient.ColorStops = parseColorStops(colorStops)
	} else if gradient.Type == "radial" && (firstParam == "circle" || firstParam == "ellipse") {
		gradient.Direction = firstParam
		colorStops := paramList[1:]
		if len(colorStops) == 0 {
			return Gradient{}, errors.New("no color stops in gradient")
		}
		gradient.ColorStops = parseColorStops(colorStops)
	} else {
		gradient.ColorStops = parseColorStops(paramList)
	}
	gradient.ID = UniqueGradientID(cssGradient)

	return gradient, nil
}

func splitParams(params string) []string {
	var parts []string
	var buf strings.Builder
	nesting := 0

	for _, r := range params {
		switch r {
		case ',':
			if nesting == 0 {
				parts = append(parts, buf.String())
				buf.Reset()
				continue
			}
		case '(':
			nesting++
		case ')':
			if nesting > 0 {
				nesting--
			}
		}
		buf.WriteRune(r)
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

func parseColorStops(params []string) []ColorStop {
	var colorStops []ColorStop
	for _, p := range params {
		p = strings.TrimSpace(p)
		parts := strings.Fields(p)

		switch len(parts) {
		case 1:
			colorStops = append(colorStops, ColorStop{Color: parts[0]})
		case 2:
			colorStops = append(colorStops, ColorStop{Color: parts[0], Position: parts[1]})
		default:
			continue
		}
	}
	return colorStops
}

func GradientToSVG(gradient Gradient) string {
	switch gradient.Type {
	case "linear":
		return LinearGradientToSVG(gradient)
	case "radial":
		return RadialGradientToSVG(gradient)
	default:
		return ""
	}
}

func LinearGradientToSVG(gradient Gradient) string {
	x1, y1, x2, y2 := parseLinearGradientDirection(gradient.Direction)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<linearGradient id="%s" `, gradient.ID))
	sb.WriteString(fmt.Sprintf(`x1="%s" y1="%s" x2="%s" y2="%s">`, x1, y1, x2, y2))
	sb.WriteString("\n")

	totalStops := len(gradient.ColorStops)
	for i, cs := range gradient.ColorStops {
		offset := cs.Position
		if offset == "" {
			offsetValue := float64(i) / float64(totalStops-1) * 100
			offset = fmt.Sprintf("%.2f%%", offsetValue)
		}
		sb.WriteString(fmt.Sprintf(`<stop offset="%s" stop-color="%s" />`, offset, cs.Color))
		sb.WriteString("\n")
	}
	sb.WriteString(`</linearGradient>`)
	return sb.String()
}

func parseLinearGradientDirection(direction string) (x1, y1, x2, y2 string) {
	x1, y1, x2, y2 = "0%", "0%", "0%", "100%"

	direction = strings.TrimSpace(direction)
	if strings.HasPrefix(direction, "to ") {
		dir := strings.TrimPrefix(direction, "to ")
		dir = strings.TrimSpace(dir)
		parts := strings.Fields(dir)
		xStart, yStart := "50%", "50%"
		xEnd, yEnd := "50%", "50%"

		xDirSet, yDirSet := false, false

		for _, part := range parts {
			switch part {
			case "left":
				xStart = "100%"
				xEnd = "0%"
				xDirSet = true
			case "right":
				xStart = "0%"
				xEnd = "100%"
				xDirSet = true
			case "top":
				yStart = "100%"
				yEnd = "0%"
				yDirSet = true
			case "bottom":
				yStart = "0%"
				yEnd = "100%"
				yDirSet = true
			}
		}

		if !xDirSet {
			xStart = "50%"
			xEnd = "50%"
		}

		if !yDirSet {
			yStart = "50%"
			yEnd = "50%"
		}

		x1, y1 = xStart, yStart
		x2, y2 = xEnd, yEnd
	} else if strings.HasSuffix(direction, "deg") {
		angleStr := strings.TrimSuffix(direction, "deg")
		angle, err := strconv.ParseFloat(strings.TrimSpace(angleStr), 64)
		if err == nil {
			cssAngle := angle
			svgAngle := (90 - cssAngle) * (math.Pi / 180)

			x1f := 50.0
			y1f := 50.0
			x2f := x1f + 50*math.Cos(svgAngle)
			y2f := y1f + 50*math.Sin(svgAngle)

			x1 = fmt.Sprintf("%.2f%%", x1f)
			y1 = fmt.Sprintf("%.2f%%", y1f)
			x2 = fmt.Sprintf("%.2f%%", x2f)
			y2 = fmt.Sprintf("%.2f%%", y2f)
		}
	}

	return x1, y1, x2, y2
}

func RadialGradientToSVG(gradient Gradient) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<radialGradient id="%s">`, gradient.ID))
	sb.WriteString("\n")
	totalStops := len(gradient.ColorStops)
	for i, cs := range gradient.ColorStops {
		offset := cs.Position
		if offset == "" {
			offsetValue := float64(i) / float64(totalStops-1) * 100
			offset = fmt.Sprintf("%.2f%%", offsetValue)
		}
		sb.WriteString(fmt.Sprintf(`<stop offset="%s" stop-color="%s" />`, offset, cs.Color))
		sb.WriteString("\n")
	}
	sb.WriteString(`</radialGradient>`)
	return sb.String()
}

func UniqueGradientID(cssGradient string) string {
	h := sha1.New()
	h.Write([]byte(cssGradient))
	hash := hex.EncodeToString(h.Sum(nil))
	return "grad-" + hash
}

var GradientRegex = regexp.MustCompile(`^(linear|radial)-gradient\((.+)\)$`)

func IsGradient(color string) bool {
	return GradientRegex.MatchString(color)
}

var URLGradientID = regexp.MustCompile(`^url\('#grad-[a-f0-9]{40}'\)$`)

func IsURLGradientID(color string) bool {
	return URLGradientID.MatchString(color)
}
