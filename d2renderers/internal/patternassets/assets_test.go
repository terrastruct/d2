package patternassets

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestCanonicalAssetHashes(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		bytes  int
		digest string
	}{
		{name: "paper SVG", data: []byte(PaperSVG()), bytes: 435438, digest: "98b3c110441c6c02cb685c0a7303e7a36c1faee7fa1f65f6424b48ec51123527"},
		{name: "paper Brotli", data: paperBrotli, bytes: 133164, digest: "fe047f4104975b17b033988e49981c42a5aca928da39d149e35f4e31c8fb248a"},
		{name: "grain PNG", data: GrainPNG(), bytes: 20845, digest: "7aec37844fa033cbdb586efaa1955dc6f43d56707c0b1bf91e8b5e32386d8d69"},
		{name: "streak path", data: []byte(StreakPathData()), bytes: 9224, digest: "7b4001e7d043ac395abd015ce0bc2bbe018725a634f6e352b1eb2ff60cc99dea"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := len(test.data); got != test.bytes {
				t.Fatalf("bytes = %d, want %d", got, test.bytes)
			}
			if got := assetDigest(test.data); got != test.digest {
				t.Fatalf("SHA-256 = %s, want %s", got, test.digest)
			}
		})
	}
}

func TestSVGWrappersMatchCanonicalOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		bytes  int
		digest string
	}{
		{name: "grain", output: GrainSVG(), bytes: 28443, digest: "e23dfd58ed7febaf5c2485c087585953c4d42e37cbca621f0bfc46a8381ef075"},
		{name: "streak", output: StreaksSVG(), bytes: 9399, digest: "7f03b8ab83a8373fe3cf773c700642956503c35acefb7ed5dbe9daa98253c597"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := len(test.output); got != test.bytes {
				t.Fatalf("bytes = %d, want %d", got, test.bytes)
			}
			if got := assetDigest([]byte(test.output)); got != test.digest {
				t.Fatalf("SHA-256 = %s, want %s", got, test.digest)
			}
		})
	}
}

func assetDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%x", digest)
}
