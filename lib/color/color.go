package color

import (
	"github.com/lucasb-eyer/go-colorful"
	"github.com/mazznoer/csscolorparser"
)

func Darken(colorString string) (string, error) {
	c, err := csscolorparser.Parse(colorString)
	if err != nil {
		return "", err
	}
	h, s, l := colorful.Color{R: c.R, G: c.G, B: c.B}.Hsl()
	// decrease luminance by 10%
	return colorful.Hsl(h, s, l-.1).Clamped().Hex(), nil
}
