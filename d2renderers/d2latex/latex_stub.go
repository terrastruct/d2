//go:build !cgo

package d2latex

import "errors"

func Render(s string) (string, error) {
	return "", errors.New("not found in build")
}

func Measure(s string) (width, height int, _ error) {
	return 0, 0, errors.New("not found in build")
}
