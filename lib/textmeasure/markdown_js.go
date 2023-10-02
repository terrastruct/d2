//go:build wasm

package textmeasure

import "oss.terrastruct.com/d2/d2renderers/d2fonts"

func MeasureMarkdown(mdText string, ruler *Ruler, fontFamily *d2fonts.FontFamily, fontSize int) (width, height int, err error) {
	return 0, 0, nil
}

func RenderMarkdown(m string) (string, error) {
	return "", nil
}
