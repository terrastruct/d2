package d2target

import (
	"fmt"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
)

type Class struct {
	Fields  []ClassField  `json:"fields"`
	Methods []ClassMethod `json:"methods"`
}

type ClassField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Visibility string `json:"visibility"`
}

func (cf ClassField) Text() *MText {
	return &MText{
		Text:     fmt.Sprintf("%s%s", cf.Name, cf.Type),
		FontSize: d2fonts.FONT_SIZE_L,
		IsBold:   false,
		IsItalic: false,
		Shape:    "class",
	}
}

type ClassMethod struct {
	Name       string `json:"name"`
	Return     string `json:"return"`
	Visibility string `json:"visibility"`
}

func (cm ClassMethod) Text() *MText {
	return &MText{
		Text:     fmt.Sprintf("%s%s", cm.Name, cm.Return),
		FontSize: d2fonts.FONT_SIZE_L,
		IsBold:   false,
		IsItalic: false,
		Shape:    "class",
	}
}
