package d2themes

import (
	"fmt"
	"math"

	"oss.terrastruct.com/d2/lib/color"
)

// ThemableElement is a helper class for creating new XML elements.
// This should be preferred over formatting and must be used
// whenever Fill, Stroke, BackgroundColor or Color contains a color from a theme.
// i.e. N[1-7] | B[1-6] | AA[245] | AB[45]
type ThemableElement struct {
	tag string

	X      float64
	X1     float64
	X2     float64
	Y      float64
	Y1     float64
	Y2     float64
	Width  float64
	Height float64
	R      float64
	Rx     float64
	Ry     float64
	Cx     float64
	Cy     float64

	D         string
	Mask      string
	Points    string
	Transform string
	Href      string
	Xmlns     string

	Fill            string
	Stroke          string
	BackgroundColor string
	Color           string

	ClassName  string
	Style      string
	Attributes string

	Content string
}

func NewThemableElement(tag string) *ThemableElement {
	xmlns := ""
	if tag == "div" {
		xmlns = "http://www.w3.org/1999/xhtml"
	}

	return &ThemableElement{
		tag,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		math.MaxFloat64,
		"",
		"",
		"",
		"",
		"",
		xmlns,
		color.Empty,
		color.Empty,
		color.Empty,
		color.Empty,
		"",
		"",
		"",
		"",
	}
}

func (el *ThemableElement) SetTranslate(x, y float64) {
	el.Transform = fmt.Sprintf("translate(%f %f)", x, y)
}

func (el *ThemableElement) SetMaskUrl(url string) {
	el.Mask = fmt.Sprintf("url(#%s)", url)
}

func (el *ThemableElement) Render() string {
	out := "<" + el.tag

	if len(el.Href) > 0 {
		out += fmt.Sprintf(` href="%s"`, el.Href)
	}
	if el.X != math.MaxFloat64 {
		out += fmt.Sprintf(` x="%f"`, el.X)
	}
	if el.X1 != math.MaxFloat64 {
		out += fmt.Sprintf(` x1="%f"`, el.X1)
	}
	if el.X2 != math.MaxFloat64 {
		out += fmt.Sprintf(` x2="%f"`, el.X2)
	}
	if el.Y != math.MaxFloat64 {
		out += fmt.Sprintf(` y="%f"`, el.Y)
	}
	if el.Y1 != math.MaxFloat64 {
		out += fmt.Sprintf(` y1="%f"`, el.Y1)
	}
	if el.Y2 != math.MaxFloat64 {
		out += fmt.Sprintf(` y2="%f"`, el.Y2)
	}
	if el.Width != math.MaxFloat64 {
		out += fmt.Sprintf(` width="%f"`, el.Width)
	}
	if el.Height != math.MaxFloat64 {
		out += fmt.Sprintf(` height="%f"`, el.Height)
	}
	if el.R != math.MaxFloat64 {
		out += fmt.Sprintf(` r="%f"`, el.R)
	}
	if el.Rx != math.MaxFloat64 {
		out += fmt.Sprintf(` rx="%f"`, el.Rx)
	}
	if el.Ry != math.MaxFloat64 {
		out += fmt.Sprintf(` ry="%f"`, el.Ry)
	}
	if el.Cx != math.MaxFloat64 {
		out += fmt.Sprintf(` cx="%f"`, el.Cx)
	}
	if el.Cy != math.MaxFloat64 {
		out += fmt.Sprintf(` cy="%f"`, el.Cy)
	}

	if len(el.D) > 0 {
		out += fmt.Sprintf(` d="%s"`, el.D)
	}
	if len(el.Mask) > 0 {
		out += fmt.Sprintf(` mask="%s"`, el.Mask)
	}
	if len(el.Points) > 0 {
		out += fmt.Sprintf(` points="%s"`, el.Points)
	}
	if len(el.Transform) > 0 {
		out += fmt.Sprintf(` transform="%s"`, el.Transform)
	}
	if len(el.Xmlns) > 0 {
		out += fmt.Sprintf(` xmlns="%s"`, el.Xmlns)
	}

	class := el.ClassName
	style := el.Style

	// Add class {property}-{theme color} if the color is from a theme, set the property otherwise
	if color.IsThemeColor(el.Stroke) {
		class += fmt.Sprintf(" stroke-%s", el.Stroke)
	} else if len(el.Stroke) > 0 {
		out += fmt.Sprintf(` stroke="%s"`, el.Stroke)
	}
	if color.IsThemeColor(el.Fill) {
		class += fmt.Sprintf(" fill-%s", el.Fill)
	} else if len(el.Fill) > 0 {
		out += fmt.Sprintf(` fill="%s"`, el.Fill)
	}
	if color.IsThemeColor(el.BackgroundColor) {
		class += fmt.Sprintf(" background-color-%s", el.BackgroundColor)
	} else if len(el.BackgroundColor) > 0 {
		out += fmt.Sprintf(` background-color="%s"`, el.BackgroundColor)
	}
	if color.IsThemeColor(el.Color) {
		class += fmt.Sprintf(" color-%s", el.Color)
	} else if len(el.Color) > 0 {
		out += fmt.Sprintf(` color="%s"`, el.Color)
	}

	if len(class) > 0 {
		out += fmt.Sprintf(` class="%s"`, class)
	}
	if len(style) > 0 {
		out += fmt.Sprintf(` style="%s"`, style)
	}
	if len(el.Attributes) > 0 {
		out += fmt.Sprintf(` %s`, el.Attributes)
	}

	if len(el.Content) > 0 {
		return fmt.Sprintf("%s>%s</%s>", out, el.Content, el.tag)
	}
	return out + " />"
}
