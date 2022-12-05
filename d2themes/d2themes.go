// d2themes defines themes to make d2 diagrams pretty
// Color codes: darkest (N1) -> lightest (N7)
package d2themes

type Theme struct {
	ID     int64        `json:"id"`
	Name   string       `json:"name"`
	Colors ColorPalette `json:"colors"`
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
	N5: "#F0F3F9",
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
