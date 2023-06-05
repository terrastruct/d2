package d2target

import "strings"

const (
	NamePadding       = 10
	TypePadding       = 20
	ConstraintPadding = 20
	HeaderPadding     = 10

	// Setting table font size sets it for columns
	// The header needs to be a little larger for visual hierarchy
	HeaderFontAdd = 4
)

type SQLTable struct {
	Columns []SQLColumn `json:"columns"`
}

type SQLColumn struct {
	Name       Text     `json:"name"`
	Type       Text     `json:"type"`
	Constraint []string `json:"constraint"`
	Reference  string   `json:"reference"`
}

func (c SQLColumn) Texts(fontSize int) []*MText {
	return []*MText{
		{
			Text:     c.Name.Label,
			FontSize: fontSize,
			IsBold:   false,
			IsItalic: false,
			Shape:    "sql_table",
		},
		{
			Text:     c.Type.Label,
			FontSize: fontSize,
			IsBold:   false,
			IsItalic: false,
			Shape:    "sql_table",
		},
		{
			Text:     c.ConstraintAbbr(),
			FontSize: fontSize,
			IsBold:   false,
			IsItalic: false,
			Shape:    "sql_table",
		},
	}
}

func (c SQLColumn) ConstraintAbbr() string {
	constraints := make([]string, len(c.Constraint))

	for i, constraint := range c.Constraint {
		switch constraint {
		case "primary_key":
			constraint = "PK"
		case "foreign_key":
			constraint = "FK"
		case "unique":
			constraint = "UNQ"
		}

		constraints[i] = constraint
	}

	return strings.Join(constraints, ", ")
}
