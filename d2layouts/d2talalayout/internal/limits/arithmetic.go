package limits

import "math"

// CheckedAddUint64 returns a+b and whether the sum fits in uint64.
func CheckedAddUint64(a, b uint64) (uint64, bool) {
	if math.MaxUint64-a < b {
		return 0, false
	}
	return a + b, true
}

// CheckedMulUint64 returns a*b and whether the product fits in uint64.
func CheckedMulUint64(a, b uint64) (uint64, bool) {
	if a != 0 && math.MaxUint64/a < b {
		return 0, false
	}
	return a * b, true
}
