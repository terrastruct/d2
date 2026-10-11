package d2scenebuild

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/mazznoer/csscolorparser"

	"github.com/d2lang/d2/d2renderers/d2scene"
	"github.com/d2lang/d2/d2renderers/internal/scenepaint"
	"github.com/d2lang/d2/d2themes"
	libcolor "github.com/d2lang/d2/lib/color"
)

func (b *builder) paint(raw, description string) (d2scene.Paint, error) {
	if raw == "" || strings.EqualFold(raw, libcolor.None) {
		return nil, nil
	}
	if libcolor.IsGradient(raw) {
		paint, err := gradientPaint(raw)
		if err != nil {
			return nil, fmt.Errorf("scene: parse %s gradient %q: %w", description, raw, err)
		}
		return paint, nil
	}
	resolved := d2themes.ResolveThemeColor(b.theme, raw)
	if resolved == "" {
		return nil, fmt.Errorf("scene: %s has unknown theme color %q", description, raw)
	}
	parsed, err := csscolorparser.Parse(resolved)
	if err != nil {
		return nil, fmt.Errorf("scene: parse %s color %q: %w", description, resolved, err)
	}
	r, g, blue, a := parsed.RGBA255()
	return d2scene.SolidPaint{Color: color.NRGBA{R: r, G: g, B: blue, A: a}}, nil
}

func gradientPaint(raw string) (d2scene.Paint, error) { return scenepaint.Gradient(raw) }
