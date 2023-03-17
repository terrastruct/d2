package d2themescatalog

import "oss.terrastruct.com/d2/d2themes"

var Terminal = d2themes.Theme{
	ID:   300,
	Name: "Terminal",
	Colors: d2themes.ColorPalette{
		Neutrals: TerminalNeutral,

		B1: "#000410",
		B2: "#0000E4",
		B3: "#5AA4DC",
		B4: "#E7E9EE",
		B5: "#F5F6F9",
		B6: "#FFFFFF",

		AA2: "#008566",
		AA4: "#45BBA5",
		AA5: "#7ACCBD",

		AB4: "#F1C759",
		AB5: "#F9E088",
	},
	SpecialRules: d2themes.SpecialRules{
		Mono:                       true,
		NoCornerRadius:             true,
		OuterContainerDoubleBorder: true,
		ContainerDots:              true,
		CapsLock:                   true,
	},
}

var TerminalNeutral = d2themes.Neutral{
	N1: "#000410",
	N2: "#0000B8",
	N3: "#9499AB",
	N4: "#CFD2DD",
	N5: "#C3DEF3",
	N6: "#EEF1F8",
	N7: "#FFFFFF",
}
