package graphjson

import "github.com/d2lang/d2/lib/geo"

func serializeDirection(direction geo.Orientation) string {
	switch direction {
	case geo.Top:
		return "up"
	case geo.Bottom:
		return "down"
	case geo.Left:
		return "left"
	case geo.Right:
		return "right"
	default:
		return ""
	}
}

func deserializeDirection(direction string) geo.Orientation {
	switch direction {
	case "up":
		return geo.Top
	case "down":
		return geo.Bottom
	case "left":
		return geo.Left
	case "right":
		return geo.Right
	default:
		return geo.NONE
	}
}
