/*
 * The following code is part of https://github.com/jung-kurt/gofpdf
 *
 * Copyright (c) 2019 Arteom Korotkiy (Gmail: arteomkorotkiy)
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package font

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// Frozen from 0515f60e985ad829d652456e07831170583011a0. The original
// decoder eagerly produced two offset arrays; retain that independent oracle.
func legacyParseLOCA(utf *utf8FontFile, format, numSymbols int) []int {
	start := utf.SeekTable("loca")
	positions := make([]int, 0)
	if format == 0 {
		data := utf.getRange(start, numSymbols*2+2)
		arr := legacyUnpackOffsets(data, 2)
		for n := 0; n <= numSymbols; n++ {
			positions = append(positions, arr[n+1]*2)
		}
	} else if format == 1 {
		data := utf.getRange(start, numSymbols*4+4)
		arr := legacyUnpackOffsets(data, 4)
		for n := 0; n <= numSymbols; n++ {
			positions = append(positions, arr[n+1])
		}
	} else {
		fmt.Printf("Unknown loca table format %d\n", format)
	}
	return positions
}
func legacyUnpackOffsets(data []byte, width int) []int {
	answer := make([]int, 1)
	reader := bytes.NewReader(data)
	buf := make([]byte, width)
	count, err := reader.Read(buf)
	for err == nil && count > 0 {
		if width == 2 {
			answer = append(answer, int(binary.BigEndian.Uint16(buf)))
		} else {
			answer = append(answer, int(binary.BigEndian.Uint32(buf)))
		}
		count, err = reader.Read(buf)
	}
	return answer
}

func locaFixture(data []byte, start int) *utf8FontFile {
	return &utf8FontFile{fileReader: &fileReader{array: data}, tableDescriptions: map[string]*tableDescription{"loca": {position: start}}}
}
func recoverLOCA(operation func()) (message string) {
	defer func() {
		if value := recover(); value != nil {
			message = fmt.Sprint(value)
		}
	}()
	operation()
	return ""
}

func TestLOCAOffsetsMatchLegacy(t *testing.T) {
	random := rand.New(rand.NewSource(532))
	for _, format := range []int{0, 1} {
		for _, count := range []int{0, 1, 127, 2048, 65535} {
			width := 2 * (format + 1)
			data := make([]byte, 7+(count+1)*width)
			random.Read(data)
			old := locaFixture(bytes.Clone(data), 7)
			current := locaFixture(bytes.Clone(data), 7)
			expected := legacyParseLOCA(old, format, count)
			current.parseLOCATable(format, count)
			if old.fileReader.readerPosition != current.fileReader.readerPosition {
				t.Fatal("reader position changed")
			}
			for i, want := range expected {
				if got := current.symbolOffsets.at(i); got != want {
					t.Fatalf("format=%d count=%d index=%d got=%d want=%d", format, count, i, got, want)
				}
			}
			// Existing malformed-font behavior checks glyph indexes against the number
			// of decoded offsets, not the byte length of the source table.
			for _, index := range []int{-1, count + 1, count + 2, int(^uint(0) >> 1)} {
				want := recoverLOCA(func() { _ = expected[index] })
				got := recoverLOCA(func() { _ = current.symbolOffsets.at(index) })
				if got != want {
					t.Fatalf("invalid index=%d panic=%q want=%q", index, got, want)
				}
			}
			// A later font operation can overwrite input bytes; decoded offsets were
			// snapshots and must not start observing those writes after this change.
			clear(current.fileReader.array)
			for i, want := range expected {
				if got := current.symbolOffsets.at(i); got != want {
					t.Fatalf("snapshot changed at index %d", i)
				}
			}
		}
	}
}

func TestLOCATruncationMatchesLegacy(t *testing.T) {
	for _, format := range []int{0, 1} {
		const count = 8
		fullLength := 7 + (count+1)*2*(format+1)
		for length := 0; length <= fullLength; length++ {
			data := make([]byte, length)
			old := locaFixture(bytes.Clone(data), 7)
			current := locaFixture(bytes.Clone(data), 7)
			want := recoverLOCA(func() { legacyParseLOCA(old, format, count) })
			got := recoverLOCA(func() { current.parseLOCATable(format, count) })
			if got != want || old.fileReader.readerPosition != current.fileReader.readerPosition {
				t.Fatalf("format=%d length=%d panic=%q/%q position=%d/%d", format, length, got, want, current.fileReader.readerPosition, old.fileReader.readerPosition)
			}
		}
	}
	for _, format := range []int{-1, 2, 65535} {
		old, current := locaFixture(make([]byte, 20), 7), locaFixture(make([]byte, 20), 7)
		expected := legacyParseLOCA(old, format, 3)
		current.parseLOCATable(format, 3)
		want := recoverLOCA(func() { _ = expected[0] })
		got := recoverLOCA(func() { _ = current.symbolOffsets.at(0) })
		if got != want || old.fileReader.readerPosition != current.fileReader.readerPosition {
			t.Fatal("unknown format behavior changed")
		}
	}
}

// The fingerprints were generated with both subsetFont.go and font.go frozen
// at 0515f60e985ad829d652456e07831170583011a0, before these optimizations.
func TestSubsetFontFingerprints(t *testing.T) {
	var cases []struct {
		Font         string `json:"font"`
		Cutset       string `json:"cutset"`
		FontSHA256   string `json:"font_sha256"`
		SubsetSHA256 string `json:"subset_sha256"`
		InputSHA256  string `json:"input_sha256"`
		WoffSHA256   string `json:"woff_sha256"`
	}
	if err := json.Unmarshal([]byte(subsetFingerprintJSON), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 60 {
		t.Fatalf("got %d fingerprints, want 60", len(cases))
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("%s/%d", tc.Font, i), func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("../../d2renderers/d2fonts/ttf", tc.Font))
			if err != nil {
				t.Fatal(err)
			}
			if subsetDigest(input) != tc.FontSHA256 {
				t.Fatal("source font changed")
			}
			subset := UTF8CutFont(input, tc.Cutset)
			if subsetDigest(subset) != tc.SubsetSHA256 {
				t.Fatal("subset bytes changed")
			}
			if subsetDigest(input) != tc.InputSHA256 {
				t.Fatal("input mutation changed")
			}
			woff, err := Sfnt2Woff(subset)
			if err != nil {
				t.Fatal(err)
			}
			if subsetDigest(woff) != tc.WoffSHA256 {
				t.Fatal("WOFF bytes changed")
			}
		})
	}
}
func subsetDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func BenchmarkLOCAOffsets(b *testing.B) {
	for _, count := range []int{2048, 16384, 65535} {
		data := make([]byte, (count+1)*4)
		for i := 0; i <= count; i++ {
			binary.BigEndian.PutUint32(data[i*4:], uint32(i*12))
		}
		for _, legacy := range []bool{true, false} {
			b.Run(fmt.Sprintf("glyphs=%d/legacy=%v", count, legacy), func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					font := locaFixture(data, 0)
					total := 0
					if legacy {
						positions := legacyParseLOCA(font, 1, count)
						for i := 0; i < 64; i++ {
							total += positions[i*count/64]
						}
					} else {
						font.parseLOCATable(1, count)
						for i := 0; i < 64; i++ {
							total += font.symbolOffsets.at(i * count / 64)
						}
					}
					if total == 0 {
						b.Fatal("offsets not read")
					}
				}
			})
		}
	}
}
