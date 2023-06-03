package d2target

import "strings"

const (
	NamePadding   = 10
	TypePadding   = 20
	HeaderPadding = 10

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
	}
}

func (c SQLColumn) ConstraintAbbr() string {
	var abbrs []string

	for _, constraint := range c.Constraint {
		var abbr string

		switch constraint {
		case "primary_key":
			abbr = "PK"
		case "foreign_key":
			abbr = "FK"
		case "unique":
			abbr = "UNQ"
		}

		abbrs = append(abbrs, abbr)
	}

	return strings.Join(abbrs, ", ")
}
