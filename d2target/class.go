package d2target

import (
	"fmt"
)

const (
	PrefixPadding = 10
	PrefixWidth   = 20
	CenterPadding = 50
	// 10px of padding top and bottom so text doesn't look squished
	VerticalPadding = 20
)

type Class struct {
	Fields  []ClassField  `json:"fields"`
	Methods []ClassMethod `json:"methods"`
}

type ClassField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Visibility string `json:"visibility"`
	Underline  bool   `json:"underline"`
}

func (cf ClassField) Text(fontSize int) *MText {
	return &MText{
		Text:        fmt.Sprintf("%s%s", cf.Name, cf.Type),
		FontSize:    fontSize,
		IsBold:      false,
		IsItalic:    false,
		IsUnderline: cf.Underline,
		Shape:       "class",
	}
}

func (cf ClassField) VisibilityToken() string {
	switch cf.Visibility {
	case "protected":
		return "#"
	case "private":
		return "-"
	default:
		return "+"
	}
}

type ClassMethod struct {
	Name       string `json:"name"`
	Return     string `json:"return"`
	Visibility string `json:"visibility"`
	Underline  bool   `json:"underline"`
}

func (cm ClassMethod) Text(fontSize int) *MText {
	return &MText{
		Text:        fmt.Sprintf("%s%s", cm.Name, cm.Return),
		FontSize:    fontSize,
		IsBold:      false,
		IsItalic:    false,
		IsUnderline: cm.Underline,
		Shape:       "class",
	}
}

func (cm ClassMethod) VisibilityToken() string {
	switch cm.Visibility {
	case "protected":
		return "#"
	case "private":
		return "-"
	default:
		return "+"
	}
}
