package d2target

const (
	NamePadding = 10
	TypePadding = 20
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
			FontSize: c.Name.FontSize,
			IsBold:   false,
			IsItalic: false,
			Shape:    "sql_table",
		},
		{
			Text:     c.Type.Label,
			FontSize: c.Type.FontSize,
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
