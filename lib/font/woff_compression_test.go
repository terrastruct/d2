package font

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"math/rand"
	"os"
	"sync"
	"testing"
)

func TestWoffCompressionRetainedBuffer(t *testing.T) {
	for _, capacity := range []int{0, 1024, maxRetainedWoffBuffer, maxRetainedWoffBuffer + 1} {
		compression := acquireWoffCompression()
		compression.buffer = *bytes.NewBuffer(bytes.Repeat([]byte{0x7f}, capacity))
		// Inspect cleanup while this test still owns the workspace. Pool.Get is
		// not required to return the item most recently released.
		backing := compression.buffer.Bytes()
		retained := compression.buffer.Cap() <= maxRetainedWoffBuffer
		resetWoffCompression(compression)
		if compression.buffer.Len() != 0 || compression.buffer.Cap() > maxRetainedWoffBuffer {
			t.Fatalf("retained buffer len/cap = %d/%d", compression.buffer.Len(), compression.buffer.Cap())
		}
		if !retained && compression.buffer.Cap() != 0 {
			t.Fatal("oversized table buffer retained")
		}
		if retained && !bytes.Equal(backing, make([]byte, len(backing))) {
			t.Fatal("retained table bytes were not cleared")
		}
		releaseWoffCompression(compression)
	}
}

func TestWoffCompressionIndependentStreams(t *testing.T) {
	testWoffCompressionIndependentStreams(t)
}

func testWoffCompressionIndependentStreams(t *testing.T) {
	t.Helper()
	compression := acquireWoffCompression()
	defer releaseWoffCompression(compression)
	rng := rand.New(rand.NewSource(2))
	for _, size := range []int{262144, 1, 0, 65536, 8, 32768} {
		data := make([]byte, size)
		rng.Read(data)
		var want bytes.Buffer
		writer := zlib.NewWriter(&want)
		writer.Write(data)
		writer.Flush()
		writer.Close()
		got := compression.compressTable(data)
		if !bytes.Equal(got, want.Bytes()) {
			t.Fatalf("compressed stream differs at size %d", size)
		}
	}
}

func TestSfnt2WoffConcurrentAndOwnedOutput(t *testing.T) {
	face, err := os.ReadFile("../../d2renderers/d2fonts/ttf/SourceSansPro-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	face = UTF8CutFont(face, "D2 diagram Ω À 0123456789")
	want, err := legacySfnt2Woff(bytes.Clone(face))
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			var outputs [][]byte
			for range 8 {
				got, err := Sfnt2Woff(bytes.Clone(face))
				if err != nil {
					t.Error(err)
					return
				}
				outputs = append(outputs, got)
			}
			for _, output := range outputs {
				if !bytes.Equal(output, want) {
					t.Error("conversion differs or later pool use modified returned WOFF")
				}
			}
		})
	}
	workers.Wait()
}

func TestSfnt2WoffPoolAfterMalformedInput(t *testing.T) {
	// The first table is valid and borrows a compressor. The short head table
	// panics afterwards, exercising deferred return of that workspace.
	buf := make([]byte, SIZE_OF_SFNT_HEADER+2*SIZE_OF_SFNT_TABLE_ENTRY+8)
	binary.BigEndian.PutUint16(buf[4:], 2)
	for i, tag := range []string{"aaaa", "head"} {
		record := buf[SIZE_OF_SFNT_HEADER+i*SIZE_OF_SFNT_TABLE_ENTRY:]
		copy(record, tag)
		binary.BigEndian.PutUint32(record[SFNT_OFFSET_OFFSET:], uint32(SIZE_OF_SFNT_HEADER+2*SIZE_OF_SFNT_TABLE_ENTRY+4*i))
		binary.BigEndian.PutUint32(record[SFNT_OFFSET_LENGTH:], 4)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("malformed head unexpectedly accepted")
			}
		}()
		Sfnt2Woff(buf)
	}()
	testWoffCompressionIndependentStreams(t)
}
