package color

import (
	"fmt"
	"regexp"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/mazznoer/csscolorparser"
)

var themeColorRegex = regexp.MustCompile(`^N[1-7]|B[1-6]|AA[245]|AB[45]$`)

func IsThemeColor(colorString string) bool {
	return themeColorRegex.Match([]byte(colorString))
}

func Darken(colorString string) (string, error) {
	if IsThemeColor(colorString) {
		switch {
		case colorString[0] == 'B':
			switch colorString[1] {
			case '1', '2':
				return B1, nil
			case '3':
				return B2, nil
			case '4':
				return B3, nil
			case '5':
				return B4, nil
			case '6':
				return B5, nil
			}

		case colorString[0:2] == "AA":
			switch colorString[2] {
			case '2', '4':
				return AA2, nil
			case '5':
				return AA4, nil
			}

		case colorString[0:2] == "AB":
			switch colorString[2] {
			case '4':
				return AB4, nil
			case '5':
				return AB5, nil
			}

		case colorString[0] == 'N':
			switch colorString[1] {
			case '1', '2':
				return N1, nil
			case '3':
				return N2, nil
			case '4':
				return N3, nil
			case '5':
				return N4, nil
			case '6':
				return N5, nil
			case '7':
				return N6, nil
			}
		}

		return "", fmt.Errorf("invalid color \"%s\"", colorString)
	}

	return darkenCSS(colorString)
}

func darkenCSS(colorString string) (string, error) {
	c, err := csscolorparser.Parse(colorString)
	if err != nil {
		return "", err
	}
	h, s, l := colorful.Color{R: c.R, G: c.G, B: c.B}.Hsl()
	// decrease luminance by 10%
	return colorful.Hsl(h, s, l-.1).Clamped().Hex(), nil
}

func LuminanceCategory(colorString string) (string, error) {
	l, err := Luminance(colorString)
	if err != nil {
		return "", err
	}

	switch {
	case l >= .88:
		return "bright", nil
	case l >= .55:
		return "normal", nil
	case l >= .30:
		return "dark", nil
	default:
		return "darker", nil
	}
}

func Luminance(colorString string) (float64, error) {
	c, err := csscolorparser.Parse(colorString)
	if err != nil {
		return 0, err
	}

	// https://stackoverflow.com/a/596243
	l := float64(
		float64(0.299)*float64(c.R) +
			float64(0.587)*float64(c.G) +
			float64(0.114)*float64(c.B),
	)
	return l, nil
}

const (
	N1 = "N1" // foreground color
	N2 = "N2"
	N3 = "N3"
	N4 = "N4"
	N5 = "N5"
	N6 = "N6"
	N7 = "N7" // background color

	// Base Colors: used for containers
	B1 = "B1"
	B2 = "B2"
	B3 = "B3"
	B4 = "B4"
	B5 = "B5"
	B6 = "B6"

	// Alternative colors A
	AA2 = "AA2"
	AA4 = "AA4"
	AA5 = "AA5"

	// Alternative colors B
	AB4 = "AB4"
	AB5 = "AB5"

	// Special
	Empty = ""
	None  = "none"
)
