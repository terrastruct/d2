package rasterimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
	"time"
)

func TestJPEGEXIFOrientationAllTransforms(t *testing.T) {
	encoded := encodeOrientationTestJPEG(t)
	baseline, err := DecodeFirst(context.Background(), encoded, "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Bounds() != image.Rect(0, 0, 16, 10) {
		t.Fatalf("baseline bounds = %v", baseline.Bounds())
	}
	baselineCorners := []color.NRGBA{
		toNRGBA(baseline.At(0, 0)), toNRGBA(baseline.At(15, 0)),
		toNRGBA(baseline.At(0, 9)), toNRGBA(baseline.At(15, 9)),
	}
	for left := 0; left < len(baselineCorners); left++ {
		for right := left + 1; right < len(baselineCorners); right++ {
			if baselineCorners[left] == baselineCorners[right] {
				t.Fatalf("fixture corners %d and %d are not distinct: %#v", left, right, baselineCorners[left])
			}
		}
	}

	for orientation := uint8(1); orientation <= 8; orientation++ {
		t.Run(fmt.Sprintf("orientation_%d", orientation), func(t *testing.T) {
			var order binary.ByteOrder = binary.LittleEndian
			if orientation%2 == 0 {
				order = binary.BigEndian
			}
			data := insertJPEGAPP1(encoded, makeEXIFOrientationPayload(order, orientation))
			config, animated, err := Config(context.Background(), data, "jpeg")
			if err != nil {
				t.Fatal(err)
			}
			wantWidth, wantHeight := 16, 10
			if orientation >= 5 {
				wantWidth, wantHeight = wantHeight, wantWidth
			}
			if animated || config.Width != wantWidth || config.Height != wantHeight {
				t.Fatalf("Config = %+v animated=%v, want %dx%d still", config, animated, wantWidth, wantHeight)
			}
			decoded, err := DecodeFirst(context.Background(), data, "jpeg")
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds() != image.Rect(0, 0, wantWidth, wantHeight) {
				t.Fatalf("DecodeFirst bounds = %v, want %dx%d", decoded.Bounds(), wantWidth, wantHeight)
			}
			for y := 0; y < wantHeight; y++ {
				for x := 0; x < wantWidth; x++ {
					sourceX, sourceY := orientationSourcePoint(orientation, x, y, 16, 10)
					got := toNRGBA(decoded.At(x, y))
					want := toNRGBA(baseline.At(sourceX, sourceY))
					if got != want {
						t.Fatalf("pixel (%d,%d) = %#v, want source (%d,%d) %#v", x, y, got, sourceX, sourceY, want)
					}
				}
			}
		})
	}
}

func TestJPEGMalformedEXIFIsIgnored(t *testing.T) {
	encoded := encodeOrientationTestJPEG(t)
	baseline, err := DecodeFirst(context.Background(), encoded, "jpeg")
	if err != nil {
		t.Fatal(err)
	}
	invalidZero := makeEXIFOrientationPayload(binary.LittleEndian, 0)
	invalidNine := makeEXIFOrientationPayload(binary.BigEndian, 9)
	wrongType := makeEXIFOrientationPayload(binary.LittleEndian, 6)
	binary.LittleEndian.PutUint16(wrongType[18:20], 4)
	badOffset := makeEXIFOrientationPayload(binary.LittleEndian, 6)
	binary.LittleEndian.PutUint32(badOffset[10:14], math.MaxUint32)
	tests := [][]byte{
		[]byte("Exif\x00\x00"),
		[]byte("Exif\x00\x00ZZ\x00\x2a\x00\x00\x00\x08"),
		invalidZero,
		invalidNine,
		wrongType,
		badOffset,
		[]byte("http://ns.adobe.com/xap/1.0/\x00not-exif"),
	}
	for index, payload := range tests {
		t.Run(fmt.Sprintf("malformed_%d", index), func(t *testing.T) {
			data := insertJPEGAPP1(encoded, payload)
			config, _, err := Config(context.Background(), data, "jpeg")
			if err != nil {
				t.Fatal(err)
			}
			if config.Width != 16 || config.Height != 10 {
				t.Fatalf("Config = %+v, want unrotated 16x10", config)
			}
			decoded, err := DecodeFirst(context.Background(), data, "jpeg")
			if err != nil {
				t.Fatal(err)
			}
			for _, point := range []image.Point{{}, {15, 0}, {0, 9}, {15, 9}, {7, 4}} {
				if got, want := toNRGBA(decoded.At(point.X, point.Y)), toNRGBA(baseline.At(point.X, point.Y)); got != want {
					t.Fatalf("pixel %v = %#v, want normal %#v", point, got, want)
				}
			}
		})
	}
}

func TestJPEGEXIFOrientationCancellation(t *testing.T) {
	encoded := encodeOrientationTestJPEG(t)
	payload := make([]byte, 6+8+2+1_024*12+4)
	copy(payload, "Exif\x00\x00II")
	binary.LittleEndian.PutUint16(payload[8:10], 42)
	binary.LittleEndian.PutUint32(payload[10:14], 8)
	binary.LittleEndian.PutUint16(payload[14:16], 1_024)
	last := 16 + 1_023*12
	binary.LittleEndian.PutUint16(payload[last:last+2], exifOrientation)
	binary.LittleEndian.PutUint16(payload[last+2:last+4], 3)
	binary.LittleEndian.PutUint32(payload[last+4:last+8], 1)
	binary.LittleEndian.PutUint16(payload[last+8:last+10], 6)
	data := insertJPEGAPP1(encoded, payload)
	if orientation, err := jpegEXIFOrientation(newCancelAfterChecksContext(3), data); orientation != 1 || !errors.Is(err, context.Canceled) {
		t.Fatalf("jpegEXIFOrientation = %d, %v; want 1, context.Canceled", orientation, err)
	}
}

func encodeOrientationTestJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 16, 10))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetGray(x, y, color.Gray{Y: uint8(12 + x*9 + y*7)})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), encoded.Bytes()...)
}

func insertJPEGAPP1(jpegData, payload []byte) []byte {
	segment := []byte{0xff, jpegMarkerAPP1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:4], uint16(len(payload)+2))
	result := make([]byte, 0, len(jpegData)+len(segment)+len(payload))
	result = append(result, jpegData[:2]...)
	result = append(result, segment...)
	result = append(result, payload...)
	return append(result, jpegData[2:]...)
}

func makeEXIFOrientationPayload(order binary.ByteOrder, orientation uint8) []byte {
	payload := make([]byte, 6+8+2+12+4)
	copy(payload[:6], "Exif\x00\x00")
	if order == binary.LittleEndian {
		copy(payload[6:8], "II")
	} else {
		copy(payload[6:8], "MM")
	}
	order.PutUint16(payload[8:10], 42)
	order.PutUint32(payload[10:14], 8)
	order.PutUint16(payload[14:16], 1)
	order.PutUint16(payload[16:18], exifOrientation)
	order.PutUint16(payload[18:20], 3)
	order.PutUint32(payload[20:24], 1)
	order.PutUint16(payload[24:26], uint16(orientation))
	return payload
}

func orientationSourcePoint(orientation uint8, x, y, width, height int) (int, int) {
	switch orientation {
	case 2:
		return width - 1 - x, y
	case 3:
		return width - 1 - x, height - 1 - y
	case 4:
		return x, height - 1 - y
	case 5:
		return y, x
	case 6:
		return y, height - 1 - x
	case 7:
		return width - 1 - y, height - 1 - x
	case 8:
		return width - 1 - y, x
	default:
		return x, y
	}
}

func toNRGBA(value color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(value).(color.NRGBA)
}

func TestAnimatedGIFFirstFrame(t *testing.T) {
	palette := color.Palette{color.NRGBA{}, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}}
	first := image.NewPaletted(image.Rect(1, 1, 3, 3), palette)
	second := image.NewPaletted(image.Rect(0, 0, 4, 4), palette)
	for index := range first.Pix {
		first.Pix[index] = 1
	}
	for index := range second.Pix {
		second.Pix[index] = 2
	}
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}, Config: image.Config{ColorModel: palette, Width: 4, Height: 4}}); err != nil {
		t.Fatal(err)
	}
	config, animated, err := Config(context.Background(), encoded.Bytes(), "gif")
	if err != nil {
		t.Fatal(err)
	}
	if !animated || config.Width != 4 || config.Height != 4 {
		t.Fatalf("config = %+v animated=%v, want 4x4 animation", config, animated)
	}
	decoded, err := DecodeFirst(context.Background(), encoded.Bytes(), "gif")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != image.Rect(0, 0, 4, 4) || color.NRGBAModel.Convert(decoded.At(1, 1)).(color.NRGBA).R != 255 || color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA).A != 0 {
		t.Fatalf("unexpected GIF first frame: bounds=%v inside=%v outside=%v", decoded.Bounds(), decoded.At(1, 1), decoded.At(0, 0))
	}
}

func TestAPNGFirstAnimationFrameExcludesDefaultImage(t *testing.T) {
	poster := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for index := 0; index < len(poster.Pix); index += 4 {
		poster.Pix[index] = 255
		poster.Pix[index+3] = 255
	}
	frame := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	frame.SetNRGBA(0, 0, color.NRGBA{B: 255, A: 255})
	animated := makeAPNG(t, poster, frame, image.Pt(2, 1))

	config, isAnimated, err := Config(context.Background(), animated, "png")
	if err != nil {
		t.Fatal(err)
	}
	if !isAnimated || config.Width != 3 || config.Height != 2 {
		t.Fatalf("config = %+v animated=%v, want 3x2 animation", config, isAnimated)
	}
	decoded, err := DecodeFirst(context.Background(), animated, "png")
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(decoded.At(2, 1)).(color.NRGBA); got.B != 255 || got.A != 255 {
		t.Fatalf("first APNG frame pixel = %#v, want blue", got)
	}
	if got := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA); got.A != 0 {
		t.Fatalf("APNG poster leaked into animation frame: %#v", got)
	}
}

func TestAPNGManyTinyChunksUseBoundedContiguousMaterialization(t *testing.T) {
	poster := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	frame := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for index := range frame.Pix {
		frame.Pix[index] = byte(index*31 + index/7)
	}
	animated := makeChunkedAPNG(t, poster, frame, image.Point{}, 4_096, 1)
	plan, err := inspectPNG(context.Background(), animated)
	if err != nil {
		t.Fatal(err)
	}
	if plan.pngPrefixBytes == 0 || plan.pngFrameBytes == 0 {
		t.Fatalf("materialization plan = %+v, want prefix and frame bytes", plan)
	}
	materialized, err := buildFramePNG(context.Background(), animated, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(materialized) != cap(materialized) {
		t.Fatalf("materialized PNG len/cap = %d/%d, want one exact allocation", len(materialized), cap(materialized))
	}
	decoded, err := png.Decode(bytes.NewReader(materialized))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != frame.Bounds() {
		t.Fatalf("decoded bounds = %v, want %v", decoded.Bounds(), frame.Bounds())
	}

	base := makeChunkedAPNG(t, poster, frame, image.Point{}, 0, 0)
	baseAllocs := testing.AllocsPerRun(3, func() {
		if _, inspectErr := inspectPNG(context.Background(), base); inspectErr != nil {
			t.Fatal(inspectErr)
		}
	})
	manyAllocs := testing.AllocsPerRun(3, func() {
		if _, inspectErr := inspectPNG(context.Background(), animated); inspectErr != nil {
			t.Fatal(inspectErr)
		}
	})
	if manyAllocs > baseAllocs+4 {
		t.Fatalf("inspect allocations scale with chunks: base=%v many=%v", baseAllocs, manyAllocs)
	}

	cancelCtx := newCancelAfterChecksContext(25)
	if _, err := buildFramePNG(cancelCtx, animated, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("buildFramePNG cancellation error = %v, want context.Canceled", err)
	}
}

func TestPNGChunkChecksumCancellation(t *testing.T) {
	chunk := appendPNGChunk(nil, "tEXt", make([]byte, 1<<20))
	valid, err := validPNGChunkChecksum(newCancelAfterChecksContext(5), chunk)
	if valid || !errors.Is(err, context.Canceled) {
		t.Fatalf("validPNGChunkChecksum = %v, %v; want false, context.Canceled", valid, err)
	}
}

func TestCompositeFrameCancellation(t *testing.T) {
	canvas := image.Rect(0, 0, 1_024, 1_024)
	frame := image.Rect(256, 256, 768, 768)
	decoded := image.NewNRGBA(image.Rect(0, 0, frame.Dx(), frame.Dy()))

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := compositeFrame(canceled, canvas, frame, decoded, true); result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled compositeFrame = %v, %v; want nil, context.Canceled", result, err)
	}

	if result, err := compositeFrame(newCancelAfterChecksContext(4), canvas, frame, decoded, true); result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-composite cancellation = %v, %v; want nil, context.Canceled", result, err)
	}
}

func TestAPNGRejectsInvalidAnimationStructure(t *testing.T) {
	poster := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	frame := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	valid := makeAPNG(t, poster, frame, image.Pt(2, 1))

	tests := []struct {
		name     string
		mutate   func([]testPNGChunk) []testPNGChunk
		contains string
	}{
		{
			name: "duplicate animation control",
			mutate: func(chunks []testPNGChunk) []testPNGChunk {
				index := pngTestChunkIndex(t, chunks, "acTL", 0)
				duplicate := cloneTestPNGChunk(chunks[index])
				return insertTestPNGChunk(chunks, index+1, duplicate)
			},
			contains: "animation control",
		},
		{
			name: "animation control after IDAT",
			mutate: func(chunks []testPNGChunk) []testPNGChunk {
				controlIndex := pngTestChunkIndex(t, chunks, "acTL", 0)
				control := cloneTestPNGChunk(chunks[controlIndex])
				chunks = append(chunks[:controlIndex], chunks[controlIndex+1:]...)
				idatIndex := pngTestChunkIndex(t, chunks, "IDAT", 0)
				return insertTestPNGChunk(chunks, idatIndex+1, control)
			},
			contains: "animation control",
		},
		{
			name: "zero declared frames",
			mutate: func(chunks []testPNGChunk) []testPNGChunk {
				index := pngTestChunkIndex(t, chunks, "acTL", 0)
				binary.BigEndian.PutUint32(chunks[index].payload[:4], 0)
				return chunks
			},
			contains: "animation control",
		},
		{
			name: "declared frame count mismatch",
			mutate: func(chunks []testPNGChunk) []testPNGChunk {
				index := pngTestChunkIndex(t, chunks, "acTL", 0)
				binary.BigEndian.PutUint32(chunks[index].payload[:4], 2)
				return chunks
			},
			contains: "acTL declares 2",
		},
		{
			name: "frame control sequence",
			mutate: func(chunks []testPNGChunk) []testPNGChunk {
				index := pngTestChunkIndex(t, chunks, "fcTL", 0)
				binary.BigEndian.PutUint32(chunks[index].payload[:4], 1)
				return chunks
			},
			contains: "sequence number",
		},
		{
			name: "frame data sequence",
			mutate: func(chunks []testPNGChunk) []testPNGChunk {
				index := pngTestChunkIndex(t, chunks, "fdAT", 0)
				binary.BigEndian.PutUint32(chunks[index].payload[:4], 2)
				return chunks
			},
			contains: "sequence number",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunks := cloneTestPNGChunks(parseTestPNGChunks(t, valid))
			data := encodeTestPNGChunks(test.mutate(chunks))
			_, _, err := Config(context.Background(), data, "png")
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte(test.contains)) {
				t.Fatalf("Config error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestAPNGValidatesIDATFirstFrameControl(t *testing.T) {
	valid := makeIDATFirstAPNG(t, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	tests := []struct {
		name     string
		mutate   func([]byte)
		contains string
	}{
		{
			name: "geometry",
			mutate: func(control []byte) {
				binary.BigEndian.PutUint32(control[4:8], 1)
			},
			contains: "does not cover",
		},
		{
			name: "dispose",
			mutate: func(control []byte) {
				control[24] = 3
			},
			contains: "dispose or blend",
		},
		{
			name: "blend",
			mutate: func(control []byte) {
				control[25] = 2
			},
			contains: "dispose or blend",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunks := cloneTestPNGChunks(parseTestPNGChunks(t, valid))
			index := pngTestChunkIndex(t, chunks, "fcTL", 0)
			test.mutate(chunks[index].payload)
			_, _, err := Config(context.Background(), encodeTestPNGChunks(chunks), "png")
			if err == nil || !bytes.Contains([]byte(err.Error()), []byte(test.contains)) {
				t.Fatalf("Config error = %v, want error containing %q", err, test.contains)
			}
		})
	}
}

func TestAPNGAllowsDefaultFrameControlBeforeAnimationControl(t *testing.T) {
	frame := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	frame.SetNRGBA(1, 1, color.NRGBA{R: 255, A: 255})
	chunks := parseTestPNGChunks(t, makeIDATFirstAPNG(t, frame))
	animationControl := pngTestChunkIndex(t, chunks, "acTL", 0)
	frameControl := pngTestChunkIndex(t, chunks, "fcTL", 0)
	chunks[animationControl], chunks[frameControl] = chunks[frameControl], chunks[animationControl]
	data := encodeTestPNGChunks(chunks)

	config, animated, err := Config(context.Background(), data, "png")
	if err != nil {
		t.Fatal(err)
	}
	if !animated || config.Width != 2 || config.Height != 2 {
		t.Fatalf("Config = %+v animated=%v, want 2x2 animation", config, animated)
	}
	decoded, err := DecodeFirst(context.Background(), data, "png")
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(decoded.At(1, 1)).(color.NRGBA); got.R != 255 || got.A != 255 {
		t.Fatalf("default animation frame pixel = %#v, want opaque red", got)
	}

	animationControl = pngTestChunkIndex(t, chunks, "acTL", 0)
	withoutAnimationControl := append([]testPNGChunk(nil), chunks[:animationControl]...)
	withoutAnimationControl = append(withoutAnimationControl, chunks[animationControl+1:]...)
	if _, _, err := Config(context.Background(), encodeTestPNGChunks(withoutAnimationControl), "png"); err == nil {
		t.Fatal("Config accepted fcTL and IDAT without acTL")
	}
}

func TestAnimatedWebPFirstFrame(t *testing.T) {
	still, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	stillConfig, _, err := image.DecodeConfig(bytes.NewReader(still))
	if err != nil {
		t.Fatal(err)
	}
	animated := makeAnimatedWebP(t, still, stillConfig.Width, stillConfig.Height)
	config, isAnimated, err := Config(context.Background(), animated, "webp")
	if err != nil {
		t.Fatal(err)
	}
	if !isAnimated || config.Width != stillConfig.Width || config.Height != stillConfig.Height {
		t.Fatalf("config = %+v animated=%v, want %+v animation", config, isAnimated, stillConfig)
	}
	decoded, err := DecodeFirst(context.Background(), animated, "webp")
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != stillConfig.Width || decoded.Bounds().Dy() != stillConfig.Height {
		t.Fatalf("decoded bounds = %v, want %dx%d", decoded.Bounds(), stillConfig.Width, stillConfig.Height)
	}
}

func TestStaticWebPIgnoresStrayANIMWithoutAnimationFlag(t *testing.T) {
	still, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	stillConfig, _, err := image.DecodeConfig(bytes.NewReader(still))
	if err != nil {
		t.Fatal(err)
	}
	vp8x := make([]byte, 10)
	writeUint24(vp8x[4:7], stillConfig.Width-1)
	writeUint24(vp8x[7:10], stillConfig.Height-1)
	chunks := appendWebPChunk(nil, "VP8X", vp8x)
	chunks = appendWebPChunk(chunks, "ANIM", []byte{0xff})
	chunks = append(chunks, still[12:]...)
	static := makeTestWebP(chunks)

	config, animated, err := Config(context.Background(), static, "webp")
	if err != nil {
		t.Fatal(err)
	}
	if animated || config.Width != stillConfig.Width || config.Height != stillConfig.Height {
		t.Fatalf("config = %+v animated=%v, want %+v static", config, animated, stillConfig)
	}
	if _, err := DecodeFirst(context.Background(), static, "webp"); err != nil {
		t.Fatal(err)
	}
}

func TestAnimatedWebPSkipsUnknownPaddedFrameChunk(t *testing.T) {
	still, err := base64.StdEncoding.DecodeString(testWebPBase64)
	if err != nil {
		t.Fatal(err)
	}
	stillConfig, _, err := image.DecodeConfig(bytes.NewReader(still))
	if err != nil {
		t.Fatal(err)
	}
	unknown := appendWebPChunk(nil, "JUNK", []byte{1, 2, 3})
	animated := makeAnimatedWebPWithPrefix(t, still, stillConfig.Width, stillConfig.Height, unknown)
	config, isAnimated, err := Config(context.Background(), animated, "webp")
	if err != nil {
		t.Fatal(err)
	}
	if !isAnimated || config.Width != stillConfig.Width || config.Height != stillConfig.Height {
		t.Fatalf("config = %+v animated=%v, want %+v animation", config, isAnimated, stillConfig)
	}
	if _, err := DecodeFirst(context.Background(), animated, "webp"); err != nil {
		t.Fatal(err)
	}
}

func makeAPNG(t *testing.T, poster, frame image.Image, offset image.Point) []byte {
	t.Helper()
	return makeChunkedAPNG(t, poster, frame, offset, 0, 0)
}

func makeChunkedAPNG(t *testing.T, poster, frame image.Image, offset image.Point, prefixChunks, frameChunkBytes int) []byte {
	t.Helper()
	encode := func(input image.Image) []byte {
		var output bytes.Buffer
		if err := png.Encode(&output, input); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	posterPNG := encode(poster)
	framePNG := encode(frame)
	ihdr := pngChunkPayload(t, posterPNG, "IHDR")
	frameIDAT := bytes.Join(pngChunkPayloads(t, framePNG, "IDAT"), nil)
	result := append([]byte(nil), posterPNG[:8]...)
	result = appendPNGChunk(result, "IHDR", ihdr)
	result = appendPNGChunk(result, "acTL", []byte{0, 0, 0, 1, 0, 0, 0, 0})
	for index := 0; index < prefixChunks; index++ {
		result = appendPNGChunk(result, "tEXt", []byte("k\x00v"))
	}
	for _, payload := range pngChunkPayloads(t, posterPNG, "IDAT") {
		result = appendPNGChunk(result, "IDAT", payload)
	}
	control := make([]byte, 26)
	binary.BigEndian.PutUint32(control[4:8], uint32(frame.Bounds().Dx()))
	binary.BigEndian.PutUint32(control[8:12], uint32(frame.Bounds().Dy()))
	binary.BigEndian.PutUint32(control[12:16], uint32(offset.X))
	binary.BigEndian.PutUint32(control[16:20], uint32(offset.Y))
	control[25] = 0 // source blend
	result = appendPNGChunk(result, "fcTL", control)
	if frameChunkBytes <= 0 {
		frameChunkBytes = len(frameIDAT)
	}
	sequence := uint32(1)
	for offset := 0; offset < len(frameIDAT); offset += frameChunkBytes {
		end := offset + frameChunkBytes
		if end > len(frameIDAT) {
			end = len(frameIDAT)
		}
		framePayload := binary.BigEndian.AppendUint32(nil, sequence)
		framePayload = append(framePayload, frameIDAT[offset:end]...)
		result = appendPNGChunk(result, "fdAT", framePayload)
		sequence++
	}
	return appendPNGChunk(result, "IEND", nil)
}

func makeIDATFirstAPNG(t *testing.T, frame image.Image) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, frame); err != nil {
		t.Fatal(err)
	}
	still := encoded.Bytes()
	result := append([]byte(nil), still[:8]...)
	result = appendPNGChunk(result, "IHDR", pngChunkPayload(t, still, "IHDR"))
	result = appendPNGChunk(result, "acTL", []byte{0, 0, 0, 1, 0, 0, 0, 0})
	control := make([]byte, 26)
	binary.BigEndian.PutUint32(control[4:8], uint32(frame.Bounds().Dx()))
	binary.BigEndian.PutUint32(control[8:12], uint32(frame.Bounds().Dy()))
	result = appendPNGChunk(result, "fcTL", control)
	for _, payload := range pngChunkPayloads(t, still, "IDAT") {
		result = appendPNGChunk(result, "IDAT", payload)
	}
	return appendPNGChunk(result, "IEND", nil)
}

func pngChunkPayload(t *testing.T, data []byte, want string) []byte {
	t.Helper()
	for offset := 8; offset < len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if string(data[offset+4:offset+8]) == want {
			return append([]byte(nil), data[offset+8:offset+8+length]...)
		}
		offset += length + 12
	}
	t.Fatalf("PNG has no %s chunk", want)
	return nil
}

func pngChunkPayloads(t *testing.T, data []byte, want string) [][]byte {
	t.Helper()
	var payloads [][]byte
	for offset := 8; offset < len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if string(data[offset+4:offset+8]) == want {
			payloads = append(payloads, append([]byte(nil), data[offset+8:offset+8+length]...))
		}
		offset += length + 12
	}
	if len(payloads) == 0 {
		t.Fatalf("PNG has no %s chunk", want)
	}
	return payloads
}

func makeAnimatedWebP(t *testing.T, still []byte, width, height int) []byte {
	t.Helper()
	return makeAnimatedWebPWithPrefix(t, still, width, height, nil)
}

func makeAnimatedWebPWithPrefix(t *testing.T, still []byte, width, height int, framePrefix []byte) []byte {
	t.Helper()
	if len(still) < 20 || string(still[:4]) != "RIFF" || string(still[8:12]) != "WEBP" {
		t.Fatal("invalid static WebP fixture")
	}
	imageChunks := append([]byte(nil), still[12:]...)
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
	framePayload = append(framePayload, imageChunks...)
	chunks = appendWebPChunk(chunks, "ANMF", framePayload)
	return makeTestWebP(chunks)
}

func makeTestWebP(chunks []byte) []byte {
	result := make([]byte, 12, 12+len(chunks))
	copy(result[:4], "RIFF")
	copy(result[8:12], "WEBP")
	result = append(result, chunks...)
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return result
}

type testPNGChunk struct {
	typeName string
	payload  []byte
}

func parseTestPNGChunks(t *testing.T, data []byte) []testPNGChunk {
	t.Helper()
	var chunks []testPNGChunk
	for offset := 8; offset < len(data); {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		end := offset + length + 12
		if end > len(data) {
			t.Fatal("truncated test PNG")
		}
		chunks = append(chunks, testPNGChunk{
			typeName: string(data[offset+4 : offset+8]),
			payload:  append([]byte(nil), data[offset+8:offset+8+length]...),
		})
		offset = end
	}
	return chunks
}

func encodeTestPNGChunks(chunks []testPNGChunk) []byte {
	result := append([]byte(nil), []byte("\x89PNG\r\n\x1a\n")...)
	for _, chunk := range chunks {
		result = appendPNGChunk(result, chunk.typeName, chunk.payload)
	}
	return result
}

func cloneTestPNGChunk(chunk testPNGChunk) testPNGChunk {
	return testPNGChunk{typeName: chunk.typeName, payload: append([]byte(nil), chunk.payload...)}
}

func cloneTestPNGChunks(chunks []testPNGChunk) []testPNGChunk {
	result := make([]testPNGChunk, len(chunks))
	for index, chunk := range chunks {
		result[index] = cloneTestPNGChunk(chunk)
	}
	return result
}

func pngTestChunkIndex(t *testing.T, chunks []testPNGChunk, typeName string, occurrence int) int {
	t.Helper()
	for index, chunk := range chunks {
		if chunk.typeName == typeName {
			if occurrence == 0 {
				return index
			}
			occurrence--
		}
	}
	t.Fatalf("test PNG has no %s occurrence", typeName)
	return -1
}

func insertTestPNGChunk(chunks []testPNGChunk, index int, chunk testPNGChunk) []testPNGChunk {
	chunks = append(chunks, testPNGChunk{})
	copy(chunks[index+1:], chunks[index:])
	chunks[index] = chunk
	return chunks
}

type cancelAfterChecksContext struct {
	done      chan struct{}
	remaining int
	canceled  bool
}

func newCancelAfterChecksContext(checks int) *cancelAfterChecksContext {
	return &cancelAfterChecksContext{done: make(chan struct{}), remaining: checks}
}

func (ctx *cancelAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *cancelAfterChecksContext) Done() <-chan struct{} {
	if !ctx.canceled {
		ctx.remaining--
		if ctx.remaining <= 0 {
			ctx.canceled = true
			close(ctx.done)
		}
	}
	return ctx.done
}
func (ctx *cancelAfterChecksContext) Err() error {
	if ctx.canceled {
		return context.Canceled
	}
	return nil
}
func (ctx *cancelAfterChecksContext) Value(any) any { return nil }

const testWebPBase64 = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
