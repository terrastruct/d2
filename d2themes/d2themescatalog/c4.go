package d2themescatalog

import "oss.terrastruct.com/d2/d2themes"

var C4 = d2themes.Theme{
	ID:   303,
	Name: "C4",
	Colors: d2themes.ColorPalette{
		Neutrals: d2themes.Neutral{
			N1: "#0f5eaa", // Container font color
			N2: "#707070", // Connection font color
			N3: "#FFFFFF",
			N4: "#073b6f", // Person stroke
			N5: "#999999", // Root level objects
			N6: "#FFFFFF",
			N7: "#FFFFFF",
		},

		// Primary colors
		B1: "#073b6f", // Person stroke
		B2: "#08427b", // Person fill
		B3: "#3c7fc0", // Inner objects stroke
		B4: "#438dd5", // Inner objects fill
		B5: "#8a8a8a", // Root level objects stroke
		B6: "#999999", // Root level objects fill

		// Accent colors
		AA2: "#0f5eaa", // Container stroke
		AA4: "#707070", // Connection stroke
		AA5: "#f5f5f5", // Light background

		AB4: "#e1e1e1",
		AB5: "#f0f0f0",
	},
	SpecialRules: d2themes.SpecialRules{
		C4: true,
	},
}
