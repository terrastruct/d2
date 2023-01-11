package style

import (
	"fmt"

	"oss.terrastruct.com/d2/lib/color"
)

type ThemableSketchOverlay struct {
	el   *ThemableElement
	fill string
}

func NewThemableSketchOverlay(el *ThemableElement, fill string) *ThemableSketchOverlay {
	return &ThemableSketchOverlay{
		el,
		fill,
	}
}

// WARNING: Do not reuse the element afterwards as this function changes the Class propery
func (o *ThemableSketchOverlay) Render() (string, error) {
	if color.IsThemeColor(o.fill) {
		o.el.Class += fmt.Sprintf(" sketch-overlay-%s", o.fill) // e.g. sketch-overlay-B3
	} else {
		lc, err := color.LuminanceCategory(o.fill)
		if err != nil {
			return "", err
		}
		o.el.Class += fmt.Sprintf(" sketch-overlay-%s", lc) // e.g. sketch-overlay-dark
	}

	return o.el.Render(), nil
}
