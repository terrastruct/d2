//go:build !js || !wasm

package fontface

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/andybalholm/brotli"
)

const (
	testBundledNotoColorEmojiBrotliSize = 2_354_437
	testBundledNotoColorEmojiSize       = 4_991_984
)

var testBundledNotoColorEmojiBrotliSHA256 = [sha256.Size]byte{
	0x34, 0x2b, 0xef, 0xfe, 0x73, 0xf7, 0xfa, 0x45,
	0x0d, 0x24, 0x86, 0xd3, 0xad, 0x62, 0xab, 0x23,
	0x08, 0xdf, 0x68, 0x19, 0x27, 0x18, 0x96, 0x5b,
	0xaf, 0x23, 0xf5, 0xa8, 0x5e, 0xef, 0x82, 0x47,
}

var testBundledNotoColorEmojiCache struct {
	sync.Once
	compressed []byte
	data       []byte
	err        error
}

func bundledNotoColorEmojiForTest(t testing.TB) []byte {
	t.Helper()
	testBundledNotoColorEmojiCache.Do(func() {
		compressed, err := os.ReadFile("../../d2fonts/encoded/NotoColorEmoji-COLRv1-v2.051.ttf.br")
		if err != nil {
			testBundledNotoColorEmojiCache.err = err
			return
		}
		data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), testBundledNotoColorEmojiSize+1))
		if err != nil {
			testBundledNotoColorEmojiCache.err = err
			return
		}
		if len(data) != testBundledNotoColorEmojiSize || sha256.Sum256(data) != bundledNotoColorEmojiCOLRv1SHA256 {
			testBundledNotoColorEmojiCache.err = &testBundledNotoIntegrityError{}
			return
		}
		testBundledNotoColorEmojiCache.compressed = compressed
		testBundledNotoColorEmojiCache.data = data
	})
	if testBundledNotoColorEmojiCache.err != nil {
		t.Fatal(testBundledNotoColorEmojiCache.err)
	}
	return testBundledNotoColorEmojiCache.data
}

type testBundledNotoIntegrityError struct{}

func (*testBundledNotoIntegrityError) Error() string {
	return "bundled Noto Color Emoji test asset failed integrity checks"
}
