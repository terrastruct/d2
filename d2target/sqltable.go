package d2target

import "oss.terrastruct.com/d2/d2renderers/d2fonts"

const (
	NamePadding   = 10
	TypePadding   = 20
	HeaderPadding = 20
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

func (c SQLColumn) Texts() []*MText {
	return []*MText{
		{
			Text:     c.Name.Label,
			FontSize: d2fonts.FONT_SIZE_L,
			IsBold:   false,
			IsItalic: false,
			Shape:    "sql_table",
		},
		{
			Text:     c.Type.Label,
			FontSize: d2fonts.FONT_SIZE_L,
			IsBold:   false,
			IsItalic: false,
			Shape:    "sql_table",
		},
	}
}

func (c SQLColumn) ConstraintAbbr() string {
	switch c.Constraint {
	case "primary_key":
		return "PK"
	case "foreign_key":
		return "FK"
	case "unique":
		return "UNQ"
	default:
		return ""
	}
}

func (st *SQLTable) Copy() *SQLTable {
	if st == nil {
		return nil
	}
	return &SQLTable{
		Columns: append([]SQLColumn(nil), st.Columns...),
	}
}
