package d2themescatalog

import "oss.terrastruct.com/d2/d2themes"

var TerminalGrayscale = d2themes.Theme{
	ID:   301,
	Name: "Terminal Grayscale",
	Colors: d2themes.ColorPalette{
		Neutrals: TerminalGrayscaleNeutral,

		B1: "#000410",
		B2: "#000410",
		B3: "#FFFFFF",
		B4: "#E7E9EE",
		B5: "#F5F6F9",
		B6: "#FFFFFF",

		AA2: "#6D7284",
		AA4: "#F5F6F9",
		AA5: "#FFFFFF",

		AB4: "#F5F6F9",
		AB5: "#FFFFFF",
	},
	SpecialRules: d2themes.SpecialRules{
		Mono:                       true,
		NoCornerRadius:             true,
		OuterContainerDoubleBorder: true,
		ContainerDots:              true,
		CapsLock:                   true,
	},
}

var TerminalGrayscaleNeutral = d2themes.Neutral{
	N1: "#000410",
	N2: "#000410",
	N3: "#9499AB",
	N4: "#FFFFFF",
	N5: "#FFFFFF",
	N6: "#EEF1F8",
	N7: "#FFFFFF",
}
