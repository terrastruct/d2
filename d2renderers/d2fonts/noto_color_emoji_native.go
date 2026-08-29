//go:build !js || !wasm

package d2fonts

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"io"
	"sync"

	"github.com/andybalholm/brotli"

	"github.com/d2lang/d2/d2renderers/internal/fontface"
)

const (
	bundledNotoColorEmojiBrotliSize = 2_354_437
	bundledNotoColorEmojiSize       = 4_991_984
)

var bundledNotoColorEmojiBrotliSHA256 = [sha256.Size]byte{
	0x34, 0x2b, 0xef, 0xfe, 0x73, 0xf7, 0xfa, 0x45,
	0x0d, 0x24, 0x86, 0xd3, 0xad, 0x62, 0xab, 0x23,
	0x08, 0xdf, 0x68, 0x19, 0x27, 0x18, 0x96, 0x5b,
	0xaf, 0x23, 0xf5, 0xa8, 0x5e, 0xef, 0x82, 0x47,
}

var bundledNotoColorEmojiSHA256 = [sha256.Size]byte{
	0x0a, 0xe5, 0x7f, 0xe5, 0x86, 0x45, 0x63, 0x85,
	0x23, 0xba, 0x35, 0xf3, 0x88, 0xd9, 0x37, 0x39,
	0xd2, 0x92, 0x53, 0x9a, 0x9a, 0xcb, 0x84, 0xdf,
	0x57, 0x00, 0xc8, 0x1b, 0x1e, 0x1a, 0x28, 0xd2,
}

//go:embed encoded/NotoColorEmoji-COLRv1-v2.051.ttf.br
var bundledNotoColorEmojiBrotli []byte

var bundledNotoColorEmojiCache struct {
	sync.Once
	data []byte
	err  error
}

func bundledNotoColorEmoji() ([]byte, error) {
	bundledNotoColorEmojiCache.Do(func() {
		data, err := decodeBundledBrotliFont(
			bundledNotoColorEmojiBrotli,
			bundledNotoColorEmojiBrotliSize,
			bundledNotoColorEmojiBrotliSHA256,
			bundledNotoColorEmojiSize,
			bundledNotoColorEmojiSHA256,
		)
		if err != nil {
			bundledNotoColorEmojiCache.err = fmt.Errorf("d2fonts: decode bundled Noto Color Emoji: %w", err)
			return
		}
		canonical, err := fontface.RegisterBundledNotoColorEmoji(data)
		if err != nil {
			bundledNotoColorEmojiCache.err = err
			return
		}
		bundledNotoColorEmojiCache.data = canonical
	})
	return bundledNotoColorEmojiCache.data, bundledNotoColorEmojiCache.err
}

func decodeBundledBrotliFont(compressed []byte, expectedCompressedSize int, expectedCompressedSHA256 [sha256.Size]byte, expectedSize int, expectedSHA256 [sha256.Size]byte) ([]byte, error) {
	if expectedCompressedSize <= 0 || expectedSize <= 0 {
		return nil, fmt.Errorf("invalid compressed/decoded size limits %d/%d", expectedCompressedSize, expectedSize)
	}
	if len(compressed) != expectedCompressedSize {
		return nil, fmt.Errorf("Brotli font size is %d, want %d", len(compressed), expectedCompressedSize)
	}
	compressedDigest := sha256.Sum256(compressed)
	if compressedDigest != expectedCompressedSHA256 {
		return nil, fmt.Errorf("Brotli font SHA-256 is %x, want %x", compressedDigest, expectedCompressedSHA256)
	}
	data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), int64(expectedSize)+1))
	if err != nil {
		return nil, fmt.Errorf("read Brotli stream: %w", err)
	}
	if len(data) > expectedSize {
		return nil, fmt.Errorf("decoded font exceeds limit %d", expectedSize)
	}
	if len(data) != expectedSize {
		return nil, fmt.Errorf("decoded font size is %d, want %d", len(data), expectedSize)
	}
	digest := sha256.Sum256(data)
	if digest != expectedSHA256 {
		return nil, fmt.Errorf("decoded font SHA-256 is %x, want %x", digest, expectedSHA256)
	}
	return data, nil
}
