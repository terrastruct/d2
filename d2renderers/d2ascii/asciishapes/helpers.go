package asciishapes

import "math"

func absInt(a int) int {
	return int(math.Abs(float64(a)))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}