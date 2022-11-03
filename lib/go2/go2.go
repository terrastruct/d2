// Package go2 contains general utility helpers that should've been in Go. Maybe they'll be in Go 2.0.
package go2

import (
	"hash/fnv"
	"math"

	"golang.org/x/exp/constraints"
)

func Pointer[T any](v T) *T {
	return &v
}

func Min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func StringToIntHash(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32())
}

func Contains[T comparable](els []T, el T) bool {
	for _, el2 := range els {
		if el2 == el {
			return true
		}
	}
	return false
}

func Filter[T any](els []T, fn func(T) bool) []T {
	out := []T{}
	for _, el := range els {
		if fn(el) {
			out = append(out, el)
		}
	}
	return out
}

func IntMax(x, y int) int {
	return int(math.Max(float64(x), float64(y)))
}

func IntMin(x, y int) int {
	return int(math.Min(float64(x), float64(y)))
}
