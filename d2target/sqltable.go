package d2target

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
	Name       string `json:"name"`
	Type       Text   `json:"type"`
	Label      Text   `json:"label"`
	Constraint string `json:"constraint"`
	Reference  string `json:"reference"`
}

func (c SQLColumn) Texts(fontSize int) []*MText {
	return []*MText{
		{
			Text:     c.Label.Label,
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
