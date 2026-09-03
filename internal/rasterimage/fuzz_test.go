package rasterimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func FuzzJPEGOrientation(f *testing.F) {
	img := image.NewGray(image.Rect(0, 0, 3, 2))
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, nil); err != nil {
		f.Fatal(err)
	}
	f.Add(encoded.Bytes())
	for orientation := uint8(1); orientation <= 8; orientation++ {
		f.Add(insertJPEGAPP1(encoded.Bytes(), makeEXIFOrientationPayload(binary.LittleEndian, orientation)))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzFirstFrame(t, data, "jpeg")
	})
}

func FuzzGIFFirstFrame(f *testing.F) {
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}}); err != nil {
		f.Fatal(err)
	}
	f.Add(encoded.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzFirstFrame(t, data, "gif")
	})
}

func FuzzPNGFirstFrame(f *testing.F) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		f.Fatal(err)
	}
	f.Add(encoded.Bytes())
	apng := fuzzAPNGSeed(f)
	f.Add(apng)
	f.Add(mutatePNGSeed(apng, "acTL", 0, func(payload []byte) { binary.BigEndian.PutUint32(payload[:4], 0) }))
	f.Add(mutatePNGSeed(apng, "acTL", 0, func(payload []byte) { binary.BigEndian.PutUint32(payload[:4], 2) }))
	f.Add(mutatePNGSeed(apng, "fcTL", 0, func(payload []byte) { binary.BigEndian.PutUint32(payload[:4], 1) }))
	f.Add(mutatePNGSeed(apng, "fdAT", 0, func(payload []byte) { binary.BigEndian.PutUint32(payload[:4], 3) }))
	idatFirst := fuzzIDATFirstAPNGSeed(f)
	f.Add(idatFirst)
	f.Add(mutatePNGSeed(idatFirst, "fcTL", 0, func(payload []byte) { binary.BigEndian.PutUint32(payload[4:8], 1) }))
	f.Add(mutatePNGSeed(idatFirst, "fcTL", 0, func(payload []byte) { payload[24] = 3 }))
	f.Add(mutatePNGSeed(idatFirst, "fcTL", 0, func(payload []byte) { payload[25] = 2 }))
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzFirstFrame(t, data, "png")
	})
}

func FuzzWebPFirstFrame(f *testing.F) {
	still, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(still)
	config, _, err := image.DecodeConfig(bytes.NewReader(still))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(fuzzAnimatedWebPSeed(still, config.Width, config.Height))
	unknown := appendWebPChunk(nil, "JUNK", []byte{1, 2, 3})
	f.Add(fuzzAnimatedWebPSeedWithPrefix(still, config.Width, config.Height, unknown))
	vp8x := make([]byte, 10)
	writeUint24(vp8x[4:7], config.Width-1)
	writeUint24(vp8x[7:10], config.Height-1)
	staticChunks := appendWebPChunk(nil, "VP8X", vp8x)
	staticChunks = appendWebPChunk(staticChunks, "ANIM", []byte{0xff})
	staticChunks = append(staticChunks, still[12:]...)
	f.Add(fuzzWebPFile(staticChunks))
	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzFirstFrame(t, data, "webp")
	})
}

func fuzzFirstFrame(t *testing.T, data []byte, format string) {
	t.Helper()
	if len(data) > 64<<10 {
		return
	}
	config, _, err := Config(context.Background(), data, format)
	if err != nil || config.Width > 128 || config.Height > 128 || int64(config.Width)*int64(config.Height) > 4_096 {
		return
	}
	decoded, err := DecodeFirst(context.Background(), data, format)
	if err != nil {
		// Header-valid inputs may still contain malformed compressed pixels. The
		// invariant is that successful decoding uses the probed logical canvas.
		return
	}
	if want := image.Rect(0, 0, config.Width, config.Height); decoded.Bounds() != want {
		t.Fatalf("DecodeFirst bounds = %v, Config = %v", decoded.Bounds(), want)
	}
}

func fuzzAPNGSeed(f *testing.F) []byte {
	f.Helper()
	poster := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	frame := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	encode := func(input image.Image) []byte {
		var output bytes.Buffer
		if err := png.Encode(&output, input); err != nil {
			f.Fatal(err)
		}
		return output.Bytes()
	}
	posterPNG := encode(poster)
	framePNG := encode(frame)
	chunkPayload := func(data []byte, want string) []byte {
		for offset := 8; offset < len(data); {
			length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
			if string(data[offset+4:offset+8]) == want {
				return data[offset+8 : offset+8+length]
			}
			offset += length + 12
		}
		f.Fatalf("PNG seed has no %s", want)
		return nil
	}
	result := append([]byte(nil), posterPNG[:8]...)
	result = appendPNGChunk(result, "IHDR", chunkPayload(posterPNG, "IHDR"))
	result = appendPNGChunk(result, "acTL", []byte{0, 0, 0, 1, 0, 0, 0, 0})
	result = appendPNGChunk(result, "IDAT", chunkPayload(posterPNG, "IDAT"))
	control := make([]byte, 26)
	binary.BigEndian.PutUint32(control[4:8], 1)
	binary.BigEndian.PutUint32(control[8:12], 1)
	binary.BigEndian.PutUint32(control[12:16], 1)
	binary.BigEndian.PutUint32(control[16:20], 1)
	result = appendPNGChunk(result, "fcTL", control)
	frameData := binary.BigEndian.AppendUint32(nil, 1)
	frameData = append(frameData, chunkPayload(framePNG, "IDAT")...)
	result = appendPNGChunk(result, "fdAT", frameData)
	return appendPNGChunk(result, "IEND", nil)
}

func fuzzIDATFirstAPNGSeed(f *testing.F) []byte {
	f.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		f.Fatal(err)
	}
	still := encoded.Bytes()
	result := append([]byte(nil), still[:8]...)
	result = appendPNGChunk(result, "IHDR", pngSeedChunkPayloads(still, "IHDR")[0])
	result = appendPNGChunk(result, "acTL", []byte{0, 0, 0, 1, 0, 0, 0, 0})
	control := make([]byte, 26)
	binary.BigEndian.PutUint32(control[4:8], 2)
	binary.BigEndian.PutUint32(control[8:12], 2)
	result = appendPNGChunk(result, "fcTL", control)
	for _, payload := range pngSeedChunkPayloads(still, "IDAT") {
		result = appendPNGChunk(result, "IDAT", payload)
	}
	return appendPNGChunk(result, "IEND", nil)
}

func mutatePNGSeed(data []byte, typeName string, occurrence int, mutate func([]byte)) []byte {
	result := append([]byte(nil), data[:8]...)
	for offset := 8; offset < len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		payload := append([]byte(nil), data[offset+8:offset+8+length]...)
		chunkType := string(data[offset+4 : offset+8])
		if chunkType == typeName {
			if occurrence == 0 {
				mutate(payload)
			} else {
				occurrence--
			}
		}
		result = appendPNGChunk(result, chunkType, payload)
		offset += length + 12
	}
	return result
}

func pngSeedChunkPayloads(data []byte, want string) [][]byte {
	var payloads [][]byte
	for offset := 8; offset < len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if string(data[offset+4:offset+8]) == want {
			payloads = append(payloads, data[offset+8:offset+8+length])
		}
		offset += length + 12
	}
	return payloads
}

func fuzzAnimatedWebPSeed(still []byte, width, height int) []byte {
	return fuzzAnimatedWebPSeedWithPrefix(still, width, height, nil)
}

func fuzzAnimatedWebPSeedWithPrefix(still []byte, width, height int, framePrefix []byte) []byte {
	vp8x := make([]byte, 10)
	vp8x[0] = 0x02
	writeUint24(vp8x[4:7], width-1)
	writeUint24(vp8x[7:10], height-1)
	frameHeader := make([]byte, 16)
	writeUint24(frameHeader[6:9], width-1)
	writeUint24(frameHeader[9:12], height-1)
	frameHeader[15] = 0x02
	chunks := appendWebPChunk(nil, "VP8X", vp8x)
	chunks = appendWebPChunk(chunks, "ANIM", make([]byte, 6))
	framePayload := append([]byte(nil), frameHeader...)
	framePayload = append(framePayload, framePrefix...)
	framePayload = append(framePayload, still[12:]...)
	chunks = appendWebPChunk(chunks, "ANMF", framePayload)
	return fuzzWebPFile(chunks)
}

func fuzzWebPFile(chunks []byte) []byte {
	result := make([]byte, 12)
	copy(result[:4], "RIFF")
	copy(result[8:12], "WEBP")
	result = append(result, chunks...)
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return result
}
