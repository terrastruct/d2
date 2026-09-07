package layoutgraph

import "github.com/d2lang/d2/lib/geo"

// nonNilEquals compares two optional points by value.
func nonNilEquals(first, second *geo.Point) bool {
	return first != nil && second != nil && *first == *second
}
