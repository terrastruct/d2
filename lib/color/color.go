package color

import (
	"regexp"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/mazznoer/csscolorparser"
)

var themeRegex = regexp.MustCompile("^B[1-6]$")

func Darken(colorString string) (string, error) {
	if themeRegex.MatchString(colorString) {
		switch colorString[1] {
		case '1':
			return B1, nil
		case '2':
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
	}

	return DarkenCSS(colorString)
}

func DarkenCSS(colorString string) (string, error) {
	c, err := csscolorparser.Parse(colorString)
	if err != nil {
		return "", err
	}
	h, s, l := colorful.Color{R: c.R, G: c.G, B: c.B}.Hsl()
	// decrease luminance by 10%
	return colorful.Hsl(h, s, l-.1).Clamped().Hex(), nil
}

const (
	N1 = "N1"
	N2 = "N2"
	N3 = "N3"
	N4 = "N4"
	N5 = "N5"
	N6 = "N6"
	N7 = "N7"

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
	AB5 = "AB4"

	// Special
	Empty = ""
	None  = "none"
)
