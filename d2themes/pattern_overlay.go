package d2themes

import (
	"fmt"
)

type PatternOverlay struct {
	el      *ThemableElement
	pattern string
}

func NewPatternOverlay(el *ThemableElement, pattern string) *PatternOverlay {
	return &PatternOverlay{
		el,
		pattern,
	}
}

func (o *PatternOverlay) Render() (string, error) {
	el := o.el.Copy()
	el.Fill = ""
	el.ClassName = fmt.Sprintf("%s-overlay", o.pattern)
	return el.Render(), nil
}
