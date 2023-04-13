// d2themes defines themes to make d2 diagrams pretty
// Color codes: darkest (N1) -> lightest (N7)
package d2themes

import "oss.terrastruct.com/d2/lib/color"

type Theme struct {
	ID     int64        `json:"id"`
	Name   string       `json:"name"`
	Colors ColorPalette `json:"colors"`

	SpecialRules SpecialRules `json:"specialRules,omitempty"`
}

type SpecialRules struct {
	Mono                       bool `json:"mono"`
	NoCornerRadius             bool `json:"noCornerRadius"`
	OuterContainerDoubleBorder bool `json:"outerContainerDoubleBorder"`
	ContainerDots              bool `json:"containerDots"`
	CapsLock                   bool `json:"capsLock"`

	AllPaper bool `json:"allPaper"`
}

func (t *Theme) IsDark() bool {
	return t.ID >= 200 && t.ID < 300
}

type Neutral struct {
	N1 string `json:"n1"`
	N2 string `json:"n2"`
	N3 string `json:"n3"`
	N4 string `json:"n4"`
	N5 string `json:"n5"`
	N6 string `json:"n6"`
	N7 string `json:"n7"`
}

type ColorPalette struct {
	Neutrals Neutral `json:"neutrals"`

	// Base Colors: used for containers
	B1 string `json:"b1"`
	B2 string `json:"b2"`
	B3 string `json:"b3"`
	B4 string `json:"b4"`
	B5 string `json:"b5"`
	B6 string `json:"b6"`

	// Alternative colors A
	AA2 string `json:"aa2"`
	AA4 string `json:"aa4"`
	AA5 string `json:"aa5"`

	// Alternative colors B
	AB4 string `json:"ab4"`
	AB5 string `json:"ab5"`
}

var CoolNeutral = Neutral{
	N1: "#0A0F25",
	N2: "#676C7E",
	N3: "#9499AB",
	N4: "#CFD2DD",
	N5: "#DEE1EB",
	N6: "#EEF1F8",
	N7: "#FFFFFF",
}

var WarmNeutral = Neutral{
	N1: "#170206",
	N2: "#535152",
	N3: "#787777",
	N4: "#CCCACA",
	N5: "#DFDCDC",
	N6: "#ECEBEB",
	N7: "#FFFFFF",
}

var DarkNeutral = Neutral{
	N1: "#F4F6FA",
	N2: "#BBBEC9",
	N3: "#868A96",
	N4: "#3A3D49",
	N5: "#676D7D",
	N6: "#191C28",
	N7: "#000410",
}

var DarkMauveNeutral = Neutral{
	N1: "#CDD6F4",
	N2: "#BAC2DE",
	N3: "#A6ADC8",
	N4: "#585B70",
	N5: "#45475A",
	N6: "#313244",
	N7: "#1E1E2E",
}

func ResolveThemeColor(theme Theme, code string) string {
	if !color.IsThemeColor(code) {
		return code
	}
	switch code {
	case "N1":
		return theme.Colors.Neutrals.N1
	case "N2":
		return theme.Colors.Neutrals.N2
	case "N3":
		return theme.Colors.Neutrals.N3
	case "N4":
		return theme.Colors.Neutrals.N4
	case "N5":
		return theme.Colors.Neutrals.N5
	case "N6":
		return theme.Colors.Neutrals.N6
	case "N7":
		return theme.Colors.Neutrals.N7
	case "B1":
		return theme.Colors.B1
	case "B2":
		return theme.Colors.B2
	case "B3":
		return theme.Colors.B3
	case "B4":
		return theme.Colors.B4
	case "B5":
		return theme.Colors.B5
	case "B6":
		return theme.Colors.B6
	case "AA2":
		return theme.Colors.AA2
	case "AA4":
		return theme.Colors.AA4
	case "AA5":
		return theme.Colors.AA5
	case "AB4":
		return theme.Colors.AB4
	case "AB5":
		return theme.Colors.AB5
	default:
		return ""
	}
}
