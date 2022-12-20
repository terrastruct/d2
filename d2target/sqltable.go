package d2target

import (
	"fmt"

	"oss.terrastruct.com/d2/d2renderers/d2fonts"
)

type SQLTable struct {
	Columns []SQLColumn `json:"columns"`
}

type SQLColumn struct {
	Name       Text   `json:"name"`
	Type       Text   `json:"type"`
	Constraint string `json:"constraint"`
	Reference  string `json:"reference"`
}

func (c SQLColumn) Text() *MText {
	return &MText{
		Text:     fmt.Sprintf("%s%s%s%s", c.Name.Label, c.Type.Label, c.Constraint, c.Reference),
		FontSize: d2fonts.FONT_SIZE_L,
		IsBold:   false,
		IsItalic: false,
		Shape:    "sql_table",
	}
}
