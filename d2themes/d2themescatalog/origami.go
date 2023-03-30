package d2themescatalog

import "oss.terrastruct.com/d2/d2themes"

var Origami = d2themes.Theme{
	ID:   302,
	Name: "Origami",
	Colors: d2themes.ColorPalette{
		Neutrals: OrigamiNeutral,

		B1: "#170206",
		B2: "#A62543",
		B3: "#E07088",
		B4: "#F3E0D2",
		B5: "#FAF1E6",
		B6: "#FFFBF8",

		AA2: "#0A4EA6",
		AA4: "#3182CD",
		AA5: "#68A8E4",

		AB4: "#E07088",
		AB5: "#F19CAE",
	},
	SpecialRules: d2themes.SpecialRules{
		NoCornerRadius:             true,
		OuterContainerDoubleBorder: true,
		AllPaper:                   true,
	},
}

var OrigamiNeutral = d2themes.Neutral{
	N1: "#170206",
	N2: "#6F0019",
	N3: "#FFFFFF",
	N4: "#E07088",
	N5: "#D2B098",
	N6: "#FFFFFF",
	N7: "#FFFFFF",
}
