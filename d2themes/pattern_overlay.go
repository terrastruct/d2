package d2themes

import (
	"fmt"
)

// PatternOverlay renders a legacy pattern overlay for a themable element.
//
// Deprecated: PatternOverlay is no longer used by D2 and will be removed after
// one compatibility release. There is no replacement.
type PatternOverlay struct {
	el      *ThemableElement
	pattern string
}

// NewPatternOverlay returns a legacy pattern overlay.
//
// Deprecated: PatternOverlay is no longer used by D2 and will be removed after
// one compatibility release. There is no replacement.
func NewPatternOverlay(el *ThemableElement, pattern string) *PatternOverlay {
	return &PatternOverlay{
		el,
		pattern,
	}
}

// Render renders the legacy pattern overlay.
//
// Deprecated: PatternOverlay is no longer used by D2 and will be removed after
// one compatibility release. There is no replacement.
func (o *PatternOverlay) Render() (string, error) {
	el := o.el.Copy()
	el.Fill = ""
	el.ClassName = fmt.Sprintf("%s-overlay", o.pattern)
	return el.Render(), nil
}
