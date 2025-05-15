package d2themescatalog

import (
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2themes"
)

var LightCatalog = []d2themes.Theme{
	NeutralDefault,
	NeutralGrey,
	FlagshipTerrastruct,
	CoolClassics,
	MixedBerryBlue,
	GrapeSoda,
	Aubergine,
	ColorblindClear,
	VanillaNitroCola,
	OrangeCreamsicle,
	ShirleyTemple,
	EarthTones,
	EvergladeGreen,
	ButteredToast,
	Terminal,
	TerminalGrayscale,
	Origami,
	C4,
}

var DarkCatalog = []d2themes.Theme{
	DarkMauve,
	DarkFlagshipTerrastruct,
}

func Find(id int64) d2themes.Theme {
	for _, theme := range LightCatalog {
		if theme.ID == id {
			return theme
		}
	}

	for _, theme := range DarkCatalog {
		if theme.ID == id {
			return theme
		}
	}

	return d2themes.Theme{}
}

func CLIString() string {
	var s strings.Builder

	s.WriteString("Light:\n")
	for _, t := range LightCatalog {
		s.WriteString(fmt.Sprintf("- %s: %d\n", t.Name, t.ID))
	}

	s.WriteString("Dark:\n")
	for _, t := range DarkCatalog {
		s.WriteString(fmt.Sprintf("- %s: %d\n", t.Name, t.ID))
	}

	return s.String()
}
