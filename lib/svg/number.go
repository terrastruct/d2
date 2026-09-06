package svg

import (
	"math"
	"strconv"
	"strings"
)

// FormatFloat serializes an SVG coordinate with the same six decimal places
// as %f, but omits redundant trailing zeros. Keeping the original rounding is
// important: formatting the full float or reducing precision changes geometry.
func FormatFloat(value float64) string {
	// Most laid-out coordinates are integers. Avoid allocating and formatting
	// a fractional part only to remove it again. The upper bound is exclusive
	// because float64(math.MaxInt64) rounds up to 2^63.
	if value >= -0x1p63 && value < 0x1p63 {
		integer := int64(value)
		if value == float64(integer) {
			if integer == 0 && math.Signbit(value) {
				return "-0"
			}
			return strconv.FormatInt(integer, 10)
		}
	}
	s := strconv.FormatFloat(value, 'f', 6, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	return s
}
