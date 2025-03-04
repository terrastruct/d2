// d2themes defines themes to make d2 diagrams pretty
// Color codes: darkest (N1) -> lightest (N7)
package d2themes

import (
	"oss.terrastruct.com/d2/d2target"
	"oss.terrastruct.com/d2/lib/color"
)

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
	C4                         bool `json:"c4"`

	AllPaper bool `json:"allPaper"`
}

func (t *Theme) IsDark() bool {
	return t.ID >= 200 && t.ID < 300
}

func (t *Theme) ApplyOverrides(overrides *d2target.ThemeOverrides) {
	if overrides == nil {
		return
	}

	if overrides.B1 != nil {
		t.Colors.B1 = *overrides.B1
	}
	if overrides.B2 != nil {
		t.Colors.B2 = *overrides.B2
	}
	if overrides.B3 != nil {
		t.Colors.B3 = *overrides.B3
	}
	if overrides.B4 != nil {
		t.Colors.B4 = *overrides.B4
	}
	if overrides.B5 != nil {
		t.Colors.B5 = *overrides.B5
	}
	if overrides.B5 != nil {
		t.Colors.B5 = *overrides.B5
	}
	if overrides.B6 != nil {
		t.Colors.B6 = *overrides.B6
	}
	if overrides.AA2 != nil {
		t.Colors.AA2 = *overrides.AA2
	}
	if overrides.AA4 != nil {
		t.Colors.AA4 = *overrides.AA4
	}
	if overrides.AA5 != nil {
		t.Colors.AA5 = *overrides.AA5
	}
	if overrides.AB4 != nil {
		t.Colors.AB4 = *overrides.AB4
	}
	if overrides.AB5 != nil {
		t.Colors.AB5 = *overrides.AB5
	}
	if overrides.N1 != nil {
		t.Colors.Neutrals.N1 = *overrides.N1
	}
	if overrides.N2 != nil {
		t.Colors.Neutrals.N2 = *overrides.N2
	}
	if overrides.N3 != nil {
		t.Colors.Neutrals.N3 = *overrides.N3
	}
	if overrides.N4 != nil {
		t.Colors.Neutrals.N4 = *overrides.N4
	}
	if overrides.N5 != nil {
		t.Colors.Neutrals.N5 = *overrides.N5
	}
	if overrides.N6 != nil {
		t.Colors.Neutrals.N6 = *overrides.N6
	}
	if overrides.N7 != nil {
		t.Colors.Neutrals.N7 = *overrides.N7
	}
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
	N4: "#676D7D",
	N5: "#3A3D49",
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
