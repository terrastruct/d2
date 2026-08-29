//go:build js && wasm

package fontface

// The wasm host supplies resolved font data, so this build omits the private
// bundled Noto Color Emoji validation face.
func RegisterBundledNotoColorEmoji(data []byte) ([]byte, error) { return data, nil }

func compilePrivateBundledNotoColorEmojiCOLRv1Plan(_ uint32, _ trustedCOLRv1Limits) (*COLRv1Plan, bool, error) {
	return nil, false, nil
}
