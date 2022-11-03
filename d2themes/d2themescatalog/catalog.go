package d2themescatalog

import (
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2themes"
)

var Catalog = []d2themes.Theme{
	NeutralDefault,
	NeutralGrey,
	FlagshipTerrastruct,
	MixedBerryBlue,
	CoolClassics,
	GrapeSoda,
	Aubergine,
	ColorblindClear,
	VanillaNitroCola,
	OrangeCreamsicle,
	ShirleyTemple,
	EarthTones,
}

func Find(id int64) d2themes.Theme {
	for _, theme := range Catalog {
		if theme.ID == id {
			return theme
		}
	}

	return d2themes.Theme{}
}

func CLIString() string {
	var s strings.Builder
	for _, t := range Catalog {
		s.WriteString(fmt.Sprintf("- %s: %d\n", t.Name, t.ID))
	}
	return s.String()
}
