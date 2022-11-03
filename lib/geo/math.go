package geo

import "math"

func EuclideanDistance(x1, y1, x2, y2 float64) float64 {
	if x1 == x2 {
		return math.Abs(y1 - y2)
	} else if y1 == y2 {
		return math.Abs(x1 - x2)
	} else {
		return math.Sqrt((x1-x2)*(x1-x2) + (y1-y2)*(y1-y2))
	}
}

// compare a and b and consider them equal if
// difference is less than precision e (e.g. e=0.001)
func PrecisionCompare(a, b, e float64) int {
	if math.Abs(a-b) < e {
		return 0
	}
	if a < b {
		return -1
	}
	return 1
}

// TruncateDecimals truncates floats to keep up to 3 digits after decimal, to avoid issues with floats on different machines.
// Since we're not launching rockets, 3 decimals is enough precision for what we're doing
func TruncateDecimals(v float64) float64 {
	return float64(int(v*1000)) / 1000
}

func Sign(i float64) int {
	if i < 0 {
		return -1
	}
	if i > 0 {
		return 1
	}
	return 0
}
