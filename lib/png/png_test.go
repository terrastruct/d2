package png

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	stdpng "image/png"
	"strings"
	"testing"

	"github.com/d2lang/d2/lib/version"
)

func TestStaticRasterScale(t *testing.T) {
	if SCALE != 2 {
		t.Fatalf("SCALE = %v, want 2", SCALE)
	}
}

func TestAddExifPreservesPNGAndAddsMetadata(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	source.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := stdpng.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}

	withExif, err := AddExif(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := stdpng.Decode(bytes.NewReader(withExif))
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Bounds().Eq(source.Bounds()) {
		t.Fatalf("bounds = %v, want %v", decoded.Bounds(), source.Bounds())
	}
	for x := 0; x < source.Bounds().Dx(); x++ {
		got := color.NRGBAModel.Convert(decoded.At(x, 0)).(color.NRGBA)
		want := source.NRGBAAt(x, 0)
		if got != want {
			t.Fatalf("pixel %d = %#v, want %#v", x, got, want)
		}
	}

	var exifChunks int
	for _, chunk := range parseTestPNGChunks(t, withExif) {
		if chunk.chunkType == pngChunkEXIF {
			exifChunks++
		}
	}
	if exifChunks != 1 {
		t.Fatalf("output has %d eXIf chunks, want 1", exifChunks)
	}
}

func TestAddExifPayload(t *testing.T) {
	originalVersion := version.Version
	version.Version = "v1.2.3-test"
	t.Cleanup(func() { version.Version = originalVersion })

	withExif, err := AddExif(encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1))))
	if err != nil {
		t.Fatal(err)
	}
	chunks := parseTestPNGChunks(t, withExif)
	if len(chunks) < 2 || chunks[0].chunkType != pngChunkIHDR || chunks[1].chunkType != pngChunkEXIF {
		t.Fatalf("first chunks are %#x, %#x; want IHDR, eXIf", chunks[0].chunkType, chunks[1].chunkType)
	}

	exifData := chunks[1].data
	if bytes.HasPrefix(exifData, []byte("Exif\x00\x00")) {
		t.Fatal("eXIf payload includes the JPEG-only Exif ID code")
	}
	if !bytes.HasPrefix(exifData, []byte{'M', 'M', 0, 42}) {
		t.Fatalf("eXIf payload begins % x, want big-endian TIFF header", exifData[:min(len(exifData), 6)])
	}
	makeValue, modelValue := parseTestD2ExifData(t, exifData)
	if makeValue != "D2" {
		t.Fatalf("EXIF Make = %q, want D2", makeValue)
	}
	if modelValue != version.Version {
		t.Fatalf("EXIF Model = %q, want %q", modelValue, version.Version)
	}
}

func TestAddExifModelEncoding(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })

	input := encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	models := []string{"", "a", "ab", "abc", "abcd", "abcde", "a\x00b", "µ", "🙂", "release-with-a-longer-build-identifier"}
	for _, model := range models {
		version.Version = model
		output, err := AddExif(input)
		if err != nil {
			t.Fatalf("model %q: %v", model, err)
		}
		chunks := parseTestPNGChunks(t, output)
		makeValue, modelValue := parseTestD2ExifData(t, chunks[1].data)
		if makeValue != "D2" || modelValue != model {
			t.Fatalf("EXIF Make/Model = %q/%q, want D2/%q", makeValue, modelValue, model)
		}
	}
}

func TestAddExifRejectsMalformedPNG(t *testing.T) {
	encoded := encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	chunks := parseTestPNGChunks(t, encoded)
	iend := chunks[len(chunks)-1]

	withExif, err := AddExif(encoded)
	if err != nil {
		t.Fatal(err)
	}
	withExifChunks := parseTestPNGChunks(t, withExif)
	exifChunk := withExifChunks[1]
	duplicateExif := make([]byte, 0, len(withExif)+exifChunk.end-exifChunk.start)
	duplicateExif = append(duplicateExif, withExif[:exifChunk.end]...)
	duplicateExif = append(duplicateExif, withExif[exifChunk.start:exifChunk.end]...)
	duplicateExif = append(duplicateExif, withExif[exifChunk.end:]...)

	badCRC := bytes.Clone(encoded)
	badCRC[chunks[0].end-1] ^= 0xff

	nonemptyIENDChunk := make([]byte, pngChunkOverhead+1)
	writeTestPNGChunk(nonemptyIENDChunk, 0, pngChunkIEND, []byte{1})
	nonemptyIEND := append(bytes.Clone(encoded[:iend.start]), nonemptyIENDChunk...)

	tests := []struct {
		name string
		png  []byte
		want string
	}{
		{name: "not PNG", png: []byte("not PNG"), want: "not PNG data"},
		{name: "missing IEND", png: encoded[:iend.start], want: "no IEND"},
		{name: "truncated chunk", png: encoded[:len(encoded)-1], want: "truncated PNG chunk"},
		{name: "trailing data", png: append(bytes.Clone(encoded), 0), want: "trailing bytes after IEND"},
		{name: "duplicate eXIf", png: duplicateExif, want: "multiple eXIf"},
		{name: "bad CRC", png: badCRC, want: "invalid CRC"},
		{name: "nonempty IEND", png: nonemptyIEND, want: "IEND length is 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := AddExif(tt.png)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("AddExif() error = %v, want containing %q", err, tt.want)
			}
			if output != nil {
				t.Fatalf("AddExif() returned %d output bytes on error", len(output))
			}
		})
	}
}

func TestAddExifDoesNotModifyInput(t *testing.T) {
	input := encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	want := bytes.Clone(input)
	if _, err := AddExif(input); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(input, want) {
		t.Fatal("AddExif modified its input")
	}
}

func TestAddExifInPlaceReusesSpareCapacityExactly(t *testing.T) {
	originalVersion := version.Version
	version.Version = "v1.2.3-test"
	t.Cleanup(func() { version.Version = originalVersion })

	input := encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 3, 2)))
	want, err := AddExif(input)
	if err != nil {
		t.Fatal(err)
	}
	consumed := make([]byte, len(input), len(want))
	copy(consumed, input)
	got, err := AddExifInPlace(consumed)
	if err != nil {
		t.Fatal(err)
	}
	if &got[0] != &consumed[0] {
		t.Fatal("AddExifInPlace did not reuse sufficient input capacity")
	}
	if !bytes.Equal(got, want) {
		t.Fatal("AddExifInPlace output differs from AddExif")
	}

	version.Version = "x"
	wantReplacement, err := AddExif(got)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := AddExifInPlace(got)
	if err != nil {
		t.Fatal(err)
	}
	if &replaced[0] != &got[0] {
		t.Fatal("AddExifInPlace did not reuse storage while shrinking EXIF")
	}
	if !bytes.Equal(replaced, wantReplacement) {
		t.Fatal("in-place EXIF replacement differs from AddExif")
	}
}

func TestAddExifInPlaceDoesNotModifyInvalidInput(t *testing.T) {
	encoded := encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	input := make([]byte, len(encoded), len(encoded)+32)
	copy(input, encoded)
	input[len(input)-1] ^= 0xff
	fullBacking := input[:cap(input)]
	for index := len(input); index < len(fullBacking); index++ {
		fullBacking[index] = 0xa5
	}
	want := bytes.Clone(fullBacking)
	if _, err := AddExifInPlace(input); err == nil {
		t.Fatal("AddExifInPlace accepted an invalid PNG")
	}
	if !bytes.Equal(fullBacking, want) {
		t.Fatal("AddExifInPlace modified invalid input backing storage")
	}
}

func TestAddExifInPlaceCapacityAndReplacementMatrix(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })
	base := encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 9, 7)))

	type sourceCase struct {
		name  string
		model *string
	}
	short, long := "x", "source-release-with-a-long-build-identifier"
	for _, sourceCase := range []sourceCase{
		{name: "no existing EXIF"},
		{name: "short existing EXIF", model: &short},
		{name: "long existing EXIF", model: &long},
	} {
		t.Run(sourceCase.name, func(t *testing.T) {
			source := base
			if sourceCase.model != nil {
				version.Version = *sourceCase.model
				var err error
				source, err = AddExif(base)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, targetModel := range []string{"", "v1.2.3", "target-release-with-an-even-longer-build-identifier"} {
				version.Version = targetModel
				want, err := AddExif(source)
				if err != nil {
					t.Fatal(err)
				}
				capacities := []int{len(source), max(len(source), len(want)-1), max(len(source), len(want)), max(len(source), len(want)+31)}
				seen := make(map[int]bool)
				for _, capacity := range capacities {
					if seen[capacity] {
						continue
					}
					seen[capacity] = true
					storage := make([]byte, len(source), capacity)
					copy(storage, source)
					got, err := AddExifInPlace(storage)
					if err != nil {
						t.Fatalf("target %q capacity %d: %v", targetModel, capacity, err)
					}
					if !bytes.Equal(got, want) {
						t.Fatalf("target %q capacity %d differs from AddExif", targetModel, capacity)
					}
					reused := &got[0] == &storage[0]
					if wantReuse := capacity >= len(want); reused != wantReuse {
						t.Fatalf("target %q capacity %d reused=%v, want %v", targetModel, capacity, reused, wantReuse)
					}
				}
			}
		})
	}
}

func TestAddExifToEncoderOutputInPlaceMatchesValidatedPath(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })
	for _, model := range []string{"", "dev", "v1.2.3-test", "release-with-a-long-build-identifier"} {
		version.Version = model
		for _, bounds := range []image.Rectangle{
			image.Rect(0, 0, 1, 1),
			image.Rect(0, 0, 37, 29),
			image.Rect(-3, -2, 67, 51),
		} {
			source := image.NewNRGBA(bounds)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					source.SetNRGBA(x, y, color.NRGBA{
						R: uint8(x*31 + y*17), G: uint8(x*7 + y*13), B: uint8(x ^ y), A: uint8(64 + (x-y)&191),
					})
				}
			}
			input := encodeTestPNG(t, source)
			want, err := AddExif(input)
			if err != nil {
				t.Fatal(err)
			}
			storage := make([]byte, len(input), len(want))
			copy(storage, input)
			got, err := AddExifToEncoderOutputInPlace(storage)
			if err != nil {
				t.Fatalf("model %q bounds %v: %v", model, bounds, err)
			}
			if &got[0] != &storage[0] {
				t.Fatalf("model %q bounds %v did not reuse sufficient capacity", model, bounds)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("model %q bounds %v differs from validated AddExif", model, bounds)
			}
		}
	}
}

func TestAddExifToEncoderOutputInPlaceRejectsInvalidEnvelopeWithoutModification(t *testing.T) {
	encoded := encodeTestPNG(t, image.NewNRGBA(image.Rect(0, 0, 3, 2)))
	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{name: "signature", mutate: func(input []byte) { input[0] ^= 0xff }},
		{name: "IHDR length", mutate: func(input []byte) { input[len(pngSignature)+3] = 12 }},
		{name: "IHDR type", mutate: func(input []byte) { input[len(pngSignature)+4] = 'X' }},
		{name: "IHDR CRC", mutate: func(input []byte) { input[len(pngSignature)+pngChunkOverhead+12] ^= 0xff }},
		{name: "IEND", mutate: func(input []byte) { input[len(input)-1] ^= 0xff }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := make([]byte, len(encoded), len(encoded)+128)
			copy(input, encoded)
			tt.mutate(input)
			fullBacking := input[:cap(input)]
			for index := len(input); index < len(fullBacking); index++ {
				fullBacking[index] = 0xa5
			}
			want := bytes.Clone(fullBacking)
			if output, err := AddExifToEncoderOutputInPlace(input); err == nil || output != nil {
				t.Fatalf("output/error = %d/%v, want nil/error", len(output), err)
			}
			if !bytes.Equal(fullBacking, want) {
				t.Fatal("invalid encoder output was modified")
			}
		})
	}
}

func TestAddExifPreservesChunks(t *testing.T) {
	originalVersion := version.Version
	t.Cleanup(func() { version.Version = originalVersion })

	source := image.NewPaletted(image.Rect(0, 0, 3, 2), color.Palette{color.Black, color.White})
	source.SetColorIndex(2, 1, 1)
	input := encodeTestPNG(t, source)
	inputChunks := parseTestPNGChunks(t, input)
	version.Version = "v1.2.3"
	withExif, err := AddExif(input)
	if err != nil {
		t.Fatal(err)
	}
	withExifChunks := parseTestPNGChunks(t, withExif)
	if len(withExifChunks) != len(inputChunks)+1 || withExifChunks[1].chunkType != pngChunkEXIF {
		t.Fatal("output chunks do not contain one eXIf chunk after IHDR")
	}
	if !bytes.Equal(withExif[:withExifChunks[1].start], input[:inputChunks[0].end]) ||
		!bytes.Equal(withExif[withExifChunks[1].end:], input[inputChunks[0].end:]) {
		t.Fatal("adding EXIF changed another PNG chunk")
	}

	version.Version = "release-with-a-longer-build-identifier"
	replaced, err := AddExif(withExif)
	if err != nil {
		t.Fatal(err)
	}
	replacedChunks := parseTestPNGChunks(t, replaced)
	if len(replacedChunks) != len(withExifChunks) || replacedChunks[1].chunkType != pngChunkEXIF {
		t.Fatal("replacing EXIF changed the chunk structure")
	}
	if !bytes.Equal(replaced[:replacedChunks[1].start], withExif[:withExifChunks[1].start]) ||
		!bytes.Equal(replaced[replacedChunks[1].end:], withExif[withExifChunks[1].end:]) {
		t.Fatal("replacing EXIF changed another PNG chunk")
	}
}

func encodeTestPNG(t testing.TB, source image.Image) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := stdpng.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

type testPNGChunk struct {
	chunkType uint32
	data      []byte
	start     int
	end       int
}

func parseTestPNGChunks(t testing.TB, input []byte) []testPNGChunk {
	t.Helper()
	if len(input) < len(pngSignature) || !bytes.Equal(input[:len(pngSignature)], pngSignature[:]) {
		t.Fatal("invalid PNG signature")
	}
	var chunks []testPNGChunk
	for offset := len(pngSignature); offset < len(input); {
		if len(input)-offset < pngChunkOverhead {
			t.Fatalf("truncated chunk at %d", offset)
		}
		dataLength := int(binary.BigEndian.Uint32(input[offset : offset+4]))
		end := offset + pngChunkOverhead + dataLength
		if end > len(input) {
			t.Fatalf("truncated chunk data at %d", offset)
		}
		chunks = append(chunks, testPNGChunk{
			chunkType: binary.BigEndian.Uint32(input[offset+4 : offset+8]),
			data:      input[offset+8 : end-4],
			start:     offset,
			end:       end,
		})
		offset = end
	}
	return chunks
}

func writeTestPNGChunk(destination []byte, offset int, chunkType uint32, data []byte) int {
	binary.BigEndian.PutUint32(destination[offset:offset+4], uint32(len(data)))
	binary.BigEndian.PutUint32(destination[offset+4:offset+8], chunkType)
	copy(destination[offset+8:offset+8+len(data)], data)
	end := offset + pngChunkOverhead + len(data)
	binary.BigEndian.PutUint32(destination[end-4:end], crc32.ChecksumIEEE(destination[offset+4:end-4]))
	return end
}

func parseTestD2ExifData(t testing.TB, data []byte) (makeValue, modelValue string) {
	t.Helper()
	if len(data) < 38 {
		t.Fatalf("EXIF data has %d bytes, want at least 38", len(data))
	}
	if !bytes.Equal(data[:4], []byte{'M', 'M', 0, 42}) {
		t.Fatalf("EXIF header = % x, want MM 00 2a", data[:4])
	}
	if binary.BigEndian.Uint32(data[4:8]) != 8 || binary.BigEndian.Uint16(data[8:10]) != 2 {
		t.Fatalf("EXIF root IFD header is invalid")
	}
	makeValue = parseTestExifASCII(t, data, 10, 0x010f)
	modelValue = parseTestExifASCII(t, data, 22, 0x0110)
	if binary.BigEndian.Uint32(data[34:38]) != 0 {
		t.Fatalf("EXIF next-IFD offset is not zero")
	}
	expectedSize := 38
	if len(modelValue) >= 4 {
		expectedSize += len(modelValue) + 1
	}
	if len(data) != expectedSize {
		t.Fatalf("EXIF data has %d bytes, want %d", len(data), expectedSize)
	}
	return makeValue, modelValue
}

func parseTestExifASCII(t testing.TB, data []byte, entryOffset int, wantTag uint16) string {
	t.Helper()
	if binary.BigEndian.Uint16(data[entryOffset:entryOffset+2]) != wantTag {
		t.Fatalf("EXIF tag at %d = %#x, want %#x", entryOffset, binary.BigEndian.Uint16(data[entryOffset:entryOffset+2]), wantTag)
	}
	if binary.BigEndian.Uint16(data[entryOffset+2:entryOffset+4]) != 2 {
		t.Fatalf("EXIF tag %#x is not ASCII", wantTag)
	}
	count := int(binary.BigEndian.Uint32(data[entryOffset+4 : entryOffset+8]))
	if count == 0 {
		t.Fatalf("EXIF tag %#x has an empty encoded value", wantTag)
	}
	var value []byte
	if count <= 4 {
		value = data[entryOffset+8 : entryOffset+8+count]
	} else {
		valueOffset := int(binary.BigEndian.Uint32(data[entryOffset+8 : entryOffset+12]))
		if valueOffset < 0 || valueOffset > len(data)-count {
			t.Fatalf("EXIF tag %#x points outside the payload", wantTag)
		}
		value = data[valueOffset : valueOffset+count]
	}
	if value[len(value)-1] != 0 {
		t.Fatalf("EXIF tag %#x is not NUL-terminated", wantTag)
	}
	return string(value[:len(value)-1])
}

func BenchmarkAddExif(b *testing.B) {
	input := benchmarkExifInput(b)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		output, err := AddExif(input)
		if err != nil {
			b.Fatal(err)
		}
		if len(output) <= len(input) {
			b.Fatalf("output size = %d, want greater than input size %d", len(output), len(input))
		}
	}
}

func BenchmarkAddExifInPlace(b *testing.B) {
	input := benchmarkExifInput(b)
	want, err := AddExif(input)
	if err != nil {
		b.Fatal(err)
	}
	storage := make([]byte, len(input), len(want))
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		storage = storage[:len(input)]
		copy(storage, input)
		b.StartTimer()
		output, err := AddExifInPlace(storage)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(output, want) {
			b.Fatal("in-place output differs")
		}
	}
}

func BenchmarkAddExifToEncoderOutputInPlace(b *testing.B) {
	input := benchmarkExifInput(b)
	want, err := AddExif(input)
	if err != nil {
		b.Fatal(err)
	}
	storage := make([]byte, len(input), len(want))
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		storage = storage[:len(input)]
		copy(storage, input)
		b.StartTimer()
		output, err := AddExifToEncoderOutputInPlace(storage)
		if err != nil {
			b.Fatal(err)
		}
		if !bytes.Equal(output, want) {
			b.Fatal("encoder-output path differs")
		}
	}
}

func benchmarkExifInput(b *testing.B) []byte {
	b.Helper()
	source := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := range source.Bounds().Dy() {
		for x := range source.Bounds().Dx() {
			source.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x*31 + y*17),
				G: uint8(x*7 + y*13),
				B: uint8(x ^ y),
				A: 0xff,
			})
		}
	}
	var encoded bytes.Buffer
	encoder := stdpng.Encoder{CompressionLevel: stdpng.NoCompression}
	if err := encoder.Encode(&encoded, source); err != nil {
		b.Fatal(err)
	}
	return encoded.Bytes()
}
