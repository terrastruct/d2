//go:build !js || !wasm

package fontface

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sync"

	gotextfont "github.com/go-text/typesetting/font"
)

var trustedNotoColorEmojiCOLRv1 struct {
	sync.Once
	data   []byte
	source BundledNotoColorEmojiSource
	err    error
}

// trustedNotoColorEmojiCOLRv1Mu guards publication and reads of the registered
// validation face. Its mutable Face caches are never returned; controlled
// renderer clones share only the Font tables that go-text documents as
// read-only and concurrent-safe.
var trustedNotoColorEmojiCOLRv1Mu sync.RWMutex

// RegisterBundledNotoColorEmoji retains a package-private copy and parse of
// D2's exact authenticated Noto Color Emoji resource. Decoding lives in
// d2fonts so the raster kernel's dependency closure contains no
// network-capable codec API. The returned bytes are package-owned and must be
// treated as immutable; d2fonts reuses them as its process cache so only one
// permanent decoded copy is retained.
func RegisterBundledNotoColorEmoji(data []byte) ([]byte, error) {
	if len(data) != bundledNotoColorEmojiCOLRv1Size {
		return nil, fmt.Errorf("d2fonts: bundled Noto Color Emoji decoded resource is not authenticated")
	}
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	canonical := trustedNotoColorEmojiCOLRv1.data
	registeredErr := trustedNotoColorEmojiCOLRv1.err
	trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	if canonical != nil {
		if !sameSlice(data, canonical) && !bytes.Equal(data, canonical) {
			return nil, fmt.Errorf("d2fonts: bundled Noto Color Emoji decoded resource is not authenticated")
		}
		return canonical, registeredErr
	}
	digest := sha256.Sum256(data)
	if digest != bundledNotoColorEmojiCOLRv1SHA256 {
		return nil, fmt.Errorf("d2fonts: bundled Noto Color Emoji decoded resource is not authenticated")
	}
	return registerAuthenticatedBundledNotoColorEmoji(data, digest)
}

func registerAuthenticatedBundledNotoColorEmoji(data []byte, digest [sha256.Size]byte) ([]byte, error) {
	return registerAuthenticatedBundledNotoColorEmojiOwnership(data, digest, false)
}

// RegisterOwnedBundledNotoColorEmoji authenticates and takes ownership of a
// freshly allocated decoded resource. The caller must not retain or mutate
// data after this call. Unlike RegisterBundledNotoColorEmoji, this path avoids
// a complete second decoded-font allocation; it is restricted to D2's private
// loader, which has sole ownership of its decoder output.
func RegisterOwnedBundledNotoColorEmoji(data []byte) ([]byte, error) {
	if len(data) != bundledNotoColorEmojiCOLRv1Size {
		return nil, fmt.Errorf("d2fonts: bundled Noto Color Emoji decoded resource is not authenticated")
	}
	digest := sha256.Sum256(data)
	if digest != bundledNotoColorEmojiCOLRv1SHA256 {
		return nil, fmt.Errorf("d2fonts: bundled Noto Color Emoji decoded resource is not authenticated")
	}
	return registerAuthenticatedBundledNotoColorEmojiOwnership(data, digest, true)
}

func registerAuthenticatedBundledNotoColorEmojiOwnership(data []byte, digest [sha256.Size]byte, owned bool) ([]byte, error) {
	trustedNotoColorEmojiCOLRv1.Do(func() {
		canonical := data
		if !owned {
			canonical = append([]byte(nil), data...)
		}
		var face *ParsedFace
		collection, parseErr := parseFaceCollectionWithLimitUsingParserAndDigest(canonical, 1, gotextfont.ParseTTC, digest)
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
			trustedNotoColorEmojiCOLRv1.source.face = face
		}
		trustedNotoColorEmojiCOLRv1.err = parseErr
		trustedNotoColorEmojiCOLRv1Mu.Unlock()
	})
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	defer trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	return trustedNotoColorEmojiCOLRv1.data, trustedNotoColorEmojiCOLRv1.err
}

// RegisteredBundledNotoColorEmoji authenticates an exact candidate and returns
// a package-owned source for parser-issued clones. The registered Face and its
// mutable caches remain private; clones share only go-text's concurrent-safe,
// read-only Font tables.
// A same-sized unrelated font is not claimed, so callers can continue through
// their ordinary parser path.
func RegisteredBundledNotoColorEmoji(data []byte, faceIndex uint16) (*BundledNotoColorEmojiSource, bool, error) {
	if len(data) != bundledNotoColorEmojiCOLRv1Size {
		return nil, false, nil
	}
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	canonical := trustedNotoColorEmojiCOLRv1.data
	trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	var digest [sha256.Size]byte
	if canonical != nil {
		if !sameSlice(data, canonical) && !bytes.Equal(data, canonical) {
			return nil, false, nil
		}
		digest = bundledNotoColorEmojiCOLRv1SHA256
	} else {
		digest = sha256.Sum256(data)
		if digest != bundledNotoColorEmojiCOLRv1SHA256 {
			return nil, false, nil
		}
	}
	if faceIndex != 0 {
		return nil, true, fmt.Errorf("load font face %d: collection has 1 faces", faceIndex)
	}
	if _, err := registerAuthenticatedBundledNotoColorEmoji(data, digest); err != nil {
		return nil, true, err
	}
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	defer trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	if trustedNotoColorEmojiCOLRv1.err != nil {
		return nil, true, trustedNotoColorEmojiCOLRv1.err
	}
	if trustedNotoColorEmojiCOLRv1.source.face == nil {
		return nil, true, fmt.Errorf("d2fonts: bundled Noto Color Emoji is not registered")
	}
	return &trustedNotoColorEmojiCOLRv1.source, true, nil
}

// RegisteredBundledNotoColorEmojiBackingDigest identifies the exact private
// decoded backing without scanning it. RegisteredBundledNotoColorEmoji still
// authenticates and parses the resource before a clone source is used.
func RegisteredBundledNotoColorEmojiBackingDigest(data []byte) ([sha256.Size]byte, bool) {
	if len(data) != bundledNotoColorEmojiCOLRv1Size {
		return [sha256.Size]byte{}, false
	}
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	defer trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	if trustedNotoColorEmojiCOLRv1.data != nil && sameSlice(data, trustedNotoColorEmojiCOLRv1.data) {
		return bundledNotoColorEmojiCOLRv1SHA256, true
	}
	return [sha256.Size]byte{}, false
}

func compilePrivateBundledNotoColorEmojiCOLRv1Plan(glyphID uint32, limits trustedCOLRv1Limits) (*COLRv1Plan, bool, error) {
	trustedNotoColorEmojiCOLRv1Mu.RLock()
	defer trustedNotoColorEmojiCOLRv1Mu.RUnlock()
	if trustedNotoColorEmojiCOLRv1.err != nil {
		return nil, false, trustedNotoColorEmojiCOLRv1.err
	}
	if trustedNotoColorEmojiCOLRv1.source.face == nil {
		return nil, false, fmt.Errorf("d2fonts: bundled Noto Color Emoji is not registered")
	}
	return compileCOLRv1Plan(trustedNotoColorEmojiCOLRv1.source.face, glyphID, limits)
}
