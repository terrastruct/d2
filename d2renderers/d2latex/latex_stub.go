//go:build wasm

package d2latex

func Render(s string) (_ string, err error) {
	return "", nil
}

func Measure(s string) (width, height int, err error) {
	return
}
