//go:build js && wasm

package d2fonts

// The wasm host supplies resolved font data, so this build omits the bundled
// Noto Color Emoji payload.
func bundledNotoColorEmoji() ([]byte, error) {
	return nil, nil
}
