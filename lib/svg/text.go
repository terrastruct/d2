package svg

import (
	"html"
)

func EscapeText(text string) string {
	return html.EscapeString(text)
}
