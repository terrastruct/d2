//go:build !js || !wasm

package fontface

import (
	"crypto/sha256"
	"fmt"
	"sync"
)

var trustedNotoColorEmojiCOLRv1 struct {
	sync.Once
	data []byte
	face *ParsedFace
	err  error
}

// trustedNotoColorEmojiCOLRv1Mu guards reads from the independently parsed
// validation face retained below. It is never returned to callers, so exported
// go-text COLR/CPAL fields cannot be replaced beneath trusted compilation.
var trustedNotoColorEmojiCOLRv1Mu sync.RWMutex

// RegisterBundledNotoColorEmoji retains a package-private copy and parse of
// D2's exact authenticated Noto Color Emoji resource. Decoding lives in
// d2fonts so the raster kernel's dependency closure contains no
// network-capable codec API. The returned bytes are package-owned and must be
// treated as immutable; d2fonts reuses them as its process cache so only one
// permanent decoded copy is retained.
func RegisterBundledNotoColorEmoji(data []byte) ([]byte, error) {
	if len(data) != 4_991_984 || sha256.Sum256(data) != bundledNotoColorEmojiCOLRv1SHA256 {
		return nil, fmt.Errorf("d2fonts: bundled Noto Color Emoji decoded resource is not authenticated")
	}
	trustedNotoColorEmojiCOLRv1.Do(func() {
		canonical := append([]byte(nil), data...)
		var face *ParsedFace
		collection, parseErr := ParseFaceCollectionWithLimit(canonical, 1)
		switch {
		case parseErr != nil:
			parseErr = fmt.Errorf("d2fonts: parse bundled Noto Color Emoji: %w", parseErr)
		case collection.NumFaces() != 1:
			parseErr = fmt.Errorf("d2fonts: bundled Noto Color Emoji has %d faces, want 1", collection.NumFaces())
		default:
			face, parseErr = collection.Face(0)
			if parseErr != nil {
				parseErr = fmt.Errorf("d2fonts: load bundled Noto Color Emoji: %w", parseErr)
			}
		}
		trustedNotoColorEmojiCOLRv1Mu.Lock()
		if parseErr == nil {
			trustedNotoColorEmojiCOLRv1.data = canonical
			trustedNotoColorEmojiCOLRv1.face = face
		}
		trustedNotoColorEmojiCOLRv1.err = parseErr
		trustedNotoColorEmojiCOLRv1Mu.Unlock()
	})
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	defer trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	return trustedNotoColorEmojiCOLRv1.data, trustedNotoColorEmojiCOLRv1.err
}

func compilePrivateBundledNotoColorEmojiCOLRv1Plan(glyphID uint32, limits trustedCOLRv1Limits) (*COLRv1Plan, bool, error) {
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	defer trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	if trustedNotoColorEmojiCOLRv1.err != nil {
		return nil, false, trustedNotoColorEmojiCOLRv1.err
	}
	if trustedNotoColorEmojiCOLRv1.face == nil {
		return nil, false, fmt.Errorf("d2fonts: bundled Noto Color Emoji is not registered")
	}
	return compileCOLRv1Plan(trustedNotoColorEmojiCOLRv1.face, glyphID, limits)
}
