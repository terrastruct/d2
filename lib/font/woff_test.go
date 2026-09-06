package font

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestSfnt2WoffMatchesLegacy(t *testing.T) {
	paths, err := filepath.Glob("../../d2renderers/d2fonts/ttf/*.ttf")
	if err != nil || len(paths) == 0 {
		t.Fatalf("find bundled fonts: %v (%d files)", err, len(paths))
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			face, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			checkWoffLegacy(t, face)
			for _, corpus := range []string{"", "D2 diagrams 0123 -> []", "Àéß Ω Ж 中 😀 e\u0301"} {
				checkWoffLegacy(t, UTF8CutFont(bytes.Clone(face), corpus))
			}
		})
	}
}

func TestSfnt2WoffIndependentCompressionStreams(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, lengths := range [][]int{
		{}, {0}, {1, 2, 3, 4, 8, 16, 64},
		{65536, 16, 32768, 0, 1, 131072, 1024},
	} {
		buf := make([]byte, SIZE_OF_SFNT_HEADER+len(lengths)*SIZE_OF_SFNT_TABLE_ENTRY)
		binary.BigEndian.PutUint16(buf[4:], uint16(len(lengths)))
		for i, length := range lengths {
			data := make([]byte, longAlign(uint32(length)))
			if i%2 == 0 {
				rng.Read(data[:length])
			} else {
				for j := range data[:length] {
					data[j] = byte(j % 7)
				}
			}
			record := buf[SIZE_OF_SFNT_HEADER+i*SIZE_OF_SFNT_TABLE_ENTRY:]
			copy(record, fmt.Sprintf("%04d", len(lengths)-i))
			binary.BigEndian.PutUint32(record[SFNT_OFFSET_CHECKSUM:], calcChecksum(data))
			binary.BigEndian.PutUint32(record[SFNT_OFFSET_OFFSET:], uint32(len(buf)))
			binary.BigEndian.PutUint32(record[SFNT_OFFSET_LENGTH:], uint32(length))
			buf = append(buf, data...)
		}
		checkWoffLegacy(t, buf)
	}
}

func checkWoffLegacy(t *testing.T, face []byte) {
	t.Helper()
	wantInput, gotInput := bytes.Clone(face), bytes.Clone(face)
	want, wantErr := legacySfnt2Woff(wantInput)
	got, gotErr := Sfnt2Woff(gotInput)
	if fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
		t.Fatalf("errors differ: got %v, want %v", gotErr, wantErr)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("WOFF differs: got %d bytes, want %d", len(got), len(want))
	}
	if !bytes.Equal(gotInput, wantInput) {
		t.Fatal("input checksum adjustment differs")
	}
}

func BenchmarkSfnt2Woff(b *testing.B) {
	face, err := os.ReadFile("../../d2renderers/d2fonts/ttf/SourceSansPro-Regular.ttf")
	if err != nil {
		b.Fatal(err)
	}
	face = UTF8CutFont(face, "D2 diagrams 0123 -> []")
	for _, tc := range []struct {
		name    string
		convert func([]byte) ([]byte, error)
	}{{"legacy", legacySfnt2Woff}, {"reuse", Sfnt2Woff}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if _, err := tc.convert(bytes.Clone(face)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
