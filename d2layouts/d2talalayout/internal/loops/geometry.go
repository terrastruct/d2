package loops

import "github.com/d2lang/d2/lib/geo"

func nonNilEquals(first, second *geo.Point) bool {
	return first != nil && second != nil && *first == *second
}
