package png

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"

	"github.com/d2lang/d2/lib/version"
)

// SCALE is the static raster device scale used by PNG, PDF, and PPTX export.
const SCALE = 2.

var pngSignature = [...]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

const pngIENDChunk = "\x00\x00\x00\x00IEND\xae\x42\x60\x82"

const (
	pngChunkOverhead = 12
	pngChunkIHDR     = uint32(0x49484452) // IHDR
	pngChunkEXIF     = uint32(0x65584966) // eXIf
	pngChunkIEND     = uint32(0x49454e44) // IEND
	d2ExifIFDSize    = 38
)

// WriteExif writes D2's complete eXIf chunk. A streaming PNG encoder calls it
// after IHDR, before emitting image data, without retaining the encoded image.
func WriteExif(w io.Writer) error {
	dataSize, err := d2ExifDataSize(version.Version)
	if err != nil {
		return err
	}
	chunk := make([]byte, pngChunkOverhead+dataSize)
	writeD2ExifChunk(chunk, 0, version.Version, dataSize)
	n, err := w.Write(chunk)
	if err == nil && n != len(chunk) {
		return io.ErrShortWrite
	}
	return err
}

// AddExif returns png with D2's EXIF metadata inserted after IHDR. An existing
// EXIF chunk is replaced at its original position. The input is validated
// without copying chunk payloads so peak memory remains one input and one
// output buffer.
func AddExif(png []byte) ([]byte, error) {
	return addExif(png, false)
}

// AddExifInPlace returns png with D2's EXIF metadata inserted after IHDR. It
// reuses the input's backing storage when it has sufficient spare capacity and
// otherwise allocates an exact-sized result. On success, callers must consider
// the input and every alias of its backing storage consumed. Invalid input is
// never modified.
func AddExifInPlace(png []byte) ([]byte, error) {
	return addExif(png, true)
}

// AddExifToEncoderOutputInPlace inserts D2's EXIF metadata into a PNG emitted
// directly by an encoder. Unlike AddExifInPlace, it does not rescan and
// checksum every generated image-data chunk. It still verifies the fixed PNG
// envelope before modifying the input. Callers must not use it for untrusted
// or externally supplied PNG data.
func AddExifToEncoderOutputInPlace(png []byte) ([]byte, error) {
	const ihdrEnd = len(pngSignature) + pngChunkOverhead + 13
	if len(png) < ihdrEnd+len(pngIENDChunk) || !bytes.Equal(png[:len(pngSignature)], pngSignature[:]) {
		return nil, fmt.Errorf("not PNG encoder output")
	}
	if binary.BigEndian.Uint32(png[len(pngSignature):len(pngSignature)+4]) != 13 ||
		binary.BigEndian.Uint32(png[len(pngSignature)+4:len(pngSignature)+8]) != pngChunkIHDR {
		return nil, fmt.Errorf("PNG encoder output has an invalid IHDR chunk")
	}
	wantIHDRCRC := binary.BigEndian.Uint32(png[ihdrEnd-4 : ihdrEnd])
	if got := crc32.ChecksumIEEE(png[len(pngSignature)+4 : ihdrEnd-4]); got != wantIHDRCRC {
		return nil, fmt.Errorf("PNG encoder output has an invalid IHDR CRC")
	}
	if !bytes.Equal(png[len(png)-len(pngIENDChunk):], []byte(pngIENDChunk)) {
		return nil, fmt.Errorf("PNG encoder output has an invalid IEND chunk")
	}
	return addExifAt(png, ihdrEnd, ihdrEnd, true)
}

func addExif(png []byte, allowInPlace bool) ([]byte, error) {
	insertAt, replaceEnd, err := exifChunkLocation(png)
	if err != nil {
		return nil, err
	}
	return addExifAt(png, insertAt, replaceEnd, allowInPlace)
}

func addExifAt(png []byte, insertAt, replaceEnd int, allowInPlace bool) ([]byte, error) {
	model := version.Version
	exifDataSize, err := d2ExifDataSize(model)
	if err != nil {
		return nil, err
	}
	chunkSize := pngChunkOverhead + exifDataSize
	replacedSize := replaceEnd - insertAt
	if chunkSize > math.MaxInt-len(png)+replacedSize {
		return nil, fmt.Errorf("PNG with EXIF exceeds the platform integer domain")
	}
	outputSize := len(png) + chunkSize - replacedSize
	if allowInPlace && outputSize <= cap(png) {
		inputSize := len(png)
		output := png[:outputSize]
		copy(output[insertAt+chunkSize:], output[replaceEnd:inputSize])
		writeD2ExifChunk(output, insertAt, model, exifDataSize)
		return output, nil
	}
	output := make([]byte, outputSize)
	written := copy(output, png[:insertAt])
	written = writeD2ExifChunk(output, written, model, exifDataSize)
	written += copy(output[written:], png[replaceEnd:])
	if written != len(output) {
		return nil, fmt.Errorf("PNG EXIF output size %d differs from planned size %d", written, len(output))
	}
	return output, nil
}

func d2ExifDataSize(model string) (int, error) {
	if len(model) < 4 {
		return d2ExifIFDSize, nil
	}
	if len(model) > math.MaxInt32-d2ExifIFDSize-1 {
		return 0, fmt.Errorf("PNG EXIF model is too large: %d bytes", len(model))
	}
	return d2ExifIFDSize + len(model) + 1, nil
}

// exifChunkLocation returns the range to replace. Without an existing eXIf
// chunk it returns the empty range immediately after IHDR.
func exifChunkLocation(png []byte) (start, end int, err error) {
	if len(png) < len(pngSignature) || !bytes.Equal(png[:len(pngSignature)], pngSignature[:]) {
		return 0, 0, fmt.Errorf("not PNG data")
	}
	offset := len(pngSignature)
	insertion := 0
	existingStart, existingEnd := 0, 0
	foundIEND := false
	for chunkIndex := 0; offset < len(png); chunkIndex++ {
		if len(png)-offset < pngChunkOverhead {
			return 0, 0, fmt.Errorf("truncated PNG chunk at byte %d", offset)
		}
		dataLength := uint64(binary.BigEndian.Uint32(png[offset : offset+4]))
		if dataLength > math.MaxInt32 {
			return 0, 0, fmt.Errorf("PNG chunk at byte %d exceeds the maximum length: %d", offset, dataLength)
		}
		if dataLength > uint64(len(png)-offset-pngChunkOverhead) {
			return 0, 0, fmt.Errorf("truncated PNG chunk data at byte %d", offset)
		}
		chunkEnd := offset + pngChunkOverhead + int(dataLength)
		chunkType := binary.BigEndian.Uint32(png[offset+4 : offset+8])
		if chunkIndex == 0 {
			if chunkType != pngChunkIHDR {
				return 0, 0, fmt.Errorf("first PNG chunk is %q, want IHDR", png[offset+4:offset+8])
			}
			if dataLength != 13 {
				return 0, 0, fmt.Errorf("PNG IHDR length is %d, want 13", dataLength)
			}
			insertion = chunkEnd
		}
		wantCRC := binary.BigEndian.Uint32(png[chunkEnd-4 : chunkEnd])
		gotCRC := crc32.ChecksumIEEE(png[offset+4 : chunkEnd-4])
		if gotCRC != wantCRC {
			return 0, 0, fmt.Errorf("PNG chunk %q at byte %d has invalid CRC", png[offset+4:offset+8], offset)
		}
		if chunkType == pngChunkEXIF {
			if existingStart != 0 {
				return 0, 0, fmt.Errorf("PNG has multiple eXIf chunks")
			}
			existingStart, existingEnd = offset, chunkEnd
		}
		if chunkType == pngChunkIEND {
			if dataLength != 0 {
				return 0, 0, fmt.Errorf("PNG IEND length is %d, want 0", dataLength)
			}
			if chunkEnd != len(png) {
				return 0, 0, fmt.Errorf("PNG has %d trailing bytes after IEND", len(png)-chunkEnd)
			}
			foundIEND = true
		}
		offset = chunkEnd
	}
	if insertion == 0 {
		return 0, 0, fmt.Errorf("PNG has no IHDR chunk")
	}
	if !foundIEND {
		return 0, 0, fmt.Errorf("PNG has no IEND chunk")
	}
	if existingStart != 0 {
		return existingStart, existingEnd, nil
	}
	return insertion, insertion, nil
}

func writeD2ExifChunk(destination []byte, offset int, model string, dataSize int) int {
	binary.BigEndian.PutUint32(destination[offset:offset+4], uint32(dataSize))
	binary.BigEndian.PutUint32(destination[offset+4:offset+8], pngChunkEXIF)
	data := destination[offset+8 : offset+8+dataSize]

	// A PNG eXIf payload starts directly with the TIFF header, without the
	// "Exif\x00\x00" identifier used by JPEG APP1 segments.
	copy(data[0:4], "MM\x00\x2a")
	binary.BigEndian.PutUint32(data[4:8], 8)
	binary.BigEndian.PutUint16(data[8:10], 2)
	binary.BigEndian.PutUint16(data[10:12], 0x010f) // Make
	binary.BigEndian.PutUint16(data[12:14], 2)      // ASCII
	binary.BigEndian.PutUint32(data[14:18], 3)
	copy(data[18:22], "D2\x00\x00")
	binary.BigEndian.PutUint16(data[22:24], 0x0110) // Model
	binary.BigEndian.PutUint16(data[24:26], 2)      // ASCII
	binary.BigEndian.PutUint32(data[26:30], uint32(len(model)+1))
	if len(model) < 4 {
		clear(data[30:34])
		copy(data[30:34], model)
	} else {
		binary.BigEndian.PutUint32(data[30:34], d2ExifIFDSize)
		copy(data[d2ExifIFDSize:len(data)-1], model)
		data[len(data)-1] = 0
	}
	binary.BigEndian.PutUint32(data[34:38], 0)

	end := offset + pngChunkOverhead + dataSize
	crc := crc32.ChecksumIEEE(destination[offset+4 : end-4])
	binary.BigEndian.PutUint32(destination[end-4:end], crc)
	return end
}
