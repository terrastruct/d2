//go:build js && wasm

package fontface

import "crypto/sha256"

// The wasm host supplies resolved font data, so this build omits the private
// bundled Noto Color Emoji validation face.
func registerAuthenticatedBundledNotoColorEmoji(data []byte, _ [sha256.Size]byte) ([]byte, error) {
	return data, nil
}

func RegisterOwnedBundledNotoColorEmoji(data []byte) ([]byte, error) { return data, nil }

func RegisteredBundledNotoColorEmoji(_ []byte, _ uint16) (*BundledNotoColorEmojiSource, bool, error) {
	return nil, false, nil
}

func RegisteredBundledNotoColorEmojiBackingDigest(_ []byte) ([sha256.Size]byte, bool) {
	return [sha256.Size]byte{}, false
}

func compilePrivateBundledNotoColorEmojiCOLRv1Plan(_ uint32, _ trustedCOLRv1Limits) (*COLRv1Plan, bool, error) {
	return nil, false, nil
}
